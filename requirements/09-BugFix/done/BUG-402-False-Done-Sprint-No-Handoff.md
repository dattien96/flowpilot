# BUG-402: Vibe sprint settles `done`/`audit:DONE` with no handoff artifact, RED suite, tasks untouched

## Metadata

- Document ID: `BUG-402`
- Title: `False-done vibe sprint — audit DONE without handoff-sprint-*.yaml, suite RED, 2/3 tasks untouched, run reports "The flow completed"`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md), [CP-49-Reverse-Documentation-And-Doc-Ingestion](../../07-Coding-Plan/done/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Feature Keys: `vibe-mode`, `sprint-handoff`, `flow-engine`

## AI Quick View

### Summary

- Two independent vibe runs (run-9, run-2290) settled `done` mid-sprint with **no `handoff-sprint-N.yaml` ever produced** — `requirements/.flowpilot/vibe/handoffs/` was never created. run-9 settled with `coder:SKIPPED`, `validate:SKIPPED`, `synthesis:DONE`, `audit:DONE`; run-2290 settled at `vibeTaskIndex 1/3` then surfaced question q-8185 "The flow completed. What should I continue with?" while the suite was RED and 2 of 3 sprint tasks untouched.
- Completion is reported to the user while contracted work is undelivered and the hard-ceiling handoff contract (audit emits `handoff-sprint-*.yaml` for the next sprint) is violated. Related wedge: the `submit_review_outcome` verdict that should have driven recovery was lost — captured as BUG-403.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom A (run-9):** tdd child run-1588 parked on requirement-drift; `gate-decision keep-test-fix-code` was accepted but the child could not be re-driven while the parent parked (`flow_awaiting_user`); the hub turn had been cancelled mid-`submit_review_outcome`. Run settled `done`: `coder:SKIPPED`, `validate:SKIPPED`, `synthesis:DONE`, `audit:DONE` — **audit DONE with no `handoff-sprint-*.yaml` written**.
- **Symptom B (run-2290):** after the tdd owner-debate rounds the loop settled `done` at `vibeTaskIndex 1/3`; the run then surfaced q-8185 "The flow completed. What should I continue with?" — completion reported while the suite was RED and tasks 2–3 never ran. After `agent-loop/resume` + answer, loop went `blocked` (`hub_stalled`).
- **Expected:** a vibe sprint may only reach `done`/`audit:DONE` after every planned task dispatched and the audit node wrote `handoff-sprint-*.yaml` (CP-49 hard ceiling); if the flow terminates early the run must surface an incomplete/blocked state, never "The flow completed".
- **Actual:** terminal `done` is reachable with tasks undispatched and no handoff artifact; the user-facing message claims completion.
- **Impact:** false-done on the flagship vibe pipeline — undelivered work presented as finished; downstream sprints lose their contractual high-priority input (the handoff file). The handoff writer + enrichment are unit-verified (`TestSprintHandoff_*` green), so the live gap is purely orchestration: the sprint never reaches a state where audit emits the artifact.

## Reproduction

- Drive a multi-task vibe sprint where the tdd child parks (requirement-drift gate or owner-debate). While the parent is parked, allow the hub/debate path to settle — e.g. cancel the hub turn mid-`submit_review_outcome` (run-9 shape) or let the debate resolve `done` (run-2290 shape, same settle family as BUG-401).
- Observe `steps-final` showing `audit:DONE` + downstream `SKIPPED` nodes and confirm `requirements/.flowpilot/vibe/handoffs/` absent; run surfaces "The flow completed".

## Root cause

- Same settle-family defect as BUG-401: a hub-level `done`/`submit_review_outcome(done)` resolves through `applyFlowControl` → `markFlowRunComplete` (`apps/local-runner/internal/runner/interactive_service.go:1646`) instead of restoring the parked sprint via `onVibeCpNodeDone`/`restoreVibeFlowAfterDebate` (`vibe_cp.go:666`), so remaining tasks and the real audit never run.
- Compounding: the `audit` node can be marked `DONE` without verifying its hard-ceiling output (`handoff-sprint-*.yaml`) exists — there is no terminal-state invariant check that blocks `done` when sprint tasks remain PENDING/SKIPPED or the handoff artifact is absent.
- run-9 aggravator: `keep-test-fix-code` accepted while parent parked `flow_awaiting_user` left the gated child undrivable (dead-option family, cf. BUG-411).

## Evidence

- `~/fp-beds/lt-evidence/cp49/L49-2-run9-steps-final.json` — `coder:SKIPPED`, `validate:SKIPPED`, `synthesis:DONE`, `audit:DONE`.
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-step-transitions.ndjson` — ends at `debate_synthesis DONE`; no `tdd`/`coder`/`validate`/`audit` completion for tasks 2–3.
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-agentgraph-final.json` — `loopState.status=blocked`, `blockReason=hub_stalled`, `vibeTaskIndex 1/3`.
- `~/fp-beds/lt-evidence/cp49/L49-2-sprint-plan.md`, `L49-2-tdd-signatures.md` — 3-task plan + TDD contract the run abandoned.
- Absence of `requirements/.flowpilot/vibe/handoffs/` in bed `lt-cp49`; `~/fp-beds/lt-evidence/cp49/L49-2-run2290-bed-status.txt`, `runner.log`.
- `~/fp-beds/lt-evidence/cp49/RESULT.md` (BUG-LIVE-2).

## Severity

`critical` — user-visible false completion on contracted multi-task work; audit artifact contract violated; both repros deterministic once the wedge state is reached.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause: nothing stopped an agent `done` verdict while `vibeTaskPlan - vibeSprintIndex > 0` — run-9/run-2290 published done mid-sprint with no handoff-sprint-*.yaml.
- Fix: new `FlowControlInput.agentInitiated` set inside `turnBridge.SubmitFlowControl` (the single agent funnel); `applyFlowControl`'s done case refuses (unstamp one-decision + escalate) only for agent-initiated verdicts — operator settles (boundary cancel/decline, HTTP flow-control) remain unflagged so the human decision cannot wedge.
- Files: `internal/runner/agent_orchestrator.go`, `interactive_service.go`.
- Tests: `TestBug402_DoneRefusedWhileSprintTasksRemain` (agent refused+escalated, operator settle publishes done). Baseline-red verified.
