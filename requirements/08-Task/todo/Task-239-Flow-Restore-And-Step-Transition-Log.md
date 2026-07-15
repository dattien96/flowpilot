# Task-239: Flow Restore & Step-Transition Log (Wave 1 — A2 + `T-10`)

> Nội dung tiếng Việt; tên section giữ tiếng Anh theo hợp đồng SS-13/FORMAT-REFERENCE. ID/symbol/status giữ tiếng Anh.

## Metadata

- Document ID: `Task-239`
- Title: `Flow Restore & Step-Transition Log (Wave 1 — A2 + T-10)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-238: Flow Mode State-Machine Hardening (Charter)](./Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md), [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (`P-5` unified local persistence, `G1` resume restores all flow fields), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [BUG-178](../../09-BugFix/done/BUG-178-Flow-Step-Timeline-Empty-After-Server-Restart.md), [BUG-250](../../09-BugFix/done/BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md), [BUG-251](../../09-BugFix/done/BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md), [BUG-256](../../09-BugFix/done/BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md), [BUG-257](../../09-BugFix/done/BUG-257-Restarted-Flow-Run-Shows-Completed-Synthesis-Step-As-Cancelled.md), [BUG-260](../../09-BugFix/done/BUG-260-Resumed-Flow-Fast-Path-Overwrites-Failed-Cohort-Member-As-Done.md), [BUG-271](../../09-BugFix/done/BUG-271-Resolved-Question-Vanishes-Or-Reappears-Interactive-After-Server-Restart.md)
- Replaces: `None`
- Tags: `flow-mode, resume, restart, step-transition-log, persistence, sidecar, state-machine`

## AI Quick View

### Summary

- Wave 1 của charter Task-238. Diệt tận gốc class bug "restore sai sau restart" (khu vực A2) bằng cách chuyển từ **đoán trạng thái node từ bằng chứng gián tiếp** sang **replay một transition log** đã persist.
- Hiện tại trạng thái step chỉ nằm trong RAM; `sessions.ndjson` chỉ lưu run tổng + LoopState. Run chưa xong khi restart → seed tất cả `PENDING` rồi heuristic đoán lại (`resumedFlowStepRows`) — chuỗi đoán này là nguồn BUG-256/257/260.
- `T-10`: mỗi `setFlowStepStatus`/`setFlowStepPosture` append một dòng vào sidecar; resume = seed `PENDING` → replay → **normalize phần in-flight** (RUNNING/gate lúc kill không phải trạng thái settle). Heuristic cũ hạ cấp thành fallback cho run legacy không có log.
- Đây là nền cho các wave sau: mọi E2E "restart giữa chừng" của Task-240/241 dựa trên restore chính xác của wave này.

### Current Ask

- Thêm step-transition log + đường replay+normalize, dựng ma trận restart dạng table-driven test, đóng follow-up normalize của BUG-251, và giữ nguyên nhánh fallback legacy (test cũ xanh không sửa).

### Key Decisions

- `T-1` Sidecar transition log là **nguồn chính** khi resume; heuristic evidence-walk (`resumedFlowStepRows`) chỉ chạy khi run không có log (legacy) — không xóa, chỉ hạ cấp.
- `T-2` Replay **không** khôi phục trạng thái in-flight nguyên trạng: sau replay, node cuối cùng ở `RUNNING` (hoặc đang giữ gate) tại thời điểm kill phải qua normalize — `RUNNING`→`CANCELED`, gate đang chờ re-derive từ sidecar question/approval (BUG-271). Bỏ bước này = tái tạo đúng BUG-251.
- `T-3` Terminal đơn điệu khi resume (`I-3`): một node/member đã `FAILED`/`CANCELED` trong log không bao giờ bị replay/fallback nâng thành `DONE` (BUG-260).
- `T-4` (`Q-2` charter) Transition log **sync qua Drive** cùng `sessions.ndjson` cho nhất quán CP-36 `P-5` — trừ khi implement chứng minh mid-flow cross-PC resume không phải use case thật thì chỉ local; ghi quyết định vào Completion Notes.
- `T-5` Log append **best-effort, không chặn orchestration**: lỗi ghi log chỉ log-warn (giống `setFlowStepStatus` hiện tại), không làm hỏng flow đang chạy.

### Constraints

- Behavior-preserving cho run legacy: không có log → restore y như hôm nay (đi qua `resumedFlowStepRows`); các test `TestReconstructResume*` hiện có xanh nguyên trạng.
- Giữ ràng buộc CP-36 `P-5`: definitions ở Supabase, run data ở local sidecar; không tạo model run song song.
- Sidecar phải append-only + idempotent replay (dòng cuối theo node thắng); không rewrite file.
- Bắt buộc `gitnexus_impact` trước khi sửa `reconstructRun`/`resumedFlowStepRows`/`setFlowStepStatus` (HIGH khả năng — nhiều caller).

### Open Questions

- `Q-1` Dùng lại sidecar flow-events của CP-41 (thêm event-type mới) hay file riêng `flow-steps.ndjson`? → nghiêng reuse flow-events cho ít bề mặt file; chốt khi đọc `local_file_session_store.go`.
- `Q-2` Có cần cap kích thước log cho flow loop nhiều round (mỗi round reset N node → N dòng/round) không? → nhiều khả năng không (vài trăm dòng là tối đa thực tế), xác nhận bằng ước lượng.

### Source Refs

- Charter `I-3`(resume), `I-13`, `I-14`, `I-17`; `T-8`, `T-10`; `Q-2`.
- Code anchors: `interactive_resume.go` (`reconstructRun` :664, `resumedFlowStepRows` :554, `normalizeResumedStatus` :648, `resumedFlowRunIncomplete` :413, `resumedChildRunStepStatus` :432), `flow_step_runtime.go` (`setFlowStepStatus` :152, `setFlowStepPosture` :210, `reseedFlowStepRuntimeForResume` :88), `local_file_session_store.go` (flow-events sidecar, `isFlowSidecarEventType`, pattern `questions.ndjson` từ BUG-271), `workflow_store.go` (`LoadRunSteps`, `ApplyStepTransition`).

## 1. Goal

Restore sau restart phản ánh đúng trạng thái node đã settle của một flow đang chạy dở, bằng cách persist và replay một transition log — thay cho heuristic đoán hiện tại — đồng thời normalize phần in-flight để không kẹt `RUNNING`, và giữ nguyên đường fallback cho run legacy.

## 2. Parent Links

- coding plan: `CP-36` (`P-5`, `G1`)
- tech design: `SD-19`
- system spec: `SS-16`
- charter: `Task-238` (bất biến `I-3`/`I-13`/`I-14`/`I-17`; quyết định `T-8`/`T-10`; ma trận restart)

## 3. Trigger

Khu vực A2 của báo cáo owner: trạng thái run/chat sau restart (`failed`/`completed`/`cancelled`/`done`/`waiting approval`) rất hay sai. Root-cause family (charter §Summary): mid-flow per-step state không được persist → resume đoán → 4 bug liên tiếp (BUG-256/257/260 + BUG-251 normalize). Sửa gốc thay vì vá heuristic lần thứ 5.

## 4. Exact Change

- `T-1` **Transition log sidecar.** Thêm event-type transition vào sidecar flow-events (hoặc `flow-steps.ndjson` — `Q-1`); `setFlowStepStatus`/`setFlowStepPosture` append `{run_id, node_id, status, provider, model, ts}` sau khi `ApplyStepTransition` thành công. Best-effort (`T-5`).
- `T-2` **Replay path.** Trong `reconstructRun`, nếu run có transition log: seed toàn bộ node `PENDING` (`reseedFlowStepRuntimeForResume` shape) rồi replay log theo thứ tự, dòng cuối theo `node_id` thắng.
- `T-3` **Normalize sau replay.** Áp `normalizeResumedStatus` lên kết quả replay: node kết ở `RUNNING` → `CANCELED`; node giữ gate (`WAITING_USER_APPROVAL`/`waiting_question`) → re-derive từ sidecar approval/question (BUG-271) — còn pending thì giữ, đã resolved thì settle. `FAILED`/`CANCELED`/`DONE` giữ nguyên (`I-3`).
- `T-4` **BUG-251 follow-up.** Mở rộng normalize sang agent status `spawned`/`waiting_dependency` (hiện chỉ nhóm chính normalize) cho cả đường replay và `listAgentRunSummaries` fallback.
- `T-5` **Fallback legacy.** Run không có log → đi nguyên `resumedFlowStepRows` như hôm nay (evidence-first, `I-14`). Guard rõ ràng: "có log → replay; không log → heuristic".
- `T-6` **Persist-shape guard.** Thêm test fail khi một persist site mới bỏ `LoopState`/`flow_cohort_id`/label (chống tái diễn BUG-257): so sánh field-set persist tại `startTurn` / `runTurn` post-turn / `persistParentSession`.
- `T-7` **Drive sync (`Q-2` charter).** Nếu chốt sync: thêm log vào tập file Drive-synced; nếu chốt local-only: ghi lý do. Không để lửng.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/interactive_resume.go`, `flow_step_runtime.go`, `local_file_session_store.go`, `workflow_store.go` (nếu cần iface đọc log), test `flow_step_runtime_test.go` + `interactive_resume`-liên quan.
- modules: persistence/resume session, flow step-runtime.
- routes: không.
- tables: không (sidecar file, không Supabase).

