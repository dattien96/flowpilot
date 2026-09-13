package driftdetect

import (
	"fmt"
	"strings"
)

// Task-335 §4 T-4 / §11: the Correction Ladder. Thresholds per CP-23 Phase 2:
//
//	 0 - 29  ActionNone              (healthy)
//	30 - 59  ActionInjectSystemNote (soft warning note in the next prompt)
//	60 - 79  ActionNarrowContext    (tightened context pack for the next turn)
//	   80+   ActionPauseForHuman    (pause, wait for human — never rollback)

// resolveCorrectionAction determines the ladder step for a drift score
// (Task-335 §11 Code Guide).
func resolveCorrectionAction(score int) CorrectionAction {
	switch {
	case score >= 80:
		return ActionPauseForHuman
	case score >= 60:
		return ActionNarrowContext
	case score >= 30:
		return ActionInjectSystemNote
	default:
		return ActionNone
	}
}

// signalAdvisories maps each triggered signal to its deterministic
// Vietnamese/English advisory line (cited verbatim in the system note).
var signalAdvisories = map[string]string{
	SignalOutOfScopeEdit: "- out_of_scope_edit: Bạn vừa sửa file ngoài phạm vi đã cam kết. Hãy quay lại đúng phạm vi khai báo. " +
		"(You edited files outside the declared scope — return to the committed scope.)",
	SignalRepeatedTestFailure: "- repeated_test_failure: Cùng một test đã fail lặp lại >= 2 lượt liên tiếp. Hãy đọc kỹ thông báo lỗi và ĐỔI cách sửa, không lặp lại chỉnh sửa cũ. " +
		"(The same test failed repeatedly — study the failure and change the fix strategy.)",
	SignalApologyLoop: "- apology_loop: Bạn đang lặp lại cụm từ xin lỗi/điền từ qua nhiều lượt. Thay vì xin lỗi, hãy phân tích nguyên nhân gốc và đổi hướng tiếp cận. " +
		"(You are repeating apology phrases across turns — analyze the root cause and change approach instead of apologizing.)",
	SignalZeroDeltaProgress: "- zero_delta_progress: Lượt này tiêu tốn nhiều token nhưng không thay đổi file nào. Hãy thực hiện một thay đổi cụ thể hoặc trình bày kế hoạch rõ ràng. " +
		"(High token spend with zero file changes — make a concrete change or present a clear plan.)",
}

// signalAdvisoryOrder is the deterministic citation order (weight order,
// heaviest first) used by GenerateSystemNote regardless of the order signals
// were appended.
var signalAdvisoryOrder = []string{
	SignalOutOfScopeEdit,
	SignalRepeatedTestFailure,
	SignalApologyLoop,
	SignalZeroDeltaProgress,
}

// GenerateSystemNote builds the deterministic warning injected into the next
// prompt when the ladder resolves to inject_system_note (Task-335 §11). The
// note cites every triggered signal with its advisory so the AI knows exactly
// which behaviour to change. Pure function: same event → same bytes.
func GenerateSystemNote(event DriftEvent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[FlowPilot Drift Detector] Cảnh báo lệch hướng — drift_score=%d/100. Drift warning — drift score %d/100.\n",
		event.DriftScore, event.DriftScore)

	// Deterministic citation: canonical weight order first, then any unknown
	// signals in their received order with a generic advisory.
	cited := make(map[string]bool, len(event.TriggeredSignals))
	for _, name := range signalAdvisoryOrder {
		if hasSignal(event.TriggeredSignals, name) {
			sb.WriteString(signalAdvisories[name] + "\n")
			cited[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	for _, name := range event.TriggeredSignals {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || cited[key] {
			continue
		}
		cited[key] = true
		fmt.Fprintf(&sb, "- %s: Tín hiệu lệch hướng bất thường — hãy xem xét lại hướng làm hiện tại. (Unusual drift signal — re-evaluate the current approach.)\n", name)
	}

	sb.WriteString("Hãy THAY ĐỔI chiến lược tiếp cận ngay ở lượt kế tiếp; không lặp lại cùng một hành động. " +
		"CHANGE your approach in the next turn — do not repeat the same action.")
	return sb.String()
}
