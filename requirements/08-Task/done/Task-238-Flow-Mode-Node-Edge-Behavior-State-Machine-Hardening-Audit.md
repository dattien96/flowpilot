# Task-238: Gia Cố State-Machine Node/Edge/Behavior Của Flow Mode (Hardening Audit)

> Ghi chú ngôn ngữ: nội dung viết bằng tiếng Việt theo yêu cầu owner; tên section giữ nguyên tiếng Anh đúng hợp đồng `SS-13`/`FORMAT-REFERENCE-TASK.md` để tooling và compliance check hoạt động. ID, tên symbol, status code giữ nguyên tiếng Anh vì là tham chiếu code.

## Metadata

- Document ID: `Task-238`
- Title: `Gia cố state-machine Node/Edge/Behavior của Flow Mode (Hardening Audit)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15` (chuyển thành **charter**: giữ catalog bất biến + quyết định + ma trận dùng chung; tách thực thi ra 4 Task con theo wave — Task-239..242)
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (`P-1`..`P-6`, `G1`, `G3`, `G5`), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (`P-1`, `P-3`), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) (`D-2`, `D-3` — khu vực A6), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) (`AC-0`, ràng buộc "bounded and stoppable")
- Child Documents: [Task-239: Flow Restore & Step-Transition Log (A2 + `T-10`)](./Task-239-Flow-Restore-And-Step-Transition-Log.md), [Task-240: Flow Ordering & Synthesis Settle Contract (A1 + A3)](./Task-240-Flow-Ordering-And-Synthesis-Settle-Contract.md), [Task-241: Cohort Join Matrix, Stall Policy & Confirm-Loop (A4 + A5 + `T-11`)](./Task-241-Cohort-Join-Matrix-Stall-And-Confirm-Loop.md), [Task-242: Flow-Mode Three-Tier Gate (A6 + `T-9`)](./Task-242-Flow-Mode-Three-Tier-Gate.md); các fix phát sinh ngoài 4 con sẽ tách thành `BUG-xxx` riêng, link ngược về đây
- Related Documents:
  - Engine/topology hiện tại: [Task-176: Node-Behavior Registry And Dispatch](../done/Task-176-Node-Behavior-Registry-And-Dispatch.md), [Task-237: Generalize Post-Node "Done" Edge-Walking](../done/Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md), [Task-235: Hub Notify Node Behavior](../done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md), [Task-236: telegram.notify Inline Node Behavior](../done/Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md), [BUG-282: Step Definition Stores Flow-Scoped Depends-On](../../09-BugFix/done/BUG-282-Step-Definition-Stores-Flow-Scoped-Depends-On-Breaking-Cross-Flow-Reuse.md)
  - A1 — timeline đúng sự thật: [BUG-174](../../09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md), [BUG-233](../../09-BugFix/done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md), [BUG-235](../../09-BugFix/done/BUG-235-Stale-Child-Run-Status-And-Flow-Engine-Prompt-Overwrites-History-Title.md), [BUG-242](../../09-BugFix/done/BUG-242-Coder-Reentry-Miscategorized-Closed-And-Synthesis-Step-Stuck-Running.md), [BUG-247](../../09-BugFix/done/BUG-247-Stop-From-Child-Focused-View-Leaves-Parent-Loop-Running.md), [BUG-248](../../09-BugFix/done/BUG-248-Stop-Leaves-Main-Run-Permanently-Stuck-Running.md), [BUG-258](../../09-BugFix/done/BUG-258-Delete-Active-Chat-Leaves-Stale-Flow-Timeline-And-Agents-Panel.md)
  - A2 — restore sau restart: [BUG-178](../../09-BugFix/done/BUG-178-Flow-Step-Timeline-Empty-After-Server-Restart.md), [BUG-250](../../09-BugFix/done/BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md), [BUG-251](../../09-BugFix/done/BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md), [BUG-256](../../09-BugFix/done/BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md), [BUG-257](../../09-BugFix/done/BUG-257-Restarted-Flow-Run-Shows-Completed-Synthesis-Step-As-Cancelled.md), [BUG-260](../../09-BugFix/done/BUG-260-Resumed-Flow-Fast-Path-Overwrites-Failed-Cohort-Member-As-Done.md), [BUG-271](../../09-BugFix/done/BUG-271-Resolved-Question-Vanishes-Or-Reappears-Interactive-After-Server-Restart.md)
  - A3 — hang ở step cuối: [BUG-179](../../09-BugFix/done/BUG-179-Hub-Finalizes-Flow-On-First-Turn-Before-Review-Cohort.md), [BUG-181](../../09-BugFix/done/BUG-181-Synthesis-Step-Status-Races-Cohort-Join-And-Finalization.md), [BUG-231](../../09-BugFix/done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md), [BUG-244](../../09-BugFix/done/BUG-244-Synthesis-Step-Stuck-Running-On-Escalate-Fallback.md), [BUG-284](../../09-BugFix/done/BUG-284-Hub-Notify-Deferred-Reinvoke-Retried-With-Wrong-Generic-Prompt.md), [BUG-285](../../09-BugFix/done/BUG-285-Hub-Notify-Reinvoke-Dropped-When-Loop-Blocked-Mid-Defer.md), [BUG-287](../../09-BugFix/done/BUG-287-Hub-Notify-Prompt-Told-AI-To-Call-Status-Done-Tool-Has-No-Such-Value.md)
  - A4 — ma trận cohort join: [BUG-100](../../09-BugFix/done/BUG-100-Yolo-Off-Spawn-Agent-Child-Approval-Hangs-Without-Surfacing-The-Gate.md), [BUG-176](../../09-BugFix/done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md), [BUG-177](../../09-BugFix/done/BUG-177-Sub-Agent-Approvals-Not-Surfaced-On-Main-Hub-View.md), [BUG-234](../../09-BugFix/done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md), [BUG-254](../../09-BugFix/done/BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md), [BUG-259](../../09-BugFix/done/BUG-259-Failed-Turn-With-Masked-Nil-Error-Bulk-Completes-Workflow-Steps.md)
  - A5 — kết quả song song lẫn lộn / vòng confirm lặp: [BUG-255](../../09-BugFix/done/BUG-255-Coder-Reentry-Output-Duplicated-Into-Main-After-Stale-Loop-State.md), [BUG-275](../../09-BugFix/done/BUG-275-Hub-Synthesis-Prompt-Duplicates-Joined-Notes-And-Misresolves-Feature-History.md), [BUG-286](../../09-BugFix/done/BUG-286-Continue-Round-Reset-Used-Wrong-Entry-Node-Stuck-Context-Coder.md)
  - A6 — flow-gate rules trong Flow Mode: [BUG-152](../../09-BugFix/done/BUG-152-Flow-Gate-Fires-On-Child-Agent-Turns-Mid-Loop.md), [BUG-243](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md), [BUG-278](../../09-BugFix/done/BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md), [BUG-279](../../09-BugFix/done/BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md), [CP-38: Flow Mode Todo (stub)](../../07-Coding-Plan/todo/CP-38-Flow-Mode-Todo.md)
