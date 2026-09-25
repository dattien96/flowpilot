# CA-957 — BUG-460: skip auto-index + knowledge distill inside managed worktrees

## What changed

- `internal/runner/gitnexus_autoindex.go`: added
  `isRunnerManagedWorktreePath` (checks for the
  `.flowpilot/worktrees/` path segment via `worktree.WorktreeRootRel`);
  `ensureGitNexusIndexAsync` returns early on such paths.
- `internal/runner/knowledge_bootstrap.go`:
  `ensureKnowledgeBaseForWorkspace` guarded by the same predicate —
  `knowledge.WriteFull` output inside a candidate worktree is untracked and
  would land in the candidate's diff via intent-to-add.
- `internal/runner/bug460_worktree_autoindex_test.go`: predicate table
  test + a worktree-path auto-index suppression test (RED: pre-fix the
  indexer actually launched on the worktree dir).

## Why

Live `run-21364`: `createRun` fired `ensureGitNexusIndexAsync` on the
candidate child's `workspaceCwd` (a tournament worktree). `gitnexus
analyze` rewrote the worktree's tracked `AGENTS.md`/`CLAUDE.md` index
headers; that noise became candidate-b's whole patch, won the arbiter, and
merged into the main workspace — main's `AGENTS.md` briefly claimed index
name `candidate-candidate-b`.

## Scope / parity

- Guard is path-shape based, provider-agnostic; affects only
  runner-managed scratch dirs, never real workspaces.
- No schema/payload changes; existing once-map semantics unchanged.

## Evidence

- RED: `TestBug460AutoIndexSkipsManagedWorktree` — pre-fix run shows
  `[gitnexus] auto-index start workspace=".../worktrees/candidate-x"`.
- GREEN: both new tests pass.
- `go test -count=1 ./internal/runner -run 'TestBug460|TestIsRunnerManagedWorktreePath'`
  green.
