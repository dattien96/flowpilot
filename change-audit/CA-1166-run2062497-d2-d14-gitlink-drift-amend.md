# CA-1166 — run-2062497 D2+D14: gitlink fingerprinting and directory-path amend

## What changed

`apps/local-runner/internal/runner/preflight.go` / `gate_hook.go`:

- `worktreeFileFingerprint` handles directories: a dir (submodule checkout
  or nested repo) fingerprints as its embedded `git rev-parse HEAD`, falling
  back to an unreadable sentinel — deterministic, never "".
- `baselineWorktreeFingerprint` now delegates to `worktreeFileFingerprint`
  per path. Previously the baseline stored sha256(content) (64 hex) for
  files and "" for dirs while the gate compared a 16-byte content|size|mode
  hash (32 hex) — formats never matched, so the freeze-baseline subtraction
  was dead code for every path type.
- Snapshot/diff keys are normalized (trailing `/` stripped) on both the
  snapshot-store and the lookup side — git reports untracked dirs as `dep/`
  but committed gitlinks as `dep`.

`apps/local-runner/internal/changecontract/paths.go`:

- New `IsAmendableDriftPath(workspace, p)` — the single Allow-path
  predicate: `IsUserAllowableDriftPath` OR an extension-less path naming an
  existing directory (submodule gitlink / untracked-dir row). Makefile
  shapes and nonexistent buckets stay rejected per CA-427 Finding 5.

`apps/local-runner/internal/changecontract/frozen_scope.go`:

- `AmendFrozenContractForAllow` accepts the directory case through
  `IsAmendableDriftPath` into `AllowedExtraPaths` — exact-match covers the
  gitlink/untracked-dir diff row the gate reports.

`apps/local-runner/internal/runner/interactive_handlers.go`:

- `handleAmendFlow` partitions by `IsAmendableDriftPath` (shared predicate —
  endpoint and contract layer can never disagree). Existing directories are
  amendable; an all-unamendable batch keeps the loud 422 with the
  concrete-code-target wording.

## Invariant

A path's drift fingerprint is format-identical on both sides of the
baseline comparison, and a real directory-shaped drift row is sanctionable
by the operator's Allow — while buckets, globs, flags, and extension-less
files remain rejected.

## Tests

`run2062497_gitlink_drift_test.go` (fingerprint parity, committed +
uncommitted gitlink no-drift), `bug366_allow_doc_amend_test.go`
(`TestRun2062497_ForAllowAcceptsExistingDirectory`,
`TestRun2062497_ForAllowStillRejectsNonexistentAndFileBuckets`).
