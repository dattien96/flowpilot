# CA-351: BUG-289 A3 residual — settle boot fail-closed (no default-allow)

## Summary

Review found that BUG-289 `F-7` boot wiring called `SettleDriver.DriveSettle` with only `Store` set. `planNext` then defaulted `allow=true` for every terminal+`settle_pending` record, which could silently finalize without real gate re-eval (`resumePendingFlowGate`). That is the wrong fix for A3 prune pressure.

## Fix

- Replace `drivePendingSettlesOnBoot` with `inventoryPendingSettlesOnBoot` (log + leave phase unchanged).
- `planNext`: require `EvaluateGate` at `settle_pending`/`settle_none` — refuse default-allow.
- `ListAttention`: surface kind `settle_pending` for operator visibility.
- Tests: boot inventory does not finalize; nil EvaluateGate errors; reprompt supersedes.
- Docs: BUG-289 residual risk; Task-251 remains `in_progress` for T-1..T-4; Task-255 still independent/open.

## Explicit non-claims

- Does **not** close Task-251 or Task-255.
- Does **not** unblock 90-day prune by fake-passing gates.
- Does **not** wire `RetrySettleWithBackoff` or real durable settle effects.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-289
change_type: bugfix
summary: Fail-closed A3 settle boot path — refuse nil EvaluateGate default-allow; inventory only until Task-251 T-1
# --->8---
