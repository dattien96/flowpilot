# CA-335: BUG-288 Vòng 19 Codex Critical/Important (6 issues)

## Scope

Close Round 18 residual: Stop can resurrect durable start after pre-persist unlock; crash between prep-persist and provider launch clears restart/reprompt via bare idempotency replay; warn/approve gate bypasses epoch + fail-closed contract; marker mint still global on behavior/inline/retry paths; `gate_settle_checkpoint` blocked marker left after third persist success; R18 tests only covered helpers.

## Changes

- `startTurn`: capture start token before durable unlock; `abortDurableStartIfStaleLocked` revalidates turnInFlight/currentTurnID/Cancelled/loop before markStepRunning/TurnStarted.
- Durable idempotency two-phase: `prep:<turnID>` pre-persist → bare turnID launch-ack after side effects; only launched short-circuits; prepared reuses turnID and relaunches (skips re-increment turnCount).
- Gate warn/approve path: `withGateEpochDurable(... commitChangeContract ...)` fail-closed (block on false/error).
- Thread `s.markerSecret` through `behaviorContextRender`, inline `renderFlowContextPromptWithSecret`, `ComposeRetryPromptWithSecret`; inject skip uses `isFlowContextHandoffWithSecret(s.markerSecret, ...)`.
- `markPendingFlowGateSettleLocked`: after third persist success clear `intentBlockedKind` and persist clean snapshot.
- Regression: `bug288_round19_test.go`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 19 — durable start Stop revalidate, prep→launch idempotency, warn epoch commit, markerSecret paths, clear settle blocked marker
# --->8---
