# SD-19: Agent Flow Engine

## Metadata

- Document ID: `SD-19`
- Title: `Agent Flow Engine (Generic Coordination + FlowDefinition)`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [SS-16: Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- Child Documents: `None` (a generic-engine CP follows once the review-loop template lands)
- Related Documents: [SD-18: Main-Hub Agent Review Loop](./SD-18-Main-Hub-Agent-Review-Loop.md), [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SD-13: Multiple Agents](./SD-13-Multiple-Agents.md), [SD-05: Workflow Engine](./SD-05-Workflow-Engine.md), [CP-36: Agent Review Loop And Main-Hub Orchestration](../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, generic, flow-definition, flow-catalog, workflow-bridge, local-runner`

## AI Quick View

### Summary

- Design a **domain-free coordination engine**: it spawns children, enforces dependencies, delivers results to the main, re-invokes the main on a barrier, and terminates on a control signal — with **no knowledge of roles or use cases**.
- Introduce **FlowDefinition** (topology + coordination policy) as data, loaded by a **FlowCatalog** that mirrors the existing `AgentCatalog` — file-based first (Chat mode), Supabase-backed later (Flow mode), one shape both ways.
- Keep three concepts separate: **AgentDefinition** (persona, exists), **FlowDefinition** (topology, new), **Workflow/Step** (macro order, exists).
- Generalize the review verdict tool into a generic **`flow_control`** concept; domain tools like `submit_review_outcome` are **declared instances** that route to the one generic handler.
- Bridge to the linear Workflow engine: an agent flow is **one `agent_flow` step** (reusing `step_definitions.agent_type`); its live sub-graph stays in `agent_runs`/`agent_messages` (CP-19 Task-085) and is **never flattened** into `workflow_steps`.

### Current Ask

- Settle the generic engine's contracts and data model so any future flow needs only a FlowDefinition + a skill, and so an agent flow can nest cleanly inside the existing Workflow model.

### Key Decisions

- `D-1` **Engine is domain-free.** Primitives only: spawn, depend, barrier, deliver, re-invoke, control/terminate.
- `D-2` **Three concepts, never conflated** (SS-16 `BR-2`): `AgentDefinition` (who) / `FlowDefinition` (how) / `Workflow`+`Step` (when).
- `D-3` **FlowDefinition is data**; `FlowCatalog` mirrors `AgentCatalog` (file → provider-home → built-in precedence); same shape across Chat (file) and Flow (Supabase) backends.
- `D-4` **Generic `flow_control`** ({status: continue|done|escalate, payload}); domain tools (`submit_review_outcome`) are per-flow *declarations* routing to one engine handler.
- `D-5` **`agent_flow` step bridge**: an agent flow embeds in a Workflow as one step; sub-graph in `agent_runs`/`agent_messages`, not `workflow_steps`.
- `D-6` **Decision lives in the main agent, not the engine.** "What to spawn next" is the main agent's call (via `spawn_agent`), so the engine never encodes a use-case state machine.

### Constraints

- Reuse the CP-19/SD-16 substrate (spawn, `dependsOn`, `pendingAgentContext`, board, auto-reinvoke) and the linear Workflow model (`cp07_workflow_engine`); no parallel system.
- Do not alter `ProviderRuntimeAdapter` or the SSE transport except additively.
- Run data for **both** chat and flow modes persists **locally** (`localFileSessionStore` / `sessions.ndjson`, Drive-synced); no Supabase **run** migration. Flow/Step **definitions** stay on Supabase. This supersedes the original CP-19 Task-085 Supabase `agent_runs`/`agent_messages` plan (see CP-36 `P-5`/`Q-5`).
- GitNexus impact analysis before editing named symbols; warn on HIGH/CRITICAL.

### Open Questions

- `Q-1` FlowDefinition minimum fields for Phase 1 vs Flow-mode (SS-16 `Q-1`).
- `Q-2` Reuse `agent_type='autonomous'` vs add `agent_type='agent_flow'` (SS-16 `Q-4`).
- `Q-3` "Save ad-hoc flow as FlowDefinition" in Phase 1 (SS-16 `Q-2`); sub-flow nesting (SS-16 `Q-3`).

### Source Refs

- SS-16 `AC-1`…`AC-7`, `BR-1`…`BR-7`, §7 diagram.
- SD-16 (spawn/tool/result substrate), SD-18 (review template), SD-05 / `cp07_workflow_engine.sql` (linear Workflow/Step), CP-19 Task-085 (`agent_runs`/`agent_messages`).
- Anchors: `agent_catalog.go` (`AgentCatalog`, `AgentDefinition`); `agent_orchestrator.go` (graph/bus/loop); `interactive_service.go` (spawn, barrier, reinvoke); `supabase/migrations/20260520033000_cp07_workflow_engine.sql:42-118` (`step_definitions.agent_type`, `workflows`, `workflow_steps`).

## 1. Goal

Provide a generic coordination engine plus a FlowDefinition data model so that: (a) a user can run "1 main + some children" ad-hoc in Chat mode; (b) repeatable flows are authored as data; (c) an agent flow plugs into the linear Workflow engine as one step; and (d) the SS-15 review loop is just the first FlowDefinition template — all without role-specific engine code.

## 2. Input Documents

- [SS-16: Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md) — `AC-1`…`AC-7`, `BR-1`…`BR-7`.
- [SD-16](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SD-18](./SD-18-Main-Hub-Agent-Review-Loop.md), [SD-05](./SD-05-Workflow-Engine.md).

## 3. Architecture Decision

- `D-1` **Domain-free engine.** The coordinator exposes only: `spawn(child)`, `dependsOn(barrier)`, `deliverResults(main)`, `reinvoke(main, onBarrier)`, `control(continue|done|escalate)`. No role strings, no use-case branches. (SS-16 `AC-3`, `BR-1`.)
  - *Alternatives:* hardcode each pattern (status quo) — rejected: every flow needs Go changes.
- `D-2` **Three concepts, separated** (SS-16 `BR-2`). `AgentDefinition` (persona; `agent_catalog.go`, exists) is the *who*. `FlowDefinition` (new) is the *how*. `Workflow`/`Step` (`cp07_workflow_engine`) is the *when*. A FlowDefinition **references** AgentDefinitions by name; it does not redefine them.
- `D-3` **FlowDefinition as data + FlowCatalog.** Mirror `AgentCatalog` exactly: discover from project files, provider homes, and built-ins, with name precedence. Phase 1 stores FlowDefinitions as files (e.g. `.flowpilot/flows/*.yaml`); Flow mode stores the same shape in Supabase. (SS-16 `AC-2`, `AC-4`.)
- `D-4` **Generic control signal.** The engine has one control concept, `flow_control({status: "continue"|"done"|"escalate", summary, payload?})`. A flow may **declare** a domain-named tool (e.g. `submit_review_outcome`) whose schema is data and whose handler maps onto `flow_control` (e.g. `approved→done`, `changes_requested→continue`, `blocked→escalate`). One engine handler, many declared faces — exactly how `AgentCatalog` treats agents as data. (SS-16 `AC-3`.)
- `D-5` **`agent_flow` step bridge.** An agent flow embeds in a linear Workflow as **one step**. Reuse `step_definitions.agent_type` (today `standard|autonomous`); either treat `autonomous` as "runs a FlowDefinition" or add a new `agent_flow` value (`Q-2`). The step's live sub-graph is the agent run tree persisted **locally** in the session record (`sessions.ndjson`, Drive-synced) for both Chat and Flow modes — superseding the original Task-085 Supabase `agent_runs`/`agent_messages` plan (CP-36 `P-5`); it is **never** expanded into `workflow_steps`, preserving the linear engine. (SS-16 `AC-5`, `BR-4`.)
- `D-6` **The main agent owns "what next", not the engine.** On re-invocation the main agent receives the consolidated results and decides the next action (spawn more, finish) via existing tools. The engine never holds a use-case state machine — this is what makes the review loop's "restart coder on changes" a *skill behavior*, not engine code. (SS-16 `BR-1`.)

- `D-7` **Node/edge/policy field vocabulary.** A FlowDefinition compiles to three domain-free shapes (canonical Go structs in CP-36 [Task-089](../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)): `FlowNode{agent, run: inline|delegate, lifecycle: once|reinvoke, join: all|any|quorum(n)}`, `FlowEdge{from, to, when, kind: forward|back}`, `FlowPolicy{cap, onCap, extendBy, extendMax}`. `run` distinguishes "the hub does the step itself" from "spawn a child"; `lifecycle` distinguishes "respawn fresh" from "restart-with-feedback" on re-entry; a `back` edge is a bounded loop (retry) counted against `cap`. Edges match the **mapped generic `flow_control` status**, never a domain string. This is what makes "wait for reviewer 2" (a `join`) and "failed → coding" (a `back` edge) expressible without role logic.
- `D-8` **Unified local run persistence.** Run data for both Chat and Flow modes persists through `localFileSessionStore` (`sessions.ndjson`) and syncs cross-PC via Drive; flow/step **definitions** remain on Supabase. The legacy `workflow_run_*` tables and the Supabase agent-run plan are deprecated. One run sink, two authoring surfaces (emergent chat / predeclared FlowDefinition), and the coordinator (model-driven or definition-driven) is the only difference between modes — the executor is identical. (CP-36 `P-5`/`P-6`.)

Why chosen: it turns the orchestration substrate FlowPilot already has (spawn, barrier, deliver, auto-reinvoke) into a reusable engine by moving all domain meaning into data (FlowDefinition + skill) and pushing the "next step" decision to the model, while bridging cleanly to the linear Workflow model.

## 4. Component Impact

- **Impacted modules:** local-runner orchestrator (generalize loop control to `flow_control`), interactive service (FlowDefinition load + ad-hoc run), agent catalog (sibling `FlowCatalog`), provider adapters (declare per-flow control tools), desktop board (already has a generic graph fallback).
- **New modules:** `FlowCatalog` + `FlowDefinition` loader; the generic `flow_control` handler; (Flow mode) an `agent_flow` step executor in the workflow engine.
- **Unchanged modules:** `ProviderRuntimeAdapter`, SSE transport (additive), the linear Workflow/Step tables and execution (an agent flow is one opaque step).

## 5. Data Model

- **AgentDefinition** (exists): persona — `name, role, provider, model, tools, systemPrompt, source`.
- **FlowDefinition** (new, data): topology + policy. Indicative shape:
  ```yaml
  name: review-loop
  description: Review a change with N reviewers until clean.
  main:     { agent: orchestrator, skill: agent-review-loop }
  children:
    - { agent: coder,    role: coder }
    - { agent: reviewer, role: reviewer, lens: correctness, dependsOn: [coder] }
    - { agent: reviewer, role: reviewer, lens: security,    dependsOn: [coder] }
  barrier:  all                 # any | quorum(n) | all
  reinvoke: onBarrier           # re-prompt main when the barrier is satisfied
  control:                      # how the flow ends (declares the domain tool)
    tool: submit_review_outcome
    done_when:     status == approved
    continue_when: status == changes_requested
    escalate_when: status == blocked
    cap: 3
    onCap: escalate
  ```
- **Run-time entities:** the existing agent run tree (`interactiveRun` + orchestrator graph/bus), persisted **locally** in the session record (`sessions.ndjson`, Drive-synced) for **both** Chat and Flow modes. A FlowDefinition is the *template*; a run is the *instance*.
- **Workflow bridge:** a `workflow_steps` row of an `agent_flow` step type carries a reference to a FlowDefinition; its run's sub-graph lives in the local session record (not Supabase per-agent rows). No new per-agent rows in `workflow_steps`.

## 6. Interfaces and Contracts

- **Generic control:** `flow_control({status, summary, payload?})` → loop transition (continue/done/escalate); bounded by the FlowDefinition `cap`.
- **Declared domain tools:** a FlowDefinition's `control.tool` (e.g. `submit_review_outcome`) is registered per provider from data and dispatched to the generic handler (SD-18 §6 is the review instance of this).
- **FlowCatalog:** `listFlows(cwd) []FlowDefinition` mirroring `AgentCatalog.listAgents`; `GET /client/flows` mirroring `GET /client/agents`.
- **Ad-hoc run:** Chat mode needs no FlowDefinition — the main agent uses `spawn_agent` + `dependsOn` + `flow_control`; a FlowDefinition simply pre-wires those calls.
- **Workflow step:** Flow mode adds an `agent_flow` step executor that, on entry, starts the FlowDefinition's root agent run and completes the step when the flow terminates.

## 7. Execution Flow

Reference placement of an agent flow inside a linear Workflow (SS-16 §7):

```
Workflow (linear)
  Step 1: scaffold        (standard)
  Step 2: review-loop     (agent_flow)  ← FlowDefinition runs main+children+loop INSIDE here
  Step 3: write-docs      (standard)
```

1. **Entry.** Either the user runs a flow ad-hoc in Chat mode, or the Workflow engine reaches an `agent_flow` step and starts its FlowDefinition.
2. **Fan-out.** The engine spawns the main + children per the FlowDefinition (`dependsOn` wiring), reusing the existing spawn/barrier path.
3. **Barrier + deliver.** When the barrier is satisfied, children's results are folded into the main's context (consolidated note, SD-18 `D-5`).
4. **Re-invoke.** The engine re-prompts the main (auto-reinvoke, CP-36 P-4); the main decides next steps via its skill — spawn more children or finish.
5. **Control.** The main calls the flow's control tool; the engine maps it to `continue` (loop, bounded by `cap`), `done` (terminate, hand off), or `escalate` (ask the user).
6. **Exit.** Ad-hoc: the flow's result returns to the chat. Workflow: the `agent_flow` step completes (or fails) and the Workflow advances to Step 3.

## 8. Failure and Edge Handling

- `F-1` FlowDefinition references a missing AgentDefinition → definition-load error, surfaced before run (SS-16 `E-2`).
- `F-2` An `agent_flow` step fails → the Workflow uses its existing step-failure handling; the agent sub-graph reports its own result/error (SS-16 `E-3`).
- `F-3` Control tool never called / cap hit → engine escalates (ask user) or terminates per policy; never loops unbounded (SS-16 `BR-6`).
  - **BUG-231 settlement contract**: `escalate` (and a cap hit routed through `continue`) is a non-terminal "awaiting user" pause, distinct from `done`/`failed` — the engine must never leave it silently indistinguishable from an active turn. Concretely: (a) the flow's hub/control node settles to `WAITING_USER_APPROVAL` on the step timeline, not `RUNNING` (reads as a hang) and not `FAILED` (reads as an error); (b) the loop's `AgentLoopState.BlockReason` (`"cap"` | `"escalate"`) records why, so the client can render the correct recovery affordance without parsing `GateReason` prose; (c) the client-facing run status derived from a `blocked` loop must be distinct from `running`, so the chat composer unlocks instead of staying gated as "waiting for the current turn"; (d) resuming (the desktop's unified "Continue" action, `POST .../agent-loop/continue`) hands any user guidance back to the hub and lets the hub re-decide via the same control tool — it does not hard-route anywhere itself, and auto-raises the cap only when the block reason was the cap.
- `F-4` Ad-hoc vs workflow-step parity → same FlowDefinition yields equivalent behavior modulo persistence (SS-16 `E-4`).

## 9. Security and Operational Concerns

- **auth:** running a flow is a local runner / workflow action under the current project authority; child gates remain (SS-08).
- **secrets:** FlowDefinitions and AgentDefinitions carry no credentials.
- **audit:** flow start, child spawns, control signals, and termination are logged + on the bus; Flow mode persists to `agent_runs`/`agent_messages`.
- **rollback:** the engine is additive; with no FlowDefinitions and no `agent_flow` steps, Chat and Workflow behave exactly as today.

## 10. Risks and Trade-Offs

- `R-1` **Over-generalization too early.** Mitigation: ship the engine by *extracting* it from the review-loop template (CP-36), not before — prove genericity with a second template before formalizing Flow-mode tables.
- `R-2` **Two orchestration models drifting** (agent flow vs linear Workflow). Mitigation: the `agent_flow` step is the single, narrow bridge; sub-graph stays in agent-run tables (`D-5`).
- `R-3` **Declared-tool indirection** harder for the model to use than a fixed tool. Mitigation: keep clear domain tool names/schemas per flow (data), backed by one handler.
- `R-4` **FlowDefinition schema churn** between Chat-file and Flow-Supabase. Mitigation: one shape, two backends (`D-3`); validate against a shared schema.

## 11. Validation Strategy

- **unit:** FlowCatalog discovery/precedence (mirror AgentCatalog tests); `flow_control` mapping for a declared tool; cap/termination bounding.
- **integration:** ad-hoc Chat flow runs end-to-end with spawn+barrier+reinvoke+control; the review-loop FlowDefinition reproduces SS-15 behavior; an `agent_flow` step inside a Workflow runs and completes the step (Flow mode).
- **manual:** author a second, non-review FlowDefinition (e.g. a 2-child research fan-out) and confirm it runs with **zero engine code changes** (`AC-3`).
- **observability:** flow lifecycle logs + board graph for any flow, not just review.

## 12. Traceability to Spec

- `AC-1` ad-hoc chat flow → `D-1`, `D-6`, §6 (ad-hoc run).
- `AC-2` FlowDefinition as data → `D-3`, §5.
- `AC-3` no role-specific engine code → `D-1`, `D-4`, `D-6`, §11 (second-template test).
- `AC-4` one shape, two backends → `D-3`, §5.
- `AC-5` agent flow as one Workflow step → `D-5`, §7 diagram.
- `AC-6` review loop is a template → SD-18 / CP-36 re-linked; `D-4` (declared `submit_review_outcome`).
- `AC-7` bounded + isolated for all flows → `D-1`/`D-6`, §8 `F-3`, §9.
- `BR-1` domain-free → `D-1`. `BR-2` three concepts → `D-2`. `BR-3` flow is data → `D-3`. `BR-4` nest not flatten → `D-5`. `BR-5` hub-only → `D-6`. `BR-6` bounded → §8 `F-3`. `BR-7` review = template → `D-4`, SD-18/CP-36.
