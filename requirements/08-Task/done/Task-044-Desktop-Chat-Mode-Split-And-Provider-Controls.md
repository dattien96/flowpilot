# Task-044: Desktop Chat Mode Split And Provider Controls

## Metadata

- Document ID: `Task-044`
- Title: `Desktop Chat Mode Split And Provider Controls`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [05: Codex App-Server Migration Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md), [07: Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [Task-010: Yolo Mode](../todo/Task-010-Yolo-Mode.md), [Task-013: Dynamic Way To Add Support Model](./Task-013-Dynamic-Way-To-Add-Support-Model.md), [Task-032: Document Proxy Yolo And Provider Approval Config](../todo/Task-032-Document-Proxy-Yolo-And-Provider-Approval-Config.md), [CA-020: Workflow UI Chat Feed and Continue Flow](../../change-audit/CA-020-workflow-ui-chat-feed-and-continue-flow.md), [CA-021: Workflow Chat Runtime and Font Tuning](../../change-audit/CA-021-workflow-chat-runtime-and-font-tuning.md), [CA-036: Update Supported Models Constraint](../../change-audit/CA-036-update-supported-models-constraint.md), [BUG-060: Desktop Run History Empties After Switching Runs](../../09-BugFix/done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [CA-075: Desktop Chat Mode Split And BUG-060 History Fix](../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md)
- Replaces: `none`
- Tags: `desktop, chat, provider-controls, codex, claude, workflow-run, skills, yolo`

## AI Quick View

### Summary

- Split the desktop chat dashboard into two explicit modes: `normal_chat` (direct provider chat) and `workflow_step_auto` (existing workflow/step automation).
- Normal chat exposes provider, model, reasoning effort, YOLO, and slash skill controls; workflow/step mode keeps the existing forced workflow-or-step selection and hides provider-tuning controls.
- Both Go runner and desktop TypeScript layers are fully implemented. BUG-060 (run history rehydration) landed in the same pass.

### Current Ask

- Done. All T-1 through T-7 items implemented. T-8 (tests) partially done (Go runner tests pass; desktop component tests not yet written).

### Key Decisions

- `KD-1` `chatMode: "normal_chat" | "workflow_step_auto"` added to store state.
- `KD-2` In `normal_chat`, only project + provider required; no workflow/step.
- `KD-3` In `workflow_step_auto`, workflow or step required exactly as before.
- `KD-4` Model options filtered from `supportedModels` by provider.
- `KD-6` YOLO wired through `StartRunInput.yoloMode`.
- `KD-7` Slash skills loaded from `listSkills(provider, cwd?)` and reloaded on provider change.

### Constraints

- Base reasoning values (`low|medium|high`) are wired. Claude-specific values (`Max`, `Extra`, `Ultracode`) deferred pending canonical CLI-flag confirmation.
- BUG-060 F-2/F-5 (Supabase `SessionHistoryReader`) remain pending for full production durability of normal-chat logs.

### Open Questions

- Canonical stored values for Claude Sonnet `Max`, Claude Opus `Extra`, and `Ultracode` reasoning modes — deferred.

### Source Refs

- Changed files: `apps/local-runner/internal/runner/{provider_event.go,interactive_service.go,interactive_handlers.go,interactive_catalog.go,provider_registry.go,workflow_store.go,bug060_test.go}`.
- Changed files: `apps/desktop-flowpilot/src/{types/contract.ts,state/store.ts,components/Navigator.tsx,components/ChatInput.tsx,components/RunStatus.tsx,client/HttpWsRunnerClient.ts,client/MockRunnerClient.ts}`.

## 1. Goal

Add a clear mode split to the desktop chat dashboard so users can either use a normal provider chat without workflow/step selection, or run automated work through an existing workflow or single step.

## 2. Parent Links

- coding plan: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [03: Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Trigger

Codex app-server and Claude controlled-mode adapter work is now baked into the refactor system. The desktop chat dashboard still behaved like a workflow launcher. Normal-chat mode was needed for direct provider interaction without a workflow/step configuration.

## 4. Exact Change

- `T-1` ✓ `chatMode: "normal_chat" | "workflow_step_auto"` state; `setChatMode` action (resets run on switch).
- `T-2` ✓ Navigator rebuilt with mode tab; Chat panel hides workflow/step selectors; Workflow panel keeps existing selectors.
- `T-3` ✓ `selectedModel`, `setSelectedModel`; `supportedModels` loaded via `admin.providers.listSupportedModels()`; model dropdown filtered by provider.
- `T-4` ✓ `reasoningEffort`, `setReasoningEffort`; wired through `StartRunInput`/`TurnInput`/`TurnRequest`; Go runner pass-through confirmed.
- `T-5` ✓ `yoloMode`, `setYoloMode`; YOLO toggle in Chat mode; wired through `StartRunInput`.
- `T-6` ✓ `listSkills(provider, cwd?)` contract updated; `loadSkills` action; `selectProvider` reloads skills; slash picker hidden in `workflow_step_auto`.
- `T-7` ✓ `StartRunInput.stepId` optional; `chatMode?: string` added; runner mints synthetic `chat-<runId>` step + sets `runKind = "chat"`; `sendPrompt` routes to chat-mode start when `chatMode === "normal_chat"`. Multi-turn + resume reuse the synthetic step via `activeStepId` (store) and `resumeRun` returning the chat step — review fix, see CA-075.
- `T-8` ⏳ Desktop component tests for mode switching not yet written; Go runner HTTP tests for normal-chat start not yet written.

## 5. Touched Areas

- `apps/local-runner/internal/runner/provider_event.go` — `StartRunInput.stepId` optional, `chatMode`, `reasoningEffort`; `TurnInput.reasoningEffort`.
- `apps/local-runner/internal/runner/provider_registry.go` — `TurnRequest.ReasoningEffort`.
- `apps/local-runner/internal/runner/interactive_service.go` — `interactiveRun.reasoningEffort`, `runKind`; `runTurn` reasoning pass-through.
- `apps/local-runner/internal/runner/interactive_handlers.go` — `runHistoryItem.RunKind`; `createRun` chat-mode branch; `projectRunHistory` `SessionHistoryReader` merge.
- `apps/local-runner/internal/runner/interactive_catalog.go` — `listSkills(provider, cwd string)` signature.
- `apps/local-runner/internal/runner/workflow_store.go` — `SessionHistoryReader` interface; `fakeWorkflowStore.ListProviderSessionsByProject`.
- `apps/local-runner/internal/runner/bug060_test.go` — `TestRunHistoryEmptiesAfterServiceRecreation`.
- `apps/desktop-flowpilot/src/types/contract.ts` — `StartRunInput.stepId?`, `chatMode?`, `reasoningEffort?`; `TurnInput.reasoningEffort?`; `RunHistoryItem.runKind?`; `listSkills(provider, cwd?)`.
- `apps/desktop-flowpilot/src/state/store.ts` — new state fields; new actions; updated `loadProjects`, `sendPrompt`, `selectProvider`, `loadRunHistory`.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` — full rewrite with mode tabs.
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — mode-aware `canSend` and skill picker visibility.
- `apps/desktop-flowpilot/src/components/RunStatus.tsx` — `historyLoadError` display.
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts` — `listSkills` cwd; `sendTurn` `reasoningEffort`.
- `apps/desktop-flowpilot/src/client/MockRunnerClient.ts` — `listSkills` cwd; `startRun` returns `stepId` for chat mode.

## 6. Acceptance Check

- ✓ Mode tab shows `Chat` / `Workflow`; switching hides/shows respective controls.
- ✓ In Chat mode: workflow and step selectors hidden; provider required to send; model and reasoning dropdowns present; YOLO toggle present; slash picker active.
- ✓ In Workflow mode: existing behavior preserved; provider optional (Auto); slash picker hidden.
- ✓ Changing provider reloads skills.
- ✓ `StartRunInput` for `normal_chat` omits `stepId`/`workflowId`; runner mints synthetic step.
- ✓ Multi-turn chat and resume-from-history continue to send the synthetic `chat-<runId>` step (no empty-stepId 400).
- ✓ TypeScript build clean (`npx tsc --noEmit`).
- ✓ Go runner tests pass including `TestRunHistoryEmptiesAfterServiceRecreation`.
- ⏳ Desktop component tests (T-8) not yet written.
- ⏳ Production Supabase durability (BUG-060 F-2/F-5) pending.

## 7. Out of Scope

- Gemini controlled-mode normal chat (adapter still placeholder).
- Desktop component tests for mode switching (T-8 — follow-up).
- Claude-specific reasoning values (`Max`, `Extra`, `Ultracode`).
- Supabase `SessionHistoryReader` implementation (BUG-060 F-2).

## 8. Completion Notes

- result: Implemented. Go runner + desktop TypeScript complete. BUG-060 F-1/F-3/F-4 in same pass.
- follow-ups:
  - Write desktop component tests for mode switching, `canSend` logic (T-8).
  - Implement `SupabaseWorkflowStore.ListProviderSessionsByProject` for production history durability (BUG-060 F-2).
  - Confirm and wire Claude `Max`/`Extra`/`Ultracode` reasoning values when canonical CLI values are known.
  - Update `SS-05` if provider-specific reasoning values are added beyond `low|medium|high`.
- resolved decisions: normal-chat history reuses existing tables via synthetic `chat` step + `run_kind` discriminator; provider skills deduped by name with `workspace > flowpilot > provider` precedence (fake catalog returns static set; real resolvers TBD per adapter).
- upstream docs updated: none required this pass (base four reasoning values need no doc change; chat mode is an additive flag).
