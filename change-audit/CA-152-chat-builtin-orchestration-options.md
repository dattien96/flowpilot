# CA-152: Chat Built-In Orchestration Options

## Scope

Implement the backend-computable slice of `Task-177`: a pure function that computes which built-in flows Chat Mode should offer as optional orchestration for a given sub-mode, driven entirely by pack metadata.

## Completed

- Added `BuiltinOrchestrationOptions(subMode string) ([]BuiltinFlowOption, error)` in `chat_builtin_orchestration.go`. It reads `internal/agentpack`'s embedded pack and returns flows where `builtin.selectableIn` contains `chat`, `builtin.chatBaseline != true`, and `builtin.chatSubModes` contains the requested sub-mode.
- Verified via table tests: `bug` sub-mode returns exactly `review-loop`; `normal`/`task`/empty sub-modes return an empty slice; `rag-harness` (the chat baseline) never appears regardless of sub-mode.

## Explicitly deferred (not attempted here)

This codebase currently has **no `subMode` concept** anywhere in the runner or desktop app (`apps/desktop-flowpilot`) — grepped both trees and found zero matches outside `.git/objects`. CP-42/Task-177 assume a Chat sub-mode selector (`normal`/`bug`/`task`) already exists; it does not. Introducing it would mean:

- adding new request/response contract fields to the chat start/turn API (`turnBody` and friends in `interactive_handlers.go`),
- adding a new UI control in `apps/desktop-flowpilot` (an Electron/React app not explored in this session),
- deciding how `flowRef` selection flows from that UI back through the request contract into `FlowDefinitionResolver`.

None of that is a mechanical extension of existing code — it is a new product surface. Rather than invent an unreviewed contract and UI, this CA lands the one piece that's safely derivable from existing pack data and is a hard dependency for whatever contract/UI work comes next. The wiring from `BuiltinOrchestrationOptions` into an actual request field, and the desktop UI picker itself, remain open.

## Verification

- `go test ./internal/runner -run TestBuiltinOrchestration`
- `go test ./internal/runner/... ./internal/agentpack/...` (full suite; no new failures beyond the pre-existing environment-specific set, plus one confirmed-flaky, pre-existing, unrelated test — `TestProjectRunHistoryFiltersRunsByProject` — that fails intermittently under `-count=3` on `main` state with no code path touched by this change)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-177
change_type: feature
summary: add pack-metadata-driven builtin orchestration option computation for a future Chat Mode sub-mode picker
# --->8---
