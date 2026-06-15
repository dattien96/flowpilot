# Task-039 Desktop Provider Modal Connect Account

## Metadata

- Document ID: `Task-039`
- Title: `Desktop Provider Modal Connect Account`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Migrate-Web-To-Desktop.md`, `requirements/08-Task/done/Task-036-Desktop-Provider-Accounts-Sidebar.md`, `requirements/08-Task/done/Task-038-Desktop-Provider-Settings-Connect-Account.md`
- Child Documents: ``
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `requirements/08-Task/todo/Task-027-Ai-Provider-Config-Mirror.md`, `change-audit/CA-065-desktop-provider-modal-connect-account.md`
- Replaces: ``
- Tags: `desktop-flowpilot`, `provider-accounts`, `dashboard`, `desktop-migration`

## AI Quick View

### Summary

- Added `Connect New Account` to the provider-accounts dashboard modal opened from the `All` button.
- Reused the same desktop runner connect flow already added for the settings screen.
- The modal now refreshes automatically when a newly connected account appears for that provider.

### Current Ask

- Extend the desktop account dashboard modal so users can add a new account from the modal header instead of leaving that context.

### Key Decisions

- `T-1` Keep the action in the modal header for the currently opened provider only.
- `T-2` Reuse the existing `connectProviderAccount` contract instead of adding a modal-specific API path.
- `T-3` Keep the refresh behavior local to the modal so the account list updates in place after login starts.

### Constraints

- Keep the scope limited to the dashboard modal opened from the provider `All` button.
- Do not expand this slice into full account CRUD parity.
- GitNexus impact tooling was not available in this thread, so the code path was inspected locally and kept narrow.

### Open Questions

- Should the sidebar modal also expose verify/delete in a future parity pass, or remain a quick-access surface only?

### Source Refs

- `Task-036`
- `Task-038`
- `R3-Phase1 review item 03`

## 1. Goal

Let users start the new-account flow directly from the provider account modal in the desktop dashboard so they do not need to switch to the settings page for the same action.

## 2. Parent Links

- coding plan: `none in requirements/07-Coding-Plan for this narrow parity follow-up; closest execution parent is requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Migrate-Web-To-Desktop.md`
- tech design: `none explicitly mapped for this narrow parity follow-up`
- system spec: `none explicitly mapped for this narrow parity follow-up`
- specific upstream ids: `Task-036:T-3`, `Task-038:T-1`, `R3 Phase 1 review item 03`

## 3. Trigger

After the settings-page connect flow was restored, the remaining usability gap was the dashboard account modal opened from the provider `All` button. That modal already lists all accounts for the provider, so it is a natural second place to expose `Connect New Account`.

## 4. Exact Change

- `T-1` Added a modal-header `Connect New Account` action to `ProviderAccountsPanel` for the active provider group.
- `T-2` Reused the existing `connectProviderAccount(providerKey)` runner contract in the dashboard modal.
- `T-3` Added local refresh polling and success feedback so the modal updates automatically when a new provider account is detected.
- `T-4` Added small modal-header action layout styling to keep the new button aligned with the existing close action.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`, `dashboard account modal`
- routes:
- tables:

## 6. Acceptance Check

- Opening the provider modal from the dashboard `All` button shows `Connect New Account` in the top-right header area.
- Clicking the action starts the existing runner login flow for that provider.
- The modal refreshes the provider account list and shows feedback when the new account is detected.
- `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `npm run build` passes in `apps/desktop-flowpilot`.

## 7. Out of Scope

- Full account CRUD or verification parity inside the dashboard modal.
- Config mirroring for the newly created provider account home; that remains tracked by `Task-027`.
- Changes to the settings-page implementation beyond the already completed `Task-038` slice.

## 8. Completion Notes

- result: Implemented and verified locally with desktop typecheck and build.
- follow-ups: Revisit whether quick-access dashboard surfaces should expose more account-management actions or remain focused on activation and terminal launch.
- upstream docs updated: `Task-039`, `CA-065`
