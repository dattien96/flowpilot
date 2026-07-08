# Task-094: agent-review-loop Skill And synthesizer Built-in

## Metadata

- Document ID: `Task-094`
- Title: `agent-review-loop Skill And synthesizer Built-in (First FlowDefinition Template)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-091: Review-Loop Template](./Task-091-Review-Loop-Template.md), [Task-093: Bounded Auto-Reinvocation Of The Hub](./Task-093-Bounded-Auto-Reinvocation-Of-The-Hub.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, skill, builtin-agent, flow-definition, go`

## AI Quick View

### Summary

- Ship the **agent-facing protocol** for the review loop as the engine's first FlowDefinition template: a `.claude/skills/agent-review-loop/SKILL.md` (the main-agent playbook) and a `synthesizer` built-in agent.
- The skill spawns `coder` (`wait=true`), then a reviewer cohort (`correctness`+`security`, `wait=false`, `dependsOn:[coderRunId]`, shared `flowCohortId`, `autoOrchestrate:true`), ends the turn, and on the join note synthesizes and calls `submit_review_outcome`; on `blocked` it calls `ask_user` (Extend +2 / Accept / Stop).
- `synthesizer` is an optional offload built-in that consolidates and emits the verdict but never restarts the coder.

### Current Ask

- Add the `synthesizer` built-in and author the `agent-review-loop` skill (+ optional `.codex` mirror), with a catalog test and skill lint, and document the manual scenario.

### Key Decisions

- `T-1` Synthesis is performed by the **main agent** by default; `synthesizer` is an optional offload (SS-15 `Q-1`, SD-18 `D-1`).
- `T-2` Default lenses are `correctness`+`security`; the skill adds `regression` when the change touches tested behavior; lens set is skill config, minimum two (SS-15 `BR-9`, SD-18 `D-8`).
- `T-3` On `blocked` the skill calls `ask_user` with Extend +2 / Accept / Stop, then acts (extend-cap route / `submit_review_outcome approved` / `stopAgentLoop`); cap + stop conditions are stated explicitly so the agent never loops unbounded.

### Constraints

- Run GitNexus impact analysis before editing `agent_catalog.go`; warn on HIGH/CRITICAL.
- The skill must match the project skill format and lint clean.
- Ship in-repo so the feature works on first run (no external config).

### Open Questions

- Whether to ship the `.codex` mirror now or rely on the `.claude` skill via the shared loader — confirm against the project skill tree.

### Source Refs

- CP-36 `P-8`; SD-18 `D-1`/`D-8`/`D-9`, §7; SS-15 `AC-6`…`AC-11`, `BR-9`/`BR-10`.
- Anchors: `agent_catalog.go:391` (`builtinAgentDefinitions`); existing skills under `.claude/skills/` for format; Task-091 `submit_review_outcome`; Task-093 `autoOrchestrate`.

## 1. Goal

Provide the protocol + persona so the main agent can run the review-until-clean loop on the engine end-to-end: spawn coder + reviewer cohort, synthesize the join note, submit the verdict, and ask the user at the cap.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-8`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-1`, `D-8`, §7
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-6`
- specific upstream ids: CP-36 `P-8`; SD-18 `D-1`

## 3. Trigger

The engine + tool + note + auto-reinvoke (Tasks 089–093) are inert without an agent-facing protocol that tells the main agent how to spawn the cohort, synthesize, and submit the verdict within the bounded loop.

## 4. Exact Change

- `T-1` Built-in agent (`agent_catalog.go` `builtinAgentDefinitions():391`): add `synthesizer` — `Role:"synthesizer"`, `Tools:["Read","Grep","Glob"]`, `Source:"flowpilot"`, prompt: "Consolidate reviewer findings, dedup, resolve conflicts using the original task context and codebase, then call `submit_review_outcome` with `approved` (no issues) or `changes_requested` (a single consolidated, de-conflicted issue list). Never restart the coder yourself."
- `T-2` New skill `.claude/skills/agent-review-loop/SKILL.md` (+ optional `.codex` mirror) — the main-agent protocol:
  - On "review until clean": spawn `coder` (`wait=true`) to produce the change; capture `runId`.
  - Spawn the reviewer cohort: two `reviewer` agents (`correctness`, `security`) `wait=false`, `dependsOn:[coderRunId]`, shared `flowCohortId`, `autoOrchestrate:true`. Add a third `regression` reviewer when the change touches previously-tested behavior. Lenses are skill config; minimum two.
  - End the turn. When the consolidated join note arrives (auto-reinvoke), synthesize and call `submit_review_outcome`.
  - On `changes_requested` the runner re-enters the coder; a new cohort runs; repeat (bounded by cap).
  - On `blocked` (cap reached, issues open): call `ask_user` with *Extend cap by 2 / Accept remaining issues / Stop loop*; act on the answer (extend-cap route / `submit_review_outcome approved` / `stopAgentLoop`).
  - State the round cap and stop conditions explicitly.
- `T-3` Document the manual scenario (CP-36 §7) in the skill's examples section.

## 5. Touched Areas

- files: `agent_catalog.go`, `.claude/skills/agent-review-loop/SKILL.md` (+ optional `.codex` mirror); tests `agent_catalog_test.go`.
- modules: agent catalog + skills.
- routes: none.
- tables: none.

## 6. Acceptance Check (Definition of Done)

- [ ] `synthesizer` is discoverable via the catalog and **overridable** by an on-disk agent file of the same name (catalog test).
- [ ] The `agent-review-loop` skill file exists, lints against the project skill format, and documents spawn → cohort → synthesize → submit → ask-on-cap.
- [ ] The skill spawns the cohort with shared `flowCohortId` + `autoOrchestrate:true` and the default `correctness`+`security` lenses (third `regression` conditionally).
- [ ] The skill's `blocked` handling offers Extend +2 / Accept / Stop and maps each to the correct runner action.
- [ ] The manual scenario in CP-36 §7 runs end-to-end on desktop (coder → 2 reviewers → join note → synthesis turn → verdict → loop).
- [ ] `synthesizer` never restarts the coder (prompt + behavior check).

## 7. Out of Scope

- The tool/handler/executor (Tasks 089–091); the note + auto-reinvoke (Tasks 092–093); persistence (Task-085); board UI (Task-095).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
