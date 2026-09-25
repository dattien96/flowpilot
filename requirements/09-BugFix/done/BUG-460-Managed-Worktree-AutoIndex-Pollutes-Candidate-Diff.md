# BUG-460 — Runner auto-index stamps tracked files inside managed worktrees, polluting candidate diffs

- Status: **FIXED** (CA-957)
- Found: live `run-21364` (CP-65 full live test, post-CA-956 build)
- Severity: high — runner bookkeeping noise became a winning tournament
  patch and merged into the main workspace, corrupting main's `AGENTS.md`
  index header.

## Symptom

`run-21364` (tournament-harness; candidates grok + codex):

- `candidate-b` (codex) wrote no code — but its captured patch was
  NON-empty: `gitnexus analyze` had run inside its worktree and rewrote the
  `<!-- gitnexus -->` index headers in tracked `AGENTS.md`/`CLAUDE.md`
  ("indexed as **full**" → "indexed as **candidate-candidate-b**").
- Arbiter auto-picked `candidate-b`; merge applied the snapshot —
  main workspace's `AGENTS.md`/`CLAUDE.md` now claim a worktree index name
  that does not exist there.
- `Mul` never landed.

## Root cause

`createRun` fires `ensureGitNexusIndexAsync(in.Cwd)` for every run — for
tournament candidate children `in.Cwd` is the candidate worktree under
`.flowpilot/worktrees/`. A git worktree carries a `.git` **file** (not dir),
so the "is a git repo" stat check passes and `gitnexus analyze` runs with
`cmd.Dir` = the worktree. The indexer's side effect rewrites tracked
`AGENTS.md`/`CLAUDE.md` headers — real file modifications that
`WorktreeManager.Diff` legitimately captures into the candidate patch.

Same vector for `ensureKnowledgeBaseForWorkspace`: `knowledge.WriteFull`
writes `.flowpilot/knowledge/**` inside the worktree — untracked there
(only `.flowpilot/worktrees/` is gitignored), so `git add -N` pulls it into
the diff too.

## Fix (CA-957)

- `gitnexus_autoindex.go`: new `isRunnerManagedWorktreePath` predicate
  (path under `<repo>/.flowpilot/worktrees/`); `ensureGitNexusIndexAsync`
  returns early for managed-worktree dirs.
- `knowledge_bootstrap.go`: same guard on `ensureKnowledgeBaseForWorkspace`.

Managed worktrees are ephemeral runner scratch — indexing/distilling them
is wasted work even without the pollution.

## Regression coverage

- `TestBug460AutoIndexSkipsManagedWorktree`: a `.flowpilot/worktrees/` dir
  with a `.git` file must not mark `gitnexusAnalyzeOnce`. Verified RED —
  pre-fix the auto-indexer actually launched on the worktree path.
- `TestIsRunnerManagedWorktreePath`: predicate table (nested paths, exact
  root, sibling-prefix negatives, empty).

## Residual risk (documented, not covered by this fix)

A candidate agent itself may invoke `gitnexus analyze` (the repo's own
AGENTS.md instructs agents to run GitNexus tooling before edits). That is
agent behavior inside its sandbox — the deterministic runner-side vector is
closed here; agent-driven indexing inside candidate worktrees remains a
possible diff-noise source and should be handled at prompt/contract level
if it recurs.
