# BUG-305 — Post-"done" follow-up turn hangs the desktop via a deferred, force-blocked flow gate

## Metadata

- Document ID: `BUG-305`
- Title: A follow-up turn on an already-"done" flow loop never broadcasts its TurnCompleted live (deferred behind a gate that force-blocks a "done" loop), hanging the desktop's turn stream
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: BUG-302 (admitted the post-"done" follow-up turn this bug then mishandles), CP-36 (loop lifecycle: "loop ends, no further spawns"), CP-51 A10 (second turn on same flow)
- Child Documents: none
- Related Documents: BUG-301 (sibling live-gate settle-disposition bug), BUG-288 (origin of the deferred-completion / gate-epoch machinery)
- Replaces: none
- Tags: agent-flow-engine, chat-history, desktop, ui-state

## AI Quick View

### Summary

- Found live while verifying CP-51 A10 on run-18997 (Chat mode) and run-18371 (Workflow mode): after a Review Loop finished, a follow-up chat message completed on the backend but the **desktop UI stayed stuck** — thinking row still spinning, Stop button still shown, history item still "running" with a loading icon. Switching to another chat and back showed the correct "done" state.
- **Durable state was correct; only the live stream hung.** Evidence from the run's own artifacts:
  - `dispatch.ndjson` for `turn-19468`: `terminal_completed → gate_evaluated → completion_committed → graph_settled → dependents_released → finalized`, all within ~10ms — the durable record settled cleanly (so reopen shows "done").
  - `runner.log`: `[settle] gate-block disposition run=run-18997 turn=turn-19468: dispatch record revision is stale` with **no `[gate] cwd=… diffLen=…` line** — the tell-tale that `runFlowGateAtEpoch` returned block from its early `gateEpochStillValid` guard, before any diff observation.
- Root cause chain:
  1. `flowEngineDriven` is set once on turn 0 and never reset, so a follow-up turn on a finished flow is still flow-engine-driven.
  2. `emitLocked` therefore **defers** the follow-up's `EventTurnCompleted` (does not broadcast/persist it) until the post-turn gate passes.
  3. The post-turn gate `runFlowGateAtEpoch` calls `gateEpochStillValid`, which returns **false when the loop status is "done"** → the gate returns `block=true` immediately (before its own log line).
  4. `gateBlocked=true` → `completed=false` → the real `TurnCompleted` is **never broadcast** to live subscribers.
  5. The desktop's turn stream (`sendTurn`) only ends when it sees `TurnCompleted` for its turn id; since that never arrives, `consumeStream` never returns → thinking row / Stop button / history spinner stay stuck. A separate settle driver (`evaluateSettleGate`, which allows on `flowLoopDone`) still finalizes the durable record, so a reopen reads "done".

### Current Ask

- Treat a follow-up admitted onto an already-"done" loop as plain chat (consistent with BUG-302): do not defer its completion behind the flow gate, and do not run that gate (it cannot pass a "done" loop). Publish + broadcast `TurnCompleted` live like any normal chat turn, so the desktop's turn stream ends.

### Key Decisions

- `V-1` Capture a per-turn flag `turnStartedAfterLoopDone` at `startTurn` admission (the same point BUG-302 reads the loop status), **not** at emit time — because the hub's own synthesis turn transitions running→done *during* its own turn and must still be deferred + gated. Emit-time "is the loop done now" would wrongly skip the gate for the synthesis turn.
- `V-2` In `emitLocked`, when `turnStartedAfterLoopDone`: skip the deferral (`deferGateCompleted=false`) and route `EventTurnCompleted` through the plain-chat completion branch (`status=Completed`, `signalChild`) instead of `markPendingFlowGateSettleLocked`. `signalChild(rootID)` is a no-op for a root run (no waiter), matching every existing plain-chat turn.
- `V-3` In `runTurn`, skip only the gate **evaluation** for a root run when `loopAlreadyDoneAtTurnStart` (the local BUG-302 already computes), treating it as a pass so the pass-path bookkeeping (turnInFlight clear, `scheduleSettleAfterGatePass`, finalizer) still runs. Do not skip the whole gate block (that would leak `turnInFlight=true`).

### Constraints

