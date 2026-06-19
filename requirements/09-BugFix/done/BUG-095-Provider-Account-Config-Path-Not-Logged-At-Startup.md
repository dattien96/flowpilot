# Metadata

- Document ID: `BUG-095`
- Title: `Provider Account Config Path Not Logged At Startup`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: `—`
- Child Documents: `—`
- Related Documents: [BUG-094: Provider Account IDs Are Volatile](./BUG-094-Provider-Account-IDs-Are-Volatile-And-Regenerate-On-Missing-Config.md), [CA-110: Add Provider Account Config Path Logging](../../../change-audit/CA-110-add-provider-account-config-path-logging.md)
- Replaces: `—`
- Tags: `provider-account, local-runner, diagnostics, severity-low`

## AI Quick View

### Summary

- `providerAccountsConfigPath()` is resolved at runtime but never logged.
- When `provider-accounts.json` is missing the runner silently auto-discovers accounts with fresh IDs, leaving no trace of why IDs changed.
- Different launch methods (desktop app vs. manual restart) may resolve different paths via `APPDATA`, `HOME`, `USERPROFILE`, or `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`, and this drift is invisible in logs.

### Current Ask

- Log the resolved config path when the file is not found (warning) and when accounts are saved (info), so config-path drift is immediately visible in server logs.

### Key Decisions

- `V-1` Always log when the config file is not found, including the resolved path.
- `V-2` Log the resolved path whenever `saveProviderAccountState` writes the file.
- `V-3` No log spam on repeated successful reads (path logged only on writes and missing-file events).

### Constraints

- Do not log credentials, token values, or home-path contents.
- Log lines use the `[provider-accounts]` prefix for easy grep.

### Open Questions

- None.

### Source Refs

- Root-cause analysis from BUG-092 post-fix review, 2026-06-19.
- `apps/local-runner/internal/runner/provider_accounts.go`

## 1. Issue Summary

When `provider-accounts.json` is absent or at a different path, the runner regenerates account IDs silently. There is no log entry showing which path was resolved, making it impossible to diagnose path-drift or missing-file scenarios from server logs alone.

## 2. Parent Links

- impacted coding plan: `—`
- impacted tech design: `—`
- impacted system spec: `—`

## 3. Environment and Reproduction

- environment: any FlowPilot runner instance
- reproduction steps: trigger ID regeneration (delete config or change path env var) and observe that no log explains why IDs changed
- frequency: always — no diagnostic log exists for the not-found case

## 4. Expected vs Actual

- expected: server log shows `[provider-accounts] config not found path=...` when the file is missing, and `[provider-accounts] config saved path=...` when it is written
- actual: no log; only downstream `session_unavailable` errors indicate something went wrong

## 5. Impact

- users affected: developers diagnosing account-ID regeneration issues
- workflows affected: diagnostic/support workflows
- severity: low (diagnostic only; no functional change)

## 6. Root Cause

- hypothesis: missing diagnostic logging
- confirmed cause: `loadProviderAccountState` and `saveProviderAccountState` have no log statements for path or file-existence state
- evidence: code review of `provider_accounts.go`

## 7. Fix Strategy

- `F-1` In `loadProviderAccountState`, log `[provider-accounts] config not found path=%q — accounts auto-discovered, stored session IDs may be stale` when `os.ErrNotExist`.
- `F-2` In `saveProviderAccountState`, log `[provider-accounts] config saved path=%q accounts=%d` after a successful write.

## 8. Validation

- `V-1` Running the runner without a config file produces the expected `[provider-accounts] config not found` log line.
- `V-2` All existing provider-accounts tests continue to pass.

## 9. Regression Guard

- tests: no behavioral change; existing tests guard against regressions
- alerts: none
- audit checks: verify log lines appear in real runner output when config is absent

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: no log on successful reads to avoid spam on every `ListProviderAccounts` call
