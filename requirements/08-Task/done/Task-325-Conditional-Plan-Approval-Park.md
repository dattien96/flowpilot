# Task-325: Conditional Plan-Approval Park (Human Gate After Contested Plans)

## Metadata

- Document ID: `Task-325`
- Title: `Conditional Plan-Approval Park (Human Gate After Contested Plans)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-06`
- Last Updated: `2026-09-06`
- Parent Documents: [CP-58: Bug / Task / CP Harness](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `none`
- Related Documents: [Task-324](../../08-Task/done/Task-324-Bug-Plan-Harness-With-Bug-Doc-And-Picker-Dedup.md)
- Replaces: `none`
- Tags: `plan-approval, human-gate, park, plan-loop`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Operator distrusts AI plan review (2026-09-06): a plan the AI reviewer approves may still be wrong. Decided: conditional human gate (option B) — park for user read + approve ONLY when the plan loop already churned (reviewer rejected ≥1) or risk signals fire; clean first-pass plans run straight through (unattended runs keep working).
- Reuses proven machinery: `parkFlowForAwaitingUser` + `WAITING_USER_APPROVAL` + Continue/Stop chips + the existing `plan_synthesis → plan_writer` continue back-edge for feedback re-entry (same-session re-enter pattern from CP-58 live runs). No new edge, no new node type expected.

### Current Ask

- Spec approved; NOT yet implemented. Implementer must verify the churn signal + park durability before coding.

### Key Decisions

- D-1: Conditional (B), not hard STOP (A) — hard STOP kills unattended runs and breeds blind-approve fatigue; the strongest "review may be wrong" signal is prior churn.
- D-2: Park point is after `plan_synthesis` done, before `preflight_contract_freeze` — cheapest place to stop (nothing frozen, no code spent).
- D-3: Three outcomes — Approve (=continue → freeze proceeds), Feedback text (=changes_requested → re-enter `plan_writer` same session with the note, mirroring reviewer feedback), Stop (existing).
- D-4: Scope is the two 12-node plan flows (`task-harness`, `bug-plan-harness`). `cp-harness` CP loop is a follow-up question, not this task.

### Constraints

- Additive tests only; no pre-existing orchestrator/gate test edits.
- Must not break unattended runs (clean plans never park), cap/extend accounting, or restart recovery expectations.
- Both clients (TUI + Desktop) must surface the park with the same three outcomes.

### Open Questions

- Q-1 (RESOLVED by research 2026-09-06): churn signal is `plan_writer` child `activationSeq >= 1` (zero-based: first activation 0, one re-entry → 1; `flow_executor.go:1449`). NOT `>= 2`. `LoopState.Round` is corroboration only (shared by both loops, not plan-specific). Never `gateEpoch`.
- Q-2: risk signals beyond churn (DeclaredPaths count threshold? core-path touch?) — v1 may ship churn-only.
- Q-3: park durability across runner restart — verify `parkFlowForAwaitingUser` state rehydrates (TUI re-hydrates WAITING on open per run-127174, but restart path needs proof); if not durable, scope v1 to in-session parks and document it.
- Q-4: human feedback entry while parked — confirm the "[flow-engine] Feedback received" path accepts human composer text mid-park (as opposed to agent-only feedback).

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml:103-108,169-178` (plan_synthesis node + loop/freeze edges)
- `apps/local-runner/internal/agentpack/flow-pack/flows/bug-plan-harness.yaml` (same shape)
- Runner: `parkFlowForAwaitingUser`, `WAITING_USER_APPROVAL`, `applyFlowControl`, gateEpoch/Continue/Stop chips (existing infra, reuse)

## 1. Goal

A contested plan (one the AI reviewer already bounced) cannot proceed to freeze/code without a human reading it; uncontested plans flow through untouched.

## 2. Parent Links

- CP-58 review-loop re-entry evidence (doc-writer ×1 same session, feedback prompt) — the UX pattern to mirror.
- Task-324 (the two plan flows this gate applies to).

## 3. Trigger

- Operator 2026-09-06: "AI review sai thì sao" — no human eyes exist between plan approval and code spend.

## 4. Exact Change

- `T-1` Churn detection at `plan_synthesis`-done: park iff `plan_writer.activationSeq >= 1` (re-entered ≥1) — before `preflight_contract_freeze` fires. Insertion in `advanceHubDoneThroughEdge` after edge resolution + hub.notify exclusion, before `advanceToNextInlineOrDelegate` (covers tool-DONE and prose-DONE; freezing inside `runContractFreezeNode` would be too late).
- `T-2` Park via existing `parkFlowForAwaitingUser` with reason `plan_approval`; banner shows the plan doc path + what churned (round count, reviewer findings summary).
- `T-3` Outcomes via `POST agent-loop/continue` (composer is sealed while parked — human text enters ONLY as `feedback`): Approve (empty feedback) → advance forward to freeze; human feedback → `changes_requested` → re-enter `plan_writer` same session with note injected as plan feedback (explicit `prevBlockReason == "plan_approval"` branch in `resumeFlowWithFeedback`; stock resume would retry the writer); Stop → existing stop path.
- `T-4` Both clients surface all three outcomes (reuse Continue/Stop chips; feedback via composer while parked — verify Q-4).
- `T-5` Tests (new files): churn triggers park / clean plan passes through / feedback re-enters writer / approve proceeds to freeze. Restart-durability test only if Q-3 proves durable (else document the limitation).

## 5. Touched Areas

- `apps/local-runner/internal/runner/` — advance/reinvoke path around plan_synthesis completion (TBD by implementer)
- `apps/local-runner/internal/tui/app/` + Desktop — park banner/chips if existing ones don't cover the reason
- New test files only

## 6. Acceptance Check

- Live: run task-harness, force reviewer reject once → flow parks with plan path shown → type feedback → writer re-enters same session → approve second plan → freeze/code proceed → done.
- Live negative: reviewer approves first pass → zero park, run completes unattended.
- Unit: `go test ./internal/runner/ -run 'TestPlanApprovalPark' -count=1` PASS (names per implementation).

## 7. Out of Scope

- Hard STOP for all plans (rejected — option A).
- `bug-harness` (no plan), `cp-harness` CP loop (follow-up).
- New node types / new edges (reuse only).

## 8. Completion Notes

- Implemented 2026-09-06 (CA-749) per safe-fix-contract: history-first (agent-flow-engine CAs 739-748 read; will-not-undo list honored), provider-agnostic classification with 3-provider matrix evidence.
- Code (`runner/plan_approval_park.go` new + 2 hooks in `interactive_service.go`): `planLoopChurned` (writer `activationSeq >= 1`), park hook in `advanceHubDoneThroughEdge` scoped to `plan_synthesis→preflight_contract_freeze` (covers tool + prose DONE, freezes never start), `parkPlanForApproval` (WAITING step → blocked/plan_approval + GateReason with writer rounds and BUG-357 plan path → park → emit/persist → one-decision guard stamp), `resumePlanApproval` branch in `resumeFlowWithFeedback` (empty feedback advances forward via direct edge dispatch — never hub re-decide, which would re-park; feedback re-enters writer same session; dispatch failure falls through to generic).
- Tests (9, `task325_plan_approval_park_test.go`, race-clean): specificity unit table, live park + passthrough matrix Claude/Codex/Grok, code-churn-only no-park, missing-writer fail-open, code-hub unaffected, approve→freeze (writer untouched), feedback→writer seq+1 same child + RUNNING, restart keeps park sealed. Full runner suite failure set == clean-tree baseline (18 pre-existing, stash-proven incl. `TestResumeFlowWithFeedbackAfterEscalate` identical message); agentpack/flowgate green; zero old-test edits.
- Residual: approve-after-restart with live children untested (park durability proven; fail-closed escalate covers the gap) → live-verify item. Q-2 risk signals deferred (churn-only v1).
- Follow-up 2026-09-06 (CA-751, live-found on run-577686): T-4 claimed both clients surface all three outcomes, but TUI sent hardcoded `feedback:"continue"` with no way to attach human text (Desktop already had the textarea). Fixed: `/continue <text>` carries feedback via `ContinueFlowWithFeedback`; bare `/continue` and the Retry chip keep the legacy body.
- Follow-up 2026-09-06 (CA-752, live-found on run-584646): approve was unreachable from TUI — chip Retry and bare `/continue` both send legacy `"continue"`, but resume treated only empty feedback as approve. Fixed: `"continue"` (case-insensitive) also counts as approve on plan_approval parks. Park trigger itself live-verified on run-584646 (churned plan → blocked/plan_approval, WAITING step, freeze untouched).
