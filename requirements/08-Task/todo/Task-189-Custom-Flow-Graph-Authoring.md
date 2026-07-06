# Task-189: Custom Flow Graph Authoring (Steps + Edges + Behavior, All In UI)

## Metadata

- Document ID: `Task-189`
- Title: `Custom Flow Graph Authoring (Steps + Edges + Behavior, All In UI)`
- Phase: `task`
- Status: `draft` (PLAN — awaiting owner approval before implementation)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/inprogress/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (P-9), [Task-179: Settings Flow Pack Authoring UI](Task-179-Settings-Flow-Pack-Authoring-UI.md)
- Child Documents: `none`
- Related Documents: [BUG-236: Builtin Flow Mirror Stores Node Definition On workflow_steps Instead Of step_definitions](../../09-BugFix/todo/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md), [Task-175: Built-In Flow Mirror Sync And Resolver](../done/Task-175-Builtin-Flow-Mirror-Sync-And-Resolver.md), [Task-176: Node Behavior Registry And Dispatch](Task-176-Node-Behavior-Registry-And-Dispatch.md)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-authoring, custom-flow, step-definitions, edges, behavior, desktop, canvas`

## AI Quick View

### Summary

- **Domain contract (owner-confirmed 2026-07-06, = BUG-236):** a NODE's definition — `behavior`, `agent`, `depends_on`, `join`, `cohort`, `lifecycle`, prompt/context refs — lives in **`step_definitions`**. `workflow_steps` is a **pure relation/order table** (`step_type` + `order_index`) and must NOT store node data. A FLOW is a `workflows` row (edges + policy) linking ordered `step_definitions`. To give two flows a differently-configured "coder" node, you create two distinct `step_definitions` — you do **not** override on `workflow_steps`.
- The runtime already honors this: the flow definition store reads node data via the nested `workflow_steps → step_definitions` join (`supabase_workflow_flow_store.go:112`), so `resolveWorkflowFlowRef` builds a runnable `FlowDefinition` from step-definition data + the workflow's `edges_json`. **No type/schema/persistence change is needed on the workflow_steps side** (this supersedes the earlier, wrong plan to persist node attrs on `workflow_steps`).
- What's actually missing to author a runnable custom flow from scratch in the UI:
  1. **No edges/graph authoring.** `WorkflowsSettings.tsx` has no edges editor; `Workflow.edges` is a load→save pass-through, so a new workflow gets `edges: []` → the executor's `forwardDoneTargets` finds nothing and the flow never advances.
  2. **Node authoring is incomplete for runnability.** Task-179's step-definition form exposes Behavior/Agent/etc. fields, but (a) Behavior is free-text (should be a picker of valid behavior IDs) and (b) a from-scratch step-type needs `behavior_id`+`agent_ref` actually set or the node is skipped as a non-delegate. The seeded generic step-types (`flow-agent-delegate-coder`) carry no `behavior_id`/`agent_ref`.
  3. **No workflow-assembly + validation UX** that ties nodes (step_definitions) + order + edges + policy into a coherent, pre-validated flow.
- Consequence today: clone-a-built-in-and-run works; from-scratch custom flow does not run as a flow. This task makes from-scratch custom-flow authoring work, using the step_definitions-as-node model.

### Current Ask

- Let a user, entirely in the UI: define nodes (as `step_definitions` with behavior/agent/deps), assemble them into a workflow with an edge graph + policy, and run it in Flow Mode exactly like a built-in — with node data living only in `step_definitions`.

### Key Decisions

- `T-1` **Node data stays in `step_definitions`; `workflow_steps` remains a pure relation.** (Owner direction; = BUG-236 contract.) No `WorkflowStep` type change, no `workflow_steps` node columns, no per-instance override. The runtime already reads node data from `step_definitions` via the nested join — confirm and, if any residual `workflow_steps`-node-data read/fallback exists anywhere, remove it so the model is unambiguous.
- `T-2` **Edges editor — BOTH a form-list AND a visual canvas, delivered together** (owner: "cả hai cùng lúc"). Author `workflows.edges_json` (from/to/when/kind) + policy (cap/onCap/extendBy/extendMax). Form-list = editable rows with node-id + terminal-pseudo-node (`done`/`ask_user`) dropdowns; canvas = drag/drop nodes, draw edges by connecting them, both bound to the same `edges` state so they stay in sync.
- `T-3` **Behavior picker = a static shared list** mirroring `DefaultBehaviorRegistry`'s IDs (owner-confirmed), replacing the free-text Behavior field on the step-definition form. Revisit as a runtime endpoint only if behaviors become dynamic.
- `T-4` **Validation before save:** every edge `from`/`to` references a declared node (a step in this workflow) or a terminal pseudo-node; a `agent.delegate` node has an `agent_ref`; behavior IDs are known; policy values sane; ≥1 no-dependency entry node exists. Surface errors in the UI.
- `T-5` **Workflow-assembly UX:** pick/create the step_definitions that are this flow's nodes, order them, draw edges, set policy — save produces a user-owned `workflows` row + `workflow_steps` relation rows that `resolveWorkflowFlowRef` runs.

### Constraints

- Reuse the existing `workflows`/`workflow_steps`/`step_definitions` schema and the `FlowDefinitionResolver`/`SupabaseWorkflowFlowStore` path (CP-42/Task-175). Built-in mirror sync must keep working unchanged. Custom flows resolve through the SAME `resolveWorkflowFlowRef` → `startResolvedFlow`/`startInlineEntryChain` path.
- Do not regress the "clone a built-in and run" path.
- Validate/audit node EXECUTION (mid-flow inline nodes) is out of scope — that is BUG-243, deferred into CP-43. This task's runnable E2E targets a delegate-based graph (e.g. coder + reviewer cohort + a hub), not the validate/audit tail. A custom flow may still declare such nodes; they just won't execute until CP-43.

### Open Questions (all resolved 2026-07-06)

- `Q-1` Edge editor UX → **RESOLVED: build both the form-list and the visual canvas, together.**
- `Q-2` Per-instance vs shared node data → **RESOLVED: node data lives only in `step_definitions`; `workflow_steps` is a pure relation. No per-instance override.**
- `Q-3` Behavior picker source → **RESOLVED: static shared list mirroring `DefaultBehaviorRegistry`.**

### Source Refs

- Runtime (already correct, node data from step_definitions): `apps/local-runner/internal/runner/supabase_workflow_flow_store.go:112` (`workflowSelect` nested join), `recordFromWorkflowRow`, `flow_executor.go` `resolveWorkflowFlowRef`/`forwardDoneTargets`/`tryAdvanceFlowFromNode`.
- Gaps (UI): `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (no edges editor; Behavior is free-text), `Workflow.edges` in `packages/flowpilot-client-core/src/domain/adminModels.ts` (load→save passthrough), `supabaseAdminRepository.ts` `saveWorkflow`/`cloneWorkflow` (persist `edges_json`/policy).
- Behavior set: `apps/local-runner/internal/runner/behavior_registry_builtin.go` `DefaultBehaviorRegistry` (the IDs to mirror in the static picker list).

