package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Additive: F1 rag-harness pending vs done must not both look ✓.
// Before the fix, pending defaulted to ✓/+ so validate/audit pending looked done.
func TestF2Step_PendingVsDoneGlyph(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.asciiMode = false
			m.width, m.height = 120, 30
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "context", Status: "DONE"},
				{StepID: "s2", NodeID: "implement", Status: "DONE"},
				{StepID: "s3", NodeID: "validate", Status: ""},
				{StepID: "s4", NodeID: "audit", Status: "PENDING"},
			}
			lines := m.flowStepsPanelLines()
			joined := strings.Join(lines, "\n")
			// Pending/empty must be [ ] (space), not ✓.
			for _, want := range []string{"[ ] validate", "[ ] audit"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("%s: pending step must be [ ] not ✓, got:\n%s", pk, joined)
				}
			}
			if strings.Contains(joined, "[✓] validate") || strings.Contains(joined, "[✓] audit") {
				t.Fatalf("%s: pending must not show ✓:\n%s", pk, joined)
			}
			// Done must stay ✓ (unicode) / [+] ascii check in ascii subtest below.
			if !strings.Contains(joined, "[✓] context") || !strings.Contains(joined, "[✓] implement") {
				t.Fatalf("%s: DONE must be [✓]:\n%s", pk, joined)
			}
		})
		t.Run(pk+"-ascii", func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.asciiMode = true
			m.width, m.height = 120, 30
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "context", Status: "DONE"},
				{StepID: "s2", NodeID: "validate", Status: "PENDING"},
			}
			lines := m.flowStepsPanelLines()
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "[+] context") {
				t.Fatalf("%s ascii DONE must be [+]:\n%s", pk, joined)
			}
			if !strings.Contains(joined, "[ ] validate") {
				t.Fatalf("%s ascii pending must be [ ]:\n%s", pk, joined)
			}
		})
	}
}

func TestF2Step_RunningSpinnerAndFailedGlyph(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.asciiMode = false
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "validate", Status: "RUNNING"},
				{StepID: "s2", NodeID: "audit", Status: "FAILED", RejectionNote: "bad"},
			}
			m.thinkingFrame = 0
			lines := m.flowStepsPanelLines()
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "RUNNING") {
				t.Fatalf("%s: RUNNING must keep token:\n%s", pk, joined)
			}
			// Spinner glyph is rendered, not [✓]/[ ]
			if strings.Contains(joined, "[ ] validate") || strings.Contains(joined, "[✓] validate") {
				t.Fatalf("%s: RUNNING must not use pending/done glyph:\n%s", pk, joined)
			}
			if !strings.Contains(joined, "[x] audit") {
				t.Fatalf("%s: FAILED must be [x]:\n%s", pk, joined)
			}
		})
	}
}

func TestF2Step_RagHarness_FullFlowPendingVsDone(t *testing.T) {
	// Rag-harness 4 steps: context DONE, implement DONE, validate/audit empty → pending [ ] not ✓.
	// This is the screenshot repro: after implement DONE, F2 showed validate/audit as ✓.
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 120, 30
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "preflight_contract_plan", Status: "DONE"},
				{StepID: "s2", NodeID: "preflight_contract_freeze", Status: "DONE"},
				{StepID: "s3", NodeID: "context", Status: "DONE"},
				{StepID: "s4", NodeID: "implement", Status: "DONE"},
				{StepID: "s5", NodeID: "validate", Status: ""},
				{StepID: "s6", NodeID: "audit", Status: ""},
			}
			joined := strings.Join(m.flowStepsPanelLines(), "\n")
			if strings.Contains(joined, "[✓] validate") || strings.Contains(joined, "[✓] audit") {
				t.Fatalf("%s: rag-harness pending validate/audit must not be ✓:\n%s", pk, joined)
			}
			if !strings.Contains(joined, "[ ] validate") || !strings.Contains(joined, "[ ] audit") {
				t.Fatalf("%s: rag-harness pending must be [ ]:\n%s", pk, joined)
			}
		})
	}
}
