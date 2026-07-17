package runner

import (
	"strings"
	"testing"
)

func TestKeepTestFixCodePromptRequiresWriteTool(t *testing.T) {
	p0 := keepTestFixCodePrompt("TestAdd", 0)
	if !strings.Contains(p0, "write/edit tool") {
		t.Fatalf("base prompt must require write tool, got %q", p0)
	}
	if strings.Contains(p0, "[retry") {
		t.Fatal("attempt 0 must not include retry nudge")
	}
	p1 := keepTestFixCodePrompt("TestAdd", 1)
	if !strings.Contains(p1, "[retry 1]") {
		t.Fatalf("attempt>0 must include retry nudge, got %q", p1)
	}
}

func TestApplyGateFixCodeAutoRepromptSuppressesModal(t *testing.T) {
	rs := &interactiveRun{
		id:                "run-child",
		lastTurnStepID:    "step-1",
		gateFixCodeActive: true,
	}
	opts := []string{"keep-test-fix-code", "suggest-requirement-change", "custom"}
	tests := []string{"TestAdd"}

	emit, prompt, step := applyGateFixCodeAutoRepromptLocked(rs, rs.id, opts, tests)
	if emit != nil {
		t.Fatalf("emitOptions must be nil to suppress modal, got %#v", emit)
	}
	if prompt == "" || step != "step-1" {
		t.Fatalf("auto prompt/step missing: prompt=%q step=%q", prompt, step)
	}
	if rs.gateFixCodeAttempts != 1 {
		t.Fatalf("attempts=%d, want 1", rs.gateFixCodeAttempts)
	}

	// Exhaust budget.
	rs.gateFixCodeAttempts = maxGateFixCodeAutoReprompts
	emit2, prompt2, _ := applyGateFixCodeAutoRepromptLocked(rs, rs.id, opts, tests)
	if prompt2 != "" {
		t.Fatal("exhausted budget must not auto-reprompt")
	}
	if emit2 == nil || len(emit2) == 0 {
		t.Fatal("exhausted budget must re-surface decision options")
	}
	if rs.gateFixCodeActive {
		t.Fatal("gateFixCodeActive must clear when budget exhausted")
	}
}

func TestApplyGateFixCodeAutoRepromptInactiveShowsModal(t *testing.T) {
	rs := &interactiveRun{id: "run-x", gateFixCodeActive: false}
	opts := []string{"keep-test-fix-code"}
	emit, prompt, _ := applyGateFixCodeAutoRepromptLocked(rs, rs.id, opts, []string{"TestAdd"})
	if prompt != "" {
		t.Fatal("inactive must not auto-reprompt")
	}
	if len(emit) != 1 {
		t.Fatalf("emitOptions = %#v, want original options", emit)
	}
}
