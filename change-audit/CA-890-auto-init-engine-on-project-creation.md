# CA-890 — Auto-Init Engine on Project Creation (Desktop & TUI)

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-377
change_type: feature
summary: Auto-init project engine (scaffold requirements, install platform skills, gate config, git hook) on project creation for both Desktop App API and TUI wizard
# --->8---

## Why

When creating a new project in an empty folder, users previously had to manually trigger `/init` (or wait until binding an existing project) to scaffold the `requirements/` directory structure, copy platform-specific skills, generate `.flowpilot/settings/gate-config.json`, and set up git hooks. Without this automatic step:
- Starting a conversation or vibe flow right after creation lacked platform skills and gate configurations.
- Users had to memorize and run the `/init` slash command manually.

## Change

1. **API / Runner Layer (`apps/local-runner/internal/runner/interactive_handlers.go`)**:
   - In `handleCreateProject` (`POST /client/projects`), after successfully writing the project record, if `input.DirectoryPath` resolves to a local directory on the runner host, automatically execute `s.runEngineInit(project.ID, dir, input.Platform, engineInitTriggerBind, "all")`.
   - This automatically covers the Desktop App (which invokes `POST /client/projects` during project creation in Settings / Projects) as well as direct API consumers.

2. **TUI Application Layer (`apps/local-runner/internal/tui/app/app.go`)**:
   - In `ProjectCreatedMsg` (when the onboarding wizard completes project creation), dispatch `m.cmdInitEngine("all")` along with LSP status fetch so the terminal displays real-time initialization results.

3. **Additive Tests (`apps/local-runner/internal/runner/project_auto_init_test.go`)**:
   - Added `TestCreateProject_AutoInitsEngine` verifying that calling `handleCreateProject` automatically creates `requirements/01-PRD`, `.flowpilot/engine-init.json`, and registers skills.

## Impact

- **Idempotency**: Engine initialization uses `.flowpilot/engine-init.json` state tracking; repeated or subsequent manual `/init` executions will not duplicate work or corrupt files.
- **Manual Command**: Slash command `/init` (and `/init skill`, `/init all`) remains fully intact and available for manual re-runs.
- **Risk**: Low. Scaffolding is only executed when the local directory path exists and resolves cleanly.

## Verification

- `go test ./internal/runner -run TestCreateProject_AutoInitsEngine` -> PASS (1.22s)
- `go test ./internal/tui/app -run "TestProjectWizard|TestProjectPlatform"` -> PASS (2.17s)
- `go test ./internal/runner -run "TestHandleCreateProject|TestCreateProject"` -> PASS (0.68s)
- No regression on existing tests.
