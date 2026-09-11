package driftdetect

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// hasSignalName is the test-side signal membership helper.
func hasSignalName(t *testing.T, signals []string, name string) bool {
	t.Helper()
	for _, s := range signals {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

// Scenario: Phiên làm việc bình thường, AI làm đúng scope và không có hành vi lặp
// Input: FinalMessage="Đã hoàn thành cập nhật hàm X", ScopeOutOfScopePaths=[]
// Expect: DriftScore < 30, Action="none"
func TestDriftDetector_HealthyTurn_NoDrift(t *testing.T) {
	history := []TurnSummary{
		{
			TurnID:         "turn-1",
			FinalMessage:   "Đã cập nhật xong phần xử lý đầu vào.",
			TokensConsumed: 1200,
			FilesChanged:   []string{"internal/calc/calc.go"},
		},
	}
	current := TurnSummary{
		TurnID:         "turn-2",
		FinalMessage:   "Đã hoàn thành cập nhật hàm X",
		TokensConsumed: 900,
		FilesChanged:   []string{"internal/calc/calc.go"},
	}
	event := EvaluateTurnDrift(history, current)

	if event.DriftScore >= 30 {
		t.Fatalf("healthy turn must score < 30, got %d (signals=%v)", event.DriftScore, event.TriggeredSignals)
	}
	if event.CorrectionAction != ActionNone {
		t.Fatalf("healthy turn must resolve ActionNone, got %q", event.CorrectionAction)
	}
	if len(event.TriggeredSignals) != 0 {
		t.Fatalf("healthy turn must trigger no signals, got %v", event.TriggeredSignals)
	}
	if event.SystemNotePrompt != "" {
		t.Fatalf("ActionNone must not carry a system note, got %q", event.SystemNotePrompt)
	}
}

// Scenario: Phát hiện vòng lặp xin lỗi nhiều lượt liên tiếp
// Input: 2 lượt liên tiếp chứa các câu "Tôi rất xin lỗi, tôi sẽ thử lại cách khác"
// Expect: Signal="apology_loop", DriftScore >= 30, Action="inject_system_note"
func TestDriftDetector_ApologyLoop_InjectsNote(t *testing.T) {
	msg := "Tôi rất xin lỗi, tôi sẽ thử lại cách khác."
	history := []TurnSummary{
		{TurnID: "turn-1", FinalMessage: msg, TokensConsumed: 1500}, // single apology: cheap turn, no signal yet
	}
	// The loop turn also burns tokens with zero file delta — the canonical
	// Task-335 §3 wrong-way signature (apologize + retry + no progress).
	current := TurnSummary{TurnID: "turn-2", FinalMessage: msg, TokensConsumed: 3000}
	event := EvaluateTurnDrift(history, current)

	if !hasSignalName(t, event.TriggeredSignals, SignalApologyLoop) {
		t.Fatalf("2 consecutive apology turns must trigger %q, got %v", SignalApologyLoop, event.TriggeredSignals)
	}
	if event.DriftScore < 30 {
		t.Fatalf("apology loop must score >= 30, got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionInjectSystemNote {
		t.Fatalf("apology loop must inject a system note, got %q", event.CorrectionAction)
	}
	if !strings.Contains(event.SystemNotePrompt, SignalApologyLoop) {
		t.Fatalf("system note must cite the apology_loop signal, got %q", event.SystemNotePrompt)
	}
}

// Scenario: Tái sử dụng tín hiệu r-scope kết hợp test lặp lỗi
// Input: ScopeOutOfScopePaths=["config/secret.go"], TestFailedCount >= 2 cùng tên test
// Expect: DriftScore >= 65, Action="narrow_context" hoặc "pause_for_human"
func TestDriftDetector_ScopeViolationAndTestLoop_EscalatesLadder(t *testing.T) {
	history := []TurnSummary{
		{
			TurnID:         "turn-1",
			FinalMessage:   "Đang sửa lỗi test.",
			TokensConsumed: 800,
			FilesChanged:   []string{"internal/calc/calc.go"},
			TestResults:    []string{"TestCalc_Add_Fails"},
		},
	}
	current := TurnSummary{
		TurnID:               "turn-2",
		FinalMessage:         "Đã thử sửa lại nhưng test vẫn fail.",
		TokensConsumed:       900,
		FilesChanged:         []string{"config/secret.go"},
		TestResults:          []string{"TestCalc_Add_Fails"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
	}
	event := EvaluateTurnDrift(history, current)

	if event.DriftScore < 65 {
		t.Fatalf("scope violation + repeated test failure must score >= 65 (35+30), got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionNarrowContext && event.CorrectionAction != ActionPauseForHuman {
		t.Fatalf("must escalate to narrow_context or pause_for_human, got %q", event.CorrectionAction)
	}
	if !hasSignalName(t, event.TriggeredSignals, SignalOutOfScopeEdit) ||
		!hasSignalName(t, event.TriggeredSignals, SignalRepeatedTestFailure) {
		t.Fatalf("must trigger both r-scope and repeated test signals, got %v", event.TriggeredSignals)
	}
}

// Scenario: Điểm drift nghiêm trọng (>= 80) -> Dừng tiến trình hỏi người dùng
// Input: Tổng hợp 3 tín hiệu tiêu cực cùng lúc
// Expect: Action="pause_for_human", trạng thái workflow chuyển sang chờ can thiệp
func TestDriftDetector_CriticalDrift_PausesForHuman(t *testing.T) {
	msg := "Bạn hoàn toàn đúng, để tôi thử lại."
	history := []TurnSummary{
		{
			TurnID:         "turn-1",
			FinalMessage:   msg,
			TokensConsumed: 500,
			FilesChanged:   []string{"internal/calc/calc.go"},
			TestResults:    []string{"TestCalc_Add_Fails"},
		},
	}
	current := TurnSummary{
		TurnID:               "turn-2",
		FinalMessage:         msg, // apology loop (turn-1 + turn-2)
		TokensConsumed:       400,
		FilesChanged:         []string{"config/secret.go"},
		TestResults:          []string{"TestCalc_Add_Fails"}, // repeated test failure
		ScopeOutOfScopePaths: []string{"config/secret.go"},   // out-of-scope edit
	}
	event := EvaluateTurnDrift(history, current)

	// 3 negative signals together: 35 (scope) + 30 (repeated test) + 25 (apology) = 90.
	if event.DriftScore < 80 {
		t.Fatalf("3 combined signals must score >= 80, got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionPauseForHuman {
		t.Fatalf("critical drift must pause for human, got %q", event.CorrectionAction)
	}
}

// [Edge] Scenario: Chỉ 1 lần xin lỗi -> Chưa đủ kích hoạt apology_loop
// Input: 1 lượt duy nhất chứa "Tôi xin lỗi"
// Expect: DriftScore < 30, không có signal "apology_loop"
func TestDriftDetector_SingleApology_NoTrigger(t *testing.T) {
	history := []TurnSummary{
		{
			TurnID:         "turn-1",
			FinalMessage:   "Đã kiểm tra xong ca sử dụng đăng nhập.",
			TokensConsumed: 700,
			FilesChanged:   []string{"internal/auth/auth.go"},
		},
	}
	current := TurnSummary{
		TurnID:         "turn-2",
		FinalMessage:   "Tôi xin lỗi, tôi sẽ kiểm tra lại logic này.",
		TokensConsumed: 600,
		FilesChanged:   []string{"internal/auth/auth.go"},
	}
	event := EvaluateTurnDrift(history, current)

	if hasSignalName(t, event.TriggeredSignals, SignalApologyLoop) {
		t.Fatalf("a single apology must NOT trigger apology_loop, got %v", event.TriggeredSignals)
	}
	if event.DriftScore >= 30 {
		t.Fatalf("single apology must stay < 30, got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionNone {
		t.Fatalf("single apology must resolve ActionNone, got %q", event.CorrectionAction)
	}
}

// [Edge] Scenario: Hai test khác nhau fail -> Không phải loop
// Input: Turn 1 fail "TestA", Turn 2 fail "TestB" (tên test khác nhau)
// Expect: Không có signal "repeated_test_failure"
func TestDriftDetector_DifferentTestFailures_NoLoop(t *testing.T) {
	history := []TurnSummary{
		{
			TurnID:         "turn-1",
			FinalMessage:   "Sửa lỗi biên dịch.",
			TokensConsumed: 800,
			FilesChanged:   []string{"internal/calc/calc.go"},
			TestResults:    []string{"TestA"},
		},
	}
	current := TurnSummary{
		TurnID:         "turn-2",
		FinalMessage:   "Sửa tiếp lỗi khác.",
		TokensConsumed: 800,
		FilesChanged:   []string{"internal/calc/parse.go"},
		TestResults:    []string{"TestB"},
	}
	event := EvaluateTurnDrift(history, current)

	if hasSignalName(t, event.TriggeredSignals, SignalRepeatedTestFailure) {
		t.Fatalf("two DIFFERENT failing tests must not count as a loop, got %v", event.TriggeredSignals)
	}
	if event.DriftScore != 0 {
		t.Fatalf("different test failures alone must not score, got %d", event.DriftScore)
	}
}

// [Edge] Scenario: Điểm drift reset sau correction thành công
// Input: Score trước = 50, lượt hiện tại AI thay đổi chiến lược và tạo delta
// Expect: DriftScore giảm xuống (không cộng dồn vô hạn)
func TestDriftDetector_SuccessfulCorrection_ScoreDecays(t *testing.T) {
	history := []TurnSummary{
		{
			// turn-1: out-of-scope edit (+35) with the failing test on record.
			TurnID:               "turn-1",
			FinalMessage:         "Mở rộng phạm vi để sửa nhanh.",
			TokensConsumed:       500,
			FilesChanged:         []string{"config/secret.go"},
			TestResults:          []string{"TestCalc_Add_Fails"},
			ScopeOutOfScopePaths: []string{"config/secret.go"},
		},
		{
			// turn-2: same test fails again (+30) → running score 65.
			TurnID:         "turn-2",
			FinalMessage:   "Vẫn chưa xong.",
			TokensConsumed: 500,
			FilesChanged:   []string{"config/secret.go"},
			TestResults:    []string{"TestCalc_Add_Fails"},
		},
	}
	peak := EvaluateTurnDrift(history[:1], history[1])
	if peak.DriftScore != 65 {
		t.Fatalf("setup: score after the two drift turns must be 65, got %d", peak.DriftScore)
	}

	// Current turn: the AI changed strategy and produced a concrete in-scope
	// delta with no negative signal → the carried score must decay.
	current := TurnSummary{
		TurnID:         "turn-3",
		FinalMessage:   "Đã đổi chiến lược: cô lập phụ thuộc và sửa trong phạm vi khai báo.",
		TokensConsumed: 800,
		FilesChanged:   []string{"internal/calc/calc.go"},
	}
	event := EvaluateTurnDrift(history, current)
	if event.DriftScore >= peak.DriftScore {
		t.Fatalf("drift score must decay after a successful correction: got %d, want < %d", event.DriftScore, peak.DriftScore)
	}
	if len(event.TriggeredSignals) != 0 {
		t.Fatalf("corrected turn must trigger no signals, got %v", event.TriggeredSignals)
	}

	// A second healthy correction decays again — no infinite accumulation.
	nextHistory := append(append([]TurnSummary(nil), history...), current)
	followUp := TurnSummary{
		TurnID:         "turn-4",
		FinalMessage:   "Hoàn tất sửa trong phạm vi, test đã xanh.",
		TokensConsumed: 700,
		FilesChanged:   []string{"internal/calc/calc.go"},
	}
	next := EvaluateTurnDrift(nextHistory, followUp)
	if next.DriftScore >= event.DriftScore {
		t.Fatalf("score must keep decaying on sustained corrections: got %d, want < %d", next.DriftScore, event.DriftScore)
	}
}

// [Error] Scenario: Lượt đầu tiên, không có lịch sử
// Input: history=[], current là turn đầu tiên
// Expect: DriftScore = 0, Action = "none", không panic
func TestDriftDetector_EmptyHistory_NoError(t *testing.T) {
	current := TurnSummary{
		TurnID:         "turn-1",
		FinalMessage:   "Bắt đầu phân tích yêu cầu.",
		TokensConsumed: 300,
	}
	var history []TurnSummary
	event := EvaluateTurnDrift(history, current)

	if event.DriftScore != 0 {
		t.Fatalf("first turn with empty history must score 0, got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionNone {
		t.Fatalf("first turn must resolve ActionNone, got %q", event.CorrectionAction)
	}
	if event.TurnID != "turn-1" {
		t.Fatalf("event must carry the current TurnID, got %q", event.TurnID)
	}
	if len(event.TriggeredSignals) != 0 {
		t.Fatalf("first turn must trigger no signals, got %v", event.TriggeredSignals)
	}
}

// [Extra] All four signals at once cap the score at 100 (Task-335: 0-100).
func TestDriftDetector_AllSignals_ScoreCappedAt100(t *testing.T) {
	msg := "Tôi rất xin lỗi, để tôi thử lại."
	history := []TurnSummary{
		{TurnID: "turn-1", FinalMessage: msg, TokensConsumed: 3000, TestResults: []string{"TestA"}},
	}
	current := TurnSummary{
		TurnID:               "turn-2",
		FinalMessage:         msg,
		TokensConsumed:       3000,
		TestResults:          []string{"TestA"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
	}
	event := EvaluateTurnDrift(history, current)

	// Raw 35+30+25+20 = 110 → capped at 100.
	if event.DriftScore != 100 {
		t.Fatalf("score must cap at 100, got %d", event.DriftScore)
	}
	if event.CorrectionAction != ActionPauseForHuman {
		t.Fatalf("capped score must pause for human, got %q", event.CorrectionAction)
	}
}

// [Extra] GenerateSystemNote is deterministic and cites every triggered signal.
func TestGenerateSystemNote_CitesTriggeredSignals(t *testing.T) {
	event := DriftEvent{
		RunID:            "run-335",
		TurnID:           "turn-7",
		DriftScore:       65,
		TriggeredSignals: []string{SignalOutOfScopeEdit, SignalRepeatedTestFailure},
		CorrectionAction: ActionNarrowContext,
	}
	note := GenerateSystemNote(event)
	if note == "" {
		t.Fatalf("system note must not be empty")
	}
	again := GenerateSystemNote(event)
	if note != again {
		t.Fatalf("GenerateSystemNote must be deterministic")
	}
	if !strings.Contains(note, "drift_score=65/100") {
		t.Fatalf("note must cite the score, got %q", note)
	}
	for _, name := range event.TriggeredSignals {
		if !strings.Contains(note, name) {
			t.Fatalf("note must cite signal %q, got %q", name, note)
		}
	}
	if strings.Contains(note, SignalApologyLoop) {
		t.Fatalf("note must not cite untriggered signals, got %q", note)
	}
}

// [Extra] DriftEvent JSON shape matches the Task-335 Code Guide json tags and
// round-trips (the persisted JSONL is the Task-336 handoff artifact).
func TestDriftEvent_JSONTagsRoundTrip(t *testing.T) {
	event := DriftEvent{
		RunID:            "run-335",
		TurnID:           "turn-2",
		DriftScore:       45,
		TriggeredSignals: []string{SignalApologyLoop, SignalZeroDeltaProgress},
		CorrectionAction: ActionInjectSystemNote,
		SystemNotePrompt: "change strategy",
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, tag := range []string{`"run_id":`, `"turn_id":`, `"drift_score":`, `"triggered_signals":`, `"correction_action":`, `"system_note_prompt":`} {
		if !strings.Contains(string(raw), tag) {
			t.Fatalf("json must contain %s, got %s", tag, raw)
		}
	}
	var back DriftEvent
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, event) {
		t.Fatalf("round-trip mismatch: %+v != %+v", back, event)
	}

	// Empty SystemNotePrompt is omitempty (Code Guide).
	empty := DriftEvent{RunID: "r", TurnID: "t", DriftScore: 0, CorrectionAction: ActionNone}
	rawEmpty, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if strings.Contains(string(rawEmpty), "system_note_prompt") {
		t.Fatalf("empty system_note_prompt must be omitted, got %s", rawEmpty)
	}
}

// Scenario: Ladder boundaries land on the exact thresholds from CP-23 Phase 2
// Input: scores 29, 30, 59, 60, 79, 80
// Expect: 29 -> none; 30/59 -> inject_system_note; 60/79 -> narrow_context; 80 -> pause_for_human
func TestResolveCorrectionAction_ExactThresholds(t *testing.T) {
	cases := map[int]CorrectionAction{
		29: ActionNone,
		30: ActionInjectSystemNote,
		59: ActionInjectSystemNote,
		60: ActionNarrowContext,
		79: ActionNarrowContext,
		80: ActionPauseForHuman,
	}
	for score, want := range cases {
		if got := resolveCorrectionAction(score); got != want {
			t.Fatalf("resolveCorrectionAction(%d) = %q, want %q", score, got, want)
		}
	}
}
