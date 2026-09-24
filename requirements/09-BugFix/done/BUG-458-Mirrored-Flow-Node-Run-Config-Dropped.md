# BUG-458: Mirrored flow definitions drop FlowNode.Run/Model/Config/Posture/ContextProfile — tournament rollout never spawns

- status: done
- found: live run-16693 (post-rebase consolidated live test, CP-65 tournament on devin/swe-2-high hub)
- fixed_by: CA-955
- tests: internal/runner/bug458_mirror_node_fields_test.go (3 additive tests)

## Symptom (live)

run-16693 (tournament-harness, canonical `workflowId`) completed
`problem_scout`, then the flow never fanned out. Diag log:

```
flow_advance_targets_resolved   completed=problem_scout targets=[parallel_rollout]
flow_advance_target_not_spawnable target=parallel_rollout behavior=""
```

The hub was reinvoked, stalled, and the run ended with every downstream node
(candidates, arbiter, merge) never dispatched.

## Root cause

`step_definitions` has no column for `run`, `posture`, `context_profile`, or
the free-form `config:` map — `upsertNodeStepDefinitions` never writes them
and `recordFromWorkflowRow`/`workflowSelect` never read them. `model` has a
column but sync deliberately never writes pack values (admin-reserved), so
pack-declared `model:` was lost too. Every mirrored definition reconstructed
nodes with `Run: ""`/`Config: nil`/`Posture: ""`/`ContextProfile: ""` and
no pack model.

For most nodes this was invisible because dispatch keys off `behavior`. But
tournament-harness `parallel_rollout` is a **behaviorless inline marker** —
`tournamentRolloutPassthrough` (and the `resumeTournamentChoice` retry
finder) identify it ONLY by `run: inline`. `""` fails `EqualFold("inline")`
→ `not_spawnable` → hub verdict quagmire. The arbiter's
`config: {auto_pick, max_attempts: 2}` also vanished, silently degrading
max_attempts to the 1 default (retry round lost), and candidate `model:`
dropped to "" → both candidates inherited the hub provider (run-17745:
devin free-tier rate limit killed the whole cohort).

Live proof the mirror path was active: `~/.flowpilot` keychain holds the
service-role key → `FlowDefinitionStoreFor` non-nil → startup mirror sync +
`GetByPackFlow` returned the mirrored row (verified directly against
Supabase: `workflows.pack_flow_id=tournament-harness`, `is_builtin=true`,
`parallel_rollout.behavior_id=null`, no run column).

## Fix

`recordFromWorkflowRow` now restores pack-declared fields after
reconstruction:

- **Builtin mirror rows** (`is_builtin` + `pack_id` + `pack_flow_id`):
  `Run`/`Posture`/`ContextProfile`/`Config`/`Model` are filled from the
  embedded pack definition by node id. The first four have no column at
  all; `model` is restored only when the row carries none, so admin
  overrides keep precedence (`resolveConfiguredModelForAgent` consults the
  DB row before `node.Model` anyway).
- **All rows** (user/cloned flows, builtin nodes absent from the pack):
  empty `Run` is derived — `agent.*` canonical behavior → `delegate`,
  everything else (incl. behaviorless markers) → `inline`. `""` is never a
  valid run value, so derivation is strictly better than the old "".

## Tests

- `TestRecordFromWorkflowRowBuiltinMirrorRestoresRunInline` — RED before
  fix: mirrored tournament row rebuilds `parallel_rollout` with
  `Run=="inline"` and `Config["candidates"]` present.
- `TestRecordFromWorkflowRowBuiltinMirrorRestoresArbiterConfig` — RED:
  arbiter `Run=="inline"` + `Config["max_attempts"]==2` restored.
- `TestRecordFromWorkflowRowBuiltinMirrorRestoresNodeModel` — candidate
  `model:` restored (claude-sonnet / gpt-5.4-mini) so the cohort spawns on
  its declared providers instead of inheriting the hub's.
- `TestRecordFromWorkflowRowDerivesRunForUserRows` — RED: user-row
  behaviorless marker → inline, `agent.delegate` → delegate,
  `command.validate` → inline.

## Follow-ups (documented, not fixed here)

- `posture`/`context_profile`/`config` remain un-writable for
  **user-authored** flows (no schema column). If user flows ever need them,
  add a `step_definitions` migration + round-trip in
  `upsertNodeStepDefinitions`/`workflowSelect`/`dbStepDefinitionRow`.
- CP-65 live retry leg needs the arbiter `max_attempts:2` that this fix
  restores — re-run the tournament on a build containing CA-955.

## Live evidence

- run-16693 diag `flow_advance_target_not_spawnable target=parallel_rollout
  behavior=""` at 10:59:41 local; probe of the embedded pack definition
  returned `passthrough ok=true` — isolating the loss to the mirror path.
