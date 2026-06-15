# BUG-059: Claude Login Regression After Account Sync Fix

## Metadata

- Document ID: `BUG-059`
- Title: `Claude login regression after account sync fix`
- Phase: `bugfix`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-057-Claude-Stale-Usage-Metadata-Blocks-Reset-Account.md`, `requirements/09-BugFix/done/BUG-058-Claude-Auth-BOM-Breaks-Provider-Account-Sync.md`, `change-audit/CA-072-fix-claude-credential-config-dir.md`
- Replaces: `none`
- Tags: `desktop`, `local-runner`, `claude`, `provider-accounts`, `auth`, `regression`

## AI Quick View

### Summary

- After the BOM/account-sync fix, Codex worked but Claude could fail with `Not logged in - Please run /login`.
- Claude account detection treated `.claude.json` profile metadata as valid login state even though the live CLI credential file is `.claude/.credentials.json`.
- The live Claude adapter also set `CLAUDE_CONFIG_DIR` to the account home instead of the account's `.claude` config directory.

### Current Ask

- Ensure selected Claude accounts are backed by real Claude CLI credentials and the live adapter launches with the credential config directory.

### Key Decisions

- `V-1` Treat `.claude/.credentials.json` with `claudeAiOauth` tokens as the primary Claude auth signal.
- `V-2` Launch the live Claude adapter with `CLAUDE_CONFIG_DIR=<account home>/.claude` and account home/profile environment variables.
- `V-3` Do not treat `.claude.json` email-only profile metadata as sufficient login proof.

### Constraints

- Preserve BUG-057 behavior that stale usage-limit metadata must not preflight-block a genuinely authenticated Claude account.
- Preserve BUG-058 BOM tolerance for local JSON credential reads.
- GitNexus impact tooling was not available in this thread, so required symbol impact checks were replaced with local blast-radius inspection.

### Open Questions

- Should the UI distinguish profile metadata present from credential file present when showing Claude account health?

### Source Refs

- User report `2026-06-15`: `After this one, codex worked. but CLAUDE got Not logged in - Please run /login as regression`.
- Local credential shape check: `C:\Users\dat.nguyen\.claude\.credentials.json` contains top-level `claudeAiOauth` credential data.
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/provider_registry.go`
- `apps/local-runner/internal/runner/claude_adapter_test.go`
- `apps/local-runner/internal/runner/provider_accounts_test.go`

## 1. Issue Summary

Claude could be selected as a connected provider account, but the live Claude CLI returned `Not logged in - Please run /login`. This surfaced after the account-sync changes that kept stored Claude accounts connected when auth was found at the saved home path.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner on Windows with Claude selected.
- reproduction steps:
  1. Keep a Claude account whose home path has `.claude.json` profile metadata.
  2. Select or sync the account after the stored-home-path account recovery fix.
  3. Start a Claude turn through the live local-runner adapter.
  4. Observe Claude's login error.
- frequency: Reproducible when the selected account is considered connected from profile metadata or when the live adapter points `CLAUDE_CONFIG_DIR` at the home directory instead of the `.claude` config directory.

## 4. Expected vs Actual

- expected: FlowPilot only treats Claude as connected when credential-bearing auth exists, and the live adapter launches Claude with the config directory containing those credentials.
- actual: FlowPilot could accept `.claude.json` email metadata as auth and launch Claude with `CLAUDE_CONFIG_DIR=<account home>`, so the CLI did not see credentials and reported `/login`.

## 5. Impact

- users affected: Desktop users running Claude through the local runner after the account sync fix.
- workflows affected: Claude provider turns using the live adapter.
- severity: `high`, because Claude turns fail immediately even when Codex turns work.

## 6. Root Cause

- hypothesis: account sync selected the wrong Claude home or failed to pass the selected account environment to the live adapter.
- confirmed cause: Claude auth detection did not include `.claude/.credentials.json` and treated `.claude.json` email metadata as valid auth. In addition, `ProviderRegistryFor` set `CLAUDE_CONFIG_DIR` to `account.HomePath` rather than `account.HomePath/.claude`.
- evidence: local credential inspection showed active Claude credentials under `.claude/.credentials.json` with `claudeAiOauth`; code inspection showed `accountAuthPaths` omitted that file and `ProviderRegistryFor` used the account home as the config dir; focused tests now assert credential-file detection and correct live adapter env.

## 7. Fix Strategy

- `F-1` Add `.claude/.credentials.json` to Claude default and stored account auth paths.
- `F-2` Require credential-bearing Claude auth data such as `claudeAiOauth.accessToken` or `claudeAiOauth.refreshToken` instead of email-only profile metadata.
- `F-3` Launch the live Claude adapter with account home/profile env values plus `CLAUDE_CONFIG_DIR=<account home>/.claude`.
- `F-4` Keep stale usage-limit metadata from blocking authenticated accounts.
- `F-5` Add focused tests for the credential config dir, BOM-prefixed credential auth, and metadata-only false-positive auth.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(ClaudeRegistryIgnoresStaleUsageLimitMetadata|ClaudeRegistryUsesCredentialConfigDirForAccount|ListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|ListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth|ListProviderAccountsMarksClaudeMetadataOnlyAccountFailed|MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata)$"` - **PASS**
- `V-2` Full `go test ./internal/runner` not rerun for this follow-up; previous full run in this session failed on unrelated Google Drive MCP setup and Windows shell environment issues.

## 9. Regression Guard

- tests: `TestClaudeRegistryUsesCredentialConfigDirForAccount`, `TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected`, `TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth`, `TestListProviderAccountsMarksClaudeMetadataOnlyAccountFailed`
- alerts: none
- audit checks: `change-audit/CA-072-fix-claude-credential-config-dir.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none - this corrects the local-runner interpretation of Claude CLI credential storage and does not change provider selection requirements.
- notes left unchanged on purpose: BUG-057 remains valid for stale quota metadata; BUG-058 remains valid for BOM-prefixed local JSON handling.
