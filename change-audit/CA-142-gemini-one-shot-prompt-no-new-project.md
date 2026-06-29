# CA-142: Gemini One-Shot Prompt No New Project

## Summary

Fixed Gemini one-shot prompt execution so `Runner.ExecutePrompt` and Gemini summarization do not launch AGY print mode with `--new-project` after FlowPilot has already resolved or bootstrapped the workspace project config.

## What Changed

- Updated `resolvePromptExecutionAdapter` to call `geminiCLIArgs` with `createProject=false` for Gemini after `geminiSessionProjectID` returns the workspace project id.
- Added `TestResolvePromptExecutionAdapterGeminiOmitsNewProjectAfterConfigBootstrap` to verify the one-shot path uses `--project <uuid>` without `--new-project` or `--continue`.

## Verification

- `go test ./internal/runner -run 'TestResolvePromptExecutionAdapter|TestGeminiAdapter|TestStartSessionGemini|TestSendMessageGemini|TestCreateRunGeminiRequiresUsableWorkspacePath|TestSendTurnWithRetryDoesNotRetryGeminiWorkspaceRequired' -count=1`

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-147
change_type: bugfix
summary: Remove AGY --new-project from Gemini one-shot prompt launches
# --->8---
