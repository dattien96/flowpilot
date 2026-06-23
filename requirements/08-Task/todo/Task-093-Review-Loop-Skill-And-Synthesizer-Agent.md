# Task-093: Review Loop Skill And Synthesizer Agent

## Metadata

- Document ID: `Task-093`
- Title: `agent-review-loop Skill And synthesizer Built-In`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-081: Agent Abstraction And Catalog Loader](../done/Task-081-Agent-Abstraction-And-Catalog-Loader.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, skill, agent-catalog`

## AI Quick View

### Summary

- Ship the `agent-review-loop` skill that defines the main agent's protocol: spawn coder, spawn the reviewer cohort, synthesize on re-prompt, submit a verdict, loop until clean or ask the user at the cap.
- Add a `synthesizer` built-in agent as an optional offload for synthesis.

### Current Ask

- Author the skill (the agent-facing protocol) and add the built-in so the loop behaves consistently across providers on first run.

### Key Decisions

- `T-1` Default cohort = two reviewers (`correctness`, `security`); add `regression` when the change touches tested behavior (SD-18 `D-8`).
- `T-2` On `blocked` the agent calls `ask_user` (Extend +2 / Accept / Stop); Extend is bounded (SD-18 `D-9`); the skill states the cap explicitly so the agent never loops unbounded.

### Constraints

- The skill must keep sub-agents isolated (only results shared) and must not auto-approve child gates (SS-15 `BR-1`, `BR-6`).
- The `synthesizer` built-in is overridable by an on-disk `.claude/agents`/`.codex/agents` file of the same name (SD-16 `D-5`).

### Open Questions

- Default vs main-agent synthesis is settled (main agent default, SD-18 `D-1`); the `synthesizer` child is optional.

### Source Refs

- CP-36 `P-5`; SD-18 `D-1`, `D-8`, `D-9`, §7; SS-15 `US-1`…`US-5`, `BR-1`…`BR-10`.
- Anchors: `agent_catalog.go:391` (`builtinAgentDefinitions`); existing skills under `.claude/skills/`.

## 1. Goal

A user instruction to "review until clean" causes the main agent to follow a defined, bounded protocol producing the SS-15 behavior, with a `synthesizer` built-in available for offload.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-5`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-1`, `D-8`, `D-9`
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `US-1`…`US-5`
- specific upstream ids: CP-36 `P-5`

## 3. Trigger

The runner primitives (Tasks 089–092) need an agent-facing protocol so the main agent uses them consistently; without a skill the behavior depends on ad-hoc prompting.

## 4. Exact Change

- `T-1` Add the `synthesizer` built-in to `builtinAgentDefinitions()` (role `synthesizer`, read-only tools, prompt: dedup + resolve conflicts + call `submit_review_outcome`; never restart the coder).
- `T-2` Author `.claude/skills/agent-review-loop/SKILL.md` (+ `.codex` mirror if the project keeps separate trees) covering: spawn coder (`wait=true`); spawn the default two-lens cohort (`wait=false`, `dependsOn:[coderRunId]`, shared `reviewCohortId`, `autoOrchestrate:true`); end the turn; on the consolidated note, synthesize and call `submit_review_outcome`; on `blocked` call `ask_user` (Extend +2 bounded / Accept / Stop); state the round cap explicitly.
- `T-3` Document the lens set as skill config so it is tunable without a code change.

## 5. Touched Areas

- files: `agent_catalog.go`; `.claude/skills/agent-review-loop/SKILL.md` (+ optional `.codex` mirror).
- modules: agent catalog; skill content.
- routes: none.
- tables: none.

## 6. Acceptance Check

- Catalog test: `synthesizer` is discoverable and overridable by a same-named on-disk file.
- The skill lints against the project skill format.
- Manual: following the skill, the main agent runs the full loop (coder → 2 reviewers → synthesis → verdict → loop) and asks the user at the cap.

## 7. Out of Scope

- Runner primitives (Tasks 089–092); UI (Task-094); a one-click UI affordance (deferred, SS-15 `Q-4`).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
