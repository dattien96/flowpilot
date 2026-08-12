# Task-285: TUI Sub-Agent Status And Focus Switch (CP-56 P-6)

## Metadata

- Document ID: `Task-285`
- Title: `TUI Sub-Agent Status And Focus Switch`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, agent-spawn, agent-flow-engine`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-6), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-284](./Task-284-TUI-Statusline-Account-And-Token-Usage.md)
- Child Documents: `none`
- Related Documents: Desktop `focusAgentRun`, `agent-graph` API
- Replaces: `None`
- Tags: `cli-tui, agents, focus`

## AI Quick View

### Summary

- Project agent graph onto statusline strip; Tab / `/agent` cycles focus.
- Child focus: stream that run’s events (read-only); block send with desktop-parity message.
- Main focus: restore send + main timeline stream.

### Current Ask

Implement P-6 agent strip + focus.

### Key Decisions

- `T-1` Main always first in strip; mark focused with `*`.
- `T-2` Child send blocked: `Child transcript is read-only. Return to main (/agent main).`
- `T-3` Prefer `agent_graph_updated` events; poll `GetAgentGraph` as fallback on turn boundaries.
- `T-4` The main orchestration stream stays subscribed regardless of viewport focus so agent/token updates continue. Focus switch cancels/replaces only the separate child-focus stream.
- `T-5` Keep `map[runID][]TimelineItem` and `map[runID]int64` cursors. Focus changes swap the viewport projection without destroying main/child history.
- `T-6` Every focus stream carries a generation token; late messages from a cancelled stream are ignored.

### Constraints

- No runner edits. Depends on statusline (284) + flow launch (283) for multi-agent demos.

### Open Questions

- None.

### Source Refs

- CP-56 P-6, D-6; A6.1–A6.9; M5.

---

## 1. Goal

Show live sub-agents and switch viewport focus between main and children.

## 2. Parent Links

- coding plan: CP-56 P-6, D-6
- tech design: agent-graph and per-run event-stream contracts
- system spec: SD-19 / CP-36 multi-agent flow behavior
- specific upstream ids: CP-56 A6.1–A6.9

## 3. Trigger

Task-284 introduces the agent footer slot; flow mode now needs durable per-run focus state.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/agents.go
internal/tui/app/agents_test.go
internal/tui/client/agents.go  # GetAgentGraph, ListAgentRuns if missing
```

### 4.2 Types

```go
type AgentStatusLine struct {
    RunID  string
    Name   string
    Role   string
    Status string
    IsMain bool
}

func ProjectAgents(graph client.AgentGraphSnapshot, mainRunID string) []AgentStatusLine
func cycleAgent(agents []AgentStatusLine, current string, dir int) string // +1/-1 wrap
```

### 4.3 Model fields

```go
FocusRunID string // empty or mainRunID => main
MainRunID  string
Agents     []AgentStatusLine
Timelines  map[string][]TimelineItem
AfterSeq   map[string]int64
StreamGeneration uint64
CancelChildFocusStream context.CancelFunc
```

### 4.4 Commands

- Tab / Shift+Tab → cycle
- `/agent main|<name|runId|list>`
- On focus child: set `Streaming` viewer on child run; `sendDisabled=true`
- On focus child: keep main stream alive; cancel prior child-focus context, increment child generation, restore cached child timeline/cursor, and start the separate child stream
- On focus main: cancel only the child-focus stream and restore cached main viewport; do not restart/cancel the main orchestration stream
- On focus main: restore send only when the main run itself is not otherwise blocked/streaming
- Ignore stream messages carrying a stale generation

### 4.5 Client

```go
func (c *Client) GetAgentGraph(ctx context.Context, runID string) (AgentGraphSnapshot, error)
func (c *Client) ListAgentRuns(ctx context.Context, parentRunID string) ([]AgentRunSummary, error)
```

Mirror desktop DTO fields needed for name/status/role only.

### 4.6 Statusline

`StatusModel.Agents` + `FocusRunID` rendered as:

```text
agents: [main*] coder● reviewer-1○
```

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/agents.go`, `internal/tui/client/agents.go`, additive tests
- modules: existing TUI packages
- routes: existing agent graph/list and per-run event streams
- tables: none

## 6. Acceptance Check

- [ ] A6.1–A6.9 green, including focus round-trip preservation, child-stream cancellation, stale-generation suppression, and continued main agent/token updates while child-focused
- [ ] Manual M5 on review-loop

## 7. Out of Scope

- Spawn-from-CLI; orchestration board; gate UI (286)

## 8. Completion Notes

- result: pending
- follow-ups: Task-286
- upstream docs updated: CP-56-Test-Steps evidence when complete
