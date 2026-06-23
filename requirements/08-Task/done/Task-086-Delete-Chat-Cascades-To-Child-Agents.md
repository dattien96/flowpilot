# Task-086: Delete Chat Cascades To Child Agents

## Metadata

- Document ID: `Task-086`
- Title: `Delete Chat Cascades To Child Agents`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-077: Delete Chat History](./Task-077-Delete-Chat-History.md), [CP-17: Workflow Chat And Session](../../07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `none`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](./Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [BUG-123: Remote Restore Flattens And Loses Child Agent Chats](../../09-BugFix/done/BUG-123-Remote-Restore-Flattens-And-Loses-Child-Agent-Chats.md)
- Replaces: `none`
- Tags: `desktop, local-runner, delete, chat-history, child-agents`

## AI Quick View

### Summary

- Deleting a main chat now deletes every persisted child-agent run under that chat.
- Cascade delete walks stored session rows as well as live in-memory runs, so restored children are removed even after restart.
- Parent and child turn logs are deleted together.
- Provider rollout/session files are removed for every run in the deleted tree.

### Current Ask

- Completed: when a deleted chat owns child agents, delete the whole chat tree instead of leaving child sessions behind.

### Key Decisions

- `T-1` Build the delete set from the persisted session index plus the live run map.
- `T-2` Delete provider artifacts for every run in the tree before pruning state.
- `T-3` Prune agent-orchestrator references for all deleted runs so no orphan child metadata survives in memory.

### Constraints

- Keep delete scoped to the selected run and its descendants only.
- Preserve existing best-effort provider file deletion behavior.
- Do not re-expose child runs in main history while implementing cascade cleanup.

### Open Questions

- None.

### Source Refs

- [Task-077: Delete Chat History](./Task-077-Delete-Chat-History.md)
- [CP-17: Workflow Chat And Session](../../07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md)
- [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)

## 1. Goal

Make chat deletion remove the selected chat and any child-agent chats attached to it, including their persisted sessions, turn logs, in-memory state, and provider session files.

## 2. Parent Links

- coding plan: [CP-17](../../07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md), [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-077 `T-1` through `T-5`, CP-19 `P-2`

## 3. Trigger

Remote restore and restart now preserve child-agent sessions under a main chat. Deleting only the main chat leaves those child runs and their rollout files behind, which breaks the expected “delete this chat” behavior.

## 4. Exact Change

- `T-1` Update `deleteChatSession` to collect the full descendant run tree rooted at the selected run.
- `T-2` Read descendants from both persisted session rows and live in-memory runs so restored child agents are deleted even when they are not currently active.
- `T-3` Delete provider session files for each run in the tree using the existing cross-account provider-home scan.
- `T-4` Remove every deleted run from in-memory state, agent-orchestrator state, persisted session storage, and turn-log storage.
- `T-5` Add a regression test that seeds a stored parent-child run pair, deletes the parent, and verifies both parent and child artifacts are gone.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_resume.go`
  - `apps/local-runner/internal/runner/cross_account_resume_test.go`
- modules: `local-runner` interactive delete path
- routes: existing `DELETE /client/workflow-runs/{runId}`
- tables: local `sessions.ndjson` and per-run turn-log files

## 6. Acceptance Check

- Deleting a root chat removes its child-agent session rows from persistent storage.
- Deleting a root chat removes child-agent turn logs.
- Deleting a root chat removes child-agent provider rollout/session files.
- Restored child agents that exist only in the persisted session index are still deleted.
- Main history is empty after deleting a parent chat and its children.

## 7. Out of Scope

- Deleting remote Google Drive copies of the chat tree.
- Adding a separate UI affordance for deleting child agents directly.
- Redesigning history visibility rules for child-agent runs.

## 8. Completion Notes

- result: implemented cascade delete for parent chats with child agents
- follow-ups: remote Drive delete parity is still separate work
- upstream docs updated: no upstream business/design change required; this task narrows local delete behavior to match restored multi-agent chat persistence
