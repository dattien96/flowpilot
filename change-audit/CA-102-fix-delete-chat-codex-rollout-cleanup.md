# CA-102 - Fix Delete Chat Codex Rollout Cleanup

## Scope

Fix local chat deletion so Codex chats remove the full local rollout chain across registered Codex account homes, not just the stable stored `provider_session_id` file.

## Completed

- Updated `apps/local-runner/internal/runner/interactive_resume.go`.
  - `deleteChatSession` now gathers all provider session ids that belong to the run before deleting provider files.
  - Added `deleteSessionIDsForRun` to combine:
    - the stable `ProviderSessionID`
    - every Codex `turnLogKindCodexSession` id from the run turn log
  - Provider-file cleanup now iterates all collected ids across every registered account home for the run's provider.

- Added regression coverage in `apps/local-runner/internal/runner/cross_account_resume_test.go`.
  - `TestDeleteChatSessionRemovesCodexStableAndTurnLogRolloutsAcrossAccounts`
  - Verifies deletion removes:
    - the stable Codex rollout
    - later Codex per-turn rollouts from the turn log
    - copies under multiple account homes
    - the local session record
    - the turn-log sidecar

## Verification

- GitNexus impact: `deleteChatSession` -> LOW risk, 1 direct caller (`handleDeleteRun`), 0 affected processes.
- Targeted pass:
  - `go test ./internal/runner -run 'TestDeleteChatSessionRemovesCodexStableAndTurnLogRolloutsAcrossAccounts' -count=1 -v`
- `npx gitnexus detect_changes` is not available in the installed CLI; scope was checked with `git status --short`.

## Residual Notes

- Google Drive chat-session sync artifacts are not deleted by this local delete path.
- Provider-file deletion remains best-effort by design; history removal still succeeds even if a file cannot be removed.
