# CA-851 — CP-62 P-3: structured escalation cards + schema'd or-explained for r-dod-complete

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-339
change_type: task
summary: request_user_decision tool face + UserDecisionCard parser/emit — escalate flow_control with payload.decision_card emits a user_decision_card_requested event (malformed → log + drop, prose card stays = Q-1 wrap-around); TurnResult.DodExplanation becomes the primary or-explained channel for r-dod-complete (legacy phrase/section text-match kept as backward-compatible fallback)
# --->8---

## Why

Escalate/ask_user reached the user as GateReason prose a non-tech vibe user cannot decide from, and r-dod-complete's or-explained text-matched free text (a phrasing miss = false block). CP-62 P-3 makes both machine-decidable (AskUserQuestion pattern: options + consequences + recommended).

## Change

- `flow-pack/tools/request-user-decision.yaml` (new): tool face — question, ≥1 option {id,label,consequence}, recommended (must reference an option), evidence[{path,line,excerpt}], detail escape hatch.
- `runner/user_decision_card.go` (new): `UserDecisionCard` + strict `parseUserDecisionCard` + `EventUserDecisionCardRequested` + `emitUserDecisionCard` (stamps `interactiveRun.decisionCard`).
- `runner/interactive_service.go`: `applyFlowControl` escalate branch parses `payload["decision_card"]`, emits the structured card; prose path untouched (fallback guaranteed).
- `flowgate/rules.go`: `TurnResult.DodExplanation *DodExplanation`.
- `flowgate/evaluate.go`: `hasValidDodExplanation` — structured field wins; phrase/section heuristics unchanged below it (backward compat).

## Tests

`flowgate/r_dod_structured_test.go` (4: structured pass→warn, blank explanation→block, legacy text-match compat, structured-beats-silent-prose); `runner/decision_card_test.go` (structured card emitted with options/recommended, malformed matrix (no-options/recommended-unknown/missing-consequence) → parser error, prose park attaches no card). Full flowgate suite + agentpack suite green; targeted runner regression (vibe/gate/verdict/card/pack patterns) green.

## Providers

Case 1 provider-agnostic — cards originate in the runner (`applyFlowControl` + parser) and pack data; no adapter logic involved.

## Prior claims intact

CA-835/CA-845/CA-846/CA-848 (r-dod parser hardening line) — `ParseDefinitionOfDone` and the legacy `hasValidDodExplanation` heuristics are untouched, only extended; BUG-365 park-emits-graph-event semantics preserved; BUG-231 escalate≠done settle states untouched.
