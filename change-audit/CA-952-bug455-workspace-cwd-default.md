---
id: CA-952
title: BUG-455 — default run workspaceCwd to runner workspace (create + resume heal)
feature_key: local-runner
date: 2026-09-24
status: landed
---

## Summary

A run created via the API with a bound project but no explicit `cwd` recorded
`interactiveRun.workspaceCwd=""`. Provider children still ran in
`Runner.workspace` (sessions.go fallback), so work landed in the real project
dir, but every Go-inline consumer of `rs.workspaceCwd` — command.validate's
`loadValidateCommand`, `captureGitHead`, contract-scope prompt injection,
handoff, gate hooks — saw an empty workspace. Live run-94 dead-parked at
`validate`: `skipped_no_command` escalate whose documented resolutions
(configure `test_baseline.json`, or Continue) could never succeed because
`loadValidateCommand("")` returns before reading the baseline.

## Change

- `internal/runner/interactive_handlers.go` (`createRun`): when `in.Cwd` is
  empty, default it to `s.runner.workspace` (nil-guarded) — the same fallback
  `sessions.go` and `worktreeRepoDir` already apply. Runs before indexing/
  knowledge-base kickoff so those also target the real dir.
- `internal/runner/interactive_resume.go` (`reconstructRunInternal`): same
  default on resume so pre-fix durable records heal instead of re-trapping.

## Evidence

- Repro: live run-94, validate step stuck `WAITING_USER_APPROVAL` with two
  `flow_validation_retry` `skipped_no_command` events; `.flowpilot/guard/
  test_baseline.json` present and valid.
- Tests: `internal/runner/bug455_workspace_cwd_default_test.go` — 5 additive
  tests (create default, explicit-cwd wins, baseline e2e, resume heal,
  persisted-cwd preserved).
- Focused regression: createRun/resume/reconstruct/worktree/BUG-378/405-409/
  run-207435 suites green.

## Notes

- Explicit `in.Cwd` and persisted `WorkingDirectory` remain authoritative.
- Gemini `workspace_required` guard unaffected when `s.runner == nil`
  (test path) and still correct in production (default supplies a real dir).
- Live verification: restart runner → resume run-94 → Continue → validate
  runs the real `go test ./...` (tracked in CP-Full-Live-Test.md).
