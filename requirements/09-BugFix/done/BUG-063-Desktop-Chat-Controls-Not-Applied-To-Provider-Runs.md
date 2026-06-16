# BUG-063: Desktop Chat Controls Not Applied To Provider Runs

## Metadata

- Document ID: `BUG-063`
- Title: `Desktop Chat Controls Not Applied To Provider Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Child Documents: `none`
- Related Documents: [BUG-062: Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders](./BUG-062-Desktop-Chat-Skill-Picker-Clears-Prompt-And-Ignores-Provider-Folders.md), [BUG-064: Codex Approval Decision Value Rejected By App-Server](./BUG-064-Codex-Approval-Decision-Value-Rejected-By-AppServer.md), [BUG-065: Desktop Open In IDE Fails With spawn EINVAL On Windows](./BUG-065-Desktop-Open-In-IDE-Spawn-EINVAL-On-Windows.md)
- Replaces: `none`
- Tags: `desktop, bugfix, chat, provider, codex, claude`

## AI Quick View

### Summary

- Chat mode allowed users to select provider controls, but the runner did not carry the selected model into the provider turn request.
- Codex app-server turns received YOLO and skills, but not model or reasoning params at the app-server thread boundary.
- Claude turns received YOLO and skill prompt injection, but its CLI args did not include the selected model or reasoning effort.
- Follow-up (same report, `2026-06-16`): model and YOLO were frozen at run creation, so they could not be changed between chat prompts even though both providers re-apply them per turn; chat controls were not disabled while a turn was in flight; the selected skill was cleared from the picker on send; and selected-skill injection read the runner workspace and matched by a fragile name/id, so the picked skill was not reliably delivered.

### Current Ask

- Done. Chat-mode provider runs now apply selected model, reasoning, YOLO, and skills for Codex and Claude.
- Done (follow-up). Model and YOLO are now per-turn in chat mode (changeable between prompts); all chat controls and Send are disabled only while a turn is in flight; selected skills are cleared on send so they must be explicitly re-picked each turn (no silent re-injection); and every selected skill's full content is delivered to the model by its explicit path.
- Done (follow-up #2). The desktop now opens in chat mode with Codex pre-selected, and the YOLO toggle shows a disabled style while a turn is in flight (completing the in-flight grey-out ask).
- Flow mode is explicitly out of scope for this fix and will be handled later.
- Split out: the YOLO=off "Approve doesn't unblock Codex" defect and the "open in IDE" `spawn EINVAL` defect are tracked as their own bug docs (see Related Documents), not here.

### Key Decisions

- `V-1` Treat model as a run-level chat default, but resend it (and YOLO) on every chat turn so they can change between prompts; a nil per-turn pointer falls back to the run-level value (workflow/step mode is unaffected).
- `V-2` Keep selected skills as turn-level data and deliver full selected-skill content into prompts, resolved by the picker's explicit file path.
- `V-3` Preserve workflow/flow mode behavior for a later slice.
- `V-4` Runtime change = changing controls between prompts (allowed); "turn in flight" = the AI is producing the current answer (all controls + Send greyed out).
- `V-5` Selected skills are always delivered to the model; the provider may still auto-read other skills via native discovery (acceptable) — no runner-side exclusion is attempted.

### Constraints

- Scope is direct chat mode only.
- Avoid changing desktop workflow/step launch behavior.
- Codex app-server integration still uses the runner's JSON-RPC param builder rather than a per-turn shell command.

### Open Questions

- None for this slice.

### Source Refs

- User report on `2026-06-16`: selected chat controls must be handled in the real Codex and Claude processes.
- Follow-up user report on `2026-06-16`: controls must be changeable between prompts (runtime) and greyed out only while the AI is answering; selected skills must be delivered to the AI and must not disappear from the picker after a turn (auto-reading other skills natively is acceptable).

## 1. Issue Summary

Desktop chat mode exposed provider, model, reasoning, YOLO, and skills controls. The UI sent those values, but not every selected value reached the real provider execution boundary for Codex and Claude.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode with `apps/local-runner`
- reproduction steps:
  - select a project in chat mode
  - select Codex or Claude
  - select a model, reasoning effort, YOLO mode, and one or more skills
  - send a chat prompt
  - inspect the provider boundary request/args
- frequency: always before the fix for model handling

## 4. Expected vs Actual

- expected:
  - selected model reaches the provider process boundary
  - selected reasoning reaches the provider process boundary
  - selected YOLO controls approval posture
  - selected skills influence the provider turn
- actual:
  - selected model was not stored on the interactive run and could not reach adapters
  - Codex app-server `thread/start` did not receive model or reasoning params
  - Claude CLI args did not receive `--model` or `--effort`
  - YOLO and skills were already partially wired, but lacked a regression guard covering the full chat-mode path
  - model and YOLO were frozen at run creation: only `reasoningEffort` and skills had a per-turn path, so changing model/YOLO between prompts had no effect
  - the approval bridge read run-level `yolo`, so a per-turn YOLO change could not drive runner auto-approve
  - chat controls (provider/model/reasoning/YOLO/skills) stayed enabled while a turn was in flight, while only the textarea + Send were disabled
  - the picker cleared the selected skill on send — this is correct: skills must be explicitly re-picked per turn to avoid silent re-injection
  - `injectSkillContent` ignored the run cwd (read the runner workspace) and matched skills by `ID` against the selected name, so the chosen skill was often not injected

## 5. Impact

- users affected: desktop direct chat users
- workflows affected: Codex and Claude chat mode
- severity: high for model-specific chat behavior

## 6. Root Cause

- hypothesis:
  - the desktop sent chat control values, but the runner only persisted a subset on the run
- confirmed cause:
  - `interactiveRun` stored `yolo` and `reasoningEffort`, but not the selected model
  - `TurnRequest` had no `ModelName`, so adapters could not apply the selected model
  - `codexThreadStartParams` only carried `cwd`, sandbox, approval mode, and MCP servers
  - `claudeArgs` ignored model and reasoning inputs
- evidence:
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/provider_registry.go`
  - `apps/local-runner/internal/runner/codex_appserver.go`
  - `apps/local-runner/internal/runner/claude_permission_mcp.go`
