# CA-555: REMOTE CHATS list reads Drive index first

## Problem

`GET /client/projects/{id}/chat-sessions/remote` walked every
`chat-sessions/runs/<machine>/<run>/manifest.json` (CA-404 discover) and then
downloaded parent manifests again (BUG-123 legacy child scan) before writing
response headers. TUI `ResponseHeaderTimeout` is 60s, so a project with ~56
synced chats timed out:

`Remote chat list failed: ... timeout awaiting response headers`

Desktop `loadRemoteChatSessions` (store.ts) hits the same runner GET.

## Fix (runner only — Desktop + TUI share the GET)

- `readChatSessionDriveIndex` downloads `_index/sessions.ndjson` only.
- `loadChatSessionDriveIndexRecords`: non-empty index is returned immediately
  (no `runs/` walk). Empty/missing index still discovers + merges
  synchronously (CA-404 orphan-manifest repair stays on the request path).
- Non-empty index schedules one background discover+merge per project
  (`chatSessionIndexRepairing` debounce, same per-project mutex).
- BUG-123 parent-manifest scan runs only when **no** index row has
  `ParentRunID` (legacy BUG-119 shape). A modern index already marks children.

List = read index. Restore still downloads the real manifest + provider file.

## Provider parity

Case 1 agnostic: `listRemoteChatSessions` / `readChatSessionDriveIndex` /
`scheduleChatSessionIndexRepair` take no `providerKey` and never branch on one.
Claude / Codex / Grok rows are the same ndjson lines.

## Will not undo

- CA-404: empty index + orphan `runs/` folders still repair on the same request.
- CA-418 / CA-407: restore path unchanged.
- BUG-123 `TestListRemoteChatSessionsHidesChildAgentRecords`: legacy index
  (ParentRunID stripped) still scans parent manifests.

## Tests (additive)

`ca555_remote_list_index_fastpath_test.go`:

- index present + extra orphan → list is index-only (Claude/Codex/Grok)
- empty index + mixed-provider orphans → CA-404 sync repair
- modern index hides children without planting parent manifests
- legacy index still hides children via BUG-123 scan
- background repair merges the skipped orphan into `sessions.ndjson`
- `chatSessionIndexHasParentRunID` helper

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run "TestCA555|TestListRemote|TestSyncChat|TestRestoreChat|TestMergeChatSession" -count=1
```

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: CP-51
change_type: bugfix
summary: list remote chats from Drive sessions.ndjson first; discover only when index empty
# --->8---