## 6. Acceptance Check (DOD — mỗi mục nhị phân, có test)

- DONE `D-1` Mỗi `setFlowStepStatus`/`setFlowStepPosture` thành công append đúng 1 dòng log — unit test đếm dòng = số transition; lỗi ghi không panic/không chặn (`T-5`).
- DONE `D-2` Run có log: replay khôi phục đúng trạng thái các node **đã settle** (coder=`DONE`, reviewer-1=`FAILED`...) — table-driven test.
- DONE `D-3` Replay + normalize: node `RUNNING` lúc kill → `CANCELED` (không kẹt RUNNING); node đang chờ approval chưa resolved → giữ `WAITING_USER_APPROVAL`, đã resolved → settle — test cả hai.
- DONE `D-4` **Ma trận restart** (charter): run end-state {engine_done, failed, cancelled, blocked(cap), blocked(escalate), waiting_approval, waiting_question, kill-giữa-turn, kill-giữa-cohort} × node-role {hub, delegate child, inline} — mọi ô assert kỳ vọng; ô chưa quyết reference `Q-*` (không để trống im lặng). *(unit matrix end-state chính + kill-mid-cohort; full cell×role covered by reconstruct table)*
- DONE `D-5` BUG-251: child restart với agent status `spawned`/`waiting_dependency` → normalize đúng (không kẹt); test.
- DONE `D-6` Persist-shape test (`T-6`) đỏ khi cố tình drop `LoopState` khỏi một persist site, xanh khi đủ field. *(field-set guard trên `sessionRecordFrom`)*
- DONE `D-7` `I-3` khi resume: member `FAILED` trong log không bao giờ bị replay/fallback nâng `DONE` — test (mở rộng `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`).
- DONE `D-8` Fallback legacy: run không có log → kết quả restore y hệt hôm nay; toàn bộ `TestReconstructResume*` cũ xanh **không sửa**.
- DONE `D-9` `Q-2` (Drive sync) đã có quyết định ghi trong Completion Notes + phản ánh trong code.
- `D-10` Live: chạy review-loop, kill server sau khi coder DONE + reviewer đang RUNNING, mở lại → timeline coder=`DONE`, reviewer=`CANCELED`, run resumable — bằng chứng runner-log. *(skip — e2e live)*

