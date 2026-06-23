# Task-100: Regression Suite And Oracle Rule

## Metadata

- Document ID: `Task-100`
- Title: `Regression Suite And Oracle Rule`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-099: Post-Step Flow Gate](./Task-099-Post-Step-Flow-Gate.md), [CP-32: UnitTest Rule](../../07-Coding-Plan/done/CP-32-UnitTest-Rule.md)
- Replaces: `None`
- Tags: `flowgate, regression, oracle, tests, cp-32`

## AI Quick View

### Summary

- Make the existing test suite the primary regression signal: a previously-green test that breaks (and was not changed by the task) is a hard regression → block.
- Fix must restore green by correcting the new code, never by weakening the test. Delivers `CP-32`.
- Distinguishes pre-existing tests (regression) from task-authored tests (not-done-yet).

### Current Ask

- Implement `internal/flowgate/baseline.go` + `oracle.go` on top of the Task-099 gate, per CP-35 §4.5.

### Key Decisions

- `T-1` `regressed = baseline_green ∩ now_red ∩ {test file not in diff}` → `regression_test_broke` (block).
- `T-2` A pre-existing test edited in the same turn as code → flag for human review (possible oracle tampering).

### Constraints

- Depends on Task-099 (gate framework). No specs in repo → route to "ask the user," never auto-adjudicate.

### Open Questions

- `test_command` auto-detection coverage on polyglot repos (config override exists).

### Source Refs

- `CP-35 §4.5` (P-5); `SD-17 D-10`, `D-12`, §7.2; `SS-14 AC-6`, `AC-14`; delivers `CP-32`.

## 1. Goal

Block changes that break previously-passing tests, and forbid making tests pass by weakening them — re-checking SS/SD (or asking the user) instead.

## 2. Parent Links

- coding plan: `CP-35` P-5
- tech design: `SD-17` `D-10`, `D-12`, §7.2
- system spec: `SS-14` AC-6, AC-14
- specific upstream ids: `P-5`, `D-12`, `AC-14`; `CP-32`

## 3. Trigger

The strongest, cheapest regression signal is "old test now fails" — it needs no code graph and directly addresses the regression pain.

## 4. Exact Change

- `T-1` `baseline.go` — `CaptureBaseline()` at `startTurn`; `test_command` detection (`go test` / `npm test` / `pytest`); write `.flowpilot/guard/test_baseline.json`.
- `T-2` `oracle.go` — re-run after change; compute `regressed`; emit `regression_test_broke` (**always block**, not gate_mode-gated — `SS-14 Q-1`) with the regressed list.
- `T-3` test-file detection in diff; flag pre-existing test edits; no-spec → ask user, specs → re-check AC (`SS-13 §8.3`).
- `T-4` unit tests: `regressed` formula, test-file detection, no-spec routing.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/baseline.go`, `oracle.go`
- modules: `flowgate`
- routes: SSE regression event
- tables: none new

## 6. Acceptance Check

- CP-35 P-5 DoD: breaking a pre-existing green test → blocked with the regressed list; pre-existing test edit flagged; no-spec repo asks the user.
- `go test ./internal/flowgate/...` passes; closes `CP-32`.

## 7. Out of Scope

- The gate framework itself (Task-099); behavioral/progress drift (CP-23); symbol-level analysis.

## 8. Completion Notes

- result: planned
- follow-ups: when done, update `CP-32` status to reflect delivery
- upstream docs updated: none (will mark `CP-32` on completion)
