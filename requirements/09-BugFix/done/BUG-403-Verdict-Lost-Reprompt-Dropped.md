# BUG-403: Verdict lost / reprompt dropped — hub verdicts with zero flow effect, three repro paths

## Metadata

- Document ID: `BUG-403`
- Title: `submit_review_outcome claim has no flow effect; verdict reprompt dropped on turnInFlight; debate hub turn suppressed while blocked then coder reprompt lost`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md), [CP-58-Test-Steps](../../07-Coding-Plan/done/CP-58-Test-Steps.md), [CP-62-Test-Steps](../../07-Coding-Plan/done/CP-62-Test-Steps.md)
- Feature Keys: `flow-engine`, `owner-debate`, `review-loop`

## AI Quick View

### Summary

- Repro 1 (CP49-3, run-2290/turn-6967): debate-synthesis reprompt transcript asserts `Verdict submitted: approved — flow step complete`, but `dispatch.ndjson` records **zero `flow_control` effect**; `tdd` never transitioned DONE, sprint never advanced → `hub_stalled`. Settle logged `dispatch record revision is stale` before a later finalize.
- Repro 2 (CP58-1, deterministic): missing-verdict reprompt calls `scheduleChildTurn` while the child's previous turn is still `turnInFlight` → `startTurn` rejects `"a turn is already in flight for this session"`; the reprompt path never sets `reinvokeInFlight`, so the failure escapes the drain/re-arm recovery → reviewer stuck, cohort never joins, hub times out. Three E2E tests fail deterministically (8s `waitLoop` timeouts).
- Repro 3 (CP62-1, run-2860): `debate_synthesis` hub turn never dispatched while loop `blocked(escalate)` — ~7 min dead window, step RUNNING with no turn — until manual `agent-loop/continue`; after the debate resolved `done`, the queued coder reprompt was dropped and the coder child stayed `waiting_user_approval` → silent stall.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** the runner accepts/claims a review verdict (or schedules a verdict reprompt) but no flow-control effect is recorded — the step stays non-terminal, the loop wedges `blocked`/`hub_stalled`, and only manual operator action (`agent-loop/continue`, `flow-control`) unsticks it — and even then follow-on dispatches can be silently dropped.
- **Expected:** a `submit_review_outcome` call the runner acknowledges must produce exactly one durable `flow_control` effect; a verdict reprompt that cannot start must enter the drain/re-arm path (`reinvokeInFlight`) instead of vanishing; a `blocked(escalate)` park must not suppress the debate synthesis hub dispatch nor strand the post-debate reprompt.
- **Actual:** three independent loss points — (1) claimed verdict never lands in `dispatch.ndjson`; (2) reprompt `startTurn` failure swallowed with `will_drain:false, transient_busy:false`; (3) `parkFlowForAwaitingUser` cancels in-flight turns / marks children `waiting_user_approval` while clearing `pendingGateReprompt*`, so post-debate remediation evaporates.
- **Impact:** review loops and vibe owner-debates wedge silently (minutes to forever); deterministic E2E suite failures mask any provider whose settle ordering races; recovery requires undocumented manual operator surgery with no guarantee the queued remediation survives.

## Reproduction

- **CP49-3:** vibe run-2290 — let the debate-synthesis reprompt turn claim `submit_review_outcome`; inspect `.flowpilot/chats/lt-cp49/dispatch.ndjson` for a `flow_control` entry (none); observe `tdd` never DONE → `hub_stalled`.
- **CP58-1:** `go test ./internal/runner -run 'TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes|TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes|TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes'` — all three hit `waitLoop` 8s timeouts at HEAD `435e336b` (in-process fake provider hits the settle-boundary race deterministically; live providers race timing-dependently).
- **CP62-1:** vibe run on opencode; coder writes outside frozen contract → scope-drift `escalate` + owner-debate overlay simultaneously; observe `debate_synthesis` RUNNING with zero hub turns while `status=blocked`; `agent-loop/continue` releases it; then observe coder child stuck `waiting_user_approval` with the reprompt never delivered.

## Root cause

- Repro 2 (confirmed, `apps/local-runner/internal/runner/interactive_service.go` ~L5217–5277): missing-verdict reprompt calls `scheduleChildTurn` synchronously; `startTurn` rejects on `turnInFlight`; the reprompt path does not set `reinvokeInFlight`, so the failure never enters the drain/re-arm recovery used by `hub_reinvoke_start_failed`. Diag NDJSON: repeated `hub_reinvoke_start_failed` / `scheduled hub reinvoke startTurn failed` / `a turn is already in flight for this session` with `will_drain:false`.
- Repro 3 (suspected mechanism): `parkFlowForAwaitingUser` (`interactive_service.go:2560+`) cancels in-flight turns and marks running children `waiting_user_approval`; `parkFlowForAwaitingUserLocked` clears `pendingGateReprompt*` fields (L2643–2649). The `debate_synthesis` hub dispatch is suppressed while `loopStatus=blocked`; after `continue`, synthesis ran but the reprompt intent was already dropped and the child status never restored. `maybeSettleVibeOwnerDebate` (`vibe_debate.go`) handles owner FAILED only — not owner-DONE + escalate-park interleaving.
- Repro 1 (suspected): settle raced the dispatch record (`[settle] attempt 1 ... dispatch record revision is stale`), and the retried finalize path persisted the transcript claim without the `flow_control` write — no idempotent verdict-effect ledger on the reprompt path.

## Evidence

- CP49-3: `~/fp-beds/lt-evidence/cp49/L49-2-run2290-turns.ndjson` (turn-6967 transcript claim), `.flowpilot/chats/lt-cp49/dispatch.ndjson` (no flow_control entries), `runner.log` settle line; `RESULT.md` BUG-LIVE-3.
- CP58-1: `~/fp-beds/lt-evidence/cp58/autotest.log` (FAIL lines 417, 589, 756; package FAIL line 1459), `diag-child-run-26-reprompt-drop.ndjson`, `diag-child-run-40-reprompt-drop.ndjson`, `BUG-LIVE-1-verdict-reprompt-dropped-turninflight.md`.
- CP62-1: `~/fp-beds/lt-evidence/cp62/l62-2-run2860-admin-events-final.json` (seq 128 @21:48:12Z → seq 129 @21:55:14Z silence; owner verdicts evt-3912/3928; turn-3953 only after continue), `l62-2-run2860-steps-final.json` (debate_synthesis 21:48:09→21:56:11), `l62-2-wedge-timeline.txt`, `monitor.log`, `l62-2-run2860-agentgraph-final.json` (coder run-3670 `waiting_user_approval`), `BUG-LIVE-1.md`.

## Severity

`high` — silent stalls on the main review/debate paths plus deterministic suite failures; recoverable only via manual operator intervention, and remediation itself can be lost.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause: the missing-verdict reprompt path scheduled the child turn without setting `reinvokeInFlight`, so a `turn_in_progress` rejection left no armed recovery marker — the reprompt was dropped and the hub never re-fired.
- Fix: `reinvokeInFlight` is set BEFORE `scheduleChildTurn` so its error handler re-arms + drains on rejection.
- Files: `internal/runner/interactive_service.go`.
- Tests: `TestBug403_VerdictRepromptArmsReinvokeRecovery`. Baseline-red verified.
