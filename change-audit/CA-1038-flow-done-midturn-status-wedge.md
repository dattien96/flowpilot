# CA-1038 — Flow sealed done mid-turn: run status wedged `running` + durable read-path split-brain

Date: 2026-09-27 — exposed by the live bug-harness run (run-45, devin /
swe-2-high, 9-node chain). The audit node submitted `done` while the hub's
provider turn was still in flight; the sequence below wedged the run at
`running` permanently.

## Defect 1 — `markPendingFlowGateSettleLocked` re-armed a terminal loop

`applyFlowControl("done")` owns the terminal transition: it marks the flow
run complete, sets `loopState.Status=done`, and synchronously persists the
parent session. On live run-45 the audit's `submit_review_outcome` sealed
the loop at 18:00:18 while hub turn `turn-2971` was still in flight. When
`EventTurnCompleted` arrived at 18:00:20, `markPendingFlowGateSettleLocked`
armed `pendingFlowGateSettle` and — worse — regressed `rs.status` back to
`running`, then persisted that as a durable checkpoint.

The deferred post-turn gate then refused to evaluate:
`gateEpochStillValidLocked` sees the loop already terminal and returns a
silent block, so nothing ever flipped the status back. Durable evidence on
the wedged row: `status=running`, `loop_state.status=done`,
`pending_flow_gate_settle=true` — then a second checkpoint at the same
instant with the flag cleared but status still `running`.

Fix (`interactive_service.go`, `markPendingFlowGateSettleLocked`): for a
root run (`parentRunID == ""`), refuse to arm the deferred gate when the
mounted loop is already `done` or `stopped`. The seal path owns terminal
status; a post-turn gate on a sealed loop has nothing to evaluate.

## Defect 2 — `durableRunSnapshot` skipped resume normalization

Even accounting for defect 1, the single-run endpoint and the history list
disagreed on the same durable row. `projectRunHistory` projects every
persisted row through `normalizeResumedFlowStatus` (BUG-StaleCancel —
in-flight statuses go stale the moment the process dies), while
`durableRunSnapshot` returned the raw `Status`. Result on the restarted
runner: history listed run-45 `completed`; `GET /client/workflow-runs/run-45`
answered `running` forever.

Fix (`interactive_handlers.go`, `durableRunSnapshot`): apply the same
`normalizeResumedFlowStatus` projection, so a persisted-only run with
`loop=done` reads `completed` even when the last durable row still carries
the stale in-flight status.

## Test changes (additive)

- `flow_done_midturn_status_wedge_test.go` —
  `TestFlowDoneMidTurnKeepsSettledCompletedStatus` reproduces the live
  shape: flow hub seals `done` mid-turn, `EventTurnCompleted` arrives
  after. Before the fix the run regressed to `running`; now it stays
  `completed`.
- `durable_snapshot_normalize_test.go` —
  `TestDurableRunSnapshotNormalizesLoopDoneToCompleted` seeds a store row
  `status=running + loop_state=done` with no in-memory run and asserts the
  single-run snapshot projects `completed` (matching the history list).

## Live verification (entry point, fixed binary)

- `GET /client/workflow-runs/run-45` on the rebuilt binary against the
  untouched wedged durable row → `"status": "completed"` (was `"running"`
  before both fixes; row still reads `running` on disk).
- `kill -9` + restart → same answer, so the projection survives the
  persisted-only path, not just warm memory.

## Quota ledger — live verification (case 4)

- `quota_exhausted` ledger block on the pinned devin account survived
  `kill -9` + restart; on admission `pinnedAccountHardVeto` deferred to
  fresh telemetry (account healthy, low state, exact confidence) and the
  stale block correctly did NOT veto — the designed self-heal, not a
  swallowed rejection.
- `billing_required` block on the same pin → `POST .../turns` rejected
  `quota_route_required`, zero `session/prompt` dispatched to devin, run
  parked `waiting_question` on a durable route card offering the grok
  account as alternate plus `stop`.

## Definition of Done

- [x] Red test reproducing the mid-turn regression before any prod change
- [x] `markPendingFlowGateSettleLocked` refuses to arm on a terminal loop
- [x] `durableRunSnapshot` normalizes via `normalizeResumedFlowStatus`
- [x] Focused tests green; BUG-521 suite unbroken
- [x] Live: wedged run-45 row reads `completed` through the real endpoint
- [x] Live: `kill -9` restart keeps the corrected read
- [x] Live: quota ledger block persists restart; hard-kind veto rejects
      pinned turn with `quota_route_required`, no provider send