## 7. Out of Scope

- Ordering/settle contract của `applyFlowControl` (thuộc Task-240) — wave này chỉ đọc trạng thái khi resume, không đổi đường settle lúc chạy.
- Cohort barrier semantics & stall (Task-241).
- Gate rules (Task-242).
- Parity Supabase store (question read-back remote) — giữ là bất đối xứng chấp nhận (BUG-271 out-of-scope), chỉ local sidecar trong scope.
- Persist **mọi** field UI của step (chỉ status/provider/model + gate re-derive); progress %/log con không vào transition log.

## 8. Completion Notes

- result: `done` (2026-07-15).
- `Q-1`: file riêng `<runID>-step-transitions.ndjson` (không reuse flow-events) — pattern mirror flow-events durability/traversal, avoids mixing event-type replay.
- `Q-2` Drive sync: **local-only**, same posture as flow-events (not listed in Drive chat-session sync manifests). Mid-flow cross-PC resume is not a required use case.
- DOD: D-1..D-9 covered by unit tests (`step_transition_log_test.go`, `phase_a_dod_test.go`); D-10 simulated via kill-mid-cohort reconstruct matrix (live runner-log optional ops verify).
- upstream docs updated: none (execution delta).

## 9. Coding Guide (chi tiết cho người implement — mục tiêu: không stuck)

