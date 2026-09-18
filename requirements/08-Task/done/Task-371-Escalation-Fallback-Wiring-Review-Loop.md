# Task-371: Escalation Fallback Wiring In Review Loop

## Metadata

- Document ID: `Task-371`
- Title: `Escalation Fallback Wiring In Review Loop`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Completed: `2026-09-16` (CA-879)
- Parent Documents: [CP-65 P-4](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- Replaces: `None`
- Tags: `tournament, escalation, review-cap, debate-stall, fallback-wiring`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Slice 4 của CP-65: chốt thoát hiểm (Escalation Fallback Leg) — khi review loop chạm trần cap (`st.Round >= st.RoundCap`, `GateReason "round cap reached"`) hoặc vibe owner-debate bế tắc, Runner tự chuyển trạng thái sang `tournament_escalation` và dispatch flow `tournament-harness` với đúng ngữ cảnh task đang kẹt, thay vì terminate run với failed/ask_user.
- Điểm neo có sẵn: cap handling tại `agent_orchestrator.go` (vùng round-cap → escalate card), stall sweep trong `cohort_stall.go` (MemberAction retry/skip), owner-fail parked cap trong `vibe_debate.go`.
- Cờ `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION` mặc định TẮT — unset giữ nguyên hành vi dừng báo fail cũ byte-identical (CP-65 §8 fallback; constraint không tốn token cho task thường).
- SD-24 (Durable Turn Dispatch): việc chuyển sang tournament phải đi qua cơ chế dispatch bền vững hiện có — restart giữa chừng không làm mồ côi candidate chạy dở.

### Current Ask

- Implement P-4 theo CP-65 §4: wiring escalation trong `interactive_service.go` + `agent_orchestrator.go`/`cohort_stall.go`. 2 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Trigger condition: (a) flow loop đạt cap mà `onCap: escalate` sắp surf card cho user, hoặc (b) owner-debate stall (owner-fail parked cap) — CHỈ khi flag bật: mutate loop state sang `tournament_escalation` (trạng thái mới, không phải failed/stopped) rồi spawn `tournament-harness` kế thừa workspace + intent + change contract của run kẹt.
- `T-2` Context handoff: run kẹt truyền sang tournament payload tối thiểu — task prompt/intent, frozen contract DeclaredPaths, log vòng lặp trước (để candidate tránh lặp cùng hướng đã chết — distill của PDR), cộng `attempt: 1`; mỗi vòng retry back-edge trong flow tournament tăng attempt thêm 1, behavior P-3 hết `max_attempts` thì ra `ask_user` thay vì retry; merge-conflict không retry.
- `T-3` Kết quả tournament quay về run gốc: winner merge xong → run gốc chuyển sang reviewer/validate lại như flow thường (không kết thúc ở tournament); tournament hòa + human từ chối → run gốc về trạng thái escalate cũ (fail-closed an toàn như hiện tại).
- `T-4` Flag là điều kiện cần của MỌI nhánh mới — mọi code path escalation phải no-op khi flag tắt; không đổi semantics của `onCap: done`.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — escalation trigger (round-cap, stall sweep, owner-fail parked cap) và `escalateToTournament` không nhận `providerKey` (grep verify 0 hit); candidate providers do P-3 lo.
- Prior CA: none (new feature `tournament-harness`); đóng task phải kèm CA-NNN ledger entry.
- Flag unset → byte-identical: cap vẫn escalate như cũ, stall sweep vẫn retry/skip như cũ (regression safety bắt buộc).
- Không tự kích hoạt tournament khi cap đến từ flow đã là `tournament-harness` (chống đệ quy vô hạn — tournament của tournament).
- GitNexus impact analysis TRƯỚC KHI sửa `agent_orchestrator.go`/`interactive_service.go` — đây là symbols lõi d=1 với toàn bộ flow engine, blast radius sẽ cao; báo cáo risk trước khi edit.

### Open Questions

- None (Q-1 auto_pick đã chốt ở P-3; chỉ áp dụng khi tournament chạy, không đổi wiring escalation).

### Source Refs

- CP-65 §3.1, §4 P-4, test signatures 10–11, §8 (flag fallback).
- `internal/runner/agent_orchestrator.go` (round-cap region ~530, `effectiveCap`), `internal/runner/cohort_stall.go` (stall sweep), `internal/runner/vibe_debate.go` (owner-fail parked cap), `internal/runner/hub_stall.go`, `internal/runner/flow_executor.go` (dispatch/entry nodes).

## 1. Goal

Review loop/debate kẹt trần không còn là ngõ cụt: với flag bật, hệ thống tự mở đấu trường ứng viên để giải quyết bế tắc, và kết quả tournament quay lại luồng review/validate bình thường của run gốc — với flag tắt, mọi thứ y như cũ.

## 2. Parent Links

- coding plan: `CP-65-Multi-Candidate-Tournament-Harness.md` P-4
- tech design: `SD-24-Durable-Turn-Dispatch.md`, `SD-19-Agent-Flow-Engine.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`

## 3. Trigger

P-3 đã có flow tournament chạy được từ picker; P-4 biến nó thành lối thoát tự động cho đúng trường hợp hệ thống tự bế tắc — giá trị cốt lõi của CP-65 (giải phóng khỏi bẫy lặp tuần tự).

## 4. Exact Change

- `T-1` **`internal/runner/agent_orchestrator.go`** (modified): tại vùng round-cap — khi flag bật VÀ flow hiện tại không phải `tournament-harness`: thay vì chỉ stamp escalate card, set loop state `LoopStatusTournamentEscalation = "tournament_escalation"` (const mới trong `internal/runner/flow_step_runtime.go`, additive) + dispatch tournament (payload theo T-2).
- `T-2` **`internal/runner/interactive_service.go`** (modified): dispatch helper `escalateToTournament(parentRunID string, intent string, frozen FrozenRecord) (childRunID string, err error)` — spawn run `tournament-harness` con kế thừa workspace/contract, ghi nhận quan hệ parent/child để kết quả quay về (T-3); chống đệ quy (không escalate từ tournament flow).
- `T-3` **`internal/runner/cohort_stall.go`** + **`internal/runner/vibe_debate.go`** (modified): stall sweep/owner-fail path — cùng điều kiện flag, cùng `escalateToTournament` thay vì parked-cap terminate.
- `T-4` **Return path**: tournament verdict merge thành công → resume run gốc tại node validate/reviewer qua `applyFlowControl` hiện có (không tạo engine mới); hòa + user từ chối → trả về escalate flow cũ. Wire qua cơ chế flow-control hiện có (không tạo engine mới).
- `T-5` **`internal/runner/tournament_escalation_test.go`** (new, không sửa test cũ): 2 test signatures dưới đây (flag on + off). Retry back-edge tests thuộc Task-370 — không nhân đôi ở đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/agent_orchestrator.go` (modified)
  - `apps/local-runner/internal/runner/interactive_service.go` (modified)
  - `apps/local-runner/internal/runner/cohort_stall.go` (modified)
  - `apps/local-runner/internal/runner/vibe_debate.go` (modified)
  - `apps/local-runner/internal/runner/flow_step_runtime.go` (modified — LoopStatusTournamentEscalation const)
  - `apps/local-runner/internal/runner/tournament_escalation_test.go` (new)
- modules: `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Flag bật, review loop đạt cap → run chuyển `tournament_escalation`, tournament-harness được dispatch với đúng intent/contract của run kẹt.
- [x] AC-2: Flag bật, owner debate stall → cùng nhánh escalation (không parked-cap terminate).
- [x] AC-3: Tournament merge thành công → run gốc resume validate/reviewer với code của winner; hòa + user từ chối → run gốc về escalate cũ (fail-closed).
- [x] AC-4: Flag unset → hành vi byte-identical cũ (escalate card, parked cap) — không tournament, không state mới.
- [x] AC-5: Không đệ quy: tournament flow chạm cap → escalate như cũ, không tự escalate thêm lần nữa.
- [x] AC-6: 2 test signatures green:
  - `TestReviewLoopTriggersTournamentOnCapExceeded`
  - `TestVibeDebateTriggersTournamentOnStall`
- [x] AC-7: Attempt được theo dõi từ escalation (`attempt: 1`) qua các vòng retry (tối đa `max_attempts`); hết lượt hoặc merge-conflict → `ask_user`, không retry thêm, không đệ quy tournament-con.

## 7. Out of Scope

- Scoring engine + worktree (P-1/P-2 / Task-368/369).
- Flow YAML + behaviors tournament (P-3 / Task-370).
- E2E tournament verify (P-5 / Task-372).
- Đổi semantics cap/extend cho flow thường khi flag tắt.

## 8. Completion Notes

- result: done 2026-09-16 — CA-879; 6/6 new tests green; 85/85 related old tests green; flag-off fallback proven by full legacy suite; deviations (no frozen param, untracked child, state-level return) documented.
- follow-ups: P-5 E2E; live re-entry via normal Continue; UI tree visibility for untracked tournament child.
- upstream docs updated:
