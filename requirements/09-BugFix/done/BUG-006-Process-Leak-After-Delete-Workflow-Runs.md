# BUG-006: Process Leak After Delete Workflow Runs

## Problem
Deleting workflow runs can remove the run records while leaving the local-runner session process alive.

This creates a process leak:

1. A workflow run starts and creates one or more live runner sessions.
2. The user deletes that workflow run.
3. The workflow row and artifact files are removed, but the runner process can still keep running.

## Current Behavior
- Workflow run deletion removes the target run rows.
- Local artifact cleanup runs for the deleted workflow run ids.
- The delete flow did not consistently close active `workflow_run_sessions` before deleting the workflow run.
- The workflow-run history page could also keep showing stale active-process counts until the next refresh cycle.

## Expected Behavior
- Before deleting workflow runs, the app must collect all related sessions for those runs.
- Every session with `status = active` and a non-null `processKey` must be closed in the local runner first.
- After session shutdown succeeds, artifact cleanup and workflow row deletion can continue.
- The workflow-run history page should immediately remove deleted runs from its in-memory active-process count.

## Root Cause
The workflow delete flow was only responsible for artifact cleanup and database deletion.

It did not include a mandatory pre-delete session shutdown step for active local-runner processes tied to the selected workflow runs.

## Fix Solution
Add pre-delete session cleanup in the shared local-first workflow gateway:

1. Load workflow run detail for each selected run id.
2. Read `detail.sessions` and collect every `active` session with a non-null `processKey`.
3. Call `localRunnerGateway.closeSession(...)` for each live session before deleting anything else.
4. Delete local artifacts for the selected workflow run ids.
5. Delete the workflow run rows from persistence.
6. Update workflow-run history page state so deleted runs are also removed from the in-memory active session list immediately.

This keeps the shutdown rule centralized in the delete flow used by both workflow-run deletion entry points.

## Result
After this fix, deleting workflow runs no longer leaves their live local-runner processes behind.

The delete flow now shuts down active sessions first, then removes artifacts, then removes workflow rows, and the history page counter updates immediately after success.

## Verification
- Added focused test coverage for `LocalFirstWorkflowGateway.deleteWorkflowRuns(...)` to verify active sessions are closed before artifact cleanup and row deletion.
- Verified the focused `admin-web` test slice for the local-first workflow gateway.
- Verified the `admin-web` build succeeds after the change.
