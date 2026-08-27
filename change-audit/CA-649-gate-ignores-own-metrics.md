# CA-649 — FrozenContractScopeDrift ignores runner's own gate-metrics file

## What

Live F3 flow (run-161570): the coder's gate pass parked with

```
flow gate block: flow scope drift: wrote outside the frozen contract's
declared paths: .flowpilot/gate-metrics.ndjson
```

The writer never touched it — the RUNNER's own gate observability file did.

## Why

`appendGateMetric` (gate_metrics.go, CP-53 P-6) appends `.flowpilot/gate-metrics.ndjson`
on EVERY gate pass (block/override/escalate/accepted). The file therefore is
ALWAYS dirty at the exact moment `runChildArtifactOutputGateAtEpoch` computes
`ObserveGitDiffSince(rec.BaseSHA)` for the coder's own `FrozenContractScopeDrift`
check, and it is not among the runner-ledger bookkeeping exemptions — so the
gate parks itself on its own observability write. (Other runner-owned files
such as `.flowpilot/canonical/*.json` are protected by the turn-start/baseline
fingerprint subtraction; gate-metrics is special because it grows during the
very gate pass being judged.)

## Fix

`apps/local-runner/internal/changecontract/frozen_scope.go`:

- `RunnerLedgerBookkeepingPaths()` gains exactly `.flowpilot/gate-metrics.ndjson`.
- The gate filter already routes through `IsRunnerLedgerBookkeepingPath`, so
  `gate_hook.go` needs no change.
- CA-427's security hole stays closed: `.flowpilot/settings/flow-rules.json`
  is still drift (only exact bookkeeping files are exempt).

Provider-agnostic (Case 1).

Will not undo: CA-427 (no `.flowpilot/**`-wide exemption), CA-640/645/648
freeze + gate exemptions, BUG-327 ledger exemption.

## Tests

Additive only — `ca648_tool_scaffold_gate_exemption_test.go` and legacy suites
untouched.

- `apps/local-runner/internal/runner/ca649_gate_metrics_exemption_test.go` (new):
  - `TestGateDriftIgnoresGateMetricsNdjson` — exact repro: gate-metrics present
    → gate passes.
  - `TestGateDriftStillBlocksCodeAlongsideGateMetrics` — `src/surprise.go`
    still blocks even with metrics churn.
  - `TestCA649GateRulesFileStillDrifts` — `flow-rules.json` rewrite still
    blocks (CA-427 guard).
  - `TestRunnerLedgerBookkeepingPathsIncludesGateMetrics` — predicate matrix,
    incl. negative on `flow-rules.json`.

## Verification

- `go test ./internal/runner -run 'TestGateDrift|TestCA649|TestRunnerLedgerBookkeepingPaths' -count=1` → green.
- `go test ./internal/runner -run 'TestRun151954|TestFlowScopeDrift|TestFlowCoder' -count=1` → green.
- `go test ./internal/changecontract/...` → green. `go vet` clean.

## Manual (operator tick)

Restart runner → re-run F3: `gate-metrics.ndjson` no longer parks the coder
gate; a REAL out-of-scope file still parks, then amend via CA-647.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: FrozenContractScopeDrift exempts .flowpilot/gate-metrics.ndjson (the runner's own gate observability file appended on every gate pass) from the frozen-scope drift check so the gate no longer parks itself on its own metrics write (run-161570)
# --->8---