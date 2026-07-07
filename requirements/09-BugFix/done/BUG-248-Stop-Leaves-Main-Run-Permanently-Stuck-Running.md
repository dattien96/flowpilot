# BUG-248: Stop Leaves Main Run Permanently Stuck "Running"

## Metadata

- Document ID: `BUG-248`
- Title: `Stop Leaves Main Run Permanently Stuck "Running"`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: [BUG-247: Stop From Child-Focused View Leaves Parent Loop Running](./BUG-247-Stop-From-Child-Focused-View-Leaves-Parent-Loop-Running.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `none`
- Related Documents: [CA-223: Cascade Main Stop To Running Child Agents](../../../change-audit/CA-223-parent-interrupt-cascades-to-child-agents.md), [CA-244: Stop From Child-Focused View Cascades To Parent Loop](../../../change-audit/CA-244-stop-from-child-view-cascades-to-parent.md)
- Replaces: `none`
- Tags: `agent-spawn, multi-agent, desktop, stop-control, regression, race-condition`

## AI Quick View

### Summary

- User report (follow-up to BUG-247): "bấm stop thì child agent stop nhưng main hang. bấm tiếp cũng k được nữa" (pressing Stop stops the child, but the main run hangs; pressing Stop again does nothing either).
- Root cause is a genuine race, not a UI routing bug: `stopAgentLoop`'s `turnCancel()` only *signals* cancellation — the run's own `finishTurn` flips its `status` to a terminal value asynchronously, once that turn's goroutine actually observes `ctx.Done()`. `stopAgentLoop` builds and returns its `AgentGraphSnapshot` synchronously, in the same instant it fires `turnCancel()`, so the snapshot can (and, per a new deterministic Go test, reliably does) report a just-cancelled child as still `"running"`.
- Worse, that stale reading is permanent, not just momentarily stale: a cancelled child's `finishTurn` path never re-emits an `agent_graph_updated` event to the parent's orchestration stream (that only happens on the normal-completion path), so no later event ever arrives to correct it. The desktop's `deriveOrchestrationRunStatus` let any "child still running" reading override even a definitive `loopState.status === "stopped"`, so `store.status` got stuck at `"running"` forever — explaining why a second Stop press did nothing (the loop was already server-side `"stopped"`, so the second press fell through to a no-op interrupt on an already-cancelled run).
- A second, independent instance of the same class of bug: `AgentGraphSnapshot.Runs[]` is served from `AgentOrchestrator`'s own summary cache (`upsertSummary`/`graphSnapshot`), not from `interactiveRun.status` directly — the two are kept in sync only by `emitLocked` reacting to turn-progress events. `stopAgentLoop` cancelling a turn does not go through that sync path synchronously either, so the Agents-panel-facing summary was independently stale for the same reason.
- Fix has two parts: (1) `deriveOrchestrationRunStatus` (desktop) now treats a `"stopped"` loop as authoritative over any child's (possibly stale) `"running"` reading; (2) `stopAgentLoop` (Go) now eagerly writes `RunStatusCancelled` into both `interactiveRun.status` and `AgentOrchestrator`'s cached summary for the parent and every child it just cancelled, before building the snapshot it returns — closing the race at its source instead of only papering over it on the client.

### Current Ask

- Fix the permanent "main hangs after Stop" regression surfaced immediately after the BUG-247 fix started actually driving the parent-loop-stop path from a child-focused view (the race existed for the main-chat Stop path too, per CA-223, but was rarely exercised early enough after `stopAgentLoop` to be visible before this).

### Key Decisions

- `D-1` Treat `loopState.status === "stopped"` as an unconditional override at the top of `deriveOrchestrationRunStatus`, ahead of the existing "any child running/waiting → overall running/waiting" checks. This is deliberately narrower than also special-casing `"done"`: a loop reaching `"done"` naturally may still have a last child genuinely finishing up, where "still show running until the last child lands" is correct (BUG-231's existing "blocked loop, running child still wins" test covers the same precedence question for a *non-terminal* pause) — but `"stopped"` means every child was just told to cancel, so there is no legitimate reason left for a "running" reading to be honored.
- `D-2` Fix the race at the source in Go (`stopAgentLoop`), not only client-side: patching only `deriveOrchestrationRunStatus` would have fixed the aggregate `status`/`blocked` flag but left individual child run badges (Agents panel) reporting stale "running" indefinitely, since those are populated from the exact same snapshot.
- `D-3` `stopAgentLoop` writes the cancelled status into *both* `interactiveRun.status` (read by `listAgentRunSummaries`, run-history/status endpoints) and `AgentOrchestrator`'s summary cache (read by `graphSnapshot`/`AgentGraphSnapshot.Runs`, what the desktop's orchestration board and status derivation actually consume) — confirmed by direct code reading that these are two independently-populated stores kept in sync only by `emitLocked`, not by a single source of truth.
- `D-4` No change to `finishTurn` itself: it still runs later and re-asserts the same terminal status/emits its own `EventTurnFailed` on the run's own stream — writing the same value twice is idempotent and harmless.

### Constraints

- Scoped to the stop/cancel path (`stopAgentLoop`, `deriveOrchestrationRunStatus`); `Interrupt` (the non-loop single-turn cancel path) was not changed — CA-223 already covers its child-cascade behavior, and it does not return a synchronous `AgentGraphSnapshot` the way `stopAgentLoop` does, so it is not subject to this exact race.
- Did not attempt to make a cancelled child's `finishTurn` re-emit `agent_graph_updated` for the parent (the deeper asymmetry vs. the normal-completion path) — eagerly setting status in `stopAgentLoop` closes the actual user-visible gap without touching the broader turn-lifecycle/event-emission contract.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` `deriveOrchestrationRunStatus` (line ~2291).
- `apps/local-runner/internal/runner/interactive_service.go` `stopAgentLoop` (line ~458), `finishTurn` (line ~2995, confirmed source of the async status flip), `emitLocked` (line ~1651, confirmed source of `AgentOrchestrator.upsertSummary` sync and the `EventTurnFailed` → `emitAgentGraphLocked` path that fires on failure but happens *after* `finishTurn` would have already returned in the stop case).
- `apps/local-runner/internal/runner/agent_orchestrator.go` `upsertSummary`, `graphSnapshot`, `currentSummary` (confirmed `AgentGraphSnapshot.Runs` is sourced from the orchestrator's own cache, not `interactiveRun.status`).
- New tests: `store.test.ts` `"deriveOrchestrationRunStatus: a stopped loop wins even if a child's status snapshot is stale-running (BUG-248)"`; `interactive_service_test.go` `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`.

## 1. Issue Summary

Immediately following the BUG-247 fix (which made a child-focused Stop press also drive the parent-loop-stop path), users hit a worse regression: pressing Stop correctly stopped the child, but the main run's status stayed stuck on `"running"` forever — not just briefly. A second Stop press had no effect either, because the backend had already fully stopped the loop; there was simply nothing left to correct the desktop's stale status reading.

## 2. Parent Links

- Direct follow-up to [BUG-247](./BUG-247-Stop-From-Child-Focused-View-Leaves-Parent-Loop-Running.md) — that fix made `stop()` actually exercise the `stopAgentLoop` + `deriveOrchestrationRunStatus` path from a child-focused view for the first time, which is what surfaced this pre-existing race quickly and reliably (it existed for the main-chat Stop press too, per CA-223, but a real network round trip there usually gave the async cancellation just enough time to land first).

## 3. Environment and Reproduction

- environment: Desktop app, any provider/chat mode, a chat with an actively running child agent whose provider turn does not return instantly on cancellation (i.e., realistic timing, not an already-idle child).
- reproduction steps:
  1. Start a chat/flow that spawns a child agent; wait until it is `running`.
  2. Press Stop (from either the child-focused view or the main chat).
  3. Observe: the child's own transcript reflects the interruption, but the main chat's composer stays locked ("Waiting for the current turn…") and the Stop/Send controls remain in the blocked state indefinitely.
  4. Press Stop again — no change.
- frequency: race-dependent (timing between the synchronous snapshot read and the async turn-cancellation goroutine), but the new deterministic Go test (`TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, using a blocking fake adapter) proves the stale reading is the *reliable*, not occasional, outcome for a still-in-flight child turn.

## 4. Expected vs Actual

- expected: after Stop, the main run's status settles to a terminal state (`cancelled`) as soon as the stop call resolves, regardless of exactly how far each child's own turn-cancellation goroutine has progressed.
- actual: the main run's status could get stuck on `"running"` permanently, because (a) the snapshot `stopAgentLoop` returns can report a cancelled child as still `"running"`, (b) `deriveOrchestrationRunStatus` let that reading override the loop's own definitive `"stopped"` state, and (c) no later event exists to ever correct it for the cancellation path specifically.

## 5. Impact

- users affected: any user stopping a multi-agent run while a child's turn is genuinely mid-flight (the common case, not an edge case).
- severity: high — this is a hard hang with no in-app recovery path once triggered; the only workaround was restarting the chat/app.

## 6. Root Cause

- confirmed cause 1 (desktop): `deriveOrchestrationRunStatus` checked `snapshot.runs` for any child `"running"`/`"waiting_*"` status *before* checking `snapshot.loopState.status`, so a stale-running child reading always won over a definitive `"stopped"` loop.
- confirmed cause 2 (backend): `stopAgentLoop` (`interactive_service.go`) calls `child.turnCancel()` (a signal, not a synchronous state change) and immediately builds/returns the `AgentGraphSnapshot` from `AgentOrchestrator`'s cached summaries. The child's `status` field only becomes `RunStatusCancelled` later, inside `finishTurn`, once that child's turn goroutine notices `ctx.Done()` — a window proven non-zero by the new test (a blocking fake adapter that provably has not returned when the snapshot is inspected). And because a cancelled child's `finishTurn` path does not itself re-emit `agent_graph_updated` for the parent (unlike the normal-completion path via `releaseDependentAgents`/`advanceOrNotifyHub`), no later corrective event exists — the desktop has nothing to fall back on.
- confirmed via direct code inspection of `emitLocked`/`finishTurn`/`stopAgentLoop`/`AgentOrchestrator.graphSnapshot`, and via a new deterministic regression test that reproduces the exact race with a fake adapter that blocks past `ctx.Done()` so `finishTurn` provably has not run when the snapshot is inspected — it failed before the fix (`status "running", want "cancelled"`) and passes after.

## 7. Fix Strategy

- `F-1` `store.ts`'s `deriveOrchestrationRunStatus`: check `snapshot.loopState.status === "stopped"` first and return `"cancelled"` immediately, ahead of the existing childStatuses precedence checks.
- `F-2` `interactive_service.go`'s `stopAgentLoop`: when `turnCancel()` is actually invoked for the parent, eagerly set `parent.status = RunStatusCancelled` on the spot (previously only the async `finishTurn` did this).
- `F-3` Same for each child whose `turnCancel()` fires: eagerly set `child.status`/`child.agentStatus`, and additionally patch `AgentOrchestrator`'s cached summary for that child (`currentSummary` + `upsertSummary`) before building the snapshot — closing the second, independent staleness source (`AgentGraphSnapshot.Runs` is served from the orchestrator's cache, not `interactiveRun.status`).

## 8. Validation

- `V-1` `go build ./...` — clean.
- `V-2` New test `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously` (uses a fake adapter that blocks past `ctx.Done()` so `finishTurn` provably has not run when the snapshot is inspected): failed before the fix (`want "cancelled"`, got `"running"`), passes after.
- `V-3` `go test ./internal/runner/...` (full package): 4 pre-existing environment-only failures (Windows path-quoting in `TestStartInteractiveAuthLaunchesFromWorkspace`, provider-home skill precedence in the 3 `TestSkillsMerge*WithPrecedence` tests) confirmed present identically on the pre-fix baseline via `git stash` — no regressions, all other tests including `TestStopAgentLoopCancelsParentTurn` and `TestInterruptParentCancelsRunningChildAgents` pass.
- `V-4` `apps/desktop-flowpilot`: `tsc --noEmit` — clean.
- `V-5` New test `"deriveOrchestrationRunStatus: a stopped loop wins even if a child's status snapshot is stale-running (BUG-248)"` added to `store.test.ts`; full suite run via the same isolated `tsc` + Node `--test` + alias-hook harness used for BUG-247 (this repo's `test:phase1` script does not itself execute `store.test.ts`): 77 tests, 75 passed, the same 2 pre-existing environment-only failures (no DOM `localStorage`; one timing-sensitive test) confirmed present on the pre-BUG-247 baseline too.

## 9. Regression Guard

- tests: `interactive_service_test.go` `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`; `store.test.ts` `"deriveOrchestrationRunStatus: a stopped loop wins even if a child's status snapshot is stale-running (BUG-248)"`.
- audit checks: `gitnexus_detect_changes()` was not run this session — GitNexus MCP tools remained unavailable in this thread (see BUG-247's same note).

## 10. Follow-Up Document Updates

- None. This closes a race the BUG-247/CA-223 stop-cascade design already assumed away; no upstream SS/SD/CP rule change.
