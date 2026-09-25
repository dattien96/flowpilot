# CA-976 — BUG-483: Drive session-index read failures fail closed

- **Status**: done
- **BUG**: BUG-483 (CP-59 / Task-317)
- **Area**: `apps/local-runner/internal/runner/chat_session_sync.go`

## Symptom

`mergeAndUpsertChatSessionDriveIndexLocked` discarded any download error on
the existing `sessions.ndjson`: `existingIndex, _ = downloadGoogleDriveFileByID(...)`.
A transient read failure produced an empty baseline, the caller's new rows
were merged onto it, and the result was uploaded over the existing index —
silently dropping every other device's synced rows from discovery.

Two adjacent loss modes existed in the same path:

- `mergeChatSessionDriveIndex` silently DROPPED lines it could not interpret
  (malformed JSON, or JSON rows without source identity) — a successful merge
  still destroyed remote bytes.
- Cross-device last-write-wins: a second machine writing between our read and
  our upsert had its rows clobbered.

## Fix

**Fail-closed authority read** (`mergeAndUpsertChatSessionDriveIndexLocked`):

- `findGoogleDriveFile` lookup error → abort (502), no upsert.
- Index file found → download error → abort (502), no upsert.
- Only a confirmed absent index file may merge onto an empty baseline.

**Lossless merge** (`mergeChatSessionDriveIndex`):

- Unparseable lines and identity-less JSON rows are preserved verbatim at the
  tail of the merged file instead of dropped. Output converges: re-merging
  the same input reproduces identical bytes, so the `bytes.Equal` no-op path
  still holds.

**Optimistic concurrency** (bounded):

- Before the upsert, the index is re-read once. If the baseline changed since
  our first read (a concurrent device wrote), our rows are re-merged onto the
  FRESH content — merge-forward instead of clobber. A failed re-read aborts
  for the same fail-closed reason. Bounded to one re-read (documented); the
  in-process `chatSessionIndexLock` still serializes same-process writers.

## Tests

`bug483_drive_index_read_failure_test.go` + two fault hooks on
`fakeChatDriveAPI` (`failDownload`, `onDownload` — additive to the shared
test harness):

- `TestBUG483_IndexDownloadFailureAbortsMerge` — error returned, remote index
  bytes untouched (previously overwrote with only the new row).
- `TestBUG483_IndexNotFoundCreatesFreshIndex` — absent index is the only
  empty-baseline case.
- `TestBUG483_CorruptIndexLinesPreservedThroughMerge` — malformed and
  identity-less lines survive the merge verbatim.
- `TestBUG483_ConcurrentWriterMergedForward` — a simulated second-device
  write between read and upsert is merged forward; no rows lost.

Existing merge oracles (`TestMergeChatSessionDriveIndexLastWinsBySourceIdentity`,
`TestMergeChatSessionDriveIndexIgnoresMalformedLines`) remain green — the
latter counts parseable rows, which preserved lines never inflate.

## Verification

- `go test -run 'TestBUG483|TestMergeChatSessionDriveIndex|TestBUG476'` green.
- Chat-sync surface green.
- Full `internal/runner` suite: only documented env-baseline + timing flakes.

## Hardening note (test-only)

`bug476_drive_chat_legs_test.go` chatIDs were made unique per test
(`chat-476-{manifest,sync,list,restore}`) — a stale background index-repair
goroutine from a previous test can merge its discovered rows through the
shared `httpRequestFn` hook, and identical hardcoded chatIDs let a ghost row
form a second group under the OTHER test's machine id. Test-harness leak
only; production has a single Drive backend.

## Not covered (live phase)

- Real Drive download-failure injection + true two-device concurrent write
  (G-series live tests).
