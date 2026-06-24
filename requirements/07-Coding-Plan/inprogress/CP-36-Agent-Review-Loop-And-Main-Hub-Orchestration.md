# CP-36: Agent Review Loop And Main-Hub Orchestration

## Metadata

- Document ID: `CP-36`
- Title: `Agent Review Loop And Main-Hub Orchestration (Loop-Until-Clean Multi-Reviewer)`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop (Review Until Clean)](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: [Task-089](../../08-Task/todo/Task-089-Submit-Review-Outcome-Tool.md), [Task-090](../../08-Task/todo/Task-090-Review-Loop-Driver.md), [Task-091](../../08-Task/todo/Task-091-Consolidated-Reviewer-Note.md), [Task-092](../../08-Task/todo/Task-092-Auto-Reinvoke-Orchestrator.md), [Task-093](../../08-Task/todo/Task-093-Review-Loop-Skill-And-Synthesizer-Agent.md), [Task-094](../../08-Task/todo/Task-094-Review-Loop-Board-And-Contract.md), [Task-095](../../08-Task/todo/Task-095-Gate-Legacy-Keyword-Loop.md)
- Related Documents: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) + [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (this loop is the first FlowDefinition template of that generic engine), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [CP-19: Multiple Agents](../inprogress/CP-19-Multiple-Agents.md), [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-084: Dependency Feedback Loop And Orchestration Board](../../08-Task/todo/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- Replaces: `None`
- Tags: `multi-agent, orchestration, review-loop, main-hub, synthesis, consensus, local-runner, desktop`

## AI Quick View

### Summary

- Add a **main-chat-as-hub** review loop: the main (parent) agent spawns one `coder` plus **N reviewers**, receives every reviewer's result, **synthesizes and resolves conflicts itself**, and decides whether to iterate — sub-agents never talk to each other directly (preserves SD-16 `D-6` isolation).
- Introduce a deterministic, **bounded loop** that repeats `coder → reviewers → synthesis` until the synthesizing agent reports **zero open issues** (`approved`) or a **round cap** is hit; on cap-with-open-issues the loop **blocks and asks the user** instead of silently stopping.
- Add a new provider tool **`submit_review_outcome`** (Claude MCP + Codex DynamicTool) so the orchestrating agent signals the verdict explicitly and observably, replacing the fragile substring keyword detection currently used for the single-reviewer loop.
- Add a runner **auto-reinvocation** primitive: when all reviewers a parent is waiting on complete, the runner folds their results into one **consolidated, per-reviewer-labelled note** and re-prompts the orchestrating agent to synthesize — advancing the loop without a human typing (the concrete build of SD-16 §14.5 "foundation for auto agent-to-agent mode").
- Ship a **`agent-review-loop` skill** (the protocol the main agent follows) and a **`synthesizer` built-in agent**; extend the Orchestration Board to render N reviewers, per-round verdicts, the open-issue count, and an **extend-cap** control.

### Current Ask

- Deliver a plan detailed enough that any implementer (human or AI) can build the loop-until-clean multi-reviewer review feature end-to-end without further design questions: exact files, symbols, tool schemas, routes, execution order, and per-slice Definition of Done.

### Key Decisions

- `P-1` **Main agent is the only router.** Reviewers post results to the parent via the existing `pendingAgentContext` channel; the parent agent (guided by a skill) does dedup, conflict resolution, and the verdict. No peer-to-peer agent messaging is added.
- `P-2` **Explicit verdict tool over keyword parsing.** A new `submit_review_outcome` tool carries a structured `{status, issues[], feedback, roundCapOverride?}`. The legacy `strings.Contains(msg, "changes requested")` path (`interactive_service.go:880-935`) is gated OFF when a run is in explicit-loop mode so two reviewers cannot each independently restart the coder.
- `P-3` **Bounded + ask-on-cap.** The loop has a default round cap of 3 (overridable per run). Reaching the cap with open issues moves the loop to a new `blocked` status; the skill instructs the orchestrator to call `ask_user` (extend cap / accept as-is / stop). The loop never terminates silently with unresolved issues.
- `P-4` **Runner-assisted auto-reinvocation, opt-in per run.** A per-parent `autoOrchestrate` flag (set when the review loop starts) lets the runner re-prompt the orchestrator after a reviewer cohort completes. It is single-flight, bounded by the cap, and cancelled by Stop. Runs without the flag behave exactly as today.
- `P-5` **Reviewers run in parallel via `dependsOn`.** Each reviewer is spawned `wait=false` with `dependsOn:[coderRunId]`; the existing barrier (`dependenciesSatisfiedLocked`, `releaseDependentAgents`) starts them when the coder completes. The "debate" is the parent's synthesis turn, not cross-agent chatter.
- `P-6` **Additive only.** New fields on `AgentLoopState`/`SpawnAgentInput`, a new tool, a new skill, a new built-in agent, and new routes. No change to `ProviderRuntimeAdapter`, the SSE transport contract (additive event reuse only), or single-agent run behavior.

### Constraints

- Run GitNexus impact analysis before editing any Go/TS symbol named below and report blast radius; warn on HIGH/CRITICAL (CLAUDE.md mandate). Run `gitnexus_detect_changes()` before commit.
- Do not change the `ProviderRuntimeAdapter` interface or the `ProviderEvent`/`ProviderEventDTO` union except by additive fields/variants.
- Do not regress normal chat, the existing single coder↔reviewer loop, session resume (SD-14), cross-PC sync, or Drive sync.
- No Supabase migration in this CP (Phase-1 chat mode, mirroring CP-19 P-6). Durable persistence of outcomes is deferred to CP-19 Task-085.
- The auto-reinvocation loop MUST be bounded and stoppable; an unbounded re-prompt loop is a release blocker.
- Reuse the existing `artifact`/`pendingAgentContext`/bus channels; do not invent a new transport.

### Open Questions

- `Q-1` **Resolved** — synthesis is performed by the **main agent** by default (skill-guided); the `synthesizer` built-in (`P-5`) is an optional offload. (SS-15 `Q-1`, SD-18 `D-1`.)
- `Q-2` **Resolved** — this feature has its own document chain **SS-15 → SD-18 → CP-36** (a dedicated SD, not an SD-16 extension).
- `Q-3` **Resolved** — default lenses are `correctness` + `security`, with `regression` added by the skill when the change touches tested behavior; the lens set is skill config (tunable without code). (SS-15 `BR-9`, SD-18 `D-8`.)
- `Q-4` **Resolved** — extend-cap is bounded to +2 rounds, at most twice (ceiling = initial cap + 4); beyond that only Accept/Stop remain. (SS-15 `BR-10`, SD-18 `D-9`.)

### Source Refs

- `SS-15` `AC-1`…`AC-11`, `BR-1`…`BR-10` (the business behavior this plan implements).
- `SD-18` `D-1`…`D-9`, §5 (data model), §6 (interfaces), §7 (execution flow), §12 (AC traceability).
- `SS-16` / `SD-19` — the generic Agent Flow Engine; this loop is its first FlowDefinition template, and `submit_review_outcome` is a declared instance of the generic `flow_control` (SD-19 `D-4`).
- `SD-16` `D-1`…`D-15`, §7 (execution flow), §14 (result sharing), §14.5 (auto mode foundation), §15 (runtime behavior).
- `CP-19` `P-2`, `P-4`, `P-9`, `P-11`, `P-15`; Task-082 (`AgentOrchestrator`, `SpawnAgentInput`), Task-084 (graph/bus/board).
- Code anchors (verified): `agent_orchestrator.go` (`transition`, `advanceRound`, `addBus`, `queueFeedback`, `AgentLoopState`); `interactive_service.go:394-571` (`dependenciesSatisfiedLocked`, `releaseDependentAgents`, `resumePendingLoopWork`, `scheduleChildTurn`, `takeQueuedFeedbackPrompt`), `:880-998` (turn-completed coder/reviewer logic, `releaseDependentAgents` call), `:1174` (`turnBridge.SpawnAgent`), `appendPendingAgentContextLocked`; `provider_registry.go:25-32` (`TurnBridge`); `codex_adapter.go:89-107,299-315` (spawn tool reg + dispatch); `claude_mcp_server.go:235-296` (tool defs + dispatch), `claude_permission_mcp.go:182-195` (`handleClaudeSpawnAgent`); `interactive_handlers.go:41-48` (agent routes); desktop `OrchestrationBoard.tsx`, `state/store.ts`, `types/contract.ts:91-136,461-466`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`.

## 1. Goal

Let a FlowPilot user say *"review this until there are no issues left"* and have the main chat agent drive a bounded, observable, multi-reviewer loop:

1. Spawn a `coder` to produce a change.
2. Spawn **N reviewers in parallel**, each with a distinct lens.
3. Collect all reviewer results into one consolidated note.
4. Have the **main agent synthesize**: dedup issues, resolve reviewer conflicts using full task context, produce one actionable list.
5. If issues remain → re-run the coder with the consolidated feedback and loop; if none → finish; if the round cap is hit with issues open → ask the user how to proceed.

The implementation reuses the existing spawn / dependency-barrier / result-sharing / bus / board stack (CP-19 Phase 1) and adds only: one verdict tool, a bounded loop driver, an opt-in auto-reinvocation primitive, a skill, a built-in agent, and additive board UI.

## 2. Input Documents

- [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) — the design this plan implements (`D-1`…`D-7`, data model, interfaces, execution flow).
- [SS-15: Agent Review Loop (Review Until Clean)](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) — business behavior and acceptance criteria (`AC-1`…`AC-11`, `BR-1`…`BR-8`).
- [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) — the spawn/tool/result-sharing substrate (§7, §14, §14.5, §15).
- [CP-19: Multiple Agents](../inprogress/CP-19-Multiple-Agents.md) — Phase-1 orchestrator, spawn tool, board, result sharing.

## 3. Implementation Strategy

- **Overall approach:** Layer a deterministic loop driver and an explicit verdict tool on top of the existing orchestrator, and let the main agent supply the intelligence (synthesis/conflict resolution) under a skill. Keep sub-agent isolation; the parent is the hub.
- **Sequencing logic:** Build bottom-up so each slice is independently testable.
  1. `P-1` verdict tool contract + parsing (pure, no behavior change).
  2. `P-2` loop driver in the orchestrator/service consuming the verdict (synchronous `wait=true` path first — works without auto-reinvoke).
  3. `P-3` consolidated multi-reviewer note format.
  4. `P-4` opt-in auto-reinvocation (the async/parallel experience).
  5. `P-5` skill + `synthesizer` built-in (the agent-facing protocol).
  6. `P-6` board + contract + client surface.
  7. `P-7` gate the legacy keyword loop off in explicit mode.
- **Dependencies:** `P-2` depends on `P-1`. `P-4` depends on `P-2`+`P-3`. `P-6` depends on `P-1`/`P-2` DTOs. `P-7` depends on `P-2`. `P-5` is independent but only useful once `P-1`/`P-2` land.

Task-089 -> 095

## 4. Work Breakdown

> Each `P-x` is one task-sized slice with its own Definition of Done. Cut as Task-088 … Task-094.

### `P-1` — `submit_review_outcome` tool contract (Task-089)

- **Backend types** (`agent_orchestrator.go`, new section near `SpawnAgentInput`):
  ```go
  // ReviewIssue is one actionable finding produced during synthesis.
  type ReviewIssue struct {
      ID         string `json:"id,omitempty"`
      Title      string `json:"title"`
      Severity   string `json:"severity,omitempty"`   // blocker|major|minor|nit
      File       string `json:"file,omitempty"`
      Resolution string `json:"resolution,omitempty"` // how the coder should fix it
  }

  // ReviewOutcomeInput is the structured verdict the orchestrating agent submits
  // after synthesizing all reviewer results for the current round.
  type ReviewOutcomeInput struct {
      Status           string        `json:"status"`           // approved|changes_requested|blocked
      Issues           []ReviewIssue `json:"issues,omitempty"` // empty when approved
      Feedback         string        `json:"feedback,omitempty"`
      RoundCapOverride int           `json:"roundCapOverride,omitempty"`
  }

  type ReviewOutcomeResult struct {
      Status       string `json:"status"`       // accepted verdict echo
      Round        int    `json:"round"`
      RoundCap     int    `json:"roundCap"`
      OpenIssues   int    `json:"openIssues"`
      NextAction   string `json:"nextAction"`   // looping|done|awaiting_user
  }
  ```
  Add `parseReviewOutcomeInput(args map[string]any) (ReviewOutcomeInput, error)` mirroring `parseSpawnAgentInput` (`agent_orchestrator.go:344`). Validate `status ∈ {approved, changes_requested, blocked}`; require `feedback` non-empty when `status==changes_requested`.
- **Bridge** (`provider_registry.go:25-32`): add to `TurnBridge`:
  ```go
  // SubmitReviewOutcome records the orchestrating agent's verdict for the current
  // review round and returns the resulting loop state (does NOT block).
  SubmitReviewOutcome(in ReviewOutcomeInput) (ReviewOutcomeResult, error)
  ```
  Implement `(*turnBridge).SubmitReviewOutcome` near `interactive_service.go:1174` (`SpawnAgent`) → calls `s.submitReviewOutcome(b.runID, in)` (added in `P-2`). Update every fake bridge that implements `TurnBridge` (test files listed in §5).
- **Claude registration** (`claude_mcp_server.go`):
  - Add tool def to `claudeMCPToolDefs()` (after `spawn_agent`, ~`:296`) with `inputSchema` advertising `status` (enum), `issues` (array of objects), `feedback`, `roundCapOverride`; `required:["status"]`.
  - Add dispatch `case "submit_review_outcome": return handleClaudeSubmitReviewOutcome(args, bridge), nil` in the `tools/call` switch (`:250-259`).
  - Add `handleClaudeSubmitReviewOutcome` in `claude_permission_mcp.go` next to `handleClaudeSpawnAgent:184` (parse → `bridge.SubmitReviewOutcome` → `claudeMcpTextResult(json)`). Add `claudeReviewOutcomeToolName` const next to `:38`.
- **Codex registration** (`codex_adapter.go`):
  - Add `codexReviewOutcomeDynamicTool()` mirroring `codexSpawnAgentDynamicTool:91`; append to `dynamicTools` slice at `:133`.
  - Add dispatch `case "submit_review_outcome", codexReviewOutcomeToolName:` in `handleDynamicToolCall` switch (`:289-318`), parse via `parseReviewOutcomeInput`, call `bridge.SubmitReviewOutcome`, reply `codexDynamicToolResult(json, true)`.
  - Reserve-name parity: if `submit_review_outcome` is NOT reserved in the Codex app-server, register it under its public name (no alias). Confirm against the same reserved-name check that forced `flowpilot_spawn_agent` (`D-13`); if reserved, add `codexReviewOutcomeToolName = "flowpilot_submit_review_outcome"` and normalize at the boundary.
- **DoD:** `parseReviewOutcomeInput` unit tests (valid/invalid status, missing feedback on changes_requested, issues array parse); `claude_permission_mcp_test.go`-style assertion that `tools/list` advertises `submit_review_outcome` with the full schema; Codex dynamic-tool registration test asserts the tool is present on start AND resume.

### `P-2` — Loop driver consuming the verdict (Task-090)

- **`AgentLoopState`** (`provider_event.go` Go + `contract.ts:123`): add additive fields:
  ```go
  type AgentLoopState struct {
      Status        string `json:"status"`        // idle|running|waiting_review|synthesizing|blocked|approved|rejected|stopped
      Round         int    `json:"round"`
      RoundCap      int    `json:"roundCap"`
      GateReason    string `json:"gateReason,omitempty"`
      OpenIssues    int    `json:"openIssues,omitempty"`    // NEW
      Mode          string `json:"mode,omitempty"`          // NEW: "keyword" (legacy) | "explicit" (this CP)
      CoderRunID    string `json:"coderRunId,omitempty"`    // NEW: the run re-started on changes_requested
  }
  ```
- **Service method** `interactive_service.go`, new `submitReviewOutcome(parentRunID string, in ReviewOutcomeInput) (ReviewOutcomeResult, error)`:
  1. Lock; load loop state via `agentOrchestrator.ensureLoopLocked`. Set `Mode="explicit"`.
  2. Record a bus message `kind="review-outcome"` (`recordAgentBus`) carrying `status` + issue count.
  3. Apply override: if `in.RoundCapOverride > 0`, set `RoundCap`.
  4. Switch on `in.Status`:
     - `approved`: `transition(parentRunID,"approved")`, `OpenIssues=0`; append a parent `pendingAgentContext` handoff note ("Review loop approved after N rounds"); set `NextAction="done"`. Clear `autoOrchestrate`.
     - `changes_requested`: set `OpenIssues=len(in.Issues)`; call `advanceRound`; if new round `>= cap` → set `Status="blocked"`, `GateReason="round cap reached with N open issues"`, `NextAction="awaiting_user"` (do NOT restart coder); else re-start the coder run (`CoderRunID`) with the consolidated feedback prompt via the existing restart path (`scheduleChildTurn` / `pendingRestartRunID`+`resumePendingLoopWork` semantics at `:899-927`), `NextAction="looping"`.
     - `blocked`: set `Status="blocked"`, `GateReason=in.Feedback`, `NextAction="awaiting_user"`.
  5. `emitAgentGraph(parentRunID, snapshot)`; return `ReviewOutcomeResult`.
- **Coder re-start prompt:** compose feedback from `in.Issues` (numbered list of `Title` + `Resolution`, grouped by `File`) plus `in.Feedback`. Reuse `takeQueuedFeedbackPrompt` so any queued `user-feedback` merges in.
- **HTTP mirror** (`interactive_handlers.go:41-48` block): add `mux.HandleFunc("POST /client/workflow-runs/{runId}/review-outcome", s.handleSubmitReviewOutcome)` and `POST .../agent-loop/extend-cap` → `s.handleExtendRoundCap` (body `{roundCap:int}` → `agentOrchestrator.mutateLoop` set cap, resume if was `blocked`). Handlers mirror `handleInjectAgentFeedback:860`.
- **Bounded extend (SD-18 `D-9`):** add `extendCount int` to `AgentLoopState`. `handleExtendRoundCap`/`submitReviewOutcome` reject an extend when `extendCount >= 2`, and otherwise raise the cap by exactly `+2` and increment `extendCount` (hard ceiling = initial cap + 4). Once at the ceiling the orchestrator must offer only Accept/Stop. This guarantees termination under repeated user extends.
- **DoD:** pure-logic orchestrator tests for: approved → done; changes_requested under cap → round++ and coder restart scheduled; changes_requested at cap → `blocked` + no restart; blocked → awaiting_user; extend-cap from `blocked` resumes looping. HTTP contract test for both new routes.

### `P-3` — Consolidated multi-reviewer note (Task-091)

- **Problem:** today `appendPendingAgentContextLocked` appends one line per child (`interactive_service.go:870-879, 944-947`). With N reviewers the parent sees N ungrouped lines and no conflict signal.
- **Change:** add `reviewCohortId` to `SpawnAgentInput` (`json:"reviewCohortId,omitempty"`) and to the child `interactiveRun` (additive field near `:103-110`). When a child with a `reviewCohortId` reaches `EventTurnCompleted`, instead of appending an isolated line, accumulate into a per-cohort buffer `cohortResults map[string][]cohortEntry` (orchestrator). When **all** children of the cohort are terminal, build ONE consolidated note:
  ```
  [FlowPilot review round R — N reviewers reported]
  Reviewer "security" (codex): <finalMessage truncated 1500>
  Reviewer "correctness" (claude): <finalMessage truncated 1500>
  Reviewer "regression" (claude): <finalMessage truncated 1500>
  ---
  Synthesize: dedup the findings, flag any conflicting verdicts, resolve them using the
  task context, then call submit_review_outcome with the consolidated issue list.
  ```
  Append via `appendPendingAgentContextLocked` (single entry) so it folds into the parent's next turn exactly like existing notes (SD-16 §14.3) and survives restart (persisted in `sessions.ndjson`).
- **Cohort completeness check:** reuse `dependenciesSatisfiedLocked`-style logic — cohort complete when every child whose `reviewCohortId==X` has `status ∈ {completed, failed, cancelled}`.
- **DoD:** unit test: 3 children in one cohort completing out of order produce exactly one consolidated note containing all three, only after the last completes; a failed reviewer is included as `failed: <error>`.

### `P-4` — Opt-in auto-reinvocation of the orchestrator (Task-092)

- **Flag:** add `autoOrchestrate bool` to the parent `interactiveRun` (additive). Set true when the review loop starts (the skill's first spawn carries `reviewCohortId` AND a new `SpawnAgentInput.AutoOrchestrate bool`, OR the HTTP `review-outcome`/start path sets it). Persist in `sessions.ndjson` so resume keeps the loop alive.
- **Primitive:** new `maybeAutoReinvokeOrchestrator(parentRunID string)` called at the end of the cohort-complete branch in `P-3` (and from `releaseDependentAgents`). Guards (ALL required):
  1. `autoOrchestrate == true` for the parent.
  2. Loop `Status ∉ {paused, stopped, blocked, approved, rejected}` (`loopAllowsNextTurnLocked` extended).
  3. Parent has no turn in flight (`!parent.turnInFlight`).
  4. Single-flight: a per-parent `reinvokeInFlight` bool prevents overlap.
  5. Round `< cap`.
- **Action:** schedule a parent turn (not a child) via `scheduleChildTurn(parentRunID, parentStepID, synthesisPrompt)` where `synthesisPrompt` is the consolidated note from `P-3` plus the synthesis directive. This is the **non-user-visible** re-prompt — it must use the same "system note" prepend path as `pendingAgentContext` (SD-16 §14.3), never a user bubble.
- **Termination guarantees:** the re-prompt only fires once per cohort completion; the orchestrator's `submit_review_outcome` either advances the round (new cohort, new completion → next reinvoke) or ends the loop. Cap + `blocked` status are the hard stops. `stopAgentLoop` (`:356`) clears `autoOrchestrate` and `reinvokeInFlight`.
- **DoD:** unit tests: reinvoke fires exactly once when a cohort completes with auto on; does NOT fire when paused/stopped/blocked/at-cap/turn-in-flight; Stop cancels a pending reinvoke; a run without the flag never auto-reinvokes (regression guard for normal chat).

### `P-5` — `agent-review-loop` skill + `synthesizer` built-in (Task-093)

- **Built-in agent** (`agent_catalog.go` `builtinAgentDefinitions():391`): add `synthesizer`:
  ```go
  {
      Name:        "synthesizer",
      Role:        "synthesizer",
      Description: "Consolidates multiple reviewer results, resolves conflicts, and emits a single verdict.",
      Tools:       []string{"Read", "Grep", "Glob"},
      SystemPrompt: "You are the synthesizer. You receive findings from several reviewers. " +
          "Deduplicate issues, identify any conflicting verdicts, resolve each conflict using the " +
          "original task context and the codebase, then call submit_review_outcome with status " +
          "approved (no issues) or changes_requested (a single consolidated, de-conflicted issue list). " +
          "Never restart the coder yourself.",
      Source: "flowpilot",
  },
  ```
- **Skill** — new `.claude/skills/agent-review-loop/SKILL.md` (and a `.codex` mirror if the project keeps separate trees) defining the main-agent protocol:
  - When the user asks to "review until clean", spawn `coder` (`wait=true`) to produce the change; capture its `runId`.
  - Spawn the reviewer cohort (SD-18 `D-8`): **two `reviewer` agents by default** — lenses `correctness` and `security` — `wait=false`, `dependsOn:[coderRunId]`, same `reviewCohortId`, `autoOrchestrate:true`. Add a third `regression` reviewer when the change touches previously-tested behavior. Lenses are skill config (tunable without code); minimum two.
  - End the turn. When the consolidated note arrives (auto-reinvoke), synthesize and call `submit_review_outcome`.
  - On `changes_requested` the runner restarts the coder; a new cohort runs; repeat.
  - On `blocked` (cap reached, issues open) call `ask_user` with options: *Extend cap by 2 / Accept remaining issues / Stop loop*; act on the answer (extend-cap route, approved, or stopAgentLoop).
  - Stop conditions and the round cap are stated explicitly so the agent never loops unbounded.
- **DoD:** catalog test asserts `synthesizer` is discoverable and overridable by an on-disk file of the same name; skill file lints against the project skill format; a manual scenario in §7.

### `P-6` — Board, contract, and client surface (Task-094)

- **`contract.ts`:** add `ReviewIssue`, `ReviewOutcomeInput`, `ReviewOutcomeResult`; extend `AgentLoopState` (`:123`) with `openIssues?`, `mode?`, `coderRunId?`; add client methods (`:461-466`): `submitReviewOutcome?(parentRunId, in): Promise<AgentGraphSnapshot>`, `extendRoundCap?(parentRunId, roundCap): Promise<AgentGraphSnapshot>`.
- **`HttpWsRunnerClient.ts` + `MockRunnerClient.ts`:** implement the two methods against the new routes; mock returns a deterministic snapshot for tests.
- **`state/store.ts`:** add `submitReviewOutcome`, `extendRoundCap` actions; surface `openIssues`/`mode` from the `agent_graph_updated` SSE snapshot already handled in `startOrchestrationStream`.
- **`OrchestrationBoard.tsx`:** generalize beyond the hardcoded single coder + single reviewer (`:42-48`):
  - Render the coder node plus a **row of reviewer nodes** (one per cohort member) with per-node status.
  - Show **round R / cap**, **open-issue count**, and loop `status` (`synthesizing`, `blocked`, `approved`).
  - When `status==blocked`: render an **Extend cap** control (calls `extendRoundCap`) and surface the `gateReason`.
  - Keep existing pause/resume/stop/inject controls.
- **DoD:** frontend tests: board renders N reviewers from a snapshot; shows open-issue count and round; `blocked` renders the extend-cap control; `submitReviewOutcome`/`extendRoundCap` call the client and update the snapshot; single-agent/no-cohort fallback still renders.

### `P-7` — Gate the legacy keyword loop in explicit mode (Task-095)

- In the `EventTurnCompleted` reviewer branch (`interactive_service.go:884-935`), wrap the keyword logic in `if loopState.Mode != "explicit"`. When `Mode=="explicit"`, a reviewer completing only contributes to the cohort note (`P-3`) and never directly transitions the loop or restarts the coder — the orchestrator's `submit_review_outcome` is the sole driver. This prevents N reviewers from racing N coder restarts.
- **DoD:** unit test: in explicit mode, two reviewers completing produce zero coder restarts (only the cohort note + reinvoke); in keyword mode (legacy), behavior is byte-for-byte unchanged.

## 5. Touched Areas

- **files (backend):**
  - `apps/local-runner/internal/runner/agent_orchestrator.go` (`ReviewIssue`/`ReviewOutcomeInput`/`ReviewOutcomeResult`, `parseReviewOutcomeInput`, cohort buffers, loop-state fields)
  - `apps/local-runner/internal/runner/interactive_service.go` (`submitReviewOutcome`, consolidated note, `maybeAutoReinvokeOrchestrator`, `autoOrchestrate`, explicit-mode gate, `turnBridge.SubmitReviewOutcome`)
  - `apps/local-runner/internal/runner/provider_registry.go` (`TurnBridge.SubmitReviewOutcome`; `interactiveRun` fields `reviewCohortId`, `autoOrchestrate`)
  - `apps/local-runner/internal/runner/provider_event.go` (`AgentLoopState` new fields)
  - `apps/local-runner/internal/runner/codex_adapter.go` (`codexReviewOutcomeDynamicTool`, dispatch case, const)
  - `apps/local-runner/internal/runner/claude_mcp_server.go` (tool def + dispatch case), `claude_permission_mcp.go` (`handleClaudeSubmitReviewOutcome`, const)
  - `apps/local-runner/internal/runner/interactive_handlers.go` (`POST .../review-outcome`, `POST .../agent-loop/extend-cap`)
  - `apps/local-runner/internal/runner/agent_catalog.go` (`synthesizer` built-in)
  - Tests: `agent_orchestrator_test.go`, `interactive_service_test.go`, `claude_permission_mcp_test.go`, `claude_mcp_server_test.go`, `codex_appserver_test.go`, plus every fake bridge implementing `TurnBridge` (`codex_appserver_test.go:240`, `codex_resume_process_test.go:20`, `claude_adapter_test.go:51`, `fake_provider_adapter.go`).
- **files (frontend):** `apps/desktop-flowpilot/src/types/contract.ts`, `state/store.ts`, `components/OrchestrationBoard.tsx`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `state/store.test.ts`, `styles.css`.
- **skill/config:** `.claude/skills/agent-review-loop/SKILL.md` (+ optional `.codex` mirror).
- **modules:** local-runner orchestration + provider adapters; desktop board + state.
- **database:** none (Phase 1).
- **external systems:** none new (Drive sync carries the additive `pending_agent_context`/loop fields already).

## 6. Data or Migration Steps

- **schema:** none. All new state is in-memory orchestrator maps + additive `interactiveRun`/`AgentLoopState` fields persisted to the existing `sessions.ndjson` manifest (extend the session-snapshot serializer near `interactive_service.go:744-762` to include `reviewCohortId`, `autoOrchestrate`, and the loop `mode`/`openIssues`).
- **data backfill:** none — absent fields default to legacy behavior (`mode=""→keyword`, `autoOrchestrate=false`).
- **config updates:** ship the `synthesizer` built-in and the `agent-review-loop` skill in-repo so the feature works on first run.

## 7. Validation Plan

- **tests to add:**
  - Go unit: `parseReviewOutcomeInput`; `submitReviewOutcome` transitions (approved/changes_requested/blocked, cap boundary, override); consolidated cohort note (out-of-order, failed reviewer); `maybeAutoReinvokeOrchestrator` guards + single-flight + Stop-cancels; explicit-mode gate (no double coder restart).
  - Go contract: `tools/list` advertises `submit_review_outcome` (Claude + Codex, start + resume); `POST .../review-outcome` and `POST .../agent-loop/extend-cap` round-trips; additive `AgentLoopState` JSON serialization.
  - Frontend: board renders N reviewers, round/cap, open-issue count, `blocked` + extend-cap control; store `submitReviewOutcome`/`extendRoundCap`; SSE snapshot updates; single-agent fallback.
- **manual checks (desktop):**
  1. YOLO on. Prompt the main agent: *"Use the agent-review-loop skill: have a coder add input validation to X, then 2 reviewers (correctness + security) review until no issues, max 3 rounds."*
  2. Verify: coder runs → 2 reviewers run in parallel (Agents panel) → board shows round 1, both reviewer nodes → consolidated note triggers a synthesis turn (no user typing) → `submit_review_outcome` posts a verdict → on changes_requested the coder restarts with the merged feedback → loop continues.
  3. Force a conflict (one reviewer says "add handling", one says "unreachable") and confirm the synthesis turn resolves it into ONE issue list, not two contradictory restarts.
  4. Drive to the cap with open issues → board shows `blocked` + Extend cap; the agent calls `ask_user`; choosing "Extend cap by 2" resumes the loop.
  5. Press Stop mid-loop → no further reinvocation; main run idle.
  6. Restart the server mid-loop → resume keeps `autoOrchestrate`/round and continues.
- **failure cases:** a reviewer fails mid-turn (note records `failed`, synthesis proceeds with remaining); orchestrator submits an invalid status (tool returns error, no state change); auto-reinvoke attempted while a turn is in flight (suppressed); cap=0 / override to 0 (treated as default 3); a non-loop normal chat never auto-reinvokes.

## 8. Rollout and Fallback

- **rollout order:** `P-1` → `P-2` → `P-3` → `P-7` (gate) → `P-4` (auto) → `P-5` (skill/agent) → `P-6` (UI). Land `P-1`…`P-3`+`P-7` first: this gives a working **synchronous** loop where the main agent drives rounds with `wait=true` spawns and explicit verdicts (no auto-reinvoke needed). `P-4` then upgrades it to parallel + auto.
- **fallback path:** all additions are additive and mode-gated. Setting `autoOrchestrate=false` and `mode="keyword"` reverts to current CP-19 behavior. Not registering `submit_review_outcome` / not shipping the skill leaves single-agent and single-reviewer flows untouched. Hiding the extended board controls reverts the UI.
- **monitoring:** reuse the `[agent-spawn]` log prefix; add `[review-loop]` lines for round transitions, verdict submissions, reinvoke fire/suppress decisions, and cap-hit. Surface the bus log + round/issue state on the board.

## 9. Risks

- `R-1` **Unbounded auto-reinvocation** (the main agent re-prompted forever). Mitigation: `P-4` hard guards (cap, single-flight, status gate, Stop clears flag) + a `[review-loop]` reinvoke-count log; cap is mandatory and `blocked` halts.
- `R-2` **Two reviewers racing two coder restarts.** Mitigation: `P-7` gates the legacy keyword path off in explicit mode; only `submit_review_outcome` restarts the coder.
- `R-3` **Synthesis quality** (main agent merges poorly / misses a conflict). Mitigation: explicit skill protocol + `synthesizer` built-in with a focused prompt; conflicts surfaced in the consolidated note format; not a correctness blocker (human can inspect the board + ask_user gate).
- `R-4` **Token growth** from N reviewer results folded into the parent. Mitigation: truncate each reviewer result (1500 chars) in the consolidated note; isolation means transcripts are never shared (SD-16 `D-6`).
- `R-5` **Resume drift** — loop state lost on restart. Mitigation: persist `mode`/`round`/`autoOrchestrate`/`reviewCohortId` in `sessions.ndjson` and restore in `interactive_resume.go` (mirror the existing `pendingAgentContext` restore).
- `R-6` **Provider tool-name reservation** for `submit_review_outcome` on Codex (as happened with `spawn_agent`, `D-13`). Mitigation: `P-1` checks the reserved list and aliases to `flowpilot_submit_review_outcome` if needed, normalizing at the boundary.

## 10. Definition of Done

- A user can ask the main chat agent to "review until no issues" and observe a bounded coder → N-reviewer → synthesis loop that terminates on `approved` or asks the user at the round cap — never silently with open issues.
- `submit_review_outcome` is registered and callable on both Claude and Codex (start and resume), and is the sole driver of the loop in explicit mode.
- Multiple reviewers run in parallel via `dependsOn`; their results arrive as one consolidated, per-reviewer-labelled note; the main agent synthesizes and resolves conflicts into a single issue list.
- The runner auto-reinvokes the orchestrating agent after a reviewer cohort completes (opt-in, single-flight, bounded by cap, cancelled by Stop) and never auto-reinvokes a normal chat run.
- The Orchestration Board renders N reviewers, the current round/cap, the open-issue count, and a `blocked` + Extend-cap control; pause/resume/stop/inject still work.
- The legacy single-reviewer keyword loop is unchanged when `mode != "explicit"`; normal chat, workflows, session resume, and Drive sync show no regressions (tests green).
- No Supabase migration introduced; loop state survives a runner restart via the existing chat manifest.
- All §7 unit, contract, and frontend tests pass; GitNexus impact analysis was run for each edited symbol and `gitnexus_detect_changes()` was clean before commit.
