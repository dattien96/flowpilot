# CA-962 — TUI /wt-merge worktree merge-resolution command

## Summary

Task-437. The TUI showed `wt:merge_pending` on the badge but had no way to
resolve it — no client method, no command, no decision handling. A user in
the terminal had to switch to desktop or curl the API.

Adds `/wt-merge <mode> [--confirm]` (alias `/wtm`, also `/wt merge <mode>`):

- No-arg probes `GET /client/workflow-runs/{id}` and prints the live binding
  state + usage.
- With a mode it POSTs `/client/workflow-runs/{id}/worktree/resolve`
  `{mode, confirm}`.
- A `409 requiresConfirm` body is rendered as the file list at risk +
  the `/wt-merge <mode> --confirm` retry hint (never auto-confirms).
- A `409 conflict` body is rendered fail-closed: conflict paths +
  patchArtifactRef + "fix in main workspace, then re-run" — never
  force-applies.
- Success stamps `noteWorktreeBinding(worktreeState, slug)` so the badge
  tracks merged/kept_branch/discarded.

Client side: `ResolveWorktree` + `WorktreeResolveResult` +
`RunSnapshot.Worktree` (nested binding echo), and `APIError.Details` now
carries the full response body — symmetric with the desktop
`RunnerApiError.details` (CA-960). Wire-truth: evidence 409 bodies have no
`code` field, so conflict detection keys off `Details["conflict"]` (code
kept for compatibility).

## Changes

- `internal/tui/client/client.go` — `APIError.Details`, `RunWorktreeInfo`,
  `RunSnapshot.Worktree`, `WorktreeResolveResult`, `ResolveWorktree`,
  `parseAPIError` preserves the raw body.
- `internal/tui/app/worktree_toggle.go` — `cmdWorktreeMerge`,
  `handleWorktreeMergeMsg`, `anyStrings`.
- `internal/tui/app/app.go` — `/wt-merge`, `/wtm`, `/wt merge` dispatch +
  `worktreeMergeMsg` Update case.
- `internal/tui/app/model.go` — `worktreeMergeMsg` type + `/wt-merge` in
  `knownSlashCommands`.

## Tests (additive)

- `internal/tui/client/worktree_resolve_test.go` — endpoint+body, confirm
  flag on the wire, 409 evidence Details (requiresConfirm + conflict shapes).
- `internal/tui/app/worktree_merge_test.go` — requires-run guard, bad-mode
  rejection, resolve success updates badge, no-arg binding probe,
  requiresConfirm file-list hint, fail-closed conflict render, `--confirm`
  reaches the wire.

Provider-agnostic: git/worktree state only, no provider adapter path.
