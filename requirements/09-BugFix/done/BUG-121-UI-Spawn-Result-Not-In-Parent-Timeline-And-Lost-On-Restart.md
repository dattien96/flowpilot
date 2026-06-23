# BUG-121: UI-Spawn Result Not Recorded In Parent Timeline And Lost On Restart

## Metadata

- Document ID: `BUG-121`
- Title: `UI-Spawn Result Not Recorded In Parent Timeline And Lost On Restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-120: UI Spawn Leaves Parent Running And Modal Open](../done/BUG-120-UI-Spawn-Leaves-Parent-Running-And-Modal-Open.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, agents-panel, spawn, timeline, persistence, restart, runner`

## AI Quick View

### Summary

- When a user spawns a child agent via the UI (AgentsPanel dialog), the parent run's timeline receives no persisted record of the spawn event or the child's result.
- When a parent's AI model spawns via the `spawn_agent` tool call, the tool events (`tool_started` / `tool_completed`) land in the parent's event log on disk and survive a server restart; UI-spawn has no equivalent.
- The current workaround (`appendSystemMessage` in `AgentsPanel.tsx`) adds an ephemeral React state entry — it shows live when `wait=true` but disappears after server restart because it is never written to the event log.
- No record is written to the parent timeline when `wait=false`, leaving the main chat unaware that a UI-spawn happened at all.

### Current Ask

- Persist a synthetic "spawn annotation" event into the parent run's event store when the user triggers a spawn from the UI, both at spawn time and on child completion (for `wait=true`), so the parent timeline survives a server restart.

### Key Decisions

- `V-1` After UI-spawn with `wait=true`, opening the parent chat after a server restart must display the child's final message in the parent timeline.
- `V-2` After UI-spawn with `wait=false`, opening the parent chat after a server restart must display at minimum a "spawned agent X" annotation.

### Constraints

- The parent run may not have an active turn in flight when UI-spawn is triggered; any fix must write events outside the normal turn lifecycle.
- Injecting events into the parent run's event store mid-stream must not corrupt the monotonic `seq` ordering used by the reconnect/replay cursor (BUG-112).

### Open Questions

- Should the synthetic event be a new `agent_spawned_by_user` event type, or should it reuse `agent_bus_message` which is already in the event stream?
- Should the child's result on `wait=false` async completion also be injected, or only `wait=true` blocking results?

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — `spawn()` function, line ~87
- `apps/desktop-flowpilot/src/state/store.ts` — `appendSystemMessage`, line ~526
- `apps/local-runner/internal/runner/interactive_service.go` — `spawnChildRun`, line ~1046
- `apps/local-runner/internal/runner/agent_orchestrator.go` — `addBus`, line ~247

## 1. Issue Summary

When the user spawns a child agent from the AgentsPanel UI dialog, the parent run's timeline shows nothing and the parent has no persisted record of the spawn.

Contrast with AI-driven spawn: when the parent agent calls `spawn_agent` as a tool, `tool_started` and `tool_completed` events are written to the parent run's event log. On history reopen or server restart, these events are replayed and appear in the parent timeline.

UI-spawn has no equivalent. The only feedback written to the parent is a call to `appendSystemMessage` in the frontend, which:
- only fires when `wait=true` AND `result.finalMessage` is non-empty
- writes to React state only — not to the backend event log
- is lost on any page reload or server restart

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) — P-2 states both AI-callable and UI spawn actions go through one backend path with identical `wait:true/false` behavior
- impacted tech design: TBD (SD for multi-agent orchestration)
- impacted system spec: TBD

## 3. Environment and Reproduction

- environment: Desktop app, local runner (no Supabase), branch `task/agents`
- reproduction steps:
  1. Start a Claude or Codex parent chat.
  2. Open AgentsPanel, click "Spawn", fill the dialog, set "Wait for result: ON", submit.
  3. Wait for child to complete. Observe `appendSystemMessage` appears in main timeline.
  4. Restart the local runner.
  5. Reopen the same chat session (History → open).
  6. Observe: the spawn result is gone from the parent timeline.
  7. Repeat steps 1–2 with "Wait for result: OFF". Observe: no entry in parent timeline at all.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: UI-spawn writes at least one annotation to the parent run's event log; on restart, that annotation replays in the parent timeline.
