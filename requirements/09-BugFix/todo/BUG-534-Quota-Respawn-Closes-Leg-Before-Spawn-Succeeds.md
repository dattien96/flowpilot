# BUG-534 — respawnChildOnRoute commits route + closes leg before the replacement spawn is known to succeed → no-retry wedge

**Status:** fixed (CA-1042) — spawn-before-close ordering; red test →
focused/package/race/vet green → live-verified both refusal and success
legs on `/tmp/fp-live3` (runner :4319).

## Reproduction

1. Flow child turn hits admission veto (e.g. billing_required on the pinned
   account) → quota route card → operator picks `use_for_run|<provider>`.
2. `commitQuotaRotation` emits durable `route_committed`, then calls
   `respawnChildOnRoute`.
3. `respawnChildOnRoute` mutates `child.legState = LegStateClosed` in-memory,
   persists the closed-leg session row, THEN calls `spawnChildRun`.
4. `spawnChildRun` fails (live run-1269: refused while the parent loop was
   blocked/parked on the arbiter decision card — "parent loop is blocked
   (escalate)").
5. Result: `route_committed` is durably committed, the old leg is durably
   closed, and NO replacement child exists. Nothing re-drives the respawn
   when the parent unblocks — the flow node waits on a child that will never
   be spawned (the card is consumed; admission re-entry needs a new turn,
   which the closed leg refuses with `leg_closed`).

## Root cause

Ordering in `apps/local-runner/internal/runner/quota_gate.go`
`respawnChildOnRoute`: in-memory leg close → durable persist → spawn. The
commit point precedes the operation that can fail, and there is no durable
"pending respawn" intent the parent unblock / restart path can replay.

## Fix (CA-1042) — spawn-first ordering

`respawnChildOnRoute` in `quota_gate.go` now:

1. Builds the successor spawn input (same label, `WorkspaceCwd`,
   `FlowCohortID`, inflight `PendingPrompt`).
2. Calls `spawnChildRun` FIRST — on refusal (parent loop blocked, BUG-432)
   the error propagates with the old leg still `active`, no
   `route_committed` emitted, nothing persisted.
3. Only on success sets `legState=Closed` + `leg_closed_reason=
   provider_switch` and persists the session row.
4. `commitQuotaRotation` emits `quota_route_committed` only after
   `respawnChildOnRoute` returns nil.

The vetoed leg stays alive on refusal → the next admission turn re-enters
the gate → a fresh quota card — no stranded "committed but dead" route and
no durable pending-intent needed (option 1 of the suggested shapes).

A latent defect surfaced by the reorder: run ids and event ids share
`idCounter`, so emit-before-spawn used to burn a tick and hide collisions;
spawn-first let the successor mint `run-1` over a resident run. `createRun`
now bumps past any resident run id before insert (BUG-117-class seeding).

## Regression test

`TestBug534_RefusedRespawnKeepsLegOpen` — parent blocked → respawn refused
→ old leg stays `active`, zero `quota_route_committed` events, no
replacement child under the same label.

## Live evidence (/tmp/fp-live3, runner :4319, tournament run-2634)

- **Refusal (2×)**: run-1408 under a manual escalate-park AND run-3107
  under an organic gate-escalate park — answer `use_for_run|devin|…`
  returned `quota_gate: respawn child: parent run "run-2634" loop is
  blocked (escalate)`; leg stayed `active`, zero committed events, no
  successor.
- **Re-admission**: a new turn on the still-open run-3107 leg re-entered
  admission → veto refired → new card `q-4146` (proves the leg is not
  stranded).
- **Success ordering**: answering `q-4146` while the parent ran spawned
  successor `run-4150` (devin/swe-2-high, same label `candidate-a`, same
  cohort `flow-auto-parallel_rollout-attempt-0`, same worktree
  `candidate-candidate-a`), THEN closed run-3107 (`leg_state=closed`,
  `leg_closed_reason=provider_switch`), THEN emitted `quota_route_committed`
  (evt-4155 seq 3, after the successor existed).
- `quota_route_committed` is in the run event stream + admin events API;
  it is not in the CP-41 sidecar whitelist (`isFlowSidecarEventType`) —
  unchanged pre-existing design, durability comes via the session rows.

## Evidence (pre-fix)

- `quota_gate.go` `respawnChildOnRoute`: legState mutated before persist;
  spawn last; error propagates with the leg already durably closed.
- Live run-1269 (Devin-only drill): `route_committed` durable event emitted
  for candidate-a grok→devin, respawn refused on blocked parent, repin never
  retried — required manual intervention. Recorded in CA-1039 + the R6/R7
  runbook residual list.
