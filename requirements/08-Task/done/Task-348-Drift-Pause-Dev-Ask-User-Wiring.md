# Task-348: Drift Pause Wiring — Dev Mode Hỏi User Thật (80+)

## Metadata

- Document ID: `Task-348`
- Title: `Drift Pause Wiring — Dev Mode Drift ≥80 Hỏi User Thật, Vibe Mode Về Owner Debate`
- Feature Keys: `zcode-parity, drift-pause`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-13`
- Last Updated: `2026-09-13`
- Parent Documents: [CP-62: Zcode Harness Parity](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-23: Auto-Learn-To-Skill](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md), [Task-337: Gate Precedence Contract](./Task-337-Gate-Precedence-Contract-And-Wiring.md)
- Child Documents: `None`
- Related Documents: [Task-335: Drift Resume Scoring](./Task-335-Drift-Resume-Scoring-Hardening.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- Replaces: `None`
- Tags: `zcode-parity, drift-ladder, pause-for-human, dev-mode, cp-62`

## AI Quick View

### Summary

- Thang drift CP-23: 30–59 system note, 60–79 narrow context, **80+ `ActionPauseForHuman`** — nhưng chân pause hiện chỉ **ghi nhận + log** (`gate_hook.go:3043`, deferred theo Task-335 vì chưa có confirm backend an toàn).
- Operator chốt mục tiêu 2026-09-13: **wire thật** — drift ≥80 ở **dev mode** → hỏi user (user-confirm pause qua confirm backend hiện có); ở **vibe mode** → tiếp tục bắn cho owner debate (P-1 đã làm, giữ nguyên).
- Điều kiện an toàn từ Task-335: chỉ emit khi confirm backend đã đăng ký — emit khi chưa có backend sẽ kẹt modal client trên endpoint không thể trả lời.

### Current Ask

- Operator duyệt 2026-09-13 (nhóm 5 task). Discovery: đăng ký `ssLockGate`/confirm backend ở đâu, event `EventUserConfirmRequired` phát và client trả lời qua kênh nào, khi nào backend KHÔNG có (headless).

### Key Decisions

- `D-1` Dev mode + drift ≥80 + backend đã đăng ký → emit user-confirm pause (card hỏi user), run dừng chờ trả lời; trả lời quyết định tiếp/quay về.
- `D-2` Vibe mode → KHÔNG BAO GIỜ emit pause cho drift (P-1 `applyVibeDriftOnlyResolver` / owner debate giữ nguyên — verify thứ tự chạy trước pause).
- `D-3` Backend chưa đăng ký (headless/không client) → giữ hành vi hiện tại (log-only) — không bao giờ kẹt modal.
- `D-4` Flag-gated giữ nguyên (`FLOWPILOT_ENABLE_DRIFT_DETECTOR`); không đổi ladder thresholds; không đổi precedence contract (P-1).

## 1. Goal

Đóng nốt chân 80+ của thang CP-23: ở dev mode, agent lạc đề nặng thì người dùng thực sự được hỏi (thay vì chỉ log im lặng); ở vibe mode tự trị giữ nguyên qua owner debate.

## 2. Parent Links

- CP-62 P-1 T-3/Task-335 Completion Notes (pause wiring deferred — "wiring it to a dedicated non-bypassable confirm flow is the follow-up integration").

## 3. Trigger

- Operator duyệt 2026-09-13.

## 4. Exact Change

### 4.0 Before → After

| | Before (hiện trạng trước Task-348) | After (sau Task-348) |
|---|---|---|
| **Drift ≥ 80 (dev mode)** | `ActionPauseForHuman` chỉ được **ghi nhận + log** (deferred Task-335 — sợ bắn event confirm khi chưa có backend sẽ kẹt modal); user **không bao giờ được hỏi** dù agent lạc đề nặng | Run bị **park thật**: loop status `blocked`, BlockReason `drift`, GateReason mang score + signals + step; emit event `drift_pause_required` (additive) — user thấy blocked card kèm lý do và được hỏi thật |
| **Cách resume** | Không có — không có gì để resume | Qua kênh **parked-run feedback hiện có** (`POST /agent-loop/continue`) — đúng kênh gate và decision card dùng → không có endpoint mới, không thể strand client (mối lo của Task-335 được giải trọn vẹn) |
| **Drift ≥ 80 (vibe mode)** | Log-only (và P-1 route debate ở tầng khác) | **Không bao giờ** hỏi user — `armDriftPause` guard mode ngay từ đầu, owner debate (P-1) tiếp tục sở hữu (SS-18 AC-7) |
| **Ai bị park** | n/a | Flow child drift → park **parent hub** (hub điều khiển loop); chat run thường → park chính nó; idempotent khi đã park-drift (không duplicate event) |
| **Lock order** | n/a | `armDriftPause` chạy **sau** `st.mu.Unlock()` — không bao giờ lấy `s.mu` khi đang giữ `st.mu` (lớp lỗi đã fix trong review CP-62); surface drift/gate `-race` xanh |


- `T-1` Discovery: confirm backend (`ssLockGate` registration, `EventUserConfirmRequired` lifecycle, kênh trả lời client) — chốt điểm emit an toàn.
- `T-2` `gate_hook.go` case `ActionPauseForHuman`: dev mode + backend ready → emit confirm (đóng gói drift score + signals + step để user hiểu vì sao bị hỏi); vibe mode → không emit (đường owner debate đã xử lý).
- `T-3` Xử lý câu trả lời: user xác nhận → run tiếp tục (clear pending ladder state); user từ chối/không trả lời → hành vi theo cơ chế confirm hiện có (không tự resume).
- `T-4` Tests mới: dev+80+backend → confirm emitted 1 lần/turn; vibe+80 → owner debate, không confirm; dev+80+no-backend → log-only (hiện trạng); không đụng test cũ.

## 5. Touched Areas

- `internal/runner/gate_hook.go` (case pause), điểm đăng ký backend (theo discovery), test mới `drift_pause_wiring_test.go`.

## 6. Acceptance Check

- [ ] AC-1: Dev mode, drift ≥80, backend sẵn → user nhận confirm pause đúng 1 lần cho turn đó, kèm lý do drift.
- [ ] AC-2: Vibe mode, drift ≥80 → owner debate (P-1), không có confirm nào tới user.
- [ ] AC-3: Backend chưa đăng ký → hành vi giữ nguyên (log-only), không kẹt client.
- [ ] AC-4: 30–79 ladder (note/narrow) không đổi; dev mode byte-stable trừ pause leg mới.
- [ ] AC-5: Suite xanh (trừ pre-existing), test cũ untouched.

## 7. Out of Scope

- Đổi threshold/score; pause cho lỗi gate khác (đã có Dev 1/2/3); UI mới (dùng confirm card hiện có).

## 8. Completion Notes

- Trạng thái: `done` (2026-09-13).
- Thiết kế tinh chỉnh so §4: KHÔNG tái dùng event/modal ss-lock (HandleSSLockConfirm là standardize-specific — publish docs). Thay vào đó park run bằng machinery blocked-flow có sẵn: `mutateLoop` (status blocked, BlockReason `drift`, GateReason mang score/signals/step) + `parkFlowForAwaitingUser` + emit graph + event MỚI additive `drift_pause_required`. Resume đi qua kênh parked-run feedback hiện có (`POST /agent-loop/continue`) — đúng kênh gate và decision card dùng → không thể strand client (mối lo của Task-335 được giải trọn vẹn).
- Flow child drift → park parent hub; chat run thường → park chính nó. Idempotent khi đã park-drift (BlockReason đọc từ loopStateFor).
- Lock order: `armDriftPause` chạy SAU `st.mu.Unlock()` trong gate_hook — không bao giờ lấy s.mu khi đang giữ st.mu (lớp lỗi đã fix trong review CP-62). `-race` xanh trên surface drift/gate.
- Chi tiết: CA-859 (feature key `drift-pause`).

## 9. Definition of Done

- [x] AC-1..AC-5 tick (AC-3 mở rộng: "backend chưa đăng ký" vô nghĩa với thiết kế mới — không emit event cần backend nào cả; park dùng machinery blocked-flow nên client nào cũng render được).
- [x] CA note + key `drift-pause` đăng ký (CA-859).
- [x] Commit `[Feature][zcode-parity] ... Task-348`.
