# CA-1179 — spawned/reprompted child park drops the orchestrator summary

## Evidence (live run-204891, CP-03 Task-039 sprint)

Child `run-205821` (scaffold-architect) finished its provider turn, was
diverted into an owner-debate, then its reprompt re-dispatch hit the
`flow_awaiting_user` 409 fence while the parent loop was still blocked.
`GET /client/workflow-runs/run-205821` returned `waiting_user_approval`, but
`GET /client/workflow-runs/run-204891/agents` kept reporting
`status=running, agentStatus=running` for the same child — a phantom-live
member with no actionable decision lane. The desktop's executing-agent badge
and any summary-driven liveness check misread the parked child as working.

## Root cause

`handleSpawnedChildTurnFailure` (`flow_awaiting_user` branch,
interactive_service.go) is the only child park site that mutates
`child.status`/`child.agentStatus` without syncing the orchestrator summary:
it emits no `ProviderEvent`, so the generic per-event sync inside
`emitLocked` (which upserts `Status: rs.status` on every child event) never
runs. Every sibling park site either calls `emitLocked` (vibe gate
requirement park), or upserts the summary explicitly
(`parkFlowForAwaitingUser`, `settleChildStatusAfterGateBlockLocked`).

The same handler serves spawned-child first-turn refusals AND gated-child
reprompt re-dispatch refusals (see the P1-04 comment at
resumePendingFlowGate: "reprompt→durable fenced park via
handleSpawnedChildTurnFailure"), so the gap hits both the spawn leg and the
post-debate reprompt leg — exactly the live sequence.

## Fix

Inside the `flow_awaiting_user` park (while `s.mu` is held), upsert the
child's summary: preserve an existing summary's other fields (ActivationSeq
et al.) or create a minimal entry — the same pattern
`parkFlowForAwaitingUser` and `settleChildStatusAfterGateBlockLocked` use.
Guarded on `agentOrchestrator != nil`. No new persistence channel — the
existing `sessionStateOf` + `persistProviderSession` checkpoint is
unchanged, and the durable resume intent (`pendingResume*`) still carries
the re-drive.

## Regression

`internal/runner/bug1179_child_park_summary_sync_test.go`:

- `TestBug1179_SpawnedChildAwaitingUserParkSyncsSummary` — red before the
  fix (summary stayed `running`), green after; also asserts the durable
  resume intent survives the park.
- `TestBug1179_RequirementParkOnChildSyncsSummary` — pins the already-correct
  emitLocked-mediated sync for the gate-requirement park as a positive
  invariant.
