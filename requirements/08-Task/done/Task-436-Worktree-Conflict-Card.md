# Task-436 — Worktree Merge Card: Inline Conflict State

- Document ID: `Task-436`
- Title: `Worktree Merge Conflict Card`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `CP-71 (run-worktree), CP-84 (attention-queue)`
- Child Documents: ``
- Related Documents: `Task-435 (confirm gate), CA-954 (patchArtifactRef)`
- Replaces: ``
- Tags: `desktop-ui, worktree, attention-queue, conflict-ux`

## AI Quick View

### Summary

- When `apply_patch` hits a real merge conflict, the runner returns
  `409 worktree_merge_conflict` with `conflictPaths` + `patchArtifactRef` —
  but the desktop only shows a bare error toast, so the user cannot see
  which files conflict or where the patch artifact is.
- Task-435 added the confirm-state pattern; this task reuses it for the
  conflict state.

### Current Ask

- Surface `worktree_merge_conflict` 409s as an in-card conflict state:
  conflicted paths + patch artifact ref + Cancel/Retry — retry resends
  the same mode (no confirm flag needed).

### Key Decisions

- `T-1` Same queue-projection pattern as `worktreeConfirm` — separate
  `worktreeConflict` field + map so a confirm and a conflict cannot
  coexist ambiguously; each clears the other when set.
- `T-2` Retry resends `onAct(mode)` without `confirm` — conflict retry is
  the identical call; the user fixed the files in between.
- `T-3` `details.conflictPaths` / `patchArtifactRef` already ride the
  `RunnerApiError.details` channel added in Task-435.

### Constraints

- Additive-only; `requiresConfirm` handling untouched.
- Conflict state is per-run and cleared on evict/dismiss like confirm.

### Source Refs

- `SS-23 retry contract`, `Task-435`, `CA-954`

## 4. Exact Change

- `T-1` `attentionQueue`: `WorktreeConflict{mode, conflictPaths, patchRef, message}` +
  `worktreeConflicts` map + `setWorktreeConflict`/`clearWorktreeConflict` +
  overlay + evict clear; `setWorktreeConfirm` clears any conflict and vice versa.
- `T-2` `store.submitAttentionDecision` catch: `code==="worktree_merge_conflict"` →
  `setWorktreeConflict` + return true; add `dismissWorktreeConflict`.
- `T-3` `decisionControlModel`: `worktree_conflict` model (mode, conflictPaths,
  patchRef, message); `DecisionControls` renders paths + ref + Cancel/Retry.
- `T-4` `AttentionInbox` wires `onDismiss` to clear both states.

## 7. Test Signatures

- `test("worktree conflict 409 flips card to conflict state")` — paths+ref captured (T-3)
- `test("worktree conflict retry resends same mode")` — `apply_patch` resent (T-2)
- `test("dismissWorktreeConflict restores mode buttons")` — model back to `worktree` (T-1)
- `test("confirm and conflict states are mutually exclusive")` — latest set wins (T-1)
- `test("worktree conflict detected on wire shape (no code field)")` — wire
  body carries `conflict:true` not `code` (T-3)

## 10. Definition of Done

- [x] §7 tests exist, green, additive-only
- [x] Pre-existing inbox tests still green
- [x] CA ledger entry written — `CA-961`

## 11. Completion Notes

- result: apply_patch conflict 409 flips the inbox card to a conflict state
  — message, ≤5 conflict paths + overflow, patch artifact ref, Cancel/Retry
  (retry resends same mode, no confirm). Confirm and conflict are mutually
  exclusive. Wire-shape fix: the runner writes the evidence map bare (no
  `error.code`), so detection keys off `details.conflict === true` and the
  card message prefers `details.reason`.
- follow-ups: none
- upstream docs updated: CA-961
