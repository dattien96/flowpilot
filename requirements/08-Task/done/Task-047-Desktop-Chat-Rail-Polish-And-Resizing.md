# Task-047: Desktop Chat Rail Polish And Resizing

## Metadata

- Document ID: `Task-047`
- Title: `Desktop Chat Rail Polish And Resizing`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [Task-046: Desktop Chat Workspace Layout Rework](../done/Task-046-Desktop-Chat-Workspace-Layout-Rework.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-037: Desktop Project Run History Popover](../done/Task-037-Desktop-Project-Run-History-Popover.md)
- Replaces: `none`
- Tags: `desktop, ui, layout, chat, resize, sidebar, workflow`

## AI Quick View

### Summary

- Replace the text sidebar toggles with icon-only chrome controls.
- Move the chat controller stack into the right rail above the accounts panel and make it visually tighter.
- Restore workflow/step selection in workflow mode and add draggable left/right rail resizing.

### Current Ask

- Update the desktop UI only:
  - icon-only left/right sidebar buttons
  - right-rail chat controller above accounts
  - compact provider/model/reasoning and project list rows
  - draggable left and right sidebar widths
  - outside-click dismissal for the skills picker
  - restored workflow/step selectors in workflow mode

### Key Decisions

- `T-1` Preserve the current data flow and runner contract.
- `T-2` Keep direct chat composition behavior intact; only relocate and compress the controller UI.
- `T-3` Reuse the existing workflow/step catalog and selection logic from the pre-layout version.

### Constraints

- UI only.
- No backend or runner changes.
- No contract changes.
- Keep the task narrow and avoid redesigning unrelated desktop surfaces.

### Open Questions

- None for this slice.

### Source Refs

- `Task-046`

## 1. Goal

Refine the desktop chat workspace so it better matches the reference mockup: icon chrome, compact control bands, draggable side rails, and workflow-mode selectors restored in the left rail.

## 2. Parent Links

- coding plan: `Task-046`
- tech design: `Task-046`
- system spec: `SS-11`
- specific upstream ids: `Task-046`

## 3. Trigger

The first workspace layout pass established the new three-area shell, but the screenshots show a tighter chrome treatment, a right-rail control stack, and the need for resizable sidebars plus restored workflow selectors.

## 4. Exact Change

- `T-1` Replace the text-based sidebar toggles with icon-only buttons.
- `T-2` Move the chat controller UI into the right sidebar above the account panel and compress its height.
- `T-3` Restore the workflow/step selection UI when workflow mode is active.
- `T-4` Add drag handles for resizing the left and right sidebars.
- `T-5` Make the skills modal dismissible by clicking outside its bounds.
- `T-6` Tighten the project row density and controller field heights to match the screenshot.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/App.tsx`, `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- The header sidebar toggles use icons instead of text labels.
- The chat controller stack appears in the right rail above the account panel.
- Workflow mode exposes workflow and step selection again.
- The left and right sidebars can be resized by dragging the vertical handles.
- Clicking outside the skills modal closes it.
- The control bands and project list rows are noticeably more compact.

## 7. Out of Scope

- No backend changes.
- No new provider behavior.
- No changes to run execution or history semantics.

## 8. Completion Notes

- result: Implemented the icon chrome, right-rail chat controller, restored workflow selectors, resize handles, and outside-click skills dismissal.
- follow-ups: none for this UI-only slice
- upstream docs updated: `Task-047`

