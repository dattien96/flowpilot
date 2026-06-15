# Task-038 Desktop Provider Settings Connect Account

## Metadata

- Document ID: `Task-038`
- Title: `Desktop Provider Settings Connect Account`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Migrate-Web-To-Desktop.md`, `requirements/08-Task/done/Task-036-Desktop-Provider-Accounts-Sidebar.md`
- Child Documents: ``
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `requirements/08-Task/todo/Task-027-Ai-Provider-Config-Mirror.md`, `change-audit/CA-064-desktop-provider-settings-connect-account.md`
- Replaces: ``
- Tags: `desktop-flowpilot`, `provider-accounts`, `settings`, `desktop-migration`

## AI Quick View

### Summary

- Added the missing desktop `Connect New Account` action for each provider in the AI Providers settings screen.
- Reused the existing local-runner `/provider-accounts/connect` flow instead of introducing a desktop-only path.
- The settings screen now shows per-provider account counts and auto-refreshes after a new account is detected.

### Current Ask

- Fix review item 03 from the desktop migration parity review by restoring provider-account connection from desktop settings.

### Key Decisions

- `T-1` Extend the desktop runner contract with `connectProviderAccount` and map it directly to the runner HTTP endpoint.
- `T-2` Keep the change inside the existing `AI Providers` desktop screen instead of adding a separate desktop accounts page.
- `T-3` Limit this slice to connect flow visibility and refresh behavior; account config mirroring remains tracked separately.

### Constraints

- Keep the implementation aligned with the existing Admin Web `/settings/accounts` behavior.
- Do not expand this slice into full account management parity beyond the missing connect action.
- GitNexus impact tooling was not available in this thread, so the code path was inspected locally and kept narrow.

### Open Questions

- Should the desktop also expose verify/delete actions in the settings screen, or should those remain in a later parity slice?

### Source Refs

- `R3-Phase1 review item 03`
- `Task-036`
- `Task-027`

## 1. Goal

Restore the missing provider-account connection action in the desktop settings UI so each provider row can launch the same new-account auth flow that already exists in Admin Web.

## 2. Parent Links

- coding plan: `none in requirements/07-Coding-Plan for this parity fix; closest execution parent is requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Migrate-Web-To-Desktop.md`
- tech design: `none explicitly mapped for this narrow parity fix`
- system spec: `none explicitly mapped for this narrow parity fix`
- specific upstream ids: `R3 Phase 1 review item 03`, `Task-036:T-3`

## 3. Trigger

`requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md` item 03 reported that the desktop `Provider setting` area was missing `Connect new acc for each provider`, with Admin Web `/settings/accounts` called out as the reference behavior.

## 4. Exact Change

- `T-1` Added `connectProviderAccount(providerKey)` to the desktop runner contract and implemented it in both the HTTP and mock runner clients.
- `T-2` Updated `AiProvidersSettings` to show provider account counts plus a `Connect New Account` action next to the existing auth action for each provider.
- `T-3` Added desktop-side refresh polling so the settings screen detects and reflects a newly created provider account after the login terminal flow starts.
- `T-4` Added small settings layout styles for multi-line provider summaries and per-provider action groups.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/types/contract.ts`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`, `desktop runner client contract`
- routes: `/provider-accounts/connect`
- tables:

## 6. Acceptance Check

- Desktop `AI Providers` settings shows `Connect New Account` for each provider row.
- Clicking the action calls the existing runner connect flow and surfaces login progress feedback in the desktop UI.
- The settings screen refreshes provider accounts and shows the newly detected account summary after connection starts.
- `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `npm run build` passes in `apps/desktop-flowpilot`.

## 7. Out of Scope

- Full desktop parity for verify, delete, or active-account management in the settings screen.
- Config mirroring into newly created provider account homes; that remains tracked by `Task-027`.
- Editing the review-capture markdown itself beyond implementing the referenced item.

## 8. Completion Notes

- result: Implemented and verified locally with desktop typecheck and build.
- follow-ups: Revisit `Task-027` so newly connected accounts inherit required provider config automatically where product expects it.
- upstream docs updated: `Task-038`, `CA-064`
