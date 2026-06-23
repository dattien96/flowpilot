# Task-089: Submit Review Outcome Tool

## Metadata

- Document ID: `Task-089`
- Title: `submit_review_outcome Provider Tool Contract`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-090: Review Loop Driver](./Task-090-Review-Loop-Driver.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, provider-tool, codex, claude, go`

## AI Quick View

### Summary

- Add a new provider tool `submit_review_outcome` so the orchestrating agent reports a structured review verdict (`{status, issues[], feedback, roundCapOverride?}`) instead of the runner inferring it from keywords.
- Register it on both providers (Claude MCP server + Codex DynamicTool), add `parseReviewOutcomeInput`, and add a `TurnBridge.SubmitReviewOutcome` method.
- This slice is the **contract only** — it wires the tool to a thin service entry point; the loop behavior it drives lands in Task-090.

### Current Ask

- Implement and register the tool + bridge + parsing with cross-provider parity (start and resume), with no change to existing loop behavior.

### Key Decisions

- `T-1` `submit_review_outcome` mirrors the `spawn_agent` registration pattern on both providers; on Codex, alias to `flowpilot_submit_review_outcome` only if the public name is reserved (SD-16 `D-13`).
- `T-2` Validation: `status ∈ {approved, changes_requested, blocked}`; `feedback` required when `status==changes_requested`.

### Constraints

- Run GitNexus impact analysis before editing `TurnBridge`, `claude_mcp_server.go`, `codex_adapter.go`; warn on HIGH/CRITICAL.
- Additive only; do not alter `spawn_agent`/`ask_user` behavior.
- Every fake bridge implementing `TurnBridge` must be updated or the build breaks.

### Open Questions

- Codex reserved-name status for `submit_review_outcome` — verify against the live app-server (SD-18 `Q-2`).

### Source Refs

- CP-36 `P-1`; SD-18 `D-2`, §5 (data model), §6 (interfaces); SS-15 `AC-3`.
- Anchors: `agent_orchestrator.go:344` (`parseSpawnAgentInput`); `provider_registry.go:25-32` (`TurnBridge`); `interactive_service.go:1174` (`SpawnAgent` bridge impl); `codex_adapter.go:89-107,299-318`; `claude_mcp_server.go:264-296,250-259`; `claude_permission_mcp.go:182-195`.

## 1. Goal

Expose `submit_review_outcome` to both Claude and Codex with a validated structured schema, parsed into `ReviewOutcomeInput`, delivered through `TurnBridge.SubmitReviewOutcome`, returning a `ReviewOutcomeResult` — the contract the Task-090 loop driver consumes.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-1`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-2`, §6
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-3`
- specific upstream ids: CP-36 `P-1`; SD-18 `D-2`

## 3. Trigger

The loop driver (Task-090) needs a deterministic, observable verdict signal. Keyword parsing of reviewer text is brittle and breaks with multiple reviewers, so the verdict must be an explicit tool call.

## 4. Exact Change

- `T-1` Add `ReviewIssue`, `ReviewOutcomeInput`, `ReviewOutcomeResult` types + `parseReviewOutcomeInput(map[string]any)` in `agent_orchestrator.go` (mirror `parseSpawnAgentInput`). Validate per `T-2` in Quick View.
- `T-2` Add `SubmitReviewOutcome(in ReviewOutcomeInput) (ReviewOutcomeResult, error)` to the `TurnBridge` interface (`provider_registry.go:25-32`); implement `(*turnBridge).SubmitReviewOutcome` near `interactive_service.go:1174` to call `s.submitReviewOutcome(b.runID, in)` (the service method is stubbed here and fully implemented in Task-090 — for this task it may record the outcome and return current loop state).
- `T-3` Claude: add the tool def to `claudeMCPToolDefs()` with full `inputSchema` (status enum, issues array, feedback, roundCapOverride; `required:["status"]`); add the `tools/call` dispatch case → new `handleClaudeSubmitReviewOutcome` in `claude_permission_mcp.go`; add `claudeReviewOutcomeToolName` const.
- `T-4` Codex: add `codexReviewOutcomeDynamicTool()`; append to the `dynamicTools` slice (`codex_adapter.go:133`); add the `handleDynamicToolCall` case; add reserved-name alias handling if needed.
- `T-5` Update all fake bridges implementing `TurnBridge` to satisfy the new method (`codex_appserver_test.go`, `codex_resume_process_test.go`, `claude_adapter_test.go`, `fake_provider_adapter.go`, `agent_orchestrator_test.go` helpers).

## 5. Touched Areas

- files: `agent_orchestrator.go`, `provider_registry.go`, `interactive_service.go`, `codex_adapter.go`, `claude_mcp_server.go`, `claude_permission_mcp.go`, plus the fake-bridge test files in `T-5`.
- modules: local-runner provider adapters + interactive service.
- routes: none (HTTP mirror is Task-090).
- tables: none.

## 6. Acceptance Check

- `tools/list` advertises `submit_review_outcome` with the full schema on Claude AND Codex, on both `thread/start` and `thread/resume` paths.
- `parseReviewOutcomeInput` unit tests pass: valid statuses; rejects unknown status; rejects `changes_requested` with empty feedback; parses the `issues` array.
- The runner builds; every `TurnBridge` implementation compiles.
- No change to existing `spawn_agent`/`ask_user` behavior (regression tests green).

## 7. Out of Scope

- The loop state machine, coder restart, round/cap, and the HTTP routes (Task-090); the consolidated note (Task-091); auto-reinvoke (Task-092); skill/agent (Task-093); UI (Task-094).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
