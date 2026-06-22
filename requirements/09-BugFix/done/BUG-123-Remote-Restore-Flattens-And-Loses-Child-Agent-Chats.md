# BUG-123: Remote Restore Flattens And Loses Child Agent Chats

## Metadata

- Document ID: `BUG-123`
- Title: `Remote Restore Flattens And Loses Child Agent Chats`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-119: Chat Sync Excludes Child Agent Runs](./BUG-119-Chat-Sync-Excludes-Child-Agent-Runs.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [CA-113](../../../change-audit/CA-113-restore-parent-chat-with-child-agents.md)
- Replaces: `None`
- Tags: `multi-agent, chat-sync, restore, google-drive, desktop, regression`

## AI Quick View

### Summary

- Remote Chats showed the parent and every child agent as separate top-level rows.
- Restoring the parent restored only its transcript and transient child summaries; child transcripts were not restored and the Agents panel became empty after runner restart.
- The fix filters child records from the remote root list and restores/persists the complete parent-child chat tree from the parent action.

### Current Ask

- Show only main chats in Remote Chats and restore their child agent chats automatically when the main chat is restored.

### Key Decisions

- `V-1` Child runs remain independent Drive manifests and transcript files, but are not top-level restore choices.
- `V-2` Parent restore downloads every child referenced by `ChildAgents`, remaps collision-safe run IDs, and persists agent relationship metadata.
- `V-3` Legacy BUG-119 index rows without `parent_run_id` are classified through their parent manifests.
- `V-4` The parent is persisted to main history only after all referenced children restore successfully.
- `V-5` A missing or damaged child fails the restore and keeps the parent out of main history.

### Constraints

- Google Drive child upload remains best-effort, matching BUG-119.
- Existing synced data must work without requiring users to re-sync all child chats manually.
- A parent manifest that references an unavailable child remains unrestored until that child is available or the parent is re-synced.
- User changes in `AGENTS.md` and `CLAUDE.md` remain outside this fix.

### Open Questions

- Live PC-A-to-PC-B Google Drive verification remains recommended because automated tests use the Drive fake.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/chat_session_sync_test.go`
- User report on PC B, `2026-06-22`

## 1. Issue Summary

Remote restore treated child agents as independent root chats and did not restore their provider transcripts when the user restored the parent chat. The in-memory summary-only reconstruction was insufficient for opening child chats and did not survive a runner restart.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: `None identified; this corrects implementation of the existing multi-agent restore intent.`

## 3. Environment and Reproduction

- environment: PC A with a main chat and child agents synced to Google Drive; PC B restoring that project.
- reproduction steps:
  1. Sync a main chat containing child agents on PC A.
  2. Load Remote Chats on PC B.
  3. Observe the main and child chats as separate rows.
  4. Restore the main chat and open it.
  5. Observe that the Agents panel has no durable/openable child chats.
- frequency: reproducible for synced parent chats with child agents.

## 4. Expected vs Actual

- expected: Remote Chats shows only the main chat; restoring it downloads and persists all referenced children, which appear in the right-side Agents panel and can be opened.
- actual: Remote Chats showed parent and children flat; parent restore persisted only the parent and transient summaries.

## 5. Impact

- users affected: users moving multi-agent chats between PCs.
- workflows affected: Google Drive chat sync, remote restore, child-agent focus navigation.
- severity: High because child-agent work appeared lost or unavailable after restore.

## 6. Root Cause

- hypothesis: child transcripts were uploaded but restore reconstructed only summary metadata.
- confirmed cause: BUG-119 added one Drive index row per uploaded child, while `listRemoteChatSessions` returned all rows. `restoreChatRunFromDrive` loaded `ChildAgents` only into the in-memory orchestrator and did not restore or persist child sessions.
- evidence: child index rows had no parent classification, and the restore path called `setHistoricalChildren` without downloading child manifests.

## 7. Fix Strategy

- `F-1` Add parent/agent metadata to child manifests and Drive index rows.
- `F-2` Filter explicit child rows and legacy child identities from top-level remote results.
- `F-3` Restore child manifests recursively from the parent action with cycle protection.
- `F-4` Persist remapped parent IDs, dependency IDs, agent names, roles, statuses, providers, and models for restart-safe panel hydration.
- `F-5` Defer the parent session upsert until every child and child relationship update is durable.

## 8. Validation

- `V-1` `TestListRemoteChatSessionsHidesChildAgentRecords` verifies new and legacy child rows are hidden.
- `V-2` `TestRestoreParentChatRestoresChildrenAndPersistsAgentTree` verifies child restore, parent-last publication, openability, metadata persistence, and reconstruction after restart.
- `V-3` `TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails` verifies strict publication blocking.
- `V-4` `go test ./internal/runner -count=1` passes.
- `V-5` `git diff --check` passes.

## 9. Regression Guard

- tests: remote root filtering, parent-triggered child hydration, parent-last publication, failed-child publication blocking, child resume, collision remapping, and restart reconstruction.
- alerts: child restore failures retain the existing `[chat-sync] child restore failed` runner log.
- audit checks: Remote Chats contains no child row whose parent manifest references it.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; CP-19 and SD-16 already require durable multi-agent behavior without regressing chat sync.
- notes left unchanged on purpose: deeper nested agents are restored recursively when present, while sync discovery still follows BUG-119's currently captured child summaries.
