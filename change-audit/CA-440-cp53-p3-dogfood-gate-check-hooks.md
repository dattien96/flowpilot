# CA-440 — CP-53 P-3 Dogfood Gate-Check + Hooks (Task-275)

## Scope

Retroactive change-audit for **Task-275 / CP-53 P-3**: ship `scripts/gate-check` and installable git pre-commit + Claude Stop hook so FlowPilot dogfoods its regression oracle.

## Prior CA (intact)

- **CA-438** gate_blind classification consumed by dogfood baseline checks.
- BUG-288 contracts not weakened.

## Changes

- `apps/local-runner/cmd/gate-check/main.go` (new): CLI entry for dogfood suite check.
- `flowgate/dogfood_check.go` (new): separate Go + TS baseline load/save; missing Go baseline hard-blocks; missing TS baseline warn-only (R-4).
- `scripts/gate-check`, `scripts/install-gate-hooks.sh`, `scripts/hooks/pre-commit`.
- `scripts/claude-stop-hook.settings.json.example` — merge into local `.claude/settings.json` (gitignored).
- Tests (additive): `flowgate/cp53_dogfood_check_test.go`.

## Provider impact

**Provider-agnostic** — script invokes local test baselines, not provider adapters.

## Verification

```bash
go test ./internal/flowgate/ -run 'TestCheckDogfood|TestLoadSaveBaselineFile' -count=1
# Linux/CI: scripts/gate-check after bootstrap (manual / CI follow-up)
```

## Out of scope / residual

- Full pre-commit dogfood proof on Linux CI not recorded in this CA session — operator should run on Linux/WSL.
- Windows native spawn parity documented as Git Bash/WSL only.
- TS baseline soft-block until suite green.

## Commits

- `2746fd3` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-275
change_type: feature
summary: Task-275 P-3 dogfood gate-check CLI with separate Go and TS baselines plus pre-commit and Claude Stop hook bootstrap scripts
# --->8---
