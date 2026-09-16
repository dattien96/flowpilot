# CA-877 — CP-65 P-2 Worktree Rollout Manager (Task-369)

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: Task-369
change_type: feature
summary: WorktreeManager — isolated git worktrees per candidate, patch-based winner merge, typed conflict error with card evidence, idempotent no-orphan cleanup
# --->8---

## Why

P-1 scores but candidates need isolated ground to run on: sharing one
working directory lets candidates overwrite each other (Frankenstein tree →
garbage verdicts → merging an untested blend). Git worktrees give one base
commit, free diff/patch/conflict machinery, and cheap teardown.

## Change

- **`internal/tournament/worktree_manager.go`** (new): `WorktreeManager`
  with `Create` (worktree at `.flowpilot/worktrees/candidate-<id>` from
  base commit = flow-start HEAD; rejects non-repos, unresolvable bases,
  duplicates, path-escape ids; records base in a sidecar + additive
  `.gitignore` entry, best-effort), `Diff` (intent-to-add + `git diff`),
  `MergeWinner` (HEAD-drift check + `apply --check` before real apply;
  conflict → auto-cleanup + typed `*MergeConflictError` carrying patch +
  parsed conflict paths for the ask_user card — no-orphan policy, no
  investigation retention), `Cleanup` (remove --force + prune, idempotent).
  Git-only, no providerKey (Case-1 agnostic), never commits.
- **`internal/tournament/worktree_manager_test.go`** (new): 3 Task-369
  signatures + conflict-evidence + bad-input matrix, all on real temp git
  repos (autocrlf pinned, local test identity).

## Tests

- 5/5 green: isolation (A↮B↮main invisible + same base + gitignore),
  cleanup (list clean, dirs + sidecars gone, 2nd call no-op),
  merge (content lands, HEAD sha + commit count unchanged),
  conflict (typed error, patch + `base.txt` path, workspace untouched,
  worktree gone, re-merge errors cleanly), bad input (6 bad ids, non-repo,
  bad base, dup create, ghost diff/merge).
- `gofmt`/`go vet` clean, `go build ./...` clean.
- R1: zero existing files modified (2 new files only) — no old suite
  surface; tournament package `ok`.
- R2: Case-1 agnostic — no providerKey param/branch (grep: only the
  classification comment); candidate providers are P-3 concern.

## Prior CA claims kept intact

- CA-876 arbiter contract untouched (no P-1 edits). CP-65 retry design +
  no-orphan policy implemented as specified (conflict evidence in error
  for the P-3 card). No pre-existing test modified.
