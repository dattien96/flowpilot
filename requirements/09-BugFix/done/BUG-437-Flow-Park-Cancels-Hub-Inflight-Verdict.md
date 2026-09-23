# BUG-437: Flow park cancels the hub's own in-flight verdict MCP call

## Metadata

- Document ID: `BUG-437`
- Title: `cp-harness park at cohort verdict cancels hub submit_review_outcome mid-flight → blocked reinvoke → operator-done skips successors`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/done/), [BUG-374](BUG-374-Devin-Permission-ToolCallId-Only-Verdict-Gate-Wedge.md), [BUG-401](BUG-401-HubDone-Verdict-Settles-Whole-Flow.md), [BUG-403](BUG-403-Verdict-Lost-Reprompt-Dropped.md)
- Feature Keys: `agent-flow-engine`, `provider-devin`

## AI Quick View

### Summary

- cp-harness run-10564 (devin): reviewer `submit_review_outcome` auto-rejected ×4 (BUG-374), 2 `cohort_member_verdict_reprompt` retried and re-rejected. The **hub's own** verdict call (bypass mode) was cancelled mid-flight by the park — log: "Canceled due to user interrupt" at 18:02:44 — then on reinvoke returned `{"status":"blocked","round":0,"cap":3,"nextAction":"awaiting_user","openIssues":2}` → `flow_control_rejected_missing_review_verdict` ×2 → parked → operator `done` settled run with `task_splitter`+`audit` SKIPPED.
- Park logic cancels even the flow's own hub verdict call — the one call that could resolve the park.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: when a cohort verdict park fires, an in-flight hub `submit_review_outcome` (which would satisfy the gate) is cancelled; run can only be rescued by operator-done which skips remaining nodes.
- **Expected**: park should not cancel the hub's own verdict submission, or the reinvoke should replay it to completion.
- **Actual**: cancel mid-flight → reinvoke returns `blocked` (round:0) → `flow_control_rejected_missing_review_verdict` → permanent park → operator-done false-completes (BUG-401 family).
- **Impact**: on devin every cp-harness/review-gated run ends via operator-done with `task_splitter`/`audit` skipped — two compounding defects (BUG-374 + this) make the loop unreachable.

## Reproduction

1. Launch cp-harness on devin (`FLOWPILOT_DEVIN_AGENT=1`).
2. Reviewer calls `submit_review_outcome` → auto-`reject_once` (BUG-374) → `cohort_member_verdict_reprompt` ×2.
3. Observe hub's own `submit_review_outcome` cancelled at park ("Canceled due to user interrupt"); reinvoke → `blocked` → `flow_control_rejected_missing_review_verdict` → `flow_control_done` with successors skipped.

## Root cause

- Park/cancel path in `interactive_service.go`/`interactive_handlers.go:752` region treats the hub's in-flight MCP verdict call as interruptible work; combined with BUG-374 (cohort calls auto-rejected) the gate can never be satisfied on devin.

## Evidence

- `~/fp-beds/lt-evidence/cp58/RESULT-RETEST.md` + `retest/` artifacts; `retest-runner.log` lines ~1463-64/1489-90/2250/2942 (rejections) + park at 18:02:44.
- run-10564 (hub `well-substance`), children run-10569/10762/11334.

## Severity

high

## Completion Notes (implemented 2026-09-23, CA-921)

- Root cause: a flow-control decision that triggers a park cancelled the in-flight turn that submitted it, killing the provider turn mid-verdict.
- Fix: dedicated `parkPreserveTurnID` marker armed only at the agent funnel (`turnBridge.SubmitFlowControl`); both park paths (`parkFlowForAwaitingUser` and the locked variant) preserve that turn while still cancelling unrelated in-flight work and arming `parkCancelCause`/`parkCancelSuppress`. The marker deliberately does NOT reuse `lastFlowControlTurnID` — engine-internal `applyFlowControl` calls (audit escalate, machine-verdict credit) stamp it too, and keying preserve on it regressed `TestRun203966AuditEscalateParkKeepsParentNonterminal` (engine escalate must still cancel an unrelated in-flight hub turn). Caught by the full-suite delta and corrected.
- Files: `internal/runner/interactive_service.go`.
- Tests: `TestBug437_ParkPreservesFlowControlSubmittingTurn`, `TestBug437_ParkStillCancelsNonDecisionTurn` (pins that an engine-stamped `lastFlowControlTurnID` alone does NOT preserve), `TestRun203966*` re-green. Baseline-red verified.
