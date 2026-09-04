# CA-741 — audit park-cancel keeps parent non-terminal; Stop wins; Retry heals (run-203966)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: an engine park (audit escalate / hub_stall / delegate_failed) cancelling the in-flight hub turn no longer stamps the parent Cancelled/Failed — finishTurn's park-cancel branch keeps the run Running with the parked loop owning liveness, Stop (loop sealed "stopped") still wins, Retry heals a park-poisoned Cancelled parent, and park-time parkCancelSuppress stops the adapter's mid-cancel TurnFailed from abandoning canonical heads or signalChild
# --->8---

## Problem

- Live run-203966 (`/flow task-harness`, CP-58 S2, GCD prompt): the flow reached `audit` with the `synthesis` hub turn still in flight. The audit tier-3 escalate (missing change-audit note) called `parkFlowForAwaitingUser`, which cancelled the hub turn; `finishTurn`'s `context.Canceled` branch mapped it to the generic **"interrupted by user"** → parent stamped **Cancelled**.
- A terminal parent makes `flowRunTerminalLocked` true, so every later advance after Retry was silently skipped (`flow_inline_dispatch_skipped_terminal`: "run terminal/stopped") until the watchdog parked `hub_stalled`. The turn could not finish cleanly and the flow died.
- CA-740's note ("Turn failed: interrupted by user is the operator interrupting after the card") misattributed the same failure — the interruption is the ENGINE's own park, not the operator.

## Root cause

`parkFlowForAwaitingUser(Locked)` unconditionally cancels the in-flight hub turn while parking; `finishTurn` had no way to distinguish "engine park cancel" from "user interrupt".

## Changes (all in `apps/local-runner/internal/runner/interactive_service.go`)

- `interactiveRun` gains one-shot `parkCancelCause` + `parkCancelSuppress` (same shape as BUG-288's `stalledRetryCause`).
- `parkFlowForAwaitingUser` / `parkFlowForAwaitingUserLocked`: arm both flags before `turnCancel()` (CA-403's `preserveParentTurnID` park still skips both — cap on the submitting turn stays untouched).
- `emitLocked` (`EventTurnFailed`): `parkCancelSuppress` keeps the run non-terminal and skips the root-flow failure side effects (canonical-head abandon, `signalChild`).
- `finishTurn`:
  - entry: clear both flags for any non-`context.Canceled` error (including `nil`) — a stale cause must not misclassify the next real interrupt, a stale suppress must not swallow a real failure (clear happens BEFORE the switch so no branch emits under suppression);
  - `stalledSkipCause` / `stalledRetryCause` branches: also clear the park flags;
  - new park branch: heal to **Running**, emit recoverable TurnFailed "interrupted by flow park (awaiting user decision)", patch the child summary cache, settle the dispatch record as cancelled — turn is durably over, run stays live;
  - **Stop wins**: if the loop was already sealed `"stopped"` or the parent already stamped Cancelled (stopAgentLoop), fall through to the generic "interrupted by user" branch — BUG-248's stop contract intact.
- `resumeFlowWithFeedback` self-heal: a blocked loop resuming a park-poisoned **Cancelled** parent restores Running so `flowRunTerminalLocked` no longer skips every advance. Only Cancelled heals — a Failed root is a real failure; a stopped loop never reaches the heal (`wasBlocked=false`).

## Tests added (new file only)

- `run203966_audit_park_cancel_nonterminal_test.go` (matrix Claude/Codex/Grok; new file, no pre-existing test touched):
  - `TestRun203966AuditEscalateParkKeepsParentNonterminal` — repro: audit missing-CA escalate with hub turn in flight → parent stays Running, flags armed, `flowRunTerminalLocked=false`.
  - `TestRun203966FinishTurnParkCancelDoesNotStampCancelled` — `context.Canceled` + cause → Running, flag consumed.
  - `TestRun203966StopWinsOverParkCancelCause` — stopAgentLoop → Cancelled/stopped survives the park-cancel finishTurn.
  - `TestRun203966ParkCancelCauseClearedOnCleanFinish` — `nil` err clears both flags.
  - `TestRun203966ResumeHealsCancelledBlockedNotStopped` — blocked+Cancelled heals; stopped does not; Failed root never heals.
  - `TestRun203966CapPreserveDoesNotSetParkCancelCause` — CA-403 preserve parks arm no flags; default park does.

## Verification

- New tests PASS (3-provider matrix); old related suites PASS untouched: `TestRun202550|TestRun45103|TestParkFlow|TestRun1675|TestStopAgentLoop|TestFinishTurnStallRetry|TestApplyFlowControl|TestBug288|TestBug248|TestRun333|TestRun63960|TestRun201295|TestBug353|TestRun200816|TestRun199617|TestCA623|TestBug327|TestFlowSettle|TestV9|TestRun135037|TestBug302|TestBug308|TestFlowHubNotify|TestRun1618|TestRAGHarness|TestReviewLoop|TestContextCoding|TestFlowFrozenWriter|TestFlowPendingCanonical|TestGateTier|TestFlowContractFreeze|TestFlowRootHub|TestNormalChat|TestAdHoc|TestRun5296|TestRun2047|TestBug318|TestBug305|TestRun198699|TestFlowGate|TestPhaseA|TestBug298` (R1).
- Baseline-failing pre-existing (stash-confirmed identical failures with and without this change — environmental git-index behavior, unrelated): `TestRun144900_PreexistingSkillDoesNotCauseScopeDrift`, `TestRun144900_MutatedLeftoverStillBlocks`, `TestRun147126_AuditHonorsFrozenContract`, `TestRun147126_SupersededFrozenStillBlocks`, `TestRun147126_ContinueAfterAuditParkDoesNotFlapSynthesis`.
- Provider-agnostic (R2): `parkFlowForAwaitingUser(Locked)`, `finishTurn`, `resumeFlowWithFeedback`, `emitLocked` take no `providerKey` and never branch on one (grep-verified in-session); matrix still runs all three keys to lock against future drift.
- Will not undo: CA-403 (preserveParent park), CA-740 (missing-CA Retry re-enters writer), BUG-248 (Stop → Cancelled + loop sealed), BUG-288 stalledRetry/stalledSkip contracts.
- Residual (out of scope, documented): a park-cancelled CHILD run still gets the generic Cancelled stamp (parent-level only); audit **pass** + hub turn still in flight may defer `EventTurnCompleted` behind the gate (`deferGateCompleted`) — separate issue, not this bug.
- `go vet ./internal/runner/` clean; runner builds; gofmt churn in `interactive_service.go` is pre-existing (CA-424/425, stash-confirmed baseline flag).
