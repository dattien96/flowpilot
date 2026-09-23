# Task-424 — Worktree Uniqueness Verification for Parallel Runs

- Document ID: `Task-424`
- Title: `Worktree Uniqueness Verification (Parallel Same-Project Runs)`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-82 (Multi-Project Parallel Vibe Operations)`
- Child Documents: ``
- Related Documents: `SS-23, SD-27, CP-71, CA-922`
- Replaces: ``
- Tags: `run-worktree, local-runner, testing`

## AI Quick View

### Summary

- Code review already shows the guarantee exists: `ownerID` = chatId (chat
  runs) / runId (flow runs) → path `<repo>/.flowpilot/worktrees/<ownerID>`,
  branch `fp/<slug>-<owner-suffix>`; `Manager.Create` fails closed on an
  existing path. This task turns that review into executable proof.
- Test-only slice: add Go tests covering distinct owners, collision
  fail-closed, chat-leg inheritance, and concurrent provisioning.

### Current Ask

- Add the §7 test matrix under `apps/local-runner/internal/worktree` and
  `internal/runner`. Production code changes ONLY if a test exposes a real
  race/gap — then STOP and report per safe-fix-contract.

### Key Decisions

- `T-1` Verification-first: assume the code is correct; tests prove it. Any
  production change found necessary becomes a bugfix with its own CA note.
- `T-2` Ownership contract under test: chat runs → `chatId` owner (legs of
  the same chat share one worktree by design); flow runs → `runId` owner
  (every flow run gets its own tree).

### Constraints

- Additive tests only; no edits to existing worktree/runner tests.
- Tests must use `t.TempDir()` repos — no network, no user repos.

### Open Questions

- Whether `Manager.Create` has an atomic mkdir-level race between `os.Stat`
  and `git worktree add` — the concurrent test answers this; if racy, a mutex
  around Create is the minimal fix (would be reported, not silently added).

### Source Refs

- `CP-82 P-3`, `SS-23`, `SD-27 (D-7/D-8)`, `CP-71`

## 1. Goal

Executable proof that two worktree-enabled parallel runs in one repo can
never receive the same worktree — the user's only duty is toggling worktree
on; collision safety is code-guaranteed.

## 2. Parent Links

- coding plan: CP-82
- tech design: SD-27
- system spec: SS-23
- specific upstream ids: CP-71, CA-922

## 3. Trigger

User confirmed: parallel same-project runs are their responsibility to opt
into via worktree mode; FlowPilot must guarantee no random same-worktree
assignment once enabled.

## 4. Exact Change

- `T-1` New test file `internal/worktree/manager_uniqueness_test.go`.
- `T-2` New test file `internal/runner/run_worktree_parallel_test.go`
  covering provisioning + inheritance under concurrent owners.
- `T-3` No production edits unless a test proves a gap → report first.

## 5. Touched Areas

- files: the two new `_test.go` files only
- modules: `internal/worktree`, `internal/runner`
- routes: none
- tables: none

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/worktree/manager_uniqueness_test.go (new)
func TestManager_CreateDistinctOwnersGetDistinctPaths(t *testing.T)
func TestManager_CreateFailsClosedOnExistingPath(t *testing.T)
func TestManager_BranchNamesIncludeOwnerSuffix(t *testing.T)
```

```go
// apps/local-runner/internal/runner/run_worktree_parallel_test.go (new)
func TestWorktreeOwnerIDFor_ChatRunsUseChatID(t *testing.T)
func TestWorktreeOwnerIDFor_FlowRunsUseRunID(t *testing.T)
func TestWorktreeOwnerIDFor_ChatAndFlowNeverCollide(t *testing.T)
func TestProvisionRunWorktree_ConcurrentDistinctOwners(t *testing.T)
func TestChatLegsInheritExistingWorktreeBinding(t *testing.T)
```

## 7. Test Signatures

- `TestManager_CreateDistinctOwnersGetDistinctPaths` — two owners → two paths, two branches (AC-1)
- `TestManager_CreateFailsClosedOnExistingPath` — second create same ownerID → `worktree` error, no reuse (AC-2)
- `TestManager_BranchNamesIncludeOwnerSuffix` — `fp/<slug>-<owner-suffix>` (AC-1)
- `TestWorktreeOwnerIDFor_*` — owner-key contract (AC-3)
- `TestProvisionRunWorktree_ConcurrentDistinctOwners` — N goroutines, distinct owners → N distinct trees, zero errors (AC-4)
- `TestChatLegsInheritExistingWorktreeBinding` — second leg same chat → same path, no second create (AC-5)

## 8. Acceptance Check

- `go test ./internal/worktree ./internal/runner -run 'Worktree|Manager_'` green on Windows + CI.
- If any test exposes a real collision → STOP, report, fix under a bugfix CA.

## 9. Out of Scope

- Changing owner-key semantics, merge-back flow, GC policy.
- Forcing worktree on for users (explicitly rejected — user opt-in).

## 10. Definition of Done

- [ ] All §6/§7 tests exist, green, additive-only
- [ ] Pre-existing worktree/runner tests still green — old failure → STOP (R1)
- [ ] Provider-agnostic (worktree is provider-independent) — evidence in CA (R2)
- [ ] `feature_key` = `run-worktree`; CA ledger entry written
- [ ] GitNexus `detect_changes` clean before commit

## 11. Completion Notes

- result: verified — 8 new Go tests green (distinct owners, fail-closed collision, branch suffix, owner-key contract, 6-way concurrent provisioning, leg inheritance). No production gap found; zero prod edits.
- follow-ups: none — collision safety is code-guaranteed per spec.
- upstream docs updated: CA-925.
