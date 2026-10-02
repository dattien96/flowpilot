# CA-1101 — BUG-554: exempt runner engine-state files from frozen scope drift

## Change

`internal/changecontract/frozen_scope.go` — additive entries to the two
existing runner-bookkeeping exemption lists:

- `RunnerLedgerBookkeepingPaths` += `.flowpilot/ledger/.cursor`
  (changeledger scan cursor), `.flowpilot/ledger-needs-update`
  (changeledger sentinel), `.flowpilot/catalog/features.ndjson`
  (contextsync/featurecatalog — same class as already-exempted
  `ledger/chat_summary.ndjson` / `feature_history.ndjson`).
- `RunnerOwnedConfigPaths` += `.flowpilot/engine-init.json`
  (`runEngineInit` idempotency record), `.flowpilot/tooling.json`
  (`tooling.CheckAll` persisted snapshot).

## Why

Live run-38799 (PrivateVault CP-02, 2026-10-02): the coder's
`FrozenContractScopeDrift` gate parked the flow on those five files, all
written by the runner itself mid-turn inside the gate's own diff window —
same self-park class as CA-649 (gate-metrics) and CA-1089
(workflow_drift_events), for engine files added after those lists.
`/agent-loop/amend` rejects `.flowpilot/*` as non-concrete targets, so no
product unblock path existed; the operator hand-minted a superseding
contract v3 to proceed.

## Security boundary unchanged (CA-427 Finding 2)

Exact-path exemptions only. Verified the newly-exempted files are not
gate-decision inputs: `tooling.CheckTool` re-probes binaries via `exec`
(tool result never read from tooling.json — `LoadToolingStatus` is
display/context only); `loadEngineInitState` only decides init re-run
idempotency. `.flowpilot/settings/flow-rules.json`, forged
`.flowpilot/contracts/*.ndjson`, and every other `.flowpilot/**` sibling
still drift — pinned by `TestBUG554EngineStateListsCoverLiveFlaggedPaths`
and the existing `TestCA649GateRulesFileStillDrifts`.

## Tests

- `bug554_runner_engine_state_drift_test.go` (new, reproduce-first —
  failed RED before the fix):
  - `TestGateDriftIgnoresRunnerEngineStateFiles` — all five runner files
    in the gate window → no block.
  - `TestGateDriftStillBlocksCodeAlongsideEngineState` — real
    out-of-scope code still blocks alongside engine-state churn.
  - `TestBUG554EngineStateListsCoverLiveFlaggedPaths` — pins path→list
    ownership + the CA-427 hole stays closed.
- `go test -count=1 ./internal/changecontract/...` — green.
- `go test -count=1 ./internal/runner/` — the BUG-554 tests green. The
  package suite has ~28 pre-existing failures on this machine
  (environmental: gitnexus chdir crashes, missing provider binaries; and
  stale expectations from unrelated uncommitted work). Verified
  identical RED on the same tests with the fix stashed — no regression
  introduced by this change.

## Provider scope

Shared runner gate path, computed identically for every provider —
provider-agnostic by construction (no adapter/session code touched).

## GitNexus

`detect-changes --scope all`: 3 symbols changed, 0 affected processes,
risk LOW. (Index 25 commits stale at time of run; the two functions are
leaf list/predicate producers consumed via the gate filter chain.)
