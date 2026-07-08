# Task-091: Review-Loop Template (Outcome Tool, Config, Legacy Gate)

## Metadata

- Document ID: `Task-091`
- Title: `Review-Loop Template: submit_review_outcome Declared Face, Config, Legacy Gate`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-089: Generic Flow Vocabulary And flow_control Handler](./Task-089-Generic-Flow-Vocabulary-And-Flow-Control-Handler.md), [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, flow-control, provider-tool, claude, codex, go`

## AI Quick View

### Summary

- Make the review-until-clean loop the engine's **first template**: register `submit_review_outcome` as the **declared face** of the generic `flow_control`, express the loop as node/edge/policy **config**, and gate the legacy keyword path off in explicit mode.
- `submit_review_outcome{status, issues[], feedback, roundCapOverride?}` maps onto `flow_control` (`approved→done`, `changes_requested→continue`, `blocked→escalate`) via Task-089's face registry; `issues` ride in `Payload`. It is the **only** flow tool the model sees (CP-36 **G3**).
- The loop = `coder`(`reinvoke`) + `reviewer×N`(`dependsOn:coder`) + synthesis(`run:inline`,`join:all`), back-edge `synthesis→coder when continue` (`cap:3`). The legacy `strings.Contains` path is wrapped in `if Mode != "explicit"`.

### Current Ask

- Register the review outcome tool on both providers (start + resume), wire it to `flow_control`, express the review loop as config over the Task-090 executor, and gate the legacy keyword loop — with cross-provider, mapping, and gate tests.

### Key Decisions

- `T-1` `submit_review_outcome` mirrors the `spawn_agent` registration pattern on both providers; on Codex, alias to `flowpilot_submit_review_outcome` only if the public name is reserved (SD-16 `D-13`).
- `T-2` Validation: `status ∈ {approved, changes_requested, blocked}`; `feedback` required when `status==changes_requested`; the handler maps to `flow_control` and calls `bridge.SubmitFlowControl` — **no second loop driver**.
- `T-3` In explicit mode a completing reviewer only contributes to the cohort note; `flow_control` is the sole driver (prevents N reviewers racing N coder restarts).

### Constraints

- Run GitNexus impact analysis before editing `claude_mcp_server.go`, `claude_permission_mcp.go`, `codex_adapter.go`, and the `interactive_service.go` keyword branch; warn on HIGH/CRITICAL.
- Additive; do not alter `spawn_agent`/`ask_user` behavior. Keyword-mode behavior must stay byte-for-byte unchanged.
- Every fake bridge already gains `SubmitFlowControl` in Task-090; this task adds no new bridge method.

### Open Questions

- Codex reserved-name status for `submit_review_outcome` — verify against the live app-server (SD-18 `Q-2`).

### Source Refs

- CP-36 `P-4`/`P-5`(review-config)/`P-10`; SD-18 `D-2`…`D-7`, §5/§6; SS-15 `AC-3`…`AC-9`.
- Anchors: `agent_orchestrator.go` (`reviewOutcomeFace`, `parseSpawnAgentInput:344`); `claude_mcp_server.go:235-296,250-259`; `claude_permission_mcp.go:182-195,38`; `codex_adapter.go:89-107,133,289-318`; `interactive_service.go:884-935` (keyword branch), `takeQueuedFeedbackPrompt`.

## 1. Goal

The review-until-clean loop runs entirely on the generic engine: a declared `submit_review_outcome` tool drives `flow_control`, the topology + cap live as config, and the legacy keyword path is inert in explicit mode — with no review-specific transition code in the service.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-4`, `P-5`, `P-10`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-2`, §6
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-3`
- specific upstream ids: CP-36 `P-4`/`P-5`/`P-10`; SD-18 `D-2`

## 3. Trigger

The engine (Task-089/090) is domain-free; the review loop needs a thin, declared face + config to run on it, and the brittle keyword detection must be disabled when the explicit verdict tool is in use.

## 4. Exact Change

- `T-1` Types (`agent_orchestrator.go`, review-template section): `ReviewIssue{ID,Title,Severity,File,Resolution}`, `ReviewOutcomeInput{Status,Issues,Feedback,RoundCapOverride}`, `ReviewOutcomeResult` (echo of `FlowControlResult` + open count). `parseReviewOutcomeInput(map[string]any)` validates per `T-2`, and **maps onto `FlowControlInput`** using `reviewOutcomeFace()` (Task-089), placing `Issues` in `Payload`.
- `T-2` Claude: add the tool def to `claudeMCPToolDefs()` (full `inputSchema`: status enum, issues array, feedback, roundCapOverride; `required:["status"]`); add the `tools/call` dispatch case → `handleClaudeSubmitReviewOutcome` (`claude_permission_mcp.go`, next to `handleClaudeSpawnAgent:184`) → map → `bridge.SubmitFlowControl`. Add `claudeReviewOutcomeToolName` const next to `:38`.
- `T-3` Codex: add `codexReviewOutcomeDynamicTool()`; append to `dynamicTools` (`:133`); add the `handleDynamicToolCall` case (`:289-318`) → `parseReviewOutcomeInput` → map → `bridge.SubmitFlowControl`; reserved-name alias if needed.
- `T-4` Review config: define the review FlowDefinition fragment (nodes `coder`/`reviewer`/synthesis, edges incl. the `when:continue` back-edge to `coder`, `cap:3`, `onCap:escalate`) that the skill (Task-094) spawns. Coder re-entry prompt composes `Issues` (numbered `Title`+`Resolution`, grouped by `File`) + `Feedback`, merging `takeQueuedFeedbackPrompt` so queued `user-feedback` folds in.
- `T-5` Legacy gate (`interactive_service.go:884-935`): wrap the `strings.Contains(...)` keyword logic in `if loopState.Mode != "explicit"`. In explicit mode the branch is skipped entirely; a completing reviewer only contributes to the cohort note (Task-092).

## 5. Touched Areas

- files: `agent_orchestrator.go`, `claude_mcp_server.go`, `claude_permission_mcp.go`, `codex_adapter.go`, `interactive_service.go`; tests `claude_permission_mcp_test.go`, `claude_mcp_server_test.go`, `codex_appserver_test.go`, `agent_orchestrator_test.go`, `interactive_service_test.go`.
- modules: provider adapters + interactive service (review template).
- routes: none new (reuses Task-090's `flow-control` route).
- tables: none.

## 6. Acceptance Check (Definition of Done)

- [ ] `tools/list` advertises `submit_review_outcome` with the full schema on **Claude AND Codex**, on both start and resume paths.
- [ ] `parseReviewOutcomeInput` accepts the three valid statuses, rejects unknown status, rejects `changes_requested` with empty feedback, and parses the `issues` array.
- [ ] The handler **maps to `flow_control`** (`approved→done`, `changes_requested→continue`, `blocked→escalate`) and calls `bridge.SubmitFlowControl`; there is **no separate review loop driver**.
- [ ] `submit_review_outcome` is the **only** flow control tool registered (no generic `flow_control` tool exposed to the model).
- [ ] The review loop runs as config over the executor with **no review-specific transition branch** in the service (covered by a test asserting the executor path is hit).
- [ ] In **explicit mode**, two reviewers completing produce **zero** coder restarts (only the cohort note); the loop advances solely via `submit_review_outcome`.
- [ ] In **keyword mode** (`mode != "explicit"`), the legacy branch behaves **byte-for-byte unchanged** (regression test).
- [ ] `roundCapOverride` flows through to the executor `Cap`; coder re-entry prompt contains the grouped issue list + feedback.
- [ ] The runner builds; `spawn_agent`/`ask_user` behavior unchanged.

## 7. Out of Scope

- The generic vocabulary/handler (Task-089) and executor (Task-090); loop-state persistence (Task-085); the consolidated note builder (Task-092); auto-reinvoke (Task-093); skill/agent (Task-094); UI (Task-095).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
