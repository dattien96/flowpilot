# CA-868 — Task-353 review fixes: onboarding chain, binding logging, parse errors

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-353
change_type: bugfix
summary: fix Task-353 review findings F-1..F-5 — first-load onboarding chain now reaches the Supabase Setup modal when unconfigured, chain re-arms after login/supabase save, binding failures log instead of silent, empty create-project response yields a clean error, cross-platform wizard test
# --->8---

## Why

Review of commit f2d222a (Task-353) found: (F-1) the first-load onboarding chain never opened the Supabase Setup modal because the offline fake catalog always reports 2 projects, making the `!SupabaseConfigured && len(projects) == 0` guard unreachable — users landed on a Login modal that cannot succeed before Supabase is configured; (F-2) the chain stopped after login/save; (F-3) workspace binding insert failures were silently swallowed while the TUI claimed "created and bound"; (F-5) an empty insert response produced a `%!w(<nil>)` error message; (F-4) the new wizard test hardcoded a Windows path.

## Change

- `apps/local-runner/internal/tui/app/app.go`: SessionDefaultsMsg first-load block now calls `handleOnboardingAfterSession`; `LoginResultMsg` re-arms the project wizard when the path is unbound; `SupabaseConfigSavedMsg` sets `supabaseJustConfigured` to re-arm the chain on the follow-up (non-firstLoad) session load.
- `apps/local-runner/internal/tui/app/model.go`: added `supabaseJustConfigured` field plus `handleOnboardingAfterSession` / `maybeAutoOnboard` helpers. Setup modal opens when supabase is unconfigured and no project is bound (F-1), login when unauthenticated, wizard when the path is unbound — idempotent per modal, never interrupts an established bound session (CA-633 typing contract).
- `apps/local-runner/internal/runner/supabase_catalog_store.go`: split the created-project parse error path (clean "no project row" error when the response is empty — F-5); workspace binding failure now logs `[supabase] project ... created but workspace binding failed ...` instead of being dropped (F-3).
- `apps/local-runner/internal/tui/app/standalone_onboarding_test.go`: `TestProjectWizardNavigationAndCycle` uses `filepath.Join(t.TempDir(), ...)` instead of a hardcoded Windows path (F-4, cross-platform).

## Old-test edits (explicitly approved by operator)

- `ca633_idle_frame_cache_and_sidebar_test.go`: `SessionDefaultsMsg{...}` gains `SupabaseConfigured: true` — the test constructs the message by hand; production `cmdLoadSessionDefaults` always populates this field via `GetSupabaseConfig`. Restores HEAD-parity behavior in every environment; the test's intent (CA-633 composer typing after session ready) is unrelated to onboarding.
- `chat_ux_test.go`: same minimal `SupabaseConfigured: true` addition — restores "chat unlocks after load" intent; this test was already failing at HEAD (Task-353 regression).

## Tests

- Added `standalone_onboarding_chain_test.go` (tui/app):
  - `TestMaybeAutoOnboardSetupTakesPrecedenceOverLogin` — F-1 regression: setup modal opens with fake-catalog projects present.
  - `TestMaybeAutoOnboardSkipsSetupWhenProjectBound` — established session never interrupted.
  - `TestMaybeAutoOnboardLoginWhenUnauthenticated`, `TestMaybeAutoOnboardWizardWhenProjectUnbound`, `TestMaybeAutoOnboardNoopWhenEverythingReady`, `TestMaybeAutoOnboardIdempotentWhenModalAlreadyOpen`.
  - `TestSessionDefaultsFirstLoadOpensSetupWhenUnconfigured` — Update-level wiring for F-1.
  - `TestSessionDefaultsSupabaseJustConfiguredRearmsChain` / `TestSessionDefaultsRefreshDoesNotOpenModals` — F-2 re-arm gate, no modal spam on refresh.
  - `TestLoginResultOpensProjectWizardWhenUnbound` / `TestLoginResultNoWizardWhenProjectBound` — F-2a.
- Added `supabase_catalog_create_test.go` (runner):
  - `TestCreateProjectSuccessAndBindingRequest`, `TestCreateProjectBindingFailureReturnsProjectAndLogs` (F-3), `TestCreateProjectEmptyResponseReturnsCleanError` (F-5), `TestCreateProjectMalformedResponseReturnsParseError`.

## Providers

Provider-agnostic. Evidence: changes touch only `AppModel` (TUI chrome) and `SupabaseCatalogStore.CreateProject` (runner HTTP catalog); no Claude/Codex/Grok adapter code or provider-selection paths involved. Verified via full `internal/tui/app` package run.

## Known issues (pre-existing from Task-353 commit f2d222a, NOT introduced by this fix — verified failing at HEAD with changes stashed)

- `internal/tui/app`: `TestApprovalBarAndStopAreClickable`, `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` — "[stop]" clickable/armed assertions fail; likely a render-path change in f2d222a, requires separate investigation.
- `internal/runner`: `TestSupabaseCatalogStoreShaping` (fails at f2d222a and its parent too), `TestCatalogStoreForFallsBackToFake`.
- `internal/runner` full suite hangs in `grok_process_test.go` (`fakeGrok.serve` waiting on a channel) — environmental/network, file last touched by Task-209, unrelated to this diff.