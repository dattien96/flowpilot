package runner

import (
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// Task-337 (CP-62 P-1): runner wiring of the drift-aware vibe gate classifier
// and the owner-debate context-pruning bypass (T-3).

// Scenario: Wiring runner — classifyVibeGateWithDrift nhận drift >= 80 non-requirement trên run vibe -> vibeGateOwnerDebate
func TestVibeGatePrecedence_Drift80RoutesToOwnerDebate(t *testing.T) {
	got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: "r-reg"},
		}},
	}, 80)
	if got != vibeGateOwnerDebate {
		t.Fatalf("got %d, want owner debate", got)
	}
}

// Scenario: Wiring runner — dev mode không bao giờ vào nhánh owner debate dù drift cao
func TestVibeGatePrecedence_DevModeNeverRoutesToOwnerDebate(t *testing.T) {
	got := classifyVibeGateWithDrift(workingmode.Dev, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: "r-reg"},
		}},
	}, 95)
	if got != vibeGatePassthrough {
		t.Fatalf("got %d, want passthrough", got)
	}
}

// Scenario: Drift-only escalation — clean gate + drift 80 trên vibe vẫn vào owner debate
func TestVibeGatePrecedence_DriftOnlyCleanGateEscalates(t *testing.T) {
	got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{}, 80)
	if got != vibeGateOwnerDebate {
		t.Fatalf("got %d, want owner debate", got)
	}
}

// Edge: drift dưới ngưỡng giữ nguyên semantics cũ (block vẫn debate, clean passthrough).
// Task-351: classifier consume ResolvePrecedence — routing derive từ từng
// violation (shape thật do flowgate.Enforce tạo), không từ EnforceResult.Action
// trống không có violations (shape nhân tạo không thể xảy ra trong production).
func TestVibeGatePrecedence_DriftBelowThresholdLegacySemantics(t *testing.T) {
	if got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{}, 79); got != vibeGatePassthrough {
		t.Fatalf("clean 79: got %d, want passthrough", got)
	}
	if got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: "r-scope", Action: "block"},
		}},
	}, 79); got != vibeGateOwnerDebate {
		t.Fatalf("block 79: got %d, want owner debate", got)
	}
}

// Edge: requirement-class vẫn thắng drift 80+ (user-only, SS-18 BR-4).
func TestVibeGatePrecedence_RequirementBeatsDrift(t *testing.T) {
	got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID},
		}},
	}, 95)
	if got != vibeGateRequirement {
		t.Fatalf("got %d, want requirement", got)
	}
}

// Scenario: latestVibeDriftScore đọc lastScore từ drift state của run
func TestLatestVibeDriftScore_ReadsLastScore(t *testing.T) {
	t.Setenv("FLOWPILOT_ENABLE_DRIFT_DETECTOR", "1")
	svc, _ := newTestServer(t)
	runID := "run-vibe-drift-score"
	st := driftStateFor(svc, runID)
	st.mu.Lock()
	st.lastScore = 85
	st.mu.Unlock()
	if got := svc.latestVibeDriftScore(&interactiveRun{id: runID}); got != 85 {
		t.Fatalf("got %d, want 85", got)
	}
}

// Scenario: T-3 — stashVibeFlowForDebate xóa pending drift-ladder actions
// (narrow/note) để lượt debate hội đủ ngữ cảnh vi phạm.
func TestStashVibeFlowForDebate_ClearsDriftLadderActions(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	st := driftStateFor(svc, parent.RunID)
	st.mu.Lock()
	st.pendingNote = "drift system note"
	st.pendingNarrow = true
	st.mu.Unlock()

	svc.stashVibeFlowForDebate(parent.RunID)

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.pendingNote != "" || st.pendingNarrow {
		t.Fatalf("pending ladder actions must be dropped before a debate turn: note=%q narrow=%v",
			st.pendingNote, st.pendingNarrow)
	}
}

// Task-352 re-review (P2): warn-mode gate — violation-routed debate downgrade
// về passthrough (Enforce hạ block → warn); drift-routed debate vẫn escalate.
func TestVibeGatePrecedence_WarnModeDowngradesViolationDebate(t *testing.T) {
	violations := []flowgate.Violation{{
		Rule: flowgate.Rule{ID: "r-scope", Action: "block"},
	}}
	// Enforce ở warn mode hạ Action về "warn" → block/reprompt debate → passthrough.
	if got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action:     "warn",
		Violations: violations,
	}, 40); got != vibeGatePassthrough {
		t.Fatalf("warn-mode block violations: got %d, want passthrough", got)
	}
	// Drift-routed debate (clean gate) không phụ thuộc gate action → vẫn debate.
	if got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{}, 95); got != vibeGateOwnerDebate {
		t.Fatalf("drift-routed debate in warn mode: got %d, want owner debate", got)
	}
}
