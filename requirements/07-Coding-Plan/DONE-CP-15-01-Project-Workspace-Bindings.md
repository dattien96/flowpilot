# CP-15-01: Project Workspace Bindings

## 1. Goal

Support one shared project across many devices by storing many local workspace paths per project instead of relying on one global `project_path`.

## 2. Problem

Today, a project may store one shared path such as:

```text
C:/xxx
```

That breaks when the same user logs in on another machine where the real path is different, for example:

```text
/Users/name/yyy
```

The user should not need to create a duplicate project just because the local path is different on another device.

## 3. Target Model

Shared project:

- one project record
- shared metadata
- shared workflows
- shared workflow runs and artifacts metadata

Local workspace bindings:

- one project can have many local paths
- each binding is just a known candidate local path for that project
- path validation happens only when workflow execution starts

Recommended table:

`project_workspace_bindings`

Fields:

- `id`
- `project_id`
- `local_path`
- `label` optional
- `created_at`
- `updated_at`

Suggested uniqueness:

- unique `(project_id, local_path)`

## 4. Scope

This CP covers:

- schema for workspace bindings
- domain model and gateway support
- migration away from single-path assumptions

This CP does not cover:

- the project detail launch UX itself
- the full Go-runner workflow execution flow
- step-definition runtime-field editing

## 5. Required Changes

### 5.1 Database

1. create `project_workspace_bindings`
2. add indexes for:
   - `project_id`
   - `local_path`
   - `(project_id, local_path)`
3. add RLS rules so the owning user/team can manage bindings
4. keep `projects.project_path` only as temporary compatibility if needed

### 5.2 Migration Strategy

1. existing projects with `project_path` should be migrated into an initial binding when possible
2. do not hard-delete `projects.project_path` in the first migration if current code still reads it
3. mark `projects.project_path` as deprecated in domain comments and follow-up cleanup plan

### 5.3 Binding Resolution Responsibility

The system does not need `device_id` in this phase.

Instead, the Golang runner is responsible for checking bindings only when a workflow is triggered.

Runtime rule:

1. load all bindings for `project_id`
2. check each `local_path`
3. if exactly one path is usable, run there
4. if multiple paths are usable, the runner may pick the first valid path by deterministic order or follow future selection rules
5. if no path is usable, fail the trigger and return the root cause to the user

### 5.4 Domain and Gateway

Add support for:

- create workspace binding
- update workspace binding
- list bindings for a project
- delete workspace binding

### 5.5 Compatibility Rule

During migration:

1. runner loads project bindings first
2. if no binding exists, optionally fall back to legacy `projects.project_path`
3. once bindings are stable, remove legacy fallback in a later cleanup pass

### 5.6 Project UI Requirement

The project detail page must contain one dedicated tab:

- `Directory Binding`

This tab must:

1. list all bindings for the project
2. allow add new binding
3. allow edit existing binding
4. allow delete binding

## 6. Implementation Checklist

### 6.1 Database

- [x] create Supabase migration for `project_workspace_bindings`
- [x] add columns:
  - `id`
  - `project_id`
  - `local_path`
  - `label`
  - `created_at`
  - `updated_at`
- [x] add unique constraint for `(project_id, local_path)`
- [x] add indexes for `project_id` and `local_path`
- [x] add RLS policies for read/write/delete by allowed project users
- [x] keep legacy `projects.project_path` unchanged in this phase

### 6.2 Migration

- [x] backfill one initial binding from `projects.project_path` for rows where it exists
- [x] make backfill idempotent so rerun is safe
- [x] document legacy fallback behavior during migration period

### 6.3 Domain Model

- [x] add project workspace binding entity/model
- [x] add create binding payload
- [x] add update binding payload
- [x] add list bindings response model
- [x] update project-related domain contracts to reference bindings where needed

### 6.4 Gateway and Repository

- [x] add `listProjectWorkspaceBindings(projectId)`
- [x] add `createProjectWorkspaceBinding(projectId, payload)`
- [x] add `updateProjectWorkspaceBinding(bindingId, payload)`
- [x] add `deleteProjectWorkspaceBinding(bindingId)`
- [x] implement the above in Supabase repository
- [x] implement the above in demo/in-memory repository if still needed for local/demo mode

### 6.5 Project Create Flow

- [x] update create-project UI to support one or more initial directory bindings
- [x] allow browse/add multiple paths before submit
- [x] allow remove/edit binding rows before submit
- [x] require at least one binding before project creation succeeds
- [x] create project first, then persist all initial bindings

### 6.6 Project Detail UI

- [x] add `Directory Binding` tab on project detail page
- [x] show all bindings for the project
- [x] add create binding action
- [x] add edit binding action
- [x] add delete binding action
- [x] show empty state when no bindings exist

### 6.7 Runner Preparation

- [x] add runner-facing contract to fetch all bindings for one project
- [x] define deterministic path-check order for bindings
- [x] keep actual path validation for CP-15-04 runner execution phase

### 6.8 Testing

- [x] add repository tests for CRUD on project workspace bindings
- [x] add project create UI tests for multiple initial bindings
- [x] add project detail UI tests for directory binding tab
- [x] add migration verification notes or test coverage if available

### 6.9 Manual Verification

- [x] create a project with multiple bindings
- [x] reopen the project and verify all bindings appear in `Directory Binding`
- [x] edit one binding and verify it persists
- [x] delete one binding and verify it is removed
- [x] confirm legacy projects with `project_path` are backfilled into bindings

## 7. Acceptance Criteria

- [x] one project can store multiple local paths across devices
- [x] bindings are unique by `(project_id, local_path)`
- [x] runner can list all bindings for one project
- [x] project detail page has a `Directory Binding` tab
- [x] directory bindings can be added, edited, and deleted
- [x] runner checks bindings only when workflow execution starts
- [x] legacy single-path dependency is reduced to compatibility only

## 8. Risks

### 8.1 Partial migration

Some code may still read `projects.project_path`.

Mitigation:

- keep compatibility for one phase
- document every reader that must be migrated in follow-up CPs

## 9. Exit Condition

This CP is complete when project-local directory bindings exist as a real first-class model and the system no longer assumes one global path per project.
