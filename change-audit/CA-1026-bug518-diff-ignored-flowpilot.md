# CA-1026 — BUG-518: worktree Diff tolerates repo-ignored `.flowpilot/`

## Context

Round-5 live test on the post-always-on binary (run-49109, tournament
devin hub + grok/devin candidates): the arbiter's patch snapshot failed
with `git add -N -- . ':(exclude).flowpilot'` exit 1 — "paths are
ignored: .flowpilot". The bed carries `.flowpilot/` in `.git/info/exclude`.
Verdict-time cleanup wiped both candidate worktrees and the run parked
`blocked` with no decision card.

## Changes

- `internal/worktree/manager.go` `Manager.Diff`: replaced the
  `.` + `:(exclude).flowpilot` pathspec sweep with explicit enumeration —
  `git ls-files -z --modified --others --exclude-standard` piped into
  `git add -N --pathspec-from-file=- --pathspec-file-nul`. Ignored paths
  are never named, so no ignore source can error the sweep. The
  `--modified` leg keeps the index write live whenever any change exists,
  preserving the BUG-453 fail-closed fault surface. Clean worktrees skip
  `add` entirely (`--pathspec-from-file` on empty input would error
  "nothing specified").
- Added `gitOutRaw` — `gitOut` without `TrimSpace`, since `-z` output
  carries NUL terminators as data.
- New test `internal/worktree/bug518_diff_ignored_flowpilot_test.go`
  reproduces the live condition (`.git/info/exclude` → `.flowpilot/` +
  runtime file inside the candidate's `.flowpilot`).

## Provider impact

Provider-agnostic — git plumbing inside the worktree manager; identical
for every adapter. No provider code touched.

## Residual

- The escalate-time cleanup already deleted run-49109's candidate
  worktrees before the fix — re-drive on the rebuilt binary exercises
  the re-run arbiter path (snapshot succeeds, or the BUG-453/459
  empty-patch escalate handles genuinely-empty worktrees).
- `ensureGitignore` writes `.flowpilot/worktrees/` into the repo
  `.gitignore`; the runner's broader `.flowpilot/` state exclusion comes
  from `.git/info/exclude` — both ignore sources are now handled.
