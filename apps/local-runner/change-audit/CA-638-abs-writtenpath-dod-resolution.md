# CA-638 — Absolute WrittenPaths Resolve Inside Workspace (BUG-545)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-545

---

## Problem

Live `run-34296` / `run-34322` (`:4322`): provider file-change events emit
**absolute** paths (e.g.
`/private/tmp/fp-live5/.flowpilot/worktrees/candidate-candidate-b/requirements/08-Task/done/Task-06.md`).
`MissingDodDocs` rejected every absolute path outright via
`filepath.IsAbs(clean)` → appended to missing — even when the path resolved
inside `WorkspaceCwd` and the file contained a valid `## Definition of Done`
checklist. Result: the DoD gate flagged a compliant document on every
evaluation, the reprompt budget drained, and the member escalate-looped —
a permanent wedge no agent action could clear.

## Fix

`internal/flowgate/dod.go` — `MissingDodDocs` now resolves an absolute
candidate with `filepath.EvalSymlinks` and accepts it **only** when it lands
inside the resolved workspace root; out-of-root and traversal escapes keep
the fail-closed rejection. Valid in-root paths are then read and validated
normally.

## Tests

- `internal/flowgate/bug545_abs_writtenpath_dod_test.go` (red first):
  - absolute path inside workspace + valid DoD → not missing;
  - absolute path outside workspace → still missing;
  - absolute path inside workspace without DoD → still missing.
- `go test -count=1 ./internal/flowgate/` green.

## Live evidence

Post-fix binary on `:4322`: candidate-b's next gate evaluation dropped the
`MissingDodDocs` violation for the same absolute `Task-06.md` path; the step
advanced `candidate-b → DONE` (run-34296 flow-events, 12:42:48Z).

## Provider parity

Provider-agnostic: the change is path-resolution inside the DoD gate; any
provider emitting absolute `WrittenPaths` benefits, relative-path providers
are untouched.
