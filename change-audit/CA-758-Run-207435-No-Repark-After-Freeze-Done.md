# CA-758 — run-207435: no plan_approval re-park after freeze DONE

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-325
change_type: bugfix
summary: a stale plan_synthesis done after freeze DONE no longer re-parks plan_approval; first churned park before freeze is unchanged
# --->8---

## Live repro (run-207435, TUI Task Harness on gate-sandbox)

- Churned plan approved → `preflight_contract_freeze` DONE → code phase running (`implement`).
- `hub_stalled` Retry reinvoked the hub while `activeHubNodeID` still pointed at `plan_synthesis`.
- Stale done passed the still-`approved` verdict snapshot; `planLoopChurned` latch (permanent, `writer round >= 1`) re-parked `plan_approval` with the stale reason, cancelling the live implement turn.
- Runner `LoopState` read `blocked/plan_approval` with freeze DONE — TUI rendered faithfully; runner parked wrongly, not a TUI display bug.

## Change (runner, 1 guard + helper)

- `plan_approval_park.go`: new `flowFreezeStepDone` — true iff the freeze step reads DONE via `LoadRunSteps`. Load error emits `plan_approval_freeze_check_failed` and returns false so the first park keeps CA-749 behavior (distinguishable from a real first park in flow diagnostics).
- `interactive_service.go` `advanceHubDoneThroughEdge` churned branch: when freeze is already DONE, skip `parkPlanForApproval`; stamp the one-decision guard (BUG-353/226) and return `done`/`advancing` with the live loop state — no loop mutation, no step change, no Round reset, no re-dispatch.
- Plan subtests use `run207435PlanServiceWithCohort` (new-file fixture mirroring `task325PlanService` + `plan_reviewer` Cohort `plan`) so the CP-61 verdict gate evaluates on the freeze-DONE / freeze-pending / clean paths; the shared legacy helper is untouched.
- CP-61 P-1 gate untouched and still runs first: a FAIL snapshot still routes continue/escalate before this branch.

## Deliberately not changed

- `activeHubNodeID` is not cleared/retargeted after freeze: clearing falls back to first `hub.inline` (`plan_synthesis` again); pointing at `synthesis` early would misroute a stale plan done to `audit`. The guard is idempotent, so the stale pointer is harmless.
- `hub_stalled` resume target unchanged: the guard breaks the corruption chain (stale hub turn becomes a harmless advancing no-op); rerouting resume to the code child is riskier and needs live verification.
- `planLoopChurned` latch unchanged: still correct for the first park.

## Tests

- New `run207435_plan_approval_no_repark_after_freeze_test.go` (`TestRun207435NoReparkAfterFreezeDone`, 4 subtests × claude/codex/grok): freeze-DONE + churned done advances (no `plan_approval`, loop running, freeze stays DONE, decision stamped); freeze-pending + churned still parks (CA-749); clean plan still dispatches freeze; code hub after freeze dispatches audit with no plan park.
- Verification: new suite PASS; `TestPlanApprovalPark_ChurnedPlanParks|TestPlanApprovalPark_CodeHubDoneDoesNotPark`, `TestCP61HubDone/churned*|plan_synthesis_missing|plan_synthesis_changes`, `TestCP53ReviewDoneVerdictHub*|PassAllowsDone`, `TestFlowRequiresSynthesisMachineVerdict` PASS, old files untouched.
- Windows `testing.go:1464` TempDir `unlinkat` flake hit `clean_plan_still_dispatches_freeze/claude` once after assertions; passed on retry.

## Prior CA intact (R2)

- Case 1 agnostic: `flowFreezeStepDone` / park branch / `advanceHubDoneThroughEdge` take no `providerKey` and never branch on one; matrix locks against drift.
- Will-not-undo: CA-749 first park before freeze, CA-752 empty=approve, CA-755 Round reset on real dispatch, CA-757 verdict gate order, CA-353 one-decision stamp, CA-741 park-cancel.
