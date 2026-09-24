# BUG-410: Post-SIGKILL flow does not self-recover — run loads `cancelled`, stale `hub_stalled`, needs manual flow-control to finalize

## Metadata

- Document ID: `BUG-410`
- Title: `Mid-flow SIGKILL leaves run cancelled/parked with stale blockReason hub_stalled; resume surfaces don't re-drive; outer run can stay running after loop done`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md), [CP-64-Test-Steps](../../07-Coding-Plan/done/CP-64-Test-Steps.md)
- Feature Keys: `flow-resume`, `recovery`, `dispatch-durability`

## AI Quick View

### Summary

- Symptom A (CP43-002, run-4325 grok bug-harness): runner SIGKILLed mid-flow after pending canonical record staged; restart → `POST /resume` loads the run `status=cancelled`, reviewer child `cancelled`, `synthesis` parked `WAITING_USER_APPROVAL`, loopState `status:"running"` with **stale** `blockReason:"hub_stalled"`. `/agent-loop/resume`, `/agent-loop/continue`, and a plain `/turns` prompt all failed to re-drive the hub; only `POST /flow-control {"status":"done"}` advanced synthesis→done (audit SKIPPED) and finalized the pending record exactly once.
- Symptom B (CP-64, run-3914 opencode): after `continue`, the run reached `loopState=done` with all steps DONE but the outer run status stayed `running` — the settle attempt hit `dispatch record revision is stale` once (runner.log:6157) and the handle was never re-terminalized.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** after a mid-flow SIGKILL+restart, a resumed run does not re-drive: it loads `cancelled`/parked with stale `blockReason:"hub_stalled"` while `loopState` reports `running`; standard resume/continue surfaces return snapshots but dispatch nothing; a plain turn runs as ordinary chat. Separately, a run whose loop reached `done` can keep outer `status:"running"` after a `dispatch record revision is stale` settle failure (run-3914 limbo).
- **Expected:** post-restart resume restores the loop to a drivable state (or fails loudly); terminal loop state always propagates to the outer run status.
- **Actual:** reaching terminal state after a mid-flow kill requires manual `flow-control` surgery — a Desktop user sees an unexplained park or a "running" run that is actually finished.
- **Impact:** crash recovery exists at the dispatch/pending-store layer (durable records, idempotent finalize verified) but not at the flow-orchestration layer; operator-visible wedge on every mid-flow crash; stale outer status leaves zombie "running" runs.

## Reproduction

- Symptom A: run `bug-harness` (grok); wait for implement gate pass + pending record staged; `pkill -9` the runner; restart; `POST /client/workflow-runs/run-4325/resume` → observe `status=cancelled`, child `cancelled`, `synthesis` `WAITING_USER_APPROVAL`, `blockReason:"hub_stalled"`. Try `/agent-loop/resume`, `/agent-loop/continue`, plain `/turns` — no dispatch. Only `POST /flow-control {"status":"done"}` advances (audit SKIPPED).
- Symptom B: on a run where settle hit `dispatch record revision is stale` (runner.log:6157), observe `GET /client/workflow-runs/run-3914` → `{"status":"running"}` while `agent-graph` shows `loopState=done` + all steps DONE.

## Root cause

- Flow-level rehydration gap: `/resume` reloads the run handle but does not re-drive a hub parked mid-turn or clear stale `blockReason`/`WAITING_USER_APPROVAL` step states; `agent-loop/continue` only acts on `blocked` loops and `/agent-loop/resume` returns the snapshot without dispatching — no path reconciles `status=cancelled` + `loopState running` post-crash (CP43-002; pending-store side verified correct — durability + exactly-once finalize work).
- Symptom B: settle attempt failed on `dispatch record revision is stale` and no retry re-terminalized the outer run handle, leaving `status:"running"` over a `done` loop.

## Evidence

- `~/fp-beds/lt-evidence/cp43/l434b-steps-final.json` — reviewer `CANCELED`, audit `SKIPPED`; `l434b-events.json`, `l434b-run.json`; `runner.log:14661-14683`; `RESULT.md` (BUG-LIVE-CP43-002).
- `~/fp-beds/lt-evidence/cp64/run3914/status.json` — `{"runId":"run-3914","status":"running"}` post-done; `runner.log:6157` (`dispatch record revision is stale`); `RESULT.md` (L-64-1 notes).
- Also observed in CP-51: post-restart run-level resume lands `cancelled`, hub never re-driven — "known BUG-LIVE-CP43-002 shape" (`~/fp-beds/lt-evidence/cp51/RESULT.md` L-51-1/L-51-7).

## Severity

`medium` — recoverable only via undocumented manual `flow-control`; durable state itself is intact, but every mid-flow crash produces an operator-visible wedge or a zombie running status.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause (restart symptom): `loadPersistedRun` kept a stale block-only `BlockReason` on a non-blocked restored loop, and `resumePendingLoopWork` only replayed durable intents — a hub that died mid-turn with no armed intent never re-drove (cancelled run + stale `hub_stalled` + inert resume).
- Fix A: `loadPersistedRun` clears `BlockReason` when the restored loop isn't `blocked`; new `redriveQuietFlowLoop` re-drives the hub when a resumed flow parent's loop reads `running` but nothing is in flight/queued/parked anywhere (heals crash `cancelled`, re-arms `autoOrchestrate`). Wired into `resumeAgentLoop` + `/resume`.
- Root cause (settle symptom): `reconcileDispatchTurn` was a wake-marker no-op — a stale `CommitTerminalAndSettleIntent` never retried.
- Fix B: bounded retry — re-read the durable record and re-attempt the terminal/receipt CAS instead of dropping terminalization.
- Files: `internal/runner/interactive_resume.go`, `interactive_service.go`, `interactive_handlers.go`, `dispatch_live.go`.
- Tests: `TestBug410_ReconstructClearsStaleBlockReason`, `TestBug410_ResumeRedrivesQuietMidFlightFlow`, `TestBug410_TerminalCommitRetriesOnStaleRevision`, `TestBug410_BridgeTerminalRecoversFromStaleCAS`. Baseline-red verified.
- Note: first cut deadlocked on `loopAllowsNextTurnLocked` under `s.mu` (helper re-locks in the blocked:paused branch) — caught by `TestCA801_InMemoryResumeParksConfirm`; advancing check now inlined under the held lock.
