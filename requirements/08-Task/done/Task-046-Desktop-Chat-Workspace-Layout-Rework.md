# Task-046: Desktop Chat Workspace Layout Rework

## Metadata

- Document ID: `Task-046`
- Title: `Desktop Chat Workspace Layout Rework`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-037: Desktop Project Run History Popover](../done/Task-037-Desktop-Project-Run-History-Popover.md)
- Child Documents: `none`
- Related Documents: [Task-045: Desktop Artifacts Generated / Storage / Catalog Parity](../done/Task-045-Desktop-Artifacts-Generated-Storage-Catalog-Parity.md)
- Replaces: `none`
- Tags: `desktop, ui, layout, chat, projects, history, sidebar`

## AI Quick View

### Summary

- Rework the desktop workspace into three visible areas: left project rail, center chat area, and right history/control rail.
- Move the current account/status surface out of the left rail and into a right-side menu area.
- Keep the work UI strictly visual for this slice: no contract changes, no runner behavior changes, no workflow logic changes.

### Current Ask

- Update the desktop app UI to match the provided mockup direction:
  - compact project list with recent projects and collapse/expand
  - bottom chat controller strip
  - skill-first chat controls with other controls shown but disabled/greyed
  - project-grouped run history panel with collapse/expand and per-project "show more"
  - three-area left/center/right layout
  - top-left buttons to hide/show left and right sidebars

### Key Decisions

- `T-1` Preserve existing chat, provider, and history data sources; only change layout, presentation, and visibility behavior.
- `T-2` Keep the selected project highlighted in the project rail and use it as the anchor for history grouping.
- `T-3` Show at most 3 recent worked projects in the default visible list, with collapse/expand for the remainder.
- `T-4` Show at most 10 history items per project group by default, with a per-project "show more" affordance.

### Constraints

- UI only.
- Do not change provider/runtime contracts.
- Do not change task execution, turn sending, or history retrieval semantics.
- Preserve current desktop functionality when both sidebars are shown.

### Open Questions

- None for this slice.

### Source Refs

- `Task-044`
- `Task-037`

## 1. Goal

Create a more command-center-like desktop workspace that visually separates project selection, chat composition, and history/status controls while keeping the existing data flow intact.

## 2. Parent Links

- coding plan: `Task-044`
- tech design: `Task-037`
- system spec: `SS-11`
- specific upstream ids: `Task-044`, `Task-037`

## 3. Trigger

The current desktop app already has split chat and provider controls, but the workspace still presents the left rail as a single-purpose navigator and keeps history in a modal/popover pattern. The requested mockup needs a more layered, desktop-native arrangement.

## 4. Exact Change

- `T-1` Rework the desktop shell into left, center, and right regions with independent visibility toggles for the sidebars.
- `T-2` Replace the current single project selector with a compact project rail that highlights the active project, shows recent projects first, and supports collapse/expand for the rest.
- `T-3` Move account/status-related controls out of the left rail and into a right-side panel area.
- `T-4` Place the chat controller strip below the conversation area and make the visible controls feel like the mockup: selected skill active, other feature buttons present but greyed out.
- `T-5` Replace the modal-style history entry point with an always-available right-side history rail grouped by project, sorted by most recent activity, collapsed per project, and capped at 10 items before "show more".
- `T-6` Add top-left chrome controls to toggle left and right sidebars; when both are hidden, the center chat area should fill the window.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/App.tsx`, `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/components/RunStatus.tsx`, `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- The desktop app shows a clear left-center-right layout when both sidebars are enabled.
- The project rail highlights the active project and can collapse/expand extra projects.
- The chat controller sits below the main conversation area.
- Only the selected skill action appears active; the other feature buttons/icons are visible but greyed out.
- The history area is grouped by project, collapsible, and limited to 10 runs per project before "show more".
- The left and right sidebars can each be hidden from top-left chrome controls, and hiding both leaves only the chat area visible.
- Existing chat sending, history loading, and provider selection behavior still works.

## 7. Out of Scope

- No backend data shape changes.
- No new history persistence logic.
- No provider/account workflow changes beyond relocating the visible controls.
- No behavior changes for workflow execution, approvals, or prompts.

## 8. Completion Notes

- result: Implemented the desktop workspace layout refresh with left project/history rail, bottom chat controller, sidebar toggles, and right account rail.
- follow-ups: none for this UI-only slice
- upstream docs updated: `Task-046`

