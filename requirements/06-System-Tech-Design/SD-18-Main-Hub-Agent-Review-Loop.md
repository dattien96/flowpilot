# SD-18: Main-Hub Agent Review Loop

## Metadata

- Document ID: `SD-18`
- Title: `Main-Hub Agent Review Loop And Verdict Tool`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [SS-15: Agent Review Loop (Review Until Clean)](../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- Related Documents: [SD-19: Agent Flow Engine](./SD-19-Agent-Flow-Engine.md) (the generic engine this design is the first template of), [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SD-13: Multiple Agents](./SD-13-Multiple-Agents.md), [SS-16: Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-12: Multiple Agents](../05-System-Specs/SS-12-Multiple-Agents.md), [SS-08: Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, main-hub, verdict-tool, auto-reinvoke, synthesis, local-runner, desktop`

## AI Quick View

### Summary

- Designs a **main-chat-as-hub** review loop on top of the existing CP-19/SD-16 spawn + dependency-barrier + result-sharing stack: a coder produces a change, **N reviewers** assess it in parallel, the main agent **synthesizes + resolves conflicts**, and the loop repeats until clean or capped.
- Replaces brittle keyword verdict detection with an **explicit `submit_review_outcome` tool** (`{status, issues[], feedback, roundCapOverride}`) that is the single, observable driver of the loop.
- Adds an **opt-in, bounded, single-flight auto-reinvocation** primitive so the runner re-prompts the orchestrating agent after a reviewer cohort completes — advancing the loop with no human in the middle (the concrete build of SD-16 §14.5).
- Reviewers run **in parallel via `dependsOn:[coderRunId]`**; their results are folded into **one consolidated, per-reviewer-labelled note** delivered through the existing `pendingAgentContext` channel.
- The loop is **bounded + honest**: cap reached with open issues → `blocked` status → the orchestrator asks the user; nothing is auto-committed. All additive; legacy single-reviewer flow is gated behind a `mode` flag.

### Current Ask

- Settle the runtime design: the verdict contract, who synthesizes, how reviewers run in parallel and report, how the loop advances and terminates, and how it stays bounded and observable — enough for CP-36 to implement without design questions.

### Key Decisions

- `D-1` **Main agent is the hub/synthesizer** (default); reviewers never message each other. A dedicated `synthesizer` built-in is shipped as an optional offload.
- `D-2` **Explicit verdict tool** `submit_review_outcome` is the sole loop driver in the new "explicit" mode; the legacy keyword path is gated off.
- `D-3` **Opt-in auto-reinvocation** re-prompts the orchestrator when a reviewer cohort completes; bounded by cap, single-flight, cancelled by Stop, off for normal runs.
- `D-4` **Parallel reviewers via `dependsOn`**, reusing the verified barrier (`dependenciesSatisfiedLocked` + `releaseDependentAgents`).
- `D-5` **Consolidated cohort note** folds all reviewer results into one labelled `pendingAgentContext` entry for synthesis.
- `D-6` **Bounded + ask-on-cap**: a new `blocked` loop status routes to a user decision; no silent termination.
- `D-7` **Additive, no migration**: new tool, new loop-state fields, new built-in/skill, new routes; Phase-1 chat-mode persistence via the existing manifest.
- `D-8` **Default cohort = correctness + security** (two reviewers, distinct lenses); the skill may add a `regression` lens when the change touches tested behavior. Minimum two reviewers per round (SS-15 `BR-9`).
- `D-9` **Bounded extension** at the cap: extend only in +2-round increments, at most twice (hard ceiling cap + 4), enforced by the `extend-cap` route/orchestrator. Beyond the ceiling only Accept/Stop remain — guarantees termination under repeated extends (SS-15 `BR-10`, closes `R-1` sub-risk).

### Constraints

- No change to `ProviderRuntimeAdapter` or the SSE transport except additive fields/variants (SD-16 boundary).
- Reuse spawn / `dependsOn` / `pendingAgentContext` / bus / board; do not invent a new transport.
- The auto-reinvocation loop MUST be provably bounded and stoppable (release blocker otherwise).
- No Supabase migration (Phase 1). Loop state persists in the existing chat manifest and restores on resume.
- GitNexus impact analysis before editing any named symbol; warn on HIGH/CRITICAL (CLAUDE.md).

### Open Questions

- `Q-1` **Resolved** (`D-1`): default synthesizer is the **main agent**; the `synthesizer` child is an optional offload.
- `Q-2` `submit_review_outcome` name reservation on Codex (as with `spawn_agent` → `flowpilot_spawn_agent`, SD-16 `D-13`) — alias to `flowpilot_submit_review_outcome` if reserved. *Open until verified against the live app-server reserved list.*
- `Q-3` **Resolved** (`D-9`): `extend cap` is bounded to +2 ×2 (ceiling cap + 4); termination is guaranteed.

### Source Refs

- `SS-15` `AC-1`…`AC-11`, `BR-1`…`BR-8`.
- `SD-16` `D-1`…`D-15`, §7 (flow), §14 (result sharing), §14.5 (auto-mode foundation), §15 (runtime rules).
- Code anchors (verified 2026-06-23): `agent_orchestrator.go` (`AgentLoopState`, `transition`, `advanceRound`, `addBus`, `queueFeedback`, `parseSpawnAgentInput`); `interactive_service.go:394-571` (`dependenciesSatisfiedLocked`, `releaseDependentAgents`, `resumePendingLoopWork`, `scheduleChildTurn`, `takeQueuedFeedbackPrompt`), `:880-998` (turn-completed coder/reviewer logic + `releaseDependentAgents` call at `:997`), `:1174` (`turnBridge.SpawnAgent`), `appendPendingAgentContextLocked`; `provider_registry.go:25-32` (`TurnBridge`); `codex_adapter.go:89-107,299-318`; `claude_mcp_server.go:235-296`, `claude_permission_mcp.go:182-195`; `interactive_handlers.go:41-48`; desktop `OrchestrationBoard.tsx`, `state/store.ts`, `types/contract.ts:91-136,461-466`.

## 1. Goal

Define a runtime in which the **main chat agent coordinates a bounded, multi-reviewer, review-until-clean loop**, reusing FlowPilot's existing agent machinery and adding only: an explicit verdict tool, a deterministic loop driver, an opt-in auto-reinvocation primitive, a consolidated result note, a skill, and a built-in agent. The design must satisfy SS-15 `AC-1`…`AC-11` and `BR-1`…`BR-8` without regressing existing flows.

## 2. Input Documents

- [SS-15: Agent Review Loop (Review Until Clean)](../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) — specifically `AC-1`…`AC-11`, `BR-1`…`BR-8`.
- [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md) — the spawn/tool/result-sharing substrate this design extends.

## 3. Architecture Decision

- `D-1` **Main agent as hub and synthesizer.** Cross-agent information flows only through the parent (SS-15 `BR-1`). The parent agent consolidates reviewer findings and resolves conflicts (`AC-3`, `BR-3`). A `synthesizer` built-in agent is provided so the main agent can optionally offload synthesis to a focused child, but the default is the main agent itself.
  - *Alternatives considered:* (a) peer-to-peer reviewer debate — rejected: no arbiter, context bloat, breaks isolation (SD-16 `D-6`); (b) a pure runner-side deterministic synthesizer — rejected: conflict resolution needs task semantics only an LLM with full context has.
  - *Why chosen:* the main agent already holds the original task context, making it the correct arbiter, and isolation is preserved.
- `D-2` **Explicit verdict tool replaces keyword detection.** `submit_review_outcome({status, issues[], feedback, roundCapOverride?})` is the only driver of the loop when a run is in `mode="explicit"`. The current substring detection (`interactive_service.go:884-935`) is gated off in that mode so N reviewers cannot each trigger a coder restart (`AC-7`).
  - *Alternatives:* keep parsing the reviewer's free text — rejected: brittle and unsafe with multiple reviewers.
  - *Generic-engine note:* `submit_review_outcome` is a **declared instance** of the generic `flow_control` signal (SD-19 `D-4`); its `approved/changes_requested/blocked` map to `done/continue/escalate`. The engine handler is domain-free; only this tool's name and schema are review-specific.
- `D-3` **Opt-in auto-reinvocation.** A per-parent `autoOrchestrate` flag lets the runner re-prompt the orchestrating agent once a reviewer cohort completes. Guards: flag on, loop status runnable, no parent turn in flight, single-flight, round < cap. This is the concrete realization of SD-16 §14.5.
  - *Alternatives:* require the user to type each round (status quo) — rejected: fails `AC-5`; or one giant `wait=true` main turn that runs all rounds inline — kept as the Phase-1a fallback but not the target (long turn, poor resumability).
- `D-4` **Parallel reviewers via `dependsOn`.** Reviewers are spawned `wait=false`, `dependsOn:[coderRunId]`, sharing a `reviewCohortId`. The verified barrier (`dependenciesSatisfiedLocked` checks all deps `RunStatusCompleted`; `releaseDependentAgents` fires at `interactive_service.go:997`) starts them when the coder completes (`AC-1`).
- `D-5` **Consolidated cohort note.** When every child of a `reviewCohortId` is terminal, the runner builds ONE labelled note ("Reviewer X (provider): …") and appends it via `appendPendingAgentContextLocked`, so it folds into the orchestrator's next turn exactly like existing result notes (SD-16 §14.3) and survives restart (`AC-2`, `AC-10`).
- `D-6` **Bounded loop with ask-on-cap.** `AgentLoopState` gains `openIssues` and a `blocked` status. `changes_requested` at the cap sets `blocked` + a gate reason instead of restarting the coder; the skill instructs the orchestrator to call `ask_user` (`AC-6`, `BR-4`, `BR-5`).
- `D-7` **Additive, no migration.** All new state lives in orchestrator maps + additive `interactiveRun`/`AgentLoopState` fields persisted to the existing chat manifest (`sessions.ndjson`). Legacy behavior is the default when fields are zero-valued (`AC-11`, `BR` compatibility).
- `D-8` **Default reviewer cohort.** The `agent-review-loop` skill spawns two reviewers by default — lenses `correctness` and `security` — and may add a third `regression` reviewer when the change touches previously-tested behavior. Two is the enforced minimum (SS-15 `BR-9`, `Q-2`). The lens set is skill config, not hard-coded in the runner, so it is tunable without a code change.
  - *Alternatives:* a single generalist reviewer — rejected: fails the "more classes of problems" goal (`US-2`); fully dynamic count — deferred: harder to make the board legible.
- `D-9` **Bounded extension.** At the cap the orchestrator presents Extend / Accept / Stop. Extend raises the cap by 2 and is allowed at most twice (hard ceiling = initial cap + 4), enforced in `submitReviewOutcome`/the `extend-cap` route by tracking an `extendCount`. Beyond the ceiling the extend option is withdrawn, leaving only Accept or Stop — so no sequence of user actions can make the loop run forever (SS-15 `BR-10`, closes the `R-1` sub-risk).

## 4. Component Impact

- **Impacted modules:**
  - local-runner orchestrator (`agent_orchestrator.go`): verdict types, loop-state fields, cohort buffers.
  - local-runner interactive service (`interactive_service.go`): `submitReviewOutcome`, consolidated note, `maybeAutoReinvokeOrchestrator`, explicit-mode gate, bridge method.
  - provider adapters (`codex_adapter.go`, `claude_mcp_server.go`, `claude_permission_mcp.go`): register + dispatch `submit_review_outcome`.
  - HTTP surface (`interactive_handlers.go`): `review-outcome`, `agent-loop/extend-cap` routes.
  - agent catalog (`agent_catalog.go`): `synthesizer` built-in.
  - desktop board + state + contract + clients.
- **New modules:** the `agent-review-loop` skill; the `synthesizer` built-in definition.
- **Unchanged modules:** `ProviderRuntimeAdapter` interface, base SSE transport (reused additively), provider model internals, single-agent run lifecycle.

## 5. Data Model

- **Entities:**
  - `ReviewIssue` — `{ id?, title, severity?, file?, resolution? }`: one actionable finding after synthesis.
  - `ReviewOutcomeInput` — `{ status: approved|changes_requested|blocked, issues[], feedback?, roundCapOverride? }`: the orchestrator's verdict for a round.
  - `ReviewOutcomeResult` — `{ status, round, roundCap, openIssues, nextAction: looping|done|awaiting_user }`: returned to the agent.
  - `AgentLoopState` (extended) — adds `openIssues`, `mode` (`keyword`|`explicit`), `coderRunId`.
  - `interactiveRun` (extended, additive) — adds `reviewCohortId`, `autoOrchestrate`.
  - Reviewer cohort buffer — orchestrator map `reviewCohortId → []{runId, agentName, provider, finalMessage|error, status}`.
- **Fields of note:**
  - `reviewCohortId`: groups the reviewers of one round so their completion forms a single barrier + a single consolidated note.
  - `autoOrchestrate`: opt-in flag enabling runner re-prompting of the orchestrator.
  - `mode`: selects the explicit verdict driver vs the legacy keyword loop.
- **State transitions (loop status):**
  - `idle → running` (coder spawned) `→ waiting_review` (reviewers running) `→ synthesizing` (cohort complete, orchestrator re-invoked) `→` verdict:
    - `approved` → terminal clean.
    - `changes_requested` (round < cap) → `running` (coder restarted) → … (new cohort).
    - `changes_requested` (round == cap) → `blocked` → (user) `→ running` on extend, or `stopped`/`approved` per choice.
    - `blocked` (explicit) → user decision.
    - any → `stopped` (user Stop).
  - Parent graph mirrors child status so the UI never appears hung (SD-16 `R-2`).

## 6. Interfaces and Contracts

- **Provider tool contract** (registered on both providers; Codex aliases to `flowpilot_submit_review_outcome` if the name is reserved — `Q-2`, SD-16 `D-13`):
  ```
  submit_review_outcome({
    status: "approved" | "changes_requested" | "blocked",   // required
    issues?: [{ id?, title, severity?, file?, resolution? }],
    feedback?: string,                                       // required when changes_requested
    roundCapOverride?: number
  })  ->  { status, round, roundCap, openIssues, nextAction }
  ```
  Registered alongside `spawn_agent`/`ask_user` via the same channels: Claude MCP `tools/list` + `tools/call` (`claude_mcp_server.go:264-296`, `:250-259`); Codex `DynamicToolSpec` + `handleDynamicToolCall` (`codex_adapter.go:133`, `:289-318`). Backed by a new `TurnBridge.SubmitReviewOutcome` (`provider_registry.go:25-32`).
- **HTTP contracts** (mirror the tool for UI/debug; additive to `interactive_handlers.go:41-48`):
  - `POST /client/workflow-runs/{runId}/review-outcome` — body = `ReviewOutcomeInput` → `AgentGraphSnapshot`.
  - `POST /client/workflow-runs/{runId}/agent-loop/extend-cap` — body `{roundCap}` → `AgentGraphSnapshot` (resumes a `blocked` loop).
- **SSE contract:** reuse existing additive `agent_graph_updated` / `agent_bus_message` events (SD-16 §15.3); the loop state changes ride the snapshot. New bus `kind` values: `review-outcome`, `round-advanced`, `loop-blocked`.
- **Result-sharing contract:** the consolidated cohort note uses the existing `pendingAgentContext` mechanism (SD-16 §14.3) — one entry, folded into the orchestrator's next turn, persisted in `sessions.ndjson`.
- **Catalog contract:** new `synthesizer` built-in (`agent_catalog.go`); overridable by an on-disk `.claude/agents`/`.codex/agents` file of the same name (SD-16 `D-5`).

## 7. Execution Flow

1. User asks to "review until clean"; the main agent loads the `agent-review-loop` skill.
2. Main agent spawns `coder` (`wait=true`) to produce the change; captures `coderRunId`. Loop `mode="explicit"`, `autoOrchestrate=true`, round = 1.
3. Main agent spawns the reviewer cohort: 2–3 `reviewer` agents, `wait=false`, `dependsOn:[coderRunId]`, shared `reviewCohortId`, distinct lenses. The main turn ends.
4. Coder completes → barrier releases the reviewers (`releaseDependentAgents`); they run in parallel, each on its own stream (`AC-1`).
5. Each reviewer completes; the runner accumulates results into the cohort buffer. When the **last** cohort member is terminal, it builds the **consolidated note** (`D-5`) and calls `maybeAutoReinvokeOrchestrator` (`D-3`).
6. If the guards pass, the runner re-prompts the orchestrator (a parent turn seeded with the consolidated note + synthesis directive — not a user-visible bubble).
7. The orchestrator synthesizes: dedup, resolve conflicts using task context (`AC-3`, `BR-3`), then calls `submit_review_outcome`:
   - `approved` (no issues) → loop terminal-clean; a handoff note summarizes to the user (`AC-4`). `autoOrchestrate` cleared.
   - `changes_requested` + `feedback` → `submitReviewOutcome` advances the round; if `round < cap`, the coder run is restarted with the consolidated feedback (reusing the existing restart path `interactive_service.go:899-927`), returning to step 4 (`AC-5`); if `round == cap`, status → `blocked` (`AC-6`).
   - `blocked` → awaiting user.
8. On `blocked`, the skill has the orchestrator call `ask_user` (extend / accept / stop). Extend → `extend-cap` route resumes looping; accept → treated as approved; stop → `stopAgentLoop` (`AC-6`, `AC-9`, `BR-5`, `BR-7`).
9. Every transition emits an updated graph snapshot + bus message so the board reflects agents, findings, round/cap, open-issue count, and verdict (`AC-8`, `BR-8`).

## 8. Failure and Edge Handling

- `F-1` A reviewer fails mid-turn — recorded in the cohort buffer as `failed: <error>`; the consolidated note includes it; synthesis proceeds with the rest (`E-1`).
- `F-2` Orchestrator submits an invalid `status`/missing required `feedback` — the tool returns an error result; no state change; the agent can retry.
- `F-3` Auto-reinvoke attempted while a parent turn is in flight, or loop paused/stopped/blocked/at-cap — suppressed by guards (`D-3`); logged `[review-loop] reinvoke suppressed: <reason>`.
- `F-4` Two reviewers complete near-simultaneously — the cohort completeness check + single-flight reinvoke ensure exactly one note and one re-prompt (`AC-7`).
- `F-5` Cap reached on round 1 — `blocked` + ask-user, never silent finish (`E-4`, `BR-5`).
- `F-6` Server restart mid-loop — `mode`/`round`/`autoOrchestrate`/`reviewCohortId`/`openIssues` restored from the manifest; the loop continues (`AC-10`, `E-6`).
- `F-7` User Stop mid-round — `stopAgentLoop` clears `autoOrchestrate` + single-flight; in-flight children follow existing interrupt semantics; no new round (`AC-9`, `E-5`).
- `F-8` Degenerate single reviewer — works as a bounded explicit-verdict loop (`E-7`).

## 9. Security and Operational Concerns

- **auth:** loop control (start/extend/stop) is a local runner action under the current project/session authority; no elevation.
- **secrets:** no new secret surfaces; reviewer/coder definitions carry no credentials (SD-16 §9).
- **audit:** verdicts, round transitions, reinvoke fire/suppress decisions, and cap-hit are logged (`[review-loop]` prefix) and recorded on the bus; consolidated notes persist in the chat manifest.
- **rollback:** all additions are mode-gated; `autoOrchestrate=false` + `mode="keyword"` reverts to current CP-19 behavior; not registering the tool / not shipping the skill leaves existing flows untouched.
- **permissions:** child agents keep their own approval/YOLO gates (SS-08, SD-16 §9, `BR-6`); the loop never auto-approves a gated child action.

## 10. Risks and Trade-Offs

- `R-1` **Unbounded auto-reinvocation.** Mitigation: hard guards (cap, single-flight, status gate, Stop clears flag), mandatory cap, `blocked` halt, reinvoke-count logging. The earlier sub-risk (unlimited user "extend") is closed by `D-9`: extend is capped at +2 ×2, after which only Accept/Stop remain.
- `R-2` **Competing coder restarts from multiple reviewers.** Mitigation: explicit-mode gate disables the legacy keyword path (`D-2`); only `submit_review_outcome` restarts the coder.
- `R-3` **Synthesis quality** (poor merge / missed conflict). Mitigation: explicit skill protocol + focused `synthesizer` prompt; conflicts surfaced in the note; board + ask-user gate keep a human in the loop. Not a correctness blocker.
- `R-4` **Token growth** from folding N reviewer results into the parent. Mitigation: truncate each reviewer result in the note; transcripts never shared (SD-16 `D-6`).
- `R-5` **Resume drift.** Mitigation: persist loop fields in the manifest and restore on resume (mirror existing `pendingAgentContext` restore).
- `R-6` **Codex tool-name reservation.** Mitigation: alias to `flowpilot_submit_review_outcome` and normalize at the boundary if reserved (`Q-2`).

## 11. Validation Strategy

- **unit:** verdict parsing (valid/invalid status, missing feedback); `submitReviewOutcome` transitions (approved/changes_requested/blocked, cap boundary, override); consolidated note (out-of-order completion, failed reviewer → one note); `maybeAutoReinvokeOrchestrator` guards + single-flight + Stop-cancels; explicit-mode gate (no double coder restart).
- **integration:** Claude + Codex register/dispatch `submit_review_outcome` on start and resume; `review-outcome` and `extend-cap` routes round-trip; parent graph mirrors child cohort state; UI-spawned and tool-driven verdicts produce equivalent loop state.
- **manual:** the SS-15 scenarios — two reviewers with a forced conflict resolved to one list; loop iterates without re-prompting; cap-hit asks the user; Stop halts; restart resumes (`AC-2`…`AC-10`).
- **observability:** `[review-loop]` logs for round transitions, verdicts, reinvoke decisions, cap-hit; bus log + round/issue state on the board.

## 12. Traceability to Spec

- `AC-1` parallel coder + ≥2 reviewers → `D-4`, §7.2-7.4.
- `AC-2` consolidated labelled summary → `D-5`, §6 (result-sharing), §7.5.
- `AC-3` one de-duplicated, conflict-resolved list → `D-1`, §7.7.
- `AC-4` zero issues → clean end → `D-6`, §7.7 (approved).
- `AC-5` auto re-run under cap, no re-prompt → `D-3`, §7.7 (changes_requested), §7.4.
- `AC-6` cap + open issues → ask user → `D-6`, §7.8, `F-5`.
- `AC-7` no competing restarts → `D-2`, §8 `F-4`.
- `AC-8` board observability → §6 (SSE), §7.9.
- `AC-9` user stop → §7.8, `F-7`.
- `AC-10` survives restart → `D-5`/`D-7`, `F-6`.
- `AC-11` legacy flows unchanged → `D-2`/`D-7` (mode gate), §9 rollback.
- `BR-1`/`BR-2` hub-only + isolation → `D-1`, `R-4`.
- `BR-3` hub resolves conflicts → `D-1`, §7.7.
- `BR-4`/`BR-5` bounded + no silent success → `D-6`, `R-1`.
- `BR-6` human gates intact → §9 permissions.
- `BR-7` user override → §7.8, `F-7`.
- `BR-8` explainability → §6 (bus kinds), §7.9.
- `BR-9` default two-lens cohort → `D-8`, §7.3.
- `BR-10` bounded extension → `D-9`, §7.8.
