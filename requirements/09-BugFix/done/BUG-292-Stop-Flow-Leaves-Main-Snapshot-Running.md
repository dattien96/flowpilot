# BUG-292: Stop Flow Leaves Main Snapshot Running

## Metadata

- Document ID: `BUG-292`
- Title: `Stop Flow Leaves Main Snapshot Running`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: [CP-51 Phase A/B log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md), [CP-51 durable dispatch](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `none`
- Related Documents: [BUG-291](./BUG-291-Regression-Gate-Dismiss-Orphans-Child-Run.md), [CA-369](../../../change-audit/CA-369-regression-gate-dismiss-terminal-action.md), [CA-370](../../../change-audit/CA-370-stop-flow-snapshot-reconciliation.md)
- Replaces: `none`
- Tags: `agent-flow-engine, stop-flow, child-run, ui-state, regression, a1`

## AI Quick View

### Summary

- In live run `run-11679`, the runner correctly stopped parent and coder, but desktop still rendered main as active after stopping from a child-focused view.
- A cached parent snapshot retained `running` and won the main-card display over the current terminal store status.

### Current Ask

- Reconcile every saved local run snapshot with the terminal `stopAgentLoop` graph, so returning to main cannot restore a stale active state.

### Key Decisions

- `F-1`: Treat a stopped loop as authoritative for the parent and any non-terminal child snapshot.
- `F-2`: Preserve already completed and failed child snapshots; clear terminally invalid UI residue on snapshots changed to cancelled.
- `F-3`: Apply the same reconciliation when `stopAgentLoop` returns a stopped graph inside a durable-checkpoint error.

### Constraints

- The runner's stop cascade is not redesigned; logs show it already persisted `cancelled` for both runs.
- Add tests only; no existing test files are changed.

### Open Questions

- A live retest in the running desktop process is still required after loading the rebuilt client.

### Source Refs

- Run log: `.flowpilot/logs/features/agent-flow-engine/run-11679.ndjson`.
- Persisted sessions: `.flowpilot/chats/sessions.ndjson` (`run-11679`, `run-11684`).
- Desktop: `store.ts` `stop`, `AgentsPanel.tsx` main-card snapshot selection.

## 1. Issue Summary

Stopping an A1 review-loop flow while viewing its coder child cancelled the child correctly, yet the main card continued to display as active. The run did not remain active on the runner; the stale indicator was a desktop reconciliation regression.

## 2. Parent Links

- CP-51 Phase A1 defines the live parent/child flow-liveness verification.
- CP-51 durable dispatch defines the Stop linearization and recovery contract.
- SD-20 and SS-14 define the flow-gate/regression-safety context; this fix restores their existing terminal UI behavior without changing policy.

## 3. Environment and Reproduction

- Environment: FlowPilot desktop with local runner, 2026-07-20.
- Reproduction steps:
  1. Start review-loop bug flow and focus its coder child.
  2. Stop the flow from the child-focused UI.
  3. Return to or inspect main.
- Frequency: deterministic when a saved parent snapshot still says `running`.

## 4. Expected vs Actual

- Expected: a successful Stop renders parent and active children terminal immediately; returning to main must not restore a spinner or active card.
- Actual: `stop()` updated the active store state and agent graph but left `_runSnapshots[parent]` unchanged. `AgentsPanel` preferred that stale snapshot while a child remained focused.

## 5. Impact

- Users affected: operators stopping a flow from a child transcript.
- Workflows affected: A1 review-loop flows and any child-focused orchestrated flow.
- Severity: high usability/liveness presentation regression; runner work is already stopped and no data is lost.

## 6. Root Cause

- Hypothesis: runner cancellation failed to propagate from coder to main.
- Confirmed cause: runner evidence contradicts that hypothesis. `run-11679` persisted parent `status=cancelled`, `loop_state.status=stopped`, child `status=cancelled`, and child `parent_stop_gen_seen=1`. Desktop `stop()` left saved snapshots untouched, while `AgentsPanel` selected `mainSnapshotStatus ?? activeStatus` during child focus.
- Evidence: run log and session records above; local source trace of `store.ts` `stop` and `AgentsPanel.tsx` `mainRunStatus`.

## 7. Fix Strategy

- `F-1`: after `stopAgentLoop` returns its authoritative stopped graph, reconcile saved parent/child snapshots.
- `F-2`: mark the parent and any reported non-terminal child snapshot `cancelled`; preserve reported `completed`, `failed`, and `cancelled` children.
- `F-3`: remove thinking rows, pending approval/question cards, recoverable state, and streaming markers from snapshots converted to terminal.
- `F-4`: reuse the reconciliation for both successful Stop responses and partial durable-failure responses carrying an embedded stopped graph; the no-snapshot fallback also closes non-terminal local snapshots.

## 8. Validation

- `V-1`: New dedicated UI matrix test covers parent, cancelled/completed/failed children, stale `running`/`waiting_approval`/`waiting_question` children, and a partial durable-failure response carrying a stopped graph; passed 2/2.
- `V-2`: Existing runner probes passed: `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, `TestInterruptParentCancelsRunningChildAgents`, and `TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle`.
- `V-3`: `npm --prefix apps/desktop-flowpilot run build` passed (`tsc --noEmit`, Vite, Electron builds).
- `V-4`: `git diff --check` passed; only existing line-ending warnings were reported.

## 9. Regression Guard

- New `stop_parent_snapshot_reconciliation.test.ts` verifies terminal snapshot reconciliation from a child-focused flow.
- The test explicitly covers stale child state windows that can occur while child `finishTurn` observes cancellation asynchronously.
- No pre-existing test was edited.

## 10. Follow-Up Document Updates

- Upstream docs unchanged: this is a desktop reconciliation correction to the existing Stop contract, not a policy or architecture change.
- CP-51 Phase A/B run log should record `run-11679` as backend stop-cascade passed with a pre-fix stale-main UI failure; live retest remains required.
