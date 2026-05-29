# BUG-013: Run State Does Not Update In Workflow Run List

## Problem

When a user starts a quick run from Dashboard, opens the run detail page, waits until the run finishes, and then uses the left menu to open `Workflow Runs`, the run card in the list can still show `RUNNING`.

If the user manually reloads the page, the same run correctly shows `DONE`.

## Reproduction

1. Open Dashboard.
2. Start `Test - Single Claude Step`.
3. Wait until the run finishes on the detail page.
4. Click the left menu item `Workflow Runs`.
5. Observe the run list still showing `RUNNING` for the finished run.
6. Refresh the browser.
7. Observe the run status changes to `DONE`.

## Expected Result

When returning to `Workflow Runs`, the page must show the latest persisted state for each run without requiring manual refresh.

## Actual Root Cause

The stale state was not in the Next.js page under `app/(protected)`.

The real user path goes through the TanStack route:

- [workflow-runs.tsx](/C:/working/flowpilot/apps/admin-web/src/routes/_authenticated/workflow-runs.tsx)

`WorkflowRunHistoryPage` is the parent route for:

- `/workflow-runs`
- `/workflow-runs/$runId`

While the user is on the detail page, the parent route stays mounted and renders `<Outlet />`.
The list state (`runs`, `workflows`, `projects`, `runTitles`) was loaded only once on initial mount.

So when the user clicked the left menu back to `/workflow-runs`, the list reused old in-memory state and still showed `RUNNING`.
Only a full page reload forced a fresh fetch and then showed `DONE`.

## Fix

Refactored the route to reload list data whenever the location returns to `/workflow-runs`.

### Code Change

File:

- [workflow-runs.tsx](/C:/working/flowpilot/apps/admin-web/src/routes/_authenticated/workflow-runs.tsx)

Changes:

1. Exported `WorkflowRunHistoryPage` for direct test coverage.
2. Extracted initial list loading into `loadRunHistory()`.
3. Replaced the mount-only load effect with a pathname-driven effect.
4. Reload now runs whenever `location.pathname` becomes:
   - `/workflow-runs`
   - `/workflow-runs/`

This ensures the sidebar navigation path refreshes run data before rendering the list again.

## Regression Test

Added:

- [workflow-runs.test.tsx](/C:/working/flowpilot/apps/admin-web/src/routes/_authenticated/workflow-runs.test.tsx)

Covered scenario:

1. Initial list render returns `RUNNING`.
2. Route changes to `/workflow-runs/run-1` and parent renders `Outlet`.
3. Route changes back to `/workflow-runs`.
4. Reload runs again and updated data returns `DONE`.

## Verification

### Automated Test

Executed:

```bash
npm test -- --run 'src/routes/_authenticated/workflow-runs.test.tsx'
```

Result:

- Passed

### Live Browser Verification

Verified against the running local app and local runner.

Environment:

- App: `http://127.0.0.1:3002`
- Runner health endpoint: `http://127.0.0.1:4317/health`

Browser flow executed:

1. Sign in to the admin app.
2. Open Dashboard.
3. Run `Test - Single Claude Step`.
4. Use project `Flowpilot` because it has a valid workspace binding on this machine.
5. Wait until the database status becomes `DONE`.
6. Click left menu `Workflow Runs`.
7. Confirm the matching run card shows `DONE` immediately in the list.

Verified run:

- Run ID: `e8bd647a-3ab7-433c-b742-8ec07b060555`

Observed result:

- Database status became `DONE`
- Workflow run list card also showed `DONE`
- No manual page refresh was required

## Notes

- A separate attempt using project `FlowPilot Admin Web Demo` failed to start because that project has no directory binding on this machine. That was not related to this bug.
- Temporary verification user, favorite entry, and test runs were cleaned up after validation.
