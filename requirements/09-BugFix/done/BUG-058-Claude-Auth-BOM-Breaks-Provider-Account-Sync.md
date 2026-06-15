# BUG-058: Claude Auth BOM Breaks Provider Account Sync

## Metadata

- Document ID: `BUG-058`
- Title: `Claude auth BOM breaks provider account sync`
- Phase: `bugfix`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-057-Claude-Stale-Usage-Metadata-Blocks-Reset-Account.md`, `change-audit/CA-071-fix-bom-prefixed-local-json-auth.md`
- Replaces: `none`
- Tags: `desktop`, `local-runner`, `claude`, `provider-accounts`, `json`, `regression`

## AI Quick View

### Summary

- A recent provider-account sync change began reading stored account auth from the saved account home path.
- BOM-prefixed local JSON auth/state files could trip Go JSON parsing with `invalid character 'ï' looking for beginning of value`.
- The runner now strips a UTF-8 BOM before unmarshalling local JSON files used by provider account state, JSON config validation, shared runner JSON reads, and Claude usage metadata reads.

### Current Ask

- Fix the regression where local JSON auth files that begin with a UTF-8 BOM can break account sync or related local JSON parsing.

### Key Decisions

- `V-1` Normalize UTF-8 BOM bytes at the runner JSON-read boundary instead of requiring users to manually rewrite local auth files.
- `V-2` Keep the change scoped to local file JSON reads; do not alter provider command output parsing or remote API response parsing.

### Constraints

- Preserve existing account selection behavior from BUG-057.
- Do not suppress genuinely malformed JSON after BOM stripping.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection and this limitation is recorded.

### Open Questions

- Should the duplicated CLI-package `readJSONFile` helper also be unified with the runner helper in a later cleanup?

### Source Refs

- User report `2026-06-15`: `regression (invalid character 'ï' looking for beginning of value) after current code change`.
- `apps/local-runner/internal/runner/provider_accounts.go`
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/claude_usage.go`
- `apps/local-runner/internal/runner/provider_accounts_test.go`

## 1. Issue Summary

After the current provider-account code change, local runner execution could fail with `invalid character 'ï' looking for beginning of value`. In Go JSON decoding, that symptom is consistent with a UTF-8 BOM (`EF BB BF`, displayed as `ï»¿` under the wrong encoding) appearing before the opening JSON object.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner on Windows with Claude provider account sync enabled.
- reproduction steps:
  1. Store a Claude account whose saved `home_path` contains a `.claude.json` file that starts with a UTF-8 BOM.
  2. Run provider account listing or sync after the BUG-057 account-home-path change.
  3. Observe the JSON decode failure.
- frequency: Reproducible when a local JSON file that is parsed through the affected runner path begins with a UTF-8 BOM.

## 4. Expected vs Actual

- expected: UTF-8 BOM-prefixed local auth JSON is accepted the same as normal UTF-8 JSON, while truly malformed JSON still fails.
- actual: raw bytes were passed directly to `json.Unmarshal`, causing Go to reject the leading BOM before the JSON object.

## 5. Impact

- users affected: Desktop users with BOM-prefixed local provider auth or runner state/config JSON.
- workflows affected: provider account sync and Claude account recovery after the prior account-home-path fix.
- severity: `medium`, because a valid local account can be blocked by file encoding metadata.

## 6. Root Cause

- hypothesis: the recent account-sync change made FlowPilot read a BOM-prefixed local auth JSON file that previous logic did not touch.
- confirmed cause: runner-side local JSON readers passed file bytes directly into `json.Unmarshal`; Go's decoder rejects the UTF-8 BOM at byte zero.
- evidence: code inspection found raw `json.Unmarshal` calls in provider account state/config and Claude metadata readers; the new regression test with a BOM-prefixed `.claude.json` passes after stripping the BOM.

## 7. Fix Strategy

- `F-1` Add a small runner helper that removes a leading UTF-8 BOM from local JSON file bytes.
- `F-2` Apply the helper to provider account state loading, Gemini JSON config validation, the shared runner `readJSONFile`, and Claude usage metadata reads.
- `F-3` Add a provider-account regression test that keeps a stored default Claude account connected when `.claude.json` starts with a UTF-8 BOM.

## 8. Validation

- `V-1` `go test ./internal/runner -run "TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth"` - **PASS**
- `V-2` `go test ./internal/runner -run "TestListProviderAccounts|TestDeleteProviderAccount"` - **PASS**
- `V-3` `go test ./internal/runner` - **FAIL** due existing Google Drive MCP setup and Windows shell environment failures unrelated to this BOM fix.

## 9. Regression Guard

- tests: `TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth`
- alerts: none
- audit checks: `change-audit/CA-071-fix-bom-prefixed-local-json-auth.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none - this is an implementation compatibility fix for local JSON encoding and does not change provider-account business behavior.
- notes left unchanged on purpose: BUG-057 remains the source for stored default Claude account recovery behavior.
