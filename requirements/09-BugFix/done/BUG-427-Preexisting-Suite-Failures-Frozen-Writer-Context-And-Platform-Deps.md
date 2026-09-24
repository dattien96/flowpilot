# BUG-427: Pre-existing suite failures — `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` deterministic fail + platform-dependent finalize/freeze tests

## Metadata

- Document ID: `BUG-427`
- Title: `internal/runner suite red on clean HEAD 435e336b — frozen-writer context package never built; NTFS-only filename assumption and macOS symlink false-positive in sibling tests`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-35-Context-And-Regression-Engine-Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp35/RESULT.md` (BUG-LIVE-004) + `~/fp-beds/lt-evidence/cp43/RESULT.md` (BUG-LIVE-CP43-004)
- Feature Keys: `test-suite-health`, `frozen-writer`, `context-package`, `platform-portability`

## AI Quick View

### Summary

- `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` fails deterministically on clean HEAD `435e336b` — `flow_frozen_writer_e2e_test.go:68` "expected a context package to have been built" (freeze chain produces no `planContextPackage`); re-verified in isolation. Sibling `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus` passes.
- `TestFinalizePartialFailureCommitsNoHeadInTheBatch` injects failure via feature key `zzz-bad?feature` — `?` is illegal only on NTFS, so the write succeeds on POSIX and the test fails on macOS/Linux (non-portable failure injection).
- `TestRun147126_AuditHonorsFrozenContract` (grok/codex/claude subtests) fails under default macOS TMPDIR: `t.TempDir()` lives under `/var/folders/…` where `/var`→`/private/var`; declared-path resolution evals symlinks on the file but not the workspace → `declared path "format.go" resolves outside workspace via symlink` — a real robustness gap, not just test-only (same family as CP-55 BUG-LIVE-1).

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

`go test ./internal/runner` is red on clean HEAD in three independent ways:

1. `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` — deterministic, environment-independent.
2. `TestFinalizePartialFailureCommitsNoHeadInTheBatch` — fails on POSIX (passes only where `?` is an illegal filename char, i.e. NTFS).
3. `TestRun147126_AuditHonorsFrozenContract` — fails only when TMPDIR traverses a symlink (macOS default); passes with `TMPDIR=/non-symlinked`.

### Expected

Suite green on a clean checkout across supported dev platforms; failure injection portable; freeze path symlink-safe.

### Actual

- (1) Freeze chain produces no `planContextPackage` → `t.Fatal` at `flow_frozen_writer_e2e_test.go:68` (also asserted at :138).
- (2) Batch finalize commits because the `?` filename is legal on POSIX → expected failure never injected.
- (3) `NormalizeDeclaredCodePaths` builds `absWorkspace` via `filepath.Abs` only (no `EvalSymlinks`) while declared paths are compared after `filepath.EvalSymlinks` → false-positive escape rejection.

### Impact

- Red suite on clean HEAD masks real regressions for anyone running `internal/runner` scoped tests.
- (3) suggests a production gap: workspaces under symlinked roots (macOS `/tmp`, symlinked project dirs) may fail `contract.freeze` — verified live-shaped in CP-55 (same tests pass under non-symlinked TMPDIR).

## Reproduction

```bash
cd apps/local-runner
go test -count=1 -run TestFirstCoderContextUsesCurrentFlowDeclaredPaths ./internal/runner/   # deterministic FAIL
go test ./internal/runner/ -run 'TestFinalizePartialFailureCommitsNoHeadInTheBatch'          # FAIL on macOS/Linux
go test ./internal/runner/ -run 'TestRun147126_AuditHonorsFrozenContract'                    # FAIL under default TMPDIR; PASS with TMPDIR=<non-symlinked>
```

## Root cause

- (1) `apps/local-runner/internal/runner/flow_frozen_writer_e2e_test.go:68` — the freeze chain does not produce `planContextPackage` at HEAD `435e336b` (context-package-build regression guard failing; cause not yet isolated — capture only).
- (2) Failure injection via feature key `zzz-bad?feature` assumes `?` is illegal — NTFS-only assumption; on POSIX the head file write succeeds and the batch commits.
- (3) `apps/local-runner/internal/changecontract/paths.go` `NormalizeDeclaredCodePaths` (~:119-129) — `absWorkspace` not symlink-resolved while declared paths are; `filepath.Rel` reports escape for `/var` vs `/private/var`.

## Evidence

- `~/fp-beds/lt-evidence/cp35/RESULT.md` — BUG-LIVE-004 (`autotest.log` L403, L903-904; isolated re-run confirmed).
- `~/fp-beds/lt-evidence/cp43/RESULT.md` — BUG-LIVE-CP43-004 (`automated-tests.txt:1590, 1764-1770`).
- `~/fp-beds/lt-evidence/cp55/RESULT.md` — BUG-LIVE-1 confirms the symlink mechanism: same tests FAIL under default TMPDIR, PASS under `TMPDIR=/Users/tiendat/fp-beds/tmp` (`automated-tests.txt` vs `automated-runner-clean-tmpdir.txt`).
- Verified on main worktree HEAD `435e336b`: `flow_frozen_writer_e2e_test.go:68` and :138 carry the assertion.

## Severity

- `medium` — deterministic suite failure on clean HEAD + two platform-dependent failures; (3) maps to a real symlinked-workspace freeze defect (see also the CP-55 live finding family).

## Completion Notes (implemented 2026-09-23, CA-927b)

- Sub-bug (1) `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` — resolved
  by CA-924b (Cluster I): the reworked `runContextProduceNode` seeds
  contract/feature context, so the freeze chain produces
  `planContextPackage` again. Verified green on this branch.
- Sub-bug (3) `TestRun147126_AuditHonorsFrozenContract` — resolved by the
  `changecontract/paths.go` symlink normalization (BUG-396 family): the
  workspace side is now `EvalSymlinks`-resolved so `/var`→`/private/var`
  no longer false-positives. Verified green on this branch.
- Sub-bug (2) `TestFinalizePartialFailureCommitsNoHeadInTheBatch` — fixed
  in CA-927b without touching the test: `changecontract.StageHeadWrite` now
  rejects feature keys that cannot produce a portable Head filename
  (NTFS-illegal chars, `.`/`..`, control chars) via new
  `unsafeHeadFeatureKey`; `LoadHead` reports such keys absent. The
  `zzz-bad?feature` record therefore fails staging deterministically on
  POSIX too, exercising the intended two-phase abort.
- Tests added: `internal/changecontract/bug427_head_key_safety_test.go`.
- Remaining known suite noise is tracked separately (pre-existing
  flakes documented in earlier cluster notes).
