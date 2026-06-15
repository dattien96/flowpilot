# BUG-051: Claude Limit Reported As Login

## Metadata

- Document ID: `BUG-051`
- Title: `Claude limit reported as login`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot maintainers`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-018-claude-missing-acc-infor.md`, `requirements/09-BugFix/done/BUG-050-Desktop-Claude-Provider-Blank-Chat.md`, `change-audit/CA-061-fix-claude-limit-error-classification.md`
- Replaces: `none`
- Tags: `desktop, local-runner, claude, provider-runtime, quota, regression`

## AI Quick View

### Summary

- Claude runs could show `Not logged in · Please run /login` when the selected Claude account was actually usage-limited.
- The existing account metadata reader already knew about Claude extra-usage disabled reasons such as `out_of_credits`.
- The live Claude run path did not check that metadata before launching the CLI.
- Quota-shaped Claude result metadata was not normalized before surfacing the failure to the desktop timeline.

### Current Ask

- Report Claude account/token-limit exhaustion as a usage-limit failure, not as a login failure.

### Key Decisions

- `F-1` Preflight active Claude account metadata for usage-limit disabled reasons before spawning `claude`.
- `F-2` Normalize quota-shaped Claude result failures to a usage-limit message.
- `V-1` Add focused tests for auth metadata preflight and result-message normalization.

### Constraints

- GitNexus MCP tools were not exposed in this thread, so symbol impact checks were performed with local call-graph search and targeted tests.
- Claude CLI can emit misleading login text for exhausted accounts; FlowPilot should prefer local account metadata and quota-shaped result context when available.

### Open Questions

- Exact reset-time display remains a future improvement because Claude local auth metadata does not expose Codex/Gemini-style numeric quota windows.

### Source Refs

- User report on `2026-06-15`: Claude returned login guidance, but the account was token/usage limited.
- `apps/local-runner/internal/runner/claude_usage.go`
- `apps/local-runner/internal/runner/claude_event_mapper.go`
- `apps/local-runner/internal/runner/provider_registry.go`
- `apps/local-runner/internal/runner/claude_adapter_test.go`

## 1. Issue Summary

After Claude was enabled in the desktop runner flow, a Claude account that had exhausted its token/usage allowance surfaced as `Not logged in · Please run /login for claude`. The user was logged in; the actionable condition was to switch accounts or wait for usage reset.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner on `2026-06-15`.
- reproduction steps:
  1. Select a connected Claude account that has exhausted its available usage.
  2. Select Claude in the desktop chat.
  3. Send a prompt.
- frequency: Reproducible when Claude local account metadata or provider result metadata indicates quota exhaustion but the CLI reports a generic login message.

## 4. Expected vs Actual

- expected: The timeline reports that Claude usage limit was reached and suggests switching account or waiting for reset.
- actual: The timeline reported `Not logged in` and instructed the user to run `/login`.

## 5. Impact

- users affected: Desktop users running Claude after exhausting usage on the selected account.
- workflows affected: Claude provider launches and turns.
- severity: `medium`, because the workflow still fails but the remediation guidance was wrong.

## 6. Root Cause

- hypothesis: The runner trusted Claude CLI's generic login error and did not use local usage metadata.
- confirmed cause: The live Claude adapter path did not preflight `cachedExtraUsageDisabledReason` from the selected account's Claude auth metadata. The event mapper also returned raw Claude result text unless the result itself had an obvious error message.
- evidence: Existing account metadata code for the sidebar already parsed `cachedExtraUsageDisabledReason`; the runtime adapter registry did not. Focused tests now cover `out_of_credits` metadata and result frames that contain both misleading login text and quota metadata.

## 7. Fix Strategy

- `F-1` Added `claudeUsageLimitError(homePath)` in the runner package to read active Claude auth metadata and detect usage-limit disabled reasons.
- `F-2` Wired the preflight into the live Claude registration in `ProviderRegistryFor(r)` before constructing the adapter.
- `F-3` Added `claudeUsageLimitMessage` in the Claude event mapper so quota-shaped result frames surface as usage-limit errors.
- `F-4` Updated the Claude adapter plan to document quota normalization.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|ClaudeRegistryGating)$"` from `apps/local-runner` passed.
- `V-2` `npm run typecheck` from `apps/desktop-flowpilot` passed.

## 9. Regression Guard

- tests: `TestClaudeUsageLimitErrorFromAuthMetadata` and `TestMapClaudeLineResultLimitDoesNotReportLogin`.
- alerts: none added.
- audit checks: `change-audit/CA-061-fix-claude-limit-error-classification.md` records the change.

## 10. Follow-Up Document Updates

- upstream docs that must change: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md` now notes quota normalization.
- notes left unchanged on purpose: `SS-05` and `SD-06` already require provider auth/readiness surfaces and do not need behavioral changes.
