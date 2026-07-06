# Task-189: Custom Flow Graph Authoring (Edges + Per-Instance Node Behavior)

## Metadata

- Document ID: `Task-189`
- Title: `Custom Flow Graph Authoring (Edges + Per-Instance Node Behavior)`
- Phase: `task`
- Status: `draft` (PLAN — awaiting owner approval before implementation)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/inprogress/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (P-9), [Task-179: Settings Flow Pack Authoring UI](Task-179-Settings-Flow-Pack-Authoring-UI.md)
- Child Documents: `none`
- Related Documents: [Task-175: Built-In Flow Mirror Sync And Resolver](../done/Task-175-Builtin-Flow-Mirror-Sync-And-Resolver.md), [Task-176: Node Behavior Registry And Dispatch](Task-176-Node-Behavior-Registry-And-Dispatch.md)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-authoring, custom-flow, workflow-steps, edges, behavior, desktop, supabase`

## AI Quick View

### Summary

- Task-179 gave the Settings > Workflows screen per-node **form fields** and a Clone action, but two gaps mean a user cannot author a genuinely-runnable custom flow from scratch (verified 2026-07-06):
  1. **No edges/graph authoring.** `WorkflowsSettings.tsx` has no edges editor; `Workflow.edges` is a pure load→save pass-through, so a brand-new workflow gets `edges: []`. Without edges the flow executor's `forwardDoneTargets` finds nothing and `tryAdvanceFlowFromNode` bails — no cohort/join/retry.
  2. **Per-instance node attrs not persisted.** `WorkflowStep` (`adminModels.ts:157-166`) has no `nodeId`/`behaviorId`/`agentRef`/…; `saveWorkflow`/`cloneWorkflow` write only `step_type`/`order_index`/`is_enabled`/`requires_approval` into `workflow_steps`. Node behavior/agent today come only from the shared `step_definitions` row (via the runtime's definition fallback), so two flows can't give their own node distinct behavior/agent/deps.
- Consequence today: **clone-a-built-in-and-run works** (edges + node attrs are deep-copied and still join to attr-bearing `step_definitions`), but a **from-scratch custom flow does not run as a flow** — `resolveWorkflowFlowRef` finds no delegate entry node and silently falls back to the legacy non-flow hub path.
- This task closes both gaps so a user can define a flow's nodes, per-node behavior/agent, dependencies, and edges entirely in the UI and have it execute exactly like a built-in.

### Current Ask

- Make "create a custom flow (steps + edges + per-node behavior) in Settings and run it in Flow Mode" work end to end, matching built-in flow execution.

### Key Decisions

- `T-1` Persist per-instance node attrs on `workflow_steps`, and make the runtime prefer them over the `step_definitions` fallback (the DB columns and the runtime read-with-fallback already exist per Task-175/CP-42; the missing link is the client write path + the `WorkflowStep` type).
- `T-2` Add an edges editor to `WorkflowsSettings.tsx` and persist `edges_json` (column exists); MVP = a form-based edge list (`from`/`to`/`when`/`kind` rows with node-id + terminal-pseudo-node dropdowns), not a visual canvas (canvas is a follow-up).
- `T-3` Feed the behavior-ID picker from the runner's registered behavior set (a read-only endpoint or a shared constant), so users pick valid behaviors (`agent.delegate`, `hub.inline`, `context.produce`, …) rather than free-text.
- `T-4` Client-side validation before save: every edge `from`/`to` references a declared node or a terminal pseudo-node (`done`/`ask_user`); a `agent.delegate` node has an agent ref; policy cap/extend values are sane; at least one no-dependency entry node exists.

### Constraints

- Reuse, don't replace, the existing `workflows`/`workflow_steps` schema and `FlowDefinitionResolver`/`SupabaseWorkflowFlowStore` path (CP-42/Task-175). Built-in mirror sync must keep working unchanged.
- A custom (user-owned, `is_builtin=false`) flow must resolve through the SAME `resolveWorkflowFlowRef` → `startResolvedFlow`/`startInlineEntryChain` path built-ins use — no parallel executor.
- Do not regress the "clone a built-in and run" path that already works.
- Validation node execution (validate/audit) is out of scope here — that is BUG-243, deferred into CP-43. A custom flow with a `command.validate`/`artifact.audit_draft` node will have the same mid-flow-inline gap until CP-43; this task's E2E targets a delegate-based graph (e.g. a coder + reviewer cohort), not the validate/audit tail.

### Open Questions

- `Q-1` Edge authoring UX: form-based edge list (recommended MVP) vs a visual node-graph canvas (later)?
- `Q-2` Should per-instance node attrs, when left blank, still fall back to the referenced `step_definitions` row (backward-compatible), or must a custom flow set them explicitly? (Recommend: fall back, so existing clones keep working.)
- `Q-3` Behavior-ID source for the picker: a new `GET /client/flow/behaviors` endpoint from the runner registry, or a shared static list in `flowpilot-client-core`? (Recommend: static shared list mirroring `DefaultBehaviorRegistry`, revisited if behaviors become dynamic.)

### Source Refs

- Gaps: `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (no edges editor; step form edits `step_definitions`), `packages/flowpilot-client-core/src/domain/adminModels.ts:157-166` (`WorkflowStep` lacks node attrs), `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` `saveWorkflow`/`cloneWorkflow` (write only step_type/order_index).
- Runtime target: `apps/local-runner/internal/runner/flow_executor.go` `resolveWorkflowFlowRef`/`tryAdvanceFlowFromNode`/`forwardDoneTargets`, `supabase_workflow_flow_store.go` `recordFromWorkflowRow`/node construction (per-instance-over-definition precedence).
- Schema: `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql` (`edges_json` on workflows; node attrs on step_definitions — confirm/add the same on `workflow_steps`).

