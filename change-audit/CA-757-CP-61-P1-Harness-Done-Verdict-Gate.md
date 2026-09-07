# CA-757 — CP-61 P-1: harness hub done requires machine PASS

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-61
change_type: feature
summary: plan_synthesis/synthesis/cp_synthesis cannot dispatch their done-successor on a self-grade; the inbound cohort must record approved via submit_review_outcome
# --->8---

## Change (runner, shared flow runtime)

- `review_done_verdict.go`: added `hubInboundCohortName` (`plan_synthesis`/`cp_synthesis` → `plan`, `synthesis` → `review`, else ungated), `cohortNodeLabels(nodes, cohort)` with `reviewCohortNodeLabels` kept as a one-line wrapper (old behavior + `TestFlowRequiresSynthesisMachineVerdict` untouched), `flowRequiresHubMachineVerdict(rs, hubID)`, `hubDoneVerdictError(parentRunID, hubID)`, `hubDoneCohortHasChangesRequested`.
- `interactive_service.go` `advanceHubDoneThroughEdge`: after real-target resolve, before hub.notify dispatch / Task-325 park / Round reset, a gated hub with a non-approved snapshot routes `changes_requested` → `applyFlowControl(continue)` (writer re-entry) and missing/`blocked` → `applyFlowControl(escalate)`; returns `(res, true)` so the flow never falls through to freeze/audit/splitter and never settles terminal done. PASS falls through unchanged. Follow-up: `applyHubDoneVerdictTransition` — if `applyFlowControl` errors (missing run / one-decision already stamped), fail-closed like the undispatchable-successor branch (`blocked`/`awaiting_user`, stamp one-decision, no dispatch) instead of `return (empty, true)` which `SubmitFlowControl` treated as success.
- `SubmitFlowControl` cohort-member branch + `runTurn` offer-tool branch: record-only `submit_review_outcome` now also offered to plan-cohort children (`plan_reviewer`, `cp_reviewer`) — unblocks `cp-harness` whose acceptance nodes (`cp_synthesis`+`audit`) never satisfied the old synthesis-acceptance check.
- `applyFlowControl` case `done` unchanged (review-loop `synthesis→done` keeps the Task-274 gate); `cp53_review_done_verdict_test.go` unedited.

## Why this shape

- Dual-loop fix: requiring every `cohort:review` node at plan-done would block freeze until the code reviewer (not yet run) PASSes — expected labels come from the hub's inbound cohort, so `reviewer=approved` alone still blocks `plan_synthesis` and `plan_reviewer` is never required at `synthesis`.
- Verdict check runs before the churn park and `resetPlanPhaseRound`: reviewers already ran, park is human read of an already-PASSed plan; a self-grade `approved` with no PASS never parks or resets Round.
- Unknown hub ids stay ungated (fail-open, same as today); flows without cohort nodes are unaffected (all pre-existing park/freeze/audit tests green with no fixture edits).

## Tests

- New `cp61_hub_done_verdict_test.go` (`TestCP61HubDone`, 13 subtests × claude/codex/grok): plan missing/changes/PASS/wrong-cohort, synthesis missing/PASS (audit RUNNING-or-DONE; audit's own `blocked_validation_failed` escalate is downstream of the gate), cp missing/PASS (splitter RUNNING + 1 child), `cp_reviewer` record-only, normal-chat unaffected, churned PASS parks / churned missing escalates (never `plan_approval`), applyErr escalate + applyErr continue → `blocked`/`awaiting_user` and no freeze/writer.
- Verification: `TestCP61HubDone` PASS (applyErr + missing/changes/wrong-cohort/cp_reviewer 19s); `TestCP53ReviewDoneVerdict*` + `TestFlowRequiresSynthesisMachineVerdict` PASS. Freeze-spawn old tests (`TestPlanPhaseRoundResetOnFreezeDispatch`, `TestPlanApprovalPark_CleanPlanPassesThrough`) fail only on Windows `testing.go:1464` TempDir `unlinkat` (file in use) after assertions — not an assertion regression; same flake seen pre-follow-up on `plan_synthesis_approved`.

## Prior CA intact (R2)

- Shared flow runtime, provider-agnostic by construction (no `providerKey` branch on the gated paths); claude/codex/grok matrix locks against drift.
- Will-not-undo: CA-755 Round reset (PASS path still resets), CA-749/752 park (churned PASS still parks), CA-353 one-decision stamp (gate routes through `applyFlowControl`, which stamps), existing `cp53_review_done_verdict_test.go`.
