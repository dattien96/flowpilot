# CP-36: Generic Agent-Flow Engine And Review Loop (First Template)

## Metadata

- Document ID: `CP-36`
- Title: `Generic Agent-Flow Engine And Review Loop (Domain-Free Hub Coordination, First Template)`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-15: Agent Review Loop (Review Until Clean)](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: [Task-089: Generic Flow Vocabulary And flow_control Handler](../../08-Task/todo/Task-089-Generic-Flow-Vocabulary-And-Flow-Control-Handler.md), [Task-090: Bounded Flow Runtime Executor](../../08-Task/todo/Task-090-Bounded-Flow-Runtime-Executor.md), [Task-085: Unified Local Run Persistence](../../08-Task/todo/Task-085-Unified-Local-Run-Persistence.md), [Task-091: Review-Loop Template (Outcome Tool, Config, Legacy Gate)](../../08-Task/todo/Task-091-Review-Loop-Template.md), [Task-092: Consolidated Multi-Result Join Note](../../08-Task/todo/Task-092-Consolidated-Multi-Result-Join-Note.md), [Task-093: Bounded Auto-Reinvocation Of The Hub](../../08-Task/todo/Task-093-Bounded-Auto-Reinvocation-Of-The-Hub.md), [Task-094: agent-review-loop Skill And synthesizer Built-in](../../08-Task/todo/Task-094-Agent-Review-Loop-Skill-And-Synthesizer-Builtin.md), [Task-095: Orchestration Board, Contract, And Client](../../08-Task/todo/Task-095-Orchestration-Board-Contract-And-Client.md)
- Related Documents: [CP-19: Multiple Agents](../done/CP-19-Multiple-Agents.md), [CP-41: RAG Harness Flow Mode](../todo/CP-41-RAG-Harness-Flow-Mode.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-084: Dependency Feedback Loop And Orchestration Board](../../08-Task/done/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, generic, node-edge-policy, flow-control, review-loop, main-hub, synthesis, local-persistence, local-runner, desktop`

## AI Quick View

### Summary

- Build a **domain-free agent-flow engine** and ship the **review-until-clean loop as its first template**. The engine knows only `nodes`, `edges`, a `join` (barrier), `re-invocation`, and a bounded `control` signal — never "coder"/"reviewer"/"plan".
- Coordination is **hub-only**: the main (parent) agent is the sole router. Children never talk to or wait on each other. The hub does two things — **join** (barrier on incoming edges) and **route** (fire conditional out-edges). "Wait for reviewer 2" is `join: all`; "failed → back to coding" is a bounded **back-edge**.
- A single generic verdict concept, **`flow_control({status: continue|done|escalate, ...})`**, drives every transition. The review tool **`submit_review_outcome`** is a *declared face* mapping onto it (`approved→done`, `changes_requested→continue`, `blocked→escalate`). It is the **only** tool the model sees; `flow_control` is the internal handler, not a second tool (**G3**). Edges match the **mapped generic status**, not domain strings (**G5**). The legacy substring keyword path (`interactive_service.go:880-935`) is gated OFF in explicit mode.
- Bounded + ask-on-cap: back-edge traversals are capped (default 3, overridable; bounded extend +2 ×2). Reaching the cap with open work → `blocked` → the skill calls `ask_user`. Never a silent stop, never an unbounded re-prompt.
- **Unified local persistence:** ALL run data — chat **and** flow — persists through `localFileSessionStore` (`sessions.ndjson`) and syncs cross-PC via Drive (already shipped). Flow/Step **definitions** stay on Supabase. Legacy `workflow_run_logs`/`workflow_run_sessions`/`workflow_run_steps` + the `SupabaseWorkflowStore` run path are **deprecated/ignored** (already true in production at `cli/root.go:120-123`; the desktop reads run state over the HTTP gateway).
- Node properties keep the model uniform: `run: inline|delegate` (hub does it vs spawns a child) and `lifecycle: once|reinvoke` (fresh spawn vs restart-with-feedback). Resume restores all flow fields from `sessions.ndjson` (**G1**).
- This engine is the substrate **CP-41** consumes: Plan→Coding→Testing is a FlowDefinition + node skills + the context harness, with **no further changes to agent interaction** (`DOD-11`).

### Current Ask

- Deliver a plan detailed enough that any implementer (human or AI) can build (a) the domain-free flow engine, (b) the unified local run-persistence cutover, and (c) the review-until-clean loop as the engine's first template — end-to-end, with exact files, symbols, tool schemas, routes, execution order, and full per-task Definition of Done.

### Key Decisions

- `P-1` **Domain-free engine, review loop is data.** Coordinator primitives only: `spawn / edge / join / deliver / reinvoke / control(continue|done|escalate)`. No role strings or use-case branches in Go. Review semantics live in a skill + node/edge/policy config (SS-16 `AC-3`/`BR-1`, SD-19 `D-1`).
- `P-2` **Main agent is the only hub/router; children are isolated.** Children post results via `pendingAgentContext`; the parent synthesizes and decides. No peer-to-peer messaging (SD-16 `D-6`, SS-16 `BR-5`). Coordination is **join + route** done by the hub, not agents waiting on each other.
- `P-3` **Generic `flow_control` with declared faces.** One handler maps `{continue|done|escalate}`. `submit_review_outcome` is a per-template declaration whose schema is data and whose handler routes to `flow_control` (SD-19 `D-4`). It is the only registered flow tool (**G3**). Edges match the mapped generic status (**G5**). The legacy keyword path is gated OFF in explicit mode.
- `P-4` **Bounded + ask-on-cap.** Retries are **back-edges** bounded by `cap` (default 3, per-run override). At the cap with open work the loop goes `blocked` and the skill calls `ask_user` (extend +2 / accept / stop). Extend is bounded to +2 at most twice (ceiling = cap + 4). Never silent, never unbounded.
- `P-5` **Unified local run persistence.** Run data for **both** chat and flow modes flows through `localFileSessionStore` (`sessions.ndjson`), Drive-synced, and is restored on resume. Flow/Step **definitions** remain on Supabase. Legacy `workflow_run_*` tables + `SupabaseWorkflowStore` run methods are deprecated and unwired in production; desktop reads run state via the local-runner HTTP gateway.
- `P-6` **Node lifecycle + run mode are properties, not branches.** `run: inline|delegate` and `lifecycle: once|reinvoke` make "hub does it vs spawn a child" and "respawn vs restart-with-feedback" config, covering retries and sequence/parallel topologies uniformly.
- `P-7` **Additive only; CP-41 builds on top.** New engine types, one generic handler + one declared tool, a skill, a built-in agent, new routes, additive board UI. No change to `ProviderRuntimeAdapter` or the SSE transport except additive reuse. Single-agent/normal-chat behavior unchanged. CP-41 adds the context harness over this engine **without touching agent interaction**.

### Constraints

- Run GitNexus impact analysis before editing any Go/TS symbol named below; report blast radius; warn on HIGH/CRITICAL (CLAUDE.md mandate). Run `gitnexus_detect_changes()` before commit.
- The engine MUST be domain-free: no `coder`/`reviewer`/`plan`/review-verdict strings in coordinator Go code. All domain meaning lives in skill + node/edge/policy data. A guard test asserts this.
- Do not change the `ProviderRuntimeAdapter` interface or the `ProviderEvent`/`ProviderEventDTO` union except by additive fields/variants.
- Do not regress normal chat, the existing single coder↔reviewer loop, session resume (SD-14), cross-PC sync, or Drive sync.
- **Run data persists locally for both modes** via `localFileSessionStore`; do NOT wire `SupabaseWorkflowStore` for run data. Flow/Step **definitions** continue to be read from Supabase.
- The auto-reinvocation loop MUST be bounded and stoppable; an unbounded re-prompt loop is a release blocker.
- Reuse the existing `artifact`/`pendingAgentContext`/bus channels; do not invent a new transport.
- No new Supabase **run** migration. Definition tables already exist on Supabase and are unchanged.
- **Prerequisite (G7):** CP-36 builds on CP-19 Phase 1 — Task-082 (spawn tool + orchestrator core, done) and Task-084 (dependency/bus + Orchestration Board, currently `todo`). Task-084 must land (or its DTOs be frozen) before `P-2`/`P-6`/`P-7`/`P-9`.

### Open Questions

- `Q-1` **Resolved** — synthesis is performed by the **main agent** by default (skill-guided); the `synthesizer` built-in is an optional offload (SS-15 `Q-1`, SD-18 `D-1`).
- `Q-2` **Resolved** — the review loop is realized as the **first FlowDefinition template** of the SS-16/SD-19 engine built here.
- `Q-3` **Resolved** — default lenses `correctness` + `security`; `regression` added by the skill when tested behavior changes; lens set is skill config (SS-15 `BR-9`, SD-18 `D-8`).
- `Q-4` **Resolved** — extend-cap bounded to +2 at most twice (ceiling = initial cap + 4) (SS-15 `BR-10`, SD-18 `D-9`).
- `Q-5` **Resolved** — run persistence unified on `localFileSessionStore` + Drive for both modes; legacy `workflow_run_*` Supabase run tables deprecated (desktop reads over HTTP). Definitions stay on Supabase. This supersedes the original CP-19 Task-085 (Supabase agent runs).

### Source Refs

- `SS-16` `AC-1`…`AC-7`, `BR-1`…`BR-7`, §7; `SD-19` `D-1`…`D-6`, §5–§7.
- `SS-15` `AC-1`…`AC-11`, `BR-1`…`BR-10`; `SD-18` `D-1`…`D-9`, §5–§7, §12.
- `SD-16` `D-1`…`D-15`, §7, §14, §14.5, §15.
- `CP-19` `P-2`/`P-4`/`P-5`/`P-6`; Task-082 (done), Task-084 (orchestrator/bus/board), Task-085 (superseded — repurposed here).
- Code anchors (verified): `workflow_store.go` (`WorkflowStore` interface); `local_file_session_store.go:16-37`; `supabase_workflow_store.go:28-30` (legacy run path); `cli/root.go:120-123` (production wiring uses the local store); `chat_session_sync.go` (Drive sync); `agent_orchestrator.go` (`AgentLoopState`, `transition`, `advanceRound`, `parseSpawnAgentInput:344`, `addBus`, `queueFeedback`); `interactive_service.go:394-571,744-762,860,870-879,884-998,1174`; `provider_registry.go:25-32` (`TurnBridge`); `codex_adapter.go:89-107,133,289-318`; `claude_mcp_server.go:235-296`, `claude_permission_mcp.go:182-195`; `interactive_handlers.go:41-48`; `interactive_resume.go`; desktop `OrchestrationBoard.tsx`, `state/store.ts`, `types/contract.ts:91-136,461-466`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`; `apps/admin-web/.../http-local-runner-gateway.ts`.

## 1. Goal

Let a FlowPilot user (or the main chat agent) run a **bounded, observable flow of agents** — one hub coordinating N children under a coordination policy — for *any* purpose, and have the **review-until-clean loop be the first template** of that engine, not a special case.

The engine the user observes:

1. The hub spawns children per a topology (`dependsOn` edges); a node may run **inline** (the hub does it) or **delegate** (spawn a child).
2. When a node's **join** condition is met (`all`/`any`/`quorum(n)` of incoming edges), the hub receives consolidated results.
3. The hub **synthesizes** and emits **`flow_control`**: `continue` (fire out-edges incl. bounded back-edges), `done` (terminate + hand off), or `escalate` (ask the user).
4. Back-edges (e.g. "failed → coder") are bounded by `cap`; the re-entered node restarts with feedback (`lifecycle: reinvoke`) or respawns (`lifecycle: once`).
5. Cap with open work → `blocked` → `ask_user`.

The review loop is one configuration: nodes `coder` + N `reviewer`, edges `reviewer dependsOn coder` (`join: all` into synthesis), back-edge `synthesis → coder when continue` (`cap: 3`), and `submit_review_outcome` as the declared `flow_control` face.

All run state persists through the **local** `localFileSessionStore`, syncs via Drive, and restores on resume; definitions live on Supabase. This is the substrate **CP-41** reuses for Plan→Coding→Testing — it adds the context harness as node behavior and changes **nothing** about agent interaction.

## 2. Input Documents

- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — generic engine design (`D-1`…`D-6`, FlowDefinition data, interfaces, execution flow).
- [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) — review template behavior (`D-1`…`D-9`).
- [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) — generic flow business rules (`AC-1`…`AC-7`, `BR-1`…`BR-7`).
- [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) — review acceptance criteria.
- [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) — spawn/tool/result-sharing/isolation substrate.
- [CP-19: Multiple Agents](../done/CP-19-Multiple-Agents.md) — Phase-1 orchestrator, spawn tool, board, result sharing (Task-082/084 substrate).
- [CP-41: RAG Harness Flow Mode](../todo/CP-41-RAG-Harness-Flow-Mode.md) — downstream consumer of this engine.

## 3. Implementation Strategy

- **Overall approach:** Build the domain-free runtime (node/edge/policy executor + generic `flow_control`) as the foundation, cut run persistence over to the local store for both modes, then realize the review loop as the engine's first template (declared tool + skill + built-in + board). Domain meaning is data; the model supplies the intelligence under a skill.
- **Sequencing logic (bottom-up, each task independently testable):**
  1. **Task-089** (`P-1`) Generic flow vocabulary + `flow_control` handler (pure, no behavior change).
  2. **Task-090** (`P-2`) Bounded flow runtime executor: spawn/join/route/reinvoke + cap, hub-only (synchronous `wait=true` path first).
  3. **Task-085** (`P-3`) Unified local run persistence: route all run data through `localFileSessionStore`; resume restore; deprecate the Supabase run path; verify Drive + HTTP read.
  4. **Task-091** (`P-4`/`P-5`/`P-10`) Review-loop template: `submit_review_outcome` declared face + review config over the executor + gate the legacy keyword path off in explicit mode.
  5. **Task-092** (`P-6`) Consolidated multi-reviewer note (the `join`/deliver payload).
  6. **Task-093** (`P-7`) Opt-in auto-reinvocation of the hub after a cohort `join`.
  7. **Task-094** (`P-8`) `agent-review-loop` skill + `synthesizer` built-in (the first FlowDefinition template).
  8. **Task-095** (`P-9`) Board + contract + client surface.
- **Dependencies:** Task-090→089. Task-091→089+090. Task-092→090. Task-093→091+092. Task-095→089/090 DTOs + Task-084 board. Task-085 is independent (wiring) and can land in parallel with 089/090. Task-094 is independent but only useful once 091 lands. **Prerequisite:** CP-19 Task-082 (done) + Task-084 (orchestrator/bus/board).

## 4. Work Breakdown

> Eight task-sized slices over the existing 084–095 ID range (no new IDs). Each maps to one child task doc with its own full Definition of Done. Two cohesive merges: Task-091 carries the review tool + config + legacy gate; Task-092 carries the join note.

### Task-089 — Generic flow vocabulary + `flow_control` handler (`P-1`)

- **Types** (`agent_orchestrator.go`, new domain-free section): `FlowNode{ID,Agent,Run,Lifecycle,Join}`, `FlowEdge{From,To,When,Kind}`, `FlowPolicy{Cap,OnCap,ExtendBy,ExtendMax}`, `FlowControlInput{Status,Summary,Payload}`, `FlowControlResult{Status,Round,Cap,OpenIssues,NextAction}`.
- Defaults: `Run=delegate`, `Lifecycle=reinvoke`, `Join=all`, `Cap=3`, `OnCap=escalate`, `ExtendBy=2`, `ExtendMax=2`.
- `parseFlowControlInput(map[string]any)` mirrors `parseSpawnAgentInput:344`; validate `Status ∈ {continue,done,escalate}`.
- **Declared-face registry:** a data map so a domain tool declares `domainStatus → generic flow_control status` (`approved→done`, `changes_requested→continue`, `blocked→escalate`). One handler, many faces.
- **Edge semantics (G5/G6):** `When` matches the **mapped generic status**; `quorum(n)` parsed for `Join`; load-time validation rejects >1 back-edge target per status.
- **DoD:** see Task-089 `§6` (parse valid/invalid; defaults applied; quorum parse; face-map resolves all three review statuses; no behavior change).

### Task-090 — Bounded flow runtime executor (`P-2`, with `run: inline` per G2)

- **`AgentLoopState`** (`provider_event.go` + `contract.ts:123`) generalized: `Status` (`idle|running|waiting_join|synthesizing|blocked|done|stopped`), `Round`, `Cap`, `GateReason`, `OpenIssues`, `Mode` (`keyword|explicit`), `ActiveNode`, `ExtendCount`.
- **Executor (hub-only, no role knowledge):** `spawnNode` (`inline`→hub turn; `delegate`→`turnBridge.SpawnAgent` with `dependsOn`), `joinSatisfied` (reuse `dependenciesSatisfiedLocked` for `all/any/quorum`), `route` (fire out-edges matching status; `Kind=="back"` increments round, rejected at `round>=cap`), `applyFlowControl` (done/continue/escalate transitions; reinvoke via `scheduleChildTurn`/`resumePendingLoopWork:899-927`, once via fresh spawn), `extendCap` (+ExtendBy, reject past ceiling).
- **Bridge:** add `SubmitFlowControl` to `TurnBridge` (`provider_registry.go:25-32`); implement near `:1174`; update all fake bridges. **Routes:** `POST .../flow-control`, `POST .../agent-loop/extend-cap` (`interactive_handlers.go:41-48`, mirror `:860`).
- **DoD:** see Task-090 `§6` (done/continue/escalate; cap boundary; reinvoke vs once; join all/any/quorum out-of-order; inline executes on hub; extend bounded; **domain-free guard test**; HTTP round-trips).

### Task-085 — Unified local run persistence (`P-3`; repurposed, supersedes original Supabase plan)

- Make `localFileSessionStore` the explicit sole **run** sink for both modes (production already wires it at `cli/root.go:120-123`); mark `SupabaseWorkflowStore` run methods deprecated + unwired.
- Extend the session-snapshot serializer (`interactive_service.go:744-762`) and `interactive_resume.go` to persist + restore flow fields (`mode`, `round`, `cap`, `activeNode`, `extendCount`, `autoOrchestrate`, cohort/edge state) — closes **G1**.
- Flow/Step **definitions** stay on Supabase (catalog reads unchanged). Verify `chat_session_sync.go` carries the extended manifest cross-PC; verify desktop reads run state via `http-local-runner-gateway.ts`, not Supabase.
- **DoD:** see Task-085 `§6` (flow run survives restart from `sessions.ndjson`; Drive round-trips a flow run; no production write to `workflow_run_*`; definition reads still hit Supabase; single-agent regression green).

### Task-091 — Review-loop template: outcome tool, config, legacy gate (`P-4`+`P-5`+`P-10`)

- **Outcome tool (declared face):** `ReviewIssue`, `ReviewOutcomeInput{Status,Issues,Feedback,RoundCapOverride}`; `parseReviewOutcomeInput` validates `status ∈ {approved,changes_requested,blocked}`, requires `feedback` on `changes_requested`, maps onto `FlowControlInput` (issues in `Payload`). Register on Claude (`claude_mcp_server.go` + `claude_permission_mcp.go`) and Codex (`codex_adapter.go`), start+resume, reserved-name alias if needed. **Only this tool is registered** (G3).
- **Review config:** the loop as data — nodes `coder`(`reinvoke`) + `reviewer×N`(`dependsOn:coder`) + synthesis(`run:inline`,`join:all`), back-edge `synthesis→coder when continue` (`cap:3`). Coder re-entry prompt composes `Issues`(numbered, grouped by `File`)+`Feedback`, merging `takeQueuedFeedbackPrompt`.
- **Legacy gate (P-10):** wrap `interactive_service.go:884-935` keyword logic in `if loopState.Mode != "explicit"`. In explicit mode a completing reviewer only contributes to the cohort note; `flow_control` is the sole driver.
- **DoD:** see Task-091 `§6` (tool advertised both providers start+resume; mapping correct; loop runs with no bespoke transition branch; explicit mode → zero double restarts; keyword mode byte-for-byte unchanged).

### Task-092 — Consolidated multi-result join note (`P-6`)

- Add `flowCohortId` to `SpawnAgentInput` + child `interactiveRun` (additive, near `:103-110`). On a cohort child's `EventTurnCompleted`, accumulate into `cohortResults` instead of one isolated line (`:870-879,944-947`). When `Join` is satisfied, build ONE consolidated note (per-member label + truncated 1500) + a synthesis directive, appended via `appendPendingAgentContextLocked` (single entry, persisted in `sessions.ndjson`).
- **DoD:** see Task-092 `§6` (3 out-of-order children → exactly one note after the last; failed child shown as `failed:`; domain-free labels).

### Task-093 — Bounded auto-reinvocation of the hub (`P-7`)

- Add `autoOrchestrate bool` to the parent `interactiveRun`; set when a flow starts; persist for resume (via Task-085). `maybeAutoReinvokeHub` at cohort-`join` completion; guards: auto on, status ∉ {paused,stopped,blocked,done}, `!turnInFlight`, single-flight `reinvokeInFlight`, round < cap. Action: `scheduleChildTurn` with the join note via the system-note path (SD-16 §14.3), never a user bubble. `stopAgentLoop:356` clears flags.
- **DoD:** see Task-093 `§6` (fires once per join with auto on; never when paused/stopped/blocked/at-cap/in-flight; Stop cancels; non-flow chat never auto-reinvokes).

### Task-094 — `agent-review-loop` skill + `synthesizer` built-in (`P-8`)

- **Built-in** (`agent_catalog.go:391`): `synthesizer` (Tools `Read,Grep,Glob`; consolidate→dedup→resolve→call the control tool; never restart the coder; `Source:flowpilot`).
- **Skill** `.claude/skills/agent-review-loop/SKILL.md` (+`.codex` mirror): spawn `coder`(`wait=true`); spawn reviewer cohort (`correctness`+`security`, `wait=false`, `dependsOn:[coderRunId]`, same `flowCohortId`, `AutoOrchestrate:true`; add `regression` when tested behavior changes); end turn; on join note synthesize + call `submit_review_outcome`; on `blocked` call `ask_user` (Extend+2/Accept/Stop). Cap + stop conditions explicit.
- **DoD:** see Task-094 `§6` (`synthesizer` discoverable + file-overridable; skill lints; manual scenario passes).

### Task-095 — Orchestration board, contract, client (`P-9`)

- **`contract.ts`:** add `FlowControlInput`/`Result`, `ReviewIssue`/`ReviewOutcomeInput`/`Result`; extend `AgentLoopState:123` (`openIssues?`,`mode?`,`activeNode?`); client methods `submitReviewOutcome?`/`extendCap?`.
- **Clients + store:** implement in `HttpWsRunnerClient.ts`/`MockRunnerClient.ts`; add store actions; surface from `agent_graph_updated` SSE.
- **`OrchestrationBoard.tsx`:** generalize beyond hardcoded coder+reviewer (`:42-48`) — hub node + a row of child nodes per cohort, round/cap, open count, status; on `blocked` render **Extend cap** + `gateReason`; keep pause/resume/stop/inject.
- **DoD:** see Task-095 `§6` (renders N children, round/cap, open count; `blocked` + extend control; actions update snapshot; single-node fallback).

## 5. Touched Areas

- **backend:** `agent_orchestrator.go`, `interactive_service.go`, `interactive_resume.go`, `provider_registry.go`, `provider_event.go`, `codex_adapter.go`, `claude_mcp_server.go`, `claude_permission_mcp.go`, `interactive_handlers.go`, `agent_catalog.go`; persistence: `local_file_session_store.go`, `chat_session_sync.go`, `cli/root.go`, `supabase_workflow_store.go` (deprecate run path); tests: `agent_orchestrator_test.go`, `interactive_service_test.go`, `local_file_session_store_test.go`, `chat_session_sync_test.go`, `claude_permission_mcp_test.go`, `claude_mcp_server_test.go`, `codex_appserver_test.go`, every fake bridge implementing `TurnBridge`.
- **frontend:** `types/contract.ts`, `state/store.ts`, `components/OrchestrationBoard.tsx`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `state/store.test.ts`, `styles.css`.
- **skill/config:** `.claude/skills/agent-review-loop/SKILL.md` (+ optional `.codex` mirror).
- **modules:** local-runner flow engine + provider adapters; local run persistence + Drive sync; desktop board + state.
- **database:** none for run data (local file). Flow/Step **definition** tables on Supabase unchanged. Legacy `workflow_run_*` deprecated (not dropped in this CP).
- **external systems:** Drive sync (existing) carries the extended session manifest. No new external system.

## 6. Data or Migration Steps

- **schema:** no new run migration. Run state is additive fields on the session record in `sessions.ndjson`; flow/step definitions stay on existing Supabase tables.
- **data backfill:** none — absent fields default to legacy (`mode=""→keyword`, `autoOrchestrate=false`); existing `workflow_run_*` rows ignored, not migrated.
- **config updates:** ship `synthesizer` built-in + `agent-review-loop` skill in-repo; defaults `cap=3`, `extendBy=2`, `extendMax=2`.
- **deprecation:** `SupabaseWorkflowStore` run methods marked deprecated + unwired in production; type retained for compile/back-compat. CP-19 Task-085's original Supabase scope is superseded here.

## 7. Validation Plan

- **tests to add:**
  - Go unit (engine): `parseFlowControlInput`; node/edge/policy defaults; `applyFlowControl` (done/continue/escalate, cap boundary, override, bounded extend); `joinSatisfied` (all/any/quorum out-of-order); back-edge re-entry (reinvoke vs once); inline-on-hub; **domain-free guard** (no role strings in executor); `maybeAutoReinvokeHub` guards + single-flight + Stop-cancels; explicit-mode gate.
  - Go unit (review template): `parseReviewOutcomeInput` + mapping to `flow_control`; consolidated cohort note (out-of-order, failed child); review loop runs with no bespoke transition branch.
  - Go (persistence): flow run survives restart from `sessions.ndjson`; Drive sync round-trips a flow run; no production write to `workflow_run_*`; definition reads still hit Supabase; resume restores flow fields.
  - Go contract: `tools/list` advertises `submit_review_outcome` (Claude + Codex, start + resume); `POST .../flow-control` and `.../agent-loop/extend-cap` round-trips; additive `AgentLoopState` JSON.
  - Frontend: board renders N children, round/cap, open count, `blocked` + extend control; store actions; SSE snapshot updates; single-node fallback.
- **manual checks (desktop):**
  1. YOLO on. *"Use the agent-review-loop skill: coder adds input validation to X, then 2 reviewers (correctness + security) review until clean, max 3 rounds."*
  2. Verify coder → 2 reviewers parallel → board round 1 + both nodes → join note triggers a synthesis turn (no user typing) → `submit_review_outcome` posts a verdict → on `changes_requested` the coder re-enters with merged feedback → loop continues.
  3. Force a conflict → synthesis resolves into ONE issue list, not two restarts.
  4. Drive to cap with open issues → board `blocked` + Extend cap → agent calls `ask_user` → "Extend +2" resumes.
  5. Stop mid-loop → no further reinvocation.
  6. Restart server mid-loop → resume from `sessions.ndjson` keeps `autoOrchestrate`/round and continues (no Supabase).
  7. Open the run on a second machine after Drive sync → run state present.
- **failure cases:** a child fails mid-turn (note records `failed`, synthesis proceeds); invalid `flow_control`/review status (tool error, no state change); auto-reinvoke while a turn is in flight (suppressed); cap=0 / override 0 (treated as default 3); a non-flow normal chat never auto-reinvokes; Drive offline (local run proceeds; syncs later).

## 8. Rollout and Fallback

- **rollout order:** Task-089 → 090 → 085 (parallel-OK) → 091 → 092 → 093 (auto) → 094 (skill/agent) → 095 (UI). Landing 089/090/085/091 gives a working **synchronous** flow (hub drives rounds with `wait=true` spawns + explicit `flow_control`); 092/093 upgrade to parallel + auto.
- **fallback path:** all additions additive and mode-gated. `autoOrchestrate=false` + `mode="keyword"` reverts to current CP-19 behavior. Not registering `submit_review_outcome` / not shipping the skill leaves single-agent + single-reviewer flows untouched. The persistence cutover is a no-op in production (already local); the deprecated Supabase run type stays compiled.
- **monitoring:** reuse `[agent-spawn]`; add `[flow]` lines for round transitions, control submissions, reinvoke fire/suppress, cap-hit, persistence writes. Surface bus log + round/issue state on the board.

## 9. Risks

- `R-1` **Over-generalizing the engine.** Mitigation: minimum vocabulary (`node/edge/policy` + one control signal), validated by THREE concrete templates (review loop here, Plan→Coding→Testing in CP-41, ad-hoc 2-dev+reviewer); no FlowCatalog/`agent_flow`-step bridge built here.
- `R-2` **Unbounded auto-reinvocation.** Mitigation: Task-093 hard guards (cap, single-flight, status gate, Stop clears flag) + `[flow]` reinvoke-count log; cap mandatory; `blocked` halts.
- `R-3` **N children racing N node restarts.** Mitigation: Task-091 gates the legacy keyword path off in explicit mode; only `flow_control` re-enters a node.
- `R-4` **Domain leakage into the engine.** Mitigation: grep/review guard test asserts no role strings (`coder`/`reviewer`/`approved`) in the executor; review meaning lives only in the declared-face map + skill.
- `R-5` **Synthesis quality.** Mitigation: explicit skill protocol + `synthesizer` built-in; conflicts surfaced in the join note; human inspects board + `ask_user` gate.
- `R-6` **Persistence cutover regresses a remote reader.** Mitigation: desktop reads via HTTP gateway (verified); legacy admin-web out of scope; Drive cadence (not realtime) accepted; flow state proven to survive restart from `sessions.ndjson`.
- `R-7` **Resume drift.** Mitigation: Task-085 persists `mode`/`round`/`cap`/`activeNode`/`autoOrchestrate`/`flowCohortId` in `sessions.ndjson` and restores in `interactive_resume.go` (mirror existing `pendingAgentContext` restore).
- `R-8` **Provider tool-name reservation** for `submit_review_outcome` on Codex. Mitigation: Task-091 checks the reserved list and aliases to `flowpilot_submit_review_outcome`, normalizing at the boundary.
- `R-9` **Task-084 not yet landed.** Mitigation: declared prerequisite (`Constraints`); if Task-084 DTOs shift, Task-090/095 rebase on the frozen contract.

## 10. Definition of Done

- [ ] `DOD-1` (Task-089) A domain-free vocabulary (`FlowNode`/`FlowEdge`/`FlowPolicy`) + one generic `flow_control` handler exist, with defaults applied, `quorum(n)` parsed, and the declared-face map resolving `approved/changes_requested/blocked` → `done/continue/escalate`. Unit tests green; no behavior change.
- [ ] `DOD-2` (Task-090) The executor runs flows hub-only via `spawnNode`(inline+delegate) / `joinSatisfied`(all/any/quorum) / `route`(forward + bounded back-edge) / `applyFlowControl`, with **no role-specific Go** (proven by the domain-free guard test). `SubmitFlowControl` bridge + both HTTP routes round-trip.
- [ ] `DOD-3` (Task-090) Bounded retries: a back-edge increments the round, is rejected at `round>=cap` (→`blocked`), and `extendCap` raises by +2 at most twice (ceiling cap+4); `reinvoke` restarts with feedback, `once` respawns.
- [ ] `DOD-4` (Task-085) **All run data (chat + flow) persists through `localFileSessionStore`** and survives a runner restart with **no Supabase**; resume restores `mode/round/cap/activeNode/autoOrchestrate/flowCohortId`.
- [ ] `DOD-5` (Task-085) Drive syncs a flow run cross-PC; **no production path writes** `workflow_run_logs`/`workflow_run_sessions`/`workflow_run_steps`; flow/step **definitions** still read from Supabase; single-agent runs unaffected.
- [ ] `DOD-6` (Task-091) `submit_review_outcome` is registered and callable on **both Claude and Codex (start + resume)**, is the **only** flow tool the model sees, maps onto `flow_control`, and is the sole driver of the loop in explicit mode.
- [ ] `DOD-7` (Task-091) The review-until-clean loop runs as **config** over the executor (no bespoke transition branch), terminating on `done` or asking the user at the cap — never silently, never unbounded; the legacy keyword loop is **byte-for-byte unchanged** when `mode != "explicit"`.
- [ ] `DOD-8` (Task-092) N reviewer results arrive as **one** consolidated, per-member-labelled join note after the last completes (out-of-order safe; failed child shown as `failed:`); the parent synthesizes and resolves conflicts into a single issue list.
- [ ] `DOD-9` (Task-093) Auto-reinvocation is opt-in, single-flight, cap-bounded, Stop-cancellable, fires exactly once per cohort join, and **never fires on a normal chat run**.
- [ ] `DOD-10` (Task-094/095) The `agent-review-loop` skill + `synthesizer` built-in ship in-repo; the Orchestration Board renders N children, round/cap, open count, and a `blocked` + Extend-cap control; pause/resume/stop/inject still work.
- [ ] `DOD-11` The engine is the substrate **CP-41** consumes with **no further agent-interaction changes** (verified by CP-41 referencing these types/handlers, not adding new coordination code).
- [ ] `DOD-12` Normal chat, workflows, session resume, and Drive sync show no regressions; all §7 unit/contract/frontend tests pass; GitNexus impact analysis was run for each edited symbol and `gitnexus_detect_changes()` was clean before commit.



## Verdict: 🟢 Green — ready to start

I verified every task in the chain against the actual code (build + targeted tests run, not just grep). Here's the board:

| Task | Status | Code reality | Ready? |
|---|---|---|---|
| **Task-082** spawn tool/orchestrator core | done ✅ | `spawn_agent` on both providers, hardened by BUG-099…134 | foundation |
| **Task-083** agents panel | done ✅ | `OrchestrationBoard.tsx` + panel present | foundation |
| **Task-084** dep loop + board | ~~draft~~ → **done** ✅ (corrected + moved to `done/`) | routes + keyword loop (`interactive_service.go:916-982`) + bus + board + tests; **build clean, tests pass** | foundation |
| **Task-089** flow vocabulary | draft (greenfield) | nothing exists yet — extends nothing | **▶ start here** (zero deps) |
| **Task-090** executor | draft (greenfield) | extends real `AgentLoopState{Status,Round,RoundCap,GateReason}` + `TurnBridge` — deltas match code exactly | ready (after 089) |
| **Task-085** local persistence | draft (greenfield delta) | foundation works: `localFileSessionStore`, `pendingAgentContext` persist+restore (`interactive_resume.go:367`), Drive sync | ready (parallel) |
| **Task-091** review template | draft (greenfield) | gate target located (`:916-982`); registration mirrors `spawn_agent` | ready (after 089/090) |
| **Task-092** join note | draft (greenfield) | extends `appendPendingAgentContextLocked` (exists) | ready |
| **Task-093** auto-reinvoke | draft (greenfield) | extends `releaseDependentAgents`/`scheduleChildTurn` (exist) | ready |
| **Task-094** skill + synthesizer | draft (greenfield) | `agent_catalog.go` exists to extend | ready |
| **Task-095** board generalize | draft (greenfield) | `OrchestrationBoard.tsx` + 6 routes exist to extend | ready |

