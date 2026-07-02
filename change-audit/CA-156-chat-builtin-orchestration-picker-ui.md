# CA-156: Chat Built-In Orchestration Picker UI

## Scope

Land the desktop UI half of `Task-177`: a "Built-in orchestration" picker in Chat Mode's existing Bug intent panel, backed by the pack-driven `BuiltinOrchestrationOptions` (CA-152) through a new read-only backend endpoint.

## Discovery correction

Earlier session notes (CA-152, Task-177/179, CP-42 progress notes) claimed no `subMode`/chat-intent concept exists anywhere in `apps/desktop-flowpilot`. That was **wrong** — a grep for the literal string `"bug"` missed it because the desktop app's existing chat-intent selector uses the value `"bugfix"`, not `"bug"`. A proper investigation (two independent research passes) found:

- `ChatStartMode = "normal" | "task" | "bugfix"` (`src/state/store.ts:56`), a Zustand field `chatStartMode` with setter `setChatStartMode`, already feeding `TurnInput.changeType` on the first turn of a run.
- A working 3-tab UI, `ChatStartIntentPanel` in `src/components/ChatWorkspace.tsx:163-219` ("Normal"/"Task"/"Bug"), disabled while a turn is running.

This is functionally the exact concept CP-42/Task-177 assumed existed. The work below builds directly on it instead of inventing new UI.

## Completed

**Backend:**
- Added `GET /client/chat/builtin-orchestration-options?subMode=` (`handleListBuiltinOrchestrationOptions` in `interactive_handlers.go`), a pure read-only endpoint wrapping `BuiltinOrchestrationOptions` (CA-152). Added JSON tags to `BuiltinFlowOption`.
- 3 new handler tests (`chat_builtin_orchestration_handler_test.go`).

**Frontend (`apps/desktop-flowpilot`):**
- `TurnInput.subMode`/`.flowRef` and a new `BuiltinFlowOption` type in `types/contract.ts`.
- `RunnerClient.listBuiltinOrchestrationOptions?(subMode)` (optional method, same pattern as `listAgents?`), implemented in `HttpWsRunnerClient` (GET to the new endpoint) and `MockRunnerClient` (static Review Loop option for `subMode="bug"`, empty otherwise).
- `HttpWsRunnerClient.sendTurn` now forwards `input.subMode`/`input.flowRef` in the POST body.
- Store (`state/store.ts`): new `flowRef`/`builtinOrchestrationOptions` fields, `setFlowRef`, `loadBuiltinOrchestrationOptions(subMode)`. `setChatStartMode` now clears `flowRef` and the stale option list on every mode change and fires `loadBuiltinOrchestrationOptions("bug")` when entering `"bugfix"` — satisfying Task-177 T-7 ("switching away from Bug clears the Review Loop selection"). `resetRun` also clears both fields.
- `sendPrompt`'s `TurnInput` construction sends `subMode`/`flowRef` only on the first turn of a `"bugfix"`-intent run with a flow actually selected, mapping the UI's `"bugfix"` value to the runner's `"bug"` subMode key (they're spelled differently by design — the UI tab predates this feature and its value wasn't renamed to avoid disturbing existing `changeType` behavior).
- `ChatStartIntentPanel` (`ChatWorkspace.tsx`) renders a "Built-in orchestration" `<select>` (None / fetched options) directly below the existing Bug-ID input, only when `chatStartMode === "bugfix"` and options exist — reusing the existing `.nav-group`/`.chat-start-mode-hint` styling with no new CSS needed.

## Verification

- `npx tsc --noEmit` (desktop-flowpilot) — clean.
- `npx tsx --test src/state/store.test.ts` — 59 tests, 58 passed (2 new: `setChatStartMode clears flowRef...`, `setChatStartMode loads builtin orchestration options...`). The 1 failure (`selectProject resets the active chat run when switching projects`, `ReferenceError: localStorage is not defined`) is pre-existing and unrelated — that test calls `selectProject`, which touches `localStorage` directly; this session's ad-hoc `npx tsx --test` invocation (the project's actual CI test-runner invocation wasn't identified) doesn't provide a `localStorage` global. Confirmed unrelated to any file this change touches.
- Backend: `go test ./internal/runner -run 'TestHandleListBuiltinOrchestrationOptions'` and full suite — 975 passed (up from 972), identical pre-existing 15-failure set.

## Still not wired (unchanged from CA-154)

Sending a valid `flowRef` is now possible end-to-end from the UI through validation, but it still has **no execution effect** — `startTurn`'s live cohort/hub-reinvoke loop doesn't consume it yet. That remains gated on the `FlowEdge`-driven generic executor discussed in CA-155.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-177
change_type: feature
summary: add Chat Mode built-in orchestration picker UI wired to the existing Bug intent panel via a new read-only options endpoint
# --->8---
