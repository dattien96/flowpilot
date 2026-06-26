# CA-067: Fix Claude Usage Limit Message Trailing Login Phrase

## Scope

Two error-message strings in `apps/local-runner/internal/runner/` ended with the clause
`"; login is still present"`. The clause was added in BUG-051 to distinguish a quota
exhaustion from a genuine login failure, but it reads as a second problem (a login issue)
rather than as reassurance.

Files changed:

- `apps/local-runner/internal/runner/claude_usage.go` — preflight error returned from
  `claudeUsageLimitError`: removed trailing `; login is still present`.
- `apps/local-runner/internal/runner/claude_event_mapper.go` — result-frame error
  returned from `claudeUsageLimitMessage`: removed trailing `; login is still present.`.
- `apps/local-runner/internal/runner/claude_adapter_test.go` — two test assertions
  updated to match the new strings.

## Completed

- Removed the confusing trailing clause from both message sites.
- Tests updated in place; no new tests needed because the existing tests now pin the
  corrected text.

## Verification

`go test ./internal/runner -run "Test(MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|ClaudeRegistryGating)$"` — **PASS** (3/3).

## Residual Notes

- The underlying quota detection logic (`isClaudeUsageLimitReason`, `claudeUsageLimitMessage`,
  `claudeUsageLimitError`) is unchanged.
- If Claude CLI ever surfaces a reset-time estimate in its result metadata, the message
  could include it (noted as an open question in BUG-051 and BUG-054).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-051
change_type: fix
summary: Fix Claude Usage Limit Message Trailing Login Phrase
# --->8---