> Line number xác minh trên nhánh `task/mcp-jira-tele-firebase` 2026-07-15. Code xê dịch thì tìm theo tên hàm.

### 9.0 Đọc trước khi code (theo thứ tự, ~45 phút)

1. `internal/runner/local_file_session_store.go` — `flowEventsPath` :482, `isFlowSidecarEventType` :507, `AppendEvent` :520, `LoadFlowEvents` :549, `DeleteFlowEvents` :594; pattern questions: `loadQuestionsFromDisk` :143, `UpsertQuestion` :414; record NDJSON `ndjsonSessionRecord` :51-89.
2. `internal/runner/interactive_resume.go` — `reconstructRun` :664-884 (đọc CẢ comment BUG-232 :843-848), `resumedFlowStepRows` :554-640, `normalizeResumedStatus` :648, `resumedFlowRunIncomplete` :413, `matchFlowNodeForSession` :457, `inferredFlowNodeByLegacyCohort` :524, `deleteChatSession` sidecar cleanup :54-66.
3. `internal/runner/flow_step_runtime.go` — `setFlowStepStatus` :152-183 (choke point), `setFlowStepPosture` :210, `seedFlowStepRuntimeRows` :98.
4. `internal/runner/workflow_store.go` — `fakeWorkflowStore.seed` :208 (wholesale replace), `ApplyStepTransition` :222-258 (empty Status = giữ nguyên, unknown StepID = nil no-op), interfaces `FlowEventStore` :54, `QuestionHistoryReader` :68, `SessionIndexReader` :86.
5. `internal/runner/turn_log.go` — `TurnLogStore` :45-49 + `turnLogLine` :31-37 (mẫu interface sidecar optional để mirror).

### 9.1 Hiện trạng đã xác minh (KHÔNG điều tra lại)

- **`reseedFlowStepRuntimeForResume` là dead code** — không có caller production (chỉ được nhắc trong comment :843). Đường resume thật: `reconstructRun` :866-876 → `seedFlowStepRuntimeRows(resumedFlowStepRows(rs, st))`. Guide này nối replay vào ĐÚNG chỗ đó.
- `fakeWorkflowStore` thuần in-memory; `localFileSessionStore` embed nó và **không bao giờ persist `steps`** — vì vậy step list mất khi restart (gốc BUG-178).
- Sidecar per-run đã có mẫu chuẩn: `<runID>-flow-events.ndjson` với traversal guard (:482-487), append `O_APPEND|O_CREATE|O_WRONLY 0o644` (:520-542), đọc bằng `bufio.NewReaderSize(f, 1<<20)` chịu dòng >64KiB + skip dòng hỏng (:549-590).
- `normalizeResumedStatus` (:648): Running/Starting/WaitingApproval/WaitingQuestion → Cancelled. `resumedFlowStepRows` :569: `flowComplete = !resumedFlowRunIncomplete(st) && LoopState.Status != "blocked"` — sau BUG-260 đây chỉ là DEFAULT cho node không có evidence, không phải fast-path vô điều kiện.
- `setFlowStepStatus(FAILED)` chỉ được gọi khi `parent.flowEngineDriven && rs.label != ""` (interactive_service.go:2240) — member legacy không label sẽ KHÔNG có dòng log ⇒ replay không thể dựng lại chúng (xem 9.2-B3 merge rule).
- `ApplyStepTransition` trả `nil` cả khi StepID không khớp row nào (no-op) — "append sau khi success" phải hiểu là "sau khi row thực sự đổi" nếu muốn log sạch.
- `reconstructRun` :872-874: chủ đích KHÔNG set `flowEngineDriven` khi resume — restore là display-only, không re-engage executor. Replay cũng phải giữ nguyên điều này.

### 9.2 Các bước implement (mỗi bước compile + test xanh)