- Replaces: `None`
- Tags: `agent-flow-engine, flow-mode, state-machine, node-edge-behavior, cohort-join, resume, flow-gate, hardening, regression-matrix`

## AI Quick View

### Summary

- Flow Mode giờ chạy theo node/edge/behavior: node của một flow là các row `workflow_steps` trong DB (mirror từ pack YAML hoặc user tự tạo), nối với nhau bằng `edges_json`, mỗi node dispatch một behavior đã đăng ký (`agent.delegate`, `hub.inline`, `context.produce`, `command.validate`, `artifact.audit_draft`, `telegram.notify`, `hub.notify`, ...) — CP-36/CP-42/Task-176/Task-237.
- Feature đã chạy được, nhưng **25+ BugFix docs đều rơi vào đúng 6 điểm nóng**: (A1) step-timeline hiển thị sai trạng thái, (A2) restore sai sau restart server, (A3) hang ở step cuối/synthesis, (A4) ma trận join của cohort song song, (A5) kết quả song song lẫn lộn gây vòng confirm lặp vô hạn, (A6) flow-gate rules không chạy trong Flow Mode. Mỗi fix trước đây là một miếng vá điểm; các bất biến chung chưa từng được viết ra và chưa có ma trận test hệ thống nào enforce chúng.
- Task này biến lịch sử bug thành: (a) **catalog bất biến** (`I-1`..`I-17`, §4.0) mà engine phải giữ, (b) **ma trận regression test theo state×event** cho từng khu vực thay vì replay từng repro, (c) fix các vi phạm + các gap đã biết mà những doc trước để lại.
- **Đây là charter, không phải slice thực thi.** Task-238 giữ phần dùng chung (catalog bất biến, quyết định `T-1`..`T-11`, trục ma trận, roll-up DOD); công việc thực thi chia thành 4 Task con theo wave, mỗi con có DOD nhị phân riêng (§4.1): **Task-239** (A2 + `T-10` restore/replay), **Task-240** (A1 + A3 ordering + synthesis settle), **Task-241** (A4 + A5 + `T-11` cohort/stall/confirm-loop), **Task-242** (A6 + `T-9` gate 3 tầng).
- **3 quyết định lớn đã chốt với owner (2026-07-15)**: `T-9` gate 3 tầng cho A6 (doc/scope rules tại child turn có diff, `r-tests`/`r-reg` do validate node sở hữu, audit node là chốt aggregate + nơi duy nhất commit); `T-10` persist từng step transition xuống sidecar để restore sau restart bằng replay chứ không đoán; `T-11` chính sách stall — gate hiển thị thì chờ vô hạn nhưng phải surface, member mất tín hiệu thì timeout → escalate hub với card Retry/Skip/Stop.
- Các item mở đã biết được gom vào: guard cohort-incomplete bị defer từ BUG-179, child bị cancel không re-emit `agent_graph_updated` (BUG-248), normalize `spawned`/`waiting_dependency` khi restart (BUG-251), parity Supabase store (BUG-271/BUG-250), escalate/continue chưa edge-aware (Task-237 `Q-2`), và deny `git commit` tool-level ở coding step (BUG-278 `F-2`/`F-3`).