## 1. Goal

Let a user author a complete custom flow in Settings — nodes with per-node behavior/agent/dependencies, the edge graph, and policy — persisted to `workflows`/`workflow_steps`, and have it run in Flow Mode through the existing generic executor exactly as a built-in flow does.

## 2. Parent Links

- coding plan: `CP-42` (P-9 Settings UI flow authoring)
- prior task: `Task-179` (added the step form + clone; this task completes its deferred graph/persistence scope)

## 3. Trigger

Owner requirement: "I need to create my own custom flows — with steps, edges, and behavior settable in the UI — and they must work, exactly like the built-in flows are defined." Verified that from-scratch custom flows do not currently run as flows.

## 4. Exact Change (proposed — refine on approval)

- `T-1` **Persist per-instance node attrs.** Add `nodeId`/`behaviorId`/`agentRef`/`dependsOn`/`joinMode`/`cohort`/`nodeLifecycle` to the `WorkflowStep` domain type; extend `mapWorkflowStep` to read them and `saveWorkflow`/`cloneWorkflow` to write them into `workflow_steps` (add columns via migration if not already present). Fix/enable `tests/phase1/workflowFlowEngineAttrs.test.ts`.
- `T-2` **Edges editor UI.** In `WorkflowsSettings.tsx`, add an edges editor (form-based rows: from-node, to-node/terminal, `when` status, `kind` forward/back) bound to `WorkflowDraft.edges`, persisted to `workflows.edges_json`. Add policy fields (cap/onCap/extendBy/extendMax) if not already editable.
- `T-3` **Behavior picker from registry.** Replace the free-text Behavior ID field with a dropdown of registered behavior IDs (`Q-3`).
- `T-4` **Runtime precedence.** Ensure `recordFromWorkflowRow`/node construction prefers per-instance `workflow_steps` node attrs over the `step_definitions` fallback, so a custom flow's own node behavior/agent/deps drive execution. Confirm `resolveWorkflowFlowRef` builds a runnable `FlowDefinition` (entry node found, edges present) for a user-owned workflow.
- `T-5` **Validation UI** (`T-4` in Key Decisions): edge endpoints, delegate-needs-agent, known behavior IDs, sane policy, ≥1 entry node — surfaced before save.

## 5. Touched Areas

- `packages/flowpilot-client-core/src/domain/adminModels.ts`, `.../data/supabaseAdminRepository.ts`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (+ any workflow draft state)
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`, `flow_executor.go` (precedence + resolution confirmation)
- `supabase/migrations/` (new migration if `workflow_steps` lacks the node columns)
- tests: `tests/phase1/workflowFlowEngineAttrs.test.ts`, new runner test for custom-flow resolution + entry spawn

## 6. Acceptance Check / Test Items / DoD (to finalize on approval)

- A from-scratch custom flow (≥1 no-dep `agent.delegate` entry node + edges) created in Settings runs in Flow Mode: entry node spawns, forward edges advance, cohort/join behave per the authored graph — no fallback to the legacy non-flow path.
- Per-instance node attrs round-trip through `workflow_steps` and drive execution independent of the shared `step_definitions` row.
- Cloning a built-in still works (no regression); edges + node attrs are preserved.
- Client validation blocks an invalid graph (dangling edge, delegate without agent, unknown behavior) with a clear message.
- `go build ./...`, `go test ./internal/runner/...`, desktop `npm run typecheck`, and the phase1 test suite pass.

## 7. Out of Scope

- validate/audit node execution (BUG-243 → CP-43).
- Visual node-graph canvas (form-based MVP only; canvas is a follow-up).
- Context-harness / RAG retrieval rework (CP-43).

## 8. Completion Notes

- result: `plan` — not started; awaiting owner approval of scope (esp. `Q-1`/`Q-2`/`Q-3`). On approval, finalize §6 DoD/test items and implement across client-core → desktop UI → runner precedence.
