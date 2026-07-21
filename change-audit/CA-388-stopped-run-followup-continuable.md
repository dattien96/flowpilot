# CA-388 — A Stop-ped run is continuable like "done" (follow-up admitted, not 409'd)

## Summary

Fixed BUG-308, found live on run-19845 while verifying CP-51 A8/A11. After Stop
(with reviewers in-flight), a follow-up chat turn got `409 flow_stopped` — both
live and after a server restart — and because a rejected turn is never
persisted, the optimistically-shown prompt bubble vanished from the transcript
on every reopen.

Root cause: `startTurn`'s root admission gate sealed a `"stopped"` loop with
409. BUG-302 had opened `"done"` for follow-ups but deliberately kept
`"stopped"` sealed (its V-1 scope). The persisted `LoopState.Status="stopped"`
restores across restart (review-loop has non-zero Mode/Cap), so the seal — and
the vanish — reproduce every time.

Fix (deliberate reversal of BUG-302's V-1 scope, confirmed as a product decision
with the user): treat `"stopped"` the same as `"done"` in the root admission
gate — the desktop composer is identical for every run and a user naturally
keeps typing after Stop, so Stop ends the FLOW's acting, not the CHAT. Reuses
BUG-305's `turnStartedAfterLoopDone` plain-chat flag and BUG-302's
`offerReviewOutcomeTool`/`loopAlreadySealedAtTurnStart` scoping for `"stopped"`
too, so the follow-up is plain chat (no post-turn gate, no `submit_review_outcome`
offer → no BUG-226 escalate misfire). `"blocked"` still seals; the child branch
(child turn under a stopped/done parent) still 409s.

## Cross-provider parity

Classification: **Case 1, shared admission code, no per-provider branch.** The
admission gate and `offerReviewOutcomeTool` scoping read only
`AgentLoopState.Status`; no `providerKey` branch exists in the touched code
(same as BUG-302's own analysis). `TestChatFollowUpAllowedAfterFlowLoopStoppedGrok`
exercises the Grok adapter to confirm.

## additive-tests-only compliance

One existing test is intentionally updated:
`TestChatRunStillRejectsNewTurnWhenStopped` → `TestChatRunAllowsNewTurnAfterStopped`
(bug302_chat_followup_after_flow_done_test.go). It guarded the exact behavior
being reversed by explicit product decision, so it is updated to assert the new
behavior (not weakened to pass a fix). Its comment records the reversal and cites
BUG-308. All other coverage is new
(`bug308_stopped_run_followup_allowed_test.go`).

## Relationship to BUG-307

BUG-307 (companion, same session) stops a stranded cohort-join synthesis note
from poisoning a follow-up. BUG-308 now ADMITS the follow-up on a stopped run —
so BUG-307's guard is a prerequisite: without it, the newly-admitted follow-up
would inherit the stale "call the flow's control tool" note and hang the hub
watchdog (the run-19500 shape). Together they make the outcome correct and
consistent whether the restored loop status is `"stopped"` (was 409 → now
admitted, BUG-308) or lost/`""` (was admitted-then-hang → now clean, BUG-307).

## Verification

- temp-revert discipline: restoring the `st == "stopped" → 409` seal makes the
  four "allow" tests fail with `flow_stopped`; `TestChildTurnStillRejectedWhenParentStopped`
  passes in both states (child branch untouched). Fix restored → all pass.
- Combined battery (14 tests) green: BUG-302 done-followup, BUG-305 flag,
  BUG-307 cohort-note, BUG-308 stopped-followup, and BUG-289 R19-1 intent-race
  (`TestDurableStartAbortWhenStoppedDuringPersist` — independent durable-persist
  abort path, unaffected).
- Full-suite `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1`:
  18 failures, all pre-existing environment/external-CLI only; the specific
  flaky one under parallel load varies (Gemini external-CLI timeout ↔ Grok
  tempdir cleanup) and each passes in isolation. No new failure tied to the
  admission gate or cohort logic.
- `go build ./...` and `go vet` clean.

## Known follow-up (not fixed here)

`interactive_resume.go:1187` restores `LoopState` only when
`Mode != "" || Cap > 0 || Round > 0`, so `Status` can be dropped for an all-zero
LoopState (the run-19500 shape). BUG-308 makes both outcomes correct, so this is
no longer user-visible, but always carrying `Status` on restore is worth
tightening on its own.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-308
change_type: bugfix
summary: A Stop-ped flow run is now continuable like a "done" one — startTurn's root admission gate no longer 409s a follow-up on a "stopped" loop (only "blocked" still seals, and child turns under a stopped/done parent still 409). The follow-up is admitted and persisted (so it survives restart instead of vanishing) and treated as plain chat (turnStartedAfterLoopDone + offerReviewOutcomeTool scoping extended to "stopped", so no post-turn flow gate and no BUG-226 escalate misfire). Deliberately reverses BUG-302's V-1 scope, which had kept "stopped" sealed.
# --->8---
