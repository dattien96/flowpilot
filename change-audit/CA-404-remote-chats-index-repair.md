# CA-404: REMOTE CHATS index under-list repair

## Summary

Gate-sandbox Drive had many uploaded chat run folders under
`chat-sessions/runs/mch_*` (e.g. 77 on Windows machine) while
`GET .../chat-sessions/remote` returned only 1 top-level row. REMOTE CHATS
reads `_index/sessions.ndjson` only; concurrent sync last-write-wins and/or
lost index rows left blobs without catalog entries.

## Change

- Per-project mutex around Drive `sessions.ndjson` read-merge-write in
  `syncChatRunToDrive` (prevents concurrent last-write-wins).
- `listRemoteChatSessions` discovers `runs/<machine>/<run>/manifest.json` and
  merges missing rows into the index (rewrite when needed), then applies the
  existing parent-only / child-hide filter (BUG-119/123).
- Safe count logging: `index_rows` + `discovered_manifests` (no transcript).

## Provider parity

Provider-agnostic path: no `providerKey` branch in index merge, discover, or
list. Manifests for Claude/Codex/Grok all become index rows the same way.

## Tests (additive)

- `TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`
- `TestListRemoteChatSessionsRepairsIndexFromOrphanedRunManifests`
- `TestListRemoteChatSessionsHidesChildrenButKeepsSiblingParentsAfterRepair`
- `TestListRemoteChatSessionsReturnsAllTopLevelRowsAfterMultiRunSync`

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'TestListRemote|TestSyncChat|TestRestoreChat|TestMergeChatSession|TestChatSession' -count=1
```

Live: restart runner, open Gate-sandbox REMOTE CHATS — should list top-level
parents rebuilt from Windows+Mac run folders (children still hidden).

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: CP-51
change_type: bugfix
summary: repair REMOTE CHATS list by mutexing index merge and rebuilding sessions.ndjson from run manifests
# --->8---
