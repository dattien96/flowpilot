# BUG-017: Artifact Sync Must Require Valid workflow_runs And artifact_runs Rows

## Problem
Artifact sync can still start when the local artifact folder exists and the `artifact_runs` row is present, but the parent `workflow_runs` row has already been deleted.

This makes startup/bootstrap sync and manual sync accept an invalid artifact lifecycle state.

## Expected Rule
Local artifact sync is only acceptable when all of these are true:

- A valid `artifact_runs` row exists for the artifact id.
- A valid `workflow_runs` row exists for the artifact run's `workflow_run_id`.
- The local artifact folder still exists on disk.

If either Supabase row is missing, sync must be skipped or rejected instead of uploading stale local data.

## Current Root Cause
- Startup bootstrap reconciliation in `artifact-auto-sync.ts` queries `artifact_runs` only.
- Local runner `SyncArtifact(...)` reads the local manifest and syncs immediately without checking Supabase row validity.
- This allows races or stale states where a deleted workflow run can still drive a local artifact sync attempt.

## Fix Plan
1. Update bootstrap candidate selection to sync only artifacts whose `workflow_run_id` still exists in `workflow_runs`.
2. Add a local-runner sync guard that verifies both the `artifact_runs` row and the `workflow_runs` row still exist before syncing.
3. Return a normal sync failure when either row is missing so the caller can mark the artifact sync state as failed/skipped instead of uploading stale data.
4. Add focused tests for:
   - artifact row exists but workflow run row is missing
   - workflow run exists but artifact row is missing
   - both rows exist and sync continues normally

## Notes
- This bug is related to the startup orphan-local-artifact cleanup path, but it must be fixed independently because sync and cleanup can run in parallel across admin-web and local-runner processes.
