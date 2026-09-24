# Task-437 — TUI `/wt-merge` Command: Worktree Resolve Surface

- Document ID: `Task-437`
- Title: `TUI Worktree Merge Resolve Command`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `CP-71 (run-worktree)`
- Child Documents: ``
- Related Documents: `Task-435 (desktop confirm), CA-958 (confirm gate)`
- Replaces: ``
- Tags: `tui, worktree, resolve-ux`

## AI Quick View

### Summary

- The TUI has `/worktree` to arm isolation and a `wt:<state>` badge, but
  **no way to resolve a merge decision** — when a run reaches
  `merge_pending` (or `lost`), a TUI-only user is stuck; the only resolve
  path is the desktop inbox card or raw HTTP.
- Worse: `/wt` refuses to disarm while a live binding exists ("Merge or
  discard the worktree first") — pointing at an action the TUI cannot do.

### Current Ask

- `client.ResolveWorktree` + `/wt-merge <mode> [--confirm]` slash command:
  no-arg prints the live binding state (fetched fresh) + usage;
  409 `requiresConfirm` prints the file list and instructs `--confirm`;
  409 `worktree_merge_conflict` prints conflictPaths + patchArtifactRef;
  success updates the badge via `noteWorktreeBinding`.

### Key Decisions

- `T-1` Slash command, not a card — TUI worktree surface is minimal by
  design; a command matches existing conventions and covers all five
  modes including `archive`/`recreate_empty` for lost bindings.
- `T-2` Explicit `--confirm` flag mirrors the HTTP contract — informed
  consent, no hidden retry.
- `T-3` `APIError.Details` (map) carries the 409 body fields — symmetric
  with the desktop `RunnerApiError.details` added in Task-435.
- `T-4` `/wt-merge` fetches `GetRun` for the current binding state — the
  badge (`noteWorktreeBinding`) is only stamped at start/resume, so the
  command must not trust it for truth.

### Constraints

- Additive-only; does not change `/worktree` toggle semantics.
- Requires `m.runHandle` — no run → usage error.

### Source Refs

- `SS-23 AC-10`, `Task-435`, `CA-958`

## 4. Exact Change

- `T-1` `client.go`: `WorktreeResolveResult` struct (worktreeState, branch,
  applied, requiresConfirm, uncommitted, untracked, conflict, conflictPaths,
  patchArtifactRef, reason) + `ResolveWorktree(ctx, runID, mode, confirm)`
  returning the typed body; `APIError` gains `Details map[string]any` filled
  by `parseAPIError`.
- `T-2` `app.go`: `/wt-merge` + `/wtm` slash command →
  `cmdWorktreeMerge(args)` → async `worktreeMergeMsg{result|err}` → Update
  handler prints outcome + `noteWorktreeBinding`.
- `T-3` 409 branches: `requiresConfirm` → file list + `--confirm` hint;
  `worktree_merge_conflict` → conflictPaths + patchArtifactRef.

## 7. Test Signatures

- `TestTUI_WtMergeNoArgProbesBinding` — GET probe prints binding state + usage (T-4)
- `TestTUI_WtMergeResolvesAndUpdatesBadge` — client called, badge updated to merged (T-2)
- `TestTUI_WtMergeConfirmHintListsFiles` — 409 requiresConfirm → file list + hint (T-3)
- `TestTUI_WtMergeConflictFailClosed` — 409 conflict → paths + patch ref (T-3)
- `TestTUI_WtMergeConfirmFlagReachesWire` — `--confirm` → confirm:true on the wire (T-2)
- `TestTUI_WtMergeRequiresRun` — no handle → error message
- `TestTUI_WtMergeRejectsBadMode` — unknown mode → usage, no request
- `TestClient_ResolveWorktreePostsModeAndConfirm` — endpoint + body shape
- `TestClient_ResolveWorktree409EvidenceBodySurvives` — Details carries requiresConfirm
- `TestClient_ResolveWorktree409ConflictDetails` — Details carries conflict fields

## 10. Definition of Done

- [x] §7 tests exist, green, additive-only
- [x] `internal/tui/...` suite still green
- [x] CA ledger entry written — `CA-962`

## 11. Completion Notes

- result: `/wt-merge <mode> [--confirm]` (+`/wtm`, `/wt merge`) resolves a
  worktree binding from the TUI. No-arg GETs the live binding; 409
  requiresConfirm lists at-risk files and names the `--confirm` retry; 409
  conflict prints conflictPaths + patchArtifactRef fail-closed; success
  re-stamps the badge. `APIError.Details` carries the raw 409 body
  (symmetric with desktop `RunnerApiError.details`). Wire-truth fix folded
  into both sides: evidence 409 bodies carry `conflict:true`, not `code` —
  desktop + TUI detect on details first.
- follow-ups: none
- upstream docs updated: CA-961 (desktop conflict card + wire-shape fix),
  CA-962 (this change)
