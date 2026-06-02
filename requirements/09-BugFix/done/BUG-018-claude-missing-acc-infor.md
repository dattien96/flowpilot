# BUG-018 Claude account card missing plan and usage state

Symptom:
The Claude account card rendered only the email and auth-store rows after connecting an account. The `Plan` line and any usage-state detail were missing.

Root cause:
The Claude account metadata reader was still looking at stale auth fields. It expected `oauthAccount.organizationBillingType`, but current Claude auth data stores the plan under `oauthAccount.billingType` and exposes the workspace subscription through `claude auth status --json` as `subscriptionType`. The old code also only checked `hasExtraUsageEnabled`, so it missed the current disabled-state signal in `cachedExtraUsageDisabledReason`.

What we changed:
- Read the local auth payload from the first existing Claude auth file:
  - `%HOME%\\.claude.json`
  - `%HOME%\\claude\\auth.json`
  - `%HOME%\\.config\\claude\\auth.json`
- Execute `claude auth status --json` with `HOME` and `USERPROFILE` pointed at the selected account home so the CLI resolves the correct Claude account.
- Prefer `subscriptionType` from the CLI response for the human-readable plan.
- Fall back to `oauthAccount.organizationBillingType` or `oauthAccount.billingType` from the auth file when CLI status is unavailable.
- Read `oauthAccount.subscriptionCreatedAt` to append the plan start date.
- Read `cachedExtraUsageDisabledReason` to surface disabled extra-usage states such as `out_of_credits`.
- Keep `oauthAccount.hasExtraUsageEnabled` as the positive-state fallback when no disabled reason is present.
- Prefer the CLI email from `claude auth status --json`, then fall back to `oauthAccount.emailAddress`.

Current limitation:
Claude local auth and CLI status do not expose Codex/Gemini-style numeric quota windows, so this fix restores the `Plan` summary and extra-usage state only. It does not synthesize fake remaining percentages.

Verification:
- Added focused unit coverage for the Claude usage-summary helper.
- Confirmed `npm test -- src/app/api/local-runner/provider-accounts/_account-metadata.test.ts` passes.
