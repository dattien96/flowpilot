# Task-410: Worktree Merge-Back UX (CP-71 P-4)

## Metadata

- Document ID: `Task-410`
- Title: `Worktree Merge-Back — worktree_merge_requested Card, Resolve Endpoint, Conflict Evidence`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-71](../../07-Coding-Plan/todo/CP-71-Run-Worktree-Isolation.md) `P-4`, [SD-27](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md) `D-2/D-4/D-4c`, §6 contracts
- Child Documents: `None`
- Related Documents: [Task-408](./Task-408-Run-Worktree-Binding-And-Owner-Resolution.md), [Task-345](../done/Task-345-Decision-Card-Desktop-TUI.md) (card plumbing precedent)
- Replaces: `None`
- Tags: `worktree, merge, desktop, tui, runner, ux`
- Feature Keys: `run-worktree`

## AI Quick View

### Summary

- Two triggers, one contract (`D-4c`): flow runs emit `worktree_merge_requested` at terminal status; chat runs get a user-initiated "Merge worktree back" control + the same decision surfaced on chat delete.
- Card offers `apply_patch | keep_branch | discard` resolved via `POST /client/workflow-runs/{runId}/worktree/resolve`; conflict → `MergeConflictError` evidence card (paths + patch artifact); `discard` confirm lists untracked artifacts (SD-27 Q-2).
- Apply is serialized per repo, `git apply --check` pre-flight, all-or-nothing.

### Current Ask

- Implement the runner resolve endpoint + merge-pending transition + event emission, and the merge-back card on Desktop + TUI.

### Key Decisions

- `T-1` Terminal flow run with `worktreeState=active` → `merge_pending` + emit `worktree_merge_requested{runId, patchArtifactRef, diffStats}` (diff computed lazily; large patch → file artifact, `SS-23 E-6`).
- `T-2` `POST /client/workflow-runs/{runId}/worktree/resolve {mode}` — `apply_patch` (retryable: conflict → stays `merge_pending` + updated evidence), `keep_branch`, `discard` (confirm payload lists untracked files).
- `T-3` Chat delete with live binding → deletion flow surfaces the merge decision first (`SS-23 E-3`).

### Constraints

- Patch-based merge only — never `git merge`/`commit`/`push` in the user's repo (`BR-2`).
- Serialized per-repo apply + drift pre-check (SD-27 `D-4`); `--check` before real apply (`F-4`).
- Additive tests only; old suite green; provider-agnostic (Case-1).

### Open Questions

- `None`

### Source Refs

- `SD-27` §5 state machine, §6 contracts, §8 `F-3/F-4/F-5`; `SS-23 AC-4/AC-5`, `BR-2/BR-5`, `E-3/E-4/E-6`; `internal/worktree` (Task-407 `Resolve`/`Diff`); decision-card plumbing (Task-345 `user_decision_card_requested` → chat prompt channel).

## 1. Goal

Every worktree run ends with an explicit, evidenced, user-controlled merge-back — conflicts never lose work and never orphan the tree.

## 2. Parent Links

- coding plan: `CP-71 P-4`
- tech design: `SD-27 D-2, D-4, D-4c`, §6
- system spec: `SS-23 AC-4, AC-5, BR-2, BR-5, E-3..E-6`
- specific upstream ids: `Task-407` (Resolve/Diff), `Task-408` (binding), `Task-345` (card channel)

## 3. Trigger

Runs can be isolated (408); without merge-back the value never lands in the user's workspace.

## 4. Exact Change

- `T-1` Runner: `run_worktree_merge.go` — terminal-status hook sets `merge_pending` + emits event with patch artifact ref; `handleWorktreeResolve` endpoint.
- `T-2` Chat-delete path: intercept when `worktreeState ∈ {active, merge_pending}` → return/surface merge decision (`E-3`) before deleting.
- `T-3` Desktop: `WorktreeMergeCard.tsx` (3 actions, diff stats, conflict paths list, copy-branch button `Q-3`); "Merge worktree back" control in chat controls for chat runs.
- `T-4` TUI: merge card render + `keep_branch` checkout hint printed (`Q-3`).

