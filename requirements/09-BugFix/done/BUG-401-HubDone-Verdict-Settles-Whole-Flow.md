# BUG-401: Hub-level `done` verdict settles the whole flow, skipping node-done machinery

## Metadata

- Document ID: `BUG-401`
- Title: `Hub done verdict settles whole flow — parked vibe sprint never restored; operator done bypasses hub done-edge`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), [CP-58-Test-Steps](../../07-Coding-Plan/done/CP-58-Test-Steps.md)
- Feature Keys: `vibe-mode`, `flow-engine`, `owner-debate`

## AI Quick View

### Summary

- Symptom A (CP60-1, run-21751): `vibe-owner-debate` `debate_synthesis` submitted `submit_review_outcome(done)`; `advanceHubDoneThroughEdge` resolved the `debate_synthesis --done--> done` edge to the terminal pseudo-node and returned `(_, false)` at `interactive_service.go:~6443` **without** calling `onVibeCpNodeDone` → `restoreVibeFlowAfterDebate` (`vibe_cp.go:666`) unreachable. Sprint stayed parked `vibeTaskIndex 1/3`, audit never ran, run stuck `status:"running"` with terminal loop `done`; `flow-control continue` → `flow_control_rejected_terminal`. No revive path exists.
- Symptom B (CP58-2, run-1 task-harness devin / run-6210 cp-harness codex): operator `POST /flow-control {"status":"done"}` on a parked hub settles the **whole** run — every downstream node flips `SKIPPED` — skipping the hub's `done` successor (`plan_synthesis --done--> preflight_contract_freeze`, `cp_synthesis --done--> task_splitter`). Inconsistent with both sibling verdict paths, which route through `advanceHubDoneThroughEdge`.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom A — debate `done` verdict kills the parent vibe flow:** vibe run-21751 with a 3-task plan. Sprint 1 `tdd` child (run-27148) ended gated → `stashVibeFlowForDebate` parked the sprint graph → debate resolved → hub submitted `submit_review_outcome(done)` → tool result `{"status":"done","round":5,"cap":7,"nextAction":"done"}` → flow-diag shows `flow_control_received` → `flow_run_complete_begin` → `flow_run_complete_done` → `flow_control_done` (22:08:02Z) with **no restore/chain/boundary events**. Loop landed `done` with `vibeTaskIndex:1 vibeTaskTotal:3`; Task-912/913 never dispatched; run left `status:"running"` while loop terminal (limbo).
- **Symptom B — operator `done`/`approved` via `/flow-control` bypasses the hub edge:** `plan_synthesis` parked `WAITING_USER_APPROVAL` on run-1; operator verdict `{"status":"done"}` flipped it `DONE` and every downstream node went `SKIPPED` (`preflight_contract_freeze`, `test_signatures`, `implement`, `validate`, `reviewer`, `synthesis`, `synthesis_negotiation`, `audit`) — run `completed`. Same shape reproduced on cp-harness (run-6210).
- **Expected:** (A) `debate_synthesis --done-->` should return control to the parked sprint flow (`restoreVibeFlowAfterDebate` → sprint resumes toward `audit` → `maybeChainVibeSprint` starts sprint 2). (B) An operator `done` on a parked hub should traverse the hub's `done` edge like the tool-bridge and prose paths do.
- **Actual:** both paths terminate the whole run; the parked topology is abandoned mid-flight and the run reports completion with contracted work undelivered.
- **Impact:** silent false-completion of multi-task vibe sprints (2/3 tasks dropped, no audit, no handoff) and operator approvals that silently abandon the remainder of any harness flow — data-integrity + trust critical. No revive path: `flow-control continue` → `flow_control_rejected_terminal`; `POST /resume` reopens chat only; `maybeReparkVibeSprintBoundary` early-returns because `audit` never reached DONE (`vibe_sprint_boundary.go:259`); `agent-loop/continue` is a no-op on a non-blocked loop.

## Reproduction

