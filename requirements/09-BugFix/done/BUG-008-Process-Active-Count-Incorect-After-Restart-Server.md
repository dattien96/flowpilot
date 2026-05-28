# BUG-008: Process Active Count Incorrect After Restart Server

## Problem
After the server is stopped and started again, the workflow history page can show an incorrect active-process count for a run.

This was especially visible in the manual-kill restart path:

1. Start 1 workflow.
2. The run creates 1 active process.
3. Turn off the server right away.
4. Manually kill the process from the OS task list.
5. Start the server again.
6. The UI still shows 1 active session counted even though the process is no longer alive.

## Current Behavior
- Before the fix, the active count could stay stale after server restart.
- The history page was still trusting persisted session rows that had not been reconciled against the live local runner.
- If the original process was manually killed, the UI could still treat the old row as active until another refresh or cleanup path corrected it.

## Expected Behavior
- After restart, active-process counts must reflect the real local-runner state.
- Dead sessions should not continue to count as active.
- The workflow history page should reconcile stale rows after the server comes back up.

## Root Cause
The restart flow depended on persisted `workflow_run_sessions` state without validating it against the live local runner.

If the server stopped and the process was already gone, the history page could still count the old session as active because the row had not yet been marked completed.

## Fix Solution
This issue is now resolved by the shutdown/reconciliation work done for [BUG-007]:

1. When the server stops, the local runner now kills the live process tied to the active session.
2. When the server starts again, the workflow detail/history logic reconciles stale sessions instead of trusting the old active row blindly.
3. The active-process count is updated from the reconciled session state, not from the stale pre-shutdown count.

## Result
After the shutdown cleanup fix, this restart-count mismatch is no longer reproducible in the test flow we used.

The workflow history page no longer keeps counting a dead session as active after the server is restarted.

## Verification
- Re-ran the original restart scenario: start workflow, stop server, kill the process, start server again.
- Confirmed the old session is now reconciled instead of remaining counted as active.
- Verified the focused admin web workflow runtime tests still pass.
- Verified the `admin-web` build succeeds after the related session-lifecycle changes.
