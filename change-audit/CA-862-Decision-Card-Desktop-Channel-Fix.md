# CA-862 — Task-350: desktop card answers via agent-loop/continue + cross-sprint state resets

# ---8<--- flowpilot:change-ledger
feature_key: decision-card-ui
source_doc_id: Task-350
change_type: task
summary: fix the CP-62 review P1s — desktop chooseDecisionOption routed the option id through POST /turns which the runner seals with 409 flow_awaiting_user on card-parked runs (choice never captured, optimistic "answered" without rollback); it now calls the store's continueFlow (POST /agent-loop/continue, the same channel as TUI/FlowAwaitingUserCard) with BUG-172-style rollback; takeNextVibeSprintLocked resets cross-sprint verified state (lastFlowVerdicts, lastTamperedTestPaths, decisionCard+chosen, expectedACs cache) fixing the stale AC-cache P1; a NEW decision card clears a previous card's stale choice; composer/Q-1 comments and docs corrected to the real behavior
# --->8---

## Why
Review P1 (Task-345/346): the desktop answer channel was broken end-to-end, and verified-state (verdict rows / tampered tests / card choice / governing-doc AC set) leaked across vibe sprints, misattributing data in handoffs and enforcing the wrong sprint's ACs.

## Change
- `apps/desktop-flowpilot/src/state/store.ts`: chooseDecisionOption → continueFlow(optionId) with optimistic answered + rollback on error; comments corrected (composer stays blocked; Q-1 = FlowAwaitingUserCard feedback box).
- `timelineReducer.ts` + `DecisionCard.tsx` comments corrected.
- `interactive_service.go`: new card clears stale decisionCardChosen.
- `vibe_cp.go` takeNextVibeSprintLocked: cross-sprint resets (also fixes Task-344's run-lifetime AC cache).

## Tests
`sprint_handoff_enrichment_test.go`: TestHandoffEnrichment_SprintAdvanceResetsVerifiedState. Desktop typecheck: only the pre-existing error remains; reducer suite 32/32.

## Providers
Provider-agnostic — desktop client + runner run-state only.

## Prior claims intact
TUI channel unchanged (already correct); captureDecisionChoice contract unchanged; Task-342 handoff schema unchanged.
