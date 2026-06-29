# Task-166: Gemini Controlled Tools And Approvals

## Metadata

- Document ID: `Task-166`
- Title: `Gemini Controlled Tools And Approvals`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-27`
- Last Updated: `2026-06-27`
- Parent Documents: [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md), [Task-165: Gemini Controlled Adapter MVP](../done/Task-165-Gemini-Controlled-Adapter-MVP.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn Agent Tool And Orchestrator Core](../done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-010: Yolo Mode](Task-010-Yolo-Mode.md)
- Replaces: `None`
- Tags: `gemini, approvals, mcp, ask-user, spawn-agent, yolo-policy`

## AI Quick View

### Summary

- Validate Gemini ACP/MCP support for approval, tool, file, `ask_user`, and `spawn_agent` events.
- Wire only the supported categories into runner-owned `TurnBridge` gates.
- Keep unsupported capabilities disabled and explicit.

### Current Ask

- This task is complete as a conservative protocol slice: Gemini ACP tool and permission frames are mapped and tested, FlowPilot MCP server injection is wired, and unproven full parity remains disabled.

### Key Decisions

- `T-1` YOLO=false must fail closed if Gemini cannot route dangerous operations through runner approval.
- `T-2` YOLO=true may auto-approve only through runner-owned policy.
- `T-3` `ask_user` and `spawn_agent` must use the same `TurnBridge` methods as Codex/Claude.

### Constraints

- No stale provider approval allowlist may bypass FlowPilot policy.
- No direct desktop-specific Gemini approval UI.

### Open Questions

- Exact Gemini MCP/tool registration schema must be captured from live CLI behavior.

### Source Refs

- `CP-40` sections `P-5`, `P-6`, `P-7`, `G-04`, `G-05`, `G-06`, `G-07`, `G-17`.

## 1. Goal

Implement runner-controlled Gemini approvals and tools only for categories that Gemini ACP/MCP exposes in a structured and controllable way.

## 2. Parent Links

- coding plan: `CP-40`
- tech design: `SD-12`, `SD-06`
- system spec: `SS-11`, `SS-14`
- specific upstream ids: `CP-40 P-5`, `P-6`, `P-7`

## 3. Trigger

After Gemini can stream controlled text turns, parity depends on whether tools and approval gates can be observed and controlled by the runner.

## 4. Exact Change

- `T-1` Capture live Gemini ACP/MCP payloads for tool calls, approvals, user questions, file changes, and failures.
- `T-2` Add Gemini event mapper support for every proven structured category.
- `T-3` Add Gemini FlowPilot tool registration for `ask_user` and `spawn_agent` if Gemini supports it.
- `T-4` Add YOLO posture tests for deny, auto-approve, and stale allowlist isolation.
- `T-5` Update `ProviderCapabilities` only for proven capabilities.

## 5. Touched Areas

- files: Gemini adapter/event mapper/MCP files, `yolo_resolver.go` if needed, provider registry tests
- modules: local runner provider runtime, MCP tools, agent orchestration bridge
- routes: existing provider event and approval/question endpoints
- tables: none

## 6. Acceptance Check

- YOLO=false dangerous action denies before execution or Gemini capability remains disabled.
- `ask_user` emits provider-neutral question events and resumes after answer.
- `spawn_agent` works for wait true/false or remains disabled with capability false.
- Provider capabilities match passing tests.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Gemini ACP `tool_call` and `tool_call_update` frames map to normalized `tool_started` and `tool_completed` events.
- [x] `DOD-2` Gemini ACP `session/request_permission` routes through `TurnBridge.RequestApproval` and replies with a valid ACP permission outcome.
- [x] `DOD-3` Gemini `session/new` can receive the runner-hosted FlowPilot HTTP MCP server, using the same bridge-token server that provides `ask_user` and `spawn_agent`.
- [x] `DOD-4` Unsupported/unproven parity is not advertised: `ApprovalEvents`, `Mcp`, `FileEvents`, `Resume`, and `Vision` remain false in Gemini capabilities.
- [x] `DOD-5` Live ACP initialization was probed on Gemini CLI `0.40.1`; full live tool/approval validation is documented as blocked by missing local Gemini auth/API key.
- [x] `DOD-6` Targeted Gemini tests, full `internal/runner`, and full local-runner Go tests pass.

## 7. Out of Scope

- Cross-account resume.
- Transcript extraction and handoff source support.
- Vision attachments.

## 8. Completion Notes

- result: done
- implementation notes: added Gemini ACP event mapping for tool start/finish, permission-request handling with ACP permission responses, and `session/new` MCP server injection for the runner-hosted FlowPilot MCP server. Capability flags remain conservative until authenticated live validation proves end-to-end tool invocation.
- verification: `go test ./internal/runner -run 'TestGemini|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes|TestProviderKeyFromModel' -count=1`; `go test ./internal/runner -count=1`; `go test ./... -count=1` from `apps/local-runner`.
- live validation note: `gemini --acp --approval-mode plan --model gemini-2.5-flash` returned initialize capabilities (`loadSession`, `promptCapabilities.image/audio/embeddedContext`, `mcpCapabilities.http/sse`) but `session/new` failed with `Gemini API key is missing or not configured`, so full live permission/MCP tool parity remains unclaimed.
- follow-ups: Task-167 must keep resume/handoff/full live DOD honest and should revisit capability flags only after authenticated Gemini validation.
- upstream docs updated: `CP-40` Child Documents now points to this task in `done`.
