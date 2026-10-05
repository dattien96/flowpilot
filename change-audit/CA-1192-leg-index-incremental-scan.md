# CA-1192 — incremental LegIndex + history scan gating (project load timeout)

## Defect

"Loading project" hung and timed out: `GET /client/projects/{id}/workflow-runs`
for PrivateVault never returned (>90s curl). Root cause chain:

- `projectRunHistory` → `stampMissingChatIdentity` → `transcriptLegIndex` →
  `localFileChatTranscriptStore.LegIndex` opened and unmarshalled **every**
  `~/.flowpilot/chat-transcripts/chats/*/transcript.ndjson` — 69,121 files,
  285MB — on every call, uncached.
- The desktop calls this endpoint on the active-project poll (3s/10s) and on
  `loadAllProjectHistories` (30s × N projects), so the runner was continuously
  scanning. The desktop `getJSON` aborts at 30s and `withRetry` re-fires, so
  the scans queued behind `l.mu` — which also serialized live `AppendChatRecords`
  / `ReadChatRecords`, stalling chat timelines during the pile-up.
- `projectRunHistory` additionally listed provider sessions **twice** per call.

## Fix

- `localFileChatTranscriptStore.LegIndex` keeps a per-chat scan cache keyed by
  transcript mtime+size: `ReadDir` + `Stat` only; unchanged transcripts are
  not re-read; new/changed chats rescan once; deleted chats drop their legs.
  `legScanReads` counter proves the skip in tests.
- `chatLegsFromRecords(chatID, recs)` extracted — same leg-order + switch-payload
  legSeq semantics as the old inline scan.
- `stampMissingChatIdentity` early-outs when no history row needs a stamp
  (all modern rows carry ChatID), keeping the endpoint off the transcript tree
  entirely in the common case.
- `projectRunHistory` reuses the session listing already fetched for sync
  fields instead of a second `ListProviderSessionsByProject`.

## Regression evidence

- `bug1192_leg_index_scan_cost_test.go`: unchanged-tree second call performs
  zero transcript reads; single-chat append rescans exactly one transcript and
  picks up the new switch leg; deleted chat drops its legs; stamp skipped when
  all rows identified (LegIndex call count 0) and still runs once for legacy
  rows; `projectRunHistory` issues exactly one session listing.
- Existing pinned contracts green: `TestProjectHistory*` (BUG-338 backfill
  semantics incl. 3-leg stamp + legSeq from switch payloads),
  `TestLocalFileSessionStoreRoundTripsChatId`, transcript store tests.

## Files

- `apps/local-runner/internal/runner/chat_transcript_store.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/bug1192_leg_index_scan_cost_test.go`
