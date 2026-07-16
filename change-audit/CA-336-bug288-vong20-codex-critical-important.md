# CA-336: BUG-288 Vòng 20 Codex Critical/Important (4 issues)

## Scope

Close Round 19 residual: durable prep→launch-ack not atomic with provider launch (orphan bare short-circuit + fail-open launch after ack persist fail); feature-history inject still used global FCP verify; gate_settle_checkpoint cleanup fourth persist fail-open left false blocked on disk; missing regression windows.

## Changes

- `durableIdemReplaySafe` / `durableIntentClearOK`: durable short-circuit and outer intent clear only when turn is live in-process, lastTurnID matches, gate settle owns turn, or terminal events exist — not bare launch-ack alone.
- Launch-ack persist fail-closed: revert to prep, abort, do not `go runTurn`.
- `startTurnClearingIntent` / `deliverPendingRestart` clear intent only when clear-safe.
- `injectFeatureHistoryPromptWithSecret` + `runTurn` uses `s.markerSecret`.
- Settle checkpoint: never write transient `gate_settle_checkpoint` to durable snap; three settle tries; RAM blocked only if all fail.
- Tests: `bug288_round20_test.go`; R16 reconstruct test requires terminal evidence for bare replay.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 20 — recovery-owned durable launch, feature-history per-service secret, settle no false blocked marker
# --->8---
