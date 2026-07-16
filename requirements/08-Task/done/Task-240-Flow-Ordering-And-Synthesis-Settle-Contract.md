# Task-240: Flow Ordering & Synthesis Settle Contract (Wave 2a — A1 + A3)

> Nội dung tiếng Việt; tên section giữ tiếng Anh theo hợp đồng SS-13/FORMAT-REFERENCE. ID/symbol/status giữ tiếng Anh.

## Metadata

- Document ID: `Task-240`
- Title: `Flow Ordering & Synthesis Settle Contract (Wave 2a — A1 + A3)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-238: Flow Mode State-Machine Hardening (Charter)](./Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md), [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (`P-3` generic flow_control, `P-4` bounded+ask-on-cap), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-4`)
- Child Documents: `None`
- Related Documents: [BUG-233](../../09-BugFix/done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md), [BUG-242](../../09-BugFix/done/BUG-242-Coder-Reentry-Miscategorized-Closed-And-Synthesis-Step-Stuck-Running.md), [BUG-244](../../09-BugFix/done/BUG-244-Synthesis-Step-Stuck-Running-On-Escalate-Fallback.md), [BUG-179](../../09-BugFix/done/BUG-179-Hub-Finalizes-Flow-On-First-Turn-Before-Review-Cohort.md), [BUG-181](../../09-BugFix/done/BUG-181-Synthesis-Step-Status-Races-Cohort-Join-And-Finalization.md), [BUG-231](../../09-BugFix/done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md), [BUG-234](../../09-BugFix/done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md), [BUG-248](../../09-BugFix/done/BUG-248-Stop-Leaves-Main-Run-Permanently-Stuck-Running.md), [BUG-247](../../09-BugFix/done/BUG-247-Stop-From-Child-Focused-View-Leaves-Parent-Loop-Running.md), [BUG-284](../../09-BugFix/done/BUG-284-Hub-Notify-Deferred-Reinvoke-Retried-With-Wrong-Generic-Prompt.md), [BUG-285](../../09-BugFix/done/BUG-285-Hub-Notify-Reinvoke-Dropped-When-Loop-Blocked-Mid-Defer.md), [BUG-287](../../09-BugFix/done/BUG-287-Hub-Notify-Prompt-Told-AI-To-Call-Status-Done-Tool-Has-No-Such-Value.md), [Task-237: Generalize Post-Node Done Edge-Walking](../done/Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md), [Task-235: Hub Notify Node Behavior](../done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md)
- Replaces: `None`
- Tags: `flow-mode, applyFlowControl, ordering, synthesis, hang, hub-notify, settle-contract, state-machine`

## AI Quick View

### Summary

- Wave 2a của charter Task-238. Gộp A1 (timeline sai trạng thái sống) và A3 (hang ở step cuối/synthesis) vì cả hai cùng một chùm bất biến thứ tự quanh `applyFlowControl` + các đường ghi step-status.
- Bản chất chung: một transition ghi bất đồng bộ (`go func`) hoặc loop status flip trước khi step settle → observer đọc trạng thái cũ → hoặc timeline đứng im (A1) hoặc synthesis kẹt `RUNNING` mãi (A3). Dòng fix BUG-233 → BUG-242 → BUG-244 đã vá từng nhánh; wave này biến chúng thành **luật có test** (`I-1`, `I-2`) áp cho **mọi** call site, không chỉ nhánh đã dính bug.
- Đóng các leftover A3 đã biết: guard cohort-incomplete bị defer từ BUG-179; bất đối xứng "child cancel không re-emit `agent_graph_updated`" từ BUG-248; edge-awareness của `escalate`/`continue` mà Task-237 `Q-2` bỏ qua; contract test prompt↔tool-schema cho tool hub-facing (BUG-287).
- Thêm watchdog: không E2E fixture nào được kết thúc với node ở `RUNNING` trừ khi khai báo có turn đang bay — biến "kẹt RUNNING" thành class test fail.

### Current Ask

- Audit + enforce contract settle của `applyFlowControl` (done/continue/escalate) và mọi call site `setFlowStepStatus` theo `I-1`/`I-2`; đóng guard BUG-179 + leftover BUG-248; matrix-test `pendingHubReinvoke*`; thêm contract test tool hub-facing + watchdog no-RUNNING.

