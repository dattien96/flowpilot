# BUG-557 — hub ad-hoc `spawn_agent` of a verdict node loses cohort context; verdict can never be recorded

## Status
RESOLVED — `spawnChildRun` now stamps a one-member ad-hoc cohort
(`flow-adhoc-<nodeID>`, `CohortSize=1`) and binds the label to the node id
when a hub `spawn_agent` call matches a unique active verdict-bearing flow
node (`matchActiveFlowNode` + `flowNodeIsVerdictMember`) and no explicit
`FlowCohortID` was supplied. Explicit cohort (engine dispatch) still wins;
non-verdict nodes keep the plain-child shape. Regression tests:
`bug557_hub_spawn_verdict_cohort_test.go`.

## Original report
Live-found during PrivateVault CP-02 vibe run `run-38799`
(2026-10-02 ~07:07): the hub spawned reviewer `cautious-soil` ad-hoc after a
missing-verdict escalation. The child ran with `flow_cohort_id=""`, so
`submit_review_outcome` was never advertised on its MCP tools/list — and on
Devin ask-mode the call was rejected outright ("only tools annotated as
read-only"). Catch-22: the reviewer existed solely to emit a machine verdict
but had no face to emit it; synthesis `done` stayed rejected until manual
intervention re-drove the leg through the engine path.

## Root cause
`submit_review_outcome` is offered to a child only when
`rs.flowCohortId != ""` (cohort member) or the child resolves to a
`verdict_only` node. The cohort stamp is applied only by engine dispatch
(`tryAdvanceFlowFromNode`/`reinvokeExistingFlowChild` set
`SpawnAgentInput.FlowCohortID` + `CohortSize` + `Label: node.ID`). A hub
ad-hoc `spawn_agent` call matching an active verdict node got none of that —
no cohort, no verdict tool, and the verdict record path
(`recordReviewCohortMemberVerdict`, keyed by label) unreachable.

## Fix
In `spawnChildRun`, when the spawn matches a unique active flow node
(`matchActiveFlowNode`, BUG-556) that is verdict-bearing
(`flowNodeIsVerdictMember`: declared `cohort:` member or
read_only/verdict_only posture) and no `FlowCohortID` was supplied, stamp a
one-member ad-hoc cohort (`flow-adhoc-<nodeID>`, `CohortSize=1`) and bind the
label to the node id. The child is then offered `submit_review_outcome`, the
verdict records under the label synthesis expects, and the cohort join
completes after the single member — reinvoking the hub with the verdict.

## Provider scope
Provider-agnostic — the stamp happens before provider turn setup and uses
the same cohort/verdict machinery every provider adapter already honors.
