# CA-139: Antigravity CLI Provider Detection

## Summary

Updated the AI provider inventory so the existing `gemini` provider slot now checks and installs the supported Antigravity CLI (`agy`) instead of the retired Gemini CLI installer flow.

## What Changed

- Updated the local-runner provider spec for `gemini` to display as `Antigravity CLI`, detect the `agy` binary on `PATH`, and show an Antigravity-specific install hint.
- Replaced the old Gemini npm install command with the current Antigravity CLI install scripts:
  - macOS/Linux: `curl -fsSL https://antigravity.google/cli/install.sh | bash`
  - Windows CMD: `curl -fsSL https://antigravity.google/cli/install.cmd -o install.cmd && install.cmd && del install.cmd`
- Added a lightweight readiness fallback for Gemini provider auth status by treating `~/.gemini/antigravity-cli/settings.json` or `keybindings.json` as evidence that Antigravity CLI has been initialized locally.
- Updated the admin settings empty-state copy to mention `Antigravity CLI` instead of `Gemini`.
- Extended the desktop store to load local provider inventory alongside provider accounts so chat-provider gating can see both install state and account state.
- Updated the desktop chat provider picker to disable selection unless the provider CLI is installed and a connected account is currently active for that provider.
- Added a dedicated `Install AGY CLI` button in the desktop AI Providers settings for the `gemini` slot, backed by the runner install endpoint.
- Removed the separate provider `Auth` buttons from desktop and admin-web settings so account connection now flows through `Connect New Account` only.
- Updated interactive provider login terminals to start from the FlowPilot workspace before launching the provider CLI.
- Replaced the retired Gemini 2.5 / preview model inventory with the current Antigravity Gemini lineup:
  - `gemini-3.5-flash-medium`
  - `gemini-3.5-flash-high`
  - `gemini-3.5-flash-low`
  - `gemini-3.1-pro-low`
  - `gemini-3.1-pro-high`
- Added legacy Gemini model alias normalization so old saved values like `gemini-flash`, `gemini-pro`, `auto-gemini-*`, and the older preview / 2.5 ids map onto the new Antigravity-backed model ids.
- Updated admin-web workflow model options, demo data, runner defaults, summarizer defaults, and Gemini adapter tests to use the new canonical Gemini model ids.
- Added a Supabase migration that rewrites existing stored Gemini model selections in `projects`, `workflows`, `workflow_steps`, and `step_definitions`, refreshes seeded `ai_supported_models` Gemini rows, and redefines the step-definition supported-model constraint for the new lineup while keeping legacy aliases accepted during transition.
- Added runner regression coverage for the new install-command matrix and the Antigravity config-marker readiness path.

## Verification

- `go test ./internal/runner -run 'TestProviderInstallCommandMatrix|TestProviderAuthStatusGeminiUsesAntigravityConfigMarker' -count=1`
- `go test ./internal/runner -run 'TestProviderInstallCommandMatrix|TestProviderAuthStatusGeminiUsesAntigravityConfigMarker|TestStartInteractiveAuthLaunchesFromWorkspace' -count=1`
- `go test ./internal/runner -run 'TestProviderKeyFromModel|TestGemini|TestStartSessionGeminiACP|TestResolvePromptExecutionAdapter|TestProviderInstallCommandMatrix|TestProviderAuthStatusGeminiUsesAntigravityConfigMarker|TestStartInteractiveAuthLaunchesFromWorkspace|TestCreateLocalProviderInventoryHandler' -count=1`
- `npm run test -- src/data/repository/supabase/workflow-engine-mappers.test.ts src/features/workflow-engine/workflow-start-runtime.test.ts src/app/api/local-runner/provider-accounts/_account-metadata.test.ts` in `apps/admin-web`
- `npm run typecheck` in `apps/desktop-flowpilot`

## Notes

- This change is intentionally scoped to the AI Providers inventory/install/auth surface plus Gemini model registry/migration updates required to keep Antigravity model selection coherent.
- The Antigravity public docs clearly expose the current model lineup, but they do not expose a stable CLI `--model` id table in the fetched docs HTML; the new canonical ids were derived from the visible selector names in kebab-case form so the codebase can move off the retired Gemini ids consistently.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-167
change_type: feature
summary: Switch Gemini provider inventory and model registry to Antigravity CLI
# --->8---
