# CA-749 — Task-325: conditional plan-approval park (human gate after contested plans)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-325
change_type: feature
summary: churned plan loops park for human approve/feedback after plan_synthesis approves and before freeze; clean plans run unattended; approve advances forward, feedback re-enters writer
# --->8---

## Safe-fix header (contract §Fix guide)

- Prior CA read: 739/740/741 (park-cancel nonterminal, Stop wins), 742/743 (gate bounds), 744 (CP-58 closeout), 747/748 (bug-plan-harness, smoke hide).
- Will-not-undo: parkCancelCause/Suppress one-shot + Stop-wins (CA-741); gateCancel 6m / turnInFlight semantics (BUG-354); dual back-edge continue routing (Task-304); freeze-before-code + cap semantics (CP-58); prose-DONE freeze path (CA-736/737); one-decision guard (BUG-353/289).
- R2 classification: engine/orchestrator path, zero adapter branches → provider-agnostic; proven (not claimed) by running the park + passthrough tests over the Claude/Codex/Grok matrix.

## Change (runner only, 1 new file + 2 hooks)

- `runner/plan_approval_park.go` (new): `planLoopChurned` (plan_writer `activationSeq >= 1`; research corrected the spec's `>= 2` — zero-based counter; `LoopState.Round` is shared-loop corroboration only, never the gate); `parkPlanForApproval` (explicit `plan_synthesis` WAITING → `blocked/plan_approval` with writer-rounds + BUG-357 plan path in GateReason → park → emit/persist → one-decision guard stamp); `resumePlanApproval` (empty feedback advances the resolved done-edge directly — hub re-decide would re-park on still-churned plan; feedback composes the human note and re-enters the same writer; dispatch failure falls through to generic resume).
- `interactive_service.go`: park hook in `advanceHubDoneThroughEdge` scoped to (`plan_synthesis`, `preflight_contract_freeze`) after hub.notify exclusion, before dispatch (covers tool-DONE and prose-DONE; freeze can never start first); resume branch in `resumeFlowWithFeedback` after the pendingPrompt early-return, keyed on `prevBlockReason == "plan_approval"`.
- No new edge/node type; both plan flows (`task-harness`, `bug-plan-harness`) share it by topology; code-loop hub (`synthesis`) and plan-less flows cannot trigger it.

## Verification (R3 matrix + R1 + R2)

- 9 new tests (`task325_plan_approval_park_test.go`), race-clean: churn specificity table (writer seq / implement seq / Round / other-parent / nil); live park (blocked/awaiting_user, WAITING step, freeze untouched, no code spawn, guard stamped) × 3 providers; clean passthrough (freeze DONE, writer spawned) × 3 providers; code-churn-only no-park; missing-writer fail-open; code-hub unaffected; approve → freeze DONE + writer untouched; feedback → same child seq+1 + RUNNING + no freeze; restart keeps `blocked/plan_approval` + sealed `startTurn`.
- R1: full runner suite failure set (18) == clean-tree baseline, each stash-proven (incl. `TestResumeFlowWithFeedbackAfterEscalate` identical message); zero old-test edits; gofmt clean on new lines (interactive_service.go flag is pre-existing churn at line 154).
- agentpack + flowgate suites green. `go vet` clean.
- Residual (documented, not blocking): approve-after-restart with live children is live-verify (park durability proven; fail-closed escalate covers the gap); Q-2 risk signals deferred to churn-only v1.
