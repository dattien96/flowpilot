# BUG-501 — Worktree git commands silently re-scope to the parent repo when `.git` link breaks

## Status
Fixed 2026-09-25 (live-found). Build b501 verified by re-drill.

## Live-found during
Re-run of the BUG-472 destructive-resolve drill on build b500 (live-test
bed `/Users/tiendat/fp-beds/full`, project `lt-full`): created worktree-bound
chat run `run-260771` (`worktree:true`), renamed the worktree's `.git` file,
then `DELETE /client/workflow-runs/run-260771?worktree=discard`.

**Expected (BUG-472 contract):** 503 `worktree_inspection_failed` — the
untracked-scan cannot verify cleanliness, so nothing is removed.
**Observed:** silent success — run deleted, worktree dir gone, no log, no
confirm prompt. A worktree with uncommitted work would be destroyed.

## Root cause

`git -C <worktree-path>` does NOT fail when the worktree's `.git` file is
missing — git discovery walks upward and resolves to the PARENT repository
(`/Users/tiendat/fp-beds/full/.git`).

Then every in-worktree scan returns a parent-scope answer:

- `Manager.Untracked` → `git ls-files --others --exclude-standard`: the
  worktree dir is under `.flowpilot/worktrees/`, which the parent's
  `.gitignore` ignores → **empty output, exit 0**. The discard gate reads
  "no untracked artifacts" and proceeds to `git worktree remove --force`
  (metadata at `.git/worktrees/<id>/` is intact, so removal succeeds).
- `Manager.Uncommitted` → `git status --porcelain`: same silent empty answer
  → `keep_branch` could drop work that was never committed.
- `Manager.Diff` → `git add -N` + `git diff`: same re-scoping → a patch
  computed against the wrong repository.
- `Manager.Cleanup` → `git rev-parse --abbrev-ref HEAD` inside the broken
  worktree returns the PARENT's branch name → `git branch -D` could target a
  branch the worktree does not own.
- `Manager.Validate` → checked dir + sidecar + `git worktree list`
  registration, but never the `.git` linkage — a registered-but-broken
  worktree passed as resumable.

Verified empirically (`/tmp/gitprobe`): `git rev-parse --show-toplevel` inside
the broken worktree returns the parent toplevel, and
`ls-files --others --exclude-standard` returns empty with a planted
`secret.txt` present — because `--exclude-standard` honors the parent's
`.gitignore` which covers `.flowpilot/worktrees/`.

## Fix

New `requireWorktreeScope(path)` in `internal/worktree/manager.go`: runs
`git rev-parse --show-toplevel` inside the worktree path and requires the
result to equal the path itself (`normalizePath` on both sides). Any other
toplevel → `.git` linkage broken → error.

Wired into all in-worktree scans:

- `Untracked`, `Uncommitted`, `Diff` — return error → callers (discard /
  keep_branch / apply_patch gates) surface 503 `worktree_inspection_failed`
  and remove nothing.
- `Cleanup` — the `--abbrev-ref HEAD` branch probe is skipped when scope is
  broken, so the parent's branch can never be named for `-D`.
- `Validate` — a registered-but-scope-broken worktree now fails validation →
  binding reported lost (GC-safe: `gcUnknown`, never orphan-pruned).

## Tests

`internal/worktree/bug501_scope_confusion_test.go`:
- Untracked / Uncommitted / Diff error on `.git`-broken worktree under a
  gitignored worktrees root (production layout mirrored).
- Validate fails closed on the same state; passes on a healthy worktree
  (positive control).
- Healthy Untracked still reports planted untracked file.

## Live verification (build b501)
Fault re-drilled on `run-260789`/`cht_482553d4fa86`: `.git` renamed →
`DELETE ?worktree=discard` → **503 `worktree_inspection_failed`**, dir
preserved; `.git` restored → discard → dir removed, run deleted. Original
BUG-472 contract now holds under the parent-scope confusion too.

## Regression risk
Adds one `git rev-parse` subprocess per scan/gate call — cheap. Healthy
worktrees pass the toplevel check unchanged (verified by
`TestBUG501_HealthyWorktreeStillScans` + full `internal/worktree` and
runner worktree suites).
