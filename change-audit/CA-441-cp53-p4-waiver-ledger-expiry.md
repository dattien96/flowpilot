# CA-441 — CP-53 P-4 Waiver Ledger With Expiry (Task-276)

## Scope

Retroactive change-audit for **Task-276 / CP-53 P-4**: test overrides become time-bounded waiver debt with required reason; expired entries re-arm on load.

## Prior CA (intact)

- **Task-155 / BUG-289**: sticky-until-green when suite goes green before expiry unchanged.
- Override decision-card UX extended, not removed.

## Changes

- `flowgate/waiver_ledger.go` (new): ledger at `.flowpilot/settings/waiver_ledger.json`, default **14-day TTL**, prune expired on `LoadOverrides`.
- `flowgate/override.go`: `SaveOverrideWithReason`, extended `Override` with reason/expiry metadata.
- `runner/gate_hook.go`: `RecordGateAgreement(runID, testNames, reason)` rejects empty reason.
- `runner/interactive_handlers.go`: `POST .../gate-agreement` body requires non-empty `reason` (**breaking** for clients omitting reason).
- Tests (additive): `flowgate/cp53_waiver_newtest_test.go` — ledger write, empty reason reject, expired re-arm.

## Provider impact

**Provider-agnostic** — waiver persistence is gate/operator path only.

## Verification

```bash
go test ./internal/flowgate/ -run 'TestSaveOverrideWithReason|TestExpiredOverride' -count=1
```

## Out of scope / residual

- Desktop client must send `reason` on gate-agreement if not already — verify desktop parity.
- Open-waiver operator dashboard deferred.

## Commits

- `56a9997` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-276
change_type: feature
summary: Task-276 P-4 waiver ledger with 14-day expiry required reason on gate-agreement and expired override re-arm on load
# --->8---