### Current Ask

- Dùng doc này làm **charter** điều phối 4 Task con: nó sở hữu catalog bất biến, các quyết định `T-*`, trục ma trận state×event, và roll-up DOD. Mỗi Task con thực thi một wave, kiểm chứng bất biến của nó bằng test ghim, và đóng gate riêng. Charter chỉ `done` khi cả 4 con `done` + không regress test cũ + SD-20 đã update + 1 lượt live E2E xuyên khu vực. Chưa đổi code cho tới khi plan này được owner duyệt lần cuối.

### Key Decisions

- `T-1` **Bất biến trước, repro sau.** Mọi fix trong khu vực này phải gọi tên bất biến (`I-*`) mà nó khôi phục; bug không map được vào bất biến nào nghĩa là catalog (§4.0) thiếu và phải bổ sung catalog trước.
- `T-2` **Step timeline là derived state; engine là chủ sở hữu duy nhất.** Row trạng thái (`PENDING/RUNNING/WAITING_USER_APPROVAL/DONE/FAILED/SKIPPED/CANCELED`) chỉ được ghi bởi các helper transition của flow executor (`setFlowStepStatus`, `setFlowStepAwaitingUser`, `markFlowRunComplete`) — không bao giờ bởi logic refresh UI, không bao giờ bởi bulk planner trên run `flowEngineDriven` (class BUG-174/BUG-259).
- `T-3` **Bất biến thứ tự là luật:** ghi trạng thái settle đồng bộ TRƯỚC SSE emit khiến desktop refresh (`emitAgentGraph`), và nguyên nhân-trước-hệ-quả (step settle trước khi loop flip) — dòng BUG-233 → BUG-242 → BUG-244 trở thành rule có test, không phải fix từng nhánh.
- `T-4` **Trạng thái terminal là đơn điệu (monotonic).** Node/child run đã `FAILED`/`CANCELED` không bao giờ bị ghi đè thành `DONE` bởi cohort join, sweep hoàn tất flow, fast-path resume, hay merge phía desktop (class BUG-254/BUG-260/BUG-235; `mergeAgentRunsById` giữ monotonic).
- `T-5` **Mọi "blocked" phải actionable.** Bất kỳ đường nào park loop (`escalate`, chạm cap, deferred-reinvoke-khi-blocked) phải để lại: hub node `WAITING_USER_APPROVAL`, composer mở khóa, và đường resume (`resumeFlowWithFeedback`) tôn trọng pending custom prompt (class BUG-231/BUG-285). Hang im lặng không bao giờ là trạng thái kết thúc hợp lệ.
- `T-6` **Ma trận cohort được liệt kê đủ, không lấy mẫu.** Trạng thái member {completed, failed, cancelled} × gate không-terminal {waiting_approval (YOLO off), waiting_question, running} × {barrier, stop giữa round, restart} có kỳ vọng tường minh (§4.4) và test; "1 completed + 1 failed vẫn join" (BUG-254) giữ nguyên là contract.
- `T-7` **A6 đã có chính sách (xem `T-9`), giờ là enforcement.** Hiện tại full gate (`r-ca`, `r-bug`, `r-task`, `r-tests`, `r-reg`) chỉ chạy khi `parentRunID == ""` (hub); delegate child chỉ chạy artifact-output gate (`interactive_service.go:3485-3493`, chủ đích BUG-152). Trong Flow Mode step viết code LÀ delegate child, nên nhóm rule CA/bug/task/tests về cấu trúc không bao giờ thấy diff quan trọng nhất. Guard prompt-level (BUG-278 `F-1`) không được phép là tuyến phòng thủ duy nhất nữa.
- `T-8` **Restore sau restart là evidence-first (và tiến tới replay-first theo `T-10`).** Bằng chứng session của từng child luôn thắng fast-path "flow đã terminal ⇒ mọi thứ DONE" (BUG-260), mọi lần đọc từ disk đi qua normalize status (BUG-251), mọi persist site mang đủ snapshot flow (LoopState, cohort id, node label — BUG-256/BUG-257).
- `T-9` **[CHỐT 2026-07-15 — thay cho Q-1 cũ] Gate 3 tầng theo bản chất rule, không dồn một chỗ:**
  - **Tầng 1 — step tự đảm bảo gate của mình:** child turn nào có diff ≠ rỗng chạy ngay nhóm doc/scope rules (`r-ca`, `r-bug`, `r-task`, `r-fk`, `r-contract`, `r-scope`) — pure function trên diff+message, chi phí ~0; reprompt định tuyến về **đúng child đó**, tôn trọng `lifecycle: reinvoke` (bài học BUG-279). Reviewer không sửa code → diff rỗng → gate tự no-op, không tái diễn tiếng ồn BUG-152.
  - **Tầng 2 — `r-tests`/`r-reg` thuộc node `command.validate`:** flow có validate node (rag-harness) dùng chính retry-loop Task-170 làm cơ chế chạy test, nối thêm baseline oracle của flowgate để phân biệt "regression" với "fail mới"; flow **không có** validate node (review-loop) → fallback chạy `r-tests`/`r-reg` trên coder turn (parity Chat Mode, phát hiện sớm nhất — owner đã chọn phương án này).
  - **Tầng 3 — audit node là chốt aggregate:** re-check doc rules trên **diff cả flow** (bắt vi phạm cộng dồn nhiều child), và là **nơi duy nhất được commit** — kèm deny `git commit` tool-level ở coding/implement turn (đóng BUG-278 `F-2`/`F-3`).
  - Phát hiện muộn tại audit là thất bại thiết kế: review round không được phép chạy trên nền vi phạm mà tầng 1/2 bắt được. Audit là defense-in-depth, không phải detector chính.
