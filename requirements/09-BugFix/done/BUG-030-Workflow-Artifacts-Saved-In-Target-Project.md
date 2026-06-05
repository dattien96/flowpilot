## BUG-030

### Problem

Workflow-step artifacts under `.flowpilot/artifacts/...` were being written relative to the bound target project directory instead of the FlowPilot workspace.

This caused different behavior across projects:

- If the bound project path was the FlowPilot repo, artifacts looked correct.
- If the bound project path was an external Android repo, artifacts were created in that Android repo's hidden `.flowpilot` folder instead of the FlowPilot workspace.

### Expected Rule

- Paths under `.flowpilot/...` must be treated as FlowPilot-managed artifact storage.
- Those artifacts must be saved in the FlowPilot workspace, not in the target project repository.
- Non-`.flowpilot` relative output paths should continue to resolve against the target project working directory.

### Root Cause

There were two issues in the runtime path resolution:

- Workflow runtime used the execution `workingDirectory` for relative artifact paths, so `.flowpilot/...` outputs were created inside the bound target project.
- A follow-up fix still used the admin-web server `process.cwd()` instead of the local runner workspace root, so artifacts could be written to a path that the runner did not treat as its artifact source of truth.

The final source of truth for artifact storage must be the local runner workspace `cwd`, not the target project directory and not the admin-web server process directory.

### Fix

- Reroute `.flowpilot/...` artifact paths to the FlowPilot workspace root.
- Resolve that workspace root from the local runner health `cwd`.
- Keep normal project-relative output paths unchanged.
- Update fallback artifact snapshot storage to use the FlowPilot workspace root as well.
- Normalize `./.flowpilot/...` path variants so they also resolve to FlowPilot storage.
- Cover the runner-root and path-variant behavior with tests.

### Resolution

- AI execution still runs in the target project directory.
- Workflow artifacts now save under the local runner workspace root:
  `.flowpilot/artifacts/<projectId>/<workflowRunId>/...`
- The issue was verified against the live runner health path and targeted workflow runtime tests.

### Done When

- Single-step and workflow-run artifacts no longer appear in the external project’s `.flowpilot` folder.
- Artifacts appear under the FlowPilot workspace `.flowpilot/artifacts/...`.
- Existing non-`.flowpilot` output path behavior still works.
