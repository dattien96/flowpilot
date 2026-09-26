# BUG-503 — API-launched workflow run without `cwd` binds the runner's own workspace, freezing an empty-baseline contract

## Status
FIXED + LIVE-VERIFIED — 2026-09-26, fixed build, runner on :19400.

- Fix: `createRun` resolves cwd in preference order explicit `in.Cwd` →
  bound project's registered `Project.Path` (only when the path exists on
  disk — a stale/deleted registration falls through) → runner workspace.
  Change-audit: CA-1004.
- Unit: `bug503_project_cwd_resolution_test.go` — project path wins,
  explicit cwd wins, unknown project keeps runner default, nonexistent
  registered path keeps runner default (4 tests, green).
- Live re-verify (fixed build): `run-23532` created with `projectId` bound
  to lt-full and no `cwd` → durable session row `working_directory:
  /Users/tiendat/fp-beds/full` (the registered bed path). The pre-fix
  sibling `run-23515` on the old binary bound `/tmp/fp-live-2026` — same
  launch shape, opposite binding.

## Live-found during
Full CP live-test rerun, 2026-09-26, build 8c95a5bb, runner on :19400.

- `run-15708` created via `POST /client/workflow-runs` with
  `{projectId: 957928cc (lt-full bed), workflowId: bug-harness}` and **no
  `cwd` field**.
- Run record persisted `workingDirectory: /tmp/fp-live-2026` — the runner
  process's own cwd — not the project's registered bed path
  (`/Users/tiendat/fp-beds/full`).
- Consequences observed live:
  - `preflight_contract_freeze` minted contracts (v1, v2) with **no
    `base_sha` and no `baseline_worktree`** — `baselineWorktreeFingerprint`
    ran against a non-git directory (`/tmp/fp-live-2026` has no `.git`).
  - Context package for the coder child reported every declared path
    `not_found` (`strutil2/pad.go: not_found` — looked under the runner dir).
  - `reproduce_test` child wrote `strutil2/pad.go` +
    `padleft_repro_test.go` into `/tmp/fp-live-2026/strutil2/` — polluting
    the runner's home, not the bed.
  - `implement` child's turn-end gate failed closed with
    `gate observation failed against the frozen contract's baseline
    (workspace diff unreadable): exit status 128` — `ObserveGitDiffSince(cwd,
    "")` on a non-git workspace. Unrecoverable: `amend` preserves the empty
    BaseSHA, every retry re-parks. Run had to be cancelled.

## Root cause

`createRun` (interactive_handlers.go ~915) defaults `in.Cwd` to
`s.runner.workspace` when empty (the BUG-455 fix). For an API call carrying
`projectId` but no `cwd`, there is no projectId→registered-root resolution on
the run-create path — `lookupProject` exists but is not consulted for cwd.
The desktop client always sends `cwd` explicitly, so this only bites raw-API
launchers.

Downstream, nothing fails fast: the freeze and gate machinery run git against
the wrong (non-git) workspace and escalate with messages that never name the
real problem ("invalid planner proposal", "exit status 128").

## Suggested fix direction

- `createRun`: when `projectId` is bound and `cwd` is empty, resolve the
  project's registered root via `lookupProject`; if unresolvable, reject the
  create (`400 cwd_required`) rather than silently binding runner cwd.
- Defense in depth: `baselineWorktreeFingerprint`/`freeze` should refuse a
  non-git workspace for a contract-bearing flow (freeze currently mints a
  baseline-less contract on a non-repo without complaint).

## Reproduction

```bash
curl -X POST :19400/client/workflow-runs -d \
  '{"projectId":"<id>","workflowId":"<bug-harness-id>"}'   # no cwd
# run record → workingDirectory = runner cwd
```

## Related
- BUG-455 (fixed CA-952): same create path, same default — the fix addressed
  `""` vs missing cwd for inline consumers; project-root resolution was out
  of scope.
