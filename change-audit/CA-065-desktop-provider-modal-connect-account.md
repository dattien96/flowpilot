# CA-065 Desktop Provider Modal Connect Account

## Scope

Record the follow-up desktop account UX change that adds `Connect New Account` to the provider modal opened from the dashboard `All` button.

## Completed

- Updated `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx` so the active provider modal header now includes a `Connect New Account` action.
- Reused the existing `connectProviderAccount(providerKey)` desktop runner contract instead of creating another connection path.
- Added modal-local refresh polling and feedback so the provider account list updates when a newly connected account appears.
- Added a small header-action layout rule in `apps/desktop-flowpilot/src/styles.css` to keep the new button aligned with the modal close button.

## Verification

- Ran `npm run typecheck` in `apps/desktop-flowpilot`
- Ran `npm run build` in `apps/desktop-flowpilot`
- Build completed successfully; Vite still reports the existing renderer chunk-size warning above 500 kB

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- This change improves the quick-access dashboard modal only; broader account-management parity is still a separate decision.
- Provider config mirroring for newly connected accounts is still tracked by `requirements/08-Task/todo/Task-027-Ai-Provider-Config-Mirror.md`.
