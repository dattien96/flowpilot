# CA-1221 — adopt-sprint context→validate violates spawnable-target rule + stranded-pin fallback for vibe chains

- **Area**: runner / flow topology (vibe-adopt-sprint) + flow-node model
  resolution on vibe sprint chains
- **Evidence**: live run-297392 (vibe-adopt, CP-05 on cp05 worktree, devin
  hub). Preflight planner + freeze completed under CA-1219's fixed
  resolution (child run-297409 = `devin`), then the loop escalated:

  `Context production blocked: forward target validate is not a spawnable
  delegate/writer (behavior command.validate)` → `blockReason: escalate`.

- **Root cause 1 — topology**: `flow_validate_audit_dispatch.go` requires
  a `context.produce` node's single forward done target to be a spawnable
  delegate/writer (`agent.delegate`/`agent.code`/`agent.scaffold`/reproduce)
  — the context package is packed into that consumer's spawn prompt, so a
  `command.validate` target has no recipient. CA-1216's `context→validate`
  entry edge was therefore engine-illegal and unreachable in production
  (pack validation does not check behavior-vs-spawnability).
- **Fix 1**: adopt topology rerouted `context→tdd→validate`: the
  characterization scaffold (spawnable) receives the context package and
  declares the existing-behavior contract, then the suite runs so the
  review cohort still sees the baseline signal on the pre-existing diff.
  `tdd→coder` dropped — coder is reachable only via the remediation
  back-edges (`validate→coder`, `synthesis→coder`) plus an inert
  `synthesis_negotiation→coder` `remediate` forward edge that keeps it
  non-entry for topology validation (same anchor pattern as tdd's).

- **Root cause 2 — stale mirror pins still strand delegates**: the
  `vibe_adopt_sprint_*` `step_definitions` rows minted by run-295434's
  mount still carry `model=gpt-5.4` (`upsertNodeStepDefinitions` never
  writes model and `merge-duplicates` keeps whatever exists), and the
  pack yaml pins `tdd` to `claude-sonnet-4-5`. Under CA-1219 every such
  resolution → cross-provider spawn → admission gate → escalate: the
  adopt chain could never clear a single delegate on a devin-only host.
- **Fix 2**: `resolveFlowNodeModel` now treats a resolved pin (step-row
  hit *or* pack yaml `node.Model`) as absent when ALL of:
  - the parent run is on a vibe sprint chain (`vibe-sprint` or
    `vibe-adopt(-sprint)` — flows whose delegates are designed to inherit
    the hub's session provider),
  - `providerKeyFromModel(pin)` differs from the parent's provider, and
  - that provider has no connected local account (`spawnProviderConnected`,
    the bool half of the CA-1218/1219 admission gate).
  The child then inherits the session provider instead of escalating.
  Pins to *connected* providers are still honored — legitimate
  cross-provider routes (tournament candidates, reviewer cohorts) keep
  Task-320 semantics; non-vibe flows are untouched.
- **Why fallback, not gate**: on a vibe chain the pin was never operator
  intent — it is a resolution artifact of a dead run (or a pack default
  tuned for hosts with claude connected). Escalating cannot repair a
  missing account; session inherit completes the sprint on the provider
  the operator actually connected. The CA-1219 spawn gate still covers
  every non-vibe path unchanged.

- **Tests**: `ca1221_vibe_chain_stranded_pin_test.go` — 6 cases: stranded
  step-row pin → inherit, stranded yaml pin → inherit, connected
  cross-provider pin → honored, same-provider pin → honored, non-vibe
  flow → pin kept, `vibe-sprint` chain → same fallback as adopt.
  `TestVibeAdoptSprint_TopologyEntersAtValidate` re-asserted for the
  `context→tdd→validate` topology (renamed intent kept: suite still runs
  before the review cohort). Package `runner` + `agentpack` green on all
  touched paths.

- **Live follow-up**: run-297392 is unrecoverable — its mounted
  flowDef persists the `context→validate` edges, so it must be
  cancelled and relaunched on a binary carrying this fix.
