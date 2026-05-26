# CP-15: Workflow Runtime Migration — Parent Plan

## 1. Purpose

This file is now the parent coding plan for the next workflow-runtime migration work.

The original CP-15 idea was too large for one implementation pass, so it is now split into smaller coding plans that can be delivered one by one.

This parent CP keeps the full-flow direction and defines the implementation order.

## 2. Product Direction

FlowPilot should behave like this:

- one shared `project`
- many local workspace paths for that same project
- bindings are stored per project path, not per device
- workflow execution chooses from known bindings only when the Go runner starts

This means the product should no longer depend on one global shared `projects.project_path` value as the only workspace source of truth.

Instead, the model should move toward:

- `projects`
  - shared metadata only
  - name
  - description
  - repository_url
  - workflow data
- `project_workspace_bindings`
  - `project_id`
  - `local_path`
  - optional `label`

## 3. Runtime Resolution Model

When workflow execution starts:

1. load all bindings for `project_id`
2. check whether any binding path is valid and accessible on the current machine
3. if one usable path is found, run there
4. if no path is usable, fail the trigger and notify the user with the root cause

## 4. Split Plan

This parent plan is split into these child CP files:

1. [CP-15-01-Project-Workspace-Bindings.md](./CP-15-01-Project-Workspace-Bindings.md)
2. [CP-15-02-Project-Launch-And-Workspace-UX.md](./CP-15-02-Project-Launch-And-Workspace-UX.md)
3. [CP-15-03-Step-Definition-Execution-Contract.md](./CP-15-03-Step-Definition-Execution-Contract.md)
4. [CP-15-04-Workflow-Start-And-Runner-Runtime.md](./CP-15-04-Workflow-Start-And-Runner-Runtime.md)

## 5. Delivery Order

Recommended order:

1. `CP-15-01`
   - create the new project directory-binding model first
2. `CP-15-02`
   - expose directory-binding management and launch safety in the UI
3. `CP-15-03`
   - finalize step runtime fields and editing surfaces
4. `CP-15-04`
   - wire start contracts and Go-runner runtime resolution end to end

## 6. Parent Acceptance Criteria

CP-15 as a whole is complete when all child CP files are complete and all of the following are true:

- [ ] one shared project can be reused on multiple devices
- [ ] one project can store many local paths
- [ ] runner checks bindings only when workflow execution starts
- [ ] missing usable path fails the trigger with clear root-cause feedback
- [ ] project detail page exposes directory-binding management instead of forcing duplicate project creation
- [ ] project detail page remains the workflow launch surface
- [ ] step definitions store runtime orchestration fields
- [ ] runner executes using one valid bound local path
- [ ] model family still maps automatically to the correct provider command

## 7. Notes

- This split does not remove the full-flow direction from CP-15. It only makes implementation safer and easier to review.
- If the schema still contains `projects.project_path`, treat it as legacy compatibility during migration until workspace bindings fully replace it.
