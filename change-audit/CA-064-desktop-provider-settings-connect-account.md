# CA-064 Desktop Provider Settings Connect Account

## Scope

Record the desktop parity fix for `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md` item 03, which reported that the AI Providers settings screen was missing the `Connect New Account` action that already exists in Admin Web `/settings/accounts`.

## Completed

- Extended the desktop runner client contract with `connectProviderAccount(providerKey)` and wired the HTTP client to the existing local-runner `/provider-accounts/connect` endpoint.
- Added a matching mock-client implementation so desktop mock mode can still exercise the settings screen without breaking the shared contract.
- Updated `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` to:
  - show per-provider account counts and the active account label
  - render `Connect New Account` alongside the existing `Auth` action
  - trigger the existing runner login flow
  - refresh provider-account summaries until a new account is detected
- Added focused CSS for multi-line provider summaries and button groups in the settings list rows.

## Verification

- Ran `npm run typecheck` in `apps/desktop-flowpilot`
- Ran `npm run build` in `apps/desktop-flowpilot`
- Build completed successfully; Vite still reports the existing bundle-size warning for the renderer chunk over 500 kB

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- This slice restores only the missing desktop connect action. Full account-management parity inside settings remains broader than this change.
- Provider config mirroring for newly connected accounts is still tracked separately in `requirements/08-Task/todo/Task-027-Ai-Provider-Config-Mirror.md`.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: TASK-027
change_type: feature
summary: Desktop Provider Settings Connect Account
# --->8---