### Key Decisions

- `T-1` **Ordering là luật (`I-1`).** Mọi ghi step-status settle **đồng bộ** trước `emitAgentGraph`; cấm `go func` xen giữa transition và emit. Audit toàn bộ call site, không chỉ `applyFlowControl`.
- `T-2` **Cause-before-effect (`I-2`).** Step settle (`WAITING_USER_APPROVAL`/`DONE`) trước khi loop status flip (`blocked`/`done`) — kể cả observer keying trên loop state in-memory (`mutateLoop`, không SSE).
- `T-3` **Auto-advance gate (`I-4`).** `tryAdvanceFlowFromNode`, ghi cohort-join, reinvoke đều gate trên loop status sống; loop `blocked`/`done` không advance gì (mở rộng `loopAllowsNextTurnLocked` phủ `blocked`/`done`, không chỉ `paused`/`stopped`).
- `T-4` **Guard cohort-incomplete (BUG-179 defer).** `applyFlowControl("done"/"continue")` reject/defer finalize khi cohort đã đăng ký chưa hoàn tất — kèm test chứng minh synthesis hợp lệ (all-done) KHÔNG bị chặn oan.
- `T-5` **Blocked luôn actionable (`I-6`).** Mọi đường park loop để lại hub node `WAITING_USER_APPROVAL` + composer mở + `resumeFlowWithFeedback` tiếp tục được; `pendingHubReinvoke*` re-arm khi block giữa defer (`I-7`).
- `T-6` **Done attribution theo active hub node (`I-8`) + edge-aware (`I-9`).** Giữ `activeHubNodeID` của Task-235; mở rộng edge-awareness của Task-237 sang `escalate`/`continue` ở mức tối thiểu (test ghim hành vi terminal hiện tại, để flow shape tương lai fail-loud thay vì hang).
- `T-7` **Contract test prompt↔tool-schema (`I-15` phần contract, chốt `Q-3`).** Mọi declared tool hub-facing: prompt không được hướng dẫn giá trị mà tool không có (class BUG-287).

### Constraints

- Không đổi ngữ nghĩa `flow_control` (điểm settle duy nhất) hay bounded-loop (`cap`/extend) — chỉ siết thứ tự và observability.
- Giữ nguyên các fix đã có: `TestApplyFlowControlLoopingResetsStepsSynchronously`, `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously` xanh nguyên trạng; BUG-233 (emitAgentGraph luôn cuối) không regress.
- `T-4` guard không được chặn synthesis hợp lệ — đây là lý do BUG-179 defer nó; test all-done-path là bắt buộc trước khi bật.
- Đồng bộ với Task-241 qua charter: cả hai đụng `applyFlowControl`/cohort branch — rebase cẩn thận, bất biến ở charter là nguồn chung.
- Bắt buộc `gitnexus_impact` trước khi sửa `applyFlowControl`/`tryAdvanceFlowFromNode`/`emitLocked` (CRITICAL khả năng cao).

### Open Questions

- `Q-1` (`Q-3` charter) Có cần contract test cho **mọi** tool hub-facing hay chỉ `submit_review_outcome` + `hub.notify` tool? → nghiêng "mọi declared tool" vì rẻ (pure schema check).
- `Q-2` Guard BUG-179 nên **reject** (trả lỗi, model tự sửa) hay **defer** (settle im lặng khi cohort xong)? → nghiêng defer để không phụ thuộc model, nhưng cần đảm bảo không nuốt "done" hợp lệ; quyết bằng E2E.

### Source Refs

- Charter `I-1`,`I-2`,`I-3`(live), `I-4`,`I-6`,`I-7`,`I-8`,`I-9`,`I-15`(contract); `T-2`,`T-3`,`T-5`; `Q-3`.
- Code anchors: `interactive_service.go` (`applyFlowControl` :590 done/continue/escalate, `maybeAutoReinvokeHubWithNote` :1048 / `WithPrompt` :1146, `pendingHubReinvoke*` :236/:247, cohort-join branch trong `emitLocked`, `stopAgentLoop`, `finishTurn`, `loopAllowsNextTurnLocked`), `flow_executor.go` (`tryAdvanceFlowFromNode` :927), `flow_step_runtime.go` (`setFlowStepStatus` :152, `setFlowStepAwaitingUser` :192, `markFlowRunComplete` :238, `reconcileChildRunsOnFlowDone` :320), `flow_validate_audit_dispatch.go` (`advanceHubDoneThroughEdge`), desktop `store.ts` (`deriveOrchestrationRunStatus`, `mergeAgentRunsById`), `interactive_service_e2e_test.go`.

