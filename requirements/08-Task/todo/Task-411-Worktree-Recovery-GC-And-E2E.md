# Task-411: Worktree Recovery, GC & E2E Hardening (CP-71 P-5)

## Metadata

- Document ID: `Task-411`
- Title: `Worktree Recovery & GC — lost State, Boot Prune, Deletion Gate, E2E`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-71](../../07-Coding-Plan/todo/CP-71-Run-Worktree-Isolation.md) `P-5`, [SD-27](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md) `D-7`, §8 F-2/F-5
- Child Documents: `None`
- Related Documents: [Task-408](./Task-408-Run-Worktree-Binding-And-Owner-Resolution.md), [Task-410](./Task-410-Worktree-Merge-Back-UX.md), [CP-51](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-And-Recovery-Consistency.md)
- Replaces: `None`
- Tags: `worktree, recovery, gc, e2e, runner`
- Feature Keys: `run-worktree`

## AI Quick View

### Summary

- Close the lifecycle: `resumeRun`/reattach/history-open runs `D-7` validation on the binding (`git worktree list --porcelain` + dir + `.base` sidecar); external deletion → `worktreeState=lost` + notice card (`archive chat` | `recreate empty from base`) — never silent recreate/duplicate.
- New legs in a `lost` chat are blocked behind the notice (`SS-23 E-8`); chat-delete flows through the merge decision first (`F-5`/`E-3`); boot GC prunes only worktrees of deleted/corrupt owners — never `merge_pending` or resumable ones.
- E2E: concurrent vibe runs on overlapping files → sequential merge, second conflicts → evidence card.

### Current Ask

- Implement validation-on-resume, `lost` transitions + notice, boot GC scan, and the E2E coverage.

### Key Decisions

- `T-1` Validation lives in the shared resume path (`resumeRun` + history-open rehydrate) — one checkpoint for chat and flow.
- `T-2` `lost` notice resolved via `archive_chat` (retain run, mark `lost`, no worktree ops) or `recreate_empty` (`Manager.Create` from recorded `baseCommit`, state `active`, explicit "prior changes lost" card text).
- `T-3` Boot GC: scan `.flowpilot/worktrees/` + `git worktree list --porcelain`; prune entries whose owner has no live/deleted-chat reference AND whose state ∉ {active, merge_pending, resumable}; log every prune to CA ledger.

### Constraints

- Never silently recreate a missing worktree; never GC a `merge_pending`/resumable owner (`SS-23 AC-6/AC-7`, `E-8`).
- CP-51 recovery contract preserved — binding survives restart.
- Additive tests only; old suite green.

### Open Questions

- `None`

### Source Refs

- `SD-27 D-7`, §8 F-1/F-2/F-5; `SS-23 AC-1b/AC-6..AC-8`, `E-2b/E-3/E-8`; `interactive_resume.go:888` (`workspaceCwd` rehydrate point); `interactive_handlers.go` `resumeRun`; Task-369 no-orphan semantics.

## 1. Goal

A worktree binding is trustworthy across restart, external deletion, and deletion/GC edges — with evidence and explicit user decisions at every risky transition.

## 2. Parent Links

- coding plan: `CP-71 P-5`
- tech design: `SD-27 D-7`, §8
- system spec: `SS-23 AC-1b, AC-6, AC-7, AC-8, E-2b, E-3, E-8`
- specific upstream ids: `CP-51` (recovery contract), `Task-407/408/410`

## 3. Trigger

The final slice: everything before this assumes bindings stay valid; this closes the failure envelope and proves concurrency safety end-to-end.

## 4. Exact Change

- `T-1` `resumeRun` + history-open rehydrate → `validateWorktreeBinding` (`Manager.Validate`); fail → `state=lost` + `worktree_lost` notice event; new leg start in `lost` chat → blocked behind notice (`E-8`).
- `T-2` `lost` resolution endpoint/handling: `archive_chat` | `recreate_empty` (recreate from `baseCommit`, warn prior changes gone).
- `T-3` Boot GC: `sweepOrphanedWorktrees` at service init — orphan detection rules per `T-3` above; dry-run logging first pass, then prune.
- `T-4` Chat-delete integration point confirmed with Task-410 gate ordering (decision → delete → GC-eligible).

## 5. Touched Areas

- files: `internal/runner/run_worktree_lifecycle.go` (new), `interactive_resume.go`, `interactive_handlers.go`, `interactive_service.go` (boot hook).
- modules: `internal/runner`, `internal/worktree` (Validate consumer).
- routes: `POST /client/workflow-runs/{runId}/worktree/lost/resolve` (new, small) or folded into `/worktree/resolve` modes — decided at implementation, documented here if folded.
- tables: none.

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/runner/run_worktree_lifecycle.go (new)
func (s *InteractiveService) validateWorktreeBinding(b *worktreeBinding) bool   // D-7: git worktree list + dir + .base
func (s *InteractiveService) markWorktreeLost(rs *interactiveRun, cause string) // emits worktree_lost notice
func (s *InteractiveService) resolveLostWorktree(ctx context.Context, runID, mode string) *apiErr // archive_chat | recreate_empty
func (s *InteractiveService) sweepOrphanedWorktrees(ctx context.Context)        // T-3 boot GC, CA-logged
```

## 7. Test Signatures

- `TestWorktreeResume_RehydratesValidBinding` — restart → binding intact, cwd still worktree.
- `TestWorktreeResume_ExternalDeleteMarksLost` — `rm -rf` worktree → resume → `lost` + notice; no recreate.
- `TestWorktreeResume_LegInLostChatBlocked` — new leg on `lost` chat → blocked behind notice (`E-8`).
- `TestWorktreeLost_ArchiveChat` — `archive_chat` keeps run, `state=lost`, no worktree ops.
- `TestWorktreeLost_RecreateEmpty` — new worktree at `baseCommit`, state `active`, prior changes absent (warn surfaced).
- `TestWorktreeGC_PrunesOnlyOrphans` — deleted/corrupt-owner dirs pruned; `active`/`merge_pending`/resumable untouched.
- `TestWorktreeGC_LogsEveryPrune` — CA ledger entries written.
- `TestE2E_TwoVibeRunsSequentialMerge` — overlapping files → run A applies, run B conflicts → evidence card; no lost work.
- `TestE2E_ConcurrentRunsSameProject` — two chat owners, isolated dirs, no cross-talk.

## 8. Acceptance Check

- Kill runner mid-vibe-run, restart → run resumes in worktree.
- `rm -rf` the worktree, reopen chat → `lost` notice with the two explicit choices.
- Delete a merge-pending chat → decision surfaced first; GC only after confirmation.
- E2E concurrent merge behaves per `AC-5`.

## 9. Out of Scope

- Auto-heal `lost` worktrees (restore partial diffs); admin-web GC surface; remote-branch GC.

## 10. Definition of Done

- [ ] §6 signatures landed; §7 tests exist, green, additive-only.
- [ ] Old suite untouched/green — failure → STOP + report.
- [ ] Provider parity: Case-1 agnostic, grep evidence in CA.
- [ ] `feature_key: run-worktree`; CA ledger entry; `detect_changes` clean.
- [ ] CP-71 moved toward `done/` once all P-slices green + CA updated.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
