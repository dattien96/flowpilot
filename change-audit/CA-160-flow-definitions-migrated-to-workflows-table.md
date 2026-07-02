# CA-160: Flow Definitions Migrated Onto workflows/workflow_steps (Reuse Settings UI)

## Scope

A user-directed architectural correction, superseding the `flow_definitions` table design from CA-150/154/157/159: rather than a parallel table and a new Settings screen, built-in flows are now mirrored into the **existing** `workflows`/`workflow_steps` tables, and the **existing** "Workflows/Steps" Settings screen (`WorkflowsSettings.tsx`) is the authoring UI. This closes Task-179 for real (a working screen, not just a backend contract) and also resolves two open design questions from the prior session (hub wait-notice framing; the "downstream transitions aren't FlowEdge-driven" gap remains, tracked separately).

## Why this changed

The previous design (CA-150 onward) built a standalone `flow_definitions` table and Go-only `GET/POST/PUT /client/flows*` endpoints, on the assumption that Task-179's Settings UI would be new local-runner-backed screens. Investigating the real Settings architecture (`WorkflowsSettings.tsx` → `getAdminUseCases()` → `SupabaseAdminRepository`) showed Settings talks **directly to Supabase** via `packages/flowpilot-client-core`, never through local-runner's HTTP API. The user confirmed: reuse the existing Workflows/Steps screen, mirror built-ins into it, and extend its schema with the new flow-engine attributes — rather than run two parallel systems.

## Schema (supersedes the removed `flow_definitions` migration)

`supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql` (rewritten in place — the old `flow_definitions` version was never applied to any live database, so no data migration was needed):

- **`workflows`**: `is_builtin`, `editable`, `cloneable`, `cloned_from`, `pack_id`/`pack_version`/`pack_flow_id`/`pack_hash` (mirror-sync identity/staleness), `selectable_in_json`, `chat_baseline`, `chat_sub_modes_json`, `policy_cap`/`policy_on_cap`/`policy_extend_by`/`policy_extend_max`, `edges_json` (whole-graph edge list). **Update**: `edges_json` is now read by the live executor — see [CA-163](./CA-163-forward-edge-auto-spawn-reviewer-cohort.md) (forward-edge auto-spawn) and [CA-161](./CA-161-edge-driven-continue-reinvoke.md) (back-edge continue reinvoke); this line originally said "not yet read," which is no longer accurate. Unique index on `(pack_id, pack_flow_id) where is_builtin`.
- **`workflow_steps`**: `node_id` (stable flow-graph id, e.g. `"coder"` — distinct from `step_type`, which is the existing reusable step-definition key), `behavior_id`, `agent_ref`, `depends_on_json`, `join_mode`, `cohort`, `prompt_template_ref`, `context_ref`.
- Seeded 9 generic `step_definitions` rows (`flow-agent-delegate`, `flow-hub-inline`, etc.) so a mirrored flow-engine node has a valid `step_type` to satisfy the existing NOT NULL FK, while its real identity lives in the new columns.

## Go backend (`apps/local-runner`)

