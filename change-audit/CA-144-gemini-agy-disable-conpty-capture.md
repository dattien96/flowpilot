# CA-144: Gemini AGY Disable ConPTY Capture

## Summary

Disabled the Windows ConPTY capture path for Gemini AGY print mode after it crashed AGY with `exit status 0xc0000374` and caused the supervisor to shut down the runner stack.

## What Changed

- Replaced Windows `captureAgyPrint` with ordinary buffered process execution.
- Left empty-output handling to the existing AGY conversation DB recovery from `BUG-148`.
- Updated capture comments and the gated live capture test so Windows empty pipe output is treated as expected.

## Verification

- `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize|TestStripTerminalSequences|TestIsAgyCommand' -count=1`

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-149
change_type: bugfix
summary: Disable Windows ConPTY capture for Gemini AGY crash rollback
# --->8---
