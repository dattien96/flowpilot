# Task-286: TUI Approval And Question Gates (CP-56 P-7)

## Metadata

- Document ID: `Task-286`
- Title: `TUI Approval And Question Gates`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, yolo-policy, mcp-tools`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-7), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-285](./Task-285-TUI-Sub-Agent-Status-And-Focus-Switch.md)
- Child Documents: `none`
- Related Documents: approval/question APIs, flow gate-decision, agent-loop continue/stop/feedback APIs
- Replaces: `None`
- Tags: `cli-tui, approval, question, interrupt`

## AI Quick View

### Summary

- Map permission, question, regression-gate, and blocked-loop states to inline prompts.
- Submit all actions via existing client methods; keep interrupt path solid under YOLO=off.

### Current Ask

Implement P-7 gate UX.

### Key Decisions

- `T-1` While gate pending, textarea captures answer (not a new chat turn).
- `T-2` Approvals: accept only decision values advertised by `details.decisions` — show labels but submit values; raw `y`/`n` map only to advertised `approve`/`deny`.
- `T-3` Questions: numbered options; multiSelect if flag set (comma-separated indices).
- `T-4` Replay events with pre-filled `decision`/`answer` render read-only (desktop parity).
- `T-5` `flow_gate_violation.gateOptions` must be actionable through `POST .../gate-decision`; never render a blocking flow event as text-only.
- `T-6` Agent graph `loopState.status=="blocked"` exposes Continue with feedback and Stop; use the existing continue/stop endpoints and refresh graph after success.
- `T-7` Approval aliases map `y` only to an advertised decision value `approve` and `n` only to `deny`; never invent `allow`. Numeric selection works for arbitrary provider values.
- `T-8` Offer `remember` only for approval `details.kind=="exec"` and send `{remember:true}` only when explicitly chosen.
- `T-9` Question submission uses `option.value`, falling back to `label` when empty.

### Constraints

- No runner edits. Client methods from Task-278.

### Open Questions

- None.

### Source Refs

- CP-56 P-7, D-13; A7.1–A7.12; M2, M14.

---

## 1. Goal

CLI remains operable when YOLO is off, ask_user/approvals fire, regression gates block, or a flow loop awaits operator input.

## 2. Parent Links

- coding plan: CP-56 P-7, D-13
- tech design: existing interactive approval/question/gate/agent-loop routes
- system spec: SD-19 flow blocking semantics
- specific upstream ids: CP-56 A7.1–A7.12

## 3. Trigger

Task-285 can display blocked flows and agents but cannot resolve their interactive states.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/gates.go
internal/tui/app/gates_test.go
```

### 4.2 Code

```go
type GatePrompt struct {
    Kind        string // approval|question|flow_gate|blocked_loop
    ID          string
    Prompt      string
    Options     []GateOption // preserve exact value submitted and human label
    MultiSelect bool
    Resolved    bool
    ResolvedVal string
}
type GateOption struct {
    Value       string
    Label       string
    Description string
}

func GateFromEvent(e client.ProviderEvent) (GatePrompt, bool)
func GateFromAgentGraph(g client.AgentGraphSnapshot) (GatePrompt, bool)
func (m Model) HandleGateAnswer(raw string) (Model, tea.Cmd)
```

### 4.3 Update behavior

1. On gate event or blocked agent graph → set `m.gate`; `Streaming` may remain true until resolved; show banner above input.
2. Enter while unresolved → parse → `SubmitApproval`, `AnswerQuestion`, `SubmitGateDecision`, `ContinueFlow`, or `StopAgentLoop`; clear only after success.
3. Invalid option → system line error, keep gate.
4. Esc/`/stop` still Interrupt even with gate (cancel turn).

### 4.4 Extend ProviderEvent fields

Ensure client decode mirrors the wire DTO: approval `details.decisions[{value,label}]`, question `options[{label,description,value}]`, replay `decision`/`answer`, `gateOptions`, and agent graph `loopState`. `SubmitApproval` accepts `remember bool` but rejects/omits it for non-exec approvals.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/gates.go`, client gate/flow-control methods, additive tests
- modules: existing TUI packages
- routes: existing approval, question, gate-decision, agent-loop continue/stop/feedback, agent-graph
- tables: none

## 6. Acceptance Check

- [ ] A7.1–A7.12 green, including exact decision values, exec-only remember, question value fallback, flow-gate decisions, blocked-loop Continue/Stop, and replay read-only behavior
- [ ] Manual M2 (yolo off → approve)

## 7. Out of Scope

- Google Drive picker HTML special UI; dispatch uncertain operator cards

## 8. Completion Notes

- result: pending
- follow-ups: Task-287
- upstream docs updated: CP-56-Test-Steps evidence when complete