- Removed: `flow_definition_store_file.go`, `supabase_flow_definition_store.go` (and tests) — the `flow_definitions`-table-backed implementations.
- Added `SupabaseWorkflowFlowStore` (`supabase_workflow_flow_store.go`), implementing `FlowDefinitionStore` against `workflows`/`workflow_steps` via PostgREST (`select=*,workflow_steps(...)` embed). `FlowDefinitionStoreFor(r *Runner)` now returns `nil` (not a file-store fallback) when Supabase isn't configured — matching the Settings UI's own hard Supabase requirement; `FlowDefinitionResolver` already falls back to the embedded pack directly in that case.
- `FlowDefinitionRecord` gained a `Name` field (Workflow's human display name, distinct from `Definition.Description`). `FlowDefinitionStore.Upsert` now returns `(FlowDefinitionRecord, error)` instead of just `error`, since a brand-new user-owned row needs to report back its store-assigned UUID.
- `FlowRef` has two shapes now: a stable `"packId/flowId"` string for built-ins (survives a mirror row being re-created), or the workflow's own UUID `id` for user-owned rows (there is no per-user "owner" concept in this schema — a flow belongs to a project or is workspace-global, matching `WorkflowsSettings.tsx`'s "Workspace global" option).
- `CloneBuiltin(ctx, packID, flowID, name)` (was `CloneBuiltin(ctx, ownerID, packID, flowID, newFlowID)`) — no more caller-supplied id; the store assigns one.
- Removed the now-dead `GET/POST/PUT /client/flows*` endpoints (CA-159) — Settings UI never calls local-runner for this.
- Added `prompts/flow-start-wait.md` to the agent pack and `notifyHubFlowStarted` in `flow_executor.go`: per explicit user direction, when a flowRef auto-spawns the entry node, the hub gets a pending note ("an agent has already been spawned... do not write or edit code yourself") through the same delivery mechanism used for "coder completed" notices, so it doesn't redundantly try to also do the coding.

## Frontend (`packages/flowpilot-client-core`, `apps/desktop-flowpilot`)

- `Workflow`/`WorkflowStep` interfaces extended with the same new fields (camelCase), plus a `WorkflowFlowEdge` type.
- `WorkflowRepository.cloneWorkflow(workflowId, name)` added; `SupabaseAdminRepository` implements it (deep-copies steps) and `saveWorkflow` now rejects edits when the existing row's `editable` is `false`.
- `WorkflowsSettings.tsx`: a "Built-in" badge on built-in rows, Save/Delete hidden and replaced with "Clone" for non-editable workflows, a Clone modal, the step editor gated read-only for built-ins (Add/Move/Remove disabled), and six new step fields (Node ID, Behavior ID, Agent ref, Depends on, Join mode, Cohort) in the step card form — a form-based MVP per Task-179's own allowance to defer a full graph editor.
- `admin-web` does not import `@flowpilot/client-core` at all — confirmed zero impact; its pre-existing (unrelated) TypeScript errors are untouched, and per explicit user direction this app is out of scope for correctness, only "must still compile" — which it already didn't, for unrelated reasons, before this session.

## Verification

- Go: `go build ./...`, `go vet` clean (excluding one pre-existing unrelated `internal/structure/gitnexus.go` vet note), full suite 993 passed, same 15 pre-existing/unrelated failures as every prior check this session.
- Desktop: `tsc --noEmit` clean; `store.test.ts` 58/59 (the 1 failure is the same pre-existing, unrelated `localStorage` gap noted in CA-156).
- New/updated Go tests: `supabase_workflow_flow_store_test.go` (GetByRef by pack-identity vs. UUID, Upsert conflict-target selection, id-omission on brand-new rows), `flow_definition_resolver_test.go` (rewritten for the new `Upsert` signature and `CloneBuiltin` semantics), `flow_executor_test.go` (added hub wait-notice coverage).
- New TS tests: `tests/phase1/workflowFlowEngineAttrs.test.ts` (editable-guard rejection/allowance, clone semantics, new step attrs round-trip).

## Still open (unchanged from before this pivot)

- Downstream (post-entry-node) flow transitions are still driven by the existing cohort/`isCoderRun` machinery, not by walking `edges_json` — only flow *start* is behavior-ID/data-driven today.
- The `flow_engine_attrs` migration has not been applied to any live Supabase project (no database access this session).
- No live browser verification of `WorkflowsSettings.tsx`'s new UI was possible — the desktop app requires real Supabase sign-in with no demo/bypass mode, same limitation noted in CA-156.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-179
change_type: refactor
summary: migrate built-in flow mirroring and Settings authoring from a standalone flow_definitions table onto the existing workflows/workflow_steps schema and Settings UI
# --->8---