- additive-tests-only: only new tests added (`bug305_chat_followup_after_flow_done_stale_ui_test.go`); no existing test modified.
- cross-provider-parity: `emitLocked`, `runFlowGateAtEpoch`/`gateEpochStillValid`, and the `startTurn` admission gate take no `providerKey` and never branch on one — Case 1, provider-agnostic.
- Must not change any normal flow turn: the carve-out is keyed strictly on "loop already done **before** this turn started".

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` (`interactiveRun.turnStartedAfterLoopDone`, `startTurn` admission gate, `emitLocked` defer + `EventTurnCompleted` switch, `runTurn` gate-call guard)
- `apps/local-runner/internal/runner/gate_hook.go` (`gateEpochStillValid` / `gateEpochStillValidLocked`, `runFlowGateAtEpoch`)
- `apps/local-runner/internal/runner/bug305_chat_followup_after_flow_done_stale_ui_test.go` (regression tests)

## 1. Issue Summary

BUG-302 made a finished flow's hub keep accepting follow-up turns (previously rejected with 409). But the very next layer — the deferred-completion + post-turn-gate machinery — still treats such a run as active flow work. Because `gateEpochStillValid` deems a "done" loop invalid, the post-turn gate force-blocks the follow-up, the deferred `TurnCompleted` is never broadcast, and the desktop's live turn stream hangs. The durable dispatch record settles correctly via the independent settle driver, so the UI "unsticks" only on reopen.

## 2. Parent Links

- impacted coding plan: CP-51 A10 (second turn on same flow)
- impacted tech design: CP-36 loop lifecycle ("loop ends, no further spawns" — a finished loop is not a sealed chat)
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: any; the deciding factor is a non-empty `workspaceCwd` (so the gate reaches `gateEpochStillValid`) plus loop status "done".
- reproduction (live): finish a Review Loop, then send a follow-up chat message on the same run → backend completes and the dispatch record finalizes, but the desktop UI stays "running/thinking" until the chat is reopened.
- reproduction (test): `go test ./internal/runner -run TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion` — fails without the fix (the follow-up TurnCompleted is never broadcast to a subscriber).

## 4. Expected vs Actual

- expected: a follow-up on a finished flow completes like a normal chat turn — TurnCompleted broadcast live, composer returns to idle, history shows the terminal state immediately.
- actual: TurnCompleted deferred behind a gate that force-blocks the "done" loop and is therefore never broadcast; UI hangs until reopen.

## 5. Impact

- users affected: anyone sending a follow-up message after a Chat-mode or Workflow-mode flow finishes (the exact CP-51 A10 scenario unblocked by BUG-302).
- workflows affected: post-"done" follow-up chat across Codex/Claude/Grok (shared, provider-agnostic paths).
- severity: medium (no data loss or wrong durable state, but a confusing stuck UI that only clears on reopen).

## 6. Root Cause

- confirmed cause: `gateEpochStillValidLocked` (`gate_hook.go`) returns false for loop status `"done"`; `runFlowGateAtEpoch` returns `block=true` from its early `if !gateEpochStillValid { return true }` before logging `[gate]`. Combined with `emitLocked`'s deferral of a flow-engine root's `TurnCompleted`, the real completion event is never broadcast for a follow-up admitted onto a "done" loop.
- evidence: run-18997 `dispatch.ndjson` (durable settle to `finalized`) + `runner.log` line `[settle] gate-block disposition … revision is stale` with no `[gate]` line; desktop symptom clears on reopen (durable state correct).

## 7. Fix Strategy

- `F-1` Add `interactiveRun.turnStartedAfterLoopDone`, set at `startTurn` admission (`= st == "done"` for a root run; reset false every turn).
- `F-2` `emitLocked`: guard the deferral and the `markPendingFlowGateSettleLocked` root branch with `!rs.turnStartedAfterLoopDone`, so a post-"done" follow-up publishes `Completed` and broadcasts live (plain-chat branch).
- `F-3` `runTurn`: for a root run with `loopAlreadyDoneAtTurnStart`, skip the gate evaluation (treat as pass) while keeping all pass-path bookkeeping so the turn finalizes cleanly.

## 8. Validation

- `V-1` **cross-provider-parity (Case 1, agnostic — confirmed by reading + grep):** `emitLocked`, `gateEpochStillValid`, `runFlowGateAtEpoch`, and the `startTurn` admission gate take no `providerKey` and never branch on one; every `provider` reference in those paths is event/log metadata. One representative provider (Codex) in the new tests is sufficient.
- `V-2` **additive-tests-only:** only `bug305_chat_followup_after_flow_done_stale_ui_test.go` added; no existing test changed.
- `V-3` **git-stash regression discipline:** with the fix stashed, `TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion` fails at the live-broadcast assertion ("TurnCompleted was never broadcast live"); with the fix, it passes. A non-empty `workspaceCwd` in the test is required to reach the `gateEpochStillValid` block that reproduces the bug.
- `V-4` **discriminator guard:** `TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState` proves the flag is false when the loop is running at admission (normal flow turns stay deferred + gated) and true only when it is already "done".
- `V-5` **full-suite regression:** `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1` — failure set does not grow beyond the known pre-existing environment-only set (missing `codex` binary, Windows home-dir/sandbox/exec-bit quirks) plus two pre-existing parallel-load flaky flow tests (`TestAdvanceHubDoneThroughEdgeDispatchesHubNotify`, `TestRun20332FlowHubHistoryParityForEveryProvider`), both of which pass in isolation and flake identically on the unfixed code (confirmed via git-stash).
- `go build ./...` passes.

## 9. Regression Guard

- tests: `TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion` (behavioral — live broadcast), `TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState` (discriminator).
- alerts: n/a
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51 A10 note updated to reference this follow-on UI fix.
- notes left unchanged on purpose: `gateEpochStillValid`'s "done ⇒ invalid" semantics are correct for its other callers (a gate genuinely in-flight when the loop finishes via synthesis must still abort); this fix does not change that function — it only prevents a post-"done" follow-up from being deferred into / evaluated by that gate in the first place.
