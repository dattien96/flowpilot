# CA-1102 — BUG-555: deferred/plain hub reinvoke now embeds owed agent context inline

## Change

`internal/runner/interactive_service.go` — `maybeAutoReinvokeHubWithNote`
schedule path: drain `parent.pendingAgentContext` and fold ALL owed
entries into the inline prompt (exact-match dedupe of cohortNote
preserved), instead of removing only the one exact cohortNote copy.

## Why

Live run-38799 (PrivateVault CP-02): a cohort join landing while a hub
turn was in flight armed only the bool `pendingHubReinvoke`; the joined
note stayed solely in `pendingAgentContext`. Every drain site (runTurn
post-completion, `notifyTurnIdle`, `drainArmedPendingHubReinvoke`,
hub_stall watchdog, resume tail) then fired plain
`maybeAutoReinvokeHub("")` → prompt = bare `autoReinvokePromptText()`,
and the note reached the hub only inside `composeAgentContextBlock`'s
`[FlowPilot system note]` wrapper — invisible to resumed-thread
providers (the reason `WithNote` exists, BUG-synthesis-hang). Hub
synthesized nothing → escalate → park → continue loop until
`hub_reinvoke_cap_blocked`. Observed 3× in one run.

One-site fix covers every loss path: deferred drains, cap-blocked
resumes, and `resumeFlowWithFeedback`'s generic tail all schedule
through this path and now carry owed context inline.
`pendingHubReinvokePrompt` (hub.notify full-prompt channel, BUG-284)
untouched — different semantics.

## Retry/durability parity

`scheduleChildTurn`'s start-fail re-arm stashes the fully composed
prompt (notes included) into `pendingHubReinvokePrompt`, so transient
`turn_in_progress`/`gate_in_progress` retries lose nothing. The
schedule→startTurn window where the note lived only in the prompt is
identical to the pre-existing cohortNote removal ordering (BUG-275
already removed it before `scheduleChildTurn`).

## Existing-test contract update

`TestAutoReinvokeHubWithNoteDedupesEmbeddedNoteFromPendingContext`
(BUG-275): old assertion required non-embedded notes to REMAIN queued.
Under the new contract they are delivered inline — strictly stronger,
never lost. Test updated to assert the queue drains empty; prompt-level
delivery is pinned by the new bug555 tests. Semantic change documented
in BUG-555 Status section.

## Tests

- `bug555_deferred_reinvoke_drops_join_note_test.go` (reproduce-first,
  RED captured the literal `[FlowPilot system note]` wrapper in the
  prompt before the fix):
  - D1: join during in-flight turn → deferred → idle drain embeds note
    inline.
  - D2: plain reinvoke pulls owed pendingAgentContext inline + drains it.
  - D3: join-note dedupe — embedded exactly once.
  - D4: nothing owed → bare generic tail unchanged.
- Related reinvoke/join family green: Bug275/289/293/307/318/430/454/
  471/542/550/551 + AutoReinvokeHub tests.

## Provider scope

Provider-agnostic by construction: runner-side prompt assembly, no
adapter/session code touched — same reasoning `WithNote` was built on.

## GitNexus

`impact` on the touched symbol path: risk LOW. `detect-changes` clean.