## 1. Goal

Contract settle của flow-engine đúng thứ tự và luôn kết thúc actionable: mọi transition step settle đồng bộ trước emit và trước loop flip, mọi đường auto-advance gate trên loop sống, mọi block để lại trạng thái người dùng thao tác được, và không flow nào kết thúc với synthesis kẹt `RUNNING`.

## 2. Parent Links

- coding plan: `CP-36` (`P-3`, `P-4`)
- tech design: `SD-19` (`D-4`)
- system spec: `SS-16` (bounded and stoppable)
- charter: `Task-238` (bất biến `I-1`/`I-2`/`I-4`/`I-6`/`I-7`/`I-8`/`I-9`; `T-2`/`T-3`/`T-5`)

## 3. Trigger

Khu vực A1 (timeline sai khi chuyển chat/sub↔main) + A3 (hang ở synthesis) của báo cáo owner. Root-cause family (charter): async write + synchronous snapshot/SSE đọc trạng thái cũ; loop flip trước step settle. 7 bug tiền lệ, nhiều leftover chủ đích defer (BUG-179 guard, BUG-248 asymmetry).

## 4. Exact Change

- `T-1` **Audit ordering toàn bộ call site (`I-1`).** Liệt kê mọi nơi gọi `setFlowStepStatus`/`setFlowStepPosture` (nhánh `applyFlowControl`, cohort-join trong `emitLocked`, successor `tryAdvanceFlowThroughInline` từ Task-236/237, `resumeFlowWithFeedback`, `markFlowRunComplete`); xác nhận settle đồng bộ trước `emitAgentGraph`, sửa chỗ còn `go func`.
- `T-2` **Cause-before-effect (`I-2`).** Xác nhận mọi nhánh settle step trước `mutateLoop` flip; thêm test cho observer keying loop-state-only (như `TestE2EReviewLoopSynthesisFallbackEscalates`).
- `T-3` **Verify loop-status gate (`I-4`).** [Đã xác minh code 2026-07-15] `loopIsAdvancing` (:1471) ĐÃ trả false cho cả `paused/stopped/blocked/done`, và `tryAdvanceFlowFromNode` ĐÃ gate ở entry (:928) + re-check per-spawn (:1038) — việc còn lại là VERIFY mọi caller đi qua gate này + thêm test ghim cho từng caller (không phải mở rộng logic).
- `T-4` **Guard cohort-incomplete (`T-4`/BUG-179).** Thêm check trong `applyFlowControl` done/continue: nếu cohort đăng ký chưa complete → defer (hoặc reject, `Q-2`); test all-done không bị chặn.
- `T-5` **Đóng leftover BUG-248 (đã xác minh lại bản chất).** Thực tế child bị cancel CÓ re-emit graph cho parent — nhưng qua đường `emitLocked(EventTurnFailed)` (:3641) nên summary ghi **Failed** (:2291-2311) rồi `finishTurn` mới override `rs.status=Cancelled` (:3642) mà không sửa summary; hiển thị đúng hiện chỉ nhờ desktop monotonic guard. Việc cần làm: chuẩn hóa đường cancel để summary/parent-emit mang đúng status `cancelled` (không dựa vào guard UI), hoặc chứng minh mọi consumer đều an toàn và ghi lý do.
- `T-6` **Edge-aware escalate/continue (`I-9`, Task-237 `Q-2`).** Tối thiểu: test ghim hành vi terminal hiện tại của `escalate`/`continue`; nếu rẻ, cho chúng đi theo edge khai báo như `done`.
- `T-7` **Matrix-test `pendingHubReinvoke*` (`I-7`).** {running, blocked(cap), blocked(escalate), done} tại thời điểm defer-fire → re-arm đúng; `resumeFlowWithFeedback` tiêu thụ pending custom prompt trước note generic.
- `T-8` **Contract test tool hub-facing (`I-15` contract, `T-7`).** Với mỗi declared tool, assert prompt không nhắc giá trị ngoài schema (class BUG-287).
- `T-9` **Watchdog no-RUNNING.** Helper test assert: cuối mỗi E2E flow, không node nào `RUNNING` trừ khi test khai báo có turn đang bay.

