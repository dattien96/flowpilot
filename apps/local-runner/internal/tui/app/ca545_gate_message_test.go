package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-545 (run-204658): non-blocking flow_gate_violation events (reprompt/warn and
// block-without-options) must NOT render a misleading "Options:" line. The old
// buildGateMessage always printed "Options: " even with empty opts, so the chat
// showed "[GATE] Flow gate triggered.  Options:" and appeared to wait for input
// while the runner was actually auto-reprompting. Only a real block with a
// decision card may show Options. Provider-agnostic; parameterized.

func TestBuildGateMessage_RepromptHasNoOptions(t *testing.T) {
	msg := buildGateMessage("reprompt", "Flow gate: Missing Change Contract", nil, nil)
	if strings.Contains(msg, "Options:") {
		t.Fatalf("reprompt must not contain Options: got %q", msg)
	}
	if !strings.Contains(msg, "auto-reprompt") {
		t.Fatalf("reprompt must contain auto-reprompt: got %q", msg)
	}
	if !strings.Contains(msg, "Missing Change Contract") {
		t.Fatalf("reprompt must surface error: got %q", msg)
	}
}

func TestBuildGateMessage_WarnHasNoOptions(t *testing.T) {
	msg := buildGateMessage("warn", "Flow gate: some warning", nil, nil)
	if strings.Contains(msg, "Options:") {
		t.Fatalf("warn must not contain Options: got %q", msg)
	}
	if !strings.Contains(msg, "warn") {
		t.Fatalf("warn must contain warn: got %q", msg)
	}
}

func TestBuildGateMessage_BlockWithoutOptionsHasNoOptions(t *testing.T) {
	msg := buildGateMessage("block", "Flow gate: blocked but no options", nil, nil)
	if strings.Contains(msg, "Options:") {
		t.Fatalf("block without options must not contain Options: got %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "blocked") {
		t.Fatalf("block without options must contain blocked: got %q", msg)
	}
}

func TestBuildGateMessage_BlockWithOptionsStillShowsOptions(t *testing.T) {
	msg := buildGateMessage("block", "", []string{"continue", "stop"}, []string{"TestFoo"})
	if !strings.Contains(msg, "Options:") {
		t.Fatalf("block with options MUST contain Options: got %q", msg)
	}
	if !strings.Contains(msg, "continue") || !strings.Contains(msg, "stop") {
		t.Fatalf("block options must list continue/stop: got %q", msg)
	}
	if !strings.Contains(msg, "Regressed tests") {
		t.Fatalf("block with regressed must contain Regressed tests: got %q", msg)
	}
}

func TestBuildGateMessage_BlockWithOptionsContainsRegressed(t *testing.T) {
	msg := buildGateMessage("block", "", []string{"continue"}, []string{"TestFoo", "TestBar"})
	if !strings.Contains(msg, "TestFoo") || !strings.Contains(msg, "TestBar") {
		t.Fatalf("regressed tests must be listed: got %q", msg)
	}
}

func TestBuildGateMessage_RepromptCaseInsensitive(t *testing.T) {
	msg := buildGateMessage("RePrompt", "Flow gate: x", nil, nil)
	if !strings.Contains(msg, "auto-reprompt") {
		t.Fatalf("status must be case-insensitive: got %q", msg)
	}
}

func TestGateViolation_RepromptMessageHasNoOptions(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:  "flow_gate_violation",
				Status: "reprompt",
				Error:  "Flow gate: Missing Change Contract. Before your next edit...",
			}})
			am := m2.(*AppModel)
			var found bool
			for _, mm := range am.messages {
				if mm.FormatHint == "gate" {
					found = true
					if strings.Contains(mm.Content, "Options:") {
						t.Fatalf("%s: reprompt gate message must not contain Options:: %q", pk, mm.Content)
					}
					if !strings.Contains(mm.Content, "auto-reprompt") {
						t.Fatalf("%s: reprompt gate message must contain auto-reprompt: %q", pk, mm.Content)
					}
				}
			}
			if !found {
				t.Fatalf("%s: expected a gate message", pk)
			}
			if am.gate != nil {
				t.Fatalf("%s: reprompt must not arm gate", pk)
			}
		})
	}
}

func TestGateViolation_WarnMessageHasNoOptions(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 80, 24
			m.sessionLoading = false
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-warn", Status: "running"}
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:  "flow_gate_violation",
				Status: "warn",
				Error:  "Flow gate: some warning detail",
			}})
			am := m2.(*AppModel)
			for _, mm := range am.messages {
				if mm.FormatHint == "gate" && strings.Contains(mm.Content, "Options:") {
					t.Fatalf("%s: warn gate message must not contain Options:: %q", pk, mm.Content)
				}
			}
		})
	}
}

func TestGateViolation_BlockWithOptionsMessageStillHasOptions(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:               "flow_gate_violation",
				Status:             "block",
				Error:              "Flow gate: Tests failed: TestFoo",
				GateOptions:        []string{"continue", "stop"},
				GateRegressedTests: []string{"TestFoo"},
			}})
			am := m2.(*AppModel)
			var found bool
			for _, mm := range am.messages {
				if mm.FormatHint == "gate" {
					found = true
					if !strings.Contains(mm.Content, "Options:") {
						t.Fatalf("%s: block with options MUST contain Options:: %q", pk, mm.Content)
					}
					if !strings.Contains(mm.Content, "Regressed tests") {
						t.Fatalf("%s: block must contain Regressed tests: %q", pk, mm.Content)
					}
				}
			}
			if !found {
				t.Fatalf("%s: expected gate message", pk)
			}
		})
	}
}
