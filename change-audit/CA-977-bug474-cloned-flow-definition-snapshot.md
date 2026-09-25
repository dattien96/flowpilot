# CA-977: BUG-474 — cloned harness flows restore execution fields via definition snapshot

## Change

`supabase_workflow_flow_store.go` now persists and restores a full
`FlowDefinition` snapshot for user-owned/cloned flow rows so a save→reload
round-trip is execution-equivalent to the source.

## Root cause

`step_definitions`/`workflows` have no columns for node `run`, `posture`,
`context_profile`, free-form `config`, or flow-level `contextProfiles`/`tools`.
Built-in mirrors recover them from the embedded pack
(`embeddedFlowDefinition`/`embeddedFlowNodesByID`), but cloned/user rows had
no authority — every clone silently dropped `read_only` posture, context
profiles, tournament `config`, etc. CP-58's `cp-harness-smoke` only launches
through Desktop clone, so its only supported path crossed the losing
round-trip.

## Fix

- Migration `20260922000000_add_workflow_definition_json.sql`: adds
  `workflows.definition_json jsonb`.
- `dbWorkflowRow.DefinitionJSON` decodes the column (`select=*` picks it up
  automatically once the migration lands).
- `Upsert` writes `definition_json` for non-builtin rows and explicitly NULLs
  it on built-in mirror upserts so the embedded pack stays the sole builtin
  authority.
- `recordFromWorkflowRow` restores the schema-less fields from the snapshot
  for non-builtin rows only, with identical fill-only precedence as the
  embedded path: real columns/joins win, the snapshot fills only what the
  schema cannot store.
- Shared helper `restoreFlowNodeExecutionFields` now serves both the
  embedded-pack path and the snapshot path (no behavior change for
  built-ins); `flowNodesByID` keys snapshot nodes by id.
- Rows saved before this fix keep the deterministic legacy reconstruction
  (run derived from behavior) — degraded but never a silent field swap;
  re-saving the flow writes the snapshot and self-heals.

## Evidence

- RED: `bug474_cloned_flow_execution_fields_test.go` —
  `TestBUG474_ClonedFlowRestoresExecutionFieldsFromSnapshot` and
  `TestBUG474_UpsertClonedFlowWritesDefinitionSnapshot` failed before the fix
  (posture dropped, no `definition_json` in the upsert payload).
- GREEN: all 5 BUG-474 tests pass, including column-precedence and
  built-in-ignores-snapshot guards.

## Deployment ordering

The migration must land before/with this binary: the upsert writes the new
column unconditionally (same exposure pattern as `acceptance_nodes_json`).
Reads are safe either way (`select=*`).

## Risk

- Low for built-ins: snapshot column forced NULL; embedded-pack precedence
  unchanged and covered by `TestBUG474_BuiltinRowIgnoresDefinitionSnapshot`.
- Snapshot stores Go-marshalled `FlowDefinition`; restore tolerates
  undecodable/absent snapshots by staying on the legacy derive-run path.
