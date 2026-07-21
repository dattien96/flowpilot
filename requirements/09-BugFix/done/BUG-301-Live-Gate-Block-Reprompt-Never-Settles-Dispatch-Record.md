# BUG-301 — Live gate block/reprompt never settles the dispatch record

## Metadata

- Document ID: `BUG-301`
- Title: Live gate block/reprompt never settles the dispatch record
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: CP-51 (durable turn dispatch state machine), Task-251 (settle driver wiring)
- Child Documents: none
- Related Documents: CP-51-PhaseAB-Timeline-And-Verification-Log.md §3.1 A6 (Tier-1 code gate), run-18997/turn-19161
- Replaces: none
- Tags: chat-history, dispatch, settle, flow-gate, regression

## AI Quick View

### Summary

- After a Review Loop chat run's post-turn gate blocked/reprompted (Tier-1 doc-scope violation: "declared bug mode but no bugfix document found"), the completed turn's dispatch record stayed at `settle_phase=settle_pending` forever — the "Dispatch attention" card never cleared, even after the overall run reached "Completed".
- Confirmed against real data (`dispatch.ndjson`, project `db51ec26-...`): `turn-19161` (run-18997) has exactly 4 records (`prepared`→`send_claimed`→`send_started`→`terminal_completed`) and never advances, while a sibling turn (`turn-19177`) in the same project reached every settle phase (`gate_evaluated`→...→`finalized`) within milliseconds.
- `resumePendingFlowGate`'s gate-blocked branch (the boot/server-restart resume path) correctly calls `scheduleSettleAfterGateBlock` after persisting the reprompt checkpoint. The LIVE post-turn-gate blocked/reprompt branch inside `runTurn`'s tail — which runs the very first time a turn's gate resolves, without needing a restart — never had the matching call. Only the LIVE gate-**pass** branch called `scheduleSettleAfterGatePass`.
- Net effect: any turn whose gate **passes** settles correctly and immediately; any turn whose gate **blocks/reprompts** (exactly what CP-51's A6 "Tier-1 code gate" test exercises) leaves its dispatch record orphaned at `settle_pending` until the next full server restart's boot recovery scan (`drivePendingSettlesOnBoot`) happens to sweep it up.

### Current Ask

- Give the live gate-block/reprompt branch the same durable settle disposition the gate-pass branch and the boot/resume path already have, without touching any other reprompt/park/notify behavior.

### Key Decisions

- `V-1` Mirror the existing `scheduleSettleAfterGateBlock` call from `resumePendingFlowGate`'s block branch into the live `runTurn`-tail block branch, at the same point (after the reprompt/block checkpoint is durably persisted), rather than inventing a new settle path.

### Constraints

- Must not change reprompt targeting, park/notify-idle behavior, or Stop-race handling in the same block.

### Open Questions

- None.

### Source Refs

- run-18997 / turn-19161 (`.flowpilot/chats/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/dispatch.ndjson`)
- `apps/local-runner/internal/runner/interactive_service.go` (live post-turn-gate block branch, `resumePendingFlowGate`'s block branch)
- `apps/local-runner/internal/runner/dispatch_settle_wire.go` (`maybeScheduleSettleAfterTerminal`, `scheduleSettleAfterGateBlock`)

## 1. Issue Summary

Running CP-51's A6 live test (Review Loop, Chat→Bug mode, a coder edit that leaves a Tier-1 doc-scope violation) surfaced a "Dispatch attention" card reading `settle_pending — terminal bookkeeping is awaiting durable settlement`. The card never cleared, including after the run itself reached "Completed".

## 2. Parent Links

- impacted coding plan: CP-51 durable dispatch state machine
- impacted tech design: n/a
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: local dev, Claude provider, `chat` run kind, Review Loop built-in orchestration
- reproduction steps:
  1. Start a Review Loop chat run whose coder turn leaves a Tier-1 doc-scope violation (e.g. missing bugfix doc / missing change-audit note) on a flow-engine-driven hub turn.
  2. Let the post-turn gate evaluate and reprompt.
  3. Inspect the turn's dispatch record (`GET .../dispatches/{turnId}` or the raw `dispatch.ndjson`).
- frequency: deterministic for every live turn whose post-turn gate blocks/reprompts

## 4. Expected vs Actual

- expected: the dispatch record's `SettlePhase` reaches a final state (`settle_superseded_reprompt` for a block/reprompt, matching the boot/resume path's own behavior) shortly after the gate resolves, live, without needing a restart.
- actual: `SettlePhase` stays at `settle_pending` indefinitely; the Dispatch attention card never clears.

## 5. Impact

- users affected: anyone running a flow-hub turn whose post-turn gate blocks/reprompts (any CP-51 A6-style scenario)
- workflows affected: Review Loop and any other flow-engine-driven chat/workflow run where the Tier-1 doc-scope gate reprompts
- severity: low-medium (cosmetic/bookkeeping leak — the reprompt itself still dispatches correctly; only the durable settle ledger is left dangling until the next restart)

## 6. Root Cause

- hypothesis: the live gate-blocked/reprompt code path never drives the dispatch settle disposition.
- confirmed cause: `apps/local-runner/internal/runner/interactive_service.go`'s live post-turn-gate handling in `runTurn`'s tail has two outcome branches after `runFlowGateAtEpoch`/`runChildArtifactOutputGateAtEpoch` returns:
  - **blocked/reprompt branch** (`if stoppedMidGate || gateBlocked { ... else if rs.pendingFlowGateSettle { ... persist checkpoint ... reprompt-or-park ... } }`) — persists the reprompt checkpoint but never calls any `scheduleSettleAfter*` function.
  - **pass branch** (further down, both the child and root sub-cases) — correctly calls `s.scheduleSettleAfterGatePass(...)` once the completion snapshot persists.

  The separate boot/resume path, `resumePendingFlowGate`, has the same two branches and correctly calls `s.scheduleSettleAfterGateBlock(repromptRun, blockTurnID)` in its own blocked branch, immediately after persisting the checkpoint — proving the intended design already existed, just not wired into the live path.

  `maybeScheduleSettleAfterTerminal` (called right after a turn's dispatch record is durably committed to terminal) deliberately bails out without settling when `pendingFlowGateSettle` is armed and the flow loop isn't done, explicitly deferring to whichever gate-resolution path runs next. For a passing gate, that deferral is honored by the pass branch's `scheduleSettleAfterGatePass` call. For a blocking/reprompting gate, live, nothing ever fulfilled that deferral — leaving the record stuck until a server restart's `drivePendingSettlesOnBoot` sweep (which does call `resumePendingFlowGate`, and thus does reach `scheduleSettleAfterGateBlock`) happens to run.
- evidence:
  - `dispatch.ndjson` for `run-18997`/`turn-19161`: exactly 4 records, stuck at `terminal_completed` / `settle_phase=settle_pending`, versus sibling `turn-19177` (same project) reaching `finalized` within milliseconds.
  - New regression test `TestLiveGateBlockSchedulesSettleDisposition` reproduces the exact scenario (a flow-engine-driven root run whose post-turn gate blocks on a missing change-audit note) and confirms the record never leaves `settle_pending` on the unfixed code, reaching `settle_superseded_reprompt` once fixed.

## 7. Fix Strategy

- `F-1` Added `s.scheduleSettleAfterGateBlock(repromptRun, turnID)` to the live post-turn-gate blocked/reprompt branch in `runTurn`'s tail, at the same point `resumePendingFlowGate`'s block branch already calls it (immediately after the reprompt/block checkpoint persists successfully) — mirroring the existing, already-correct boot/resume-path behavior rather than inventing new settle logic.

## 8. Validation

- `V-1` New regression test `TestLiveGateBlockSchedulesSettleDisposition` (`apps/local-runner/internal/runner/live_gate_block_settle_test.go`): drives a real flow-engine-driven root run through `runTurn` with a real temp git repo and a genuine r-ca (missing change-audit note) Tier-1 violation, and asserts the dispatch record reaches `settle_superseded_reprompt`. Confirmed via git-stash that it fails (stuck at `settle_pending`, reproducing run-18997/turn-19161 exactly) without `F-1` and passes with it.
- `V-2` Full gate/dispatch/settle regression battery — `TestDispatch*`, `TestCrashMatrix*`, `TestStopCAS*`, `TestSettleDriver*`, `TestOperatorAttention*`, `TestGateSettle*`, `TestRun2383*`, `TestRun1264*`, `TestRun1618*`, `TestRun9437*`, `TestChildGate*`, `TestRunTurnGate*`, `TestFlowGate*`, `TestBug298*`, `TestBug288*`/`Test.*[Bb]ug288`, `TestGateCheckpoint*` (127 tests) plus the CP-51 §2.2 Phase-A bundle (100 tests) — all pass unchanged.
- `go build ./...` passes; `go vet ./...` shows only one pre-existing, unrelated finding (`internal/structure/gitnexus.go`, not touched by this change).

## 9. Regression Guard

- tests: `TestLiveGateBlockSchedulesSettleDisposition` guards this specific path going forward.
- alerts: n/a
- audit checks: none beyond the existing gate/dispatch/settle battery.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51-PhaseAB-Timeline-And-Verification-Log.md §3.1 A6 marked done, citing this bug and its evidence (run-18997/turn-19161).
- notes left unchanged on purpose: reprompt targeting, park/notify-idle behavior, and Stop-race handling in the same block were left untouched — this fix only adds the missing settle-disposition call.
