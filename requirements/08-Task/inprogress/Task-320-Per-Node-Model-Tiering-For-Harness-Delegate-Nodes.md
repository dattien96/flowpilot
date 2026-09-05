# Task-320: Per-Node Model Tiering For Harness Delegate Nodes (Pack YAML `model:`)

## Metadata

- Document ID: `Task-320`
- Title: `Per-Node Model Tiering For Harness Delegate Nodes (Pack YAML model:)`
- Phase: `task`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Parent Documents: [CP-58: Bug / Task / CP Harness](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `none`
- Related Documents: [Task-305](../../08-Task/done/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md), [CA-616](../../../change-audit/CA-616-run135037-hubless-planner-fail-waiting.md), [CA-358](../../../change-audit/CA-358-flow-scoped-step-model-lookup.md), [CA-228](../../../change-audit/CA-228-single-step-model-resolution-step-only.md)
- Replaces: `none`
- Tags: `agent-flow-engine, harness, model-tiering, pack-schema`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Closes the CP-58 P-7 / Task-305 follow-up "per-node model tiering not yet wired": pack YAML nodes gain an optional `model:` field so tiering (Scout cheap vs Architect high-reasoning) is version-controlled with the pack instead of per-installation DB edits only.
- Single source of truth for the model→provider prefix table moves to `agentpack.ModelProviderKey`; runner `providerKeyFromModel` delegates to it (zero call-site changes); pack validator fails closed on unknown prefixes (typo can no longer silently inherit).
- Resolution precedence for `agent.delegate`: DB step row (admin override, non-planner only) > YAML `node.Model` (pack default; DB-carried for cloned flows via `recordFromWorkflowRow`) > agent frontmatter > inherit. Planner nodes (`contract-planner` / `preflight_contract_plan`): YAML > inherit — DB rows stay skipped so the CA-616 legacy-`gpt-5.4` seeded row can never bite again.
- `model:` on any non-delegate behavior is a validator error (fail-closed: today the engine would silently ignore it).

### Current Ask

- Implement P-1 (agentpack schema + validation + prefix table), P-2 (runner delegation + resolution + DB-carry), P-3 (docs, no builtin pins), with additive tests; full suite green.

### Key Decisions

- `T-1` Admin DB row still beats pack YAML for non-planner delegate nodes (per-installation override preserved; mirror re-sync never writes `model` so it cannot clobber admin values — BUG-249 principle).
- `T-2` No concrete models pinned in builtin YAMLs in this task: pinning e.g. `claude-*` would couple builtins to one provider and break installs without it (CP-58 provider-agnostic constraint). Tiering is configured per-installation (step rows, works for planner too via the DB-carry path) or in downstream pack forks.
- `T-3` `agent.code` / `hub.inline` nodes stay inherit-only; `model:` on them is rejected rather than silently ignored.

### Constraints

- Additive only: empty `model:` (all current YAMLs) must resolve exactly as today — planner inherits (CA-616), delegate consults rows, code/inline inherit.
- `providerKeyFromModel` mapping must stay byte-identical (prefix order + `grok-build` exact match + `opencode/` prefixes); parity test pins it.
- Do not edit pre-existing tests except none needed (all coverage is new test files + new test funcs).

### Open Questions

- None blocking. Follow-up (not this task): surface YAML-effective model in Desktop step list; per-provider default-model refresh.

### Source Refs

- CP-58 §P-7, Task-305 T-5/follow-ups; `pack.go:117 FlowNode`, `pack.go:731 flowNodeFromMap`, `pack.go:900 ValidateFlowDefinition`; `flow_executor.go:1567 delegateSpawnModel`, `:1587 resolveFlowNodeModel`, `:1734 resolveFlowNodeProviderModel`; `provider_registry.go:215 providerKeyFromModel`; `supabase_workflow_flow_store.go:138 recordFromWorkflowRow`, `:632 upsertNodeStepDefinitions`; `interactive_service.go:5856` spawn priority.

## 1. Goal

A pack author can write `model: grok-4-1-fast` (or any known-prefix model) on any `agent.delegate` node; the runner honors it with documented precedence, rejects typos at pack load, and keeps every current flow resolving exactly as today when `model:` is absent.

## 2. Parent Links

- coding plan: CP-58 P-7 (Tiered Model Strategy)
- tech design: SD-19 §§5-7 (flow executor), SD-23 D-11 (pack-declared contracts)
- system spec: SS-13 §5 (doc contract, N/A to runtime)
- specific upstream ids: Task-305 follow-up "per-node model tiering not yet wired"

## 3. Trigger

Operator approved "Nấc 2" on 2026-09-04 after the model-resolution audit showed Scout (`preflight_contract_plan`, CA-616 hardcode) and `agent.code` nodes cannot be tiered, and YAML has no `model` field at all.

## 4. Exact Change

- `P-1` agentpack (`apps/local-runner/internal/agentpack/pack.go`):
  - `T-1` `FlowNode` gains `Model string`; `flowNodeFromMap` parses `model:` (trimmed; absent → `""`).
  - `T-2` New `ModelProviderKey(model string) (string, bool)`: the canonical prefix table (`gpt-`→`codex`, `gemini-`/`auto-gemini-`→`gemini`, `claude-`→`claude`, `grok-`/`grok-build`→`grok`, `opencode-`/`opencode-go-`→`opencode`), lower-cased + trimmed input, `("", false)` otherwise. Exact replica of runner's current mapping.
  - `T-3` `ValidateFlowDefinition`: node with non-empty `Model` whose canonical behavior != `agent.delegate` → error; delegate node whose model maps to no provider → error.
- `P-2` runner:
  - `T-4` `provider_registry.go providerKeyFromModel` delegates to `agentpack.ModelProviderKey` (same signature/behavior; maps back to `ProviderKey`).
  - `T-5` `flow_executor.go resolveFlowNodeModel`: planner/contract-planner branch returns `node.Model` (was hardcoded `""`); other delegate nodes return DB-row hit, else `node.Model`, else `""` (agentDef/inherit handled downstream unchanged). `delegateSpawnModel` simplifies to `resolveFlowNodeModel` with the CA-616 rationale moved onto the row-skip.
  - `T-6` `supabase_workflow_flow_store.go`: `workflowSelect` adds `model` to the `step_definitions(...)` select; `dbWorkflowStepRow` gains the field; `recordFromWorkflowRow` stamps `node.Model` from it — so cloned/DB-backed flows carry the admin-set model into resolution (planner included).
- `P-3` docs: Task-305 follow-up line flipped to wired-by-Task-320; CP-58 P-7 note (mechanism shipped, no builtin pins per T-2); `change-audit/CA-728-*.md` with ledger block.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/pack.go` (`FlowNode`, `flowNodeFromMap`, `ModelProviderKey`, `ValidateFlowDefinition`)
  - `apps/local-runner/internal/agentpack/node_model_pack_test.go` (new)
  - `apps/local-runner/internal/runner/provider_registry.go` (`providerKeyFromModel` delegation)
  - `apps/local-runner/internal/runner/flow_executor.go` (`resolveFlowNodeModel`, `delegateSpawnModel`)
  - `apps/local-runner/internal/runner/node_model_resolution_test.go` (new)
  - `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` (`workflowSelect`, row struct, `recordFromWorkflowRow`)
  - `requirements/08-Task/todo/Task-320-*.md` (this doc), `change-audit/CA-728-*.md` (new)
- modules: agent-pack loader/validator; flow executor; mirror store
- routes: none
- tables: none (read-only `model` column select; no migration)

## 6. Acceptance Check

- `cd apps/local-runner && go test ./internal/agentpack/ -count=1` PASS (incl. `TestFlowNodeModelValidation`, `TestModelProviderKeyPrefixes`).
- `go test ./internal/runner/ -run 'TestResolveFlowNodeModel|TestProviderKeyFromModel|TestRecordFromWorkflowRow' -count=1` PASS (incl. planner-YAML-honored, CA-616 legacy-row-still-ignored, parity with agentpack table, DB-carry into `node.Model`).
- `go test ./internal/runner/ -run 'TestRAGHarnessLive|TestReviewLoop|TestContextCoding' -count=1` PASS (DOD-6 slice, no regression).
- Full `go test ./internal/agentpack/ ./internal/runner/ -count=1`: only pre-existing environmental failures, zero new (diff against baseline if any fail).

## 7. Out of Scope

- Pinning concrete models in builtin harness YAMLs (T-2).
- `model:` effect on `agent.code` / `hub.inline` (rejected, stays inherit-only).
- Desktop UI surfacing of YAML-effective model; mirror-sync WRITING of `model`.
- Live `/flow` tiering round (operator item 1).

## 8. Completion Notes

- Implemented 2026-09-04 (uncommitted worktree `cp58-harness-dual-loop`):
  P-1 (`FlowNode.Model`, `flowNodeFromMap`, `ModelProviderKey`,
  `ValidateFlowDefinition` fail-closed) + P-2 (`providerKeyFromModel`
  delegation, `resolveFlowNodeModel`/`delegateSpawnModel` precedence,
  `workflowSelect`/`recordFromWorkflowRow` DB-carry; mirror upsert
  untouched so re-sync never clobbers admin `model`) + P-3 (Task-305
  follow-up flipped, CP-58 P-7 mechanism note, CA-728).
- Tests (additive): `node_model_pack_test.go`, `node_model_resolution_test.go`.
  `go test ./internal/agentpack/ -count=1` PASS;
  `go test ./internal/runner/ -run 'TestResolveFlowNodeModel|TestProviderKeyFromModel|TestRecordFromWorkflowRow'` PASS;
  DOD-6 slice `TestRAGHarnessLive|TestReviewLoop|TestContextCoding` PASS.
  Full runner suite: only pre-existing environmental failures, verified
  identical on stashed baseline — zero new (see CA-728 verification list).
