# CA-438 — CP-53 P-1 Gate Blind Fail-Closed (Task-273)

## Scope

Retroactive change-audit for **Task-273 / CP-53 P-1**: classify missing baseline, oracle EnvError, and red-at-capture as first-class **`gate_blind`** instead of silent green.

## Prior CA (intact)

- **BUG-288**: corrupt baseline → fail-closed error path unchanged (`LoadBaseline` error ≠ missing).
- **r-reg / r-tests**: always-block sticky-until-green behavior not weakened.
- Task-272 metrics (CA-437) observe blind blocks but do not alter classification.

## Changes

- `flowgate/gate_blind.go` (new): `ClassifyGateBlind`, message formatters for missing baseline / env error / red-at-capture / clear.
- `runner/gate_blind_hook.go` (new): wired into post-turn gate enforce when production diff present; docs-only turns exempt.
- Tests (additive): `flowgate/cp53_gate_blind_test.go`, `runner/cp53_gate_blind_hook_test.go` — missing baseline, env error, red-at-capture, docs-only pass-through.

## Provider impact

**Provider-agnostic** — classification uses baseline snapshot and diff scope only.

## Verification

```bash
go test ./internal/flowgate/ -run 'TestClassifyGateBlind|TestGateBlindMessage' -count=1
go test ./internal/runner/ -run 'TestGateBlind' -count=1
```

## Out of scope / residual

- Go/TS baseline independence and dogfood hooks land in Task-275 (CA-440).
- `flaky-quarantine.json` schema referenced in plan; full quarantine UX deferred.
- Baseline freshness window uses implementation default — tune via follow-up if cry-wolf.

## Commits

- `8593aa9` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-273
change_type: feature
summary: Task-273 P-1 gate_blind fail-closed for missing baseline EnvError and red-at-capture while preserving BUG-288 corrupt baseline path
# --->8---
