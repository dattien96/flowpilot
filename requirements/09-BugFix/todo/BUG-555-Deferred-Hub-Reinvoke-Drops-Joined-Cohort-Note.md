# BUG-555 — deferred hub reinvoke drops the joined cohort note; every later turn sees bare "Agent results ready" → escalate loop

## Status
RESOLVED — CA-1102 (2026-10-02). Regression tests in
`apps/local-runner/internal/runner/bug555_deferred_reinvoke_drops_join_note_test.go`
verified RED→GREEN (the captured prompt literally showed the note inside
the invisible `[FlowPilot system note]` wrapper before the fix).
Live-found during PrivateVault CP-02 vibe run `run-38799` (2026-10-02,
observed 3× at ~07:05, ~07:59, ~08:17).

### Existing-test contract update (BUG-275)
`TestAutoReinvokeHubWithNoteDedupesEmbeddedNoteFromPendingContext`'s old
assertion ("non-embedded notes remain in pendingAgentContext") encoded
the pre-fix semantics. Under the drain-all contract those notes are
CONSUMED into the same inline prompt — strictly stronger delivery, never
lost. The test was updated to assert the queue is empty after schedule;
the prompt-level embedding itself is pinned by the bug555 tests.

## Symptom
A cohort join completing while the hub still had `turnInFlight=true`
deferred its reinvoke correctly — but the deferred drain fired plain
`maybeAutoReinvokeHub("")`, so the synthesis turn prompt was the bare
generic `autoReinvokePromptText()`. The joined note survived only in
`pendingAgentContext`, which `composeAgentContextBlock` renders as a
"[FlowPilot system note]" block that resumed-thread providers do not
surface as a visible user message (the exact reason
`maybeAutoReinvokeHubWithNote` embeds the note inline — BUG-synthesis-hang).

Result: the hub synthesized with zero agent context → submitted
`escalate` (or stale `changes_requested`) → park. Operator `continue` +
feedback resumed via `resumeFlowWithFeedback`'s generic tail —
`maybeAutoReinvokeHubWithNote(resumeNote)` carried the feedback but the
deferred join note STILL stayed parked in `pendingAgentContext` → hub
saw "feedback + nothing to synthesize" → escalate again. Unbounded
park→continue→escalate loop; observed burning rounds 3–5 of the reinvoke
cap until `hub_reinvoke_cap_blocked` wedged it.

## Root cause
`maybeAutoReinvokeHubWithNote`'s deferred path (interactive_service.go)
arms only the bool `pendingHubReinvoke` — the note text is left solely
in `pendingAgentContext`. Every drain site (runTurn post-completion,
`notifyTurnIdle`, `drainArmedPendingHubReinvoke`, hub_stall watchdog)
then fires `maybeAutoReinvokeHub("")` for it. The schedule path embeds
only its `cohortNote` parameter and merely de-dupes one exact copy out
of `pendingAgentContext` — it never pulls owed context INTO the prompt.

## Fix
In `maybeAutoReinvokeHubWithNote`'s schedule path, drain
`parent.pendingAgentContext` and fold all owed entries into the inline
prompt (dedupe exact cohortNote match, preserving BUG-275). One-site
fix covers every drain path: deferred reinvokes, cap-blocked resumes,
and `resumeFlowWithFeedback`'s generic tail all now deliver owed context
inline. `pendingHubReinvokePrompt` (hub.notify full-prompt channel,
BUG-284) is untouched — different semantics.

Retry safety: `scheduleChildTurn`'s start-fail re-arm stashes the fully
composed prompt (notes included) into `pendingHubReinvokePrompt`, so a
transient `turn_in_progress`/`gate_in_progress` retry loses nothing.

## Provider scope
Provider-agnostic: the loss is in runner-side scheduling, not adapters —
exactly the provider-independence WithNote was created for.
