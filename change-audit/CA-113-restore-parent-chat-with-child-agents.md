# CA-113: Restore Parent Chat With Child Agents

## Scope

- Google Drive remote-chat listing
- Parent and child chat-session manifests
- Parent-triggered child transcript restore
- Restart-safe Agents panel reconstruction

## Completed

- Added parent and agent metadata to synced child manifests and Drive index rows.
- Hid child agent records from the top-level Remote Chats list.
- Added compatibility detection for existing BUG-119 data that has child rows without `parent_run_id`.
- Restored every child referenced by the parent manifest and persisted its remapped parent/agent metadata.
- Preserved collision-safe run IDs and remapped child dependencies.
- Added cycle protection for malformed child graphs.
- Deferred parent persistence until all child restores and metadata updates complete.
- Made child restore failure prevent the parent from appearing in main history.

## GitNexus Impact

- `restoreChatRunFromDrive`: LOW, one direct caller (`handleRestoreChatRun`).
- `listRemoteChatSessions`: LOW, one direct caller (`handleListRemoteChatSessions`).
- `BuildChatSessionSyncManifest`: LOW, one direct runtime caller plus tests.
- `manifestToDriveIndexRecord`: LOW, one direct runtime caller.
- `ChatSessionSyncManifest`: LOW, direct impact through manifest construction and sync.
- `chatSessionDriveIndexRecord`: LOW, direct impact through index serialization.
- GitNexus MCP tools were unavailable; the up-to-date local GitNexus CLI index was used.

## Review

- Full runner tests pass.
- Review found and fixed a persistence-integrity gap: child relationship metadata errors now fail restore instead of silently producing an empty Agents panel after restart.
- Follow-up review fixed early parent publication while child restore was still in progress.
- Independent reviewer-agent dispatch was unavailable because session policy requires explicit user authorization for subagents; a current-agent review loop was used.

## Verification

- `go test ./internal/runner -count=1` — pass.
- `git diff --check` — pass.
- BUG-123 phase-document compliance — pass.

## Residual Notes

- Automated Drive coverage uses the repository fake; a real PC-A-to-PC-B restore remains the final manual smoke test.
- Existing user edits in `AGENTS.md` and `CLAUDE.md` were preserved.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-119
change_type: feature
summary: Restore Parent Chat With Child Agents
# --->8---
