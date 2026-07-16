# CA-333: BUG-288 Vòng 17 Codex P0/P1 (Stop/gate race, idempotency fail-closed, Supabase recovery, markers, settle, contract)

## Scope

Close seven Codex findings on Vòng 16: Stop vs durable gate write race, durable idempotency not fail-closed + unsafe prune, Supabase null jsonb and missing ListAllProviderSessions, marker mint still global-first, settle continues when storage down, change-contract I/O fail-open.

## Changes

- stopAgentLoop: s.mu + gateEpoch bump + orchestrator.stop under lock first.
- startTurn: durable-* persist fail aborts provider launch; snapshot keeps last 48 sorted durable keys.
- Supabase: idempotency_keys always {}; ListAllProviderSessions + recovery select on lists.
- Marker: full write-lock init; activate dir secret on Init; verify all loaded secrets.
- Settle: gateCheckpointNotDurable; runTurn skips gate until re-persist; mark returns bool.
- commitChangeContract returns error; withGateEpochDurable fails closed on err.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: Vòng 17 — Stop/gate epoch race, durable idempotency fail-closed, Supabase ListAll+empty keys, per-dir mint activate, settle not-durable hard-stop, contract I/O fail-closed
# --->8---
