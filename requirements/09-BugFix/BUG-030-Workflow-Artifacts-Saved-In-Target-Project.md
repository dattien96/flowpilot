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

Workflow runtime path resolution used the execution `workingDirectory` for all relative artifact paths, including `.flowpilot/...` artifact paths.

### Fix

- Reroute `.flowpilot/...` artifact paths to the FlowPilot workspace root.
- Keep normal project-relative output paths unchanged.
- Update fallback artifact snapshot storage to use the FlowPilot workspace root as well.
- Cover both behaviors with tests.

### Done When

- Single-step and workflow-run artifacts no longer appear in the external project’s `.flowpilot` folder.
- Artifacts appear under the FlowPilot workspace `.flowpilot/artifacts/...`.
- Existing non-`.flowpilot` output path behavior still works.
