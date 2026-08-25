# CA-639 — Runner auto-indexes target project with GitNexus when .gitnexus is missing

## What

Running against a target project whose repo has no `.gitnexus` index silently
degraded scope-drift HighSeverity (CP-43 B14) and `source.dependence` (B12) to
warn/empty: `gitnexus impact` returned no dependents because the repo was never
analyzed. The runner now checks the workspace on run creation and, when `.gitnexus`
is absent but the gitnexus CLI is available and the workspace is a git repo,
auto-runs `gitnexus analyze` in the background once per process per workspace.

## Why

Operator has gitnexus installed (`/opt/homebrew/bin/gitnexus`, v1.4.8) and
`tooling.CheckTool` reports it `ok`, yet the C4/B14 live test never produced a
`block severity=high`. Root cause: `gitnexus status` in the sandbox returns
"Repository not indexed. Run: gitnexus analyze" — the repo was never analyzed, so
`HighSeverity` (structure dependents > 0) was false and the gate downgraded to
warn. Nothing told the operator to index first.

## Fix

- `apps/local-runner/internal/runner/gitnexus_autoindex.go` (new) —
  `ensureGitNexusIndexAsync(workspace)`: skip when `.gitnexus` exists / not a git
  repo / CLI unavailable; otherwise launch `gitnexus analyze` (10m timeout,
  best-effort, logged) guarded by a per-process per-workspace map; a failed
  analyze clears the guard for a later retry.
- `apps/local-runner/internal/runner/interactive_service.go` — new field
  `gitnexusAnalyzeOnce map[string]bool` on `InteractiveService`, initialized in
  `newInteractiveService`.
- `apps/local-runner/internal/runner/interactive_handlers.go` — `createRun` calls
  `ensureGitNexusIndexAsync(in.Cwd)` before creating the run (chat / flow /
  workflow all route through `createRun`).

Provider-agnostic (Case 1): fires on run creation regardless of provider; no
`ProviderKey` branch.

Will not undo: CA-637 scope-authority fix, CA-434 path→symbol, BUG-323 provider
CLI fix. The auto-index only guarantees the index exists; HighSeverity still
requires the indexed symbols to actually have dependents.

## Tests

Additive only:

- `apps/local-runner/internal/runner/gitnexus_autoindex_test.go` (new, stub
  `gitnexus` binary on PATH):
  - `TestEnsureGitNexusIndex_AutoAnalyzesOnce` — git repo w/o `.gitnexus` →
    analyze runs exactly once even when called twice.
  - `TestEnsureGitNexusIndex_AlreadyIndexedNoAnalyze` — `.gitnexus` present →
    no analyze.
  - `TestEnsureGitNexusIndex_NotGitRepoNoAnalyze` — no `.git` → no analyze.

## Verification

- `go test ./internal/runner -run 'TestEnsureGitNexusIndex|TestRun232492|TestPrepareChangeContract|TestCreateRun' -count=1` → green.
- `go test ./internal/runner -run 'TestRun232492|TestPrepareChangeContract|TestScopeDiff|TestChatGate|TestCanonicalHead|TestChangeContract|TestFlowCoderUsesFrozenScope|TestDependence' -count=1` → green (19.7s).
- `go test ./internal/changecontract/... ./internal/flowgate/... -count=1` → green.
- `go build ./internal/runner/... ./internal/tui/app/...` → clean.
- Full `./internal/runner` suite pre-existing env failures unchanged (Firebase/Supabase, flow env, LiveChat TempDir flake) — verified identical on clean tree in CA-637.

## Manual (operator tick)

After this ships and the runner restarts, opening a chat against a fresh target
project auto-runs `gitnexus analyze` (log line `[gitnexus] auto-index ...`).
Re-run C4/B14 afterwards → `block severity=high` once indexed symbols have
dependents.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: feature
summary: runner auto-runs gitnexus analyze once per target project when .gitnexus index is missing, so scope-drift HighSeverity and source.dependence get real dependents
# --->8---
