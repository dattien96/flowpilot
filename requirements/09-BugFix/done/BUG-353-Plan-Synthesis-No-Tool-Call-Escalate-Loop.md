# BUG-353: plan_synthesis hub 2 lần approve-bằng-text, không gọi submit_review_outcome (escalate loop)

## Metadata

- Document ID: `BUG-353`
- Title: `plan_synthesis hub 2 lần approve-bằng-text, không gọi submit_review_outcome (escalate loop)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/done/CP-58-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [Task-305](../../08-Task/done/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md)
- Replaces: `none`
- Tags: `agent-flow-engine, task-harness, plan-review-loop, hub-inline, tool-call, escalate`

## AI Quick View

### Summary

- Symptom (live 2026-09-04, S6 task-harness smoke, target `D:\working\gate-sandbox` GCD prompt): `plan_synthesis` escalate `Hub synthesis turn completed without calling submit_review_outcome` 2 lần liên tiếp (lần đầu + 1 Retry). Final message cả 2 lần đều là APPROVE (`Plan 'calc-core' approved …`, rồi `Re-affirmed approved – awaiting flow engine to advance…`). Retry thêm sẽ loop vô hạn → Stop.
- Expected (S6-nhánh-approved): approve → `done` → `preflight_contract_freeze`.
- Actual: engine đòi tool call thật, hub chỉ verdict bằng text → escalate; Retry giữ scope cũ nên lặp đúng kết quả.
- Loại trừ: pack shape `plan_synthesis` giống hệt `synthesis` của rag-harness (hub.inline, `join: all`, tools ở flow-level `tools/submit-review-outcome.yaml`) — proven path nên pack không phải diff. Nghiêng về turn execution phía hub trên model `opencode-go/muse-spark-1.2-contributor` (reasoning high, YOLO auto): 2/2 miss, câu chữ "awaiting flow engine to advance" cho thấy model tưởng text là đủ.
- Cần soi tiếp (chưa làm): runner log turn đó — tool có được offer trong MCP/tool list của hub turn không, prompt synthesis có câu lệnh gọi tool đủ mạnh không; đối chứng bằng 1 run S cùng prompt nhưng main model khác (codex/grok) xem có gọi tool không → phân biệt model-specific vs prompt/tool-wiring.

### Current Ask

- Fix để approve-bằng-text không kẹt flow (ít nhất 1 trong: prompt bắt gọi tool mạnh hơn, hoặc engine chấp nhận verdict text rõ ràng như fallback có kiểm soát), rồi chạy lại S6 pass.
- runId live: (chờ operator paste — cần để mò runner log turn `plan_synthesis`).

## Root cause (2026-09-04, run-198468 — trace từ cli-runner.log)

- Model `opencode-go/muse-spark-1.2-contributor` **ĐÃ gọi tool** `flowpilot_submit_review_outcome` (ACP `tool_call` + `tool_call_update`, `status=approved`, feedback duyệt GCD) → MCP thành công: `rawOutput={"status":"running","round":0,"cap":3,"nextAction":"looping"}`, `status=completed`.
- `plan_synthesis --done--> preflight_contract_freeze` (node thật, không phải terminal `done`) → `advanceHubDoneThroughEdge` chiếm transition, dispatch successor, `handled=true`.
- **BUG**: nhánh successor generic KHÔNG stamp `lastFlowControlTurnID` (nhánh `hub.notify` có, BUG-289). `flowControlSubmittedForTurn` = false → BUG-226 escalate (`[flow-step] hub synthesis turn "turn-198535" completed without submit_review_outcome, escalating`) → park `WAITING_USER_APPROVAL` trên `plan_synthesis`. Retry (`turn-198567`, `turn-198585`) lặp y chang → escalate loop.
- Không phải lỗi model không chịu gọi tool; không phải prompt/tool-wiring. Lỗi shared engine, mọi provider.

## Fix (2026-09-04, done)

- `internal/runner/interactive_service.go` `advanceHubDoneThroughEdge` nhánh successor generic: stamp `lastFlowControlTurnID = currentTurnID` (copy nhánh hub.notify BUG-289) → turn hoàn thành được tính là decision, BUG-226 không fire.
- Đồng bộ BUG-284 semantics với nhánh hub.notify: khi loop còn `running` → trả `Status:"done"` + `NextAction:"advancing"` (không còn `"looping"` — đọc như "bị reject", model từng gọi thêm escalate vì câu chữ này); `done`/`blocked` giữ nguyên.
- Tests mới `bug353_hub_done_successor_stamp_test.go` (matrix Claude/Codex/Grok qua `registerKeyedCapture`):
  - `TestBug353HubDoneRealSuccessorStampsDecision`: topology `plan_synthesis -> preflight_contract_freeze`; `advanceHubDoneThroughEdge(done)` → `flowControlSubmittedForTurn` true (đúng gate BUG-226), duplicate flow_control cùng turn bị reject, `NextAction != looping`, `Status != ""`.
  - `TestBug353HubDoneAuditSuccessorAlsoStamps`: shape `synthesis -> audit` (rag-harness); stamp + audit RUNNING/WAITING + `NextAction != looping`.
- R2: provider-agnostic — `advanceHubDoneThroughEdge`/`SubmitFlowControl` không branch trên providerKey (grep evidence); matrix 3 provider lock future drift.
- R1: pre-existing tests liên quan đều green, không sửa file test cũ: `rag_harness_synthesis_edges`, `flow_hub_notify`, `flow_telegram_notify`, `task_harness_dual_loop`, `cp53_review_done_verdict` (10), `bug289`, `bug302`, `bug306`, `bug308`, `run45103`, `TestApplyFlowControlContinueHubRouting`, `TestCoderCompletionAutoSpawnedReviewerPrompt...`. `TestSpawnChildEmitsGraphAndBusEvents` fail là pre-existing environmental (đã verify trên baseline stash trước đó), không liên quan.
- Live-verify pending: rebuild runner → `/flow task-harness` + prompt GCD → S6 expect `plan_synthesis` approve → `preflight_contract_freeze` chạy.
