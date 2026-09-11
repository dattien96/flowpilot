# Task-335: Xây dựng Bộ phát hiện lệch hướng Drift Detector và Thang ứng phó

## Metadata

- Document ID: `Task-335`
- Title: `Xây dựng Bộ phát hiện lệch hướng Drift Detector và Thang ứng phó`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-23: Bộ trí tuệ vận hành tích hợp](../../07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [Task-334: Context Resolver và Budget Packer](../done/Task-334-Context-Resolver-And-Budget-Packer.md), [Task-336: Thăng cấp bài học thành Skill](./Task-336-Mistake-To-Skill-Promotion-And-Skillpack-Sync.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Replaces: `None`
- Tags: `runtime-intelligence, drift-detector, wrong-way, correction-ladder, r-scope-reuse`

## AI Quick View

### Summary

- Hiện thực hóa **Phase 2 của CP-23**: Xây dựng bộ chẩn đoán và tổng hợp telemetry (`DriftDetector`) nhằm phát hiện sớm các biểu hiện AI đi chệch hướng, sa lầy vào vòng lặp hoặc sửa sai phạm vi.
- **Tái sử dụng 100% các Gate hiện có**: Không viết lại logic kiểm tra diff hay check contract; bộ phát hiện đăng ký thu nạp trực tiếp tín hiệu từ cổng `r-scope` (`ScopeOutOfScopePaths`) và `r-contract` đã có sẵn.
- **Tập tín hiệu Heuristics (0 token cost)**:
  - Cụm từ xin lỗi / filler words lặp lại nhiều lần.
  - Lặp lại cùng một lỗi test suite $\ge 2$ lần mà không thay đổi mã nguồn logic.
  - Tiêu tốn nhiều token nhưng không sinh bất kỳ delta thay đổi nào trên file code/artifact.
  - Sửa file ngoài declared scope (từ tín hiệu của `r-scope`).
- **Thang điểm `drift_score` (0 - 100)** và **Thang ứng phó (Correction Ladder)**:
  - `30 - 59`: Bơm system note nhắc nhở đổi cách tiếp cận.
  - `60 - 79`: Thu hẹp scope và gọi lại với tập context nhỏ gọn hơn.
  - `80+`: **Tạm dừng và yêu cầu con người can thiệp (Pause for human)**. Tuyệt đối không tự động rollback thô bạo.

### Current Ask

- Xây dựng module `apps/local-runner/internal/driftdetect/` gồm `detector.go`, `signals.go`, `ladder.go` và bộ test `driftdetect_test.go`.

### Key Decisions

- `T-1` **Đóng vai trò Telemetry Aggregator**: `DriftDetector` không cạnh tranh hay thay thế các flow-gate hiện có, mà gom các tín hiệu vi phạm gate cùng hành vi hội thoại để tính toán bức tranh sức khỏe toàn diện của phiên làm việc.
- `T-2` **Phân bổ trọng số điểm Drift**:
  - Vi phạm scope ngoài hợp đồng (`r-scope`): +35 điểm.
  - Chạy lại test fail cùng lỗi lần 2+: +30 điểm.
  - Vòng lặp xin lỗi / filler words: +25 điểm.
  - Tốn token nhưng không có file delta: +20 điểm.
- `T-3` **An toàn tối đa với Thang ứng phó**: Không tự động revert mã nguồn mà dùng cơ chế bậc thang từ cảnh báo mềm đến dừng tương tác hỏi ý kiến người dùng.

### Constraints

- Bộ tính điểm heuristics phải chạy cực nhanh, tính toán ngay sau mỗi turn.
- Lưu trữ lịch sử drift event phục vụ chuyển giao sang Phase 3 (`Task-336`).

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md` (Phase 2).
- `apps/local-runner/internal/flowgate/evaluate.go` (Nguồn tín hiệu `ScopeOutOfScopePaths` của `r-scope`).

### Open Questions

- Đã giải quyết: Vòng lặp xin lỗi cần ≥ 2 lượt mới tính, 1 lần xin lỗi đơn lẻ là bình thường.
- Đã giải quyết: Lặp test fail phải là cùng tên test, hai test khác nhau fail không tính loop.

---

## 1. Goal

Chấm dứt tình trạng AI chạy luẩn quẩn, lãng phí token và thời gian của người dùng bằng một bộ phát hiện lệch hướng xác định và thang ứng phó an toàn, bảo vệ tiến độ dự án.

---

## 2. Parent Links

- Coding Plan: [CP-23 Phase 2](../../07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md).

---

## 3. Trigger

Trong các phiên làm việc dài hoặc khi gặp một bug khó, model thường có xu hướng xin lỗi liên tục (*"Tôi rất xin lỗi vì nhầm lẫn trước..."*) và thử đi thử lại cùng một câu lệnh sai, tiêu tốn hàng chục ngàn token mà không tạo ra tiến triển.

---

## 4. Exact Change

- `T-1` Tạo package `internal/driftdetect/` với struct `DriftEvent` và `DriftReport`.
- `T-2` Triển khai bộ phân tích tín hiệu hội thoại (`CheckApologyLoop`, `CheckTestFailureLoop`, `CheckZeroDeltaProgress`).
- `T-3` Kết nối lắng nghe kết quả `TurnResult` (đặc biệt là `tr.ScopeOutOfScopePaths` từ `r-scope`).
- `T-4` Triển khai `EvaluateDriftScore` và bộ điều hướng `ApplyCorrectionLadder`.

---

## 5. Touched Areas

- `apps/local-runner/internal/driftdetect/detector.go` (Mới)
- `apps/local-runner/internal/driftdetect/signals.go` (Mới)
- `apps/local-runner/internal/driftdetect/ladder.go` (Mới)
- `apps/local-runner/internal/driftdetect/driftdetect_test.go` (Mới)
- `apps/local-runner/internal/runner/gate_hook.go` (Ghi nhận drift event sau mỗi turn)

---

## 6. Acceptance Check

- Chạy `go test ./internal/driftdetect/ -run TestDriftDetector` pass 100%.
- Khi AI xin lỗi 2 turn liên tiếp $\rightarrow$ Điểm drift tăng, hệ thống bơm note cảnh báo vào prompt kế tiếp.
- Khi AI vi phạm `r-scope` kết hợp test fail $\rightarrow$ Điểm drift $\ge 60$, kích hoạt nấc thu hẹp context hoặc tạm dừng hỏi người dùng.

---

## 7. Out of Scope

- Tự động sinh Skill từ lỗi lặp lại (thuộc `Task-336`).

---

## 8. Completion Notes

- Trạng thái: `draft` (chờ triển khai).

---

## 9. Definition of Done

- [ ] Struct `DriftEvent` lưu trữ đầy đủ: `RunID`, `StepID`, `DriftScore`, `TriggeredSignals`, `CorrectionAction`.
- [ ] Heuristic phát hiện chính xác vòng lặp xin lỗi / filler phrases mà không bắt nhầm câu trả lời thông thường.
- [ ] Tích hợp trích xuất tín hiệu từ `r-scope` mà không tạo ra code kiểm tra diff trùng lặp.
- [ ] Thang ứng phó hoạt động theo đúng ngưỡng: `30-59` bơm system note, `60-79` thu hẹp context, `80+` dừng hỏi người dùng.
- [ ] Chạy `go test ./internal/driftdetect/...` pass 100%.
- [ ] Lưu trữ lịch sử `DriftEvent` dạng JSON phục vụ chuyển giao sang Phase 3 (`Task-336`).
- [ ] Tích hợp ghi nhận drift event trong `gate_hook.go` sau mỗi turn hoàn thành.
- [ ] Điểm drift giảm dần (decay) khi AI sửa chữa thành công, không cộng dồn vô hạn.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/driftdetect/driftdetect_test.go`

```go
package driftdetect

import "testing"

// Scenario: Phiên làm việc bình thường, AI làm đúng scope và không có hành vi lặp
// Input: FinalMessage="Đã hoàn thành cập nhật hàm X", ScopeOutOfScopePaths=[]
// Expect: DriftScore < 30, Action="none"
func TestDriftDetector_HealthyTurn_NoDrift(t *testing.T) {}

// Scenario: Phát hiện vòng lặp xin lỗi nhiều lượt liên tiếp
// Input: 2 lượt liên tiếp chứa các câu "Tôi rất xin lỗi, tôi sẽ thử lại cách khác"
// Expect: Signal="apology_loop", DriftScore >= 30, Action="inject_system_note"
func TestDriftDetector_ApologyLoop_InjectsNote(t *testing.T) {}

// Scenario: Tái sử dụng tín hiệu r-scope kết hợp test lặp lỗi
// Input: ScopeOutOfScopePaths=["config/secret.go"], TestFailedCount >= 2 cùng tên test
// Expect: DriftScore >= 65, Action="narrow_context" hoặc "pause_for_human"
func TestDriftDetector_ScopeViolationAndTestLoop_EscalatesLadder(t *testing.T) {}

// Scenario: Điểm drift nghiêm trọng (>= 80) -> Dừng tiến trình hỏi người dùng
// Input: Tổng hợp 3 tín hiệu tiêu cực cùng lúc
// Expect: Action="pause_for_human", trạng thái workflow chuyển sang chờ can thiệp
func TestDriftDetector_CriticalDrift_PausesForHuman(t *testing.T) {}

// [Edge] Scenario: Chỉ 1 lần xin lỗi -> Chưa đủ kích hoạt apology_loop
// Input: 1 lượt duy nhất chứa "Tôi xin lỗi"
// Expect: DriftScore < 30, không có signal "apology_loop"
func TestDriftDetector_SingleApology_NoTrigger(t *testing.T) {}

// [Edge] Scenario: Hai test khác nhau fail -> Không phải loop
// Input: Turn 1 fail "TestA", Turn 2 fail "TestB" (tên test khác nhau)
// Expect: Không có signal "repeated_test_failure"
func TestDriftDetector_DifferentTestFailures_NoLoop(t *testing.T) {}

// [Edge] Scenario: Điểm drift reset sau correction thành công
// Input: Score trước = 50, lượt hiện tại AI thay đổi chiến lược và tạo delta
// Expect: DriftScore giảm xuống (không cộng dồn vô hạn)
func TestDriftDetector_SuccessfulCorrection_ScoreDecays(t *testing.T) {}

// [Error] Scenario: Lượt đầu tiên, không có lịch sử
// Input: history=[], current là turn đầu tiên
// Expect: DriftScore = 0, Action = "none", không panic
func TestDriftDetector_EmptyHistory_NoError(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/driftdetect/detector.go`:

```go
package driftdetect

type CorrectionAction string

const (
	ActionNone             CorrectionAction = "none"
	ActionInjectSystemNote CorrectionAction = "inject_system_note"
	ActionNarrowContext    CorrectionAction = "narrow_context"
	ActionPauseForHuman    CorrectionAction = "pause_for_human"
)

type DriftEvent struct {
	RunID            string           `json:"run_id"`
	TurnID           string           `json:"turn_id"`
	DriftScore       int              `json:"drift_score"` // 0 - 100
	TriggeredSignals []string         `json:"triggered_signals"`
	CorrectionAction CorrectionAction `json:"correction_action"`
	SystemNotePrompt string           `json:"system_note_prompt,omitempty"`
}

// EvaluateTurnDrift đánh giá mức độ lệch hướng sau mỗi turn hoàn thành.
func EvaluateTurnDrift(history []TurnSummary, current TurnSummary) DriftEvent {
	score := 0
	signals := make([]string, 0)

	// 1. Kiểm tra tín hiệu ngoài scope từ r-scope đã có sẵn
	if len(current.ScopeOutOfScopePaths) > 0 {
		score += 35
		signals = append(signals, "out_of_scope_edit")
	}

	// 2. Kiểm tra vòng lặp test fail lặp lại
	if checkRepeatedTestFailures(history, current) {
		score += 30
		signals = append(signals, "repeated_test_failure")
	}

	// 3. Kiểm tra cụm từ xin lỗi / lặp vòng
	if checkApologyPatterns(current.FinalMessage) {
		score += 25
		signals = append(signals, "apology_loop")
	}

	// 4. Xác định nấc thang ứng phó phù hợp
	action := resolveCorrectionAction(score)

	return DriftEvent{
		DriftScore:       score,
		TriggeredSignals: signals,
		CorrectionAction: action,
	}
}
```

Định nghĩa struct đầu vào trong `apps/local-runner/internal/driftdetect/detector.go`:

```go
// TurnSummary chứa thông tin tóm tắt của một lượt chạy AI phục vụ đánh giá drift.
type TurnSummary struct {
	TurnID                string   `json:"turn_id"`
	FinalMessage          string   `json:"final_message"`
	TokensConsumed        int      `json:"tokens_consumed"`
	FilesChanged          []string `json:"files_changed"`
	TestResults           []string `json:"test_results"` // Danh sách tên test fail
	ScopeOutOfScopePaths  []string `json:"scope_out_of_scope_paths"`
}
```

Chữ ký trong `apps/local-runner/internal/driftdetect/signals.go`:

```go
package driftdetect

// checkRepeatedTestFailures kiểm tra xem cùng một test có fail >= 2 lần liên tiếp.
func checkRepeatedTestFailures(history []TurnSummary, current TurnSummary) bool {
	// 1. Lấy danh sách test fail trong lượt hiện tại
	// 2. So sánh với lượt trước: nếu có cùng tên test fail >= 2 lần -> true
	return false
}

// checkApologyPatterns phát hiện cụm từ xin lỗi / filler words lặp lại.
func checkApologyPatterns(message string) bool {
	// 1. Danh sách patterns: "Tôi rất xin lỗi", "Bạn hoàn toàn đúng",
	//    "I apologize", "Let me try again", ...
	// 2. Đếm match trong message
	return false
}

// checkZeroDeltaProgress kiểm tra lượt chạy tốn token nhưng không tạo ra delta code.
func checkZeroDeltaProgress(current TurnSummary) bool {
	// true khi: TokensConsumed > 2000 && len(FilesChanged) == 0
	return false
}
```

Chữ ký trong `apps/local-runner/internal/driftdetect/ladder.go`:

```go
package driftdetect

// resolveCorrectionAction xác định nấc thang ứng phó dựa trên drift score.
func resolveCorrectionAction(score int) CorrectionAction {
	// score < 30  -> ActionNone
	// 30 <= score < 60 -> ActionInjectSystemNote
	// 60 <= score < 80 -> ActionNarrowContext
	// score >= 80 -> ActionPauseForHuman
	return ActionNone
}

// GenerateSystemNote tạo nội dung system note cảnh báo AI đổi chiến lược.
func GenerateSystemNote(event DriftEvent) string {
	// Sinh message cảnh báo dựa trên triggered signals
	// Ví dụ: "⚠️ Phát hiện lặp test fail 3 lần liên tiếp. Hãy thay đổi chiến lược..."
	return ""
}
```