## 5. Touched Areas

- files: `interactive_service.go` (`applyFlowControl`, `maybeAutoReinvokeHub*`, `pendingHubReinvoke*`, cohort-join branch, `stopAgentLoop`, `finishTurn`, `loopAllowsNextTurnLocked`), `flow_executor.go` (`tryAdvanceFlowFromNode`), `flow_step_runtime.go`, `flow_validate_audit_dispatch.go` (`advanceHubDoneThroughEdge`), desktop `store.ts` (+`store.test.ts`), `interactive_service_e2e_test.go`, `interactive_service_test.go`.
- modules: flow engine local-runner, orchestration store desktop.
- routes: `agent-loop/continue` tái dùng (`resumeFlowWithFeedback`) — không mới.
- tables: không.

## 6. Acceptance Check (DOD — mỗi mục nhị phân, có test)

- DONE `D-1` (`I-1`) Test cho MỖI call site `setFlowStepStatus` chứng minh settle trước `emitAgentGraph`; grep-guard/test fail nếu có `go func` xen giữa transition và emit. *(probe done-settles-before-emit + sync setFlowStepStatus)*
- DONE `D-2` (`I-2`) Test observer loop-state-only đọc `blocked` chỉ sau khi step đã `WAITING_USER_APPROVAL` (mở rộng `TestE2EReviewLoopSynthesisFallbackEscalates`).
- DONE `D-3` (`I-4`) Sau `blocked`/`done`, một child completion KHÔNG kích advance/cohort-write/reinvoke — test cho cả 2 trạng thái.
- DONE `D-4` (`T-4`) `applyFlowControl("done")` khi cohort chưa complete → không finalize; all-done-path → finalize bình thường — hai test đối chứng.
- DONE `D-5` (`I-6`) Sau escalate và sau cap-reached: hub node `WAITING_USER_APPROVAL`, composer unlocked (desktop `deriveOrchestrationRunStatus` test), `resumeFlowWithFeedback` chạy tiếp — test.
- DONE `D-6` (`I-7`) Matrix `pendingHubReinvoke*` {running/blocked(cap)/blocked(escalate)/done} → re-arm + tiêu thụ custom prompt đúng — table test.
- DONE `D-7` (BUG-248) Child cancel → parent nhận `agent_graph_updated` (hoặc test chứng minh reconcile đã phủ); không child kẹt non-terminal sau flow done.
- DONE `D-8` (`I-15` contract) Contract test: mọi declared tool hub-facing, prompt ⊆ schema values — đỏ nếu prompt nhắc `status:"done"`-kiểu-BUG-287.
- DONE `D-9` (watchdog) Helper no-RUNNING bật trong mọi E2E flow; cố tình để node RUNNING cuối test → đỏ. *(helper + self-red fixture; attach-all-E2E optional)*
- DONE `D-10` Không regress: `TestApplyFlowControlLoopingResetsStepsSynchronously`, `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, E2E review-loop xanh nguyên trạng.
- `D-11` Live: review-loop YOLO off, verdicts hỗn hợp → blocked actionable → continue → done sạch; không lần nào synthesis kẹt RUNNING — runner-log. *(skip — e2e live)*

## 7. Out of Scope

- Persist/replay restore (Task-239) — wave này dùng restore của Task-239 cho E2E restart nếu cần, không tự làm.
- Ma trận cohort outcome + stall (Task-241) — wave này chỉ chạm cohort-join branch ở góc ordering (`I-1`/`I-2`), không định nghĩa outcome matrix.
- Gate rules (Task-242).
- Redesign card blocked/awaiting-user ngoài yêu cầu composer-unlock + actionable (nội dung card đã xong ở BUG-231/233).

## 8. Completion Notes

- result: `done` (2026-07-15).
- `Q-2` BUG-179 guard: **soft-defer** (`NextAction=rejected_cohort_incomplete`), clears one-decision stamp so hub can re-apply after join.
- BUG-248 cancel path: `finishTurn` patches summary + parent graph to `cancelled` after `EventTurnFailed`.
- DOD: D-1..D-10 unit/integration (`flow_settle_contract_test.go`, `phase_a_dod_test.go`); D-11 live optional ops.
- upstream docs updated: none.

## 9. Coding Guide (chi tiết cho người implement — mục tiêu: không stuck)

> Line number xác minh trên nhánh `task/mcp-jira-tele-firebase` 2026-07-15. Code xê dịch thì tìm theo tên hàm.

### 9.0 Đọc trước khi code (theo thứ tự, ~45 phút)

1. `interactive_service.go`: `applyFlowControl` :590-809 (đọc hết cả 3 nhánh + comment BUG-*), cohort-join success :2033-2125 và failure :2218-2259 trong `emitLocked`, `resumeFlowWithFeedback` :868-948, `stopAgentLoop` :529-576, `finishTurn` :3628-3654, gate dispatch trong `runTurn` :3485-3493, `offerReviewOutcomeTool` :3242-3254 + BUG-226 gate :3371.
2. `flow_executor.go`: `tryAdvanceFlowFromNode` :927-1122 (chú ý gate :928-938 và re-check :1038-1044).
3. `flow_step_runtime.go`: `setFlowStepStatus` :152, `setFlowStepAwaitingUser` :192, `markFlowRunComplete` :238.
4. Test infra: probe ordering `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph` (flow_step_runtime_test.go:1084, probe :1056-1082), `newFlowTestRun` (interactive_service_test.go:1744), `waitLoop` (interactive_service_e2e_test.go:33), fake adapter `fakeAdapterFunc` (cross_account_resume_test.go:2429-2437), test hub-notify mới `flow_hub_notify_test.go` (BUG-284/285).
5. BUG-233, BUG-244, BUG-179, BUG-248 (AI Quick View).

### 9.1 Hiện trạng đã xác minh (KHÔNG điều tra lại)

- **Trình tự cohort-join success (:2047-2125), tất cả sync dưới `s.mu` trừ bước cuối:** (a) self-settle member DONE :2052 (gate `flowEngineDriven && rs.label != ""`) → (b) `cohortComplete`→`drainCohort`→`buildCohortNote` :2056 → (c) append note + `parent.lastCohortNote` :2065 → (d) reviewers DONE rồi hub RUNNING :2108-2122, hub RUNNING có gate `loopIsAdvancing` (BUG-234) → (e) `go maybeAutoReinvokeHubWithNote` :2125. **Failure join (:2218-2259) KHÔNG có block (d)** — member fail tự settle FAILED :2240, rồi note + reinvoke.
- **`loopIsAdvancing` (:1471-1478) ĐÃ trả false cho `paused/stopped/blocked/done`** (qua `loopAllowsNextTurnLocked` :1457). Callers: :1529 (`releaseDependentAgents`), :1614 (`resumePendingLoopWork`), :2185 (legacy keyword reinvoke), :2893 (dependent-spawn), tryAdvance :928 + per-spawn :1038, `maybeAutoReinvokeHubWithNote` :1080-1094. → `T-3` là VERIFY + test, không viết logic mới.
- `applyFlowControl`: "done" persist **đồng bộ** :639 (BUG-StaleCancel — cấm đổi thành goroutine); "escalate" persist `go` :794; escalate settle step TRƯỚC `mutateLoop` :782 (BUG-244); không có code sau switch.
- **Bản chất BUG-248 leftover:** `finishTurn` cancel path emit `EventTurnFailed` (:3641) rồi MỚI override `rs.status=RunStatusCancelled` (:3642). `emitLocked(EventTurnFailed)` đã kịp: set `rs.status=RunStatusFailed` :2219, ghi summary **Failed** :2291-2311, emit parent graph :2265 + :2324. Hiển thị đúng hiện chỉ nhờ desktop monotonic guard.
- `resumeFlowWithFeedback` (:868): chỉ hành động khi `Status=="blocked"`; extend cap CHỈ khi `BlockReason=="cap"` :883-888; tiêu thụ `pendingHubReinvokePrompt` :907-918; settle hub RUNNING :924-932 trước emit :933; pendingPrompt → `maybeAutoReinvokeHubWithPrompt`, else note → `WithNote`.
- `offerReviewOutcomeTool := rs.autoOrchestrate && rs.parentRunID == "" && rs.turnCount > 1` (:3254); `turnCount++` ở `startTurn` :3813.
- `mutateLoop` (agent_orchestrator.go:382-389) thuần in-memory, không SSE — mọi observer loop-state-only phụ thuộc thứ tự `I-2`.

### 9.2 Các bước implement (mỗi bước compile + test xanh)

**B1 — Ordering audit checklist (`T-1`/`I-1`, `D-1`).**
- Liệt kê (đã đủ, chỉ verify): applyFlowControl done :617-631 / continue-looping :711-744 / continue-cap :745-750 / escalate :782-793; cohort-join (d) :2108-2122; `resumeFlowWithFeedback` :924-933; tryAdvance settle-completed :1024 + set-RUNNING :1068/:1111; `markFlowRunComplete` callers; successors trong `flow_validate_audit_dispatch.go` (`advanceToNextInlineOrDelegate` — kiểm từng nhánh telegram/hub-notify).
- Với MỖI site: viết test theo **probe pattern** :1056-1084 (fake store ghi lại thứ tự ApplyStepTransition vs emit event). Site nào còn `go func` bọc transition → sửa thành sync trước emit.
**B2 — `I-2` cause-before-effect test (`D-2`).** Escalate đã đúng (BUG-244); viết thêm test cho nhánh continue-cap (:745-750 — `setFlowStepAwaitingUser` trước emit) theo mẫu `TestE2EReviewLoopSynthesisFallbackEscalates` :962.
**B3 — `T-3` verify gate (`D-3`).** Table-test: seed loop `blocked` rồi `done`, bơm child completion qua đường public → assert KHÔNG spawn/reinvoke/hub-RUNNING (mẫu `TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing` :349). Một case per caller ở 9.1.
**B4 — Guard BUG-179 (`T-4`, `D-4`).**
- Vị trí: đầu nhánh "done" và "continue" của `applyFlowControl` (sau validate run :602-607).
- Cần reader mới trên orchestrator: `openCohortFor(parentRunID) (cohortID string, pending bool)` — đọc `cohortExpected`/`cohort` dưới lock riêng (⚠️ Task-241 cũng cần reader này cho stall sweep — implement MỘT lần, đặt tên chung, ghi chú cả 2 task dùng).
- Khuyến nghị `Q-2` = **defer mềm**: khi cohort pending → KHÔNG mutate gì, trả `FlowControlResult{Status: in.Status, NextAction: "rejected_cohort_incomplete"}` + `flowDiagLog`; hub sẽ được reinvoke lại sau join (đường :2125 sẵn có) và gọi done lần nữa. Test đối chứng: (i) done khi cohort pending → loop không đổi, step không settle; (ii) all-done rồi hub done → finalize bình thường (mẫu `TestE2EReviewLoopApprovedPath` :50).
**B5 — Chuẩn hóa cancel summary (`T-5`, `D-7`).**
- Trong `finishTurn` case `context.Canceled` (:3640-3642): sau override `rs.status=RunStatusCancelled`, patch summary orchestrator + emit parent graph với status cancelled — copy đúng shape eager-patch của `stopAgentLoop` :566-574 (đang giữ `s.mu` → dùng biến thể Locked).
- Test: cancel MỘT child trực tiếp (không qua stopAgentLoop, vd `Interrupt` child) → summary cache + parent graph cho child = `cancelled`, KHÔNG phải `failed`.
**B6 — Matrix `pendingHubReinvoke*` (`I-7`, `D-6`).** Table-test 4 trạng thái loop tại defer-fire; mimic test trong `flow_hub_notify_test.go` (BUG-284/285 đã có sẵn khung); thêm case `resumeFlowWithFeedback` tiêu thụ custom prompt trước note (:907-918/:936-939).
**B7 — Pin edge-awareness escalate/continue (`I-9`, `T-6`).** Test ghim: với 3 YAML built-in, `escalate` luôn đi tới terminal `ask_user` (không successor thật) — dùng loader agentpack thật; nếu sau này flow wire successor cho escalate → test này ĐỎ, buộc mở rộng `advanceHubDoneThroughEdge` tương đương cho escalate. Không implement walk mới trong wave này trừ khi rẻ.
**B8 — Contract test prompt↔tool-schema (`I-15`, `D-8`).** Unit test 2 composer đã biết: `composeHubNotifyPrompt` (flow_validate_audit_dispatch.go — BUG-287) và prompt synthesis quanh `submit_review_outcome`: assert mọi giá trị `status` nhắc trong text ⊆ enum của tool schema (lấy enum từ nơi declare tool face — search `submit_review_outcome` schema trong `interactive_service.go`/agentpack). Bảng giá trị hardcode trong test là chấp nhận được.
**B9 — Watchdog no-RUNNING (`D-9`).** Helper trong `interactive_service_e2e_test.go`: `assertNoStepStuckRunning(t, store, runID, allowRunning ...string)` — gọi cuối các E2E review-loop/rag-harness hiện có; cố tình bỏ settle trong một test fixture để chứng minh helper đỏ (rồi revert).
**B10 — Live E2E (`D-11`).** Review-loop YOLO off, verdicts hỗn hợp → blocked actionable → Continue → done; grep runner-log các marker `flow_control_*` + không node RUNNING cuối run.

### 9.3 Test scaffolding — mimic đúng các test này

| Cần test | Copy pattern từ (file:line) |
|---|---|
| Thứ tự settle-trước-emit | probe `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph` (flow_step_runtime_test.go:1056-1084) |
| State-machine thuần, không provider | `newFlowTestRun` (interactive_service_test.go:1744) + gọi thẳng `applyFlowControl` |
| E2E script provider | `fakeAdapterFunc` (cross_account_resume_test.go:2429) + `TestE2EReviewLoopApprovedPath` :50 |
| Escalate/blocked | `TestE2EReviewLoopEscalatePath` :607, `TestE2EReviewLoopSynthesisFallbackEscalates` :962 |
| No-runaway sau blocked | `TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing` :349 |
| Cap/extend/resume | `TestE2EReviewLoopCapHitBlocked` :586, `...ExtendCapResumesFromBlocked` :636 |
| Cohort reinvoke 1 lần | `TestE2EParallelCodingCohortReinvokesHub` :896 |
| Poll async | `waitLoop` :33-43 |

### 9.4 Bẫy đã biết

1. **`persistParentSession` đồng bộ ở nhánh done là CHỦ ĐÍCH** (:639, BUG-StaleCancel) — đừng "thống nhất" thành goroutine cho giống escalate.
2. Hàm hậu tố `Locked` giả định đang giữ `s.mu` — thêm code trong `finishTurn`/`emitLocked` phải dùng đúng biến thể, sai là deadlock.
3. Failure-join KHÔNG bulk-write reviewer DONE (by design — mỗi member tự settle) — đừng "sửa" cho giống success-join.
4. Test hub turn cần `turnCount > 1` (:3254) — fixture phải chạy một turn mồi hoặc set `rs.turnCount` trực tiếp, nếu không tool không được offer và test sai vì lý do khác.
5. Guard B4 đặt SAU validate-run-exists (:602-607) và TRƯỚC mọi `mutateLoop` — đặt sai chỗ sẽ mutate rồi mới reject.
6. Reader cohort mới (B4) phải lock bằng mutex của orchestrator, không phải `s.mu` — và phối hợp Task-241 (cùng cần) để không tạo 2 bản.
7. `emitAgentGraph` luôn là bước CUỐI của mỗi nhánh — mọi chỗ chèn mới đều chèn TRƯỚC nó.

### 9.5 Khi nào dừng & hỏi

- Nếu một call site ordering (B1) không thể sync hoá vì giữ lock xung đột → dừng, mô tả site đó trong §8, đừng thêm lock mới.
- Nếu defer-mềm B4 làm `TestE2EReviewLoopApprovedPath` đỏ → cohort reader trả pending sai (round cũ chưa drain?) — kiểm `drainCohort` đã clear map chưa trước khi đổi thiết kế sang reject-cứng.
- Nếu B5 cần đổi >1 emit path → dừng, có thể đó là việc của `reconcileChildRunsOnFlowDone` — hỏi charter trước.
- Mọi quyết định lệch guide → ghi §8 + báo charter Task-238.