- follow-up confirmed cause:
  - `TurnInput` carried only `reasoningEffort`/skills per turn; model + YOLO were read from run-level state, so changing them between prompts had no effect
  - `turnBridge.RequestApproval` used `rs.yolo` instead of the per-turn posture
  - `ChatInput` left controls enabled during a turn and called `setSelectedSkills([])` on send
  - `injectSkillContent` used `r.ListSkills()` (runner workspace) and matched `skill.ID == name`, so a selected skill in the run cwd / under a different name was not injected
- follow-up evidence:
  - `apps/local-runner/internal/runner/runner.go` (`injectSelectedSkills`)
  - `apps/local-runner/internal/runner/provider_event.go`, `interactive_handlers.go`
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `src/state/store.ts`, `src/types/contract.ts`, `src/client/HttpWsRunnerClient.ts`

## 7. Fix Strategy

- `F-1` Add `ModelName` to the provider-neutral `TurnRequest`.
- `F-2` Store `StartRunInput.Model` on chat `interactiveRun` state.
- `F-3` Copy the run model into each provider turn request.
- `F-4` Add selected model and reasoning to Codex app-server `thread/start` params.
- `F-5` Add selected model and reasoning to Claude CLI args as `--model` and `--effort`.
- `F-6` Add regression tests for chat-mode request propagation and provider-specific command/param construction.
- `F-7` Add per-turn `Model *string` + `YoloMode *bool` to `TurnInput`/`turnBody`/contract `TurnInput`; the desktop resends model + YOLO on every chat turn and `runTurn` overrides the run-level defaults when supplied.
- `F-8` Thread the effective per-turn YOLO into `turnBridge` so runner auto-approve uses the same posture as the adapter sandbox/mode.
- `F-9` Disable all chat controls (provider/model/reasoning/YOLO/skills) and Send while a turn is in flight (`status` running/waiting); keep them enabled between turns.
- `F-10` Clear selected skills immediately on send so skills are only injected when explicitly re-picked for a turn; persistent selection across turns would silently re-inject skills the user did not intend to re-send.
- `F-11` Replace selected-skill injection with `injectSelectedSkills`: read each selected skill by the picker's explicit path (desktop now sends `SkillSelection.path`), fall back to run-cwd id/name discovery, and label the block "Selected Skills".
- `F-12` Default the desktop to chat mode with Codex pre-selected on open (`chatMode:"normal_chat"`, `selectedProvider:"codex"` in the store initial state) and add a visible `:disabled` style for the YOLO toggle so the in-flight grey-out is visible.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'Test(ChatModeSelectedControlsReachProviderTurnRequest|CodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes|ClaudeArgsYoloPosture|ClaudeArgsIncludesStrictMcpConfig)' -count=1`
- `V-2` `npm run typecheck --prefix apps/desktop-flowpilot`
- `V-3` `git diff --check`
- `V-4` Full `go test ./internal/runner -count=1` was attempted, but this machine fails unrelated existing environment-dependent tests because `powershell` is not on `PATH` and Google Drive MCP config prerequisites are unavailable.
- `V-5` (follow-up) `go test ./internal/runner -run 'Test(ChatModeTurnLevelControlsOverrideRunDefaults|InjectSelectedSkillsDeliversSelection)' -count=1` — per-turn model/YOLO override + selected-skill delivery.
- `V-6` (follow-up) `go build ./...` (runner) and `npm run typecheck --prefix apps/desktop-flowpilot` both pass.

## 9. Regression Guard

- tests: `TestChatModeSelectedControlsReachProviderTurnRequest`, `TestChatModeTurnLevelControlsOverrideRunDefaults`, `TestInjectSelectedSkillsDeliversSelection`, `TestCodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes`, `TestClaudeArgsYoloPosture`
- alerts: none
- audit checks: impact reviewed for `StartRunInput`, `TurnRequest`, `TurnInput`, `runTurn`, `turnBridge`, `createRun`, `interactiveRun`, `codexThreadStartParams`, `claudeArgs`, and `injectSelectedSkills`. GitNexus MCP was unavailable this session; the follow-up was verified via `go build`, targeted `go test`, and desktop `typecheck` instead.

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - Flow mode provider-control execution will be handled in a later slice.
