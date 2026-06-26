# CA-114: Delete Chat Cascades To Child Agents

## Scope

- Local runner chat delete flow
- Persisted parent/child session trees
- Child-agent turn-log cleanup
- In-memory agent-orchestrator cleanup

## Completed

- Updated `apps/local-runner/internal/runner/interactive_resume.go`.
  - `deleteChatSession` now resolves the selected run, collects all descendant child-agent runs, deletes provider files for every run in that tree, prunes in-memory/orchestrator state, and deletes every persisted session + turn log in the tree.
  - Added helpers to:
    - resolve a run from store or memory
    - collect descendants from the session index and live run map
    - delete provider artifacts per run
    - prune deleted runs from `InteractiveService` and `AgentOrchestrator`
- Added regression coverage in `apps/local-runner/internal/runner/cross_account_resume_test.go`.
  - `TestDeleteChatSessionCascadesToStoredChildAgentRuns`
  - Verifies a stored parent-child Codex tree is fully removed, including:
    - parent session row
    - child session row
    - parent and child turn logs
    - child rollout chain files
    - history visibility after delete

## GitNexus Impact

- `deleteChatSession`: LOW risk, 1 direct caller (`handleDeleteRun`), 0 affected processes.
- `DeleteProviderSession`: LOW risk, 0 upstream callers reported by the current index.
- GitNexus MCP tools were unavailable in this session; the local GitNexus CLI index was used instead.

## Verification

- `go test ./internal/runner -run 'TestDeleteChatSession(RemovesCodexStableAndTurnLogRolloutsAcrossAccounts|CascadesToStoredChildAgentRuns)' -count=1` — pass.
- `go test ./internal/runner -count=1` — pass.
- `git diff --check` — pass.

## Residual Notes

- Delete is still local-only. Synced Google Drive chat-session artifacts are not removed here.
- Provider-file deletion remains best-effort; session/history cleanup still completes if a rollout file is already missing.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: CA-114
change_type: feature
summary: Delete Chat Cascades To Child Agents
# --->8---
