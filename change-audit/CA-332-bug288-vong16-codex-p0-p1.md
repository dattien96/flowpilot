# CA-332: BUG-288 Vòng 16 Codex P0/P1 (durable idempotency, gate mutex write, per-dir secret, settle third persist)

## Scope

Close four Codex re-review findings on Vòng 15: RAM-only startTurn idempotency (duplicate stall-retry after crash), gate epoch TOCTOU on durable writes, multi-store marker secret Once trap, and non-durable gate_settle_checkpoint diagnostic.

## Changes

- Persist `IdempotencyKeys` (durable-* only) on ProviderSessionState / NDJSON / reconstruct; durable-key persist retry in startTurn; Supabase `pending_restart_gen` + `idempotency_keys` migration + upsert/get.
- `withGateEpochDurable` holds s.mu across ClearOverrideIfGreen / commitChangeContract (Stop cannot interleave).
- Marker secrets keyed by abs(dataDir); no Once; verify all loaded secrets.
- markPendingFlowGateSettle: after two fail, stamp blocked then third persist with new snap.
- Tests: `bug288_round16_test.go`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 16 — durable idempotency keys+Supabase gen, gate durable under s.mu, per-dir marker secrets, settle blocked third-persist
# --->8---