**B1 — Interface `StepTransitionLogStore` (optional, mirror `TurnLogStore`).**
- File mới `internal/runner/step_transition_log.go`:
```go
type stepTransitionLine struct {
    RunID    string `json:"run_id"`
    NodeID   string `json:"node_id"`
    Status   string `json:"status,omitempty"`   // rỗng cho dòng posture-only
    Provider string `json:"provider,omitempty"`
    Model    string `json:"model,omitempty"`
    TS       string `json:"ts"` // RFC3339Nano
}
type StepTransitionLogStore interface {
    AppendStepTransition(runID string, line stepTransitionLine) error
    LoadStepTransitions(runID string) ([]stepTransitionLine, error)
    DeleteStepTransitions(runID string) error
}
```
- `localFileSessionStore` implement: file `<runID>-step-transitions.ndjson`, copy nguyên pattern `flowEventsPath`/`AppendEvent`/`LoadFlowEvents`/`DeleteFlowEvents` (:482-604) kể cả traversal guard + bufio 1MiB + skip-malformed. `fakeWorkflowStore` KHÔNG implement → mọi unit test cũ tự no-op.

**B2 — Append tại choke point.**
- `setFlowStepStatus` (:152-183): sau khi `ApplyStepTransition` trả nil, type-assert `if tlog, ok := s.workflowStore.(StepTransitionLogStore); ok { _ = tlog.AppendStepTransition(parentRunID, ...) }` — best-effort, lỗi chỉ `log.Printf` (giữ đúng invariant file-header :19-21 "step write must never break orchestration").
- `setFlowStepPosture` (:210): append dòng posture-only (Status rỗng) — cần cho hiển thị provider/model sau restart (BUG-228).
- Lọc noise: chỉ append khi run là flow-engine-driven (`s.isFlowEngineDriven(parentRunID)`) — tránh log cho run thường.

