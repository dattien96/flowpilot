# CA-867 — Standalone TUI autonomous onboarding and interactive modals

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-353
change_type: feature
summary: implement standalone autonomous TUI onboarding with interactive login modal, Supabase setup wizard modal, project onboarding wizard with platform autodetection, and POST /client/projects backend binding
# --->8---

## Why
When installing FlowPilot as a standalone CLI on a new machine or opening an uncataloged repository, users were blocked because Supabase credentials and project onboarding were previously exclusive to the Electron Desktop App. The TUI needs to be 100% self-sufficient for setup, authentication, and project onboarding.

## Change
- `apps/local-runner/internal/runner/supabase_config.go`: Added fallback to `~/.flowpilot/settings/supabase-config.json` for global machine-level configuration.
- `apps/local-runner/internal/runner/supabase_catalog_store.go`: Added `ProjectCreatorStore` interface and `CreateProject` implementation creating projects in Supabase and inserting workspace bindings.
- `apps/local-runner/internal/runner/interactive_catalog.go`: Implemented in-memory `CreateProject` for offline runner tests.
- `apps/local-runner/internal/runner/interactive_handlers.go`: Registered `POST /client/projects` endpoint.
- `apps/local-runner/internal/tui/client/`: Added `CreateProject` and `SaveSupabaseConfig` client methods.
- `apps/local-runner/internal/tui/app/project_wizard.go`: Built interactive project onboarding modal with technology autodetection (`golang`, `android`, `nextjs`, `reactjs`, `python`, `rust`, `node`), field navigation, and instant binding.
- `apps/local-runner/internal/tui/app/login_modal.go`: Built in-TUI login modal with email and password input masking.
- `apps/local-runner/internal/tui/app/supabase_setup_modal.go`: Built in-TUI Supabase workspace credentials setup wizard.
- `apps/local-runner/internal/tui/app/app.go`, `mouse.go`, `chat_textarea.go`: Wired modal lifecycle, height budgeting in `tuiChrome`, key routing, slash commands (`/project`, `/setup`, `/supabase`, `/login`), and startup sequence.

## Tests
- Added `standalone_onboarding_test.go`:
  - `TestProjectPlatformDetection`: verifies platform detection for all supported stacks.
  - `TestProjectWizardNavigationAndCycle`: verifies Tab/Shift-Tab field cycling, Left/Right platform option cycling, and rendering.
  - `TestLoginModalNavigationAndValidation`: verifies input, masking, validation, and cancel.
  - `TestSupabaseSetupModalNavigationAndValidation`: verifies URL validation and Anon Key requirement.
- Added `TestInteractiveCatalogCreateProject` in `interactive_catalog_test.go`.
- Pre-existing tests (`TestLoginSlash_StartsEmailPhase`, `TestLoginSlash_ShowsEmailWhenAlreadySignedIn`, `TestLoginSlash_EmailThenPasswordMasked`, etc.) all pass green.

## Providers
Provider agnostic; affects CLI client and runner HTTP server.
