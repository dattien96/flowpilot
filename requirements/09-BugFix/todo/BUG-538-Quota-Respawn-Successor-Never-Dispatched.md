# BUG-538 — Quota-route successor leg spawned but never dispatched; parent wedges

Status: **OPEN** (captured from live run; not yet fixed)
Severity: Important — wedges a tournament parent run and strands an `active` leg claim forever.

## Symptom (live, `/tmp/fp-live4`, run-551)

1. Tournament leg `run-945` (candidate-a, grok) vetoed on `credits_exhausted`
   → quota card `q-1441`, step `candidate-a` → `WAITING_USER_APPROVAL`.
2. Operator answered `use_for_run|devin|…` while parent `run-551` was `running`.
3. BUG-534 spawn-first ordering worked: successor `run-1651` (devin, same
   cohort `attempt-0`, same worktree `candidate-candidate-a`) created at
   14:43:39.188; `run-945` leg closed `provider_switch` at .189;
   `quota_route_committed` emitted.
4. At the same instant the step `candidate-a` → **FAILED** — it consumed the
   OLD leg's failure outcome.
5. `run-1651` received its handoff (`bus-1653`, queued=false) but **no turn was
   ever dispatched**: `status: idle`, `agent_status: spawned`, no run dir, no
   turn-params line, `leg_state: active`.
6. Parent flow ran attempt-1 (`problem_scout` DONE 14:51:10, rest SKIPPED,
   `loop_state.status: done`) — yet `run-551.status` stays `running` because a
   child leg is still `active` (BUG-521-withheld completion is firing correctly
   on a stranded leg). Nobody ever dispatches or closes `run-1651`.

## Root cause hypothesis

`respawnChildOnRoute` spawns the successor and queues the handoff, but the leg's
first-turn dispatch is normally driven by the step executor. Here the step had
already resolved `FAILED` from the vetoed leg (`run-945`) — so nothing owns the
dispatch of `run-1651`. The successor is neither bound to the step nor
terminalized: a durable leak + permanent worktree claim.

Two contract options, either would fix:

- **A** — respawn during `WAITING_USER_APPROVAL` re-binds the step to the
  successor leg (step → RUNNING on the new run), matching what an external
  observer would expect from "use_for_run".
- **B** — if the step already consumed the leg outcome, refuse the respawn /
  terminalize the successor immediately (fail-closed) instead of leaving a
  spawned orphan.

## Evidence

- `sessions.ndjson` (`/tmp/fp-live4/.flowpilot/chats/`):
  - `run-945`: `leg_state: closed`, `leg_closed_reason: provider_switch`,
    `status: waiting_user_approval`, stale `pending_resume_*` fields.
  - `run-1651`: `leg_state: active`, `agent_status: spawned`, `status: idle`,
    `provider_key: devin`, `working_directory: …/candidate-candidate-a`.
- Step transitions: `candidate-a` `WAITING_USER_APPROVAL` (14:41:10) → `FAILED`
  (14:43:39, same ts as `route_committed`).
- Log: `[quota] route_committed run=run-945 grok/… -> devin/… scope=run` +
  `[agent-spawn] child created … child="run-1651"` — then silence; next
  activity only after a manual `agent-loop/continue` nudge.
- Related: BUG-534 (spawn-first ordering — worked), BUG-535 (dir-claim guard —
  a future candidate-a spawn on this workspace would correctly park on
  run-1651's live claim, i.e. fail-closed but permanently parked).

## Residual (cosmetic)

`run-945` keeps `status: waiting_user_approval` + `pending_resume_*` fields
after its leg closed `provider_switch` — the run-level status wasn't synced to
the leg terminalization.