## 5. Touched Areas

- files: `internal/runner/run_worktree_merge.go` (new), `interactive_handlers.go` (route+delete intercept), `flow_validate_audit_dispatch.go`/`interactive_service.go` (terminal hook), `desktop: WorktreeMergeCard.tsx` (new), `store.ts`, `HttpWsRunnerClient.ts` (`resolveWorktree`), TUI card render + keymap.
- modules: `internal/runner`, desktop UI, TUI.
- routes: `POST /client/workflow-runs/{runId}/worktree/resolve` (new).
- tables: none.

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/runner/run_worktree_merge.go (new)
func (s *InteractiveService) maybeEmitWorktreeMergeRequest(rs *interactiveRun)  // T-1 terminal hook
func (s *InteractiveService) resolveWorktree(ctx context.Context, runID, mode string) (*worktreeBinding, *apiErr) // T-2
func (s *InteractiveService) worktreeDeleteGate(rs *interactiveRun) *apiErr   // T-2/E-3
```

```go
// interactive_handlers.go
mux.HandleFunc("POST /client/workflow-runs/{runId}/worktree/resolve", s.handleWorktreeResolve)
// event: "worktree_merge_requested" {runId, patchArtifactRef, diffStats, conflictPaths?}
```

```ts
// apps/desktop-flowpilot/src/components/WorktreeMergeCard.tsx (new)
export function WorktreeMergeCard(props: {
  runId: string; diffStats: { files: number; added: number; removed: number };
  conflictPaths?: string[]; untracked?: string[]; // shown on discard confirm (Q-2)
  onResolve(mode: "apply_patch" | "keep_branch" | "discard"): Promise<void>;
}): JSX.Element

// HttpWsRunnerClient.ts
resolveWorktree(runId: string, mode: "apply_patch" | "keep_branch" | "discard"): Promise<void>
```

## 7. Test Signatures

- `TestWorktreeMerge_TerminalFlowEmitsCard` — flow done + `state=active` → `merge_pending` + event with artifact ref.
- `TestWorktreeMerge_ApplyPatchClean` — diff lands on main; state `merged`; worktree+branch removed.
- `TestWorktreeMerge_ConflictEvidence` — drifted main → `MergeConflictError`; state stays `merge_pending`; card payload has `conflictPaths` + patch ref; worktree preserved.
- `TestWorktreeMerge_RetryAfterConflict` — fix main → re-send `apply_patch` succeeds (no separate retry mode).
- `TestWorktreeMerge_KeepBranch` — branch `fp/<slug>` survives; state `kept_branch`.
- `TestWorktreeMerge_DiscardListsUntracked` — confirm payload enumerates untracked artifacts.
- `TestWorktreeMerge_ChatDeleteSurfacesDecision` — delete with live binding → merge prompt path, not silent GC.
- `TestWorktreeMerge_SerializedApplies` — two applies on one repo serialize; second re-checks drift.
- `test("WorktreeMergeCard renders three actions + conflict paths")`; `test("merge control visible only when worktreeState active/merge_pending")`.

## 8. Acceptance Check

- Vibe run finishes → card with diff stats → apply → changes in main, worktree gone.
- Conflict scenario → evidence card; worktree still inspectable; retry works after user fixes main.
- Chat: "Merge worktree back" control + delete-time decision both reach `resolve`.

## 9. Out of Scope

- `lost`-state recovery UX (Task-411); auto-conflict-resolution; remote push/PR.

## 10. Definition of Done

- [ ] §6 signatures landed; §7 tests exist, green, additive-only.
- [ ] Old suite untouched/green — failure → STOP + report.
- [ ] Provider parity: Case-1 agnostic (git ops only), grep evidence in CA.
- [ ] `feature_key: run-worktree`; CA ledger entry; `detect_changes` clean.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
