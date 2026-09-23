---
id: CA-927
title: Portable canonical-head feature-key validation — NTFS-illegal / traversal keys rejected at staging (BUG-427)
type: BugFix
feature: change-contract
date: 2026-09-23
status: done
---

## Context

BUG-427 tracked three red tests on clean HEAD. Two were already resolved by
earlier clusters on this branch:

- `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` — green since CA-924
  (the reworked `runContextProduceNode` seeds contract/feature context so
  the freeze chain produces `planContextPackage` again).
- `TestRun147126_AuditHonorsFrozenContract` — green since the
  `changecontract/paths.go` symlink normalization (BUG-396 family) —
  `absWorkspace` is now `EvalSymlinks`-resolved, so macOS `/var`→
  `/private/var` no longer false-positives as a workspace escape.

Remaining: `TestFinalizePartialFailureCommitsNoHeadInTheBatch` failed on
POSIX because its failure injection stages a pending canonical record with
feature key `zzz-bad?feature` — `?` is illegal only on NTFS, so the Head
file write succeeded and the batch committed.

## Change

`internal/changecontract/head.go`:

- new `unsafeHeadFeatureKey` — rejects empty/blank keys, bare `.`/`..`
  segments, NTFS-illegal characters `<>:"/\|?*`, and control chars
  (< 0x20). Such a key's Head file is unwritable on at least one supported
  platform, and `..` would escape `.flowpilot/canonical/` entirely — a real
  portability + traversal gap, not merely a test accommodation.
- `StageHeadWrite` rejects unsafe keys with a descriptive error → the
  two-phase finalize batch fails deterministically on every platform.
- `LoadHead` reports unsafe keys absent (no traversal read).

## Tests (added only)

- `internal/changecontract/bug427_head_key_safety_test.go` — staging rejects
  `?`, `\`, `/`, `:`, `*`, `|`, `<`, `"`, `>`, `..`, `.`, control char,
  empty/blank keys; `LoadHead` reads unsafe keys as absent; ordinary
  kebab-case keys (incl. `SD-01-Auth`) still stage/commit/load unchanged.

## Result

- `TestFinalizePartialFailureCommitsNoHeadInTheBatch` — green on POSIX for
  the first time (the `zzz-bad?feature` record now fails staging
  deterministically, exercising the two-phase abort path as intended).
- `go test ./internal/changecontract` — all green.
- Runner canonical/finalize surface: only `TestFinalizerHookSurfacesArtifacts`
  fails — the documented pre-existing racy predicate (identical on baseline).

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-427
change_type: bugfix
summary: Portable canonical-head feature-key validation; feature migrated test-suite-health -> change-contract (BUG-442)
# --->8---
