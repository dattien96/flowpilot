# BUG-378: Completed child runs leak their `devin acp` processes for the runner lifetime

## Metadata

- Document ID: `BUG-378`
- Title: `devin acp child processes stay alive 17+ min after run settles; reaped only on runner shutdown`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Feature Keys: `ai-providers, agent-spawn`

## AI Quick View

### Summary

- A `spawn_agent` child run settles `turn_completed`, but its `devin acp` subprocess is never reaped — cp70 child run-2497's acp PID 37628 (spawned 06:28:39, settled 06:28:57) was still alive at 06:46+ (~17.5 min). Flow children run-3600 (PID 43548) and run-4052 (PID 45852) show the same.
- One runner held **6** live `devin acp` children; only the main warm session (`unexpected-thumb`) plausibly needs to persist. Processes die only when the runner exits.
- Regression of the BUG-334 child-process-isolation guarantee — the R7 pass criterion (`ps aux | grep "devin acp"` clean after child completes) fails.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: `devin acp` processes spawned for child runs (spawn_agent / flow children) remain as runner children long after their runs settle.
- **Expected**: Child acp process terminated when the child run completes (or after a short TTL); `ps` shows no leaked acp processes post-settle.
- **Actual**: cp70 `r7-process-leak.txt` — run-2497 `[settle] finalized run=run-2497` at 06:28:57, PID 37628 still alive ~17.5 min later; run-3600 / run-4052 identical. 6 live `devin acp` children under one runner.
- **Impact**: medium — process/memory leak proportional to child-run count on long-lived runners; risk of fd/token accumulation (each child session also leaves `devin-mcp-stdio` shim tokens per cp66 cleanup note).

## Reproduction

1. Runner `/tmp/fp-lt-cp70 runner serve --port 19270`, `FLOWPILOT_DEVIN_AGENT=1`, provider `devin`/`devin/swe-2-max`.
2. On a chat run (cp70 run-1), have the agent call `flowpilot__spawn_agent` → child `run-2497` on its own session `plural-ornament`; child replies `CHILD_OK` and settles.
3. `ps aux | grep "devin acp"` → the child's acp PID persists ≥17.5 min after `[settle] finalized`.
4. Same for flow children in a `bug-harness` run (cp70 run-665 → children run-3600, run-4052 completed, PIDs alive).

## Root cause

- Not yet isolated (capture-only). Observed: child run teardown finalizes the run/turn state but does not terminate the per-child `devin acp` process tree; only the warm parent session should be retained. Contrast with `TestCleanupSessionsTearsDownProviderPools` family (BUG-334 coverage) — teardown gap appears specific to devin acp child sessions.

## Evidence

- `~/fp-beds/lt-evidence/cp70/RESULT.md` (BUG-LIVE-CP70-1), `r7-process-leak.txt` (ps snapshot + spawn/settle log lines), `r7-child-sessions.json`, `r7-child-snapshot.json`, `runner.log` (`[settle] finalized run=run-2497` 06:28:57; PID 37628 alive 06:46+).

## Severity

- medium

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: `CloseDevinProcessesForChildRun` existed but was never wired — child-run terminal teardown only called the Opencode variant, so Devin ACP child processes stayed alive until runner shutdown.
- Fix: added `CloseDevinProcessesForChildRun(rs.id)` alongside the Opencode teardown in the child-terminal path (`interactive_service.go`); devin process registry already segmented scopes by `|child:<runID>`.
- Tests: `bug378_devin_child_process_teardown_test.go`.
