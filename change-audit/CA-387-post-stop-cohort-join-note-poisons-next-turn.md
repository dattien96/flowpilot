# CA-387 — Post-Stop cohort join note no longer poisons the next turn

## Summary

Fixed BUG-307, found live on run-19500 (Review Loop) while verifying CP-51 A8.
Stop correctly cancelled 2 in-flight reviewers and the stop-generation fence
correctly held (A8 itself passed — no revival, no stale child re-entry). But
the cohort's join-barrier completing as part of that same Stop request queued
a "synthesize and call the flow's control tool" note into
`pendingAgentContext` unconditionally. Nothing ever drains that note once the
loop is sealed, so it silently rode along (via BUG-122's
`composeAgentContextBlock`) into the next turn on the run — after a restart,
an ordinary chat follow-up ("lan truoc fix gi vay") inherited the stale
instruction, and the hub-stall watchdog (BUG-289 F-0) timed out 2 minutes
later waiting for progress that could never come.

Fix: a new `loopSealedForReinvoke` predicate (true only for `"stopped"`/
`"done"` — NOT `"blocked"`/`"paused"`, which a later Continue/Resume
legitimately drains) now guards every one of the 6 live call sites that build
a cohort-join synthesis note before queuing it. `lastCohortNote` (the
BUG-233 GateReason diagnostic fallback) stays unconditional — only the
provider-facing queue is guarded.

## Cross-provider parity

Classification: **Case 1, shared code, no per-provider branch.** All 6 touched
call sites (in `interactive_service.go` and `cohort_stall.go`) operate purely
on `AgentLoopState.Status` and the run's own bookkeeping — none inspect
`ProviderKey`. Verified with a Grok-tagged variant of the regression test
(`TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContextGrok`) proving
identical behavior; no separate Grok code path exists here (unlike BUG-306,
which needed a literal `overlayRawGrokTurnPrompts` — this mechanism has no
provider-specific twin to check).

## additive-tests-only compliance

Only a new test file was added
(`bug307_post_stop_cohort_join_poisons_pendingcontext_test.go`, 3 tests); no
existing test file was modified. `isSystemPrompt`, `composeAgentContextBlock`,
and the boot-recovery `joinRecoveredCohort` path were deliberately left
untouched — this fix is confined to whether a cohort-join note is queued at
all, not to how notes are later classified or rendered.

## Verification

- git-stash: with the production fix stashed (and the new unit test
  temporarily commented out so the package still compiles against unfixed
  code), both integration tests fail with the stale note still present in
  `pendingAgentContext`; with the fix restored, all 3 new tests pass alongside
  the pre-existing `TestStopAgentLoopCancelsCohortMemberAndJoins` (same
  Stop-cancel-join path, unaffected — still asserts `lastCohortNote` is set).
- Wider battery (56 tests) green: BUG-289 hub-stall watchdog, `TestResumeFlowWithFeedback*`
  (blocked→Continue still drains the note — confirms `loopSealedForReinvoke`
  correctly excludes "blocked"/"paused"), run1618, run333, run9437,
  `TestCohort*`, `TestLiveStop*`.
- Full-suite `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1`:
  18 failures, all pre-existing environment-only (Windows paths, external
  `codex`/`git` CLI availability in this sandbox) — no new failures.
- `go build ./...` clean.

## Known follow-up (not fixed here)

`interactive_resume.go`'s `joinRecoveredCohort` (crash-mid-cohort boot
recovery) builds and queues the same kind of note without this guard. Adding
it there was deliberately deferred: at that point in the resume lifecycle,
`agentOrchestrator.loop[runID].Status` has not yet been seeded from durable
state in this process (a separate, pre-existing gap), so `loopSealedForReinvoke`
would read the loop as "not sealed" even for a genuinely-stopped run and give
a false sense of coverage. Needs its own investigation before fixing.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-307
change_type: bugfix
summary: A cohort-join synthesis note ("call the flow's control tool") is no longer queued to pendingAgentContext when the parent loop has already sealed (stopped/done) — loopSealedForReinvoke now guards all 6 live call sites that build such a note, so it cannot leak into a later unrelated turn's prompt and stall the hub watchdog; lastCohortNote stays unconditional for the existing GateReason diagnostic fallback, and blocked/paused resume flows are unaffected.
# --->8---
