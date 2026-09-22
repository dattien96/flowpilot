# Task-407: Shared internal/worktree Manager Package (CP-71 P-1)

## Metadata

- Document ID: `Task-407`
- Title: `Shared internal/worktree Manager — Generalize Tournament Lifecycle`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-71](../../07-Coding-Plan/todo/CP-71-Run-Worktree-Isolation.md) `P-1`, [SD-27](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md) `D-1`
- Child Documents: `None`
- Related Documents: [Task-369](../done/Task-369-Worktree-Rollout-Manager.md), `CA-877`
- Replaces: `None`
- Tags: `worktree, runner, refactor, go`
- Feature Keys: `run-worktree`

## AI Quick View

### Summary

- Extract the proven worktree lifecycle from `internal/tournament/worktree_manager.go` into a shared `internal/worktree` package parameterized by `ownerID` + name prefix; `internal/tournament` delegates (prefix `candidate-`) so CP-65 behavior is byte-identical.
- Single implementation for git worktree ops — create/list/resolve/cleanup, `.base` sidecar, `MergeConflictError`, per-repo apply mutex.

### Current Ask

- Land `internal/worktree` + delegation, green tournament suite as the compatibility oracle, zero observable change.

### Key Decisions

- `T-1` Manager API takes `ownerID` + optional `prefix`; tournament calls with `prefix="candidate-"` preserving `.flowpilot/worktrees/candidate-<id>` paths and `WorktreePath` export.
- `T-2` All existing semantics copied verbatim: `git worktree add` from base commit, `git add -N` for untracked, `git apply --check` + apply, no-orphan cleanup.

### Constraints

- CP-65 tests must stay green untouched; `tournament.WorktreePath` export unchanged (signature + behavior).
- GitNexus `impact` on `WorktreeManager`/`WorktreePath`/`MergeConflictError` before editing.
- Real git repos in tests (`t.TempDir()` + `git init`); no mocks.

### Open Questions

- `None`

### Source Refs

- `SD-27 D-1/D-8`; `internal/tournament/worktree_manager.go` (entire file is the spec); `Task-369 T-1..T-5`.

## 1. Goal

One shared, well-tested worktree manager usable by tournament candidates and opt-in run/chat worktrees — without perturbing the shipped tournament path.

## 2. Parent Links

- coding plan: `CP-71 P-1`
- tech design: `SD-27 D-1, D-4, D-8`
- system spec: `SS-23 BR-1..BR-5`
- specific upstream ids: `Task-369`, `CA-877`

## 3. Trigger

CP-71 needs the lifecycle for runs; copying `worktree_manager.go` would fork subtle git code (Windows locking, path escape, cleanup ordering) — delegation is the decided approach (SD-27 D-1).

## 4. Exact Change

- `T-1` New `apps/local-runner/internal/worktree/manager.go`: `Manager` + `WorktreeInfo` + `MergeConflictError` + `.base` sidecar + gitignore append — generalized from tournament code, parameterized by `(repoDir, ownerID, prefix)`.
- `T-2` `internal/tournament/worktree_manager.go`: rewrite bodies to delegate; keep `WorktreePath(repoDir, candidateID)` export and all observable behavior identical.
- `T-3` New `internal/worktree/manager_test.go`: full lifecycle matrix on real temp repos.

## 5. Touched Areas

- files: `apps/local-runner/internal/worktree/manager.go` (new), `manager_test.go` (new), `internal/tournament/worktree_manager.go` (delegate rewrite).
- modules: `internal/worktree` (new), `internal/tournament`.
- routes: none. tables: none.

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/worktree/manager.go
package worktree

type Info struct {
    OwnerID    string
    Path       string
    Branch     string
    BaseCommit string
    Slug       string
    CreatedAt  time.Time
}

type MergeConflictError struct {
    OwnerID       string
    Reason        string
    Patch         []byte
    ConflictPaths []string
}
func (e *MergeConflictError) Error() string

type Manager struct{ /* git runner, repo mutexes */ }
func NewManager() *Manager

// prefix "" → .flowpilot/worktrees/<ownerID>; "candidate-" → candidate-<id> (CP-65 compat)
func (m *Manager) Create(ctx context.Context, repoDir, ownerID, prefix, baseCommit, slug string) (Info, error)
func (m *Manager) List(ctx context.Context, repoDir string) ([]Info, error)
func (m *Manager) Diff(ctx context.Context, repoDir, ownerID, prefix string) (patch []byte, err error)
func (m *Manager) Resolve(ctx context.Context, repoDir, ownerID, prefix string, mode ResolveMode, failOnConflicts bool) error
func (m *Manager) Cleanup(ctx context.Context, repoDir, ownerID, prefix string, keepBranch bool) error // idempotent
func (m *Manager) Validate(ctx context.Context, repoDir, ownerID, prefix string) error // D-7: worktree list + dir + .base
func Slugify(title, ownerID string) string // branch-safe slug + short id
```

```go
// apps/local-runner/internal/tournament/worktree_manager.go — delegates; exports preserved
func WorktreePath(repoDir, candidateID string) string // unchanged signature+behavior
// (Create/Diff/MergeWinner/Cleanup bodies call worktree.Manager with prefix "candidate-")
```

## 7. Test Signatures

- `TestManager_CreateIsolatesOwners` — 2 owners same base; write in A invisible in B and main.
- `TestManager_CreateRejectsInvalidOwnerID` — path escape (`..`, `/`, `\`) rejected.
- `TestManager_DiffIncludesUntracked` — `git add -N` parity (new files appear in patch).
- `TestManager_ResolveApplyPatchClean` — patch lands on undrifted main; no commit/branch created in main.
- `TestManager_ResolveConflictErrorCarriesEvidence` — drifted main → `MergeConflictError` with `Patch` + `ConflictPaths` (errors.As).
- `TestManager_ResolveKeepBranch` — branch `fp/<slug>` remains, worktree removed.
- `TestManager_CleanupIdempotent` — remove+prune; second call no-op; `git worktree list` clean.
- `TestManager_ValidateDetectsExternalDelete` — dir removed → Validate errors.
- `TestManager_SerializedApplyMutex` — two concurrent applies on one repo serialize; second re-checks drift.
- Tournament suite: all pre-existing `internal/tournament` tests green **unmodified** (compatibility oracle).

## 8. Acceptance Check

- `go test ./internal/worktree/...` green; `go test ./internal/tournament/...` green untouched.
- `tournament.WorktreePath` returns identical strings as before; `.gitignore` append additive.
- Zero `providerKey` references in the new package (grep proof — Case-1 agnostic).

## 9. Out of Scope

- Start-run plumbing, HTTP, run-record fields (Task-408); UX (Task-409/410); GC/resume (Task-411).

## 10. Definition of Done

- [ ] §6 signatures landed; §7 tests exist, green, additive-only.
- [ ] Entire `internal/tournament` suite untouched and green — any failure → STOP + report.
- [ ] Provider parity: Case-1 agnostic, grep-verified; noted in CA.
- [ ] `feature_key: run-worktree`; CA ledger entry written.
- [ ] `detect_changes` shows only worktree/tournament symbols.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
