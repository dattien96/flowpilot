# BUG-282: Step Definition Stores Flow-Scoped depends_on_json, Breaking Cross-Flow Step Reuse

## Metadata

- Document ID: `BUG-282`
- Title: `Step Definition Stores Flow-Scoped depends_on_json, Breaking Cross-Flow Step Reuse`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (moved node/graph fields — including `depends_on_json` — onto `step_definitions`), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-2` three concepts must stay separate: AgentDefinition / FlowDefinition / Workflow+Step), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) (`BR-2` never conflate the three concepts)
- Child Documents: `none`
- Related Documents: [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps Instead Of Step Definitions](../done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md) (established the `workflow_steps`/`step_definitions` split), [BUG-262: Cloned Workflow Shares Step Definitions With Source Silently Corrupting Builtins](../done/BUG-262-Cloned-Workflow-Shares-Step-Definitions-With-Source-Silently-Corrupting-Builtins.md) (same root defect, patched only the clone path + a save-time guard), [CA-260](../../../change-audit/CA-260-clone-workflow-copies-independent-step-definitions.md), [Task-189: Custom Flow Graph Authoring](../../08-Task/done/Task-189-Custom-Flow-Graph-Authoring.md) (introduced `workflows.edges_json` as the authored edge source), migration [`20260703160000_move_flow_node_definition_to_step_definitions.sql`](../../../supabase/migrations/20260703160000_move_flow_node_definition_to_step_definitions.sql), [CA-319: Drop step_definitions.depends_on_json; Derive Flow Dependency From Edges](../../../change-audit/CA-319-drop-step-definition-depends-on-derive-from-edges.md)
- Replaces: `none`
- Tags: `agent-flow-engine, step-definitions, data-model, normalization, data-integrity, workflows, cross-flow-reuse`

## AI Quick View

### Summary

- `step_definitions.depends_on_json` stores **flow-scoped graph topology** (the list of predecessor `node_id`s for a node) on a table that is a **de-duplicated catalog keyed by `step_type`** (BUG-164: "a step type has exactly one configured model"). Topology is per-flow; a catalog row is not — the two are being conflated.
- When the same `step_type` is attached to a second flow (via the "add step" dropdown), its `depends_on_json` still names `node_id`s from the **first** flow's graph — ids that do not exist in the second flow — so the second flow's dependency/entry resolution is wrong (stale barrier that never satisfies / entry node misclassified).
- The authoritative edge list already lives on the flow itself (`workflows.edges_json`, `{from,to,when,kind}`), and the Go runner **already** derives dependency from it (`nodeHasIncomingForwardEdge(def.Edges, ...)`). `depends_on_json` is a **denormalized, edge-derived cache** OR'd with the edge check — redundant, and the sole reason a "step" carries per-flow data at all.
- This is the *root cause* behind the BUG-262 family. BUG-262 only stopped the clone path from silently cross-writing and added a save-time skip guard; it did **not** remove the denormalization, so cross-flow reuse is still broken (and BUG-262's guard now leaves the reused step's deps *stale* instead of *overwritten*).

### Current Ask

- A `step_definitions` row must be flow-independent: reusing one step across flows must not carry (or require) another flow's edge topology.
- Prefer normalizing the topology to its single authoritative home (`workflows.edges_json`) over the current "deep-copy the step per flow" workaround, so a step is genuinely reusable rather than silently duplicated.

### Key Decisions

- `V-1` (CHOSEN, implemented) Made `workflows.edges_json` the single source of truth for topology. The runner now derives each node's `DependsOn` from `def.Edges` at flow-load time via `forwardEdgeSources` (`flow_executor.go`) inside `recordFromWorkflowRow` (`supabase_workflow_flow_store.go`), instead of reading `depends_on_json`. `step_definitions.depends_on_json` is dropped (migration `20260715100000`) and no longer written by any path.
- `V-2` (rejected) Keeping `depends_on_json` and treating a graph step as per-flow (never share `step_type`) was the status-quo direction of BUG-262. Rejected: it does not deliver "an independent, reusable step" — the user's actual ask.
- `V-3` The two entry-detection helpers (`entryDelegateNodes`, `entryNodesNoDeps`) already treated `len(node.DependsOn) > 0 || nodeHasIncomingForwardEdge(def.Edges, ...)`; with `DependsOn` now itself edge-derived, both branches agree. Pack-YAML flows keep their in-memory `FlowNode.DependsOn` (from `dependsOn:`), which is a superset-or-equal of their forward edges (verified across all three pack flows), so mirrored built-ins are unaffected.

### Constraints

- Requires a DB migration to drop the column and a coordinated runner change; both the Go read path (`SupabaseWorkflowFlowStore`) and the frontend write path (`persistEdgeDerivedDependsOn`) touch this field independently.
- `DependsOn` is used at runtime for **two** things, not one: (a) entry-node classification, and (b) the spawn-time dependency barrier for downstream nodes (`FlowNode.DependsOn` carried into spawn). Both are derivable from the same forward-edge set, but the barrier path must be re-derived too, not just entry detection.
- File-pack flows (`flow-pack/flows/*.yaml`) may still express dependency via a node's own `dependsOn:` field rather than edges — the fix keeps parsing pack-authored `dependsOn` (in-memory `FlowNode.DependsOn` stays valid) and only removes the **Supabase `step_definitions` column** as a persistence home. Verified across all three pack flows (`review-loop`, `context-coding-review-synthesis`, `rag-harness`) that every `dependsOn` has a matching forward edge, so edge-derivation is equivalent-or-stronger for mirrored built-ins.
- Fix implemented and verified this turn (Go build/vet/test, desktop typecheck, phase1 TS). GitNexus MCP tools were unavailable (same as BUG-261/262); proceeded via direct inspection.

### Open Questions

- `Q-1` (resolved) Adopt `V-1` (normalize + drop column). Chosen and implemented.
- `Q-2` (resolved) No one-time data cleanup needed: the column is dropped, and the runner derives dependency from each flow's `edges_json`, so any previously-stale `depends_on_json` value is simply gone — the equivalent (correct, per-flow) information already lives on `edges_json`.

### Source Refs

- Fix (new): `supabase/migrations/20260715100000_drop_step_definition_depends_on_json.sql` (drop column); `forwardEdgeSources` in `apps/local-runner/internal/runner/flow_executor.go`; `recordFromWorkflowRow` edge-derivation in `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`; `CA-319`.
- Origin schema: `supabase/migrations/20260703160000_move_flow_node_definition_to_step_definitions.sql:7-18` (added `depends_on_json` to `step_definitions`) and its own comment `:24-29` warning multiple graph nodes can share a `step_type` row.
- Runner (before fix): `supabase_workflow_flow_store.go` `node.DependsOn` from `defn.DependsOnJSON`, `workflowSelect` pulled `depends_on_json`; `flow_executor.go` `entryDelegateNodes` / `entryNodesNoDeps` / spawn barrier read `node.DependsOn`.
- Frontend (removed): `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` `computeDependsOnByStepType` / `persistEdgeDerivedDependsOn` and its BUG-262 shared-step skip guard; `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` `mapStepDefinition` read + `saveStepDefinition` / `cloneWorkflow` writes; `packages/flowpilot-client-core/src/domain/adminModels.ts` `StepDefinition.dependsOn` field.
- New/updated tests: `TestSupabaseWorkflowFlowStoreGetByRefDerivesDependsOnFromEdges` (`supabase_workflow_flow_store_test.go`); updated BUG-262 clone test + BUG-236 field-list (`tests/phase1/workflowFlowEngineAttrs.test.ts`).
- Prior art: BUG-262 §6/§7 (same root, clone-path-only fix), CA-260.

## 1. Issue Summary

A step (`step_definitions` row) is meant to be an independent, reusable definition. But it stores `depends_on_json`, which is the list of predecessor nodes for that node **within one specific flow's graph**. When the user attaches the same step to a different flow, the stored dependency still points at the original flow's node ids, which the new flow does not contain — so the new flow errors (its dependency/entry resolution is based on ids that never exist or never complete). The user's report: a step should be independent and must not store edge information like this.

## 2. Parent Links

- impacted coding plan: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) — the plan under which node/graph fields (`node_id`, `behavior_id`, `agent_ref`, `depends_on_json`, `join_mode`, `cohort`, ...) were moved onto `step_definitions`.
- impacted tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — `D-2` mandates the three concepts (AgentDefinition / FlowDefinition / Workflow+Step) stay separate and never conflated; storing per-flow topology on the reusable Step definition violates that separation in practice.
- impacted system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) — `BR-2` (three concepts never conflated).

## 3. Environment and Reproduction

- environment: Desktop app -> Settings -> Workflows, Supabase-backed workspace; Go local-runner flow execution.
- reproduction steps:
  1. Create/have Flow A with a step S wired via edges (S has an incoming forward edge from node X). Save — the system derives and stores `depends_on_json=[X]` on S's `step_definitions` row (`persistEdgeDerivedDependsOn`).
  2. Create Flow B and add the same step S via the "add step" dropdown (reuses the same `step_type`, hence the same `step_definitions` row).
  3. Wire B's edges differently (X is not present in B). Save B.
  4. Run/resolve Flow B. Observe the error/incorrect behavior: S is treated as depending on `X`, a node id that does not exist in B (stale barrier never satisfied / entry misclassification).
- frequency: deterministic for any flow that reuses an existing `step_type` whose `depends_on_json` was derived from a different flow's edges. Post-BUG-262 the stored value is left *stale* (the shared-step guard skips the rewrite) rather than overwritten.

## 4. Expected vs Actual

- expected: a step attached to a flow derives its dependencies from **that flow's** graph (`workflows.edges_json`); the same step reused elsewhere resolves cleanly against the new flow's edges. A step definition carries no flow-specific topology.
- actual: the step carries `depends_on_json` from whichever flow last wrote it; reusing it in a new flow drags along foreign `node_id`s, breaking resolution in the new flow (or, pre-BUG-262, silently corrupting the other flow on write).

## 5. Impact

- users affected: anyone authoring more than one flow and reusing a step across them via the "add step" dropdown — the natural way to reuse a definition.
- workflows affected: any flow that shares a `step_type` with another flow; the class includes built-ins (BUG-262 observed `review-loop` corruption via this same field).
- severity: medium-high — no reuse of graph steps is actually safe today; the symptom is confusing (a flow "just errors"/hangs on resolve with no obvious link to a step edited elsewhere), and the underlying defect already caused a high-severity built-in corruption (BUG-262).

## 6. Root Cause

- hypothesis: `depends_on_json` is per-flow topology stored on a per-`step_type` shared catalog row, so it cannot be simultaneously correct for two flows.
- confirmed cause (by code inspection this turn): 
  - `step_definitions` is upserted/read by `step_type` alone (shared catalog — BUG-164), and `depends_on_json` was added to it by migration `20260703160000` (`:12`).
  - `persistEdgeDerivedDependsOn` (`WorkflowsSettings.tsx:1223`) computes the value purely from the currently-open flow's edges (`computeDependsOnByStepType`, `:1179`) and writes it keyed by `step_type` — so the value is only ever right for the last flow saved.
  - The Go runner reads it into `FlowNode.DependsOn` (`supabase_workflow_flow_store.go:213`) and uses it for entry classification (`flow_executor.go:1215`, `:772`) and the spawn-time barrier (`:1175`).
  - Crucially, the runner ALSO reads the authoritative edges (`def.Edges` from `edges_json`, `:200`) and the entry helpers already OR in `nodeHasIncomingForwardEdge(def.Edges, ...)` — so `depends_on_json` is a redundant second source of truth for a fact the edges already carry.
- evidence: BUG-262 §6 documents the exact shared-row mechanism and a live corrupted row; this bug generalizes it from "clone" to "any cross-flow reuse" and identifies the schema decision (not just the clone code) as the root cause. A fresh live capture will be attached on reproduction (see Source Refs).

## 7. Fix Strategy

> Implemented this turn via `F-1` + `F-2` (normalize + drop column). `F-3`/`F-4` were the rejected/moot alternatives.

- `F-1` (done) Derive per-node dependency from `workflows.edges_json` at flow-load time in the Go runner. Added `forwardEdgeSources(edges, nodeID)` (`flow_executor.go`) — the `from` of every forward edge targeting `nodeID`, de-duplicated, back edges excluded. `recordFromWorkflowRow` (`supabase_workflow_flow_store.go`) now sets `node.DependsOn = forwardEdgeSources(def.Edges, node.ID)` instead of reading `depends_on_json`. Entry detection and the spawn barrier both read `node.DependsOn`, so populating it from edges fixes both at once with no downstream change.
- `F-2` (done) Stopped persisting `depends_on_json`: removed the Go mirror write and the field from `workflowSelect`/`dbStepDefinitionRow`; removed the frontend `persistEdgeDerivedDependsOn` machinery, the `saveStepDefinition`/`cloneWorkflow` writes, the `mapStepDefinition` read, and the `StepDefinition.dependsOn` model field; added migration `20260715100000_drop_step_definition_depends_on_json.sql`. Pack-authored in-memory `FlowNode.DependsOn` parsing (`pack.go`) is intact.
- `F-3` (rejected) Keeping the column and scoping steps per-flow — does not deliver a reusable step (see `V-2`).
- `F-4` (moot) No data cleanup needed once the column is dropped (`Q-2`).

## 8. Validation

- `V-1` (done) Unit: `TestSupabaseWorkflowFlowStoreGetByRefDerivesDependsOnFromEdges` (`supabase_workflow_flow_store_test.go`) loads a flow whose steps carry NO `depends_on_json`, with `edges_json` wiring `coder->reviewer` (forward) and `synthesis->coder` (back); asserts `reviewer.DependsOn == [coder]`, `coder.DependsOn == []` (back edge excluded), `synthesis.DependsOn == []`. Passes.
- `V-2` (done) Regression intent covered: the BUG-262 clone test now asserts the clone does NOT copy `depends_on_json` (topology never travels with a step); combined with `V-1` this proves each flow resolves deps from its own edges. `tests/phase1/workflowFlowEngineAttrs.test.ts` — 8 passed.
- `V-3` (done) `go test ./internal/runner/ -run 'FlowStore|FlowExecutor|DerivesDependsOn|EntryDelegate|EntryNodes|FlowStepRuntime|Migration'` — 17 passed, 0 failed. `go build ./...` and `go vet ./internal/runner/` — clean.
- `V-4` (done) `npm --prefix apps/desktop-flowpilot run typecheck` — clean. `npx tsx --test tests/phase1/supabaseAdminRepository.test.ts` — 5 passed.
- `V-5` Full `go test ./internal/runner/`: the only failures are pre-existing and unrelated (codex/gemini/grok CLI adapters, google-drive MCP provider config, skills-merge precedence, interactive-auth launch, a flaky Windows TempDir cleanup) — the failing set even varied run-to-run (14 → 12), confirming they are environmental, and none are in `flow_executor_test.go` / `supabase_workflow_flow_store_test.go` / `flow_step_runtime_test.go`.
- `V-6` GitNexus MCP unavailable this session (same as BUG-261/262); verified by direct inspection of every reader/writer of `depends_on_json` / `FlowNode.DependsOn` instead. `gitnexus_detect_changes()` not run for the same reason.

## 9. Regression Guard

- tests: `TestSupabaseWorkflowFlowStoreGetByRefDerivesDependsOnFromEdges` (edges-only resolution + back-edge exclusion) and the updated BUG-262 clone test (clone strips topology) guard against the denormalization returning.
- alerts: none (design/data-model bug, not a runtime alertable condition).
- audit checks: `gitnexus_detect_changes()` not run (GitNexus unavailable — `V-6`); recorded in `change-audit/CA-319`.

## 10. Follow-Up Document Updates

- upstream docs that should be noted: [CP-42](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) and [SD-19 §5 Data Model](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) describe `depends_on_json` on `step_definitions`; per this fix, per-flow topology now lives solely on `workflows.edges_json` and `step_definitions.depends_on_json` is dropped. These are `done`-phase docs; this BugFix + CA-319 record the delta rather than rewriting them.
- notes left unchanged on purpose: `step_definitions` remaining a shared catalog for genuinely definition-level fields (`model`, `prompt_base`, etc., per BUG-164) is correct and unchanged — only the flow-scoped `depends_on_json` was the misplaced field. The BUG-262 clone deep-copy (independent workflow-scoped `step_type`) is retained for those other shared fields.
