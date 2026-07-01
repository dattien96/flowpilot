# CA-150: Flow Definition Resolver And Mirror Sync

## Scope

Implement `Task-175`: a generic `FlowDefinitionResolver` and `FlowMirrorSyncService` so built-in pack flows, mirrored definition rows, and user-owned flows all resolve to the same `FlowDefinitionRecord` shape.

## Completed

- Added `FlowDefinitionStore` interface (`GetByPackFlow`, `GetByRef`, `Upsert`) so the resolver and mirror sync are backend-agnostic — no concrete Supabase table schema exists yet for flow definitions (unlike run-state tables), so this task lands the contract and an idempotent in-memory-testable implementation path rather than inventing an unreviewed production schema.
- Added `FlowDefinitionResolver.ResolveFlowRef`/`ResolveBuiltin`: prefers a store-backed row, falls back to the embedded pack (`agentpack.LoadBuiltinPack`) when the store has no row yet or `store == nil`, satisfying local-only execution.
- Added `FlowMirrorSyncService.SyncBuiltins`: hashes each built-in flow's raw YAML bytes (`agentpack.ReadBuiltinFlowRaw`, new export) and only upserts when the mirror row is missing or its hash/version has drifted.
- Added `CloneBuiltin` (creates an editable `supabase_user_definition` copy, rejects non-cloneable flows) and `UpdateUserFlow` (rejects writes to any record with `Editable=false`, returning `ErrDefinitionNotEditable`).
- The resolver treats `flowRef` as an opaque `packId/flowId` (or `user/<owner>/<flowId>`) pair; it never inspects semantic flow names.

## Verification

- `go test ./internal/runner -run 'TestFlow(Mirror|DefinitionResolver)|TestCloneBuiltin|TestUpdateUserFlow'`
- `go test ./internal/runner/... ./internal/agentpack/...` (full suite; no regressions beyond the pre-existing, unrelated Codex/Windows-path failures)

## Follow-ups

- A real Supabase-backed `FlowDefinitionStore` implementation, plus the actual mirror sync startup hook and table schema, is deferred — this CA lands the resolver/sync contract and its guarantees only. Wiring a concrete store and a startup hook is tracked under the same Task-175 umbrella but not attempted here without a reviewed schema.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-175
change_type: feature
summary: add flow definition resolver and idempotent built-in mirror sync service with pluggable store contract
# --->8---
