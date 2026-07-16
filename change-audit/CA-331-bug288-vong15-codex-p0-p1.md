# CA-331: BUG-288 Vòng 15 Codex P0/P1 (stall-retry delivery, gate epoch, marker once, settle checkpoint)

## Scope

Close four Codex findings after Vòng 14: durable stall-retry delivery acknowledgement, gate epoch TOCTOU before durable side effects, multi-service marker secret clobber, and PendingFlowGateSettle persist fail-closed.

## Changes

- Stall-retry: `PendingRestartGen` on session + NDJSON; `deliverPendingRestart` with claim + idempotency key; wire resume/flush/runTurn tail.
- Gate: epoch recheck before/after `ClearOverrideIfGreen` and `commitChangeContract` (root + child).
- Marker: `sync.Once` on durable secret init (first service wins process-wide).
- Settle checkpoint: retry persist; stamp `gate_settle_checkpoint` blocked intent on hard fail; keep RAM settle.
- Tests: `bug288_round15_test.go`; `alwaysFailingUpsertStore.UpsertProviderSession` for checkpoint path.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 15 — durable stall-retry delivery+idempotency, gate epoch recheck before side effects, marker secret Once, settle checkpoint fail-closed
# --->8---
