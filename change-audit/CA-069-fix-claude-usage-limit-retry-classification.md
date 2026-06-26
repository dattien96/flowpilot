# CA-069 Fix Claude Usage Limit Retry Classification

## Scope

Record the follow-up fix that stops the local runner from retrying terminal Claude usage-limit errors after the quota message itself has been normalized.

## Completed

- Updated `apps/local-runner/internal/runner/interactive_service.go` so `isRecoverableSendError` treats provider usage-limit and quota messages as terminal.
- Added `isProviderUsageLimitError` to recognize usage-limit, quota reset, out-of-credits, and rate-limit messages at the retry boundary.
- Added `TestSendTurnWithRetryDoesNotRetryUsageLimit` in `apps/local-runner/internal/runner/send_retry_test.go`.
- Preserved existing retry behavior for real recoverable transport failures covered by `TestSendTurnWithRetryRecovers`.

## Verification

- Ran `go test ./internal/runner -run "Test(SendTurnWithRetryDoesNotRetryUsageLimit|SendTurnWithRetryDoesNotRetryTerminal|SendTurnWithRetryRecovers|ClaudeUsageLimitErrorFromAuthMetadata|MapClaudeLineResultLimitDoesNotReportLogin)$"` in `apps/local-runner`

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- The final user-facing quota message still appears, but it should now appear once without the `[recovering: re-sending turn...]` retry banners.
- A future cleanup could replace retry-boundary string classification with typed terminal provider errors.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-069
change_type: fix
summary: Fix Claude Usage Limit Retry Classification
# --->8---
