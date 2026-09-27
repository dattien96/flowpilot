# CA-1035 — audit knowledge hook skips runner-managed worktrees

Date: 2026-09-26 — review finding: audit knowledge Distill still reachable
inside worktrees.

## Change

`apps/local-runner/internal/runner/knowledge_bootstrap.go`:

- `updateKnowledgeForAudit` now returns early when `workspace` is a
  runner-managed worktree (`isRunnerManagedWorktreePath` — the same
  `.flowpilot/worktrees/` guard the bootstrap and GitNexus auto-indexer
  already apply). Previously an audit node completing inside a worktree
  with an existing knowledge base wrote the pending-update ledger and
  spawned `replayKnowledgeUpdates` → `knowledge.Distill` on the scratch
  path — both write into the candidate's captured diff (BUG-460 class).
  Merge-back on the main workspace still updates knowledge through the
  normal audit path, so nothing durable is lost.

## Why

The worktree guard existed only on the create-time bootstrap
(`ensureKnowledgeBaseForWorkspace`); the audit-time incremental path was
left open. `knowledge.Missing` usually short-circuited it, but a worktree
carrying a bootstrapped KB (repo commits `.flowpilot/knowledge`, or an
earlier stray write) defeated that incidental check — the durable ledger
append then ran unconditionally. Path-based guard is the correct contract:
runner-owned scratch is never a knowledge workspace.

## Tests

`knowledge_worktree_audit_test.go` (new, red-first):

- `TestAuditKnowledgeUpdateSkipsRunnerWorktree` — worktree fixture with a
  real `index.json` (so `Missing()` is false and the bug path is reachable);
  entered through `onAuditNodeCompleted`, the single choke point every
  audit completion path calls. RED before the fix: the pending-update
  ledger file was written (and `replayKnowledgeUpdates` spawned a
  worktree-scoped GitNexus distill — visible in test logs as an exec
  failure).
- `TestEnsureKnowledgeBaseSkipsRunnerWorktree` — pins the pre-existing
  bootstrap guard the review asked to unit-cover: a worktree path never
  enqueues `knowledgeBootstrapOnce`.

Regression: `go test -count=1 -run 'TestAuditKnowledgeUpdateSkipsRunnerWorktree|TestEnsureKnowledgeBaseSkipsRunnerWorktree|TestAuditHook|TestKnowledge'` — green.
