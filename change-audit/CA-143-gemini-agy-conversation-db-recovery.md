# CA-143: Gemini AGY Conversation DB Recovery

## Summary

Fixed Gemini desktop chat turns that printed an AGY answer in the runner console but rendered nothing in the UI because captured stdout/stderr were empty.

## What Changed

- Added a Gemini-only recovery helper that maps the current workspace through AGY's `last_conversations.json`, reads the latest assistant `steps.step_payload` from the conversation SQLite DB using local `sqlite3`, and extracts the assistant Markdown from protobuf-style length-delimited strings.
- Applied the recovery after empty AGY capture in `geminiAdapter.SendTurn`, Gemini session sends, `Runner.ExecutePrompt`, and Gemini summarization.
- Added regression tests for adapter recovery and AGY payload extraction.

## Verification

- `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize' -count=1`

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-148
change_type: bugfix
summary: Recover Gemini AGY responses from conversation DB when stdout is empty
# --->8---
