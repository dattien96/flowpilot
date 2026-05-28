# BUG-003: Session Update After Timeout

## Problem
In the workflow run detail step view, a session can remain shown as `active` after the workflow run has already finished and the session idle timeout has elapsed.

This is visible in two cases:

1. A new prompt is sent after timeout. The old session is marked `complete` and the new one is shown as `NEW SESSION`. This path works.
2. No prompt is sent. The user waits past the timeout and refreshes the page. The session still shows `active`, which is incorrect.

## Root Cause
The detail view was only reading the persisted `workflow_run_sessions` row. It did not reconcile stale `active` sessions from the session start timestamp, so an idle session could stay active until another message forced a transition or a manual cleanup happened.

## Fix Solution
Add refresh-time reconciliation for idle workflow sessions:

1. Read the project `sessionIdleTtlMinutes` value.
2. Compare it against each session `startedAt` timestamp when the workflow run detail page loads.
3. If a session is still `active` and the TTL has elapsed, mark the row as `completed` with a new `completed_at` timestamp and clear the runner `process_key`.
4. Update the local session data in the detail view so the UI immediately reflects the completed state on refresh.

Implementation was kept local to the workflow run detail route and a small helper in `features/workflow-engine` to avoid changing shared gateway behavior.

## Result
After this fix, refreshing the workflow run detail page after the timeout will show the session as `completed` instead of `active`, even when no new prompt was sent and the workflow run is still open.

## Verification
- Added focused unit coverage for the timeout reconciliation helper.
- Verified the admin web test suite slice for workflow runtime behavior.
- Verified the `admin-web` build succeeds after the change.
