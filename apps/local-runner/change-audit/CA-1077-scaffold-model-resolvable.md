# CA-1077: agent.scaffold model tier is honored (Settings row + pack pin)

Date: 2026-10-01
Refs: CP-67 P-3 (agent.scaffold accepted `model:` at pack-load, "consumed
the same way" as delegate), Task-320 (per-node model tier), BUG-352
(spawnable-only DB carry), CA-616 (planner DB-skip), BUG-228/CA-358
(per-node model resolution and scoping).

## Problem

`resolveFlowNodeModel` allowed only `agent.delegate` and `agent.code`:
every other canonical behavior early-returned `""` (inherit). But
`agent.scaffold` children dispatch through `behaviorAgentDelegate`
(`behavior_registry_builtin.go`) — real provider turns — and pack
validation has deliberately accepted `model:` on scaffold nodes since
CP-67 B-5 (`pack.go`, "their High-Reasoning `model:` tier is consumed the
same way").

Result: `vibe-sprint`'s `tdd` pin `claude-sonnet-4-5` was **dead config**
— the scaffold-architect child always inherited the run model — and the
Settings step-definition row for `tdd` could never take effect. The UI
additionally hid the Model field on scaffold steps entirely
(`stepDefinitionRequiresModel` only knew delegate/code).

## Solution

Three coordinated spots, all extending the existing contract (no new
mechanism):

1. `resolveFlowNodeModel` (`flow_executor.go`): `agent.scaffold` joins
   the spawnable allowlist — Settings `step_definitions.model` wins,
   else pack `node.Model`, else inherit.
2. `recordFromWorkflowRow` (`supabase_workflow_flow_store.go`): the
   BUG-352 carry now stamps `defn.Model` onto `agent.code` and
   `agent.scaffold` nodes too (all three are accepted by
   `ValidateFlowDefinition`'s spawnable-only rule), keeping cloned/
   mirror rows faithful.
3. `stepModelVisibility.ts` (desktop): `agent.scaffold` added to
   `BEHAVIOR_IDS_REQUIRING_AGENT` so the Model field renders on scaffold
   steps in Settings > Steps. Scaffold stays out of
   `FLOW_BEHAVIOR_OPTIONS` — it remains a builtin-authored topology
   marker; this only exposes the field on existing steps.

Precedence for `tdd` after this change: Settings step row >
`vibe-sprint.yaml` pin (`claude-sonnet-4-5`) > inherit run model.
**Behavior change**: vibe sprint scaffolds now run on
`claude-sonnet-4-5` by default instead of inheriting — that was always
the pack's declared intent (CP-67).

## Files

- `internal/runner/flow_executor.go` — allowlist + comment.
- `internal/runner/supabase_workflow_flow_store.go` — carry scope.
- `internal/runner/ca1077_scaffold_step_model_test.go` — 3 cases:
  step row wins, pack pin applies, empty inherits (all were red:
  resolver returned `""` for every scaffold node).
- `apps/desktop-flowpilot/src/components/settings/stepModelVisibility.ts`
  + `.agent-code.test.ts` — visibility + 2 tests.

## Verification

- Red first: `TestCA1077_*` failed `""` vs expected before the fix.
- Green after, plus the full `resolveFlowNodeModel` /
  `recordFromWorkflowRow` / `spawnFrozenWriterChild` suite —
  `DropsNonDelegateStaleModel` still drops models on non-spawnable rows.
- `go vet ./internal/runner/` clean; desktop `tsc --noEmit` clean;
  visibility tsx tests 5/5.
