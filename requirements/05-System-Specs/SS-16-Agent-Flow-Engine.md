# SS-16: Agent Flow Engine (Generic Main + Children Orchestration)

## Metadata

- Document ID: `SS-16`
- Title: `Agent Flow Engine (Generic Main + Children Orchestration)`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: `None` (foundational product feature spec)
- Child Documents: [SD-19: Agent Flow Engine Design](../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Related Documents: [SS-15: Agent Review Loop (Review Until Clean)](./SS-15-Agent-Review-Loop-Until-Clean.md), [SS-12: Multiple Agents](./SS-12-Multiple-Agents.md), [SS-06: Workflow Skill Agent](./SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](./SS-11-Workflow-With_Session.md), [SS-04: Workflow](./SS-04-Workflow.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, orchestration, generic, flow-definition, workflow, business-spec`

## AI Quick View

### Summary

- Define a **generic agent flow**: one **main** agent coordinating **N child** agents under a coordination policy (fan-out, barrier, re-invocation, termination) — independent of any single use case.
- The review-until-clean loop (SS-15) becomes the **first template instance** of this engine, not a special case; research fan-out, migration sweeps, and "1 main + some children, that's it" are other instances.
- Keep three concepts strictly separate: **AgentDefinition** (who an agent is), **FlowDefinition** (how a group is wired), and **Step/Workflow** (when things run in the macro pipeline).
- A flow works in **two tiers**: ad-hoc in Chat mode (just talk to the main agent — zero config) and declared as reusable **FlowDefinition data** (file first, Supabase later).
- A whole agent flow can plug into the existing linear Workflow engine as **one step** of type `agent_flow`; it does not get flattened into ordered workflow steps.

### Current Ask

- Define the business behavior and rules for a generic, reusable agent-flow capability so future flows need no new engine code — only data (a FlowDefinition) and a skill.

### Key Decisions

- `AC-0` The engine is **domain-free**: it knows nodes, dependencies, completion, barrier, re-invocation, and termination — never "coder", "reviewer", or any role.
- `AC-0b` A flow is **data, not code**; the same FlowDefinition shape runs in Chat mode (file) and Flow mode (Supabase), and an agent flow can be embedded as a single Workflow step.

### Constraints

- Must reuse the existing agent substrate (spawn, dependencies, result delivery, board) and the existing linear Workflow/Step model — not a parallel system.
- Must not break the linear `workflows`/`workflow_steps` model; an agent flow nests inside one step.
- Each agent stays isolated: only results flow to the main, never full transcripts.
- Every flow must be bounded and stoppable.

### Open Questions

- `Q-1` Minimum viable FlowDefinition fields for Phase 1 (chat/file) vs the richer Flow-mode (Supabase) schema.
- `Q-2` Should ad-hoc chat flows be persistable into a saved FlowDefinition ("save this flow") in Phase 1, or only authored as files?
- `Q-3` Whether a flow may contain a sub-flow (nesting) in Phase 1, or only one level (main + children).

## 1. Goal

Let a user (or the main chat agent) **define and run a flow of agents** — one coordinator plus some children, with a clear coordination policy — for *any* purpose, and reuse that flow later. The review loop is one such flow; the same machinery must serve other patterns without new engine code.

## 2. Problem

The agent feature today (and the SS-15 review loop) is shaped around one use case: coder ↔ reviewers. The coordination logic risks being baked into engine code (role names, "restart the coder", review verdict semantics). That blocks the obvious next ask — *"I just want a new flow with one main and some children"* — because every new pattern would need Go changes. There is also no defined bridge between an agent flow and the existing linear Workflow/Step model, so the two orchestration systems would drift apart.

## 3. Scope

- **In scope:**
  - A generic flow concept: main + children, with fan-out, a barrier (when the main hears back), re-invocation, and a termination policy.
  - **FlowDefinition** as reusable data (topology + policy), file-based first.
  - Two usage tiers: ad-hoc Chat-mode flows (no config) and declared FlowDefinitions.
  - A bridge so an agent flow can be **one step** (`agent_flow`) inside a linear Workflow.
  - Re-framing the review loop (SS-15) as the first FlowDefinition template.
- **Out of scope:**
  - The full durable Supabase schema for FlowDefinitions (deferred to Flow-mode rollout / CP-19 Task-085).
  - Direct agent-to-agent conversation (the main is always the hub).
  - Auto-commit/merge of results.

## 4. Non-Goals

- Not a general autonomous multi-agent framework; the main agent is always the coordinator and arbiter.
- Not replacing the linear Workflow/Step engine; agent flows nest *inside* a step.
- Not removing human approval gates (SS-08).

## 5. User Stories or Primary Use Cases

- `US-1` As a developer, I want to say "one main agent plus a couple of children doing X" and have it run, **without** any predefined flow — ad-hoc in chat.
- `US-2` As a developer, I want to **save a flow as a reusable definition** so I (and teammates) can run it again with one command.
- `US-3` As a workflow author, I want to drop an agent flow **into a Workflow as a single step**, so a pipeline can mix standard steps and agent orchestration.
- `US-4` As a platform owner, I want **new flows to require only data + a skill**, never engine code, so the system scales to many patterns.
- `US-5` As a developer, I want the review-until-clean loop to be **just one of these flows**, proving the engine is generic.

## 6. Acceptance Criteria

- `AC-1` A user can run an ad-hoc flow ("1 main + some children") in Chat mode with no predefined definition.
- `AC-2` A flow can be expressed as a **FlowDefinition** (topology + coordination policy) stored as data and re-run.
- `AC-3` The coordination engine contains **no role-specific logic** — adding a new flow requires only a FlowDefinition and (optionally) a skill, not engine code.
- `AC-4` The same FlowDefinition shape is valid in Chat mode (file) and Flow mode (Supabase); only the storage backend differs.
- `AC-5` An agent flow can be embedded in a linear Workflow as **one step** of type `agent_flow`; the workflow's other steps remain ordered and unaffected (see §7 diagram).
- `AC-6` The SS-15 review loop is realized as a FlowDefinition template on this engine, with no behavior regression.
- `AC-7` Every flow is bounded and stoppable; isolation (results-only, no transcript sharing) holds for all flows.

## 7. Where An Agent Flow Fits (Reference Diagram)

A linear Workflow stays linear; an agent flow is one step that internally runs a main + children loop:

```
Workflow (linear)
  Step 1: scaffold        (standard)
  Step 2: review-loop     (agent_flow)  ← FlowDefinition runs main+children+loop INSIDE here
  Step 3: write-docs      (standard)
```

- The Workflow engine sees three ordered steps. Step 2's *type* is `agent_flow`; its internal behavior is a full FlowDefinition (one main + N children + barrier + termination). The agent sub-graph is **not** flattened into `workflow_steps`.
- The same `agent_flow` step in this diagram is exactly what runs ad-hoc in Chat mode without a Workflow around it — same FlowDefinition, different entry point.

## 8. Business Rules

- `BR-1` **Domain-free engine.** Coordination logic (spawn, depend, barrier, re-invoke, terminate) carries no role or use-case knowledge.
- `BR-2` **Three separate concepts.** `AgentDefinition` = persona (who); `FlowDefinition` = topology + policy (how); `Workflow`/`Step` = macro order (when). None is overloaded onto another.
- `BR-3` **Flow is data.** A FlowDefinition is authored/stored as data and is portable across Chat (file) and Flow (Supabase) backends with the same shape.
- `BR-4` **Nest, don't flatten.** An agent flow embedded in a Workflow is one `agent_flow` step; its sub-graph lives in agent-run state, not in `workflow_steps`.
- `BR-5` **Hub-only + isolation.** The main agent is the sole coordinator; children share results, never transcripts (consistent with SS-15 `BR-1`/`BR-2`).
- `BR-6` **Bounded + stoppable.** Every flow has a termination policy (a control signal and/or a cap) and a user Stop.
- `BR-7` **Review loop is a template.** SS-15's behavior is a FlowDefinition instance, demonstrating genericity (`AC-6`).

## 9. Edge Cases

- `E-1` Ad-hoc flow with a single child — degenerate but valid.
- `E-2` A FlowDefinition references an agent persona that does not exist — surfaced as a definition error, not a silent no-op.
- `E-3` An `agent_flow` step inside a Workflow fails — the Workflow treats it like any failed step (its existing failure handling), while the agent sub-graph reports its own result.
- `E-4` The same FlowDefinition is run ad-hoc (chat) and as a workflow step — behavior is equivalent modulo persistence.

## 10. Dependencies

- The agent substrate and board (CP-19 / SD-16).
- The linear Workflow/Step engine and `step_definitions` catalog (SS-04, `cp07_workflow_engine`).
- Session continuity (SS-11) and approval gates (SS-08).
- The review-loop chain (SS-15 / SD-18 / CP-36) as the first template.

## 11. Open Questions

- `Q-1` Minimum FlowDefinition fields for Phase 1 (chat/file) vs the Supabase Flow-mode schema (see SD-19 `D-3`).
- `Q-2` "Save this ad-hoc flow as a FlowDefinition" in Phase 1, or file-authoring only?
- `Q-3` Sub-flow nesting in Phase 1, or one level only?
- `Q-4` Does the `agent_flow` step reuse `step_definitions.agent_type='autonomous'` or warrant a new `agent_type='agent_flow'` value (see SD-19 `D-5`)?

## 12. Definition of Done

- The generic flow concept (main + children + policy) is specified and the review loop is shown to be one instance (`AC-6`).
- The three-concept separation (`BR-2`) and the nest-don't-flatten bridge (`BR-4`, §7 diagram) are documented and adopted by SD-19.
- The Chat-first / Flow-later progression with one FlowDefinition shape (`AC-4`) is agreed.
- SD-19 (design) is linked as a child and covers every AC; SS-15/SD-18/CP-36 are re-linked as the first template.
