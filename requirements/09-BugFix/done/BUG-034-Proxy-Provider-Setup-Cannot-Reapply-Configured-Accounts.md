# BUG-034: Proxy Provider Setup Cannot Reapply Configured Accounts

## Metadata

- Document ID: `BUG-034`
- Title: `Proxy Provider Setup Cannot Reapply Configured Accounts`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-09`
- Last Updated: `2026-06-09`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-033-Proxy-MCP-Provider-Config-Uses-Unlaunchable-Flowpilot-Command.md`, `change-audit/CA-042-cp29-runner-proxy-hardening.md`
- Replaces: `none`
- Tags: `google-drive, proxy-mcp, settings-ui, provider-config, codex, usability`

## AI Quick View

### Summary

- The Google Drive setup page disabled `Configure AI Providers` once all discovered provider rows were marked `configured`.
- That blocked safe reapplication of provider config after proxy command-shape changes or other config refresh needs.
- The lockout made BUG-033 harder to recover from because users could not regenerate `~/.codex/config.toml` from the UI after the state turned green.

### Current Ask

- Record the setup-page gating bug and the change that keeps provider-config reapply available for discovered accounts.

### Key Decisions

- `V-1` `Configure AI Providers` must remain available whenever proxy MCP is enabled, proxy auth is ready, and at least one provider account home is discovered.
- `V-2` Reapplying config to already-configured accounts is valid and should be treated as a refresh, not an error path.
- `V-3` Missing account-home discovery should still block only the unresolved rows, not the whole refresh action.

### Constraints

- Keep proxy auth and proxy-enabled gating intact; do not allow configuration when the runner is not ready.
- Do not widen the action to unknown provider slots with no discovered account-home path.
- Preserve the same `ensureGoogleDriveMcpProviderConfig` backend contract; this fix is UI orchestration only.

### Open Questions

- Should the settings page offer per-provider refresh actions in addition to the bulk reapply button?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `apps/admin-web/src/routes/_authenticated/settings/components/-GoogleDriveProviderConfigCard.tsx`
- `apps/admin-web/src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.test.tsx`
- Observed UI state: `Configure AI Providers` disabled after providers reached `configured`

## 1. Issue Summary

The proxy provider setup card only considered `not_started`, `config_stale`, and `failed` rows actionable. Once all discovered providers became `configured`, the button disabled itself even though rerunning the same configuration was safe and sometimes necessary.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: admin web Google Drive setup page with proxy MCP enabled and artifact-sync auth already configured
- reproduction steps:
  1. Configure provider MCP settings until all discovered rows show `configured`.
  2. Return to the Google Drive setup page.
  3. Inspect the `Configure AI Providers` button state.
  4. Attempt to reapply provider config after changing proxy launch behavior or another provider-config detail.
- frequency: reproducible whenever every discovered provider row is in `configured` state.

## 4. Expected vs Actual

- expected: users should be able to rerun provider configuration for discovered accounts to refresh generated config files.
- actual: the button disabled itself once all rows were `configured`, so there was no UI path to regenerate config without first forcing a stale or failed state.

## 5. Impact

- users affected: anyone using the Google Drive setup page to manage proxy MCP provider config.
- workflows affected: CP-29 provider-config refresh, local recovery after command-shape or config-generation fixes, and manual validation loops.
- severity: medium because it blocks recovery and reapply flows, but not the initial happy-path setup.

## 6. Root Cause

- hypothesis: the setup card treated provider configuration as a one-time transition from bad states to good states instead of a repeatable sync action.
- confirmed cause: `GoogleDriveProviderConfigCard` filtered actionable rows down to `not_started`, `config_stale`, and `failed`, then disabled the button when no rows matched that filter.
- evidence:
  - the button-enable logic depended on `pendingProviderConfigs.length > 0`
  - already-configured rows with valid `accountHomePath` were excluded from the mutation input
  - rerun was required immediately after BUG-033 because the prior green state still held a broken command path

## 7. Fix Strategy

- `F-1` Change the actionable row set from pending-only rows to all discovered provider accounts with a non-empty `accountHomePath`.
- `F-2` Keep the button enabled whenever proxy MCP is enabled, proxy auth is ready, and at least one discovered account exists.
- `F-3` Update success messaging so reruns against already-configured accounts are described as a refresh instead of a first-time configuration.

## 8. Validation

- `V-1` UI tests confirm the button stays enabled when every discovered provider row is already `configured`.
- `V-2` UI tests confirm clicking the button reruns `ensureGoogleDriveMcpProviderConfig` for all discovered provider accounts, including configured ones.
- `V-3` UI tests confirm unresolved provider slots without account-home discovery still produce the partial-success message instead of blocking discovered rows.

## 9. Regression Guard

- tests:
  - `GoogleDriveProviderConfigCard > keeps button enabled when all providers are already configured`
  - `GoogleDriveProviderConfigCard > can refresh provider configs even when every provider is already configured`
- alerts:
  - none added; this remains covered by UI test expectations
- audit checks:
  - confirm bulk provider refresh still requires proxy-enabled and proxy-auth-ready state
  - confirm only discovered provider rows are submitted to `ensureGoogleDriveMcpProviderConfig`

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- notes left unchanged on purpose:
  - provider backend APIs and Google auth prerequisites did not change; only the admin-web action gating and refresh semantics changed.
