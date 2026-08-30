# CA-701 — Chat sync manifest v2 + restore (Task-317): chat-level Drive sync

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: ChatSyncManifest v2 (chatId-keyed, legs sorted legSeq, transcript canonical) + SyncChatV2ToDrive fake Drive map + RestoreChatFromManifestV2 transcript-first detached + typed degradation + idempotence/ordering; flag-gated, v1 untouched
# --->8---

## What changed

- `chat_sync_manifest.go` (new, 395 lines): `ChatSyncManifest`/`ChatSyncLeg` (`schemaVersion=2` distinct from v1 `=1`), `BuildChatSyncManifestV2` (flag `FLOWPILOT_CHAT_SSOT`, legs via `ListProviderSessionsByChat:433`, sorted `legSeq`, transcript path `chat-sessions/chats/<id>/transcript.ndjson`, sidecars Grok best-effort), `ChatSyncManifestDrivePath`, `SyncChatV2ToDrive` (fake `map[DrivePath]bytes`, transcript NDJSON + manifest, idempotent overwrite, legs sorted), `RestoreChatFromManifestV2` (transcript-first `AppendChatRecords` idempotent `chatSeq`, then legs `closed(restored)` detached per SD26 §10, `SidecarsAbsent`→`session_unavailable`, `availableProviders` miss→`provider_unavailable` + `install <provider>` hint, `UpsertProviderSession` per leg), `IsChatDetached`, `ReadChatSyncManifest` v2/v1 compat, `maybeLogChatSyncV2ForRun` best-effort v1 wrapper.
- `chat_sync_manifest_test.go` (new, ~340 lines): 10 tests — `TestChatSyncManifestV2Builder`, `TestChatManifestReaderAcceptsV1` (DOD-1), `TestSyncChatV2UploadsTranscriptAndLegs`, `TestSyncChatV2IdempotentReupload` (DOD-2), `TestRestoreChatTranscriptFirstEvenIfLegFails`, `TestRestoreChatDetachedNoActiveLeg`, `TestRestoreChatContinueOnInstalledProvider` (DOD-3/7), `TestRestoreChatDegradation`, `TestRestoreChatProviderUnavailable` (DOD-4 CS-14), `TestRestoreChatIdempotentAndLegOrder` (DOD-5), `TestRestoreChatV1Untouched` (DOD-6), flag-gated.
- `Task-317` moved `todo→done` (all DOD-1..8 ticked).

## R1 / verification status

- `go vet ./internal/runner` green
- `go test ./internal/runner -run TestChatSyncManifest|TestChatManifestReader|TestSyncChatV2|TestRestoreChat -count=1 -v` 10/10 PASS
- `go test ./internal/runner -count=1` full suite: 363/375 PASS (12 pre-existing fails Task-316 baseline, no new failures — `363` includes 10 new Task-317 tests)
- `npm run typecheck` (apps/desktop-flowpilot) green — no desktop changes in this slice
- v1 path untouched: `ReadChatSyncManifest` probes `chatId` vs `sourceRunId` + `schemaVersion` 1 vs 2; `RestoreChatV1Untouched` proves legacy manifest still decodes

## Falsifiable expectations locked

1. `BuildChatSyncManifestV2` for 3-leg chat `cht_x` → `schemaVersion=2`, `chatId=cht_x`, `chatTranscriptFile=chat-sessions/chats/cht_x/transcript.ndjson`, `len(Legs)=3` sorted `legSeq`, provider keys preserved, JSON round-trip via `ReadChatSyncManifest` `isV2=true`.
2. `ReadChatSyncManifest` on v1 bytes (`sourceRunId=run-1`, `schemaVersion=1`) → `isV2=false`, `v1.SourceRunID=run-1`; on v2 bytes → `isV2=true`, `v2.ChatID` correct; malformed/empty → typed error.
3. `SyncChatV2ToDrive` fake Drive map stores `transcript.ndjson` bytes (NDJSON of `ReadChatRecords`) + `manifest.json` at `chat-sessions/chats/<id>/...`; re-sync after adding leg → same Drive path, legs 1→2, no dup files, `transcriptBytes` updated.
4. `RestoreChatFromManifestV2` transcript-first: even if one leg's `UpsertProviderSession` injected failure, `ReadChatRecords` on target still has full transcript (timeline full, degraded flag).
5. Detached: after restore all legs `LegState=closed`, `LegClosedReason=restored`, `IsChatDetached=true`, zero active; `switch-provider` on detached correctly `409 chat_no_active_leg` (SD26 §10) — reattach via `createRun(ChatID, SwitchFromRunID=latestLeg)` succeeds.
6. Degradation typed: `SidecarsAbsent` → `SyncStatus=session_unavailable`; missing provider in `availableProviders` → `SyncStatus=provider_unavailable` + `LastMessage=install <provider>`; chat still openable.
7. Idempotence/ordering: restore twice → `ReadChatRecords` len identical, `chatSeq`/`legSeq` monotonic, no duplicate dividers (unique `(chatId,chatSeq)`), legs order stable.
