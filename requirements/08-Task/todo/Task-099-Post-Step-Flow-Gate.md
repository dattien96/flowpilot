# Task-099: Post-Step Flow Gate

## Metadata

- Document ID: `Task-099`
- Title: `Post-Step Flow Gate`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-100: Regression Suite And Oracle Rule](./Task-100-Regression-Suite-And-Oracle-Rule.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Replaces: `None`
- Tags: `flowgate, enforcement, runner, interactive-service`

## AI Quick View

### Summary

- Add a runner-owned gate that runs after each turn and forces required outputs (change-audit note, Task/BugFix doc).
- Hooked after `finishTurn` in `interactive_service.go` `runTurn`; works across all providers (no provider hooks needed).
- Actions: reprompt (bounded) → block → approve. Defaults to warn before enforce.

### Current Ask

- Implement `internal/flowgate/` (`rules`, `observe`, `evaluate`, `enforce`) per CP-35 §4.4.

### Key Decisions

- `T-1` Gate is post-`finishTurn`, runner-owned; provider hooks optional.
- `T-2` `TurnResult` = final message + tool calls + `git status` diff + test outcome; rules evaluated against it.

### Constraints

- `gate_mode` defaults to `warn`. Regression/oracle logic is Task-100. The `removed_referenced_code` rule needs Task-098.

### Open Questions

- Reprompt budget + which rules block vs warn by default (`SD-17 Q-2`).

### Source Refs

- `CP-35 §4.4` (P-4); `SD-17 D-6`, §3.5, §6.3, §7.1; `SS-14 AC-11`, `AC-7`.

## 1. Goal

A post-turn gate that observes the result and forces the step's required outputs before it can complete.

## 2. Parent Links

- coding plan: `CP-35` P-4
- tech design: `SD-17` `D-6`, §3.5, §6.3, §7.1
- system spec: `SS-14` AC-11, AC-7
- specific upstream ids: `P-4`, `D-6`, `AC-11`

## 3. Trigger

Context-in is handled before the call; the missing half is forcing required outputs after the AI responds. The runner mediates every provider, so it is the right enforcement point.

## 4. Exact Change

- `T-1` `rules.go` — `Rule` type + seed default rules to `settings/flow-rules.json`.
- `T-2` `observe.go` — hook after `finishTurn` in `runTurn`; build `TurnResult` (`git status --porcelain`, tool calls, final message).
- `T-3` `evaluate.go` — `Evaluate(tr, rules) []Violation` for `code_changed`, `bug_fixed` (tests/regression triggers in Task-100).
- `T-4` `enforce.go` — `reprompt` (bounded by `max_reprompt_attempts`) / `block` (withhold `RunStatusCompleted`) / `approve` (reuse SD-16 gate).
- `T-5` unit tests per trigger; integration: code change without CA note → reprompt then block.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/*`, `interactive_service.go` (`runTurn` hook)
- modules: `flowgate`, `interactive_service`
- routes: SSE violation event
- tables: `flow_rules` (mirror, CP-35 §6)

## 6. Acceptance Check

- CP-35 P-4 DoD: code change without a CA note → reprompt then block; gate runs for both Claude and Codex turns.
- `go test ./internal/flowgate/...` passes.

## 7. Out of Scope

- Regression-suite + oracle routing (Task-100); the structure-dependent rule (needs Task-098); behavioral drift (CP-23).

## 8. Completion Notes

- result: planned
- follow-ups: Task-100 adds the test/regression triggers on this framework
- upstream docs updated: none
