# CA-357 — Awaiting-user form freezes hub + children (run-1675)

## Context

Live A1 residual after CA-355/356: `run-1675` coder failed (no Claude account) →
hub reinvoked and **escalated** (Continue form correct) → but main kept running
("24 tool calls", creating BugFix doc) while **Needs your decision** was open.

Evidence: hub `turn_count=3`; third prompt was gate reprompt for missing BugFix
doc; loop already `blocked` from escalate.

## Product answers

1. **Should this case show a form?** **Yes** for provider/account failure —
   user must choose Connect Claude / re-run with Grok / Stop. Auto hub coding
   alone without a decision surface is worse UX for intentional Review Loop.
2. **May hub auto-code after sub fail?** Only **after** user Continue (or if we
   never escalate). **Not** while the form is visible.
3. **Invariant:** decision surface open ⇒ flow pending: **no new turns**,
   **cancel in-flight** hub/child work.

## Root cause

- `startTurn` rejected `stopped`/`done` but **not** `blocked`.
- Escalate did not cancel live `turnCancel` or clear `pendingGateReprompt*`.
- Post-turn gate / `flushDurableTurnIntents` could still start a reprompt turn
  behind the form.

## Changes

- `parkFlowForAwaitingUser` — cancel hub+child turns; drop reprompt/reinvoke/resume/settle.
- Wire park on **escalate**, **cap awaiting_user**, **hub_stalled**.
- `startTurn` reject `flow_awaiting_user` when loop `blocked` (parent + children).
- `flushDurableTurnIntents` skip when loop blocked/stopped/done.
- `runTurn` skip post-turn gate when loop already blocked.

## Tests

`run1675_awaiting_user_freeze_test.go`:

- `TestStartTurnRejectsWhenLoopBlocked`
- `TestParkFlowForAwaitingUserCancelsTurnAndDropsReprompt`
- `TestEscalateParksFlowAndBlocksStartTurn`
- `TestFlushDurableTurnIntentsSkippedWhenBlocked`

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 3m \
  -run 'TestStartTurnRejectsWhenLoopBlocked|TestParkFlow|TestEscalateParks|TestFlushDurable|TestBug289_F0|TestNonCohort|TestResumeFlowWithFeedbackClearsStale'
```

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Freeze flow while escalate/Continue form is open; block startTurn and drop gate reprompt behind card
# --->8---
