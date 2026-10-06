# BUG-640 — A cohort-member leg terminalized via `agent-loop/stop` (or any path that skips `finishTurn`) never appends its cohort entry; the barrier stays open forever and every subsequent `flow_control` done/continue is soft-deferred with `rejected_cohort_incomplete`

- **ID:** BUG-640
- **Severity:** Critical — a leaked open cohort is a permanent wedge for
  the hub: its own `flow_control` done/continue calls are soft-deferred
  forever while no member_stalled sweep can ever flag the dead members
  (they are already terminal, so `checkAndBlockStalledMembers` skips
  them). The run sits nominally `running` with zero forward motion until
  an operator manually binds fresh legs to the zombie cohort.
- **Status:** FIXED — CA-1225. `stopAgentLoop` now folds the stopped
  target's own grandparent cohort membership into the same
  `cancelled`-placeholder release path used for children (guarded by
  `memberAlreadyBuffered`), emits `cohort_member_seat_released`, and
  drains the barrier via the existing tail. Regression tests:
  `bug640_stop_member_releases_cohort_seat_test.go`. Audit of other
  leg-terminal paths: turn-completed/turn-failed/start-failure/
  stall-skip/parent-stop-children/park-release/interrupt all already
  append; `leg_claim_sweep` and `reconcileChildRunsOnFlowDone` only run
  after seal/join so they cannot leak seats.
- **Pairs with:** BUG-639 (member_action could not even reach the dead
  member because of first-match label resolution; both defects combined
  to produce the ~50-minute whack-a-mole).

## Evidence chain (all live)

1. Round-5 validate cohort `flow-auto-validate-round-5` spawned
   spec-aligner run-421405 + reviewer run-421410 at 05:24:20
   (`child_spawn_requested flow_cohort_id=flow-auto-validate-round-5
   cohort_size=2`). Both devin sessions produced no events and were
   terminalized via `POST .../agent-loop/stop` at ~05:38–05:40.
2. Neither stop produced a `cohort_member_completed`/`cancelled` diag
   event for that cohort — the seats stayed empty.
3. From then on, every hub `flow_control` continue/done was answered
   `rejected_cohort_incomplete` (diag log, repeatedly).
4. The hub itself diagnosed it correctly in its escalate card: "the
   node's cohort barrier still counts the terminalized dead seats … no
   submission can complete the cohort from the outside" — consolidated
   verdict `changes_requested` was ready but could not land.
5. Unblock path that worked: operator adjudication named the zombie
   cohort id + expected seats; the hub spawned run-426093 / run-426103
   bound to `flow-auto-validate-round-5` so their completions would
   finally fill the barrier.

## Root cause direction

`stopAgentLoop`/`agent-loop/stop` on a leg run transitions the run to a
terminal status but does not append a `cohortEntry{Status:"cancelled"}`
to `child.flowCohortId` — unlike the stall-skip path which explicitly
appends `{Label, Status:"failed"}` (cohort_stall.go). A `cancelled`
entry is the designed released-seat placeholder (BUG-553 semantics in
`openSeatForLabel`/`appendCohortResult`); the stop path simply never
writes it.

Audit every leg-terminal path (agent-loop/stop, interrupt, provider
session death, crash recovery marking a leg failed/cancelled) and ensure
each appends a `cancelled` cohort entry when `flowCohortId != ""` and the
label is not already buffered. That both suppresses the stall sweep for
the dead member AND keeps `hasOpenCohort` honest — the barrier then
completes with the released seats or the entry is replaced by a real
result when a successor leg rebinds.

Note `memberAlreadyBuffered` counts a `cancelled` entry as buffered for
stall-exclusion while `openSeatForLabel` keeps the seat open for a real
replacement — so the append also lets `dead_dispatch_leg_reinvoked` /
fresh spawns rebind cleanly.

## Fix direction

- In the leg terminal transition (or `markLegTerminal`-equivalent), if
  `child.flowCohortId != ""` and `!memberAlreadyBuffered(parent,
  cohortID, label)`, append `cohortEntry{Label: label, Status:
  "cancelled", Err: <terminal cause>}` under the orchestrator lock.
- Regression test (additive): spawn flow with a 2-member cohort, call
  `stopAgentLoop` on one member mid-flight — assert a cancelled entry
  appears in the cohort buffer and `hasOpenCohort` drops once the second
  member completes (or stays open with the released seat if only one
  entry exists and expected=2 — assert the exact intended semantics).
- Companion: `flow_diag_log` should emit `cohort_member_seat_released`
  when this happens so operators can see the release.

## DeclaredPaths

- `apps/local-runner/internal/runner/interactive_service.go` (stop path)
- `apps/local-runner/internal/runner/cohort_stall.go` (if helper shared)
- `apps/local-runner/internal/runner/agent_orchestrator.go` (append hook)
- `apps/local-runner/internal/runner/*_test.go` (new additive test file)
- `change-audit/CA-*.md`
