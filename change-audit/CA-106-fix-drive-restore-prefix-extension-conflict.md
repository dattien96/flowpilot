# CA-106 - Fix Drive Restore Prefix-Extension Conflict

## Scope

Fix `restoreChatRunFromDrive` so a Codex chat restore accepts a same-session prefix-compatible rollout file (one side has more appended turns than the other) instead of rejecting it as `session_file_conflict`. Preserve newer local history metadata when the local file is ahead. Tracked as BUG-091.

## Completed

- Updated `apps/local-runner/internal/runner/chat_session_sync.go`.
  - Added `"bytes"` import.
  - Replaced the single hash-equality conflict guard at the restore target path with a four-branch decision:
    - identical hash -> no write
    - Codex and remote `bytes.HasPrefix` local -> overwrite local with newer remote
    - Codex and local `bytes.HasPrefix` remote -> keep local (set `localAhead`)
    - otherwise (or any non-Codex mismatch) -> `session_file_conflict`
  - Prefix-extension acceptance is gated to `ProviderKeyCodex`; the restore branch is provider-generic and Claude append semantics are unconfirmed, so non-Codex providers keep strict equality.
  - When `localAhead`, preserve the existing local session's non-empty `LastPrompt`, `LastMessage`, `Status`, `UpdatedAt` (read via `SessionHistoryReader.GetProviderSession`) so the older remote manifest does not downgrade local history.

- Added regression coverage in `apps/local-runner/internal/runner/chat_session_sync_test.go`.
  - `TestRestoreChatRunFromDriveOverwritesWhenRemoteExtendsLocal`
  - `TestRestoreChatRunFromDriveKeepsLocalWhenLocalExtendsRemote`
  - `TestRestoreChatRunFromDrivePreservesLocalMetadataWhenLocalAhead`

## Verification

- Targeted pass:
  - `go test ./internal/runner/... -run "TestRestoreChatRunFromDrive" -count=1` -> 14 passed
  - `go test ./internal/runner/... -run "TestRestore|TestChatSession|TestSync|TestRelocate|TestResolveRestored" -count=1` -> 48 passed
- `go vet ./internal/runner/` -> no issues found.
- Existing `TestRestoreChatRunFromDriveRejectsOverwriteConflict` still returns `session_file_conflict` for genuinely divergent content.

## Residual Notes

- Prefix-extension acceptance is intentionally Codex-only. If Claude session-file append semantics are later confirmed, the gate can be widened with dedicated tests.
- This change narrows the definition of "conflict" only; genuinely divergent rollout chains sharing a session id are still rejected and never silently merged.
- Source review came from a Codex review loop; findings on stale metadata, missing direct tests, and the Codex/Claude gate were all incorporated.
