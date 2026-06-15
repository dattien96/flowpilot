# Task-051: Desktop Chat Empty Project Guard And Control Header Cleanup

## Metadata

- Document ID: `Task-051`
- Title: `Desktop Chat Empty Project Guard And Control Header Cleanup`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Child Documents: `none`
- Related Documents: [BUG-062: Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders](../../09-BugFix/done/BUG-062-Desktop-Chat-Skill-Picker-Clears-Prompt-And-Ignores-Provider-Folders.md)
- Replaces: `none`
- Tags: `desktop, ui, chat, project, overlay`

## AI Quick View

### Summary

- Remove the extra explanatory `Chat controls` / `Provider, skills, and run settings` heading text from the desktop chat controller.
- Add a first-open guard overlay when no project is selected.
- Scope the guard so chat mode blocks the whole main chat surface, while workflow mode still leaves the right-side workflow rail usable.

### Current Ask

- Done. The chat controller now relies on the real controls instead of header copy, and the main chat surface is blurred and blocked until the user selects a project.

### Key Decisions

- `T-1` Keep the empty-project guard inside `ChatWorkspace` so it covers the main column uniformly in both chat modes.
- `T-2` Make `ChatInput` send gating explicitly require a selected project instead of relying only on runner-side validation.
- `T-3` Keep this slice UI-only and avoid changing runner or project-loading contracts.

### Constraints

- Desktop UI only.
- No provider or runner contract changes.
- Keep workflow-mode rail controls interactive while the chat surface is locked.

### Open Questions

- None.

### Source Refs

- `Task-044`
- `Task-049`
- `BUG-062`

## 1. Goal

Clean up the desktop chat UI by removing redundant control-header copy and by preventing chat interaction until a project is selected, with the guard behavior matching chat mode versus workflow mode.

## 2. Parent Links

- coding plan: [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- tech design: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `Task-044`, `Task-049`, `BUG-062`

## 3. Trigger

The desktop chat UI still showed explanatory header text that the user did not want to keep, and first-open behavior allowed the chat surface to remain fully visible and interactive even when no project had been selected yet.

## 4. Exact Change

- `T-1` Remove the visible `Chat controls` / `Provider, skills, and run settings` heading copy from the chat controller.
- `T-2` Add a blurred guard overlay across the main chat workspace when `selectedProjectId` is empty.
- `T-3` Make the composer send gating and placeholder explicitly reflect the missing-project state.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- The chat controller no longer shows the removed header copy.
- With no selected project, the main chat surface is blurred and blocked by a guide overlay.
- In normal chat mode, that overlay blocks the chat timeline and chat controls.
- In workflow mode, the overlay blocks only the main chat column and the workflow rail remains usable.
- Send is disabled until a project is selected.

## 7. Out of Scope

- No runner-side project validation changes.
- No redesign of the project selector or navigator.
- No workflow rail UI rewrite.

## 8. Completion Notes

- result: Removed the redundant controller heading text, added the empty-project guard overlay, and aligned chat send gating with the project requirement.
- follow-ups: none
- upstream docs updated: `Task-051`