- `T-10` **[CHỐT 2026-07-15 — thay cho Q-2 cũ] Persist từng step transition, restore bằng replay.** Mở rộng cơ chế sidecar append-only sẵn có (flow-events sidecar CP-41, pattern `questions.ndjson` BUG-271): mỗi `setFlowStepStatus`/`setFlowStepPosture` append một dòng transition. Resume = seed toàn bộ `PENDING` rồi replay log → trạng thái chính xác 100%, hết class bug "đoán từ evidence gián tiếp" (BUG-256/257/260). Heuristic evidence-walk hiện tại giữ làm fallback cho run legacy chưa có log. Chi phí: vài chục dòng ghi/run — chấp nhận được.
- `T-11` **[CHỐT 2026-07-15 — thay cho Q-3 cũ] Chính sách chờ/stall tách 2 trường hợp:**
  - **Member chờ user thật (gate approval/question hiển thị, YOLO off):** KHÔNG timeout — chờ vô hạn là hợp lệ; nghĩa vụ là surface (`I-12`): gate hiện trên main-hub view + board ghi rõ "đang chờ approve của <node>" để không đọc nhầm thành hang.
  - **Member mất tín hiệu (stalled):** không có provider event trong `T` phút VÀ không có gate nào hiển thị → hub blocked với `BlockReason="member_stalled"` → user nhận card 3 lựa chọn: **Retry member / Skip member** (mark `FAILED`, barrier vẫn join theo `I-11` mixed-outcome) / **Stop flow**. Tín hiệu stall đo bằng "không có event", KHÔNG đo wall-clock của turn (coder chạy 10+ phút là bình thường). `T` configurable qua flow policy (pack data, additive), default 10 phút.
  - Chốt luôn 2 ô ma trận còn trống: member bị **cancel** = terminal (barrier join tiếp, note ghi cancelled); **cả 2 member cùng fail** = barrier vẫn join → hub thấy 2 kết quả failed → synthesis escalate cho user quyết.

### Constraints

