package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// Task-339 (CP-62 P-3): structured escalation cards (request_user_decision)
// wrap around the prose card — Q-1: schema payload when present, prose card
// is the fallback, never both lost.

func cardArgs() map[string]any {
	return map[string]any{
		"question": "Sprint plan produced no Task files. How should we proceed?",
		"options": []any{
			map[string]any{"id": "reslice", "label": "Re-run task slicer", "consequence": "Tasks regenerated from the locked CP"},
			map[string]any{"id": "abort", "label": "Stop the run", "consequence": "Run parks; no sprint starts"},
		},
		"recommended": "reslice",
		"evidence": []any{
			map[string]any{"path": "requirements/07-Coding-Plan/todo/CP-901.md", "line": 42, "excerpt": "Tasks are sliced per sprint"},
		},
	}
}

// Scenario: Tool request_user_decision được gọi thành công -> phát event thẻ có cấu trúc kèm options
func TestRequestUserDecision_StructuredCard_Emitted(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	card, parseErr := parseUserDecisionCard(cardArgs())
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	svc.emitUserDecisionCard(parent.RunID, card)
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	var found *ProviderEvent
	for i := len(rs.events) - 1; i >= 0; i-- {
		if rs.events[i].Type == EventUserDecisionCardRequested {
			found = &rs.events[i]
			break
		}
	}
	svc.mu.Unlock()
	if found == nil {
		t.Fatalf("user_decision_card_requested event not emitted")
	}
	emitted, ok := found.Input.(UserDecisionCard)
	if !ok || len(emitted.Options) != 2 || emitted.Recommended != "reslice" {
		t.Fatalf("event card = %+v", found.Input)
	}
}

// Scenario: Tool không được gọi hoặc payload hỏng -> fallback sang thẻ prose cũ an toàn
func TestRequestUserDecision_FallbackToProseCard(t *testing.T) {
	// Malformed payloads are rejected by the parser; the caller then keeps the
	// legacy prose path (the prose park in parkVibeRequirement is untouched).
	cases := []struct {
		name string
		mut  func(map[string]any)
		want string
	}{
		{"no-options", func(a map[string]any) { delete(a, "options") }, "at least one option"},
		{"recommended-unknown", func(a map[string]any) { a["recommended"] = "nope" }, "recommended must reference"},
		{"option-missing-consequence", func(a map[string]any) {
			a["options"] = []any{map[string]any{"id": "reslice", "label": "Re-run"}}
		}, "consequence is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := cardArgs()
			tc.mut(args)
			_, err := parseUserDecisionCard(args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
	// Prose park path is unchanged: gate reason still blocks the loop.
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.parkVibeRequirement(parent.RunID, "task_slicer produced no Task files")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	proseReason := ""
	if rs.decisionCard != nil {
		proseReason = "unexpected card"
	}
	svc.mu.Unlock()
	if proseReason != "" {
		t.Fatalf("prose park must not attach a card")
	}
	_ = workingmode.Vibe
	_ = flowgate.PrecedenceRouteUser
}
