# BUG-119: Chat Sync Excludes Child Agent Runs

## Metadata

- Document ID: `BUG-119`
- Title: `Chat Sync Excludes Child Agent Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-117: Run ID Reuse After Restart Leaks Old Sub-Agents](./BUG-117-Run-ID-Reuse-After-Restart-Leaks-Old-Sub-Agents-Into-New-Chat.md)
- Replaces: `None`
- Tags: `multi-agent, chat-sync, google-drive, child-agents, restore, runner`

## AI Quick View

### Summary

- Syncing a parent chat (e.g. `run-111` with child agents `run-134`, `run-153`) to Google Drive uploaded only the parent. On another machine the restore showed the main chat but none of the child sub-chats.
- Root cause: `syncChatRunToDrive` built and uploaded a manifest + provider transcript for the single parent run only. The parent manifest recorded the child *summaries* (`ChildAgents`) but each child's provider transcript file, manifest, and index record were never uploaded, so there was nothing to open for the children after restore.
- Fix: after uploading the parent, enumerate `manifest.ChildAgents` and upload each child run's provider file + manifest (via the existing `BuildChatSessionSyncManifest`), and merge every run's record (parent + children) into the Drive `_index`. Best-effort per child so one missing transcript doesn't fail the whole sync.

### Current Ask

- Syncing a chat must include its child agent runs so they are openable on another machine.

### Key Decisions

- `V-1` Extract `uploadChatSessionRunFiles` (provider file + manifest upload for one run) and call it for the parent and each child — each run gets its own `runs/<machine>/<runId>/` Drive folder, keyed by the child's own `SourceRunID`.
- `V-2` `BuildChatSessionSyncManifest` already works per-run (any provider), so children on a different provider than the parent (e.g. Codex children of a Claude parent) sync correctly.
- `V-3` The Drive `_index` merges parent + child records in one read-modify-write so `listRemoteChatSessions` and restore can find children; child local sync status is marked best-effort.
- `V-4` Per-child failures are logged and skipped (a child whose transcript is missing locally must not abort the parent sync).

### Constraints

- Only the parent's direct children (one level, from `listAgentRunSummaries`) are synced; deeper nesting of grandchildren is a follow-up.
- Live Google Drive verification was not run in this environment (no Drive credentials); validated by build/vet and existing mocked sync tests.

### Open Questions

- Should the restore UI nest child sub-chats under the parent, or list them flat? Out of scope here — restore already reconstructs the tree from `ChildAgents`/index `ParentRunID`.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` — `uploadChatSessionRunFiles`, `syncChatRunToDrive`, `BuildChatSessionSyncManifest`

## 1. Issue Summary

A synced chat with child agents only round-tripped the parent. Opening the chat on a second machine showed the main conversation but not the child agent sub-chats.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-117](./BUG-117-Run-ID-Reuse-After-Restart-Leaks-Old-Sub-Agents-Into-New-Chat.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner with Google Drive chat sync connected, a parent run with child agents.
- reproduction steps:
  1. Run a chat that spawns child agents (e.g. `run-111` → `run-134`, `run-153`).
  2. Sync the chat to Drive.
  3. Inspect Drive (or restore on another PC): only the parent run is present.
- frequency: Always, for any chat with child agents.

## 4. Expected vs Actual

- expected: sync uploads the parent and all its child agent runs; restore shows the full tree with openable child chats.
- actual: only the parent provider file + manifest were uploaded; children were referenced as summaries but their transcripts were absent.

## 5. Impact

- users affected: multi-agent users syncing chats across machines.
- severity: High — child agent work is lost on a cross-machine restore.

## 6. Root Cause

- confirmed cause: `syncChatRunToDrive` called `BuildChatSessionSyncManifest(runID)` for the parent only and uploaded that one provider file + manifest + index record. `manifest.ChildAgents` carried child summaries for tree reconstruction, but the children's provider transcript files were never uploaded, so restore had no child content to open.
- evidence: the sync function's upload path operated on a single `manifest`/`providerBytes`; no enumeration of child runs existed.

## 7. Fix Strategy

- `F-1` Extract `uploadChatSessionRunFiles(accessToken, rootFolderID, *manifest, providerBytes)` for one run's provider + manifest upload.
- `F-2` In `syncChatRunToDrive`, upload the parent via the helper, then loop `manifest.ChildAgents`, build each child's manifest (`BuildChatSessionSyncManifest`), upload via the helper, and collect all manifests.
- `F-3` Merge all run records (parent + children) into the Drive `_index` in one pass; mark each child's local sync status best-effort.

## 8. Validation

- `V-1` Go `build` / `vet` clean.
- `V-2` Chat-sync runner tests pass (76 passed; the 3 failures are pre-existing — two `RestoreChatRunFromDriveMissingActiveAccount*` fail identically without this change, one needs the `codex` binary).
- `V-3` Live Google Drive round-trip could NOT be run here (no Drive credentials in this environment); the upload loop reuses the already-tested single-run primitives. Manual cross-machine verification is recommended after rebuild.

## 9. Regression Guard

- tests: existing mocked chat-sync/restore tests cover the parent path and the manifest/index shape.
- audit checks: a synced chat with children whose Drive `runs/<machine>/` folder lacks child run folders would indicate a regression.

## 10. Follow-Up Document Updates

- upstream docs that must change: If CP-19 / Task-082 acceptance specifies child-sync behavior, note that child transcripts (not only summaries) are now uploaded.
- follow-up: sync grandchildren (recursive tree) if deeper agent nesting becomes common; add an integration test with a Drive mock that asserts child provider files are uploaded.
