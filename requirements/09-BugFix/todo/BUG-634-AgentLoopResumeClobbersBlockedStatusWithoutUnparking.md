# BUG-634 — `agent-loop/resume` unconditionally flips `blocked` → `running` + clears `GateReason` while the durable decision-form park stays frozen → `flow_not_blocked` amend rejection, false "running" status

- **ID:** BUG-634
- **Severity:** High — desyncs the only drift-sanction path
  (`agent-loop/amend`, requires `loopStatus=="blocked"`) and makes every
  status surface lie about a frozen run.
- **Status:** FIXED — CA-1236 (2026-10-08): resume() preserves parked statuses (blocked/done/tournament_escalation); only paused flips to running
- **Found:** run-306526 (`vibe-tasks` CP-04), 2026-10-06T02:22–02:38 —
  scope-drift park (`flow_parked_awaiting_user`, `loop_status:blocked`)
  stayed frozen 18 min with zero events, yet `agent-graph` reported
  `status:running` after `agent-loop/resume` calls; `agent-loop/amend`
  returned `flow_not_blocked` during the whole freeze.

## Symptom

`POST /client/workflow-runs/{runId}/agent-loop/resume` →
`resumeAgentLoop` → `agentOrchestrator.resume(parentRunID)`:

```go
func (o *AgentOrchestrator) resume(parentRunID string) AgentGraphSnapshot {
    return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
        st.Status = "running"; st.GateReason = ""; return st
    })
}
```

No branch on `st.Status` — a `blocked` escalate/decision-form park is
overwritten to `running` exactly like a `paused` resume. The durable park
(`flow_parked_awaiting_user`: in-flight turns cancelled, auto-intents
dropped, decision form pending) is untouched — the run is still frozen —
but `loopStateFor` now reports `running` with `GateReason` wiped.

## Defect

Two state sources diverge:

- **Durable/dispatch state**: parked awaiting a decision form — nothing
  can run, the escalate gate_reason is still armed.
- **`AgentLoopState`**: flipped to `running`, `GateReason=""` —
  `BlockReason` keeps the stale `escalate` label so surfaces show
  contradictory `running + block:escalate`.

Consequences observed live:

1. `handleAmendFlow` reads `loopStateFor().Status != "blocked"` →
   `flow_not_blocked` (409) — the drift gate's own instruction
   ("amend the contract to sanction them") becomes unreachable while the
   flow is frozen on exactly that drift park.
2. Operator/status tooling sees `running` and assumes progress while the
   engine is hard-stopped (no events for 18+ min on run-306526).
3. `resumePendingLoopWork` cannot actually resume parked work — the flip
   is status-only, so the call is pure desync with no functional resume.

## Expected fix direction

- `resume()` should only flip `paused → running` (the state it exists
  for). For `blocked` parks it must either no-op (returning the true
  snapshot) or route through the decision-form discharge path — never
  overwrite `blocked`/`stopped`/`done`/`tournament_escalation` statuses.
- `transition()` already has the pattern (the `rejected` arm keeps
  terminal/awaiting-user statuses untouched, run-2047 comment): apply the
  same status guard to `resume`.
- The amend blocked-check is itself correct — the bug is that `resume`
  poisons the source it reads. No relax needed there.
- Regression test shape: park a run via the escalate form, call
  `resumeAgentLoop`, assert `loopStateFor().Status` stays `blocked` and
  `GateReason` intact; assert `agent-loop/amend` then accepts the park.