- Symptom A: run a `workingMode:"vibe"` ingest→sprint flow with a ≥2-task plan; let the sprint-1 child end gated so `vibe-owner-debate` stashes the sprint; let the debate resolve `done`. Observe `flow_run_complete_done` with `vibeTaskIndex < vibeTaskTotal` and run stuck `running`.
- Symptom B: run `task-harness` under a provider whose verdict tool is rejected (e.g. devin CP48-2) until `plan_synthesis` parks `WAITING_USER_APPROVAL`; `POST /client/workflow-runs/{runId}/flow-control -d '{"status":"done"}'` → all downstream nodes `SKIPPED`, run `completed`.

## Root cause

- `advanceHubDoneThroughEdge` (`apps/local-runner/internal/runner/interactive_service.go:6358`): when the resolved `done` edge target is the terminal pseudo-node (`""`/`"done"`/`"ask_user"`), it clears `activeHubNodeID` and returns `FlowControlResult{}, false` at ~L6443. `onVibeCpNodeDone` — the only caller of `restoreVibeFlowAfterDebate` (`vibe_cp.go:666`) besides the slicer — is invoked only in the slicer special-case at L6432–6434, so a `debate_synthesis` `done` verdict can never restore the stashed sprint. `applyFlowControl`'s `done` case then runs `markFlowRunComplete` (`interactive_service.go:1646`) + `mutateLoop(done)` — terminal, unrecoverable.
- Operator/HTTP path: `handleSubmitFlowControl` (`interactive_handlers.go:1627`, dispatch ~L1709) routes straight to `applyFlowControl` — it is the only verdict face that bypasses `advanceHubDoneThroughEdge`. The tool-bridge path (`interactive_service.go:6358`) and prose path (`review_done_verdict.go:175`, comment at 169–177 explicitly forbids settling the whole flow because it "skips the freeze node") both route via the edge.

## Evidence

- `~/fp-beds/lt-evidence/cp60/run-21751-flow-diag.ndjson` — `flow_control_received` → `flow_run_complete_begin` → `flow_run_complete_done` → `flow_control_done` 22:08:02Z, no restore/chain/boundary events; `flow_control_rejected_terminal` 05:19:41.
- `~/fp-beds/lt-evidence/cp60/run-21751/agent-graph-final.json` — `loopState.status:"done"`, `vibeTaskIndex:1`, `vibeTaskTotal:3`; run-27148 orphaned `waiting_user_approval`.
- `~/fp-beds/lt-evidence/cp60/run-21751-step-transitions.ndjson`, `run-21751/steps-runtime-final.json` — only debate nodes present; sprint topology still parked.
- `~/fp-beds/lt-evidence/cp60/l60-2-revive-probes.txt` — every revive attempt rejected.
- `~/fp-beds/lt-evidence/cp58/l581-devin/run-1-step-transitions.ndjson` — `plan_synthesis` WAITING_USER_APPROVAL→DONE (20:26:08→20:27:15), then all downstream SKIPPED.
- `~/fp-beds/lt-evidence/cp58/l583-cp/codex-steps.json` — same skip-all shape on cp-harness.
- `~/fp-beds/lt-evidence/cp58/BUG-LIVE-2-operator-done-bypasses-hub-edge.md`, `~/fp-beds/lt-evidence/cp60/RESULT.md` (BUG-LIVE-1).

## Severity

`critical` — deterministic false-completion path on the primary vibe sprint pipeline and on every harness hub, with no recovery surface.

## Completion Notes (implemented 2026-09-23, CA-921)

- Root cause: a `done` flow-control verdict on the debate-synthesis terminal edge fell through to whole-run settle while sprint topology was stashed in `vibeParked*` fields — the parked sprint was silently dropped.
- Fix: `advanceHubDoneThroughEdge` terminal-edge branch restores the stashed `vibeParkedNodes`/`Edges`/`Acceptance`/`FlowRef` before settling; HTTP `done` now routes through the same edge path.
- Files: `internal/runner/interactive_service.go`, `interactive_handlers.go`.
- Tests: `TestBug401_DebateSynthesisDoneRestoresParkedSprint`, `TestBug401_NoStashedSprint_FallsThrough`. Baseline-red verified.
