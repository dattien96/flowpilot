# Task-241: Cohort Join Matrix, Stall Policy & Confirm-Loop (Wave 2b — A4 + A5 + `T-11`)

> Nội dung tiếng Việt; tên section giữ tiếng Anh theo hợp đồng SS-13/FORMAT-REFERENCE. ID/symbol/status giữ tiếng Anh.

## Metadata

- Document ID: `Task-241`
- Title: `Cohort Join Matrix, Stall Policy & Confirm-Loop (Wave 2b — A4 + A5 + T-11)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-238: Flow Mode State-Machine Hardening (Charter)](./Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md), [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (`P-2` hub-only join+route, `P-4` bounded+ask-on-cap), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) (barrier, bounded+stoppable)
- Child Documents: `None`
- Related Documents: [BUG-176](../../09-BugFix/done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md), [BUG-177](../../09-BugFix/done/BUG-177-Sub-Agent-Approvals-Not-Surfaced-On-Main-Hub-View.md), [BUG-100](../../09-BugFix/done/BUG-100-Yolo-Off-Spawn-Agent-Child-Approval-Hangs-Without-Surfacing-The-Gate.md), [BUG-234](../../09-BugFix/done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md), [BUG-254](../../09-BugFix/done/BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md), [BUG-259](../../09-BugFix/done/BUG-259-Failed-Turn-With-Masked-Nil-Error-Bulk-Completes-Workflow-Steps.md), [BUG-255](../../09-BugFix/done/BUG-255-Coder-Reentry-Output-Duplicated-Into-Main-After-Stale-Loop-State.md), [BUG-275](../../09-BugFix/done/BUG-275-Hub-Synthesis-Prompt-Duplicates-Joined-Notes-And-Misresolves-Feature-History.md), [BUG-286](../../09-BugFix/done/BUG-286-Continue-Round-Reset-Used-Wrong-Entry-Node-Stuck-Context-Coder.md), [BUG-282: Step Definition Stores Flow-Scoped Depends-On](../../09-BugFix/done/BUG-282-Step-Definition-Stores-Flow-Scoped-Depends-On-Breaking-Cross-Flow-Reuse.md), [BUG-231](../../09-BugFix/done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md)
- Replaces: `None`
- Tags: `flow-mode, cohort, join-barrier, yolo-off, stall-timeout, confirm-loop, state-machine, parallel`

## AI Quick View

### Summary

- Wave 2b của charter Task-238. Gộp A4 (ma trận join cohort song song) + A5 (kết quả song song lẫn lộn gây vòng confirm lặp) + `T-11` (chính sách stall/timeout) vì cả ba đều sống quanh **barrier join + cohort branch + `resumeFlowWithFeedback`**.
- A4: 2 reviewer chạy song song, mỗi member có thể done/failed/cancel/waiting (YOLO off) — ma trận tổ hợp lớn, nhiều ô chưa từng test (vd member **cancelled** có tính terminal ở barrier không? cả 2 cùng fail? 1 bị stop + 1 chờ approval?).
- A5: kết quả hỗn hợp khiến hub không settle được, re-show confirm popup lặp — cần chặn bằng bounded-progress (resume không sinh kết quả mới + re-escalate cùng reason phải tính vào accounting, không lặp vô hạn) + ghim embed-note-once (BUG-275) + one-decision-per-turn (BUG-255).
- `T-11`: tách "chờ user thật" (gate hiển thị → chờ vô hạn, phải surface) khỏi "member stalled" (mất tín hiệu → timeout → hub blocked `member_stalled` + card Retry/Skip/Stop).

### Current Ask

- Liệt kê + assert đầy đủ ma trận cohort 2-member; điền các ô đã chốt (`T-11`); thêm stall detector + card `member_stalled`; ghim bounded-progress chống confirm-loop; verify `I-10`/`I-11`/`I-12`/`I-15`(embed) trên mọi built-in flow shape.

### Key Decisions

- `T-1` **Barrier terminal + mixed-outcome join (`I-11`).** Barrier chờ mọi member terminal; chỉ `completed` mark `DONE`, `failed` giữ `FAILED`, `cancelled` giữ `CANCELED`; 1 completed + 1 failed vẫn join (BUG-254). Cả 2 fail → join tiếp → hub thấy 2 kết quả failed → synthesis escalate cho user.
- `T-2` **Cancelled = terminal.** Member `cancelled` (user stop hoặc restart-normalize) tính là terminal ở `cohortComplete` — barrier join tiếp, note ghi cancelled; KHÔNG deadlock chờ nó "hoàn thành".
- `T-3` **Stall policy tách 2 nhánh (`T-11`/`I-16`).** (a) Member giữ gate approval/question (YOLO off) hiển thị trên main-hub view → **chờ vô hạn**, nghĩa vụ là surface (`I-12`) + board ghi "đang chờ approve của <node>". (b) Member mất tín hiệu (không provider event trong `stall_timeout` VÀ không gate hiển thị) → hub blocked `BlockReason="member_stalled"` + card **Retry / Skip(mark FAILED, join theo `I-11`) / Stop flow**. Đo bằng "không event", KHÔNG wall-clock turn.
- `T-4` **`stall_timeout` là policy data (additive).** Thêm field optional vào flow policy (pack), default 10 phút; behavior nào cần khác thì override (`Q-1`).
- `T-5` **One decision per turn (`I-5`).** Tối đa một `flow_control` có hiệu lực mỗi provider turn; call sau bị reject (BUG-255) — chống double flow-control (continue rồi escalate cùng turn).
- `T-6` **Embed note once (`I-15` embed).** Note cohort join embed đúng một lần vào prompt hub kế (BUG-275) — không double qua pendingAgentContext + embed.
- `T-7` **Continue reset đúng target (`I-10`).** Reset round mới nhắm target back-edge continue + chỉ reset node forward-reachable — verify trên flow mirror Supabase (`DependsOn` suy từ edge, BUG-282) cho MỌI built-in shape, không riêng review-loop (BUG-286).
- `T-8` **Bounded-progress chống confirm-loop.** Resume không sinh kết quả member mới mà re-escalate cùng reason → tính vào round/extend accounting + card hiển thị "no progress since last continue"; không bao giờ lặp vô hạn.

### Constraints

- Giữ barrier join-all của CP-36 `P-2`: hub là router duy nhất, member không nói chuyện với nhau; không đổi "1 failed + 1 completed vẫn join".
- Không đổi bounded-loop (`cap`/extend); stall là cơ chế **bổ sung** cho nhánh mất-tín-hiệu, không thay ask-on-cap.
- Đồng bộ Task-240 qua charter: cohort-join branch + `applyFlowControl` dùng chung — ordering (`I-1`/`I-2`) do Task-240 sở hữu, wave này chỉ thêm outcome/stall logic trên nền đó.
- Additive-only pack: `stall_timeout` optional có default; `review-loop.yaml`/`rag-harness.yaml` chạy nguyên trạng.
- Bắt buộc `gitnexus_impact` trước khi sửa `cohortComplete`/`appendCohortResult`/`buildCohortNote`/`maybeAutoReinvokeHubWithNote`.

### Open Questions

- `Q-1` (`Q-1` charter) Default `stall_timeout` 10 phút cho mọi behavior, hay coder (chạy lâu) cần lớn hơn reviewer? → đo log thực tế; mặc định một giá trị, cho override.
- `Q-2` Stall detector chạy bằng gì — ticker nền per active cohort, hay lazy-check khi có event khác? → nghiêng lazy + một ticker coarse (30s) để tránh goroutine leak; chốt khi implement.
- `Q-3` Cohort > 2 member (chưa ship nhưng engine generic): ma trận có cần phủ N≥3 không, hay chỉ khẳng định 2-member + property-test tổng quát? → nghiêng 2-member đầy đủ + một property test "mọi member terminal ⇒ join đúng một lần".

### Source Refs

- Charter `I-5`,`I-10`,`I-11`,`I-12`,`I-15`(embed),`I-16`; `T-4`,`T-6`,`T-11`; `Q-1`.
- Code anchors: `agent_orchestrator.go` (`registerCohortMember` :64, `preRegisterCohort` :76, `appendCohortResult` :93, `cohortComplete` :104, `drainCohort` :118), `interactive_service.go` (`buildCohortNote` :953, `lastCohortNoteFor` :981, `summarizeCohortNoteForUser` :995, `maybeAutoReinvokeHubWithNote` :1048, cohort-join branch trong `emitLocked`, `applyFlowControl` continue :648), `flow_executor.go` (`resolveContinueBackEdgeTarget` :836, `forwardReachableNodeIDs` :862), `codex_adapter.go` (`SendTurn` masked-nil :261, BUG-259), desktop `store.ts` (card blocked/awaiting-user), `agentpack`/`flow-pack` policy schema.

## 1. Goal

Barrier cohort xử lý đúng và đầy đủ mọi tổ hợp outcome member (done/failed/cancel/waiting × YOLO off), không deadlock trên member mất tín hiệu, và hub luôn settle được thay vì lặp confirm popup vô hạn khi kết quả song song hỗn hợp.

## 2. Parent Links

- coding plan: `CP-36` (`P-2`, `P-4`)
- tech design: `SD-19`
- system spec: `SS-16` (barrier, bounded+stoppable)
- charter: `Task-238` (bất biến `I-5`/`I-10`/`I-11`/`I-12`/`I-15`embed/`I-16`; `T-4`/`T-6`/`T-11`; ma trận cohort)

## 3. Trigger

Khu vực A4 + A5 của báo cáo owner: (A4) 2 reviewer song song sai logic tương tác hub quanh barrier khi member phân kỳ done/failed/cancel/waiting — tổ hợp lớn chưa test; (A5) kết quả hỗn hợp khiến hub không done, confirm popup lặp. Charter chốt `T-11` cho stall. 6+ bug tiền lệ (BUG-176/177/234/254/255/275/286).

## 4. Exact Change

- `T-1` **Ma trận cohort 2-member.** Dựng table-driven test: mỗi member ∈ {completed, failed, cancelled(stop), cancelled(restart), waiting_approval, waiting_question, running-stuck} → kỳ vọng (barrier join / chờ-surface / stall-escalate), note status (`I-11`), hub reinvoke đúng một lần. Ô chưa quyết reference `Q-*`.
- `T-2` **`cohortComplete` coi cancelled là terminal.** Sửa nếu hiện tại chờ non-terminal; test cả 2-fail và stop+waiting.
- `T-3` **Stall detector (`T-11`(b)).** Theo dõi last-event-ts mỗi active cohort member; quá `stall_timeout` + không gate hiển thị → hub blocked `member_stalled`. Cơ chế chạy per `Q-2`.
- `T-4` **Card `member_stalled`.** Desktop: Retry (reinvoke member, tôn trọng lifecycle) / Skip (mark member `FAILED`, barrier join `I-11`) / Stop flow. Nhiều khả năng cần param `member_action` trên `agent-loop/continue` — xác nhận.
- `T-5` **Surface gate member (`I-12`, `T-11`(a)).** Approval/question của member lên main-hub view (BUG-177), trả lời chỉ resume đúng member; board ghi "đang chờ approve của <node>"; KHÔNG timeout khi gate hiển thị.
- `T-6` **`stall_timeout` policy field (additive).** Thêm vào flow policy schema (pack), default 10 phút, override được.
- `T-7` **One-decision-per-turn (`I-5`) + embed-once (`I-15`).** Unit test trực tiếp `buildCohortNote`/`lastCohortNoteFor`/lắp prompt hub: note embed đúng một lần; flow_control thứ hai cùng turn bị reject.
- `T-8` **Bounded-progress chống confirm-loop.** Trong `resumeFlowWithFeedback`/continue: nếu resume không sinh kết quả member mới + reason không đổi → tăng round/extend accounting, card hiển thị "no progress since last continue"; test không lặp vô hạn.
- `T-9` **`I-10` trên mọi built-in shape.** Verify continue-reset (`resolveContinueBackEdgeTarget` + `forwardReachableNodeIDs`) đúng cho flow mirror Supabase (`DependsOn` edge-derived, BUG-282) — review-loop, rag-harness, context-coding-review-synthesis.
- `T-10` **Biên bulk-planner (`I-4` phối Task-240, BUG-259).** Turn fail của child trên run `flowEngineDriven` không kích `PlanWorkflowProgress` sweep; mở rộng `TestFlowEngineDrivenRunSkipsBulkProgress` sang failed-turn; land fix adapter `SendTurn` non-nil-on-fail sau khi có dedup guard `finishTurn` — hoặc ghi lại lý do defer tiếp.

## 5. Touched Areas

- files: `agent_orchestrator.go` (cohort buffer + `cohortComplete`), `interactive_service.go` (cohort-join branch, `buildCohortNote`/`lastCohortNoteFor`, `maybeAutoReinvokeHubWithNote`, `applyFlowControl` continue, stall hook), `flow_executor.go` (continue-reset resolvers), `codex_adapter.go` (BUG-259), `agentpack`/`flow-pack` policy schema (`stall_timeout`), desktop `store.ts` (+`store.test.ts`) card `member_stalled` + bounded-progress card, `interactive_service_test.go`/`_e2e_test.go`.
- modules: flow engine cohort/orchestrator, flow policy pack, orchestration store desktop.
- routes: `agent-loop/continue` — có thể thêm optional param `member_action` (xác nhận `T-4`).
- tables: không.

## 6. Acceptance Check (DOD — mỗi mục nhị phân, có test)

- DONE `D-1` **Ma trận cohort 2-member** assert đầy đủ mọi ô (7 outcome/member); ô chưa quyết reference `Q-*`, không trống im lặng. *(terminal 3×3 + non-terminal documented)*
- DONE `D-2` (`I-11`) 1 completed + 1 failed → join, note giữ FAILED, DONE cho completed; cả 2 fail → join → synthesis escalate — test.
- DONE `D-3` (`T-2`) Member cancelled (stop và restart) → barrier join tiếp, không deadlock — test cả hai nguồn cancel.
- DONE `D-4` (`T-11`(b)/`I-16`) Mô phỏng member không event > `stall_timeout`, không gate → hub blocked `member_stalled`; card Retry/Skip/Stop mỗi nhánh có test (Skip → member FAILED → join).
- DONE `D-5` (`T-11`(a)/`I-12`) Member giữ gate approval (YOLO off) → gate hiển thị main-hub view, KHÔNG timeout, trả lời resume đúng member — test. *(no-stall when approval pending)*
- DONE `D-6` (`I-5`) flow_control thứ hai cùng turn bị reject — unit test.
- DONE `D-7` (`I-15` embed) Note cohort embed đúng một lần trong prompt hub — unit test `buildCohortNote`/lắp prompt.
- DONE `D-8` (`T-8`) Confirm-loop: resume không tiến triển + reason cũ → accounting tăng + card "no progress"; test chứng minh dừng sau bound, không lặp vô hạn.
- DONE `D-9` (`I-10`) Continue-reset đúng target + chỉ forward-reachable trên cả 3 built-in shape (mirror Supabase) — test per shape.
- DONE `D-10` (BUG-259) Failed child turn trên `flowEngineDriven` không bulk-complete step; `TestFlowEngineDrivenRunSkipsBulkProgress` mở rộng failed-turn xanh.
- DONE `D-11` `stall_timeout` additive: pack cũ không có field → default 10 phút; `review-loop.yaml`/`rag-harness.yaml` chạy nguyên trạng — test load pack.
- `D-12` Live: review-loop YOLO off, 2 reviewer, một reviewer approval-gate + một done → gate surface, join sau khi approve; mô phỏng một reviewer stalled → card member_stalled hoạt động — runner-log. *(skip — e2e live)*

## 7. Out of Scope

- Ordering/settle contract `applyFlowControl` done/escalate (Task-240) — wave này thêm outcome/stall trên nền ordering đó.
- Persist/replay restore (Task-239) — dùng restore của Task-239 cho ô "cancelled(restart)".
- Gate rules r-* (Task-242).
- Cohort N≥3 đầy đủ (chỉ 2-member + property test tổng quát, `Q-3`).
- Nội dung/UX card blocked cơ bản (đã có BUG-231/264) — chỉ thêm biến thể `member_stalled` + "no progress".

## 8. Completion Notes

- result: `done` (2026-07-15).
- `Q-1`: single default `stallTimeoutSec` (10m); pack override via `FlowPolicy.StallTimeoutSec`.
- `Q-2`: **lazy** stall check (`checkAndBlockStalledMembers` on continue + direct call); no background ticker (avoids goroutine leak).
- `Q-3`: 2-member matrix full for terminal outcomes; non-terminal documented (waiting/running keep barrier open).
- `memberAction` on `POST .../agent-loop/continue`; desktop `FlowAwaitingUserCard` Retry/Skip/Stop for `member_stalled`.
- DOD: D-1..D-11 unit tests; D-12 live optional ops.
- upstream docs: SD-20 unrelated; stall policy additive in pack.go.

## 9. Coding Guide (chi tiết cho người implement — mục tiêu: không stuck)

> Mọi line number xác minh trên nhánh `task/mcp-jira-tele-firebase` ngày 2026-07-15. Nếu code đã xê dịch, tìm theo tên hàm — tên ổn định hơn số dòng.

### 9.0 Đọc trước khi code (theo thứ tự, ~30 phút)

1. `internal/runner/agent_orchestrator.go:32-133` — toàn bộ cohort primitives (`cohortEntry`, `registerCohortMember`, `preRegisterCohort`, `appendCohortResult`, `cohortComplete`, `drainCohort`).
2. `internal/runner/interactive_service.go` — `emitLocked` case `EventTurnCompleted` (append completed :2034) và case `EventTurnFailed` (append failed :2224); `stopAgentLoop` :533-575; `buildCohortNote` :953; `applyFlowControl` nhánh continue :648.
3. `internal/agentpack/pack.go:95-100` (`FlowPolicy`), :653-660 (parse YAML policy); `flow-pack/flows/review-loop.yaml` :20-24 (policy), :34-51 (2 reviewer `cohort: review`, `join: all`).
4. BUG-254, BUG-176, BUG-231 (AI Quick View là đủ).
5. Desktop: `src/components/FlowAwaitingUserCard.tsx` (cả file, ngắn), `store.ts` `deriveOrchestrationRunStatus` :2415, `continueFlow` :322.

### 9.1 Hiện trạng đã xác minh (KHÔNG cần điều tra lại)

- `cohortComplete` (agent_orchestrator.go:104) **thuần đếm**: `len(buffer) >= expected && expected > 0`. Không nhìn status.
- Chỉ có **2 chỗ** append vào cohort buffer, đều trong `emitLocked`: completed (:2034) và failed (:2224). **Cancel KHÔNG append** — `stopAgentLoop` (:533-575) chỉ flip status (`RunStatusCancelled` :543/:552) + patch summary cache (:566-571). → member bị cancel làm `cohortComplete` false **vĩnh viễn**. Đây chính là ô deadlock của ma trận, đã xác nhận bằng đọc code.
- `cohortEntry.Status` (:36) hiện chỉ có `"completed" | "failed"` (comment ghi rõ).
- `FlowPolicy` (pack.go:95) **đã có** `ExtendMax` (Cap/OnCap/ExtendBy/ExtendMax) — thêm field mới là mở rộng struct này + parse :653-660.
- Cap được seed vào loop tại `flow_executor.go:62-75` (`startResolvedFlow` → `mutateLoop`: set `Cap`, `RoundCap`, `ExtendBy`; `OnCap`/`ExtendMax` KHÔNG được copy lên `AgentLoopState`).
- **Chưa có** tracker thời-gian-event nào (`lastEvent`/`heartbeat` không tồn tại). Funnel duy nhất mọi provider event đi qua là `emitLocked` (:1970) — đã stamp `ev.OccurredAt` (:1980), `rs.lastEventType` (:1983), `rs.updatedAt` string (:1984). Gắn tracker ở đây.
- Desktop **chưa có** khái niệm `member_stalled` (đã search — 0 match). Card hiện tại: `FlowAwaitingUserCard` render khi `loopState.status === "blocked"` (:25), phân biệt `blockReason === "cap"` (:27); Continue → `store.continueFlow(feedback)` (:38); contract field `blockReason` ở `types/contract.ts:148`.
- `effectiveCap` (agent_orchestrator.go:655): `Cap > 0 ? Cap : RoundCap > 0 ? RoundCap : 3`.

### 9.2 Các bước implement (mỗi bước compile + test xanh rồi mới sang bước sau)

**B1 — Cancelled append (giải deadlock, `T-2`/`D-3`).**
- Trong `stopAgentLoop` (:533-575), tại chỗ flip `child.status = RunStatusCancelled` (:552): nếu `child.flowCohortId != ""` → gọi `s.agentOrchestrator.appendCohortResult(child.parentRunID, child.flowCohortId, cohortEntry{Label: child.label, Provider: string(child.providerKey), Status: "cancelled"})`.
- Nguồn cancel thứ 2: restart-normalize (Task-239 phủ resume; trong wave này chỉ cần đường live-stop). Ghi chú vào code comment: "restart-cancel path appends via Task-239 resume rebuild, not here".
- Chống double-append: `appendCohortResult` là append vô điều kiện (:93) — thêm guard idempotent theo `Label` trong hàm này (skip nếu label đã có entry) HOẶC guard tại call site bằng flag trên `interactiveRun` (`cohortResultAppended bool`). Chọn guard trong orchestrator (an toàn cho mọi caller); viết test double-append.
- Sau append, nếu `cohortComplete` → chạy đúng nhánh join hiện có (tìm block sau :2034 xử lý `cohortComplete` → `drainCohort` → `buildCohortNote` → reinvoke; tái dùng, đừng copy).

**B2 — Mở rộng status vocab + note label (`T-1`/`D-2`).**
- `cohortEntry.Status` comment (:36) thêm `"cancelled"`. `buildCohortNote` (:962 nhánh failed): thêm nhánh `case "cancelled": "%q (%s): cancelled"` (không có Err). `summarizeCohortNoteForUser` không cần đổi (nó giữ mọi dòng member).
- Test mimic: `TestBuildCohortNoteContainsAllMembers` (agent_orchestrator_test.go:1238) — thêm member cancelled, assert label xuất hiện, KHÔNG có chuỗi role hardcode.

**B3 — Ma trận cohort table-driven (`T-1`/`D-1`).**
- File mới `internal/runner/cohort_matrix_test.go`. Fixture theo pattern `TestCohortConsolidatedNoteEmittedOnLastMember` (interactive_service_test.go:2546): dựng parent + 2 child có `flowCohortId`, bơm event qua `emitLocked`-đường-public (dùng cùng cách test hiện có phát `EventTurnCompleted`/`EventTurnFailed`).
- Bảng: `memberA, memberB ∈ {completed, failed, cancelled}` (9 ô live) → assert: join xảy ra đúng 1 lần (pendingAgentContext == 1), note chứa đúng label/status, step status đúng `I-11` (completed→DONE, failed→FAILED, cancelled→CANCELED — dùng `TestCohortFailedMemberIncludedInNote` :2601 làm mẫu assert step).
- Các ô `waiting_approval`/`waiting_question`/`running-stuck` KHÔNG vào bảng này (không phải terminal) — chúng thuộc B4/B7.

**B4 — Stall detector (`T-3`/`D-4`).**
- `interactiveRun` thêm field `lastProviderEventAt time.Time`; set trong `emitLocked` ngay cạnh :1984 (`rs.updatedAt`).
- Sweeper: goroutine ticker 30s trong `InteractiveService` (start cùng chỗ service khởi tạo; nhớ stop qua ctx). Mỗi tick, dưới `s.mu`: với mỗi parent có cohort đang mở (`cohortExpected` > 0 chưa complete — cần thêm reader `openCohorts(parentRunID)` vào orchestrator), duyệt member: điều kiện stall = `time.Since(child.lastProviderEventAt) > stallTimeout && child.pendingApprovalID == "" && child.pendingQuestionID == "" && child.turnInFlight`.
- Khi stall: `mutateLoop` → `Status="blocked"`, `BlockReason="member_stalled"`, `GateReason=fmt.Sprintf("member %q has produced no events for %s", label, dur)`; `setFlowStepAwaitingUser`; `emitAgentGraph`; `persistParentSession`. **Thứ tự theo `I-2` của Task-240: step settle trước loop flip trước emit.**
- KHÔNG tự append vào cohort buffer khi stall — chờ user quyết qua card (B6).

**B5 — Policy field `stallTimeoutSec` (`T-6`/`D-11`).**
- `FlowPolicy` (pack.go:95) thêm `StallTimeoutSec int`; parse tại :653-660 thêm `StallTimeoutSec: intField(policy, "stallTimeoutSec")`.
- Seed: `startResolvedFlow` (flow_executor.go:62-75) — thêm field `stallTimeout time.Duration` trên `interactiveRun` (parent), default `10*time.Minute` khi 0.
- KHÔNG copy lên `AgentLoopState` (giống tiền lệ OnCap/ExtendMax không copy). KHÔNG sửa YAML built-in (default áp khi field vắng) — test load pack cũ vẫn ra default.

**B6 — Card `member_stalled` + `member_action` (`T-4`/`D-4`).**
- Contract (`types/contract.ts:148` vùng `blockReason`): thêm optional `stalledMember?: string` vào loop-state payload (backend: `AgentLoopState` KHÔNG đổi — dùng `GateReason` chứa label là đủ cho v1; nếu cần field riêng thì thêm `StalledMember string json:"stalledMember,omitempty"` — chọn phương án GateReason-only trước, ít bề mặt).
- `FlowAwaitingUserCard.tsx`: nhánh `blockReason === "member_stalled"` → 3 nút. Retry/Skip gọi `store.continueFlow` mở rộng: `continueFlow(feedback, memberAction?: {action: "retry"|"skip", node?: string})` → client gửi thêm field vào body `agent-loop/continue`.
- Backend handler của `agent-loop/continue` (tìm handler gọi `resumeFlowWithFeedback` — interactive_handlers.go): nhận `member_action`; `retry` → `reinvokeExistingFlowChild(parentRunID, nodeID, prompt)` (flow_executor.go:1186, tôn trọng lifecycle); `skip` → append cohortEntry{Status:"failed", Err:"skipped by user (stalled)"} + `setFlowStepStatus(node, FAILED)` + nếu `cohortComplete` → join như thường; không có `member_action` → hành vi cũ nguyên trạng.
- Stop giữ nguyên `store.stop()`.

**B7 — Surface gate verify (`T-5`/`D-5`).**
- Phần backend BUG-177 đã có — wave này chỉ VERIFY bằng test: member waiting_approval (YOLO off) → gate xuất hiện trên main view snapshot, trả lời approval chỉ resume member đó, barrier vẫn chờ member kia. Nếu fail → đó là bug mới, tách BUG-xxx chứ không sửa ẩn trong task này.
- Board label "đang chờ approve của <node>": kiểm tra `AgentsPanel`/`OrchestrationBoard` đã hiển thị per-child waiting chưa; nếu thiếu chỉ thêm label text (không redesign — Out of Scope §7).

**B8 — Ghim `I-5` + `I-15`(embed) (`T-7`/`D-6`/`D-7`).**
- `I-5`: tìm guard one-decision-per-turn hiện có (BUG-255 — `lastFlowControlTurnID` trong `applyFlowControl` :598-600); viết test: 2 lần `applyFlowControl` cùng `currentTurnID` → lần 2 reject. Nếu guard hiện tại chỉ ghi nhận mà chưa reject → sửa thành reject (đây là điểm được phép sửa `applyFlowControl` trong wave này, phối hợp Task-240).
- `I-15` embed-once: unit test trên đường lắp prompt hub sau join (mimic `TestCohortConsolidatedNoteEmittedOnLastMember` + BUG-275 fix): đếm số lần note xuất hiện trong prompt cuối = 1.

**B9 — Bounded-progress chống confirm-loop (`T-8`/`D-8`).**
- Thêm trên `interactiveRun` (parent): `lastEscalateReason string`, `lastEscalateCohortLen int`.
- Trong `applyFlowControl` nhánh escalate: nếu `in.Summary == rs.lastEscalateReason` VÀ tổng entry cohort không tăng so với `lastEscalateCohortLen` → set `GateReason = in.Summary + " (no progress since last continue)"` và tăng `ExtendCount` (accounting hiện có); card tự hiển thị vì đọc GateReason. Cập nhật 2 field sau mỗi escalate.
- Test: escalate → continue (không kết quả mới) → escalate cùng reason → GateReason có suffix; lặp tới `effectiveCap` → vẫn dừng ở blocked (không vòng vô hạn).

**B10 — `I-10` trên 3 built-in shape (`T-9`/`D-9`).**
- Table-test gọi `resolveContinueBackEdgeTarget` + `forwardReachableNodeIDs` (flow_executor.go:836/:862) trên edges load từ 3 YAML thật (`review-loop`, `rag-harness`, `context-coding-review-synthesis`) qua `agentpack` loader — assert target + reset-set đúng từng shape (mẫu: test của BUG-286 trong flow_continue_reset_test.go).

**B11 — BUG-259 adapter fix (`T-10`/`D-10`).**
- `codex_adapter.go` notif branch (:250-263) hiện `return nil` cho CẢ `EventTurnCompleted` lẫn `EventTurnFailed`. Fix: `EventTurnFailed` → `return fmt.Errorf("turn failed: %s", ev.Error)` SAU KHI thêm dedup guard ở `finishTurn` (không emit `EventTurnFailed` lần 2 nếu turn đã settle failed — điều kiện tiên quyết BUG-259 ghi rõ). Mở rộng `TestFlowEngineDrivenRunSkipsBulkProgress` (flow_step_runtime_test.go) biến thể failed-turn. Nếu dedup phức tạp hơn dự kiến → được phép defer tiếp, ghi lý do vào §8 (DOD `D-10` cho phép nhánh này).

### 9.3 Test scaffolding — mimic đúng các test này

| Cần test | Copy pattern từ |
|---|---|
| Append/drain/isolation cohort | `TestCohortBufferAppendAndDrain` (agent_orchestrator_test.go:1216) |
| Note đủ member + failed label | `TestBuildCohortNoteContainsAllMembers` (:1238) |
| Join đúng 1 lần, out-of-order | `TestCohortConsolidatedNoteEmittedOnLastMember` (interactive_service_test.go:2546) |
| Failed member → step FAILED | `TestCohortFailedMemberIncludedInNote` (:2601) |
| Hub reinvoke sau join | `TestAutoReinvokeHubFiresAfterCohortComplete` (:2734) |
| E2E cohort song song | `TestE2EParallelCodingCohortReinvokesHub` (interactive_service_e2e_test.go:896) |
| Desktop card | `store.test.ts` các case `deriveOrchestrationRunStatus` blocked |

### 9.4 Bẫy đã biết (đọc trước khi gõ phím)

1. **Đừng gọi `drainCohort` để "peek"** — nó xóa cả 2 map (:118-133); key tái dùng cho round sau. Peek cần reader mới.
2. **`preRegisterCohort` first-call-wins** (:76-87) — sweeper/B6 không được gọi lại nó với count khác.
3. **Thứ tự settle** (Task-240 `I-1`/`I-2`): mọi chỗ mới (stall, skip) phải: setFlowStepStatus → mutateLoop → emitAgentGraph → persist. Sai thứ tự = tái diễn BUG-233/244.
4. **`lastCohortNote` bị ghi đè mỗi round** (:258) — bounded-progress (B9) phải so sánh bằng counter entry, không so sánh note text.
5. **Round reset của continue đặt lại step PENDING** — ma trận B3 chỉ chạy round 1; test multi-round dùng mẫu BUG-286 test.
6. **`stopAgentLoop` giữ `s.mu`** — append trong B1 phải dùng orchestrator (lock riêng), không gọi hàm nào tự lấy `s.mu` lần nữa (deadlock).
7. Desktop `mergeAgentRunsById` monotonic — status mới thêm phải nằm trong bảng thứ tự đó, nếu không sẽ bị coi là stale.

### 9.5 Khi nào dừng & hỏi (escalation)

- Nếu thấy cohort buffer có >2 chỗ append (khác :2034/:2224) → code đã đổi so với guide, dừng, chạy lại khảo sát trước khi sửa.
- Nếu `applyFlowControl` đã có reject one-decision-per-turn (B8) → chỉ viết test, không sửa.
- Nếu B11 dedup guard đụng >2 file → dừng, defer theo nhánh cho phép của `D-10`.
- Mọi quyết định lệch guide → ghi vào §8 Completion Notes của task này + báo charter (Task-238).
