# Task-377: Auto-Init Project Engine On Creation (Desktop & TUI)

## Metadata

- Document ID: `Task-377`
- Title: `Auto-Init Project Engine On Creation (Desktop & TUI)`
- Phase: `task`
- Status: `done`
- Owner: `flowpilot-team`
- Reviewers: `flowpilot-team`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: `CP-44, SD-17`
- Child Documents: None
- Related Documents: `Task-106-Bind-Time-Auto-Init-Orchestration.md`, `Task-112-Req-Scaffold-On-Bind.md`
- Replaces: None
- Tags: `workflow-runtime, context-regression-engine, cli-tui, onboarding, project-creation`

## AI Quick View

### Summary

- Wire automatic engine initialization into `POST /client/projects` (runner backend) and `ProjectCreatedMsg` (TUI app).
- When a project is newly created with a valid local path, auto-scaffold `requirements/`, install platform-specific skills, generate gate config, and install git hooks.
- Keep the manual `/init` command intact and idempotent via `.flowpilot/engine-init.json` state checking.

### Current Ask

- Trigger engine init automatically on project creation from both Desktop App (via runner handler) and TUI Onboarding Wizard, while preserving manual `/init`.

### Key Decisions

- `T-1`: Runner-level trigger in `handleCreateProject` ensures any client creating a project (Desktop App, API, Web) automatically has the engine initialized if the directory exists locally.
- `T-2`: TUI-level dispatch in `ProjectCreatedMsg` fires `cmdInitEngine("all")` so the user gets immediate visual feedback in the terminal status banner.
- `T-3`: Preserve existing `/init` manual command without alterations; repeated executions are idempotent via `engineInitTriggerBind`.

### Constraints

- Do not break existing `/init` command parsing or behavior.
- Do not make breaking edits to existing unit tests.
- Execution must degrade gracefully if the directory is inaccessible or remote.

### Open Questions

- None.

### Source Refs

- `CP-44`, `SD-17 §3.1`, `Task-106`, `Task-112`.

## 1. Goal

Eliminate manual intervention required after creating a new project. When a user creates a project (whether from Desktop App's Settings/Projects page or the TUI Onboarding Wizard), FlowPilot automatically scaffolds the `requirements/` directory structure, copies platform-specific skills (`.agents/skills`), sets up `.flowpilot/settings/gate-config.json`, and installs git post-commit hooks.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`, `SS-18`
- specific upstream ids: `Task-106`, `Task-112`

## 3. Trigger

Users initializing a project in an empty folder previously had to manually run `/init` to generate requirement folders, install skills, and configure gates. This created friction and risked starting conversations without platform context or gates.

## 4. Exact Change

- `T-1`: In `apps/local-runner/internal/runner/interactive_handlers.go` (`handleCreateProject`), call `s.runEngineInit(project.ID, dir, input.Platform, engineInitTriggerBind, "all")` when `input.DirectoryPath` resolves to a local directory.
- `T-2`: In `apps/local-runner/internal/tui/app/app.go` (`ProjectCreatedMsg`), append `m.cmdInitEngine("all")` to the dispatched commands.
- `T-3`: Add unit test `TestCreateProject_AutoInitsEngine` in `apps/local-runner/internal/runner/project_auto_init_test.go`.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/tui/app/app.go`
  - `apps/local-runner/internal/runner/project_auto_init_test.go`
- modules: `flowpilot-runner/internal/runner`, `flowpilot-runner/internal/tui/app`
- routes: `POST /client/projects`
- tables: none

## 6. Acceptance Check

- `POST /client/projects` creates project in database and scaffolds `requirements/` and `.flowpilot/` in target workspace.
- TUI Onboarding wizard dispatches `cmdInitEngine("all")` upon `ProjectCreatedMsg`.
- Manual `/init` remains fully functional and idempotent.
- `go test ./internal/runner -run TestCreateProject_AutoInitsEngine` passes.
- `go test ./internal/tui/app -run "TestProjectWizard|TestProjectPlatform"` passes.

## 7. Out of Scope

- Remote workspace cloning or automated git remote creation.
- Changing the schema of `projects` table in Supabase.

## 8. Completion Notes

- result: Implemented and covered with additive unit tests.
- follow-ups: None.
- upstream docs updated: `CA-890`.
