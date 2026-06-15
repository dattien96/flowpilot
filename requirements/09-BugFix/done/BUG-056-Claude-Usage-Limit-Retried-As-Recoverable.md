# BUG-056: Claude Usage Limit Retried As Recoverable

## Metadata

- Document ID: `BUG-056`
- Title: `Claude usage limit retried as recoverable`
- Phase: `bugfix`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: ``
- Related Documents: `requirements/09-BugFix/done/BUG-051-Claude-Limit-Reported-As-Login.md`, `requirements/09-BugFix/done/BUG-054-Claude-Usage-Limit-Message-Contains-Confusing-Login-Phrase.md`, `requirements/09-BugFix/done/BUG-055-Claude-Session-Execution-Bypasses-Usage-Limit-Normalization.md`, `change-audit/CA-069-fix-claude-usage-limit-retry-classification.md`
- Replaces: ``
- Tags: `desktop`, `local-runner`, `claude`, `provider-runtime`, `retry`, `quota`

## AI Quick View

### Summary

- Claude quota failures were normalized to the correct usage-limit message, but the interactive retry loop still treated the error as recoverable.
- Users saw two retry banners before the same terminal quota failure.
- The retry classifier now treats provider quota and usage-limit errors as terminal.

### Current Ask

- Stop retrying turns when the provider error says Claude usage is exhausted.

### Key Decisions

- `V-1` Preserve retries for real transport failures such as stream closure.
- `V-2` Treat usage-limit, quota reset, rate-limit, and out-of-credits messages as terminal send errors.

### Constraints

- Keep the change scoped to retry classification and regression coverage.
- Do not alter the Claude quota message text fixed by BUG-054 and BUG-055.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection.

### Open Questions

- Should provider adapters eventually return typed terminal errors instead of relying on message classification at the retry boundary?

### Source Refs

- User report `2026-06-15`: `[recovering: re-sending turn after a recoverable error (attempt 2/3)]` and `attempt 3/3` appear before `Claude usage limit reached...`.
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/send_retry_test.go`

## 1. Issue Summary

The local runner correctly reported Claude quota exhaustion, but the interactive send-with-retry loop classified the quota error as recoverable. This caused the desktop timeline to show retry banners and re-send the turn even though the selected Claude account could not succeed until the user switches account or waits for quota reset.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local runner with a Claude account whose usage is exhausted.
- reproduction steps:
  1. Select the exhausted Claude account.
  2. Send a turn through the desktop interactive runtime.
  3. Observe the recovery banners before the final quota message.
- frequency: Reproducible whenever the adapter returns a Claude usage-limit error through `sendTurnWithRetry`.

## 4. Expected vs Actual

- expected: the runner emits the Claude usage-limit failure once, without retry banners.
- actual: the runner emits retry banners for attempts 2 and 3, then emits the same quota failure.

## 5. Impact

- users affected: Desktop users with exhausted Claude usage.
- workflows affected: interactive Claude turns using the local runner retry loop.
- severity: `medium`, because the runner wastes time and makes a terminal account state look like a transient transport failure.

## 6. Root Cause

- hypothesis: the generic retry classifier only excluded cancellation and approval/question expiry, so quota failures fell through as recoverable.
- confirmed cause: `isRecoverableSendError` returned `true` for `Claude usage limit reached...` because the error was not a context cancellation or approval/question expiry.
- evidence: the user-visible retry banners come directly from `sendTurnWithRetry`, and a focused test now proves usage-limit errors are attempted only once.

## 7. Fix Strategy

- `F-1` Add `isProviderUsageLimitError` to classify quota and usage-limit errors as terminal.
- `F-2` Call the new classifier from `isRecoverableSendError` before allowing retry.
- `F-3` Add a regression test that verifies a Claude usage-limit error is not retried.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(SendTurnWithRetryDoesNotRetryUsageLimit|SendTurnWithRetryDoesNotRetryTerminal|SendTurnWithRetryRecovers|ClaudeUsageLimitErrorFromAuthMetadata|MapClaudeLineResultLimitDoesNotReportLogin)$"` - **PASS**

## 9. Regression Guard

- tests: `TestSendTurnWithRetryDoesNotRetryUsageLimit`
- alerts: none
- audit checks: `change-audit/CA-069-fix-claude-usage-limit-retry-classification.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none - this is retry classification, not a provider contract change.
- notes left unchanged on purpose: BUG-051, BUG-054, and BUG-055 remain the history for detecting and wording Claude quota failures.
