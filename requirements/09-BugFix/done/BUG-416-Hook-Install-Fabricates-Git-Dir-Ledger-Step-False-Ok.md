# BUG-416: `hook_install` fabricates `.git/hooks` in non-git dirs; `changeledger_build` reports ok for a file never created

## Metadata

- Document ID: `BUG-416`
- Title: `InstallPostCommitHook creates .git skeleton in non-git targets; changeledger_build step detail names a nonexistent feature_history.ndjson`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-34-Init-tool](../../07-Coding-Plan/done/CP-34-Init-tool.md), evidence `~/fp-beds/lt-evidence/cp34/RESULT.md` (BUG-LIVE-2 + BUG-LIVE-3)
- Feature Keys: `engine-init`, `change-ledger`, `post-commit-hook`

## AI Quick View

### Summary

- `InstallPostCommitHook` does `os.MkdirAll(<repo>/.git/hooks)` unconditionally → a stray `.git/hooks/post-commit` skeleton appears in directories that are not git repos.
- Sibling issue: the `changeledger_build` init step reports `outcome=ok` with detail path `.flowpilot/ledger/feature_history.ndjson` even when the ledger `Upsert` no-ops on zero commits and the file is never created.
- Both are cosmetic/hygiene defects on the init path; neither blocks the flow.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

1. On a non-git test bed, init's `hook_install` step reports `ok` and leaves `.git/hooks/post-commit` on disk while `git status` still fails `fatal: not a git repository`.
2. On the same bed, the `changeledger_build` step reports `ok` with detail `.flowpilot/ledger/feature_history.ndjson`, but `.flowpilot/ledger/` is empty (0 commits → `Upsert` no-op).

### Expected

- Hook install is skipped (or reports a skip outcome) when the target is not a git work tree.
- `changeledger_build` reports `ok` only when the ledger file exists, or reports a truthful `skipped`/`empty` detail.

### Actual

- A stray `.git` skeleton directory is created in non-repo targets.
- The step detail implies an artifact exists when it doesn't; after `git init` + re-init the file populates correctly (entry `feature_key=calc-core`, confidence=high).

### Impact

- Anything detecting "is a repo" via `.git` presence may misbehave on the target directory.
- Engine page step details mislead: they imply `feature_history.ndjson` exists when it doesn't. Harmless but false reporting.

## Reproduction

1. Create/point a project at a directory with no `.git` (e.g. a plain clone that dropped `.git`).
2. `POST /client/projects {directoryPath: <bed>}` → init `success`.
3. `ls <bed>/.git/hooks/post-commit` → exists; `git -C <bed> status` → `fatal: not a git repository`.
4. Inspect init step `changeledger_build` → `outcome=ok`, detail names `.flowpilot/ledger/feature_history.ndjson`; `ls .flowpilot/ledger/` → empty.

## Root cause

- `apps/local-runner/internal/changeledger/hook.go:38-58` — `InstallPostCommitHook` runs `os.MkdirAll(<repo>/.git/hooks)` unconditionally; no `git rev-parse --is-inside-work-tree` / `.git/HEAD` presence gate.
- `apps/local-runner/internal/runner/engine_setup.go:376` — `changeledger_build` step detail always names `.flowpilot/ledger/feature_history.ndjson` regardless of whether `Upsert` wrote anything (no-op on zero commits).

## Evidence

- `~/fp-beds/lt-evidence/cp34/RESULT.md` — BUG-LIVE-2 (L-34-1) and BUG-LIVE-3.
- `l34-1-engine-init.json` (step `outcome=ok` on non-git bed), bed `ls` output showing `.git` skeleton + empty `.flowpilot/ledger/`; after `git init` + re-init `feature_history.ndjson` populates (`l34-1b-init-git.json`).
- Suggested gate (from evidence): `git rev-parse --is-inside-work-tree` or presence of `.git/HEAD`.

## Severity

- `low` — cosmetic/misleading init artifacts; no functional break.

## Completion Notes (implemented 2026-09-23, CA-925)

- Root cause confirmed: `InstallPostCommitHook` unconditionally ran
  `os.MkdirAll(<dir>/.git/hooks)` — fabricating `.git/` on non-git dirs and
  failing outright on linked worktrees (where `.git` is a pointer file);
  `changeledger_build` reported the ledger path unconditionally even when
  `Build` wrote nothing.
- Fix: `changeledger.ErrNotGitRepo` + `resolveHooksDir` — `.git` absent →
  sentinel (nothing created); `.git` dir → `.git/hooks`; `.git` file →
  `gitdir:` target's `hooks/`. `engine_setup.go` maps the sentinel to
  `hook_install outcome=skipped ("not a git work tree")` and only reports
  `changeledger_build` ok when `feature_history.ndjson` actually exists —
  otherwise `skipped` with a truthful detail.
- Tests: `internal/changeledger/bug416_hook_nonrepo_test.go` (non-git skip
  + worktree gitdir resolution), `internal/runner/
  bug416_init_step_truth_test.go` (step outcomes on non-git + empty repo).
- Live: init on `/tmp/fp-live-j/nongit` → `hook_install=skipped`,
  `changeledger_build=skipped`, `.git` never created; init on the git bed
  → real `post-commit` hook + real `feature_history.ndjson`.