## 1. Goal

Enable full custom-flow authoring in Settings under the `step_definitions`-as-node model: define nodes as step-definitions (behavior/agent/deps), assemble them into a workflow with an edge graph + policy via both a form-list and a visual canvas, validate, save, and run in Flow Mode exactly like a built-in.

## 2. Parent Links

- coding plan: `CP-42` (P-9 Settings UI flow authoring)
- prior task: `Task-179` (step-definition form + clone; this task adds edges authoring, the behavior picker, assembly, and validation, under the corrected node-data-in-step_definitions model)

## 3. Trigger

Owner requirement: "I need to create my own custom flows — steps, edges, behavior, all settable in the UI — and they must work exactly like the built-in flows." Verified from-scratch custom flows don't currently run (no edges authoring; incomplete node authoring).

## 4. Exact Change (proposed — finalize on approval)

- `T-1` Confirm the runtime reads node data solely from `step_definitions` (it does, via the nested join); remove any residual `workflow_steps`-node-data read/fallback if found. No `WorkflowStep`/schema change.
- `T-2` Behavior picker: replace the free-text Behavior field on the step-definition form with a dropdown from a static shared behavior-ID list (mirroring `DefaultBehaviorRegistry`).
- `T-3` Edges editor in `WorkflowsSettings.tsx`: (a) form-list of edges (from/to/when/kind) + policy fields, bound to `WorkflowDraft.edges`; (b) a visual canvas (nodes + draw-edges) bound to the same state. Persist `edges_json`/policy in `saveWorkflow`/`cloneWorkflow`.
- `T-4` Workflow-assembly UX: add/select step-definitions as nodes, order, edges, policy — coherent create/edit flow for a user-owned workflow.
- `T-5` Client validation (Key Decisions `T-4`) surfaced before save.
- `T-6` Tests: unit (edge/graph validation; assembly round-trip), a runner test that a from-scratch custom workflow (step-definitions with behavior/agent + edges) resolves via `resolveWorkflowFlowRef` and spawns its entry node + advances an edge; manual E2E (author a custom flow in Settings, run it, agents spawn, edges followed).

## 5. Touched Areas

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (edges form-list + canvas + assembly + validation + behavior picker) and workflow-draft state.
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (persist `edges_json`/policy on save/clone), `.../domain/adminModels.ts` (a static behavior-ID list; NO WorkflowStep node fields).
- `apps/local-runner/internal/runner/` (confirmation only that node data reads from step_definitions; remove residual workflow_steps-node-data path if any).
- tests: desktop + a runner custom-flow-resolution test.

## 6. Acceptance Check / Test Items / DoD (finalize on approval)

- A from-scratch custom flow authored in the UI (nodes = step-definitions with behavior/agent; edges drawn in form + canvas; policy set) runs in Flow Mode: entry node spawns, forward edges advance, cohort/join behave per the authored graph — no fallback to the legacy non-flow path.
- Node data is stored only in `step_definitions`; `workflow_steps` carries only step_type + order.
- Form-list and canvas stay in sync (edit in one reflects in the other).
- Cloning a built-in still works (no regression).
- Validation blocks an invalid graph (dangling edge, delegate without agent, unknown behavior, no entry node) with clear messages.
- `go build ./...`, `go test ./internal/runner/...`, desktop `npm run typecheck`, phase1 tests pass.

## 7. Out of Scope

- validate/audit node EXECUTION (BUG-243 → CP-43).
- Context-harness / RAG retrieval rework (CP-43).
- Any move of node data onto `workflow_steps` (explicitly rejected — node data stays in `step_definitions`).

## 8. Completion Notes

- result: `plan` — not started; scope corrected 2026-07-06 to the step_definitions-as-node model per owner direction (dropped the earlier workflow_steps-persistence approach). On approval, finalize §6 and implement across the behavior picker → edges form-list + canvas → assembly + validation → runner confirmation → tests.
