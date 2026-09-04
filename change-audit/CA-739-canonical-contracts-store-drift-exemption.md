# CA-739 — frozen-scope drift ignores runner canonical/contracts stores

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: the frozen-scope drift gate now exempts the runner-owned Canonical Head file (.flowpilot/canonical/<feature_key>.json) and the legacy contracts.ndjson store via exact-path helpers, so a frozen writer's gate pass no longer false-blocks on the gate's own observable bookkeeping; flow-rules.json, forged siblings, and real out-of-scope code still drift
# --->8---

## Problem

- Live task-harness (`test_signatures` turn): the frozen-scope drift gate parked the flow with `flow scope drift: wrote outside the frozen contract's declared paths: .flowpilot/canonical/calc-core.json, .flowpilot/contracts/contracts.ndjson`.
- Both files are runner-owned stores the gate's own diff observes: `SaveHead` writes the Canonical Head on gate passes, `commitChangeContract` writes the legacy Store for non-frozen writers. Neither is the current frozen writer's drift — but neither was in the exact-path exemption list (CA-427 Finding 2 discipline).

## Changes

- `apps/local-runner/internal/changecontract/frozen_scope.go`:
  - `IsCanonicalHeadStorePath(p)`: exactly one level under `.flowpilot/canonical/` with a `.json` suffix. Nested paths, non-`.json`, and every other `.flowpilot/**` path stay enforced.
  - `LegacyContractsStorePath()` / `IsLegacyContractsStorePath(p)`: exactly `.flowpilot/contracts/contracts.ndjson` — a `forged.ndjson` sibling still drifts.
- `apps/local-runner/internal/runner/gate_hook.go`: the frozen-scope filter exempts both new helpers alongside the existing FrozenStore / pending-canonical / ledger / CA-note / scaffold exemptions.

## Tests added (new file only)

- `canonical_contract_store_drift_exemption_test.go`:
  - `TestCanonicalAndContractsStoreWritesDoNotTriggerScopeDrift` (Claude/Codex/Grok): declared code + both store files → not blocked.
  - `TestCanonicalStoreWritesPlusTrueDriftStillBlocks` (matrix): + `src/extra.go` → blocked, reason names it.
  - `TestFlowRulesRewriteStillBlocksDespiteStoreExemption`: `flow-rules.json` still drifts (no `.flowpilot/**`-wide hole).
  - `TestIsCanonicalHeadStorePath` / `TestIsLegacyContractsStorePath`: accept/reject unit tables.

## Verification

- New tests PASS; related old suites PASS untouched (BUG-327, CA-648/649, flow_frozen_scope_gate incl. CA-427 gate-rules/forged-file tests, freeze family, run243681/run151954/run201704): R1.
- `go test ./internal/changecontract/ ./internal/flowgate/` PASS; `go vet` clean; runner binary builds.
- Provider-agnostic (R2): drift comparison + gate filter take no providerKey; repro + true-drift matrix all three.
- Will not undo: CA-427 Finding 2 (flow-rules.json / forged siblings still drift), CA-738 (freeze planner guard), CA-737 (successor dispatch).