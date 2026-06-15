# Task-044: Desktop Chat Mode Split And Provider Controls

## Metadata

- Document ID: `Task-044`
- Title: `Desktop Chat Mode Split And Provider Controls`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [05: Codex App-Server Migration Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md), [07: Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [Task-010: Yolo Mode](./Task-010-Yolo-Mode.md), [Task-013: Dynamic Way To Add Support Model](../done/Task-013-Dynamic-Way-To-Add-Support-Model.md), [Task-032: Document Proxy Yolo And Provider Approval Config](./Task-032-Document-Proxy-Yolo-And-Provider-Approval-Config.md), [CA-020: Workflow UI Chat Feed and Continue Flow](../../change-audit/CA-020-workflow-ui-chat-feed-and-continue-flow.md), [CA-021: Workflow Chat Runtime and Font Tuning](../../change-audit/CA-021-workflow-chat-runtime-and-font-tuning.md), [CA-036: Update Supported Models Constraint](../../change-audit/CA-036-update-supported-models-constraint.md), [BUG-060: Desktop Run History Empties After Switching Runs](../../09-BugFix/todo/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md)
- Replaces: `none`
- Tags: `desktop, chat, provider-controls, codex, claude, workflow-run, skills, yolo`

## AI Quick View

### Summary

- Split the desktop chat dashboard into two explicit modes: normal provider chat and workflow/step automation.
- Normal chat must not require workflow or step selection; it exposes provider, model, reasoning effort, YOLO, and slash skill controls.
- Workflow/step automation keeps the existing forced workflow-or-step selection and hides provider-tuning controls because workflow/step definitions own model, reasoning, YOLO, and skills.
- Slash skill suggestions must come from real local provider skill directories for the selected provider instead of the current fake/static Codex list.
- Most backend plumbing already exists (Go `StartRunInput.ReasoningEffort`, the `GET /client/provider-skills` route, `listSupportedModels()`); the bulk of this task is wiring the desktop layer through to those contracts and making the static skill endpoint provider/`cwd` reactive — not building new runner infrastructure.

### Current Ask

- Plan the implementation slice for desktop chat mode behavior after Codex and Claude are baked providers in the new refactor system.

### Key Decisions

- `KD-1` Add an explicit chat mode state with values `normal_chat` and `workflow_step_auto`.
- `KD-2` In `normal_chat`, require only a selected project/workspace and provider; do not require a workflow or step.
- `KD-3` In `workflow_step_auto`, require a workflow or step target exactly as the current flow does.
- `KD-4` Model options in `normal_chat` are filtered to enabled supported models for the selected provider.
- `KD-5` Reasoning effort options are provider/model aware, not a single global list.
- `KD-6` YOLO remains the approval posture switch for normal chat and must flow into the runner start/turn request.
- `KD-7` Slash skills are loaded from the selected provider's real local skill registry and refreshed when provider or project changes.

### Constraints

- Preserve workflow as the controlled automation concept from `SS-11`; normal chat is an additional launch mode, not a replacement for workflow execution.
- Do not show workflow and step selectors in normal chat mode.
- Do not show model, reasoning, YOLO, or slash skill controls in workflow/step automation mode; those are configured in workflow and step definitions.
- `reasoningEffort` already exists end-to-end on the Go side (`StartRunInput`, the turn request), in `SS-05`, the `2026-05-25` reasoning-effort migrations, and admin-web; this task only adds it to the desktop TS contract. Only provider-specific values BEYOND the existing `low | medium | high | xhigh` (Claude `Max`, Opus `Extra`, `Ultracode`) require an upstream contract update to `SS-05` and the shared runner/client DTOs before implementation is marked complete.
- The `normal_chat` mode must reconcile with the existing `selectedProvider` "direct chat" override and its `Auto (from model)` option; do not ship two overlapping chat concepts.
- DECIDED — normal-chat history reuses the existing workflow-run + provider-session + event tables via a synthetic `chat` step and a `run_kind` (`chat` | `workflow`) discriminator; no new `chat_sessions` table or route. Durability depends on the [BUG-060](../../09-BugFix/todo/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md) rehydration fix. See `T-7`.
- DECIDED — provider slash skills load ALL sources (provider home + workspace/project `.agents/skills` + `.codex` / `.claude/skills`), deduped by name with precedence `workspace` > `flowpilot` > `provider`, showing the winning source badge. See `T-6`.
- Gemini is out of scope for this task (normal chat is limited to Codex and Claude); revisit when the Gemini adapter is no longer placeholder.
- GitNexus MCP tools were not exposed in this planning thread; no application symbols were edited.

### Open Questions

- What are the canonical stored values for Claude Sonnet `Max`, Claude Opus `Extra`, and `Ultracode` reasoning modes, and how do they map to CLI flags?

### Source Refs

- `04-Detailed-Coding-Plan`: provider runtime contract, desktop app, YOLO SSOT, provider event stream, and skill selection.
- `04-01`: desktop navigator, chat input, slash skill picker, approval/question cards, and `RunnerClient` contract.
- `05`: Codex app-server thread model, YOLO SSOT, `cwd` per thread, skill selection, provider event mapping.
- `07`: Claude adapter parity, provider-specific skills, process/session model, YOLO permission mapping.
- Current code refs: `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/types/contract.ts`, `apps/local-runner/internal/runner/interactive_catalog.go`, `apps/local-runner/internal/runner/provider_event.go`.

## 1. Goal

Add a clear mode split to the desktop chat dashboard so users can either:

1. use a normal provider chat, similar to Codex or Claude chat, without selecting a workflow or step; or
2. run automated work through an existing workflow or single step, where the workflow/step configuration remains the source of truth.

The immediate outcome is a detailed implementation plan for the next coding pass. The future implementation must make the chat controller show only controls that are meaningful for the selected mode.

## 2. Parent Links

- coding plan: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [03: Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)
- specific upstream ids:
  - `04-Detailed-Coding-Plan` Shared Contract: `ProviderTurnInput.modelName`, `reasoningEffort`, `selectedSkill`, `yoloMode`
  - `04-01` Part A Scope: project/workflow/step selector, chat input, slash skill picker
  - `05` YOLO As SSOT and Work Items `W3`, `W5`, `W6`
  - `07` Final Decision and Capability Comparison for Claude skill selection, model, permission mode, and session behavior
  - `SS-11` sections 2 and 6 for workflow-run versus AI-session boundaries

## 3. Trigger

Codex app-server and Claude controlled-mode adapter work is now baked into the refactor system. The desktop chat dashboard still behaves like a workflow launcher: `ChatInput` blocks send unless `launchMode` has a selected workflow or selected step, while the navigator always renders workflow/step controls. The user now wants a separate normal chat mode that behaves like a provider chat while keeping the existing workflow/step automation mode intact.

Current observed code shape:

- `apps/desktop-flowpilot/src/state/store.ts` defines `launchMode: "workflow" | "step"` and `sendPrompt` returns early without `selectedProjectId` plus workflow/step target.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` renders workflow and step tabs plus a provider override selector.
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` requires `hasLaunchTarget` and always exposes slash skill picking from `skills`.
- `store.loadProjects()` calls `client.listSkills("codex")`, so skills are not provider-reactive. The contract method `listSkills(provider: string)` already takes a provider but is never re-called on provider/project change and has no `cwd` parameter.
- The desktop `StartRunInput` has `providerKey`, `model`, and `yoloMode`, but no `reasoningEffort` path in `apps/desktop-flowpilot/src/types/contract.ts`.

Already-existing infrastructure this task wires through (do NOT rebuild):

- `GET /client/provider-skills` route already exists (`interactive_handlers.go` → `handleListSkills`), but `interactive_catalog.go listSkills()` returns a static catalog and ignores `provider`/`cwd`.
- Go `StartRunInput.ReasoningEffort string` and the turn request `ReasoningEffort *string` already exist (`apps/local-runner/internal/runner/types.go`); the value already flows through `SS-05`, the `2026-05-25` reasoning-effort migrations, and admin-web's `WorkflowsSettings`.
- A supported-model source already exists: `getAdminUseCases().providers.listSupportedModels()` plus the `SupportedModel` type and `buildModelOptions` helper used by `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`.
- `ProviderSkill.source` is already typed `"provider" | "flowpilot" | "workspace"`, matching the badge set in `T-6`.
- The navigator already has a `selectedProvider` override (`Auto (from model)` + per-provider) with a "Choose one to chat directly" hint, but `sendPrompt` still hard-requires a workflow/step target — a partial direct-chat path that `normal_chat` must subsume.

## 4. Exact Change

- `T-1` Add a top-level chat mode in desktop state.
  - Suggested state: `chatMode: "normal_chat" | "workflow_step_auto"`.
  - Default can remain `workflow_step_auto` for compatibility, or switch to `normal_chat` only after product sign-off.
  - Reset mode-sensitive selections cleanly: switching to normal chat hides but does not delete current workflow/step selections; switching back restores or requires a valid selection.
  - Reconcile with the existing `selectedProvider` override: in `normal_chat`, provider is a required first-class control (no `Auto (from model)` — there is no workflow/step model to derive from). The current "Choose one to chat directly" override on the navigator collapses into `normal_chat`; `workflow_step_auto` keeps `Auto (from model)` as today. Do not leave both a mode switch and a separate direct-chat override that mean the same thing.

- `T-2` Update the navigator/dashboard controls by mode.
  - Normal chat mode shows project/workspace selection and provider selection.
  - Normal chat mode hides `Run type`, `Workflow`, and `Single step` selectors.
  - Workflow/step automation mode shows the existing run type, workflow selector, and step selector.
  - Workflow/step automation mode may keep provider display only as read-only derived information if useful, but should not expose provider/model/reasoning overrides in the chat controller.

- `T-3` Add normal chat provider controls.
  - Add model selection UI in the chat controller for normal chat.
  - Reuse the EXISTING supported-model source — `getAdminUseCases().providers.listSupportedModels()` with the `SupportedModel` type and `buildModelOptions` helper already used by `WorkflowsSettings.tsx`. Do NOT add a new `GET /client/supported-models` runner route.
  - Preferred: extract the model-option logic from `WorkflowsSettings.tsx` into a shared helper so desktop chat and settings stay in sync.
  - Filter model options by the current selected provider (`codex`, `claude`; Gemini out of scope for this task).
  - When provider changes, keep the selected model only if it belongs to that provider; otherwise select the provider's default enabled model.
  - Pass selected `model` into `StartRunInput`.

- `T-4` Add provider/model-aware reasoning control for normal chat.
  - The Go runner already carries reasoning end-to-end (`StartRunInput.ReasoningEffort`, the turn request `ReasoningEffort`); the ONLY contract change is adding `reasoningEffort` to the desktop TS `StartRunInput`/`TurnInput` so the existing Go path receives it. Do not "extend the Go `StartRunInput`" — it is already present.
  - Codex options: `Low`, `Medium`, `High`, `Extra High` mapped to the existing normalized wire values (`low`/`medium`/`high`/`xhigh`); these already exist, no upstream contract change.
  - Claude Sonnet options: `Low`, `Medium`, `High`, `Max` once the runner confirms CLI values.
  - Claude Opus options: include `Extra` and `Ultracode` only after the adapter contract defines supported stored values.
  - Persist/display labels separately from wire values so provider-specific labels (`Extra High` → `xhigh`, etc.) can change without breaking stored runs.
  - Only the new Claude-specific values (`Max`, `Extra`, `Ultracode`) — anything beyond `low | medium | high | xhigh` — require updating `SS-05` and the shared DTOs; the base four do not.

- `T-5` Add YOLO control for normal chat.
  - Show a clear enable/disable toggle only in normal chat mode.
  - Pass `yoloMode` into `StartRunInput`.
  - Preserve the `05` YOLO SSOT rule: the same value drives runner approval behavior and provider-specific posture.
  - Workflow/step automation mode hides this toggle because workflow/step definitions already configure YOLO.

- `T-6` Convert slash skills from fake/static to real provider-local skills.
  - Replace the current `client.listSkills("codex")` hard-code in `store.loadProjects` with provider-reactive loading, re-fetched when provider or project changes.
  - The `GET /client/provider-skills` route ALREADY EXISTS (`interactive_handlers.go` → `handleListSkills`) but `interactive_catalog.go listSkills()` returns a static catalog ignoring `provider`/`cwd`. The work is making that existing route resolve real skill directories by `provider` + `cwd` — not adding a new route.
  - Add a `cwd`/project parameter to the contract method `listSkills` (today it takes only `provider`) and to the route query (`?provider=<provider>&cwd=<projectPath>`).
  - Load ALL sources and merge (decision resolved):
    - `provider` source: Codex active `CODEX_HOME` skills; Claude active config skills.
    - `workspace` source: project/workspace `.agents/skills`, plus `.codex` (Codex) and `.claude/skills` (Claude) where supported by the adapter.
    - `flowpilot` source: FlowPilot-managed skills if/where the adapter exposes them.
  - Dedupe by skill `name` with precedence `workspace` > `flowpilot` > `provider` (most-specific wins). The surviving entry keeps the badge of its winning source; a duplicate `name` must NOT appear twice in the picker.
  - Keep source badges (`provider`, `flowpilot`, `workspace`) so users can tell where the winning skill came from (matches the existing `ProviderSkill.source` type).
  - Hide slash skill UI entirely in workflow/step automation mode because step definitions already declare required skills.

- `T-7` Update runner/client contracts for normal chat launch (strategy resolved: synthetic chat run reusing existing tables).
  - Normal chat creates a workflow-run + provider-session in the EXISTING run/session tables, with a runner-created synthetic `chat` step. Do NOT add a `chat_sessions` table or a dedicated chat route — reuse `POST /client/workflow-runs` + `POST /client/workflow-runs/{runId}/turns`.
  - Add a `run_kind` discriminator (`chat` | `workflow`) on the run/session record so:
    - the synthetic chat step never leaks into workflow catalogs (`listWorkflows`/`listSteps`/admin catalogs filter `run_kind = "workflow"`); and
    - history can filter chat vs workflow runs (default: show both, labeled; allow filtering to one kind).
  - Chat logs persist through the SAME path as workflow runs (`persistProviderSession` for the session, `persistEvent` for the message/turn timeline) — no new persistence code. This satisfies "save the chat log even in normal chat".
  - `StartRunInput`: when `chatMode = normal_chat`, the runner mints the synthetic step id and sets `run_kind = "chat"`; the client sends `providerKey` + `model` + `reasoningEffort` + `yoloMode` and omits `workflowId`/`stepId`. Keep `TurnInput.stepId` populated with the synthetic step id so the existing turn contract is unchanged.
  - **Hard prerequisite:** the run-history read path is in-memory-only and empties on runner/app-server recreation — see [BUG-060](../../09-BugFix/todo/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md). Chat-log durability rides on the SAME rehydration fix, so BUG-060 must land before (or with) this task; otherwise chat logs are lost on restart exactly like workflow runs are today.

- `T-8` Update tests.
  - Desktop unit/component tests cover mode switching, hidden selectors, normal-chat send enabled without workflow/step, and workflow/step send still blocked until target is selected.
  - Client contract tests cover `model`, `reasoningEffort`, and `yoloMode` payload shaping.
  - Runner HTTP tests cover normal-chat start validation and provider/model mismatch rejection.
  - Skill endpoint tests cover provider-specific skill listing and workspace skill source labels.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/mockData.ts`
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` or shared supported-model helper extraction if desktop chat reuses its model option logic
  - `apps/local-runner/internal/cli/root.go`
  - `apps/local-runner/internal/runner/provider_event.go`
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go` (existing `handleListSkills` for `GET /client/provider-skills`)
  - `apps/local-runner/internal/runner/interactive_catalog.go` (make `listSkills()` provider/`cwd` aware)
  - `apps/local-runner/internal/runner/types.go` (Go `StartRunInput.ReasoningEffort` already present — reuse, do not add)
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/provider_registry.go`
  - `apps/local-runner/internal/runner/codex_appserver_process.go`
  - `apps/local-runner/internal/runner/claude_adapter.go`
- modules:
  - desktop chat dashboard
  - desktop runner client contract
  - runner interactive API
  - provider model/reasoning resolution
  - provider-local skill discovery
  - normal-chat run/session persistence
- routes:
  - `GET /client/provider-skills` (exists; make provider/`cwd` reactive)
  - `POST /client/workflow-runs`
  - `POST /client/workflow-runs/{runId}/turns`
  - models: reuse the existing `getAdminUseCases().providers.listSupportedModels()` client-core path — no new runner route.
- tables:
  - `ai_supported_models` read path
  - existing workflow-run + `workflow_provider_sessions` + provider-event tables (reused; normal chat persists through the same path)
  - schema change: add a `run_kind` discriminator (`chat` | `workflow`) to the run/session record so chat runs are filterable and excluded from workflow catalogs. No new `chat_sessions` table.

## 6. Acceptance Check

- Opening the desktop chat dashboard shows an explicit mode control with `Normal chat` and `Workflow/step auto`.
- In `Normal chat` mode:
  - workflow and step selectors are not visible;
  - send is enabled with only a selected project/workspace, selected provider, valid provider model, and non-empty prompt;
  - model dropdown shows only enabled models for the selected provider;
  - changing provider updates model choices and reloads skills for that provider;
  - reasoning dropdown shows options valid for the selected provider/model;
  - YOLO toggle is visible and the chosen value reaches the runner;
  - typing `/` shows real local skills for the selected provider and selected project/workspace, not the current fake/static Codex list;
  - the skill list merges all sources (provider + workspace + flowpilot), deduped by name with `workspace` > `flowpilot` > `provider` precedence, each row showing its winning source badge, with no duplicate `/name`;
  - selected skills attach to the next normal-chat turn.
- In `Workflow/step auto` mode:
  - workflow or step selection remains required before send;
  - model, reasoning, YOLO, and slash skill controls are hidden from the chat controller;
  - workflow/step configured model, reasoning, YOLO, MCP, and skills remain the source of truth;
  - existing workflow/step run behavior does not regress.
- A normal chat run is persisted and reopenable from history: it appears in the project run history with its chat prompt/message, distinguished from workflow runs by `run_kind = "chat"`, and reopening replays its event timeline. The synthetic `chat` step does NOT appear in any workflow/step catalog.
- Chat-log durability survives a runner/app-server restart (gated on the [BUG-060](../../09-BugFix/todo/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md) rehydration fix landing before or with this task).
- TypeScript build passes for `apps/desktop-flowpilot`.
- Targeted Go tests pass for runner contract changes.
- The base reasoning values (`low | medium | high | xhigh`) already exist and need no doc change. If provider-specific reasoning values are added beyond that set (`Max`, `Extra`, `Ultracode`), `SS-05` and the shared contract docs are updated in the same implementation pass.

## 7. Out of Scope

- Implementing Gemini controlled-mode normal chat if the Gemini adapter is still placeholder.
- Replacing workflow execution with free-form chat.
- Removing workflow, step, artifact, approval, or finalizer behavior.
- Changing provider account authentication or auto-switch policy.
- Building a new model management settings page.
- Implementing custom agent registry UI; this task covers slash skills only.
- Deep transcript migration for old runs unless required by the chosen normal-chat history design.

## 8. Completion Notes

- result: Planned only. No application code was changed in this turn.
- follow-ups:
  - Run GitNexus impact analysis before editing `store.ts`, `Navigator.tsx`, `ChatInput.tsx`, runner `StartRunInput`, or provider adapter symbols when GitNexus tools are available.
  - Confirm canonical Claude reasoning wire values before adding `Max`, `Extra`, or `Ultracode` to persisted contracts.
  - Sequence [BUG-060](../../09-BugFix/todo/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md) (run-history rehydration) before or with this task — chat-log durability depends on it.
  - Update upstream `SS-05` and `04-Detailed-Coding-Plan` if provider-specific reasoning values become official.
- resolved decisions this pass: normal-chat history = synthetic `chat` run reusing existing tables + `run_kind` discriminator (not a first-class chat-session table); provider skills = all sources deduped by name with `workspace` > `flowpilot` > `provider` precedence.
- upstream docs updated: none in this planning pass; upstream updates are explicitly listed as part of the implementation follow-up when the reasoning contract is finalized.
