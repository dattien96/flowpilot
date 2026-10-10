# BUG-665 — Member leg dead-dispatched mid-run; no recovery path reaches it

## Metadata

- Document ID: `BUG-665`
- Title: `Member leg dead-dispatched mid-run; all recovery levers (continue/resume/feedback) silent no-op — only raw POST /turns revives`
- Phase: `bugfix`
- Status: `done`
- Fixed By: `CA-1081`
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `2026-10-09T13:30:00Z`
- Last Updated: `2026-10-09T13:30:00Z`
- Related Documents: `run: run-644825 (parent), run-675483 (coder leg), BUG-660, BUG-664`
- Tags: `wedge, dead-dispatch, member-leg, recovery-gap`

## AI Quick View

### Summary

A `coder` member leg of a `vibe-sprint` parent run completed a provider turn, then never received another turn — the leg stayed `running` for **2h20m** with zero turn dispatch. The parent re-registered the leg in the agent graph (`agent_spawned_by_user` re-emitted for the same runId) but no dispatch followed. Every designed recovery lever failed silently; the only thing that worked was bypassing the loop scheduler with a raw `POST /turns` — and that was itself fenced by an invisible gate.

### Timeline (UTC)

```
10:59:22  leg run-675483: turn_completed (seq 4024) — mid-task, hunting reviewer verdict
          …leg emits nothing for 74 min…
12:13:46  parent: agent_spawned_by_user → run-675483 (re-register, no dispatch)
12:47:47  parent: agent_graph_updated — leg status still "running"; then silence
13:09+    watchdog: STALE flagged; JVM processes idle (gradle daemon, not building)
13:21:46  agent-loop/resume on leg → agent_graph_updated only; no turn_started
13:24:37  parent /resume → parent parks at HIDDEN resume-confirm gate
          (run detail still showed status "running" until gate surfaced as blocked)
13:25:39  agent-loop/feedback → leg: accepted, bus message emitted, still no turn
13:27:10  gate-decision {"option":"ok"} accepted; parent → running; still no leg turn
13:29:xx  POST /turns on leg → turn-696813 dispatched → leg streams tools+deltas
```

### Reproduction evidence

- Leg `run-675483`: `status: running`, `loopState {running, round 0, cap 3}` — last turn_completed 10:59:22Z.
- Parent re-emitted `agent_spawned_by_user` for the SAME runId at 12:13:46Z — over an hour after the leg's last turn — proving a re-dispatch intent that never produced a turn.
- Recovery matrix (all silent no-ops):
  - `agent-loop/continue` on parent → 200, no leg turn
  - `agent-loop/continue` on leg → 200, no leg turn
  - `agent-loop/resume` on leg → 200, `agent_graph_updated` emitted, no `turn_started`
  - `agent-loop/feedback` parent→leg → 200, `agent_bus_message` emitted, no `turn_started`
  - `memberAction {retry, coder}` on parent → 422 `member_action requires blockReason=member_stalled` — `checkAndBlockStalledMembers` never flagged a 2h-dead leg
- `POST /turns` on leg first returned `flow_awaiting_user: parent flow is waiting for your decision` — revealing the parent was gated while `GET /client/workflow-runs` still reported `status: running`.

### Root-cause hypotheses (to triage)

1. **Leg turn scheduler dies silently after turn_completed** — the leg's next-turn decision (continue vs. done) never evaluated or its dispatch was lost; no durable pending-turn record to redrive.
2. **`checkAndBlockStalledMembers` threshold/heuristic misses this shape** — a `running` leg with no in-flight turn is not classified `member_stalled`, so `memberAction retry` stays unreachable.
3. **Hidden gate surface** — parent `status` field reported `running` while a `resumeFrom: synthesis` resume-confirm gate was durably parked; the turn fence (correctly) saw `flow_awaiting_user` but the run detail masked it until a later poll showed `blocked`. Operator-facing surfaces disagree with the fence.
4. **Re-register ≠ re-dispatch** — `agent_spawned_by_user` for an existing runId updated graph state but skipped the dispatch path.

### Fix direction

- When a member leg is `running` with no in-flight turn beyond a bound (e.g. 2× turn p95 or a fixed stall threshold), flag `member_stalled` → surface `memberAction retry|skip` on the parent.
- `agent-loop/continue`/`resume` on a quiet leg should redrive its pending/next turn instead of returning 200 with no effect — make the no-op case explicit in the response.
- Gate state must be reflected in `GET /client/workflow-runs/{id}` consistently (the `status` field flipped to `blocked` only later; the turn fence knew before the read API did).
- Re-registering an existing member run (`agent_spawned_by_user` with reused runId) must re-dispatch a turn or fail loudly.

### Evidence files

- `~/Library/Caches/flowpilot-watch/evidence/deadleg_run-675483.json`
- `~/Library/Caches/flowpilot-watch/evidence/deadleg_run-675483_events.json`
- `~/Library/Caches/flowpilot-watch/evidence/deadleg_parent-644825.json`
- `~/Library/Caches/flowpilot-watch/evidence/stale*run-644825*` (watchdog snapshots)

### Second occurrence (same run, 14:02-14:28Z)

Coder leg finished (turn_completed 13:42) → hub itself parked at `no progress for 2m0s` gate (loop=blocked, run status stayed `running`). `agent-loop/continue` on parent cleared it — `turn_started` 14:28:55. Pattern generalizes: **every actor handoff (leg→hub, hub→next-step) can dead-dispatch**; the 2m no-progress gate catches the hub case but nothing catches the leg case (2h20m silence).
