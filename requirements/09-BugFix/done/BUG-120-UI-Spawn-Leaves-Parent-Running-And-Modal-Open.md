# BUG-120: UI Spawn Leaves Parent Run Stuck Running And Modal Open

## Metadata

- Document ID: `BUG-120`
- Title: `UI Spawn Leaves Parent Run Stuck Running And Modal Open`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-110: Orchestration Stream Re-Adds Thinking Row](./BUG-110-History-Panel-Switch-Leaves-Thinking-Row-After-Replay-Completes.md)
- Replaces: `None`
- Tags: `multi-agent, agents-panel, spawn, run-status, orchestration, desktop, runner`

## AI Quick View

### Summary

- Spawning a sub-agent from the Agents panel ("+ Spawn agent") left the **main chat stuck showing "running"** (perpetual loading / spinner in history) after the child finished, and the **Spawn modal did not close**.
- Cause A (stuck running): `emitLocked`'s `default` case set `rs.status = RunStatusRunning` for ANY non-terminal event. `agent_graph_updated` / `agent_bus_message` are orchestration relays emitted **on the parent run** to refresh the panel — so every child graph update flipped the parent (which has no turn in flight) to "running", and nothing ever set it back. This also re-flipped an MCP-tool parent back to running after its own turn had completed.
- Cause B (modal open): the spawn dialog only closed after `client.spawnAgent` resolved; a `wait:true` request blocks the HTTP call until the child finishes (freezing the dialog), and a spawn error left it open silently.
- Fix A (runner): `emitLocked` skips the running-status mutation for `agent_graph_updated` / `agent_bus_message`. Fix B (desktop): close the dialog immediately on dispatch and surface spawn errors.

### Current Ask

- A UI spawn must not leave the parent chat stuck "running"; the spawn modal must close on dispatch.

### Key Decisions

- `V-1` Orchestration relay events (`agent_graph_updated`, `agent_bus_message`) never advance a run's status — only genuine turn-progress events do. This is the server mirror of the BUG-110 desktop fix (orchestration stream must not drive timeline/status).
- `V-2` The spawn dialog closes optimistically on dispatch (the child streams into the Agents panel regardless of the wait toggle); errors are surfaced via a system message instead of silently leaving the dialog open.
- `V-3` "No result inline in the main chat" for a UI/background spawn is BY DESIGN — a panel spawn is not a main-agent turn; the child's output is in its own transcript. Only the MCP-tool spawn (model calls `spawn_agent`, wait=true) returns a result inline.

### Constraints

- Child runs' own status is unaffected — they advance via their real turn events; only the parent's relay-driven status was wrong.
- No change to the wait:true MCP-tool semantics (the model still receives the child's final message).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `emitLocked` default-case guard
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — `spawn` dialog close + error surfacing

## 1. Issue Summary

After spawning a sub-agent from the Agents panel, the child ran and completed (visible in the panel and the child transcript), but the main chat showed a perpetual "running"/loading state and the Spawn modal stayed open.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-110](./BUG-110-History-Panel-Switch-Leaves-Thinking-Row-After-Replay-Completes.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner, Claude main chat, any sub-agent spawn (UI button or MCP tool).
- reproduction steps:
  1. Open a chat; let the greeting/turn complete.
  2. Click "+ Spawn agent", fill the prompt, press Spawn.
  3. Observe: the modal stays open; after the child finishes, the main chat history shows "Running" with a spinner that never clears.
- frequency: Every spawn that emits child graph activity on the parent.

## 4. Expected vs Actual

- expected: parent stays idle/completed; modal closes on dispatch; child streams in the panel.
- actual: parent flipped to "running" by relayed graph events and never recovered; modal stayed open (or frozen for wait:true).

## 5. Impact

- users affected: all multi-agent users spawning sub-agents.
- severity: High — the main chat looks permanently busy and the spawn UI feels broken.

## 6. Root Cause

- confirmed cause A: `emitLocked` (`interactive_service.go`) `default` case set `rs.status = RunStatusRunning` for every non-terminal event. `emitAgentGraphLocked`/`emitAgentBusLocked` emit `agent_graph_updated`/`agent_bus_message` on the parent run, so child activity drove the parent to "running" with no terminal event to clear it.
- confirmed cause B: `AgentsPanel.spawn` closed the dialog only after `await client.spawnAgent(...)`; `wait:true` blocks that call until the child completes, and a rejection skipped the close entirely.
- evidence: history list shows the parent run "Running" while the Agents panel shows the parent "idle"; a child completes yet the parent spinner persists.

## 7. Fix Strategy

- `F-1` In `emitLocked` default case, guard the status mutation: `if ev.Type != EventAgentGraphUpdated && ev.Type != EventAgentBusMessage { rs.status = RunStatusRunning … }`.
- `F-2` In `AgentsPanel.spawn`, set `setDialog(null); setOpen(false)` immediately on dispatch; `await` the spawn in the background and surface failures via `appendSystemMessage`.

## 8. Validation

- `V-1` Go `build`/`vet` clean; `tsc -p tsconfig.phase1-tests.json` clean.
- `V-2` New `TestAgentGraphUpdateDoesNotResetParentRunStatus`: a completed parent stays completed after `emitAgentGraph` + `emitAgentBus`.
- `V-3` Runner suite: 91 passed for Interactive/Spawn/Agent/Turn/Graph/Loop (2 failures are pre-existing skills-merge filesystem tests).
- `V-4` Manual UI verification (modal close + parent not stuck) recommended after rebuild.

## 9. Regression Guard

- tests: `TestAgentGraphUpdateDoesNotResetParentRunStatus` locks the server fix.
- audit checks: a parent run showing "running" while its Agents panel reads "idle" indicates a regression.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
- clarification: a UI/background spawn does not post a result into the main chat — that is intended; only the MCP-tool `spawn_agent` (wait=true) returns a result inline.
