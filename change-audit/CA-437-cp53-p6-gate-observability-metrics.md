# CA-437 — CP-53 P-6 Gate Observability Metrics (Task-272)

## Scope

Retroactive change-audit for **Task-272 / CP-53 P-6**: append-only gate observability so operators can rank H-1 vs H-3 leak paths before tuning blind vs verdict gates.

## Prior CA (intact)

- BUG-288 / Task-242: corrupt baseline fail-closed and always-block `r-reg`/`r-tests` unchanged — metrics are observe-only.
- Task-155 gate settle path remains authoritative for block/override decisions.

## Changes

- `runner/gate_metrics.go` (new): `appendGateMetric` writes NDJSON to `.flowpilot/gate-metrics.ndjson` and logs `[gate-metric]` lines; dedupes rule IDs; records block / override / escalate / accept with optional cost/turn fields when present.
- `runner/gate_hook.go`: hooks on block, override, escalate, and accepted settle paths.
- Tests (additive): `runner/cp53_gate_metrics_test.go`.

## Provider impact

**Provider-agnostic** — gate metrics run in post-turn settle; no adapter wiring.

## Verification

```bash
go test ./internal/runner/ -run 'TestAppendGateMetric|TestGateMetricRuleIDs|TestRecordGateAcceptedMetric|TestRecordGateOverrideMetric' -count=1
```

## Out of scope / residual

- No desktop dashboard; operator reads NDJSON or log grep.
- Cost-per-accepted-change falls back to turn count when provider omits usage (v1).
- Does not change pass/block decisions.

## Commits

- `c8adfb7` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-272
change_type: feature
summary: Task-272 P-6 append-only gate metrics NDJSON plus gate-metric log lines on block override escalate and accept settle paths
# --->8---
