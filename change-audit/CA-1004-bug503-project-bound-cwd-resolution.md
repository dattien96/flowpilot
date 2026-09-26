# CA-1004 — BUG-503: project-bound run without cwd binds the registered project workspace

## Context

Live run-15708 (bug-harness launched via HTTP API with `projectId` but no
`cwd`): `createRun` fell back to the runner's own working directory
(`/tmp/fp-live-2026`) instead of the registered project's path. The flow
then ran against the wrong workspace — contract freeze saw no baseline SHA
and every declared path read `not_found`, producing an unrecoverable
frozen contract with an empty `base_sha`.

## Changes

- `InteractiveService.createRun` (`interactive_handlers.go`): cwd
  resolution order is now explicit `in.Cwd` → the bound project's
  registered `Project.Path` → runner workspace fallback. A project-bound
  run can no longer silently bind the runner's own directory.
- Amendment: the project path is bound only when it exists on disk
  (`os.Stat` + `IsDir`). A stale or deleted registration falls through to
  the runner-workspace default — binding a nonexistent path makes the
  post-turn gate's turn-scoped `git diff` fail closed (exit 128) and
  blocks every turn forever, the same unrecoverable class as the bug.
- Regression test `bug503_project_cwd_resolution_test.go`: asserts the
  project path wins when `in.Cwd` is empty and the project resolves,
  explicit `in.Cwd` still wins over the project path, an unknown project
  keeps the runner default, and a nonexistent registered path keeps the
  runner default.

## Verification

`go test -count=1 -run TestBug503 ./internal/runner/` — green.
Live re-verify: bug-harness run relaunched post-fix binds the lt-full bed.
