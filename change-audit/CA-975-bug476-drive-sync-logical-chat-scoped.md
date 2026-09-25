# CA-975 — BUG-476: Drive sync is logical-chat scoped (all provider legs)

- **Status**: done
- **BUG**: BUG-476 (CP-59 / SD-26)
- **Area**: `apps/local-runner/internal/runner/chat_session_sync.go`, `chat_sync_manifest.go`

## Symptom

`syncChatRunToDrive` built one per-run manifest and uploaded only that run plus
its child agents. A multi-provider chat (CP-63 leg model: `ChatID`, `LegSeq`,
`LegState`, `LegClosedReason`, `SwitchFromRunID` on `ProviderSessionState`) had
no leg identity in the manifest and no sibling legs uploaded, so a restore on
another machine could never rebuild one logical chat — only disconnected runs.

## Fix

**Sync side**

- `ChatSessionSyncManifest` (run-level, schema stays **v1**) gains additive
  `chatId`, `legSeq`, `legState`, `legClosedReason`, `switchFromRunId`
  (omitempty — older readers ignore unknown fields; a v1 manifest without
  `chatId` restores explicitly as a single-leg chat).
- `syncChatRunToDrive`: when the run carries `ChatID`, sibling legs are
  enumerated via `ChatSessionReader.ListProviderSessionsByChat` and each leg's
  manifest + provider file + child-agent subtree uploads through the same
  helpers (child loop extracted to `uploadChildAgentManifests`, reused per leg).
- New chat-level envelope `chatSessionChatManifest` written to
  `chat-sessions/chats/<machine>/<chatID>/chat.json` with `schemaVersion=2`
  (`chatSyncManifestSchemaVersion`), matching the pre-existing contract
  "v1 is run-level, v2 is chat-level" (`chat_sync_manifest.go:13-14`).
  It is uploaded **last** — only when every leg landed — so a partial upload
  leaves per-leg index rows but never advertises a complete restorable chat.
- Index rows carry `chat_id`/`leg_seq`; `listRemoteChatSessions` groups legs
  of one `(machine, chatId)` into ONE row — the newest leg (highest `legSeq`)
  represents the chat.

**Restore side**

- `restoreChatRunTreeFromDrive` gains `legMap` (source→local runID across the
  whole restore) + `expandLegs` (top-level only). The requested leg resolves
  its local runID first, then sibling legs restore in `legSeq` order so
  switch chains remap predecessor-first. Sibling failure hard-fails like a
  missing child — silently dropping a leg recreates the exact data loss this
  fixes.
- `remoteChatSiblingRunIDs` prefers `chat.json` (ordered, written only on
  complete upload) and falls back to index rows sharing `chat_id` (partial
  uploads still restore whatever legs exist).
- Restored sessions stamp `ChatID`/`LegSeq`/`LegState`/`LegClosedReason` and
  `SwitchFromRunID` remapped to the predecessor's **local** run id
  (source id kept verbatim when unmapped, for traceability).

**Decoder hardening**

- `ReadChatSyncManifest` probe now prefers v2 only when `sourceRunId` is
  ABSENT — run-level v1 manifests always carry `sourceRunId` and now may also
  carry `chatId` as leg identity. (Pre-existing dormant decoder; no prod
  callers, but the ambiguity was real.)

## Design decisions

- Run-manifest schema deliberately NOT bumped: `chatSessionManifestSchemaVersion`
  stays 1 because leg fields are additive-omitempty and the versioned artifact
  is the chat-level envelope. Bumping would also have broken the
  `chatSyncManifestSchemaVersion==2` v1/v2 discriminator.
- `localFileSessionStore` already inherits `ListProviderSessionsByChat` via the
  embedded `fakeWorkflowStore` — no store changes needed.
- Sibling leg restore failure = hard fail (mirrors child-restore semantics);
  `chat.json` upload failure = best-effort log (index rows remain the fallback
  discovery path).

## Tests

`bug476_drive_chat_legs_test.go` (4 tests, all red→green against the fake
Drive API):

- `TestBUG476_ManifestCarriesChatLegIdentity`
- `TestBUG476_SyncUploadsAllChatLegsAndChatManifest` (3 leg manifests +
  ordered `chat.json` written after, index rows carry chat identity)
- `TestBUG476_RemoteListGroupsLegsAsOneChat` (newest leg represents chat)
- `TestBUG476_RestoreRecreatesWholeChatWithRemap` (two-machine round-trip:
  3 legs restored, leg order + ChatID preserved, `SwitchFromRunID` remapped
  to local ids)

Test-harness note: `scheduleChatSessionIndexRepair` runs a background goroutine
that merges through the global `httpRequestFn` hook — a stale repair from a
previous test can merge its discovered rows into the next test's fake Drive.
List/index assertions are therefore scoped to the machine/chat under test;
the leak is a harness artifact (one real Drive backend in production), not a
product path.

## Verification

- `go test -run 'TestBUG476'` — 4/4 green, 3 consecutive runs stable.
- Chat-sync surface (`ChatSession|ChatSync|Remote|Restore|Sync|BUG31x|BUG32x`)
  green incl. `TestRestoreChatV1Untouched`, `TestChatManifestReaderAcceptsV1`.
- Full `internal/runner` suite: only documented env-baseline (Firebase/Drive
  creds, provider inventory, gitnexus) and timing flakes — all non-baseline
  suspects re-run green in isolation.

## Not yet covered (live phase)

- Real Drive G1-G7 sync/restore + detached reattach across machines.
- Provider parity (Claude/Codex/Grok legs mixed in one chat).
- Chat transcript ledger upload at chat scope (`transcript.ndjson`) — the
  dormant `ChatSyncManifest`/`chatSyncTranscriptDrivePath` contract exists;
  wiring it is follow-up, not blocking the leg-identity fix.
