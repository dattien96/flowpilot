# BUG-483: Drive session-index download failure is treated as an empty index and can overwrite remote entries

## Metadata

- Document ID: `BUG-483`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `fixed — unit+fault-injection fake-Drive green (CA-976); live pending`
- Feature Keys: `chat-history`
- Parent Documents: `CP-59`, `Task-317`
- Related Documents: `BUG-476`, `B-59-6`
- Affected Area: `internal/runner/chat_session_sync.go`

## Summary

The Drive `sessions.ndjson` merge path finds an existing index, but discards any
download error. `existingIndex` remains empty, new records are merged into that
empty byte slice, and the result is uploaded over the existing index. A
transient read failure can therefore remove every previously indexed remote
session from discovery.

## Evidence

```go
if existing, findErr := findGoogleDriveFile(...); findErr == nil && existing.ID != "" {
    existingIndex, _ = downloadGoogleDriveFileByID(...)
}
...
upsertGoogleDriveFile(..., merged, ...)
```

The per-project mutex prevents same-process writers from racing but does not
make a failed remote read safe. Manifest discovery may later rebuild some rows,
but that is asynchronous/best-effort and does not justify destructive
replacement.

## Expected vs Actual

- Expected: read-merge-write aborts if the existing authority cannot be read;
  retry later without changing remote index.
- Actual: unreadable existing content is treated as a valid empty baseline.

## Impact

Remote chats can disappear from the sync listing on all devices. Provider blobs
may still exist, but users lose discoverability until a successful repair scan,
and partial discovery may not reconstruct the intended chat-level ordering.

## Required Fix Contract

1. Distinguish index-not-found from index-download failure.
2. Abort upload on download/parse/integrity failure.
3. Use revision/etag or another conflict guard for cross-device writers.
4. Keep manifest discovery as repair, not as permission to overwrite unknown
   authority.

## Required Tests

- RED existing-index download failure performs no upsert.
- Not-found creates a fresh index normally.
- Concurrent device revision conflict retries merge instead of last-write-wins.
- Corrupt index fails closed and preserves original bytes.
- Live Drive test with injected transient download failure.

## Implementation Plan

### P-1 — RED remote-read tests

- Extend chat sync tests with Drive fakes for: file absent, find failure,
  download failure, corrupt NDJSON and stale revision/etag.
- Seed multiple existing index rows and assert current download-failure path
  uploads only the new row, demonstrating destructive replacement.

### P-2 — Fail-closed read/merge/write

- Refactor index load into a result that distinguishes `not_found`, valid bytes
  and read/integrity failure.
- Only `not_found` may use an empty baseline. Any other failure aborts before
  `upsertGoogleDriveFile`.
- Parse/validate all existing rows before merge; preserve unknown additive
  fields where the format permits.

### P-3 — Cross-device concurrency

- Capture Drive revision/etag when reading and use conditional update if the
  API supports it. On conflict, refetch, merge and retry with a bounded cap.
- If conditional update is unavailable, use a versioned immutable index
  generation plus a small compare-and-swap pointer/manifest; do not rely only
  on the per-process mutex.

### P-4 — Repair path

- Manifest discovery may rebuild a missing/corrupt index, but it must produce a
  reviewed candidate and atomically replace only after complete discovery.
- Report repair status rather than silently swallowing asynchronous merge error.

## Definition of Done

- [ ] Download/find/parse failures perform zero remote index writes.
- [ ] True not-found creates a fresh index normally.
- [ ] Existing rows survive merge byte/field-equivalently.
- [ ] Cross-device writer conflict refetches or fails typed; no last-write loss.
- [ ] Repair discovery cannot publish a partial index as complete.
- [ ] Remote listing remains available after transient failure and retry.
- [ ] Real Drive fault drill preserves every pre-existing session row.
- [ ] CP-59 sync/restore and BUG-476 chat-level tests pass together.

## Resolution

Fixed in CA-976 (`mergeAndUpsertChatSessionDriveIndexLocked` +
`mergeChatSessionDriveIndex`):

- Lookup/download failures on the existing index abort before upsert; only a
  confirmed absent index merges onto an empty baseline.
- Merge is lossless: malformed / identity-less lines preserved verbatim.
- Optimistic concurrency: index re-read once before upsert; changed baseline
  re-merges our rows onto fresh content (merge-forward, no clobber); failed
  re-read aborts.

Tests: `bug483_drive_index_read_failure_test.go` (4 tests + `failDownload` /
`onDownload` fault hooks on the fake Drive API).
