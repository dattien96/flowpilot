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

## Decision Flow

```
Does a local file exist at targetPath?
│
├─ NO (ErrNotExist)
│   → write from Drive via RestoreSessionFile   (normal first restore)
│
├─ YES, read OK
│     codexExtend = (manifest.ProviderKey == ProviderKeyCodex)
│
│     ┌─ hash(local) == remote SHA256
│     │   → identical — skip write entirely
│     │
│     ├─ codexExtend && bytes.HasPrefix(remote, local)
│     │   → remote has MORE turns than local
│     │   → overwrite local with newer remote bytes
│     │
│     ├─ codexExtend && bytes.HasPrefix(local, remote)
│     │   → local has MORE turns than remote
│     │   → keep local file untouched; set localAhead = true
│     │
│     └─ anything else
│         → session_file_conflict  ← only true conflict
│
└─ YES, read failed (not ErrNotExist)
    → workflow_state_unavailable
```

**Why `bytes.HasPrefix` works:** Codex rollout files are JSONL — each turn appends a new line. If the remote file is ahead, it starts with every byte the local file has and then has extra lines after. `HasPrefix(remote, local)` captures exactly that. The reverse means the local machine continued the chat after the Drive snapshot.

**What `codexExtend` is:** a local boolean (`codexExtend := manifest.ProviderKey == ProviderKeyCodex`) that gates the two `HasPrefix` branches. Only Codex gets prefix-extension logic; other providers fall through to `default`.

**Why Claude was unaffected by the original bug:** The old strict hash-equality check applied to all providers. In theory a Claude hash mismatch would also fail with `session_file_conflict`. In practice Claude never reaches that condition: Claude does not use an append-only rollout file that grows turn-by-turn the way Codex does, so the "local file exists with different bytes" scenario does not arise during Claude restores. The fix does not change Claude's path at all.

## Residual Notes

- Prefix-extension acceptance is intentionally Codex-only. If Claude session-file append semantics are later confirmed, the gate can be widened with dedicated tests.
- This change narrows the definition of "conflict" only; genuinely divergent rollout chains sharing a session id are still rejected and never silently merged.
- Source review came from a Codex review loop; findings on stale metadata, missing direct tests, and the Codex/Claude gate were all incorporated.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-091
change_type: fix
summary: Fix Drive Restore Prefix-Extension Conflict
# --->8---
