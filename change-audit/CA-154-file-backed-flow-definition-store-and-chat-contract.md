# CA-154: File-Backed Flow Definition Store And Chat Contract Fields

## Scope

Advance `Task-175` and `Task-177` past their previous "contract only" state: land a concrete, working `FlowDefinitionStore` implementation, and add the `subMode`/`flowRef` request contract fields with validation.

## Completed

- Added `FileFlowDefinitionStore` (`flow_definition_store_file.go`): a local, JSON-file-backed implementation of the `FlowDefinitionStore` interface (Task-175/CA-150), rooted at `<workspace>/.flowpilot/flow-definitions/`, following the existing `.flowpilot/` convention used by `flow_context_package.go`'s feature catalog lookup. This gives `FlowDefinitionResolver` and `FlowMirrorSyncService` a real backing store to run against without waiting on an unreviewed Supabase schema decision (`Q-1` is still open; a Supabase-backed store can implement the same interface later without touching resolver/sync code).
- Added `EnsureBuiltinFlowMirrors(ctx, workspace)` as a callable, tested convenience wrapper (constructs the file store + sync service + runs `SyncBuiltins`). **Not** wired into `cmd/flowpilot`'s startup sequence — this session didn't have enough visibility into that CLI/server lifecycle to insert a call there with confidence it can't block or fail startup for existing users. It's exposed and tested so wiring it in later is a one-line addition once that lifecycle is reviewed.
- Added `subMode`/`flowRef` fields to `turnBody` (`interactive_handlers.go`) and a `validateChatOrchestrationSelection(subMode, flowRef)` function (`chat_builtin_orchestration.go`) that rejects a `flowRef` not present in `BuiltinOrchestrationOptions(subMode)` — including explicitly rejecting the `rag-harness` chat-baseline flow, which must never be selectable through this contract. `handleStartTurn` calls this validation before starting the turn and returns `400 invalid_flow_ref` on rejection.
- **This is contract validation only, not execution wiring.** A valid `flowRef` is accepted and currently has no effect beyond passing validation — `startTurn`'s internal cohort/hub-reinvoke logic does not yet resolve or execute the selected flow. Wiring that up means touching the same live orchestration branches (`isAgentRole(rs, "coder")`, etc.) flagged as out of safe scope in CA-153 without GitNexus impact analysis.

## Verification

- `go test ./internal/runner -run 'TestFileFlowDefinitionStore|TestEnsureBuiltinFlowMirrors|TestValidateChatOrchestration|TestBuiltinOrchestration'`
- `go test ./internal/runner/... ./internal/agentpack/...` — 966 passed (up from 957 before this change), identical pre-existing failure set (15, unrelated Codex/Windows-path/provider-account environment tests)

## Follow-ups

- Decide where in `cmd/flowpilot`'s startup path to call `EnsureBuiltinFlowMirrors`.
- Wire an accepted `flowRef` selection through to actual flow execution — requires the `isAgentRole` migration flagged in CA-153/Task-180.
- Desktop UI control to send `subMode`/`flowRef` (still unbuilt — no `subMode` concept exists in `apps/desktop-flowpilot` yet, per CA-152).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-175
change_type: feature
summary: add file-backed FlowDefinitionStore implementation and subMode/flowRef chat request contract validation
# --->8---
