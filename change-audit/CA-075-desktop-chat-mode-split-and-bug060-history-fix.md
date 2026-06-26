# CA-075: Desktop Chat Mode Split And BUG-060 History Fix

## Scope

- Implemented Task-044 (Desktop Chat Mode Split And Provider Controls) and BUG-060 (Desktop Run History Empties After Switching Runs) end-to-end.
- Go runner: `StartRunInput`, `TurnInput`, `TurnRequest`, `interactiveRun`, `runHistoryItem`, `projectRunHistory`, `handleListSkills`, `interactiveCatalog.listSkills`, `fakeWorkflowStore`.
- Desktop TS: `contract.ts`, `store.ts`, `Navigator.tsx`, `ChatInput.tsx`, `RunStatus.tsx`, `HttpWsRunnerClient.ts`, `MockRunnerClient.ts`.

## Completed

### BUG-060 F-1 — Runner history rehydration from persisted store

- Added optional `SessionHistoryReader` interface (`workflow_store.go`) with `ListProviderSessionsByProject(ctx, projectId)`.
- Implemented `ListProviderSessionsByProject` on `fakeWorkflowStore` (reads from the in-memory sessions map that `persistProviderSession` populates — survives in-memory service recreation within the same process).
- Updated `projectRunHistory` (`interactive_handlers.go`) to type-assert `s.workflowStore` for `SessionHistoryReader` and merge persisted sessions not already in the in-memory `s.runs` map.
- Added repro test `TestRunHistoryEmptiesAfterServiceRecreation` (`bug060_test.go`) that starts two runs, recreates the service with the same store, and asserts history returns 2 items — test passes after F-1.

### BUG-060 F-3 — Desktop stale-response guard

- Added `_historyLoadSeq int` counter to store state.
- `loadRunHistory` increments `_historyLoadSeq` before the fetch and discards the response if the counter moved on (handles overlapping calls / project switches).

### BUG-060 F-4 — Desktop error state vs empty state

- Added `historyLoadError?: string` to store state; `loadRunHistory` sets it on error (no longer injects into the chat timeline).
- `RunStatus.tsx` now renders `"Failed to load history"` (with `run-history-error` class) when `historyLoadError` is set, distinct from the empty-state message.

### Task-044 T-1 / T-2 — Chat mode state and navigator split

- Added `chatMode: "normal_chat" | "workflow_step_auto"` to store (default `workflow_step_auto`) with `setChatMode` action that also resets the current run.
- `Navigator.tsx` rebuilt with a top-level Chat/Workflow mode tab; Chat panel shows required provider selector, model dropdown (from `supportedModels`), reasoning effort dropdown, and YOLO toggle; Workflow panel keeps existing run type + workflow/step selectors.

### Task-044 T-3 — Model selection

- Added `selectedModel?: string`, `setSelectedModel` action.
- `loadProjects` now loads `supportedModels` via `admin.providers.listSupportedModels()` alongside workflows/steps.
- Navigator model dropdown filters `supportedModels` by selected provider (falls back to text input when list is empty / Supabase not configured).

### Task-044 T-4 — Reasoning effort

- Added `reasoningEffort?: string`, `setReasoningEffort` action.
- Navigator shows a dropdown (`Default | Low | Medium | High`).
- `StartRunInput.reasoningEffort` and `TurnInput.reasoningEffort` added to `contract.ts`; wired through `HttpWsRunnerClient.sendTurn` body and `MockRunnerClient`.
- Go `StartRunInput` already had `ReasoningEffort`; `TurnRequest.ReasoningEffort` added to `provider_registry.go`; run-level and turn-level override wired in `interactive_service.go`.

### Task-044 T-5 — YOLO mode

- Added `yoloMode: boolean`, `setYoloMode` action; wired into `StartRunInput` for normal_chat.

### Task-044 T-6 — Provider-reactive skills

- `listSkills` in `contract.ts`, `HttpWsRunnerClient`, `MockRunnerClient` updated to accept optional `cwd` param.
- `store.loadSkills(provider, cwd?)` action added; `selectProvider` calls `loadSkills` on change.
- `loadProjects` uses `selectedProvider ?? "codex"` instead of hardcoded `"codex"`.
- Go `handleListSkills` already accepts `?provider=&cwd=`; `interactiveCatalog.listSkills` signature updated to accept `(provider, cwd string)` (fake returns static set; real adapters resolve dynamically).

### Task-044 T-7 — Normal-chat run lifecycle

- `StartRunInput.stepId` changed to optional (`stepId?: string`) in `contract.ts` and Go `provider_event.go`.
- `StartRunInput.chatMode?: string` added to `contract.ts`.
- Go runner: when `ChatMode == "normal_chat"`, `createRun` mints a synthetic `chat-<runId>` step, sets `runKind = "chat"` on `interactiveRun`, and sets `RunKind` on `runHistoryItem`.
- `MockRunnerClient.startRun` returns `stepId = "chat-{runId}"` when `chatMode === "normal_chat"`.
- `sendPrompt` in store routes to the chat-mode start path (no workflow/step required; provider required) and uses the synthetic stepId returned by the server for subsequent turns.

### Task-044 T-7 — Multi-turn + resume step-id reuse (review fix)

- Defect found in review: `sendPrompt` only learned the synthetic `chat-<runId>` step from `startRun` on turn 1, then fell back to `launchTargetId` (empty in chat) on later turns, so follow-up chat turns sent `stepId: ""` and `startTurn` rejected them with `400 stepId is required`. Resuming a chat run from history hit the same path (no step id surfaced).
- Fix (desktop): added `activeStepId?: string` to store state; set from the resolved turn step after `startRun` and from `resumeRun`'s handle in `openHistoryRun`; cleared in `resetRun` (and via `setChatMode`→`resetRun`). `sendPrompt` reuses `activeStepId` as the chat-mode step fallback.
- Fix (runner): `resumeRun` now returns `StepID = "chat-<runId>"` for `runKind == "chat"` runs so a resumed chat can continue; workflow/step resume is unchanged.

### Task-044 ChatInput update

- `canSend` in `normal_chat` requires `selectedProvider` (not workflow/step).
- Slash skill picker shown only in `normal_chat`; hidden in `workflow_step_auto` (step defaultSkill handles it).
- Placeholder text updated per mode.

## Verification

- `go build ./internal/runner/` — clean.
- `go test ./internal/runner/ -run "TestPhase|TestWorkflow|TestRun|TestBug|TestTurn|TestApproval|TestQuestion|TestInteractive" -timeout 60s` — all PASS including `TestRunHistoryEmptiesAfterServiceRecreation`.
- `npx tsc --noEmit` in `apps/desktop-flowpilot` — clean (zero errors).

## Residual Notes

- BUG-060 F-2 (Supabase `SupabaseWorkflowStore` implementing `SessionHistoryReader`) and F-5 (confirm `workflow_provider_sessions` migration) remain open — the fake store's `fakeWorkflowStore` already implements it, so dev/demo mode is fixed; production Supabase path is a follow-up.
- Task-044 T-8 (desktop unit tests for mode switching, ChatInput canSend) and Go HTTP tests for normal-chat start validation are not included in this pass.
- Claude-specific reasoning values (`Max`, `Extra`, `Ultracode`) are out of scope pending canonical CLI-flag confirmation; the base four (`low|medium|high|xhigh`) are wired.
- GitNexus tools were not exposed in this session; impact analysis was done by local code inspection.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-060
change_type: fix
summary: Desktop Chat Mode Split And BUG-060 History Fix
# --->8---