- Giữ nguyên primitive và ngữ nghĩa CP-36: `FlowNode`/`FlowEdge`/`FlowPolicy`, barrier join-all, back-edge có giới hạn (`cap`, extend), `flow_control(continue|done|escalate)` là điểm settle duy nhất, hub là router duy nhất.
- Behavior vẫn do data chọn, Go enforce (CP-42 `P-1`): không fix nào được chuyển bảo đảm state-transition vào agent markdown hay pack YAML. Gate-profile per node (nếu cần cho `T-9`) chỉ là data chọn trong các rule-family Go đã enforce.
- Không làm regress các fix cũ — test của chúng (`TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`, `TestApplyFlowControlLoopingResetsStepsSynchronously`, `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, bộ E2E review-loop/rag-harness) phải xanh nguyên trạng.
- Oracle rule: không được sửa/yếu hóa test cũ để ma trận pass; kỳ vọng xung đột = xung đột spec, phải escalate.
- Additive-only với pack built-in: `review-loop.yaml`, `rag-harness.yaml`, `context-coding-review-synthesis.yaml` phải chạy nguyên trạng (field mới như `stall_timeout` chỉ được thêm dạng optional có default).
- `T-9` tầng 2 nới ngữ nghĩa SD-20 `D-3` ("r-tests/r-reg không auto-remediable") cho riêng Flow Mode: validate-node retry (cap 3 → escalate) vẫn kết thúc ở human stop nên giữ đúng tinh thần always-block — SD-20 PHẢI được update tường minh điểm này trước/cùng lúc implement A6 (xem §8).
- Bắt buộc chạy `gitnexus_impact` trước khi sửa mọi symbol khi bắt đầu implement; nếu MCP không khả dụng trong session làm việc, ghi nhận lại và inspect thủ công (như Task-237 đã làm).

### Open Questions

- `Q-1` (→ Task-241) Default stall timeout 10 phút có phù hợp cho mọi provider/behavior không, hay cần default khác nhau theo behavior (delegate coder vs reviewer)? → tune khi implement `T-11`, đo bằng log thực tế.
- `Q-2` (→ Task-239) Sidecar step-transition log (`T-10`) có cần sync qua Drive như `sessions.ndjson` không, hay chỉ local là đủ (restore sau restart là nhu cầu local; cross-PC resume mid-flow có là use case thật?)? → quyết khi implement, mặc định nghiêng "có sync" cho nhất quán CP-36 `P-5`.
- `Q-3` (→ Task-240) BUG-285 `Q-1` chưa root-cause: vì sao model synthesizer gọi `escalate()` ngay sau `done()` trong cùng một turn. Guard một-quyết-định-mỗi-turn (BUG-255) + re-arm (BUG-285) đã mitigate; có cần thêm contract test prompt↔tool-schema (class BUG-287) cho mọi tool hub-facing không — dự kiến CÓ.
- `Q-4` (→ Task-242) Trong Flow Mode, coder tự viết CA note (đúng theo `r-ca`, BUG-278 xác nhận không phải bug) VÀ audit node build CA draft aggregate (Task-171) — hai artifact CA có thể trùng nội dung. Cần quy ước dedup/merge (coder note = per-turn, audit draft = per-flow tổng hợp?) khi implement tầng 3.

### Source Refs

- CP-36 `P-1`..`P-6`, `G1`/`G3`/`G5`; CP-42 `P-1`/`P-3`; SD-19 `D-1`/`D-4`; SD-20 `D-2`/`D-3` (gate mode + auto-remediation — sẽ nới cho Flow Mode theo `T-9`); SS-16 `AC-0`, ràng buộc bounded/stoppable.
- Code anchors (hiện tại): `apps/local-runner/internal/runner/interactive_service.go` (`applyFlowControl` :590, gate dispatch :3485-3493, nhánh cohort-join, `maybeAutoReinvokeHubWithNote`/`WithPrompt`, `pendingHubReinvoke*`), `flow_executor.go` (`tryAdvanceFlowFromNode` :927, `resolveContinueBackEdgeTarget` :836, `forwardReachableNodeIDs` :862, `reinvokeExistingFlowChild` :1186), `flow_step_runtime.go` (`setFlowStepStatus` :152, `setFlowStepAwaitingUser` :192, `markFlowRunComplete` :238, `reconcileChildRunsOnFlowDone` :320), `flow_validate_audit_dispatch.go` (`tryAdvanceFlowThroughInline` :48, `runValidateNode`/`runAuditNode`, back-edge "retrying" ~:219), `interactive_resume.go` (`reconstructRun` :664, `resumedFlowStepRows` :554, `normalizeResumedStatus` :648, `resumedFlowRunIncomplete` :413), `gate_hook.go` (`runFlowGate` :39, `runChildArtifactOutputGate` :224), `agent_orchestrator.go` (cohort buffer :64-118, `upsertSummary`/`graphSnapshot`), `internal/flowgate/rules.go` (`DefaultRules` :119), desktop `apps/desktop-flowpilot/src/state/store.ts` (`mergeAgentRunsById`, `deriveOrchestrationRunStatus`, `stop`, `deleteHistoryRun`→`resetRun`).

## 1. Goal

Làm cho state machine node/edge/behavior của Flow Mode ổn định một cách chứng minh được trên cả 6 điểm nóng lịch sử: viết ra các bất biến engine đang hứa ngầm, enforce bằng ma trận regression test liệt kê state×event, đóng các gap còn lại mà những BugFix trước chủ đích defer, và implement chính sách gate 3 tầng đã chốt để nhóm rule `r-ca`/`r-bug`/`r-task`/`r-tests`/`r-reg` thực sự áp dụng cho flow runs.

## 2. Parent Links

- coding plan: `CP-36` (engine + review loop), `CP-42` (generic node behavior / flow pack)
- tech design: `SD-19` (agent flow engine), `SD-20` (flow gate rule semantics — khu vực A6, sẽ nhận update `D-*` theo `T-9`)
- system spec: `SS-16` (flow engine generic; bounded and stoppable), `SS-15` (ngữ nghĩa review-until-clean)
- specific upstream ids: CP-36 `P-3` (generic `flow_control`), `P-4` (bounded + ask-on-cap), `P-5` (unified local persistence — nền cho `T-10`), `G1` (resume khôi phục đủ flow fields); CP-42 `P-1`/`P-3`; SD-20 `D-2`/`D-3`

## 3. Trigger

Flow Mode chạy được end-to-end, nhưng owner báo cáo bất ổn lặp đi lặp lại sau nhiều fix điểm, tập trung ở 6 khu vực (báo cáo 2026-07-15):

1. Trạng thái step trên timeline sai giữa chừng — đã start mà vẫn `PENDING`, đã done mà vẫn `RUNNING` — đặc biệt khi chuyển qua lại giữa các chat, hoặc giữa view sub-agent và main-hub.
2. Trạng thái run/chat sau khi tắt server mở lại rất hay sai — `failed`/`completed`/`cancelled`/`done`/`waiting approval` restore không đúng.
3. Flow rất hay hang ở step cuối — synthesis không thoát khỏi `RUNNING`.
4. Môi trường chạy song song (vd 2 reviewer) sai logic tương tác với main hub quanh barrier đợi-cả-hai; mỗi member có thể done/failed/cancel/waiting (YOLO off) — ma trận tổ hợp lớn, nhiều edge case chưa từng test.
5. Kết quả từ các step song song lẫn lộn khiến main agent không thể done, cứ show confirm popup lặp lại.
6. Các rule kiểu `r-ca`, `r-bug`, `r-task`, ... không chạy trong môi trường flow.

Sổ BugFix xác nhận mỗi khu vực có 4–8 sự cố trước đó (xem Related Documents) và nhiều doc đã chủ đích defer các hạng mục gia cố. Task này gom chúng lại thay vì đợi sự cố thứ 26.

## 4. Exact Change

### 4.0 `T-1` Catalog bất biến (canonical — section này là nguồn authoritative; mỗi bất biến có đúng một Task con sở hữu test ghim + được nhắc lại thành doc comment trong `flow_executor.go`)

| ID | Bất biến | Thiết lập bởi | Con sở hữu |
|---|---|---|---|
| `I-1` | Ghi step-status settle đồng bộ trước `emitAgentGraph` (không `go func` giữa transition và emit) | BUG-233, BUG-242 | Task-240 |
| `I-2` | Nguyên nhân trước hệ quả: step settle (`WAITING_USER_APPROVAL`/`DONE`) trước khi loop status flip (`blocked`/`done`) | BUG-244 | Task-240 |
| `I-3` | Trạng thái terminal của step/run là đơn điệu — không bao giờ bị ghi đè "tốt lên" bởi join, sweep hoàn tất, resume, hay UI merge | BUG-254, BUG-260, BUG-235 | Task-239 (resume) + Task-240 (live merge) |
| `I-4` | Mọi đường auto-advance (`tryAdvanceFlowFromNode`, ghi cohort-join, reinvoke) gate trên loop status sống; loop `blocked`/`done` không advance gì | BUG-234 | Task-240 |
| `I-5` | Tối đa một quyết định `flow_control` có hiệu lực mỗi provider turn; các call sau trong cùng turn bị reject | BUG-255 | Task-241 |
| `I-6` | Mọi đường park loop để lại trạng thái actionable: hub node `WAITING_USER_APPROVAL`, composer mở, endpoint resume tiếp tục được | BUG-231 | Task-240 |
| `I-7` | Deferred hub reinvoke re-arm (`pendingHubReinvoke*`) khi loop block giữa defer, và `resumeFlowWithFeedback` tiêu thụ pending custom prompt trước note generic | BUG-284, BUG-285 | Task-240 |
| `I-8` | Attribution "done" được verify theo active hub node; một `done` không liên quan không thể hoàn tất công việc của node khác | BUG-285 (`activeHubNodeID`), Task-235 | Task-240 |
| `I-9` | Node hoàn tất phải đi theo edge `done` tự khai báo của nó (terminal vs successor thật) — không bao giờ mặc định "tôi là node cuối" | Task-237 | Task-240 |
| `I-10` | Reset round mới của continue nhắm vào target của back-edge continue và chỉ reset các node forward-reachable | BUG-286 | Task-241 |
| `I-11` | Barrier cohort yêu cầu mọi member terminal; chỉ member `completed` mark `DONE`, failed giữ `FAILED`, barrier vẫn join khi outcome hỗn hợp | BUG-176, BUG-254 | Task-241 |
| `I-12` | Gate không-terminal của member (approval/question, YOLO off) phải surface trên main-hub view — gate ẩn là defect, không phải wait | BUG-100, BUG-177 | Task-241 |
| `I-13` | Mọi persist site mang đủ snapshot flow (LoopState, `flow_cohort_id`, label node của child); mọi lần đọc disk đi qua normalize status | BUG-251, BUG-256, BUG-257 | Task-239 |
| `I-14` | Rebuild khi resume là evidence-first: bằng chứng session từng child thắng fast-path terminal-loop | BUG-260 | Task-239 |
| `I-15` | Note cohort đã join được embed đúng một lần vào prompt kế của hub (embed-once → Task-241), và prompt hub-facing phải khớp schema của tool được offer (contract → Task-240) | BUG-275, BUG-287 | Task-241 + Task-240 |
| `I-16` | Barrier không bao giờ chờ vô hạn trên member đã mất tín hiệu: stall (không event trong `T` phút, không gate hiển thị) phải escalate thành trạng thái actionable (`T-11`) | Quyết định owner 2026-07-15 | Task-241 |
| `I-17` | Restart không được làm mất/đổi trạng thái node đã settle: restore = replay transition log (`T-10`) + normalize phần in-flight (RUNNING/gate đang chờ không phải trạng thái settle), không đoán; heuristic chỉ là fallback legacy | Quyết định owner 2026-07-15 | Task-239 |

Nguyên tắc audit của mỗi con: với từng bất biến nó sở hữu, tìm mọi code path bị ràng buộc, xác nhận tuân thủ, thêm regression test ở chỗ chưa có test ghim. Không bất biến nào được orphan (roll-up DOD §6 kiểm điều này).

### 4.1 `T-1` Phân rã thành 4 Task con (theo wave)

Chi tiết Exact Change + DOD của từng khu vực nằm ở Task con; charter chỉ giữ bản đồ và trục ma trận dùng chung. **Mỗi Task con có `§9 Coding Guide`** (anchor code thật đã xác minh 2026-07-15: hàm/line/chữ ký, các bước implement theo thứ tự, test scaffolding, bẫy đã biết, tiêu chí dừng-và-hỏi) — người implement bắt đầu từ §9 của con tương ứng, không cần khảo sát lại.

| Wave | Task con | Khu vực | Quyết định/Bất biến chính | Trục ma trận sở hữu |
|---|---|---|---|---|
| 1 | [Task-239](./Task-239-Flow-Restore-And-Step-Transition-Log.md) | A2 restore | `T-8`,`T-10`; `I-3`(resume),`I-13`,`I-14`,`I-17` | **Ma trận restart**: run end-state {engine_done, failed, cancelled, blocked(cap), blocked(escalate), waiting_approval, waiting_question, kill-giữa-turn, kill-giữa-cohort} × node-role {hub, delegate child, inline} |
| 2a | [Task-240](./Task-240-Flow-Ordering-And-Synthesis-Settle-Contract.md) | A1 timeline + A3 hang | `T-2`,`T-3`,`T-5`; `I-1`,`I-2`,`I-3`(live),`I-4`,`I-6`,`I-7`,`I-8`,`I-9`,`I-15`(contract) | Không ma trận riêng; sở hữu **contract settle** của `applyFlowControl` (done/continue/escalate) + `pendingHubReinvoke*` × {running, blocked(cap), blocked(escalate), done} |
| 2b | [Task-241](./Task-241-Cohort-Join-Matrix-Stall-And-Confirm-Loop.md) | A4 cohort + A5 mixed + stall | `T-4`,`T-6`,`T-11`; `I-5`,`I-10`,`I-11`,`I-12`,`I-15`(embed),`I-16` | **Ma trận cohort 2-member**: mỗi member {completed, failed, cancelled(stop), cancelled(restart), waiting_approval, waiting_question, running-stuck} → kỳ vọng barrier + hub reinvoke |
| 3 | [Task-242](./Task-242-Flow-Mode-Three-Tier-Gate.md) | A6 gate | `T-7`,`T-9` | Không ma trận; sở hữu ma trận rule×tier: {r-ca,r-bug,r-task,r-fk,r-contract,r-scope} tại tier-1 child diff · {r-tests,r-reg} tại tier-2 validate/coder · doc-aggregate + commit-deny tại tier-3 audit |

Thứ tự **khuyến nghị tuần tự**: Task-239 trước (nền restore cho E2E các con sau) → Task-240 → Task-241 → Task-242. Task-240 và Task-241 **không nên** làm song song dù về mặt scope tách được: cả hai cùng sửa `applyFlowControl` (continue branch) và cohort-join branch trong `interactive_service.go` — 240 sở hữu ordering (`I-1`/`I-2`) tại đó, 241 thêm one-decision-per-turn (`I-5`) + embed-once (`I-15`) + stall tại đó — nên chạy nối tiếp để tránh conflict trên cùng hàm nóng. Ràng buộc phụ thuộc: guard cohort-incomplete của Task-240 (`T-4`, BUG-179) **đọc** trạng thái cohort mà Task-241 sở hữu (`I-11`); cohort-tracking hiện có (`preRegisterCohort`/`cohortComplete`) đã đủ để 240 đọc, nhưng nếu 241 đổi ngữ nghĩa terminal (cancelled=terminal, `T-2`) thì 240 phải rebase theo. Task-242 cuối vì đụng upstream SD-20.

## 5. Touched Areas (bản đồ tổng — chi tiết ở từng con)

- modules: flow engine local-runner (`internal/runner`), flow gate (`internal/flowgate`), persistence/resume session, agentpack/flow-pack, orchestration store desktop.
- Task-239: `interactive_resume.go`, `flow_step_runtime.go`, `local_file_session_store.go` + sidecar transition log.
- Task-240: `interactive_service.go` (`applyFlowControl`, cohort-join branch, `maybeAutoReinvokeHub*`, `resumeFlowWithFeedback`, `stopAgentLoop`, `finishTurn`), `flow_executor.go`, `flow_validate_audit_dispatch.go`, desktop `store.ts` (`mergeAgentRunsById`, `deriveOrchestrationRunStatus`).
- Task-241: cohort branch trong `interactive_service.go`, `agent_orchestrator.go` (cohort buffer), `codex_adapter.go` (BUG-259), `agentpack`/`flow-pack` policy `stall_timeout` (additive), desktop card `member_stalled`.
- Task-242: `gate_hook.go`, `internal/flowgate/`, `flow_validate_audit_dispatch.go`, agent prompt guard + tool-level commit deny; upstream SD-20.
- routes: dự kiến không route mới (`agent-loop/continue` tái dùng; card stalled dùng chung endpoint blocked — Task-241 xác nhận `member_action` param khi implement).
- tables: không — định nghĩa `workflows`/`workflow_steps` không đổi; run data ở `sessions.ndjson` + sidecar.

## 6. Acceptance Check (roll-up gate của charter)

Charter chỉ `done` khi tất cả điều kiện dưới đúng — DOD chi tiết per-wave nằm ở §6 của từng Task con.

- `RU-1` Cả 4 Task con (Task-239, 240, 241, 242) `Status: done` với DOD riêng của chúng pass.
- `RU-2` Mọi bất biến `I-1`..`I-17` có ≥1 test ghim thuộc con sở hữu (cột "Con sở hữu" §4.0); không bất biến nào orphan — kiểm bằng một bảng traceability cuối (invariant → test name → file).
- `RU-3` Khối doc comment liệt kê `I-1`..`I-17` tồn tại ở đầu `flow_executor.go` (nguồn nhắc trong code, trỏ về charter này).
- `RU-4` Không regress: `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`, `TestApplyFlowControlLoopingResetsStepsSynchronously`, `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, bộ E2E review-loop/rag-harness — xanh nguyên trạng; `go test ./internal/runner/... ./internal/flowgate/...` + desktop `store.test.ts` xanh.
- `RU-5` SD-20 đã nhận `D-*` mới cho gate 3 tầng + nới `D-3` Flow Mode (do Task-242 thực hiện); CP-38 stub đóng/redirect về charter này.
- `RU-6` Một lượt live E2E xuyên khu vực trên review-loop, YOLO off: verdicts hỗn hợp → blocked actionable (A3/A5) → continue → done sạch; restart server giữa lúc blocked → restore đúng trạng thái actionable (A2); gate `r-ca` kích khi coder đổi code thiếu CA note (A6).

## 7. Out of Scope

- Feature flow mới, behavior node mới, pack shape mới (dòng Task-235/236/237 tiếp tục riêng).
- Redesign UI timeline/agents panel ngoài các fix đúng-sự-thật trạng thái (badge round, transcript routing — các item `Q` của BUG-234 vẫn defer).
- UX điều hướng cross-chat (BUG-235 #3) trừ khi audit chứng minh nguyên nhân corrupt trạng thái.
- Parity run-state phía Supabase (question read-back, remote step store) — ghi nhận là bất đối xứng chấp nhận trừ khi đóng được rẻ (Task-239).
- Mọi thứ vector/semantic (ràng buộc CP-41) và feature adapter provider ngoài fix tín hiệu lỗi của BUG-259.
- Thay thế ngữ nghĩa bounded-loop (`cap`/extend) — chỉ observability và resumability trong scope.

## 8. Completion Notes

- result: `done` (2026-07-15) — charter + 4 Task con đã land code/tests: [Task-239](./Task-239-Flow-Restore-And-Step-Transition-Log.md), [Task-240](./Task-240-Flow-Ordering-And-Synthesis-Settle-Contract.md), [Task-241](./Task-241-Cohort-Join-Matrix-Stall-And-Confirm-Loop.md), [Task-242](./Task-242-Flow-Mode-Three-Tier-Gate.md). Follow-up Codex re-audit gaps gộp vào [BUG-288](../../09-BugFix/done/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md) (Vòng 8–9).
- follow-ups: thứ tự thực thi **tuần tự** Task-239 → Task-240 → Task-241 → Task-242 (240 và 241 KHÔNG song song vì cùng sửa `applyFlowControl`/cohort-join — xem §4.1). Mỗi con giải Open Question được gán (`Q-1`→241, `Q-2`→239, `Q-3`→240, `Q-4`→242). Fix phát sinh ngoài 4 con → tách `BUG-xxx` link về charter.
- upstream docs updated: chưa — SD-20 PHẢI nhận `D-*` mới (gate 3 tầng + nới D-3 cho Flow Mode) trước/cùng lúc Task-242; CP-38 stub (SD-17 §4.1 "workflow run?") được Task-242 thay thế, đóng/redirect về charter này khi đó.
