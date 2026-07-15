# CA-319: Drop step_definitions.depends_on_json; Derive Flow Dependency From Edges

## Scope

Fixed BUG-282: `step_definitions.depends_on_json` stored per-flow graph topology (a node's predecessor node ids) on a de-duplicated catalog keyed by `step_type` and reusable across flows (BUG-164). A step attached to a second flow therefore carried the FIRST flow's node ids — which the new flow does not contain — breaking entry/barrier resolution (and, before BUG-262, silently corrupting the other flow on write). Topology is authoritative on `workflows.edges_json`, so the column is redundant.

Implements the bug's F-1 (derive per-node dependency from the flow's forward edges at load time) and F-2 (stop persisting `depends_on_json`; drop the column).

## Changes

- `apps/local-runner/internal/runner/flow_executor.go`: added `forwardEdgeSources(edges, nodeID)` — the `from` of every FORWARD edge targeting `nodeID`, de-duplicated in declared order (back edges excluded), mirroring the client-side `computeDependsOnByStepType`.
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`: `recordFromWorkflowRow` now sets each node's `DependsOn` from `forwardEdgeSources(def.Edges, node.ID)` instead of `step_definitions.depends_on_json`. Removed `depends_on_json` from `workflowSelect`, the `dbStepDefinitionRow.DependsOnJSON` field, and the `upsertNodeStepDefinitions` mirror write.
- `packages/flowpilot-client-core/src/domain/adminModels.ts`: removed the `dependsOn` field from `StepDefinition`; updated the `validateFlowGraph` comment (entry detection reads edges, which is unchanged).
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`: removed `depends_on_json` from `mapStepDefinition` (read), `saveStepDefinition` (write), and `cloneWorkflow` (deep-copy write). Clone still mints an independent, workflow-scoped `step_type` per step (BUG-262 protection for other shared fields is retained).
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: removed `persistEdgeDerivedDependsOn` / `computeDependsOnByStepType` / `arraysEqualUnordered` and their save-time call sites and shared-step skip messaging; removed the now-dead `dependsOn` references in the new-step default and the change-comparison snapshot.
- `supabase/migrations/20260715100000_drop_step_definition_depends_on_json.sql`: `alter table public.step_definitions drop column if exists depends_on_json;` (no data migration — the equivalent info lives on each referencing workflow's `edges_json`).
- Tests: added `TestSupabaseWorkflowFlowStoreGetByRefDerivesDependsOnFromEdges` (edges drive `DependsOn`; back edges excluded); updated the BUG-262 clone test to assert the clone no longer copies `depends_on_json`; updated the BUG-236 workflow_steps field-list comment.

## Verification

- `go build ./...` (apps/local-runner) — passed. `go vet ./internal/runner/` — no issues.
- `go test ./internal/runner/ -run 'FlowStore|FlowExecutor|DerivesDependsOn|EntryDelegate|EntryNodes|FlowStepRuntime|Migration'` — 17 passed, 0 failed (includes the new BUG-282 test).
- Full `go test ./internal/runner/`: the only failures are pre-existing and unrelated — codex/gemini/grok CLI adapters, google-drive MCP provider config, skills-merge precedence, interactive-auth launch, and a Windows TempDir cleanup — none in any file this change touches (matches the documented BUG-262 baseline).
- `npm --prefix apps/desktop-flowpilot run typecheck` — clean.
- `npx tsx --test tests/phase1/workflowFlowEngineAttrs.test.ts` — 8 passed. `npx tsx --test tests/phase1/supabaseAdminRepository.test.ts` — 5 passed.
- GitNexus MCP tools were unavailable in this session (same as BUG-261/BUG-262); proceeded via direct code inspection and by tracing every reader/writer of `depends_on_json` / `FlowNode.DependsOn` manually. `gitnexus_detect_changes()` not run for the same reason.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-282
change_type: bugfix
summary: drop step_definitions.depends_on_json and derive each flow node's dependency from workflows.edges_json at load time, so a step reused across flows no longer carries another flow's node ids
# --->8---
