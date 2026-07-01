# Task-175: Built-In Flow Mirror Sync And Resolver

## Metadata

- Document ID: `Task-175`
- Title: `Built-In Flow Mirror Sync And Resolver`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-177`, `Task-178`, `Task-179`
- Related Documents: `Task-173`, `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`, `CP-41-RAG-Harness-Flow-Mode`
- Replaces: `N/A`
- Tags: `agent-flow-engine, flow-definition, supabase, builtin-flow`

## AI Quick View

### Summary

- Add the definition-store mirror layer for built-in pack flows.
- Built-in YAML flows become read-only definition rows that UI and runner can reference like normal flow definitions.
- Runner resolves `flowRef` through one generic resolver instead of hardcoded template branches.

### Current Ask

- Implement idempotent sync from internal pack YAML to definition storage and add a resolver for `flowRef`.

### Key Decisions

- `T-1` Built-in flows are not edited directly. Users can clone them into editable custom flows.
- `T-2` Mirror rows must carry pack identity and hash/version to detect stale definitions.
- `T-3` Resolver must not know semantic flow names like Review Loop or RAG Harness beyond opaque IDs.

### Constraints

- Do not change UI in this task except optional backend metadata needed for Task-179.
- Do not execute mirrored flows yet unless called by existing tests/fakes.
- Preserve production rule: run data still goes to `sessions.ndjson`, not Supabase logs.

### Open Questions

- Exact table names/columns should match existing workflow definition schema.
- If no definition store is configured, runner should still be able to resolve built-ins from pack for local-only execution.

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
- `requirements/07-Coding-Plan/done/CP-36-DIAGRAM.md`
- `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`

## 1. Goal

Make built-in pack flows selectable and executable through the same definition path as user-created flows by mirroring them into the workflow definition store as read-only built-ins.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `Task-173`, `CP-36`, `CP-42`

## 3. Trigger

Flow Mode is already conceptually generic, but built-in flows need to appear in the same definition catalog as user flows. Without mirror sync, UI selection and runner resolution will drift.

## 4. Exact Change

- `T-1` Add `FlowDefinitionResolver` with API similar to:
  - `ResolveFlowRef(ctx, flowRef string) (FlowDefinition, error)`
  - `ResolveBuiltin(ctx, packID string, flowID string) (FlowDefinition, error)`
- `T-2` Define canonical `flowRef` format:
  - `pack:<packId>/<flowId>` or agreed equivalent.
  - Must support current planned form `flowpilot-core-flow-pack/review-loop`.
- `T-3` Add built-in mirror sync service:
  - reads parsed pack flows
  - computes content hash
  - checks definition store for matching `pack_id`, `pack_flow_id`, and hash
  - inserts or updates read-only mirror rows when missing/stale
- `T-4` Add mirror metadata to definition rows:
  - `source=builtin`
  - `editable=false`
  - `pack_id`
  - `pack_version`
  - `pack_flow_id`
  - `pack_hash`
  - `cloneable`
  - `selectable_in`
  - `chat_baseline`
- `T-5` Add startup/task hook that runs mirror sync before built-in flows are listed.
- `T-6` Add fake store tests for:
  - first sync inserts rows
  - second sync is no-op
  - hash change updates mirror row
  - user/custom flow rows are never overwritten
  - read-only built-in edit is rejected
- `T-7` Add local-only fallback resolution from embedded pack when definition store is unavailable.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/**/*flow*definition*.go`
  - `apps/local-runner/internal/agentpack/**/*.go`
  - existing definition store/gateway files
- modules:
  - local runner
  - definition store client
  - internal agentpack
- routes:
  - optional built-in flow listing endpoint if no current endpoint exists
- tables:
  - existing workflow definition tables only
  - no run-log tables

## 6. Acceptance Check

- Built-in `review-loop` and `rag-harness` mirror into definition store as read-only rows.
- Re-running sync is idempotent.
- Editing a mirrored built-in is rejected.
- Cloning a built-in creates an editable user-owned copy.
- Resolver returns identical normalized shape for built-in mirrored flow and custom flow.
- No run data is written to Supabase.

## 7. Out of Scope

- UI rendering of built-in list.
- Chat Mode picker.
- Removing semantic step-name hardcodes.

## 8. Completion Notes

- result: implemented
- notes: **superseded design, see [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md) for the current state.** CA-150/154/157 built a standalone `flow_definitions` table; per explicit user direction this was replaced with mirroring built-ins directly into the existing `workflows`/`workflow_steps` tables (extended with new flow-engine columns) so the existing Settings "Workflows/Steps" screen can be the authoring UI instead of a parallel one. `FlowDefinitionResolver`/`FlowMirrorSyncService`/`FlowDefinitionStore` now run against `SupabaseWorkflowFlowStore`, tested against an in-memory fake and the real Supabase-shaped store (mocked transport). `FlowDefinitionStoreFor` returns `nil` (not a file fallback) when Supabase isn't configured, matching the Settings UI's own hard Supabase requirement — `FlowDefinitionResolver` still falls back to the embedded pack directly in that case. `cmd/flowpilot runner serve` calls `EnsureBuiltinFlowMirrorsWithStore` at startup, non-fatally.
- follow-ups: the `add_flow_engine_attrs_to_workflows` migration needs a real review/apply pass against a live Supabase project (no database access this session).
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes and [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md)
