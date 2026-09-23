# BUG-396: NormalizeDeclaredCodePaths symlink false-positive — "resolves outside workspace" under symlinked roots

## Metadata

- Document ID: `BUG-396`
- Title: `Workspace path not symlink-resolved before declared-path escape check → freeze always blocks under symlinked dirs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-55-Test-Steps](../../07-Coding-Plan/done/CP-55-Test-Steps.md), [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md)
- Feature Keys: `change-contract`

## AI Quick View

### Summary

- `NormalizeDeclaredCodePaths` builds `absWorkspace` with `filepath.Abs` only (no `EvalSymlinks`), but resolves each existing declared path with `filepath.EvalSymlinks` before `filepath.Rel` — under a symlinked workspace (macOS `/var`→`/private/var`, `/tmp`, symlinked project dirs) the resolved file lands outside the unresolved workspace prefix → `declared path … resolves outside workspace via symlink` → `contract.freeze` always blocks.
- Also produces deterministic macOS test failures: `TestRun147126_AuditHonorsFrozenContract` (grok/codex/claude) fails under default `TMPDIR` (`t.TempDir()` lives under `/var/folders/…`) and passes with a non-symlinked `TMPDIR`.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Contract freeze rejects legitimately in-scope declared paths with `changecontract: declared path "format.go" resolves outside workspace via symlink` whenever the workspace path itself traverses a symlink.
- **Expected:** Workspace is symlink-resolved (or the comparison done in resolved space) before judging escape — same treatment on both sides of `filepath.Rel`.
- **Actual:** One-sided resolution → false-positive escape → freeze rejected → flow parks before any writer runs.
- **Impact:** Any bed/workspace under `/tmp`, `/var`, or a symlinked home cannot freeze a contract whose planner draft declares ≥1 existing file — the flow-first contract lifecycle is unusable on common macOS temp/workspace layouts. Flaky-by-platform unit failures (`TestRun147126_*`, plus `TestRunContractFreezeNodeRecordsBaselineSHA` in the same environment) mask the defect in CI.

## Reproduction

1. Point a flow run's `cwd` at a directory under a symlinked path (e.g. `/tmp/...`, `/var/...`, or `os.TempDir()`-derived).
2. Have the contract planner declare at least one **existing** file.
3. `contract.freeze` → `NormalizeDeclaredCodePaths` → `EvalSymlinks` resolves the file to `/private/var/...` while `absWorkspace` remains `/var/...` → `filepath.Rel` reports escape → rejection.
- Automated repro: `go test ./internal/runner/ -run 'TestRunContractFreezeNodeRecordsBaselineSHA|TestRun147126_AuditHonorsFrozenContract'` — deterministic FAIL with default TMPDIR, PASS with `TMPDIR=/Users/tiendat/fp-beds/tmp`.
- runIds: no live freeze hit at CP-55 (bed path has no symlink component) — captured via deterministic automated repro in two sessions; CP43-004 documents the same failure mode.

## Root cause

- `apps/local-runner/internal/changecontract/paths.go:83-89` — `absWorkspace` assigned from `filepath.Abs(workspace)` without `EvalSymlinks`.
- `apps/local-runner/internal/changecontract/paths.go:119-127` — each existing declared path is `filepath.EvalSymlinks`-resolved then `filepath.Rel(absWorkspace, resolved)`; asymmetric resolution makes any symlinked-root workspace report `../` escape.

## Evidence

- `~/fp-beds/lt-evidence/cp55/RESULT.md` (BUG-LIVE-1 — mechanism + TMPDIR A/B)
- `~/fp-beds/lt-evidence/cp55/automated-tests.txt` — default-TMPDIR failures (`resolves outside workspace via symlink`)
- `~/fp-beds/lt-evidence/cp55/automated-runner-clean-tmpdir.txt` — same suite green with non-symlinked TMPDIR
- `~/fp-beds/lt-evidence/cp43/RESULT.md` (BUG-LIVE-CP43-004 — same `TestRun147126_AuditHonorsFrozenContract` symlink failure; `automated-tests.txt:1590,1764-1770`)
- `~/fp-beds/lt-evidence/cp67/RESULT.md` (Automated suite — same env FAIL on `TestRun147126_AuditHonorsFrozenContract`)

## Severity

- `medium` — environmental (requires symlinked workspace path) but deterministic there; blocks contract freeze outright and produces platform-dependent test failures.

## Completion Notes (implemented 2026-09-23, CA-920)

- `NormalizeDeclaredCodePaths` resolves the workspace root via `EvalSymlinks` before comparing, so declared paths and workspace share one coordinate system; absolute paths and nonexistent in-scope paths handled; real escapes still rejected.
- Side effect: previously-failing `TestRun147126_*` symlink-dependent baseline tests now pass on macOS `/var→/private/var` temp dirs.
- Unit: `bug396_symlink_workspace_test.go` green.
