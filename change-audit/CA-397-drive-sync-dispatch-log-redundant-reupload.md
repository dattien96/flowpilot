# CA-397: Batch chat-sync no longer re-uploads the whole dispatch log per run

## Summary

Found live during a CP-51 C6 (Drive sync) verification session on 2026-07-22
(`task_d80cf120`): syncing gate-sandbox's 17 chats took 6-8 minutes. The runner
log showed the same 3.4MB per-project `dispatch.ndjson` re-uploaded to Google
Drive **9 times** (identical byte count each time) in about 6 minutes.

## Root cause

`syncChatRunToDrive` (`chat_session_sync.go:537`) runs once per run inside a
project-wide batch sync — the desktop's `syncRuns` loop (`store.ts`) calls the
per-run sync endpoint once for each unsynced chat, sequentially. Every single
one of those calls unconditionally re-exported (`hub.ExportProjectLog`, the
entire per-project log, not a per-run delta) and re-uploaded
(`upsertGoogleDriveFile`) the whole `dispatch.ndjson` via
`syncDispatchLogToDrive` (`dispatch_drive_sync.go:21`) — no cache, no hash
check, no dedup across calls in the same batch.

## Fix

`syncDispatchLogToDrive` now caches the sha256 (`HashBytes`, already used
elsewhere for canonical hashing) of the last successfully-uploaded dispatch log
per project, in a new `InteractiveService.dispatchLogSyncHash` map guarded by
its own dedicated `dispatchLogSyncMu` (kept separate from the main `s.mu` —
Drive uploads are slow network I/O unrelated to run-state locking). The actual
Drive round-trip (`ensureGoogleDriveFolderPath` + `upsertGoogleDriveFile`) is
skipped when the freshly-exported bytes hash-match what was last uploaded for
that project; the cache updates only after a successful upload. Since
`dispatch.ndjson` is append-only (confirmed across this whole session's live
testing — it only grows, never reverts to a prior byte-for-byte state), a
hash match reliably means "nothing new to upload," not a false negative.

Scoped to exactly the confirmed bug: no change to `syncChatRunToDrive`'s
per-run session/manifest/index upload behavior, no change to the restore path
(`restoreDispatchLogFromDrive`), no change to the batch-loop's per-run
architecture on either side (desktop or backend) — this is a backend-only,
content-hash gate that works correctly regardless of caller pattern.

## Not in scope (flagged separately in the same background task)

An unexplained desktop unsynced-count regression was also observed live during
the same batch (progress appeared to go from ~12/17 synced back to showing 15
unsynced) — not root-caused in this pass. It may be fully explained by this fix
(redundant network I/O making the batch slow enough to look like it stalled) or
may be a separate frontend progress-tracking issue; `task_d80cf120` still notes
it for further investigation if it recurs after this fix.

## additive-tests-only compliance

New file only: `dispatch_drive_sync_test.go` (this code path had zero prior
test coverage — confirmed via search before writing). No existing test file
touched. `TestRestoreChatRunFromDriveMissingActiveAccountHome` was found
failing in `chat_session_sync_test.go` during the regression sweep; confirmed
via `git stash` (production changes reverted, new test file's presence doesn't
affect that unrelated test) that it **fails identically on baseline** — a
pre-existing issue, unrelated to and not modified by this change.

## Verification

- `go build ./...`: clean.
- `go vet ./internal/runner/`: clean.
- New tests (`go test -run TestSyncDispatchLogToDrive -v`): 4/4 pass —
  unchanged-content skip, changed-content re-upload, per-project cache scoping
  (deliberately identical content across two different projects, to catch a
  content-hash-only cache that isn't actually keyed per project), and a failed
  upload not poisoning the cache (added after a follow-up review question —
  without this, a transient Drive error would make the next unchanged-content
  attempt wrongly no-op instead of retrying, permanently losing that upload).
- Regression sweep, `chat_session_sync_test.go` + related (`TestSync*`,
  `TestChatSession*`, `TestRestoreChatRun*`, `TestDeleteRun*`): 43 pass, 1 fail —
  the pre-existing `TestRestoreChatRunFromDriveMissingActiveAccountHome` failure,
  reconfirmed present on baseline via `git stash`, unaffected by this change.
- Broader CP-51 durable-dispatch bundle (`TestDispatch*`, `TestCrashMatrix*`,
  `TestRecoveryScanner*`, `TestTurn*`, `TestInteractive*`, etc.): all pass, no
  collateral impact from the `InteractiveService` struct field addition — this
  change only touches the best-effort Drive export/upload side channel
  (`chat_session_sync.go:598`, "best-effort — session sync still wins"), never
  the CAS/recovery state machine that Phase A/B and C1-C17 exercise.
- No provider branching in the changed code (`dispatch_drive_sync.go` has zero
  references to Codex/Grok/Claude): the fix operates on the project-wide
  dispatch log regardless of which provider generated its entries, so it
  benefits all three providers identically.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: task_d80cf120
change_type: bugfix
summary: Cache the per-project dispatch-log content hash so a batch chat-sync uploads it once instead of re-uploading the whole file once per run (9x redundant 3.4MB uploads confirmed live for a 17-chat batch).
# --->8---
