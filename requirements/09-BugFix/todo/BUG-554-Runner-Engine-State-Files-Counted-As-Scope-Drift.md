# BUG-554 — runner engine-state files counted as coder scope drift; gate self-parks with no product unblock path

## Status
RESOLVED — CA-1101 (2026-10-02). Regression tests in
`apps/local-runner/internal/runner/bug554_runner_engine_state_drift_test.go`
verified RED→GREEN. Live-found during PrivateVault CP-02 vibe run
`run-38799` (2026-10-02 ~07:55–08:20).

## Symptom
The coder's frozen-contract scope-drift gate parked the flow on files the
**runner itself** writes into the target repo every turn:

```text
.flowpilot/engine-init.json
.flowpilot/tooling.json
.flowpilot/ledger-needs-update
.flowpilot/catalog/features.ndjson
.flowpilot/ledger/.cursor
```

None are in `RunnerLedgerBookkeepingPaths` / `RunnerOwnedConfigPaths`, so
`FrozenContractScopeDrift` reported them as writer drift. The gate emitted
`flow scope drift` → coder escalate → park. Because `.flowpilot/*` paths are
not concrete code targets, `/agent-loop/amend` rejected them
("amendment path is not a concrete code target") — **no product path
exists to unblock**; the operator had to hand-mint a superseding contract
v3 with `allowed_extra_paths` directly in `frozen_contracts.ndjson`.

This recurs on every repo where `.flowpilot` is git-tracked engine state
(PrivateVault tracks it deliberately for device-switch durability): the
runner churns these files mid-turn, inside the exact diff window the gate
observes — same self-park class as CA-649 (gate-metrics.ndjson) and
CA-1089 (workflow_drift_events.json), for files added to the engine after
their exemption lists were written.

## Expected
Runner-owned engine-state writes must never be attributed to the flow
writer as scope drift. Exemption stays exact-path only per CA-427
Finding 2: `.flowpilot/settings/flow-rules.json`, forged
`.flowpilot/contracts/*.ndjson`, and every other `.flowpilot/**` sibling
stay fully subject to drift enforcement.

## Root cause
`frozen_scope.go` exemption lists predate several engine-state files:

| Path | Writer | Gate-input risk |
|---|---|---|
| `.flowpilot/engine-init.json` | `runEngineInit` init-idempotency record (engine_setup.go:564) | none — init bookkeeping; gates live in the runner binary |
| `.flowpilot/tooling.json` | `tooling.CheckAll` persisted snapshot (check.go:215) | none — `CheckTool` re-probes binaries via `exec`, never reads the json; `LoadToolingStatus` is display/context only |
| `.flowpilot/ledger-needs-update` | changeledger sentinel (hook.go:17) | none — a flag file, not a gate input |
| `.flowpilot/catalog/features.ndjson` | featurecatalog/contextsync (catalog.go:481) | none — same class as already-exempted `ledger/chat_summary.ndjson` / `feature_history.ndjson` (contextsync inputs) |
| `.flowpilot/ledger/.cursor` | changeledger scan cursor (ledger.go:180) | none — scan position bookkeeping |

## Fix
- `RunnerLedgerBookkeepingPaths` += `ledger/.cursor`,
  `ledger-needs-update`, `catalog/features.ndjson`.
- `RunnerOwnedConfigPaths` += `engine-init.json`, `tooling.json`.
- Regression test `bug554_runner_engine_state_drift_test.go`: runner
  bookkeeping writes during the gate window must not park; a real
  out-of-scope code write alongside them must still block;
  `.flowpilot/settings/flow-rules.json` still drifts (CA-427 Finding 2
  guard).

## Provider scope
Gate runs in shared runner code pre-dispatch of the verdict; the drift
list is computed identically regardless of provider — provider-agnostic.
