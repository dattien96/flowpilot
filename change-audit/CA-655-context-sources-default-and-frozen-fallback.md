# CA-655 — Context sources default set and frozen-contract fallback for Flow F8

## What

Flow runs against `gate-sandbox` (and any Flow preflight) never showed `### source.dependence` (Task-259, CP-43 P-6) nor `### change.contract` in the coder/reviewer prompt, even though both sources are in `defaultContextSourceIDs` and the runner's `Collect` did call them. F8 verification (DivideChecked3) therefore had no `source.dependence` block to check, and `change.contract` was also empty on Flow even though the same run's `frozen_contracts.ndjson` had the declared contract. The built-in `context_artifact` ("Default Context (All Sources)" — `00000000-0000-0000-0000-000000000001`) compounded this for CCRS: it hard-coded only 3 ids (`feature.history`, `chat.summary`, `source.excerpt`), cutting `canonical.head` / `change.contract` / `source.dependence` at the artifact-binding tier (highest precedence).

## Why

1. **Fetch read the wrong store.** `changeContractSource` and `dependenceSource` called `changecontract.OpenStoreReadOnly` → `GetLatestForRun` on `contracts.ndjson`. Flow preflight freeze (CP-55) writes `frozen_contracts.ndjson` via `FrozenStore`, so `contracts.ndjson` is empty for that `runId` → `Fetch` returned `Body == ""` → `renderFlowContextSection` omitted the heading. `source.dependence` appeared “off by default” when it was actually “on but empty”.
2. **CCRS artifact cut the default set.** `20260709092000_add_builtin_context_artifact_instance.sql` seeded the default artifact with the old 3-id set. For CCRS the artifact binding wins over the runner default set, so CCRS never collected the three newer sources even after they joined `defaultContextSourceIDs`.
3. **YAML was stale.** `rag-harness.yaml` and `context-coding-review-synthesis.yaml` listed 5 ids (missing `source.dependence`) with a retired-tier comment; the list still validates via `ValidateFlowContextSources` but no longer drives `Collect` for harness (retired), so F8 on harness fell through to the default set anyway — yet the YAML misled operators.

## Fix

- `apps/local-runner/internal/runner/contract_for_context.go` (new) — `latestContractForRun(workspace, runID) (Contract, bool)` with strict precedence: 1) `contracts.ndjson` (Chat/legacy), 2) `frozen_contracts.ndjson` (Flow — pick latest active version across all coder steps via `ListForRun` + `GetFrozenForStep` active check), 3) miss → empty (preserves `TestDependenceSourceNoContractDegrades` / dir-bucket semantics). `frozenToContract` maps `FrozenContractRecord` → `Contract` (DeclaredPaths/Intent/FeatureKey, ConfidenceDeclared).
- `apps/local-runner/internal/runner/context_source_change_contract.go` — `Fetch` now calls `latestContractForRun` instead of `OpenStoreReadOnly` directly.
- `apps/local-runner/internal/runner/context_source_dependence.go` — same helper; otherwise unchanged (cap, GitNexus provider, budget).
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml` — `contexts.main_context.sources` now exactly reproduces `defaultContextSourceIDs` (6 ids, canonical order): `canonical.head`, `feature.history`, `change.contract`, `source.dependence`, `chat.summary`, `source.excerpt`; comment notes Task-259.
- `apps/local-runner/internal/agentpack/flow-pack/flows/context-coding-review-synthesis.yaml` — same 6-id parity.
- `supabase/migrations/20260827120000_update_builtin_context_artifact_sources.sql` (new) — UPDATE artifact `00000000-...-0001` from 3 to 6 ids so CCRS binding no longer cuts `canonical.head` / `change.contract` / `source.dependence`.
- `apps/local-runner/internal/runner/context_source_frozen_contract_fallback_test.go` (new, additive only) — 8 tests:
  - `TestChangeContractSourceFrozenFallback` — frozen-only run yields Body
  - `TestChangeContractSourceLegacyWinsOverFrozen` — legacy wins when both exist
  - `TestChangeContractSourceNoDataDegrades` — empty when no store
  - `TestChangeContractSourceFrozenDirBucketDegrades` — no panic on dir-bucket
  - `TestDependenceSourceFrozenFallback` — frozen → dependence Body
  - `TestDependenceSourceFrozenDirBucketYieldsEmpty` — dir-bucket → empty
  - `TestDependenceSourceFrozenGitNexusUnavailableNote` — frozen + GitNexus unavailable → note
  - `TestFrozenFallbackPicksLatestActiveAcrossSteps` — multi-step frozen picks newest active

Provider-agnostic (no Claude/Codex/Grok paths). No existing tests edited.

## Tests

Additive only:

- `go test ./internal/runner -run 'TestChangeContractSourceFrozen|TestDependenceSourceFrozen|TestFrozenFallback|TestChangeContractSourceNoData' -count=1 -v` → 9/9 PASS
- `go test ./internal/runner -run 'TestDependence|TestChangeContract|TestCanonical|TestRagHarnessContext|TestValidateFlowContextSources' -count=1` → 20/20 PASS
- `go test ./internal/changecontract -count=1` → PASS
- `go test ./internal/agentpack -count=1` → PASS
- `go vet ./internal/runner` → clean
- Existing `TestDependenceSourceNoContractDegrades`, `TestDependenceSourceInferredDirBucketYieldsEmpty`, `TestRagHarnessContextNodeFallsThroughToDefaultSet` still PASS (no edit).

## Verification

- Re-ran F8 scenario on `gate-sandbox` after `npx gitnexus analyze`: harness now renders `### source.dependence` when `frozen_contracts.ndjson` has `calc.go` (path→symbol `Calc`) and GitNexus indexed; harness without contract still omits heading (dir-bucket) — matches Task-259 expected.
- CCRS now collects the 6-id default set (artifact parity); no regression to Chat `contracts.ndjson` path.

## Residual

- `retrieval_locus` (CP-54 backward history ranking) still reads `contracts.ndjson` only — shares the same FrozenStore gap; left to a follow-up if ranking needs the frozen locus.
- `DeclaredSymbols` on `FrozenContractRecord` is absent — dependence targets remain path→symbol heuristic (`calc.go` → `Calc`), not per-symbol, for F8's `DivideChecked3` in `calc.go`.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: bugfix
summary: fix Flow context sources to read frozen contracts and make built-in artifact/YAML reproduce default set so source.dependence appears on F8
# --->8---
