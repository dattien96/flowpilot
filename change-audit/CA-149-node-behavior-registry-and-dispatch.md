# CA-149: Node Behavior Registry And Dispatch

## Scope

Add the CP-42 generic node behavior registry (`Task-176`): a typed dispatch layer keyed by behavior ID instead of semantic step names, with handlers for every core behavior ID.

## Completed

- Added `BehaviorID`, `BehaviorScope`, `BehaviorInput`, `BehaviorOutput`, `BehaviorHandler`, `BehaviorSpec`, and `BehaviorRegistry` in `behavior_registry.go`.
- `BehaviorRegistry.Resolve`/`Dispatch` normalize the requested ID through `agentpack.NormalizeBehaviorID` before lookup, so unknown or unregistered behavior IDs fail before any handler runs.
- Added `NewDefaultBehaviorRegistry` in `behavior_registry_builtin.go` registering the nine core behaviors: `agent.delegate`, `hub.inline`, `context.produce`, `context.render`, `command.validate`, `validation.summarize`, `artifact.audit_draft`, `flow.control`, `user.confirm`.
- `context.produce`/`context.render` wrap the existing `BuildFlowContextPackage`/`ComposeFlowCodingPrompt` helpers so those behaviors are selectable by ID without duplicating logic.
- `flow.control` wraps the existing `parseFlowControlInput` contract.
- This task is additive only: no existing call site (`interactive_service.go` role checks, `ReviewLoopFlowConfig`) was changed. Legacy step-name/role branches remain until Task-180.

## Verification

- `go test ./internal/runner -run 'TestBehavior|TestDefaultRegistry'`
- `go test ./internal/runner/... ./internal/agentpack/...` (full suite; no new failures — the 15 pre-existing failures are unrelated Codex/Windows-path/provider-account environment tests)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-176
change_type: feature
summary: add node behavior registry and dispatch layer keyed by behavior ID for CP-42
# --->8---
