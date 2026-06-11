# Task-029: Workflow Runs Detail Page UX Refinements

## Metadata

- Document ID: `Task-029`
- Title: `Workflow Runs Detail Page UX Refinements`
- Phase: `task`
- Status: `done`
- Owner: `Antigravity`
- Reviewers: `User`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `[CP-07-Workflow-Engine-UI.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md)`
- Child Documents: None
- Related Documents: None
- Replaces: None
- Tags: `ui, workflow-runs, detail-page`

## AI Quick View

### Summary

- The workflow runs detail page (`/workflow-runs/$runId`) requires UX updates to align execution indicators, log sorting, and approval placement.
- Update the step list icon rendering for steps waiting for approval to use the yellow guard (`ShieldAlert`) icon consistently.
- Re-order run logs list to show the newest logs at the top.
- Replace the Workflow YOLO toggle Switch with a readonly status badge/text.
- Move the Step Approval UI from the right details workspace to the bottom of the left steps sidebar with simplified icon-only buttons and permission summary.

### Current Ask

- Author this Task plan file to document the requirements and implementation details before coding.

### Key Decisions

- `T-1` Use the `isWorkflowStepWaitingForApproval(status)` helper to determine the step icon rendering in the left sidebar.
- `T-2` Sort all logs and session event logs in descending order by `createdAt`.
- `T-3` Replace the toggle Switch with a readonly YOLO mode indicator.
- `T-4` Remove approval box from details workspace and render it pinned at the bottom of the left step sidebar.

### Constraints

- Only icons should be used for the Approve and Reject buttons (Check and X).
- No text area/comment input should be present in the new left-sidebar approval UI.

### Open Questions

- None.

### Source Refs

- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`

## 1. Goal

Improve the usability, layout, and visual feedback of the workflow runs execution dashboard detail page.

## 2. Parent Links

- coding plan: `[CP-07-Workflow-Engine-UI.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md)`
- specific upstream ids: `P-1`

## 3. Trigger

Admins executing workflows require clearer indicators, chronological top-down logs, and a more compact approval gate interface integrated into the sidebar.

## 4. Exact Change

- `T-1` Modify step list icon rendering inside `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`: check `isWorkflowStepWaitingForApproval(status)` instead of checking exact string `"WAITING_USER_APPROVAL"`.
- `T-2` Re-sort logs: Sort the `allLogs` array by `createdAt` in descending order (`right.createdAt.localeCompare(left.createdAt)`).
- `T-3` Remove the `Workflow YOLO` switch element from the header section and replace it with a readonly badge or text status component displaying `YOLO: ENABLED` or `YOLO: DISABLED`.
- `T-4` Relocate the Approval UI:
  - Remove the waiting approval card container from the step details container in the right panel.
  - Add a sticky/pinned card container at the bottom of the left `<aside>` sidebar container (outside the scrollable step list).
  - Render only two icon-only buttons: Approve (Check icon, green) and Reject (X icon, red), along with a summary text describing what command/MCP permissions are requested.

## 5. Touched Areas

- files: `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
- routes: `/workflow-runs/$runId`

## 6. Acceptance Check

- Step side list shows the yellow pulse shield (`ShieldAlert`) icon when step status is `WAITING_APPROVAL` or `WAITING_USER_APPROVAL`.
- The run logs are sorted with the newest entry at the top on both tabs.
- The Workflow YOLO element is a non-interactive status display.
- The approval gate UI appears at the bottom of the left side view, showing only Check and X icon buttons and a summary string when a step is waiting for approval.

## 7. Out of Scope

- Modifying database schemas or database trigger logic.
- Implementing custom provider runtime execution settings.

## 8. Completion Notes

- result: Pure UI/UX refinements implemented successfully. Pinned approval card placed at the bottom of the left sidebar. Cleaned up original workspace footer. Log arrays sorted descending. Checked `isWorkflowStepWaitingForApproval` in sidebar step listing. Replaced YOLO toggle switch with readonly text badge.
- follow-ups: Verified TypeScript compilation and ran vitest workflow-runs tests successfully.
- upstream docs updated: None.
