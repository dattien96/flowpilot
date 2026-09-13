# CA-858 — CP-62 P-3 completion: decision card UI for desktop app and TUI

# ---8<--- flowpilot:change-ledger
feature_key: decision-card-ui
source_doc_id: Task-345
change_type: task
summary: render the runner's user_decision_card_requested event as an interactive escalation card — desktop app gets a DecisionCard component (options as one-tap buttons with consequences, recommended highlight, evidence citations) plus a new timeline item; TUI arms a decision-card state with a numbered-options message and input handling; both answer through the parked-run feedback channel carrying the option id (runner Task-346 matches it back); unmatched prose still goes through verbatim as the Q-1 fallback; admin-web deliberately untouched per Operator decision
# --->8---

## Why

Task-339 shipped the runner-side card payload but no client rendered it (the task explicitly deferred renderer work). The Operator lifted that constraint for the desktop app and TUI on 2026-09-13 — explicitly NOT admin-web.

## Change

- Desktop (`apps/desktop-flowpilot`): `types/contract.ts` — `DecisionCardDTO`/`DecisionCardOptionDTO`/`DecisionCardEvidenceDTO` + `user_decision_card_requested` event member (payload rides the runner's `input` field); `state/timelineReducer.ts` — `decision_card` timeline item, status settles to `waiting_question` (composer stays usable for the Q-1 prose fallback); `components/DecisionCard.tsx` (new) — options as buttons, consequence under each label, recommended highlighted, evidence rows, answered state disables; `state/store.ts` — `chooseDecisionOption(itemId, optionId)` marks the specific card answered and sends the option id via `sendPrompt`; `Timeline.tsx` + `styles.css` — render case and card styles.
- TUI (`internal/tui`): `client/client.go` — `DecisionCardData`/`DecisionCardOption`/`DecisionCardEvidence` mirroring the runner schema, decoded from the event `input` field; `app/model.go` — `DecisionCardState` + `m.decisionCard`; `app/app.go` — arms the card on the event (numbered options message, recommended marker, evidence, statusMsg "decision"), `handleDecisionCardInput` (option number/id/label submits the option id via `cmdContinueFlowWithFeedback` — the parked-run feedback channel; other text falls through verbatim), headless mode prints the card non-fatally.

## Tests

- Desktop: `state/timelineReducer.test.ts` — card event → timeline item + `waiting_question` status; full reducer suite 32/32 via esbuild-bundled node:test (the repo's phase1 pipeline is blocked by two PRE-EXISTING compile errors, see below). `npm run typecheck` clean except the pre-existing `store.chat-mode-persist.test.ts(82,58)` error (reproduces with my changes stashed).
- TUI: `app/decision_card_tui_test.go` (3, additive) — event arms card + renders question/options/recommended/evidence; number input submits option id through POST /agent-loop/continue with the id as feedback (httptest-captured); prose fallback sends verbatim and disarms. Full `internal/tui/...` suite green except 6 spinner/timing tests that fail identically on base f6634215 (pre-existing machine-dependent).

## Providers

Case 1 provider-agnostic — clients render the runner event payload; no adapter involvement.

## Prior claims intact

Runner-side card schema/emission (Task-339) untouched; Q-1 prose fallback preserved on every surface (desktop composer stays usable, TUI unmatched text forwards, malformed payload keeps prose card); admin-web untouched; gate/approval/question card paths untouched (decision card is a separate state with lower routing precedence than approval/question/gate).