- actual: nothing is written to the parent run's event log; restart clears all evidence of the spawn from the parent timeline.

## 5. Impact

- users affected: All users who spawn child agents via the UI dialog.
- workflows affected: Any multi-agent workflow where the parent needs to record that a UI-triggered spawn occurred.
- severity: Medium — data loss (spawn result disappears on restart); the agents panel still shows child run status correctly via `sessions.ndjson`, so tracking is not fully lost.

## 6. Root Cause

- hypothesis: UI-spawn result is written only to ephemeral React state, not to the parent run's persistent event log.
- confirmed cause: `appendSystemMessage` in `AgentsPanel.tsx:90` appends to `timeline` (Zustand state) only. No backend call writes an event to the parent run's event store. The `addBus` call in `spawnChildRun` (`interactive_service.go:1208`) records a "handoff" bus message but bus messages are also purely in-memory (`AgentOrchestrator.bus` is a plain map — see `agent_orchestrator.go:247-254`).
- evidence:
  - `appendSystemMessage` implementation at `store.ts:526` — pure `set(s => ({ timeline: [...] }))`, no backend write.
  - `AgentOrchestrator.addBus` at `agent_orchestrator.go:247` — stores to `o.bus[parentRunID]` in-memory map, no persistence path.
  - AI-driven `spawn_agent` tool call path: provider streams `tool_started`/`tool_completed` events → `emitLocked` → event is appended to the parent run's event sidecar on disk and replayed on history open. UI-spawn has no equivalent emit.

## 7. Fix Strategy

- `F-1` Add a new internal method `emitAnnotationEvent(parentRunID, eventPayload)` on `InteractiveService` that appends a synthetic event to the parent run's event store using the existing `nextSeq` counter and sidecar write path, then broadcasts it to all SSE subscribers.
- `F-2` In the HTTP spawn handler (`interactive_handlers.go`), after `spawnChildRun` returns, call `emitAnnotationEvent` with:
  - an `agent_spawned_by_user` event (new type) carrying `agentName`, `childRunID`, `providerKey`, `modelName`, `wait` flag — written immediately at spawn time.
  - if `wait=true` and `FinalMessage != ""`, a second `agent_result_injected` event carrying `agentName`, `finalMessage` — written after the blocking wait resolves.
- `F-3` Add frontend rendering in `Timeline.tsx` for both new event types: show a compact "Spawned agent X [provider]" row at spawn time and a quoted result block on completion.
- `F-4` Remove the `appendSystemMessage` call from `AgentsPanel.tsx` — the timeline will now receive these events via the normal SSE stream, making the frontend call redundant and avoiding duplication.

## 8. Validation

- `V-1` Spawn a child agent via UI with `wait=true`, restart the server, reopen the parent chat: the child's final message must appear in the parent timeline.
- `V-2` Spawn a child agent via UI with `wait=false`, restart the server, reopen the parent chat: a "Spawned agent X" row must appear in the parent timeline.
- `V-3` Spawn a child agent via AI prompt (`spawn_agent` tool): existing timeline behavior must be unchanged.
- `V-4` `go test ./internal/runner/...` must pass after adding the new event type and emit path.

## 9. Regression Guard

- tests: Add a test in `interactive_service_test.go` asserting that `spawnChildRun` called via the HTTP path writes at least one event to the parent run's event sidecar.
- alerts: None.
- audit checks: `gitnexus_detect_changes` after implementing F-1/F-2 to verify blast radius of new event type.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-19 should be updated to explicitly state that UI-spawn also produces persisted parent-timeline events (currently only AI-tool spawn is described as writing to the event log).
- notes left unchanged on purpose: `AgentOrchestrator.bus` in-memory design is intentional for ephemeral coordination; only the parent timeline needs persistence, not the bus.
