# Task-095: Gate Legacy Keyword Loop

## Metadata

- Document ID: `Task-095`
- Title: `Gate The Legacy Keyword Loop In Explicit Mode`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-090: Review Loop Driver](./Task-090-Review-Loop-Driver.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, regression-guard, go`

## AI Quick View

### Summary

- Gate the legacy single-reviewer keyword loop (`strings.Contains(msg, "changes requested")`) so it runs ONLY when `loopState.Mode != "explicit"`.
- In explicit mode, a reviewer completing contributes only to the cohort note (Task-091) and never directly transitions the loop or restarts the coder — preventing N reviewers from racing N coder restarts.

### Current Ask

- Add the mode guard and prove that explicit mode produces zero direct coder restarts while keyword mode is byte-for-byte unchanged.

### Key Decisions

- `T-1` `submit_review_outcome` is the sole loop driver in explicit mode (SD-18 `D-2`); the keyword path is legacy-only.

### Constraints

- Run GitNexus impact analysis before editing the reviewer completion branch (`interactive_service.go:884-935`); warn on HIGH/CRITICAL.
- Legacy keyword behavior MUST be unchanged when `Mode != "explicit"` (regression guard).

### Open Questions

- None.

### Source Refs

- CP-36 `P-7`; SD-18 `D-2`, §8 `F-4`; SS-15 `AC-7`, `AC-11`.
- Anchors: `interactive_service.go:880-935` (turn-completed coder/reviewer keyword logic).

## 1. Goal

In explicit mode the loop is driven only by `submit_review_outcome`; the legacy keyword loop remains intact for runs that never enter explicit mode.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-7`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-2`
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-7`, `AC-11`
- specific upstream ids: CP-36 `P-7`

## 3. Trigger

With multiple reviewers, the existing keyword branch would fire once per reviewer and trigger competing coder restarts. Explicit mode must own the loop; the keyword path must be gated to preserve `AC-7` and `AC-11`.

## 4. Exact Change

- `T-1` Wrap the reviewer keyword logic (`interactive_service.go:884-935`) in `if loopState.Mode != "explicit" { … }`.
- `T-2` In explicit mode, a reviewer's `EventTurnCompleted` only feeds the cohort buffer (Task-091) and emits status; it does not call `transition`, `advanceRound`, or restart the coder.

## 5. Touched Areas

- files: `interactive_service.go` (reviewer completion branch).
- modules: local-runner interactive service.
- routes: none.
- tables: none.

## 6. Acceptance Check

- Unit: in explicit mode, two reviewers completing produce ZERO coder restarts (only the cohort note + reinvoke); in keyword mode the behavior is byte-for-byte unchanged.
- Regression: the existing single coder↔reviewer loop test still passes.

## 7. Out of Scope

- The explicit driver itself (Task-090); the cohort note (Task-091); auto-reinvoke (Task-092).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
