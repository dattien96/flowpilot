# Task-036 Desktop Provider Accounts Sidebar

## Metadata

- Document ID: `Task-036`
- Title: `Desktop Provider Accounts Sidebar`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-12`
- Last Updated: `2026-06-12`
- Parent Documents: `Task-026-Justfile-Command-Start-Ai-Terminal`, `Task-034-View-Acc-Limit`
- Child Documents: ``
- Related Documents: `BUG-042-Desktop-Runner-Mode-Project-List-Fetch-Blocked`, `BUG-043-Desktop-Workflow-Catalog-Uses-Project-Scoped-Runtime-Rows`, `BUG-044-Desktop-Step-Catalog-And-Launch-Mode-Are-Workflow-Scoped`
- Replaces: ``
- Tags: `desktop-flowpilot`, `provider-accounts`, `local-runner`, `ui`

## AI Quick View

### Summary

- Added a provider-accounts section to the desktop sidebar above `SYSTEM`.
- The desktop now loads enriched provider account summaries from the local runner, including compact quota bars and modal detail fields.
- Each provider modal supports reviewing all accounts, switching the active connected account, and opening a terminal bound to that account.

### Current Ask

- Surface active provider accounts and remaining limits directly inside the desktop runner UI.

### Key Decisions

- `T-1` Keep `ListProviderAccounts` and `TestProviderAccount` behavior unchanged; add a separate enriched desktop summary route instead.
- `T-2` Reuse the existing CLI account-metadata logic so desktop and terminal views stay aligned.
- `T-3` Pin only providers that currently have at least one connected account; the modal shows the broader provider account list.

### Constraints

- Do not fork a second account metadata model in the desktop app.
- Keep the existing terminal-launch flow compatible with `just runner-provider-terminal`.

### Open Questions

- None for this slice.

### Source Refs

- `Task-026`
- `Task-034`

## 1. Goal

Show valid provider accounts in the desktop sidebar with quick usage visibility, and allow operators to inspect account details or open a provider-bound terminal without leaving the desktop app.

## 2. Parent Links

- coding plan:
- tech design:
- system spec:
- specific upstream ids: `Task-026`, `Task-034`

## 3. Trigger

The desktop runner UI exposed workflow and step launch controls, but operators still had to leave the app and use Admin Web or `just runner-provider-terminal` to see account limits, confirm which provider account was active, or open a terminal under a specific account.

## 4. Exact Change

- `T-1` Added `/client/provider-accounts` to the local runner server with enriched account summaries suitable for the desktop sidebar and modal.
- `T-2` Extended desktop runner contracts and clients with provider-account listing, activation, and terminal-launch actions.
- `T-3` Added a sidebar accounts section with pinned provider cards, compact quota bars, and a per-provider modal showing full details and account actions.

## 5. Touched Areas

- files: `apps/local-runner/internal/cli/root.go`, `apps/local-runner/internal/cli/provider_account_api.go`, `apps/local-runner/internal/cli/provider_account_terminal.go`, `apps/local-runner/internal/cli/provider_account_api_test.go`, `apps/desktop-flowpilot/src/types/contract.ts`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `local-runner cli`, `desktop-flowpilot`
- routes: `/client/provider-accounts`, `/provider-accounts/activate`, `/provider-accounts/test`
- tables:

## 6. Acceptance Check

- Desktop sidebar shows connected provider accounts above `SYSTEM`.
- Compact provider cards show display label and available quota bars when usage data exists.
- Clicking a provider card opens a modal with full account details.
- Modal actions can switch the active account and open a terminal for a connected account.
- Targeted Go test, desktop typecheck, and desktop build all pass.

## 7. Out of Scope

- Connecting new accounts from the desktop app.
- Replacing the full Admin Web accounts management page.
- Auto-refreshing quotas on a timer beyond explicit sidebar refreshes triggered by page load or account actions.

## 8. Completion Notes

- result: Implemented and verified locally.
- follow-ups: Consider adding connect / verify actions to the same desktop modal if operators want full in-app account management.
- upstream docs updated: `Task-036`
