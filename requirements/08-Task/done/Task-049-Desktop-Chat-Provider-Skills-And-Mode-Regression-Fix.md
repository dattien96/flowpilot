# Task-049: Desktop Chat Provider Skills And Mode Regression Fix

## Metadata

- Document ID: `Task-049`
- Title: `Desktop Chat Provider Skills And Mode Regression Fix`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Child Documents: `none`
- Related Documents: [Task-047: Desktop Chat Rail Polish And Resizing](../done/Task-047-Desktop-Chat-Rail-Polish-And-Resizing.md)
- Replaces: `none`
- Tags: `desktop, ui, regression, chat, provider, skills, workflow`

## AI Quick View

### Summary

- Restore the provider-filtered model dropdown in chat mode.
- Reload provider skills with the selected project path and hide skill UI in workflow mode.
- Add a chat-mode expand/collapse affordance for the control box and default it to expanded for new runs.

### Current Ask

- Done. Chat mode once again filters models by provider, workflow mode hides the chat control box, and skills reload from the provider/project context instead of showing stale UI behavior.

### Key Decisions

- `T-1` Avoid changing `selectProvider` because GitNexus marked that symbol as `CRITICAL` blast radius.
- `T-2` Restore provider/project-aware skill loading from the `ChatInput` side using the existing low-risk `loadSkills` path.
- `T-3` Keep workflow-mode send gating unchanged and remove the visible skill-entry affordance entirely in workflow mode.

### Constraints

- UI and desktop-client behavior only.
- No runner contract changes.
- Keep the regression fix narrow and compatible with the earlier layout work.

### Open Questions

- None for this slice.

### Source Refs

- `Task-048`
- `Task-044`
- `CA-075`

## 1. Goal

Fix the desktop chat regressions introduced by the recent layout pass so chat mode restores provider-driven models and skills, while workflow mode hides the chat-only control box and keeps workflow/step selection as the send prerequisite.

## 2. Parent Links

- coding plan: [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- tech design: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `Task-048`, `Task-044`, `CA-075`

## 3. Trigger

The layout refactor preserved the three-pane shell but regressed several mode-specific behaviors: provider-filtered models disappeared, skill loading no longer followed the provider/project context, workflow mode still exposed chat-only skill UI, and the top chat control band no longer matched the requested collapsed/expanded behavior.

## 4. Exact Change

- `T-1` Restore the provider-filtered model selector in chat mode, with text-input fallback only when no supported models exist for that provider.
- `T-2` Reload skills from the selected provider plus selected project path in chat mode, and reset stale skill picks when the provider/project context changes.
- `T-3` Hide the entire control box above the composer in workflow mode so skill UI cannot be opened there.
- `T-4` Add an expand/collapse arrow for the chat-only control box and default it to expanded for a new run.
- `T-5` Keep workflow-mode send behavior gated on selecting at least one workflow or step.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- Chat mode shows a model list filtered by the selected provider again.
- Skills reload by provider and project path, and stale selections clear when the context changes.
- Workflow mode hides the control box above the composer and cannot open the skill list there.
- Workflow mode still requires a selected workflow or step before send is enabled.
- Chat mode exposes an expand/collapse control for the top control band and new runs start expanded.

## 7. Out of Scope

- No backend or runner changes.
- No account-provider logic changes beyond the mock-client alignment for local fallback behavior.
- No redesign of the three-pane shell.

## 8. Completion Notes

- result: Restored provider-filtered model and skill behavior, removed workflow-mode skill affordances, and added the chat-mode expand/collapse control.
- follow-ups: none
- upstream docs updated: `Task-049`
