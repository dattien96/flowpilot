# CA-434 — Symbol Input For GitNexus Impact + source.dependence (Task-259)

## Scope

Follow-up to [CA-433](CA-433-gitnexus-structure-provider-dependents-fix.md) / [BUG-323](../requirements/09-BugFix/done/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md): wire **symbol-shaped** GitNexus impact targets into change-contract drift (`HighSeverity`) and ship **`source.dependence`** context source ([Task-259](../requirements/08-Task/done/Task-259-Source-Dependence-Context-Source.md), CP-43 P-6).

## Changes

- `changecontract/parse.go`: parse `symbols:` in `[Change Contract]` blocks → `DeclaredSymbols`.
- `changecontract/impact_targets.go` (new): `GitNexusImpactTargets`, `GitNexusQueryTargetsForPath`, `pathBasenameToGitNexusSymbol`.
- `changecontract/scope.go`: `HighSeverity` queries derived symbol first, then raw path (legacy test-double compat).
- `runner/context_source_dependence.go` (new): Task-259 blast-radius source; priority 3; GitNexus-only v1; shared 25s budget.
- `runner/context_sources_builtin.go`: register + default set after `change.contract`.
- `runner/flow_context_package.go`: default priority fill for `source.dependence`.
- `apps/desktop-flowpilot/.../WorkflowsSettings.tsx`: UI descriptor.
- Additive tests: `impact_targets_test.go`, `context_source_dependence_test.go`.

## Design notes

- GitNexus CLI accepts **symbols**, not repo paths. Concrete declared files map to exported identifiers from basename (`gate_hook.go` → `GateHook`).
- `HighSeverity` keeps **path fallback** so pre-existing `scope_test.go` fakes keyed by file path stay green (additive-tests-only).
- `source.dependence` uses symbols only (no path fallback) — matches real CLI behavior post BUG-323.
- Inferred contracts (dir buckets only) → empty dependence section by design (Task-259 T-3).

## Verification

- `go test ./internal/changecontract/... -count=1`
- `go test ./internal/runner/ -run 'TestDependence|TestHighSeverity|TestScopeDiff|TestBuildFlowContextPackageOutputUnchanged' -count=1`

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-259
change_type: feature
summary: Parse symbols in change contracts, map concrete paths to GitNexus symbols, wire HighSeverity + source.dependence blast-radius context
# --->8---
