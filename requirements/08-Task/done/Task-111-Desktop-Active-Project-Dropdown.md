---
Document ID: Task-111
Title: Desktop Active Project Dropdown
Phase: task
Status: done
Owner: DatNguyen
Reviewers: —
Created: 2026-06-24
Last Updated: 2026-06-24
Parent Documents: CP-33-Desktop-Project-Chat-Drive-Folder-Selection, CP-15-02-Project-Launch-And-Workspace-UX
Child Documents: —
Related Documents: Task-036-Desktop-Provider-Accounts-Sidebar, Task-046-Desktop-Chat-Workspace-Layout-Rework, Task-051-Desktop-Chat-Empty-Project-Guard-And-Control-Header-Cleanup
Replaces: —
Tags: desktop, project-selector, navigator, ux
---

## AI Quick View

### Summary

- Replace the project list (card buttons) in the Navigator sidebar with a single `<select>` dropdown.
- The selected option always reflects the active project; the path of the selected project is shown below the dropdown.
- On app start, auto-select from `localStorage` (`fp:lastProjectId`) or fall back to the first project returned by the runner.
- Every call to `selectProject()` now persists the chosen project ID to `localStorage` so the choice survives a restart.

### Current Ask

- Implement dropdown + persistence for the active project selector in the desktop app.

### Key Decisions

- `T-1` Use native `<select>` — no custom dropdown component needed at this scope.
- `T-2` Projects ordered by recency (existing `orderedProjects` logic kept unchanged).
- `T-3` `localStorage` key `fp:lastProjectId` stores the last explicitly-selected or chat-initiated project ID.
- `T-4` Auto-select runs only when `selectedProjectId` is undefined after `loadProjects()` succeeds; it does not override an already-active selection.

### Constraints

- Do not break existing history, remote-chats, or run-start flows that depend on `selectedProjectId`.
- No new IPC or Electron APIs — browser `localStorage` is sufficient.

### Open Questions

- None.

### Source Refs

- CP-33 P-1, P-2 (project binding context)
- CP-15-02 P-3 (workspace UX)

## 1. Goal

Show a compact dropdown in the Navigator sidebar for project selection. The active project is always visible as the selected option. The selection is remembered across app restarts.

## 2. Parent Links

- coding plan: CP-33-Desktop-Project-Chat-Drive-Folder-Selection, CP-15-02-Project-Launch-And-Workspace-UX
- tech design: —
- system spec: SS-01-Project
- specific upstream ids: SS-01 §3 (project structure)

## 3. Trigger

The project list card UI required scrolling / "Show more" for users with many projects and gave no persistent memory of which project was last active. The user requested a dropdown that immediately shows the active project and remembers it across app restarts.

## 4. Exact Change

- `T-1` `apps/desktop-flowpilot/src/state/store.ts` — add `LAST_PROJECT_KEY = "fp:lastProjectId"` constant.
- `T-2` `store.ts` `loadProjects()` — after projects load, if `selectedProjectId` is undefined, restore from `localStorage` (validate against loaded list) or default to `projects[0]`.
- `T-3` `store.ts` `selectProject()` — call `localStorage.setItem(LAST_PROJECT_KEY, projectId)` before setting state.
- `T-4` `apps/desktop-flowpilot/src/components/Navigator.tsx` — replace `.project-rail-section` project-list block with a `<select>` + path caption. Remove unused `showAllProjects`, `visibleProjects`, `canShowMoreProjects`, and `PROJECT_LIMIT`.
- `T-5` `apps/desktop-flowpilot/src/styles.css` — add `.project-selector`, `.project-selector-select`, `.project-selector-path` styles.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules: Navigator sidebar, project store slice
- routes: desktop app main chat view (left rail)
- tables: none

## 6. Acceptance Check

- [ ] On first launch (no localStorage) the first project in the list is auto-selected.
- [ ] On subsequent launches the previously active project is pre-selected.
- [ ] Changing the dropdown immediately switches the active project (history + skills reload).
- [ ] After a chat run the chosen project persists as active on next open.
- [ ] All existing history / remote-chat / sync flows work unchanged.
- [ ] `npx tsc --noEmit` passes with no errors.

## 7. Out of Scope

- Custom styled dropdown (native `<select>` is sufficient).
- Per-project "Recent" badge (removed with the list; recency ordering is retained inside the dropdown).
- Cross-device sync of the last-used project (localStorage is machine-local).

## 8. Completion Notes

- result: Implemented and typecheck passes (no errors).
- follow-ups: none
- upstream docs updated: none required (no business-rule change)
