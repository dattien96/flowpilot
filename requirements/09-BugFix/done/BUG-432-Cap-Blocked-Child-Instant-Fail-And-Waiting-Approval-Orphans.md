# BUG-432: Children spawned while loop is cap-blocked fail instantly; `waiting_user_approval` children orphaned at flow done + stale pendingGateBlock survives settle

## Metadata

- Document ID: `BUG-432`
- Title: `Cap-park race fails spawned children (st=blocked); reconcileChildRunsOnFlowDone skips waiting_user_approval; pendingGateBlock still answerable on a done run`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp60/RESULT.md` (BUG-LIVE-3 + BUG-LIVE-4)
- Feature Keys: `agent-loop`, `child-spawn`, `cap-escalation`, `flow-reconcile`, `gate-decision`

## AI Quick View

### Summary

- Answering a cap-park question ("Reprompt coder anyway") spawned child `task-911-red-stub-fix` (`run-29375`) while `loop=blocked cap 5 reached` — the child was created and FAILED in the same second (cap-park race). Retry after `extend-cap` + continue (`run-29443`) completed normally.
- After `flow_run_complete`, `reconcileChildRunsOnFlowDone` settles `running`/`waiting_approval`/`waiting_question` children but **not** `waiting_user_approval` (`RunStatusWaitingUserApr`) → the gated `tdd` child `run-27148` remains `waiting_user_approval` forever.
- A stale `pendingGateBlock` (r-reg options from the final turn's failing `go test`) survives on the done run — `POST /gate-decision` still accepts `keep-test-fix-code|suggest-requirement-change|custom` and would `startTurn` a reprompt on a dead loop.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

### Symptom

1. A child spawned while the parent loop is cap-blocked is created with `st=blocked` context and transitions to FAILED immediately — same-second create+fail.
2. Terminal flow leaves a `waiting_user_approval` child orphaned forever (never reconciled).
3. A `pendingGateBlock` survives `flow_run_complete`; the gate-decision endpoint on a terminal run still enumerates live options and would dispatch a reprompt turn on a dead loop.

### Expected

- Spawn requests during cap-block are deferred/rejected cleanly (or the child waits) instead of instant-FAILED.
- Flow-done reconciliation settles **all** non-terminal child statuses, including `waiting_user_approval`.
- Terminal runs clear/reject pending gate state — `/gate-decision` on a done run should be rejected outright.

### Actual

- `run-21751-flow-diag.ndjson` 04:57:51: `child_spawn_created` at `st=blocked`; `run-21751-step-transitions.ndjson`: `task-911-red-stub-fix FAILED` 21:57:51Z (same second).
- `run-21751/agent-graph-final.json`: child `run-27148` still `waiting_user_approval` after parent `flow_run_complete_done`.
- `l60-2-revive-probes.txt`: `POST /gate-decision` on the done run enumerated live options (probe returned 400 `invalid_option` for the test value — did not fire a real one).

### Impact

- Operator-visible "instant fail" children that need a manual extend-cap+continue retry.
- Orphaned `waiting_user_approval` runs pollute run lists and hold a stale gate state that can (in principle) start a reprompt turn on a dead loop — terminal-state hygiene gap, fallout of the CP-60 wedge family.

## Reproduction

1. Drive a vibe/flow loop to `blocked` with `cap N reached`; answer the escalate question with "Reprompt coder anyway" → observe `child_spawn_created` + child `FAILED` in the same second.
2. Let a flow reach `flow_run_complete` while a child sits `waiting_user_approval` → `GET` the child run → still `waiting_user_approval`.
3. `POST /client/workflow-runs/<done-run>/gate-decision` with a valid option name → accepted path exists (would `startTurn` a reprompt on the dead loop).

## Root cause

- (spawn race) Child spawn during `st=blocked`/cap-reached is not gated — the child is created then immediately failed by the still-blocked loop state.
- (orphan) `apps/local-runner/internal/runner/flow_step_runtime.go:552` — `reconcileChildRunsOnFlowDone`'s settle switch (:560-563) covers `RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingQuestion` but omits `RunStatusWaitingUserApr` (`waiting_user_approval`) → `default: continue` skips it.
- (stale gate) No teardown clears `pendingGateBlock` on `markFlowRunComplete`, and `/gate-decision` does not reject terminal-status runs.

## Evidence

- `~/fp-beds/lt-evidence/cp60/RESULT.md` — BUG-LIVE-3 (`run-21751-flow-diag.ndjson` 04:57:51 `child_spawn_created` at `st=blocked`; `run-21751-step-transitions.ndjson` `task-911-red-stub-fix FAILED` 21:57:51Z) and BUG-LIVE-4 (`run-21751/agent-graph-final.json` run-27148 `waiting_user_approval`; `l60-2-revive-probes.txt` gate-decision probe).
- Verified on main worktree HEAD `435e336b`: `flow_step_runtime.go:552` + switch at :560-563 — `RunStatusWaitingUserApr` absent from the settle list.

## Severity

- `medium` (low per-item: instant-fail is recoverable via extend-cap+continue; orphans/stale gate are terminal-state hygiene) — grouped as one reconcile/terminal-hygiene defect family.

## Completion Notes (implemented 2026-09-23, CA-921)

- Root cause: terminal flow completion left `waiting_user_approval` children orphaned, a stale `pendingGateBlock` survived, and child spawns raced a blocked parent loop — stale gate-decision state could fire remediation on a dead loop.
- Fix: flow `done` reconciles waiting-user children to completed + clears `pendingGateBlock`; spawn path refuses new children while the parent loop is blocked; stale gate decisions reject when nothing is pending.
- Files: `internal/runner/flow_step_runtime.go`, `interactive_service.go`.
- Tests: `TestBug432_ReconcileSettlesWaitingUserApprovalChild`, `TestBug432_FlowDoneClearsPendingGateBlock`, `TestBug432_GateDecisionRejectsWhenNothingPending`, `TestBug432_SpawnRefusedWhileParentLoopBlocked`. Baseline-red verified.
