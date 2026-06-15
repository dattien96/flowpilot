# CA-068 Fix Claude Session Usage Limit Normalization

## Scope

Record the follow-up Claude quota fix that extends usage-limit normalization into the session/chat execution path after BUG-054 proved the workflow-adapter-only fix was not enough.

## Completed

- Updated `apps/local-runner/internal/runner/sessions.go` so the `claude_stream_json` session execution branch now checks `claudeUsageLimitError(accountHomePath)` before starting the Claude print command.
- Added Claude usage-limit normalization for stderr-based failures in the same session execution path.
- Added Claude usage-limit normalization for result-frame failures in the same session execution path, so raw `/login` guidance is replaced when quota metadata is present.
- Added focused regression tests in `apps/local-runner/internal/runner/sessions_test.go` for:
  - preflight metadata usage-limit detection without launching the Claude command
  - result-frame quota normalization when Claude returns misleading login-looking text

## Verification

- Ran `go test ./internal/runner -run "Test(SendMessageClaudePreflightUsageLimitDoesNotRunCommand|SendMessageClaudeResultLimitDoesNotReportLogin|MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata)$"` in `apps/local-runner`

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- This fix covers the Claude session/chat execution path in addition to the workflow-adapter path already addressed by BUG-051 and BUG-054.
- The repository currently contains an untracked earlier audit file named `CA-067-fix-claude-usage-limit-message-trailing-login-phrase.md`; this new note intentionally uses `CA-068` to avoid colliding with the existing `CA-067` task audit numbering.
