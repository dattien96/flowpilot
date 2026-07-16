# CA-334: BUG-288 Vòng 18 Codex Critical/Important (7 issues)

## Scope

Close Round 17 residual: durable idempotency key prune, settle wipe on checkpoint fail, Supabase incomplete recovery, global marker mint, override/head fail-open, ghost turn/step on durable persist fail.

## Changes

- durableIdempotencySnapshot: numeric gen ranking + protectKeys pin; zero-pad restart/reprompt gen in keys.
- runTurn else: do not clear pendingFlowGateSettle when gateCheckpointNotDurable.
- Supabase: session_runtime jsonb (migration 20260716140000) packs/unpacks full recovery state.
- InteractiveService.markerSecret + Compose/append *WithSecret for per-service mint.
- ClearOverrideIfGreen failure blocks gate; updateCanonicalHead returns err to commitChangeContract.
- startTurn: durable-* persist before markStepRunning/EventTurnStarted.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 18 — numeric idempotency protect, keep settle on checkpoint fail, Supabase session_runtime, per-service marker mint, override/head fail-closed, no ghost turn
# --->8---
