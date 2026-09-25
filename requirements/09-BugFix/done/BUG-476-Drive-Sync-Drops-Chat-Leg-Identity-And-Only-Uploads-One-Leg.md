# BUG-476: Drive sync is run-scoped and omits chat-leg identity — restored cross-provider chat cannot remain one chat

## Metadata

- Document ID: `BUG-476`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `fixed — unit+e2e fake-Drive green (CA-975); real-Drive live test pending`
- Feature Keys: `chat-history`, `cross-provider-handoff`
- Parent Documents: `CP-59`, `SD-26`, `Task-317`
- Related Documents: `B-59-4`, `B-59-6`, `BUG-405`
- Affected Area: `internal/runner/chat_session_sync.go`

## Summary

CP-59 defines the logical `chatId` as the sync/restore unit and a chat as an
ordered set of provider legs. The Drive manifest remains a legacy per-run
manifest: it has `SourceRunID` but no `ChatID`, `LegSeq`, `LegState`,
`LegClosedReason`, or `SwitchFromRunID`. `syncChatRunToDrive` uploads the chosen
run plus child agents, not sibling legs belonging to the same chat.

A restored multi-provider chat therefore lacks the data needed to reconstruct
one ordered chat and its switch lineage.

## Evidence

- `ProviderSessionState` durably carries all five chat-leg fields.
- `ChatSessionSyncManifest` omits all five fields.
- Manifest construction copies run/provider/flow fields but not chat-leg fields.
- `syncChatRunToDrive(runID, ...)` builds one parent manifest and iterates only
  `manifest.ChildAgents`; it never calls `ListProviderSessionsByChat(chatID)`.
- CP-59 Test-Steps explicitly defers Google Drive; B-59-4/B-59-6 remain blocked
  by credentials, so no live round-trip disproves this code-level gap.

## Expected vs Actual

- Expected: syncing any leg syncs the logical chat envelope, ordered legs,
  transcript/divider records and leg lifecycle; restore recreates one `chatId`.
- Actual: sync unit is one run tree; sibling provider legs and lineage are absent.

## Impact

- Cross-provider history may restore as separate chats or only the selected leg.
- Provider-switch dividers and predecessor lineage can be lost.
- First-turn reattach may not build the correct all-leg handoff envelope.
- Remote reconciliation cannot determine which uploaded runs are legs of the
  same logical chat.

## Required Fix Contract

1. Version the manifest and include chat identity + ordered leg metadata.
2. Sync all legs and the chat transcript ledger atomically or with a recoverable
   manifest protocol; child-agent trees remain attached per leg.
3. Restore with collision-safe `chatId/runId` remapping and predecessor remap.
4. Legacy run manifests remain readable with explicit single-leg semantics.

## Required Tests

- RED local manifest round-trip for a three-leg chat.
- Restore remaps `SwitchFromRunID` and preserves leg ordering/states.
- Partial upload/retry never advertises a complete chat manifest.
- Same-drive-root remote listing groups restored legs as one chat.
- Live Drive G1-G7 plus detached reattach using real credentials.

## Implementation Plan

### P-1 — Versioned chat manifest design

- Bump `chatSessionManifestSchemaVersion` and add `ChatID`, `LegSeq`,
  `LegState`, `LegClosedReason`, `SwitchFromRunID` to per-leg records.
- Add a chat-level envelope listing ordered leg manifests and the durable chat
  transcript ledger checksum/range. Mark completion only after every required
  object uploads.
- Specify legacy v1 as a one-leg chat whose `chatId` is deterministically
  synthesized; never guess sibling membership.

### P-2 — Sync by logical chat

- Resolve the selected run, then call `ListProviderSessionsByChat(chatID)` and
  sort by `LegSeq`.
- Upload each leg's provider state, turn log and child-agent tree under stable
  paths; upload transcript/divider records once at chat scope.
- Publish the complete chat manifest last. Partial uploads remain discoverable
  only as incomplete/retryable, not a restorable chat.

### P-3 — Restore and remap

- Allocate collision-safe local run IDs for every leg before writing any row.
- Remap `SwitchFromRunID`, parent/child relationships and transcript leg IDs in
  one staged mapping table.
- Persist all legs and chat transcript atomically where possible; otherwise use
  staged files + a final commit marker with idempotent replay.
- Reattach the latest eligible leg while preserving closed/restored state on
  older legs.

### P-4 — UI and reconciliation

- Remote listing groups by chat manifest, not by independent run rows.
- Sync status and retry apply to the chat while retaining per-leg diagnostics.
- A restored chat opens one timeline with exactly one divider per switch.

### P-5 — Verification

- Unit fixtures: 3 providers, 3 legs, child agent on leg 2, transcript spanning
  all legs, run-ID collisions on target machine.
- Real Drive: sync on machine A, restore machine B, reopen and send a turn that
  recalls prior-leg history.

## Definition of Done

- [ ] Manifest v2 carries complete chat and leg identity.
- [ ] Syncing any leg uploads every sibling leg and the chat transcript ledger.
- [ ] Partial upload cannot advertise a complete restorable chat.
- [ ] Restore preserves leg order/state and remaps every cross-run reference.
- [ ] Legacy manifests restore explicitly as single-leg chats.
- [ ] Remote listing groups the restored result as exactly one chat.
- [ ] Detached reattach sends a correct all-leg handoff envelope.
- [ ] B-59-4 and B-59-6/G1-G7 pass with real Drive credentials.
- [ ] Provider parity is verified for at least Claude, Codex and Grok legs.

## Resolution

Fixed in CA-975:

- Run manifest keeps schemaVersion 1 with additive `chatId`, `legSeq`,
  `legState`, `legClosedReason`, `switchFromRunId` fields.
- `syncChatRunToDrive` uploads every sibling leg (manifest + provider file +
  each leg's child subtree) via `ListProviderSessionsByChat`; chat-level
  `chat.json` (schemaVersion 2) is written last as the completeness marker.
- Index rows carry `chat_id`/`leg_seq`; remote listing groups legs as one
  chat (newest leg represents it).
- Restore expands siblings in legSeq order through a shared source→local
  `legMap`; `SwitchFromRunID` remaps to local run ids; sibling failure
  hard-fails.
- `ReadChatSyncManifest` probe disambiguated: v2 only when `sourceRunId`
  absent, so chatId-bearing leg manifests still decode as v1.

Tests: `bug476_drive_chat_legs_test.go` — manifest identity, all-legs upload +
commit marker, remote grouping, two-machine round-trip with remap.
