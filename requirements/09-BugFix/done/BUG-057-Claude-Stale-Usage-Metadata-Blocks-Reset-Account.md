# BUG-057: Claude Stale Usage Metadata Blocks Reset Account

## Metadata

- Document ID: `BUG-057`
- Title: `Claude stale usage metadata blocks reset account`
- Phase: `bugfix`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-054-Claude-Usage-Limit-Message-Contains-Confusing-Login-Phrase.md`, `requirements/09-BugFix/done/BUG-055-Claude-Session-Execution-Bypasses-Usage-Limit-Normalization.md`, `requirements/09-BugFix/done/BUG-056-Claude-Usage-Limit-Retried-As-Recoverable.md`, `change-audit/CA-070-fix-claude-stale-usage-metadata-block.md`
- Replaces: `none`
- Tags: `desktop`, `local-runner`, `claude`, `provider-runtime`, `quota`, `account-selection`

## AI Quick View

### Summary

- FlowPilot could reject or fail to select the active Claude account when local auth metadata still contained `cachedExtraUsageDisabledReason: "out_of_credits"` or the process home did not match the stored Claude account home.
- After quota reset, Claude CLI may be able to run even if the cached local metadata has not been refreshed yet.
- The runner now keeps stored authenticated Claude accounts connected and lets Claude validate quota at execution time.

### Current Ask

- Ensure the Claude provider uses the currently selected active account after quota reset instead of blocking on stale local quota metadata.

### Key Decisions

- `V-1` Remove stale local quota metadata as a preflight blocker.
- `V-2` Keep stored default accounts connected when their saved `home_path` still has valid auth, even if the current process home differs.
- `V-3` Keep runtime usage-limit normalization for stderr and result frames returned by Claude.

### Constraints

- Keep the account resolver unchanged because it already selects the active connected Claude account.
- Do not suppress real Claude quota failures returned by the CLI.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection.

### Open Questions

- Should provider account status display expose stale local quota metadata as an informational warning instead of using it for blocking?

### Source Refs

- User report `2026-06-15`: Claude account is active after token reset, but FlowPilot still reports out-of-credits and should use the correct account.
- Local state check: `provider-accounts.json` marked the only Claude account `auth_status: "failed"` and `is_active: false` even though `C:\Users\dat.nguyen\.claude.json` still contained an OAuth account email.
- `apps/local-runner/internal/runner/provider_registry.go`
- `apps/local-runner/internal/runner/provider_accounts.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/provider_accounts_test.go`
- `apps/local-runner/internal/runner/sessions_test.go`

## 1. Issue Summary

After a Claude quota reset, FlowPilot could still show `Claude usage limit reached: extra usage unavailable (out of credits). Switch Claude account or wait for quota reset` without invoking the active Claude account. Two local-state problems could combine: the runner trusted cached local Claude quota metadata as a preflight failure source, and provider account sync could mark the stored default Claude account failed when the runner process home differed from the account's saved `home_path`.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner with Claude selected.
- reproduction steps:
  1. Use a Claude account that previously exhausted usage.
  2. Let the Claude quota reset so the account is active again.
  3. Keep local Claude metadata containing `cachedExtraUsageDisabledReason: "out_of_credits"`.
  4. Send a Claude prompt through FlowPilot.
- frequency: Reproducible when local Claude metadata remains stale after provider-side reset.

## 4. Expected vs Actual

- expected: FlowPilot keeps the stored authenticated Claude account connected, selects it, and starts Claude so the CLI can validate the current account state.
- actual: FlowPilot could mark the stored account failed or return the cached out-of-credits message before starting Claude.

## 5. Impact

- users affected: Desktop users using Claude after quota reset.
- workflows affected: Claude provider runs and Claude session/chat execution.
- severity: `medium`, because an account that can run after reset can be blocked by stale local metadata.

## 6. Root Cause

- hypothesis: the active Claude account was selected correctly, but stale `cachedExtraUsageDisabledReason` was treated as authoritative.
- confirmed cause: the live Claude registry factory and `claude_stream_json` session path called `claudeUsageLimitError(account.HomePath)` before invoking Claude. In addition, `syncProviderAccounts` marked a stored default account failed when `DetectDefaultAccountHomePath` did not find auth under the current process home, without checking whether the saved `account.HomePath` still contained valid auth.
- evidence: local code inspection found the preflight calls after `ResolveProviderAccount`; local state showed the stored Claude account marked failed despite valid auth at its saved home path; the new tests prove stale quota metadata no longer prevents adapter creation or command execution, and stored default Claude auth remains connected.

## 7. Fix Strategy

- `F-1` Remove the Claude usage-limit metadata preflight from live provider adapter construction.
- `F-2` Remove the Claude usage-limit metadata preflight from the `claude_stream_json` session send path.
- `F-3` Preserve runtime normalization when Claude returns quota details through stderr or result frames.
- `F-4` Keep stored default provider accounts connected when their saved home path still contains valid provider auth.
- `F-5` Add regression tests that stale `cachedExtraUsageDisabledReason: "out_of_credits"` does not block adapter creation or command execution, and that stored default Claude auth remains connected.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(ListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|ClaudeRegistryIgnoresStaleUsageLimitMetadata|SendMessageClaudeStaleUsageLimitMetadataStillRunsCommand|SendMessageClaudeResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|SendTurnWithRetryDoesNotRetryUsageLimit)$"` - **PASS**
- `V-2` `go test ./internal/runner` - **FAIL** due existing Google Drive/Codex environment-dependent tests unrelated to this Claude fix.
- `V-3` Local provider account repair: backed up `C:\Users\dat.nguyen\AppData\Roaming\FlowPilot\provider-accounts.json` and restored the Claude default account to `auth_status: "connected"` and `is_active: true`.

## 9. Regression Guard

- tests: `TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected`, `TestClaudeRegistryIgnoresStaleUsageLimitMetadata`, `TestSendMessageClaudeStaleUsageLimitMetadataStillRunsCommand`, `TestSendMessageClaudeResultLimitDoesNotReportLogin`, `TestSendTurnWithRetryDoesNotRetryUsageLimit`
- alerts: none
- audit checks: `change-audit/CA-070-fix-claude-stale-usage-metadata-block.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none - this corrects stale-cache handling and does not change provider selection requirements.
- notes left unchanged on purpose: BUG-054 through BUG-056 remain valid for message wording, session normalization, and retry classification.
