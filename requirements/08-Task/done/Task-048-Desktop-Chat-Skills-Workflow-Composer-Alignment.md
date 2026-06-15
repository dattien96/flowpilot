# Task-048: Desktop Chat Skills Workflow Composer Alignment

## Metadata

- Document ID: `Task-048`
- Title: `Desktop Chat Skills Workflow Composer Alignment`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [Task-047: Desktop Chat Rail Polish And Resizing](../done/Task-047-Desktop-Chat-Rail-Polish-And-Resizing.md), [Task-046: Desktop Chat Workspace Layout Rework](../done/Task-046-Desktop-Chat-Workspace-Layout-Rework.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Replaces: `none`
- Tags: `desktop, ui, layout, chat, skills, workflow, accounts`

## AI Quick View

### Summary

- Recenter the desktop chat composer so the send bar lives at the bottom of the main chat area again.
- Move workflow-mode selection into the right sidebar beneath the mode control.
- Replace the multi-chip skills row with a single grouped `Select skills (n)` control and keep the skills picker dismissible by clicking outside.
- Add a small padding buffer around the active account card area.

### Current Ask

- Done. The composer is back at the center-bottom chat area, the skills control is grouped, workflow selection sits under the mode control on the right, and the active account block has extra padding.

### Key Decisions

- `T-1` Preserve the current data flow and runner contracts.
- `T-2` Keep the skills picker interaction model intact while compressing its visible trigger.
- `T-3` Treat workflow selection as right-rail chrome rather than left-rail navigation for this slice.

### Constraints

- UI only.
- No backend, contract, or runner behavior changes.
- Keep the change narrow and visually aligned to the supplied screenshots.

### Open Questions

- None for this slice.

### Source Refs

- `Task-047`
- `Task-046`
- `SS-11`

## 1. Goal

Realign the desktop chat layout so the main composer returns to the bottom of the central chat area, the skills control collapses into a compact grouped trigger, and workflow selection lives in the right sidebar beneath the mode control.

## 2. Parent Links

- coding plan: [Task-047: Desktop Chat Rail Polish And Resizing](../done/Task-047-Desktop-Chat-Rail-Polish-And-Resizing.md), [Task-046: Desktop Chat Workspace Layout Rework](../done/Task-046-Desktop-Chat-Workspace-Layout-Rework.md)
- tech design: [Task-047: Desktop Chat Rail Polish And Resizing](../done/Task-047-Desktop-Chat-Rail-Polish-And-Resizing.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `Task-047`, `Task-046`

## 3. Trigger

The previous layout pass moved too much chat-control chrome into the side rail. The newer screenshots show a tighter desktop arrangement where the composer stays centered at the bottom, skills are grouped into a compact selector, and workflow controls sit in the right sidebar instead of the left.

## 4. Exact Change

- `T-1` Move the send composer back to the bottom of the main chat area.
- `T-2` Keep the selected skills visible above the chat view, but render them as one grouped selector with a count and dropdown affordance.
- `T-3` Move workflow-mode selection to the right sidebar under the mode control.
- `T-4` Add a small padding buffer around the active account card block.
- `T-5` Preserve the outside-click dismissal behavior for the skills picker modal.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- The composer with the send button sits at the bottom of the central chat area again.
- The selected skills render as one grouped `Select skills (n)` control.
- The workflow selector appears in the right sidebar below the mode control.
- The active account block has visibly more padding from the parent container edge.
- Clicking outside the skill picker closes it.

## 7. Out of Scope

- No backend changes.
- No provider behavior changes.
- No workflow execution changes.
- No redesign of the project/history rails beyond the spacing and relocation requested here.

## 8. Completion Notes

- result: Implemented the center-bottom composer restore, grouped skills selector, right-rail workflow controls, and account padding tweak.
- follow-ups: none
- upstream docs updated: `Task-048`
