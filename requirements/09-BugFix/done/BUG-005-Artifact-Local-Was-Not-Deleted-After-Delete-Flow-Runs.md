# BUG-006: Local Artifacts Are Not Deleted After Deleting Workflow Runs

## Problem
When a user deletes workflow runs from the UI, the database rows are removed, but the related local artifact files are left behind in the workspace.

This causes orphaned artifact directories to accumulate over time.

## Current Behavior
- Workflow run deletion removes rows from `workflow_runs`.
- Related local artifact files remain on disk.
- The delete flow does not trigger any cleanup in the local runner artifact store.

## Actual Local Storage Path
The current artifact storage path is under:

- `.flowpilot/artifacts/<projectId>/<featureId>/<workflowRunId>/<stepKey>/<artifactId>/`

This bug is about the current `.flowpilot/artifacts/...` structure, not an older `.flow-pilot/workflow-runs/...` path.

## Example
For a workflow run:

- `.flowpilot/artifacts/project-1/842e8834-38f9-4667-a9a3-409c1bb66114/business_idea/...`

those directories should be removed when that workflow run is deleted.

## Expected Behavior
- Deleting one or more workflow runs should also delete all related local artifact directories for those run ids.
- The cleanup should only remove artifacts that belong to the deleted workflow runs.
- Unrelated workflow run artifacts must remain intact.

## Root Cause
The current workflow run deletion flow only deletes database records.

There is no local-runner cleanup step that removes artifact directories by `workflowRunId`.

## Fix Direction
- Add a local-runner capability to delete artifacts by workflow run id.
- Invoke that cleanup from the workflow run deletion flow.
- Keep the deletion scoped to the targeted run ids only.

## Acceptance Criteria
- Deleting a workflow run removes its related local artifact directories under `.flowpilot/artifacts/...`.
- Deleting multiple workflow runs removes artifacts for all selected run ids.
- Artifacts for non-deleted workflow runs are preserved.
- Database deletion and local artifact cleanup stay aligned for the same run ids.
