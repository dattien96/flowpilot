# BUG-007: Leak Process If Server Turnoff

## Problem
Turning off the workflow server can leave the local-runner process alive, which means the workflow keeps consuming resources even though the server is no longer running.

This bug shows up in the shutdown/restart path:

1. Start a workflow run.
2. The server creates one live runner session for the step.
3. Stop the server.
4. The child process should stop with the server, but in the failing case it stays alive.
5. Start the server again and the workflow state becomes inconsistent.

## Original Repro
This is the step origin that triggered the bug report:

1. Start 1 workflow.
2. New process session is created.
3. Turn off the server.
4. The process leaks instead of shutting down cleanly.

## Current Behavior
- In the broken path, stopping the server can leave the local runner process alive.
- After restart, the workflow detail page can still show the run as `RUNNING` even though the original process is gone.
- The step can also be reconciled to `FAILED` later, but the UI may still offer a `Resume` action even when there is no valid live session.
- The replay path initially tried to reuse the old synthetic `providerSessionId` and failed with a provider thread parsing error.
- The `Response Captured` badge could appear even when the actual output was not present after the interrupted run.

## Failed Test Cases
These are the cases we hit during testing while validating the fix:

1. `Ctrl+C` server stop.
   - Expected: child process is killed.
   - Result after the fix: good.
2. Server restart after the original process is gone.
   - Expected: the old run should not pretend it still has a resumable live session.
   - Result before the final replay fix: `Resume` was still misleading or not usable.
3. Press `Replay` on the interrupted run.
   - Expected: start a fresh session for the replay.
   - Result before the final replay fix: the runtime tried to resume using the old synthetic session id and failed with `Failed to parse thread_id`.
4. Load the detail page too early.
   - Expected: do not mark the run `FAILED` before a real session exists.
   - Result before the guard fix: the page could briefly show a false failure state.

## Expected Behavior
- Stopping the server should shut down the child runner process that belongs to the active workflow session.
- A restarted server must not treat a dead process as a live resumable session.
- If a run is truly interrupted, the UI should offer a fresh replay path instead of trying to resume a dead provider thread.
- The detail page should only show captured-response or replay affordances when the underlying session state really supports them.

## Root Cause
The lifecycle handling was split across the server shutdown path, the workflow detail reconciliation path, and the replay path.

That created three distinct failure modes:

1. The server stop path could miss cleanup for the spawned runner process.
2. The detail page could trust stale persisted session state after restart.
3. The replay runtime could reuse a synthetic local session id instead of forcing a new provider session.

## Fix Solution
The fix was applied in stages:

1. Ensure local-runner shutdown closes live sessions when the server exits.
2. Reconcile stale workflow sessions on the detail page after restart.
3. Mark truly interrupted steps with a dedicated error marker.
4. Force replay of interrupted steps to start a fresh provider session instead of resuming the old synthetic id.
5. Hide misleading UI actions when there is no valid live session.

## Result
After the fix:

- `Ctrl+C` shuts down the child process cleanly.
- Restarting the server no longer leaves the UI in a broken resumable state.
- Interrupted steps replay into a fresh session instead of reusing the old synthetic `providerSessionId`.
- The detail page reflects the actual session lifecycle instead of showing stale active state.

## Verification
- Verified the server shutdown flow kills the live process on `Ctrl+C`.
- Verified the workflow detail page no longer crashes on interrupted-step reconciliation.
- Verified replay now starts a fresh session for interrupted runs instead of reusing the synthetic local session id.
- Verified the focused `admin-web` workflow runtime test slice passes.
- Verified the `admin-web` build succeeds after the change.
