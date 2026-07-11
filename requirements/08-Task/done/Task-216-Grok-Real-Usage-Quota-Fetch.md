# Task-216: Grok Real Usage/Quota Fetch — Correct The Task-210 "No Endpoint" Finding

## Metadata

- Document ID: `Task-216`
- Title: `Grok Real Usage/Quota Fetch — Correct The Task-210 "No Endpoint" Finding`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/inprogress/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-080: Claude Account Quota Usage Fetch](../../08-Task/done/Task-080-Claude-Account-Quota-Usage-Fetch.md), [Task-036: Desktop Provider Accounts Sidebar](../../08-Task/done/Task-036-Desktop-Provider-Accounts-Sidebar.md)
- Replaces: `None`
- Tags: `grok, grok-build, xai, quota, usage, billing, provider-accounts, local-runner, desktop-chat`

## AI Quick View

### Summary

- `Task-210` (`T-5`/`Q-1`, mirrored by `CP-46` `Q-5`) concluded Grok exposes **no machine-readable quota/usage endpoint** and deliberately made `loadGrokAccountMetadata` show only email/plan text (`"Team <team_id>"`), never a `usageDetailLines`/% meter — unlike Codex/Claude/Gemini.
- That conclusion was based only on the ACP (`grok agent stdio`) JSON-RPC stream. **Live-verified correction (2026-07-10):** `GET https://cli-chat-proxy.grok.com/v1/billing`, authenticated with the same cached OAuth bearer JWT already read from `~/.grok/auth.json` (the entry's `key` field), returns a real billing/usage JSON body. Confirmed live against the connected account (`chelem_hawk1@hotmail.com`):
  ```json
  {"config":{"monthlyLimit":{"val":15000},"used":{"val":56},"onDemandCap":{"val":0},
    "billingPeriodStart":"2026-07-01T00:00:00+00:00","billingPeriodEnd":"2026-08-01T00:00:00+00:00",
    "history":[{"billingCycle":{"year":2026,"month":6},"includedUsed":{"val":0},"onDemandUsed":{"val":0},"totalUsed":{"val":0}}, ...]}}
  ```
  This account's plan reports a **monthly** cap (`monthlyLimit`); the installed binary's `BillingCycle{WEEKLY,MONTHLY}` enum implies a free-tier account would instead return `weeklyLimit` on the same endpoint — not directly re-verified against a weekly-tier account in this pass, so the Go/TS mapping code branches on whichever limit field is present rather than assuming one.
- Implemented: `loadGrokAccountMetadata` (Go, `provider_account_terminal.go`) and the admin-web `_account-metadata.ts` twin now call this endpoint and populate a `"Team Credits (Monthly)"`/`"Team Credits (Weekly)"` `usageDetailLine`, which `ProviderAccountsPanel.tsx` already renders generically as a % meter — no UI code change was needed.
- `Task-210 Q-1` and `CP-46 Q-5` have been annotated in place with this correction and a link back here.
- **Post-ship correction (2026-07-10):** the user compared this against the real `grok` CLI's own status output and found the CLI shows a *second*, distinct metric — `"Weekly limit: 1%"` / `"Next reset: July 16, 00:31 PT"` — the SuperGrok subscription's personal weekly included-usage allowance, separate from the team-credit pool this task fetches (which reported ~99% remaining, resetting Aug 1, at the same moment). The label was renamed from `"Remaining (Monthly/Weekly)"` to `"Team Credits (Monthly/Weekly)"` to avoid implying this line *is* that weekly limit. Finding and surfacing the actual weekly-allowance source is tracked separately as [Task-217](../todo/Task-217-Grok-SuperGrok-Weekly-Included-Usage-Fetch.md) — ~8 additional candidate endpoints were probed and a direct attempt at the completion endpoint's response headers was blocked by an unidentified client-version check, so it was not resolved in this task.

### Current Ask

- Done: team-credit quota fetch implemented and tested for both the Go CLI metadata builder and the admin-web TypeScript twin, with a label that accurately scopes it to team credits (not the separate SuperGrok weekly allowance — see `Task-217`). No further action required on this task unless a weekly-tier account surfaces a differently-shaped `/billing` response (see Open Questions).

### Key Decisions

- `T-1` (confirmed) The endpoint is `GET {cli-chat-proxy.grok.com}/v1/billing` (host from `GROK_CLI_CHAT_PROXY_BASE_URL`, confirmed via a live authenticated GET during this task), reusing the bearer token already parsed from `~/.grok/auth.json` — no second Grok auth read path was introduced.
- `T-2` Mapping branches on `monthlyLimit` vs `weeklyLimit` (whichever is present) rather than assuming one, since the binary's schema declares both variants and only a monthly-tier account was available to verify live.
- `T-3` Followed `CP-46 P-0` plugin-only discipline: `grokAuthEntry` gained one additive `key` field; `loadGrokAccountMetadata`'s existing email/team logic is unchanged; the admin-web `ProviderKey` union and `metadataForAccount` switch gained one additive `"grok"` case; Codex/Claude/Gemini branches were not touched (Go and TS test suites both green before/after).
- `T-4` `ProviderAccountsPanel.tsx` required no change — it already renders any provider's `usageDetailLines` generically as a % meter.
- `T-5` The pure mapping logic (`grokQuotaFromBilling` in both Go and TS) is split from the HTTP call, mirroring `codexQuotaFromWindow`/`mapGeminiQuotaUsageDetailLines`, so it is unit-tested against the captured fixture without a live network call in CI.

### Constraints

- Did not change `ProviderAccount`, `provider-accounts.json` shape, `ActivateProviderAccount`, `SetActiveAccount`, or `deterministicProviderAccountID`.
- Did not regress Codex/Claude/Gemini quota fetch behavior — both test suites (`go test ./internal/cli/...`, `vitest run _account-metadata.test.ts`) pass unchanged for those providers.
- The bearer token used for live verification was read into a scratch temp file for the duration of the probe and deleted immediately after; it is never logged, and the new code paths only log presence/absence, never the token value or full response body.

### Open Questions

- `Q-1` Whether a free/weekly-tier Grok account's `/v1/billing` response actually uses a `weeklyLimit` field with the same shape was not directly re-verified (only a monthly-tier team account was available). The Go/TS mapping already handles this case defensively (falls back to `weeklyLimit` when `monthlyLimit` is absent), but should be confirmed against a real weekly-tier account if one becomes available.
- `Q-2` Whether polling this endpoint on every account-panel refresh (same cadence as Codex's `wham/usage` call) needs caching/backoff was not assessed — left at parity with the existing Codex/Gemini refresh cadence for this task.

### Source Refs

- `CP-46 Q-5`, `P-14`; `Task-210 T-5`, `Q-1`, `DOD-5`; `Task-080` (Claude quota fetch prior art).

## 1. Goal

Give the Grok account card a real, honest usage/quota display (a % meter like Codex/Claude/Gemini already have) instead of the previous plain `"Team <uuid>"` text, by finding and wiring the actual Grok Build usage API — correcting the earlier "no machine-readable endpoint" research conclusion.

## 2. Parent Links

- coding plan: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- specific upstream ids: `CP-46 Q-5`, `Task-210 T-5`/`Q-1`/`DOD-5`

## 3. Trigger

A user asked why the Grok account card shows no token/usage bar while noting Grok enforces a weekly usage limit. Investigation found the earlier `Q-5`/`Q-1` "no machine-readable endpoint" conclusion in `CP-46`/`Task-210` was based only on the ACP JSON-RPC notification stream and had not inspected the installed `grok` binary or its interactive-TUI-only `/usage` command, which does have a real billing schema and backing API.

## 4. Exact Change

- `T-1` Captured the live billing response by extracting the bearer `key` from `~/.grok/auth.json` and issuing an authenticated `GET https://cli-chat-proxy.grok.com/v1/billing` (confirmed `200 OK` with the JSON shape documented in the Summary above).
- `T-2` Added `loadGrokQuota`/`grokQuotaFromBilling` to `apps/local-runner/internal/cli/provider_account_terminal.go`: reads the bearer token already parsed in `loadGrokAccountMetadata`, calls the confirmed endpoint, and maps `monthlyLimit`/`weeklyLimit` + `used` + `billingPeriodEnd` into one `usageDetailLine` (`"Team Credits (Monthly)"` or `"Team Credits (Weekly)"` — renamed from an initial `"Remaining (...)"` label after the post-ship correction below clarified this is the team-credit pool, not the SuperGrok weekly allowance).
- `T-3` Mirrored `T-2` in `apps/admin-web/src/app/api/local-runner/provider-accounts/_account-metadata.ts`: added `"grok"` to the `ProviderKey` union, added `grokMetadata`/`loadGrokQuota`/`grokQuotaFromBilling` parallel to `codexMetadata`/`claudeMetadata`/`geminiMetadata`, and wired the new case into `metadataForAccount`.
- `T-4` Added Go tests (`TestGrokQuotaFromBillingMonthly`, `TestGrokQuotaFromBillingWeeklyFallback`, `TestGrokQuotaFromBillingMissingLimitReturnsNil`, `TestLoadGrokQuotaEmptyTokenReturnsNil` in `provider_account_terminal_test.go`) and TS tests (`grokQuotaFromBilling` describe block in `_account-metadata.test.ts`) using the captured fixture; both suites pass alongside the pre-existing Codex/Claude/Gemini tests, unchanged.
- `T-5` Annotated `Task-210`'s `Q-1` and `CP-46`'s `Q-5` in place with the corrected finding and a link back to this task.

## 5. Touched Areas

- files: `apps/local-runner/internal/cli/provider_account_terminal.go`, `apps/local-runner/internal/cli/provider_account_terminal_test.go`, `apps/admin-web/src/app/api/local-runner/provider-accounts/_account-metadata.ts`, `apps/admin-web/src/app/api/local-runner/provider-accounts/_account-metadata.test.ts`, `requirements/08-Task/inprogress/Task-210-...md`, `requirements/07-Coding-Plan/done/CP-46-...md`
- modules: provider-accounts metadata enrichment (Go CLI + admin-web API route)
- routes: `apps/admin-web/src/app/api/local-runner/provider-accounts` (Next.js route consuming `_account-metadata.ts`)
- tables: none (no `provider-accounts.json`/Supabase schema change)

## 6. Acceptance Check

- `go build ./...` and `go vet ./internal/cli/...` pass in `apps/local-runner`.
- `go test ./internal/cli/...` passes (10 tests, including the 4 new Grok quota tests) with no changes to existing Codex/Claude/Gemini test outcomes.
- `npx vitest run src/app/api/local-runner/provider-accounts/_account-metadata.test.ts` passes (7 tests, including the 3 new `grokQuotaFromBilling` cases) in `apps/admin-web`.
- `npx tsc --noEmit` in `apps/admin-web` shows no new errors introduced by `_account-metadata.ts`/`.test.ts` (pre-existing unrelated errors in `src/routes/guide.tsx` are untouched by this task).
- Endpoint was confirmed via a real authenticated call against the connected account, not guessed.

## 7. Out of Scope

- Reworking `isProviderUsageLimitError`/`402` terminal-usage-limit classification (already correct per `Task-210 T-6`/`DOD-6`) — this task is about proactive display, not reactive error handling.
- Any change to `ProviderAccount`/`provider-accounts.json` schema, account discovery, connect, or switch behavior.
- Unifying Grok's ACP transport with Gemini's (explicit non-goal already stated in `CP-46 P-4`).
- Verifying the `weeklyLimit` response shape against a real free-tier account (no such account was available; tracked as `Q-1` above).
- End-to-end desktop-UI screenshot verification — this is a server-side metadata/API change with no new UI code; `ProviderAccountsPanel.tsx`'s existing generic `usageDetailLines` renderer was not modified and was already covered by prior tasks (`Task-036`).

## 8. Completion Notes

- result: Implemented and verified. The Grok billing endpoint was found and confirmed live (`GET https://cli-chat-proxy.grok.com/v1/billing`), wired into both the Go CLI metadata builder and the admin-web TypeScript twin, and covered by new unit tests in both languages. `Task-210 Q-1` and `CP-46 Q-5` were corrected in place. Labels renamed to `"Team Credits (Monthly/Weekly)"` after a post-ship comparison against the real `grok` CLI showed a second, distinct SuperGrok weekly-allowance metric this task's endpoint does not report.
- follow-ups: Re-verify the `weeklyLimit` branch against a real weekly/free-tier Grok account if one becomes available (`Q-1` above); consider caching/backoff if the billing endpoint proves rate-limited under frequent account-panel refreshes (`Q-2` above); find and surface the separate SuperGrok weekly included-usage allowance, tracked as [Task-217](../todo/Task-217-Grok-SuperGrok-Weekly-Included-Usage-Fetch.md).
- upstream docs updated: `requirements/08-Task/inprogress/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md` (`Q-1` annotated), `requirements/07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md` (`Q-5` annotated).
