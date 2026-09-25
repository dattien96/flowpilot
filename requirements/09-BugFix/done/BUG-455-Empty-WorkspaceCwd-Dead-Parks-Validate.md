# BUG-455: Empty run workspaceCwd dead-parks command.validate (skipped_no_command forever)

- status: done
- found: live run-94 (post-rebase consolidated live test, task-harness on devin/swe-2-high)
- fixed_by: CA-952
- tests: internal/runner/bug455_workspace_cwd_default_test.go

## Symptom (live)

Run-94 (task-harness, bound project `957928cc` = `/Users/tiendat/fp-beds/full`,
started via `POST /client/runs` without `cwd`) progressed correctly through
plan loop → Task-325 plan-approval park → approve → freeze → test_signatures →
implement. At `validate` (command.validate) the flow escalated
`skipped_no_command` and parked `WAITING_USER_APPROVAL`.

The escalate summary told the operator to either configure
`.flowpilot/guard/test_baseline.json` or Continue after manual verification.
Neither worked:

- Baseline file written → Continue → `skipped_no_command` again.
- `loadValidateCommand("")` early-returns "" before ever reading the baseline.

Dead park: every Continue re-runs validate → same escalate → re-park.

## Root cause

`StartRunInput.Cwd` is documented as "per-run/per-thread cwd is authoritative;
Runner.workspace is only a default", and two paths already implement that
default:

- `sessions.go` provider session start: `req.WorkingDirectory == ""` →
  `r.workspace`
- `run_worktree.go` `worktreeRepoDir`: `in.Cwd` → `s.runner.workspace`

But `interactiveRun.workspaceCwd` was assigned `in.Cwd` verbatim at createRun
(interactive_handlers.go). A project-bound run started without `cwd` therefore
recorded `workspaceCwd=""`, while its provider children still ran in
`r.workspace` (files landed in the real bed — masking the problem).

Every Go-inline consumer reads `rs.workspaceCwd` via `workspaceCwdFor` with no
fallback: `loadValidateCommand`, `captureGitHead`, `appendChangeContractIfAny`,
handoff, gate hooks, artifact registry. All silently degrade; validate's is
the only one that surfaces as a hard escalate, and its documented resolutions
are both no-ops under `cwd==""`.

## Fix

- `createRun` (interactive_handlers.go): when `in.Cwd` trims empty, default to
  `s.runner.workspace` (nil-guarded) before any consumer sees it — also makes
  `ensureGitNexusIndexAsync` / `ensureKnowledgeBaseForWorkspace` target the
  real workspace.
- `reconstructRunInternal` (interactive_resume.go): heal the same field on
  resume (`st.WorkingDirectory == ""` → runner workspace) so pre-fix durable
  records do not re-enter the dead park after restart.

Explicit `in.Cwd` / persisted `WorkingDirectory` always win.

## Tests (all additive)

- `TestBug455_CreateRunDefaultsWorkspaceCwd` — create-time default (red before fix).
- `TestBug455_ExplicitCwdStillWins` — caller cwd authoritative.
- `TestBug455_ValidateFindsBaselineAfterCwdDefault` — e2e: baseline file in
  workspace → `loadValidateCommand` resolves `go test ./...`.
- `TestBug455_ResumeHealsEmptyWorkspaceCwd` — persisted-empty record heals.
- `TestBug455_ResumeKeepsPersistedCwd` — persisted dir never overwritten.
