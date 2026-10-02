# CA-1104 — BUG-557: hub ad-hoc spawn of a verdict node gets cohort context

## What
`spawnChildRun` now resolves a hub ad-hoc `spawn_agent` call to a unique
active flow node (`matchActiveFlowNode`, shared with BUG-556). When the
matched node is verdict-bearing — a declared `cohort:` member or a
read_only/verdict_only posture — and the input carries no `FlowCohortID`,
the spawn is stamped with a one-member ad-hoc cohort
(`flow-adhoc-<nodeID>`, `CohortSize=1`) and its label is bound to the node
id so the recorded verdict lands on the label synthesis expects.

## Why
Live run-38799 (PrivateVault CP-02): after a missing-verdict escalation the
hub spawned reviewer `cautious-soil` ad-hoc. With `flow_cohort_id=""` the
`submit_review_outcome` tool was never advertised on the child's tools/list —
Devin ask-mode rejected the call outright ("only tools annotated as
read-only") — and the machine verdict could never be recorded. Synthesis
`done` stayed rejected until the leg was manually re-driven through the
engine dispatch path (`tryAdvanceFlowFromNode`), which is the only place
that used to stamp cohort context.

## Guarantees kept
- Explicit `FlowCohortID`/`CohortSize` (engine dispatch, re-drive) is never
  overwritten.
- Non-verdict nodes (standard posture, no declared cohort) keep the exact
  plain-child shape — writers/delegates do not get rerouted through the
  cohort-review-step error.
- Ambiguous node matches stay unbound (no stamp).
- Cohort join machinery is reused unchanged: `preRegisterCohort(1)` +
  `registerCohortMember` drain → `maybeAutoReinvokeHub` closes the loop.

## Tests
- `TestBug557_HubSpawnVerdictNodeStampsCohort` — red before the fix
  (cohort empty, label fell back to agent name), green after.
- `TestBug557_HubSpawnNonVerdictNodeKeepsNoCohort` — writer node unchanged.
- `TestBug557_ExplicitCohortStillWins` — engine-supplied cohort untouched.
