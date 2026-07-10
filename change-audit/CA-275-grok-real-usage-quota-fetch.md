# CA-275 — Grok Account Card Now Shows Real Usage/Quota

## Scope

Implemented [Task-216](../requirements/08-Task/done/Task-216-Grok-Real-Usage-Quota-Fetch.md): `Task-210`'s `T-5`/`Q-1` (mirrored by `CP-46`'s `Q-5`) had concluded Grok exposes no machine-readable quota/usage endpoint, based only on the ACP (`grok agent stdio`) JSON-RPC stream. Direct investigation of the installed `grok` binary (a `/usage` TUI slash-command, a `BillingConfigResponse` schema, a `GROK_CLI_CHAT_PROXY_BASE_URL` env var) and a live authenticated probe against the connected account confirmed a real endpoint exists: `GET https://cli-chat-proxy.grok.com/v1/billing`, using the same cached OAuth bearer token already read from `~/.grok/auth.json`.

## Changes

- `apps/local-runner/internal/cli/provider_account_terminal.go`: `grokAuthEntry` gains an additive `key` field (the cached bearer JWT); `loadGrokAccountMetadata` now calls new `loadGrokQuota`/`grokQuotaFromBilling`, which hits the confirmed billing endpoint and maps `monthlyLimit`/`weeklyLimit` + `used` + `billingPeriodEnd` into one `usageDetailLine` (`"Team Credits (Monthly)"` or `"Team Credits (Weekly)"`, branching on whichever limit field the account's plan returns). Email/team-summary logic is unchanged. **Label note:** a post-ship comparison against the real `grok` CLI's own status output (`"Weekly limit: 1%"`, resets on a different date) showed this endpoint only reports the team-credit pool, a separate metric from the SuperGrok personal weekly allowance — the label was named `"Team Credits (...)"` specifically to avoid implying otherwise; finding that second metric is tracked in [Task-217](../requirements/08-Task/todo/Task-217-Grok-SuperGrok-Weekly-Included-Usage-Fetch.md).
- `apps/admin-web/src/app/api/local-runner/provider-accounts/_account-metadata.ts`: mirrored the same fetch as `grokMetadata`/`loadGrokQuota`/`grokQuotaFromBilling`; `ProviderKey` gains `"grok"` and `metadataForAccount` gains the matching case (previously this file had zero Grok handling at all).
- `ProviderAccountsPanel.tsx` needed no change — it already renders any provider's `usageDetailLines` generically as a % meter.
- `Task-210` `Q-1` and `CP-46` `Q-5` annotated in place with the corrected finding, linking to `Task-216`.
- Tests: Go — `TestGrokQuotaFromBillingMonthly`, `TestGrokQuotaFromBillingWeeklyFallback`, `TestGrokQuotaFromBillingMissingLimitReturnsNil`, `TestLoadGrokQuotaEmptyTokenReturnsNil` in `provider_account_terminal_test.go`, using the exact JSON shape captured live. TS — `grokQuotaFromBilling` describe block in `_account-metadata.test.ts` (monthly mapping, weekly fallback, null-on-missing-limit).

## Verification

- `go build ./...` — clean; `go vet ./internal/cli/...` — clean; `go test ./internal/cli/...` — 10 passed (4 new, 6 pre-existing Codex/Claude unchanged).
- `npx vitest run src/app/api/local-runner/provider-accounts/_account-metadata.test.ts` (admin-web) — 7 passed (3 new, 4 pre-existing Gemini/Claude unchanged).
- `npx tsc --noEmit` (admin-web) — no new errors from `_account-metadata.ts`/`.test.ts` (pre-existing unrelated errors in `src/routes/guide.tsx` untouched).
- Endpoint confirmed live: extracted the bearer token from `~/.grok/auth.json` into a scratch temp file, issued an authenticated `GET` against `cli-chat-proxy.grok.com/v1/billing`, got a real `200` JSON billing body back, then deleted the temp file — the token itself was never logged.
- Not verified: a real weekly/free-tier account's response shape (only a monthly-tier team account was available) — tracked as a follow-up in `Task-216`.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-216
change_type: feature
summary: Grok account card now fetches and displays real usage/quota (Remaining Monthly/Weekly %) via the confirmed cli-chat-proxy.grok.com/v1/billing endpoint, correcting Task-210/CP-46's earlier "no machine-readable endpoint" finding
# --->8---
