# Task-353: Standalone TUI Autonomous Onboarding and Modals

## Metadata

- Document ID: `Task-353`
- Title: `Standalone TUI Autonomous Onboarding and Modals`
- Feature Keys: `cli-tui`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot Core Team`
- Reviewers: `DatNguyen`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-56](../../07-Coding-Plan/CP-56-Terminal-TUI-Client.md)
- Related Documents: [CA-867](../../../change-audit/CA-867-Standalone-TUI-Onboarding-And-Modals.md)
- Tags: `cli-tui, onboarding, modals, standalone`

## AI Quick View

### Summary
- Make FlowPilot Terminal TUI (`flowpilot chat`) fully autonomous without requiring the Electron Desktop App for initial onboarding.
- If unauthenticated / session missing on startup, automatically present an interactive In-TUI Login Modal (email, masked password, tab navigation, submit/cancel).
- If Supabase is unconfigured, automatically present an interactive In-TUI Supabase Setup Wizard (API URL, anon key, service key) mirroring the Desktop setup screen.
- When opening a project folder not yet in the catalog or via `/project add`, present an interactive Project Onboarding Wizard with auto-detected platform (`golang`, `android`, `nextjs`, `reactjs`, `python`, `rust`, `node`), tab cycling, and instant creation via `POST /client/projects` to unlock chat immediately.

### Key Decisions
- `D-1` **Interactive In-TUI Modals**: `login_modal.go`, `supabase_setup_modal.go`, and `project_wizard.go` render as centered lipgloss modal blocks within `tuiChrome`, taking key precedence and budgeting modal height so the terminal frame never overflows.
- `D-2` **First-Load Onboarding Sequence**: On cold start, if Supabase is unconfigured, trigger Supabase Setup modal; else if unauthenticated, trigger Login modal; else if project path is unbound, trigger Project Wizard.
- `D-3` **Slash Command Accessibility**: Provide `/login` (interactive modal or CLI args), `/setup` and `/supabase` (open Supabase modal), and `/project` / `/project add` (open project wizard).
- `D-4` **Backend Project Creation**: Add `CreateProject` to runner's `CatalogStore` (`supabase_catalog_store.go` and `interactive_catalog.go`), exposed via `POST /client/projects` and called by TUI client.

### Constraints
- Must maintain 100% backward compatibility with existing tests (`TestLoginSlash_StartsEmailPhase`, etc.).
- Never clobber existing Desktop session files; write session parity to Desktop's expected location.

## 1. Goal

Allow any user to download/install the standalone FlowPilot CLI on Windows/macOS/Linux and immediately set up Supabase credentials, sign in, onboard/bind their current repository, and start chatting without opening or installing the Desktop Electron app.

## 2. Parent Links

- coding plan: `CP-56-Terminal-TUI-Client.md`
- feature key: `cli-tui`

## 3. Trigger

Users running `flowpilot chat` on fresh machines or on new repositories were blocked with "project catalog empty" or "no Desktop session file found" because Supabase configuration and project binding previously required opening the Electron Desktop App.

## 4. Exact Change

- `T-1` **Runner Backend**: Added `CreateProject(ctx, input)` to `supabase_catalog_store.go` (inserts project into Supabase `projects` and creates binding in `project_workspace_bindings`) and `interactive_catalog.go`. Added handler for `POST /client/projects` in `interactive_handlers.go`.
- `T-2` **Global Supabase Config Fallback**: Runner loads and saves Supabase configuration in `~/.flowpilot/settings/supabase-config.json` in addition to workspace settings.
- `T-3` **TUI Client API**: Added `CreateProject` and `SaveSupabaseConfig` methods to `internal/tui/client`.
- `T-4` **Interactive Modals**:
  - `project_wizard.go`: Platform detection, Tab/Shift-Tab navigation, Left/Right platform cycling, name/model editing, and `POST /client/projects` binding.
  - `login_modal.go`: Email and password fields with input masking and validation.
  - `supabase_setup_modal.go`: API URL, anon key, and service key fields with URL validation.
- `T-5` **TUI Chrome Integration**: `mouse.go` and `app.go` measure `modalH` and render `c.modalBlock`, routing keyboard events to the active modal.
- `T-6` **Slash Commands**: Registered `/project`, `/setup`, `/supabase`, and updated `/login` to open the modal on 0 args.

## 5. Touched Areas

- `apps/local-runner/internal/runner/supabase_config.go`
- `apps/local-runner/internal/runner/supabase_catalog_store.go`
- `apps/local-runner/internal/runner/interactive_catalog.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/tui/client/auth.go`
- `apps/local-runner/internal/tui/client/client.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/mouse.go`
- `apps/local-runner/internal/tui/app/chat_textarea.go`
- `apps/local-runner/internal/tui/app/project_wizard.go`
- `apps/local-runner/internal/tui/app/login_modal.go`
- `apps/local-runner/internal/tui/app/supabase_setup_modal.go`
- `apps/local-runner/internal/tui/app/standalone_onboarding_test.go`
- `apps/local-runner/internal/runner/interactive_catalog_test.go`

## 6. Acceptance Check

- [x] AC-1: Runner provides `POST /client/projects` to create and bind projects.
- [x] AC-2: `detectProjectPlatform` identifies `golang`, `android`, `nextjs`, `reactjs`, `python`, `rust`.
- [x] AC-3: `project_wizard.go` navigates via Tab/Shift-Tab, cycles platforms with Left/Right/Space, and renders inside height budget.
- [x] AC-4: `login_modal.go` prompts for email and masked password and validates input.
- [x] AC-5: `supabase_setup_modal.go` validates URL prefix and required anon key.
- [x] AC-6: All unit tests pass (`go test -v ./internal/tui/app -run "TestProject|TestLogin|TestSupabaseSetup"`).
- [x] AC-7: Pre-existing tests (`TestLoginSlash_StartsEmailPhase`, etc.) remain green.

## 7. Out of Scope

- Cloud sync of provider keys (remains local to PC runner).
- Multi-user team workspace permissions (handled via Supabase RLS).

## 8. Completion Notes

- All changes verified and tested with Go unit tests.
- Standalone TUI operates completely autonomously without the desktop app.