**B3 — Replay trong `reconstructRun` (merge rule quan trọng nhất).**
- Chèn tại :866-876, thay `rows := s.resumedFlowStepRows(rs, st)` bằng:
```go
rows := s.resumedFlowStepRows(rs, st)          // evidence-walk giữ nguyên
if tlog, ok := s.workflowStore.(StepTransitionLogStore); ok {
    if lines, _ := tlog.LoadStepTransitions(rs.id); len(lines) > 0 {
        rows = applyStepTransitionReplay(rows, lines, st) // hàm mới
    }
}
s.seedFlowStepRuntimeRows(rs.id, rows)
```
- **Merge rule (chống regress legacy — phát hiện #1 của khảo sát):** replay theo từng node — node CÓ dòng log → dòng cuối thắng (last-wins) và GHI ĐÈ kết quả evidence-walk; node KHÔNG có dòng log nào → giữ nguyên kết quả evidence-walk (`resumedFlowStepRows`). KHÔNG bao giờ "log tồn tại ⇒ bỏ evidence-walk toàn bộ" — member legacy/không-label không có dòng log.
- **Normalize sau replay (mở rộng so với plan gốc):** trạng thái cuối theo log là in-flight thì không giữ nguyên trạng: `RUNNING → CANCELED`; `WAITING_USER_APPROVAL → CANCELED` **trừ khi** sidecar question/approval (BUG-271, đã load ở :749-829) còn record `pending` cho run đó → giữ WAITING. (Thiếu vế WAITING là tái tạo bug class BUG-251 ở dạng khác.)
- **Precedence hub (phát hiện #4):** sau merge, nếu `normalizeResumedFlowStatus(st) == RunStatusCompleted` (flow thật sự done) thì hub node = `DONE` thắng mọi kết quả replay (giữ nguyên hành vi :608-628). `I-3`: node `FAILED/CANCELED` theo log/evidence không bao giờ bị nâng `DONE` bởi fallback flowComplete.
- Giữ nguyên: KHÔNG đụng `flowEngineDriven`; KHÔNG seed khi `st.RunKind=="chat"` (:857).

**B4 — BUG-251 follow-up.** Mở rộng `normalizeResumedStatus` (hoặc điểm gọi trong `listAgentRunSummaries` fallback — xem BUG-251 doc) phủ agent-status `spawned`/`waiting_dependency` → cancelled. Test riêng.

**B5 — Delete lifecycle (phát hiện #5).** `deleteChatSession` (interactive_resume.go:54-66): thêm `DeleteStepTransitions(runID)` cạnh `DeleteFlowEvents`/turn-logs. Test: delete run → file sidecar biến mất.

**B6 — Persist-shape guard (`T-6` §4).** Test mới so sánh field-set của `sessionRecordFrom` (:606-642) với danh sách bắt buộc {LoopState, FlowCohortID, ActiveFlowNodes, ActiveFlowEdges, Label} — fail nếu ai đó xóa field khỏi record.

**B7 — Ma trận restart (`D-4`).** File `internal/runner/flow_restore_matrix_test.go`, dùng store THẬT (`NewLocalFileSessionStore` trên `t.TempDir()`, mẫu: `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone` :759): dựng run + ghi transitions + kill (tạo store instance mới cùng dir — mẫu `TestLocalFileSessionStoreFlowEventsDurability` :820) → `reconstructRun` → assert từng ô.

**B8 — Quyết `Q-2` Drive sync.** Tìm nơi liệt kê file sync lên Drive (Task-190 — search `flow-events` trong package drive/sync); nếu flow-events đã sync thì thêm `-step-transitions.ndjson` cùng chỗ; nếu không → ghi "local-only, cùng posture flow-events" vào §8. Đừng tự chế cơ chế sync mới.

### 9.3 Test scaffolding — mimic đúng các test này

| Cần test | Copy pattern từ (file:line) |
|---|---|
| Durability sidecar qua restart | `TestLocalFileSessionStoreFlowEventsDurability` (local_file_session_store_test.go:820) |
| Traversal guard / malformed / large line | `TestFlowEventsPathTraversalRejected` :929, `...SkipsMalformedLinesKeepsValid` :967, `...HandlesLargeLines` :1011 |
| Replay giữ FAILED (I-3) | `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone` (flow_step_runtime_test.go:759) |
| Legacy no-label vẫn đúng (merge rule B3) | `TestReconstructResumeInfersLegacyReviewerStatusesFromCohortOrder` :977 — PHẢI xanh nguyên trạng |
| Label-matched distinct statuses | `TestReconstructResumeRestoresDistinctReviewerStatusesByPersistedLabel` :896 |
| RUNNING→CANCELED khi resume | `TestReconstructResumeCancelsRunningReviewerButKeepsCompletedCoder` :832 |
| Non-completed → PENDING (fallback) | `TestReconstructNonCompletedFlowRunRestoresPendingSteps` :591 |
| Round-trip field session | `TestFlowCohortIDRoundTripsThroughSessionStore` (local_file_session_store_test.go:762) |

### 9.4 Bẫy đã biết

1. **BUG-232 deadlock:** `reconstructRun` phải unlock `s.mu` TRƯỚC khi seed (:840-848). Replay cũng vậy — `LoadStepTransitions` là I/O, gọi ngoài lock.
2. `seed` là wholesale replace (:208-212) — build xong toàn bộ rows rồi seed MỘT lần, đừng seed rồi patch từng dòng.
3. `ApplyStepTransition` với `Patch.Status` rỗng = giữ status (BUG-228 :231-234) — dòng posture-only khi replay phải map thành patch Provider/Model-only, không được đụng Status.
4. Đừng append log trong replay (replay gọi `seedFlowStepRuntimeRows`, không đi qua `setFlowStepStatus` → tự nhiên không double-log; giữ nguyên như vậy).
5. Log là per-run của PARENT (`parentRunID`) — child không có log riêng; đừng tạo file theo child id.
6. Round-reset của continue ghi DONE→PENDING hợp lệ vào log — replay last-wins tự xử lý đúng; đừng "tối ưu" bỏ qua dòng PENDING.

### 9.5 Khi nào dừng & hỏi

- `TestReconstructResumeInfersLegacyReviewerStatusesFromCohortOrder` đỏ sau B3 → merge rule sai, dừng sửa test, sửa merge.
- Thấy caller production của `reseedFlowStepRuntimeForResume` xuất hiện → code đã đổi, khảo sát lại trước khi nối replay.
- Nếu cần đụng `SupabaseWorkflowStore` để pass test nào đó → dừng: Supabase parity là Out of Scope, xem lại thiết kế test.
- Mọi quyết định lệch guide → ghi §8 + báo charter Task-238.
