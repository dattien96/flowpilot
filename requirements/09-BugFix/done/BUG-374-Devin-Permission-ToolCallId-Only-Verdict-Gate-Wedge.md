# BUG-374: Devin `session/request_permission` carries only `toolCallId` → verdict tools auto-reject → all verdict-gated flows wedge

## Metadata

- Document ID: `BUG-374`
- Title: `Devin permission request has no tool name — submit_review_outcome/ask_user denied on every gated node`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-48-Standardize-Doc](../../07-Coding-Plan/done/CP-48-Standardize-Doc.md), [CP-41-RAG-Harness-Flow-Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-66-Living-Knowledge-Base-Context-Source](../../07-Coding-Plan/done/CP-66-Living-Knowledge-Base-Context-Source.md), [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Feature Keys: `agent-flow-engine, node-isolation, ai-providers`

## AI Quick View

### Summary

- On provider `devin`, every `session/request_permission` frame carries `toolCall: {"toolCallId":"call_…"}` ONLY — no `title`, `rawInput`, or `kind`. `devinApprovalDetailsFromRequest` (`apps/local-runner/internal/runner/devin_adapter.go:922`) therefore produces `ApprovalDetails{Command:"", Reason:"", Kind:"other"}`.
- Under a gated flow-node posture (`read_only`/`verdict_only`), `isVerdictToolCall` (`node_isolation.go:120`) and `isReadOnlyToolName` need the tool name in Reason/Command → both miss → fail closed → bridge replies `{"outcome":{"optionId":"reject_once"}}`.
- `submit_review_outcome` and `ask_user` (any MCP tool) can never execute on a gated node under devin → machine verdict never recorded → `synthesis`/`plan_synthesis` blocked on `missing machine verdict` → auto-escalate → `flow_parked_awaiting_user`. The verdict-face exemption that read_only posture intends to grant is unreachable.
- Confirmed 4× live: CP-48 run-11 (reviewer run-815: 3× submit_review_outcome + 1× ask_user denied; respawn run-3142: 3× denied), CP-41 run-1050 (turn-1973/turn-2188), CP-66 run-1 (plan_reviewer denied 4×, reviewer denied 3×+; park's `session/cancel` also killed in-flight reviewer turn-10616 mid-verdict), CP-70 R6 run-665 (`submit_review_outcome` rejected 3×).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Fix direction noted by testers: correlate the in-flight tool name from `toolCallId` when building `ApprovalDetails` (the full tool args were visible in the preceding `tool_call` session/update frame), or include the MCP name in the permission request. The tool name also survives inside option labels, e.g. `"Yes, allow calling submit_review_outcome on the flowpilot MCP server (this session)"`.

## Bug report

- **Symptom**: On devin, a reviewer/plan-reviewer child on a `read_only` or `verdict_only` node calls `submit_review_outcome` (or `ask_user`) → the call is denied `reject_once` every time; the hub then blocks `missing machine verdict from review reviewer(s)` → escalate → park. Flow cannot finish; each continue re-loops the same failure.
- **Expected**: The posture whitelist explicitly intends to allow the verdict face — "a read_only reviewer MUST be able to submit it" (`chat_posture_policy.go:58-66`, Task-349) → `submit_review_outcome`/`ask_user` should be approved and the verdict recorded.
- **Actual**: All such calls denied. CP-48 `approvals.ndjson` shows appr-1375/1972/2310/2316 + appr-3585/4129/4498 all `deny`, `flow_node_posture_read_only`, empty Command/Reason. Node completes only via prose/text fallback after wasted reprompt turns; the machine verdict is never recorded.
- **Impact**: CRITICAL — every dev-startable flow with a review/plan cohort (`bug-harness`, `task-harness`, `bug-plan-harness`, `cp-harness` plan cohort) deadlocks under provider `devin`. No audit node behind a cohort-gated synthesis is reachable; hub parks `awaiting_user` indefinitely and each park sends `session/cancel` to all sessions (killed in-flight reviewer turn-10616 on CP-66).

## Reproduction

1. Runner with `FLOWPILOT_DEVIN_AGENT=1`; provider `devin`, model `devin/swe-2-max` (effective `swe-2-high` where max was rejected — see BUG-379), `yoloMode:true` (child still runs under gated node posture → `yoloModes=false` on the bridge).
2. Start any flow with a review cohort + verdict-gated synthesis, e.g. `POST /client/workflow-runs` `flowRef:"task-harness"` (CP-66 run-1, CP-48 run-11) or `flowRef:"bug-harness"` (CP-41 run-5/run-1050, CP-70 run-665).
3. Let the flow reach the reviewer node → the child calls `submit_review_outcome`.
4. Observe runner.log: `session/request_permission` params `toolCall={"toolCallId":…}` only; runner answers `{"outcome":{"optionId":"reject_once"}}` (cp48 runner.log L1896-1898, L2562-2564, L2972-2974; cp66 runner.log:1414-1415,1444-1445,2316-2317,2765-2766; cp41 runner.log:2145-2146,2643,2984). Devin reports "Tool execution was rejected: User rejected this tool call".
5. Hub `submit_review_outcome`/`done` → `flow_control_rejected_missing_review_verdict` → `WAITING_USER_APPROVAL` park.

## Root cause

- `apps/local-runner/internal/runner/devin_adapter.go:860` — permission handler calls `devinApprovalDetailsFromRequest(req.Params)`.
- `devin_adapter.go:922-952` — `devinApprovalDetailsFromRequest` derives `Command`/`Reason` only from `toolCall.title` and `toolCall.rawInput{command|cmd|filepath|filePath|path}`; devin's live wire carries none of these (only `toolCallId`) → `Command:""`, `Reason:""`, `Kind:"other"` via `devinPermissionKind` (:954).
- `node_isolation.go:107` — `readOnlyApprovalDecision`: `isReadOnlyToolName(details.Reason) || isVerdictToolCall(details)` → both false on empty fields → deny.
- `node_isolation.go:120-129` — `isVerdictToolCall` matches the verdict tool name in Reason/Command; `isAskUserTool` (:39) same blindness — name-based exemptions cannot fire.
- Downstream: `interactive_service.go:6450-6466` (hub done-verdict gate, `review_done_verdict.go:252-288`) never sees a recorded verdict → `hubDoneVerdictError` → escalate → park.

## Evidence

- `~/fp-beds/lt-evidence/cp48/RESULT.md` (BUG-LIVE-CP48-2), `L-48-3-approvals.ndjson` (7 denials, empty Command/Reason), `L-48-3-permission-deny-excerpt.txt` (`session/request_permission` ↔ `reject_once` pairs), `L-48-3-flow-diag-run-11.ndjson`, `runner.log`.
- `~/fp-beds/lt-evidence/cp41/RESULT.md` (BUG-LIVE-2), `l41-1/run-1050-turns.ndjson` (reviewer: "All FlowPilot tool calls are being rejected — submit_review_outcome (3×) and the ask_user fallback"), `runner.log`.
- `~/fp-beds/lt-evidence/cp66/RESULT.md` (BUG-LIVE-66-2), `runner.log:1414-1415,1444-1445,2316-2317,2765-2766,14087`, `L-66-3-inject-evidence.txt` (test-only workaround: direct `tools/call submit_review_outcome` to the child's live per-turn MCP token bypasses the permission layer → verdict recorded).
- `~/fp-beds/lt-evidence/cp70/RESULT.md` (R6, CP48-2 ceiling; R4 payload exposes only `toolCallId` + `_meta.editableCommand`), `r6-verdict-denials.txt`, `r4-permission-wire.txt`, `r6-reviewer-events.json`.

## Severity

- critical

## Completion Notes (implemented 2026-09-22, CA-916b)

- Root cause confirmed from live wire (cp46/cp70): `session/request_permission` `toolCall` carries only `{toolCallId, _meta.cognition.ai/editableCommand}` — no title/kind/rawInput.
- Fix: `devinToolCallIndex` per-session cache populated by `session/update` `tool_call` start frames; `devinApprovalDetailsFromRequest(params, idx)` enriches the request from the correlated frame, falls back to parsing the MCP tool name from option labels, and reads `cognition.ai/editableCommand`. Reason now carries the real tool name so verdict/ask_user/read-only matchers fire.
- Files: `internal/runner/devin_event_mapper.go`, `internal/runner/devin_adapter.go`.
- Tests: `bug_devin_toolcall_correlation_test.go` (correlation + options fallback + editableCommand). Provider-isolated — no shared-path change.
