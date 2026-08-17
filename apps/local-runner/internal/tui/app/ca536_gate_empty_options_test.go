package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-536 (run-103672): the runner emits flow_gate_violation for warn/reprompt
// verdicts and for blocks without r-reg options — those carry empty GateOptions.
// The TUI used to arm m.gate regardless, so every keystroke re-printed
// "Gate options:  (enter number or name)" with nothing to match and chat froze
// permanently. Only a genuine "block" verdict WITH a decision card may lock the
// composer. Provider-agnostic logic, parameterized over claude/codex/grok.

func gateViolationModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-103672", Status: "running"}
	return m
}

func TestGateViolation_WarnStatusDoesNotArmGate(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:   "flow_gate_violation",
				Status: "warn",
			}})
			am := m2.(*AppModel)
			if am.gate != nil {
				t.Fatalf("%s: warn verdict must not arm the gate", pk)
			}
			if am.statusMsg == "gate" {
				t.Fatalf("%s: warn verdict must not set statusMsg=gate", pk)
			}
		})
	}
}

func TestGateViolation_RepromptStatusDoesNotArmGate(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:   "flow_gate_violation",
				Status: "reprompt",
			}})
			am := m2.(*AppModel)
			if am.gate != nil {
				t.Fatalf("%s: reprompt verdict must not arm the gate", pk)
			}
		})
	}
}

func TestGateViolation_BlockWithoutOptionsDoesNotArmGate(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:   "flow_gate_violation",
				Status: "block",
			}})
			am := m2.(*AppModel)
			if am.gate != nil {
				t.Fatalf("%s: block without a decision card must not arm the gate", pk)
			}
		})
	}
}

func TestGateViolation_BlockWithOptionsStillArmsGate(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:               "flow_gate_violation",
				Status:             "block",
				GateOptions:        []string{"continue", "stop"},
				GateRegressedTests: []string{"TestFoo"},
				WorkflowRunID:      "run-103672",
			}})
			am := m2.(*AppModel)
			if am.gate == nil {
				t.Fatalf("%s: block with a decision card MUST arm the gate (CA-534)", pk)
			}
			if am.statusMsg != "gate" {
				t.Fatalf("%s: statusMsg=%q want gate", pk, am.statusMsg)
			}
			if len(am.gate.Options) != 2 {
				t.Fatalf("%s: gate options=%v want [continue stop]", pk, am.gate.Options)
			}
		})
	}
}

func TestGateViolation_EmptyOptionsChatStillSends(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := gateViolationModel(pk)
			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
				Type:   "flow_gate_violation",
				Status: "warn",
			}})
			am := m2.(*AppModel)
			// The follow-up freeform input must reach the chat send path, not be
			// swallowed by handleGateInput as a (never-matching) gate decision.
			m3, cmd := am.processInput("continue the flow")
			am3 := m3.(*AppModel)
			if cmd == nil {
				t.Fatalf("%s: chat send cmd must be non-nil after a warn gate", pk)
			}
			if am3.gate != nil {
				t.Fatalf("%s: gate must stay clear after chat send", pk)
			}
			var reprints []string
			for _, msg := range am3.messages {
				if strings.Contains(msg.Content, "Gate options:") {
					reprints = append(reprints, msg.Content)
				}
			}
			if len(reprints) != 0 {
				t.Fatalf("%s: empty gate prompt must never reprint:\n%v", pk, reprints)
			}
		})
	}
}

func TestGateInput_EmptyOptionsEscapeHatch(t *testing.T) {
	m := gateViolationModel("grok")
	m.gate = &GateState{Options: nil, RunID: "run-103672"}
	m2, cmd := m.handleGateInput("continue")
	am := m2.(*AppModel)
	if am.gate != nil {
		t.Fatal("a gate armed with no options must be cleared by the escape hatch")
	}
	if cmd != nil {
		t.Fatal("escape hatch must not emit a gate submit cmd")
	}
}
