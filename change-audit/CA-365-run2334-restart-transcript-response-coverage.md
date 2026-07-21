# CA-365 - run-2334 restart transcript response coverage

## Summary

Added a restart-path regression test for the missing assistant-response
coverage in CA-363. The test persists a completed normal chat, recreates the
interactive service as a server restart would, calls `resumeRun`, and verifies
that both the durable raw prompt and assistant response are replayed.

The matrix covers Codex, Claude, and Grok. It protects the shared resume path,
not just an individual transcript loader or direct seed helper.

## Verification

- `go test ./internal/runner -run '^(TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider|TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors|TestRun2334ClaudeRestartAnchorsSidecarsByDurableTurnID|TestRun2334CodexRestartAnchorsSidecarsByDurableTurnID|TestSeedGrokTranscriptFromDiskReplaysViaTurnLog)$' -count=1 -v`
  passed: 8 tests.
- Restarted the local runner and resumed the real `run-2334`. Its event stream
  grew from the stale partial 15 events to 59 events, including assistant
  messages, tool events, approvals, the answer card, and terminal completion.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-51
change_type: bugfix
summary: Add cross-provider restart coverage for normal-chat assistant transcript replay.
# --->8---
