package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-322: TUI steps panel shows provider+model per step row (T-1) and a
// loop round/cap chip in the steps header (T-2) — Desktop parity, render-only.

func task322Model() *AppModel {
	m := New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:4317")
	m.width, m.height = 140, 30
	m.asciiMode = true
	m.mode = ModeFlow
	enableSidebarForTest(m)
	m.runHandle = &client.RunHandle{RunID: "run-548341", Status: "running"}
	return m
}

func stripPanelLines(m *AppModel) string {
	return stripANSI(strings.Join(m.flowStepsPanelLines(), "\n"))
}

// T-1: per-step provider+model renders on the row.
func TestStepsRow_ShowsProviderAndModel(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := task322Model()
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "plan_writer", Status: "DONE", Provider: "opencode-go", Model: "omen-alpha"},
				{StepID: "s2", NodeID: "reviewer", Status: "DONE", Provider: "grok", Model: "grok-4.5"},
			}
			rows := stripPanelLines(m)
			if !strings.Contains(rows, "[+] plan_writer · opencode-go/omen-alpha") {
				t.Fatalf("%s: writer row missing provider/model chip:\n%s", pk, rows)
			}
			if !strings.Contains(rows, "[+] reviewer · grok/grok-4.5") {
				t.Fatalf("%s: reviewer row missing provider/model chip:\n%s", pk, rows)
			}
		})
	}
}

// T-1: provider-only and model-only rows render the half available.
func TestStepsRow_ShowsHalfProviderChip(t *testing.T) {
	m := task322Model()
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "plan_writer", Status: "DONE", Provider: "opencode-go"},
		{StepID: "s2", NodeID: "reviewer", Status: "DONE", Model: "omen-alpha"},
	}
	rows := stripPanelLines(m)
	if !strings.Contains(rows, "[+] plan_writer · opencode-go") {
		t.Fatalf("provider-only chip missing:\n%s", rows)
	}
	if !strings.Contains(rows, "[+] reviewer · omen-alpha") {
		t.Fatalf("model-only chip missing:\n%s", rows)
	}
}

// T-1: run posture (steps-runtime top level) is the per-step fallback —
// built-in nodes inherit it server-side, so their rows must still name it.
func TestStepsRow_FallsBackToRunPosture(t *testing.T) {
	m := task322Model()
	m.runHandle = &client.RunHandle{RunID: "run-548341", Status: "running"}
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID:    "run-548341",
		Provider: "opencode-go",
		Model:    "omen-alpha",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "plan_writer", Status: "DONE"},
			{StepID: "s2", NodeID: "reviewer", Status: "DONE", Provider: "grok", Model: "grok-4.5"},
		},
	})
	am := m2.(*AppModel)
	if am.flowStepsProvider != "opencode-go" || am.flowStepsModel != "omen-alpha" {
		t.Fatalf("run posture not stored: %q/%q", am.flowStepsProvider, am.flowStepsModel)
	}
	rows := stripANSI(strings.Join(am.flowStepsPanelLines(), "\n"))
	if !strings.Contains(rows, "[+] plan_writer · opencode-go/omen-alpha") {
		t.Fatalf("posture fallback missing on writer row:\n%s", rows)
	}
	// Per-step values win over the posture fallback.
	if !strings.Contains(rows, "[+] reviewer · grok/grok-4.5") {
		t.Fatalf("per-step values must win over posture:\n%s", rows)
	}
}

// T-1: no provider/model anywhere → row renders exactly as before (no chip,
// no "unknown" noise).
func TestStepsRow_NoProviderRendersAsBefore(t *testing.T) {
	m := task322Model()
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "plan_writer", Status: "DONE"},
	}
	rows := stripPanelLines(m)
	if rows != "[+] plan_writer" {
		t.Fatalf("row must stay byte-identical without provider/model, got %q", rows)
	}
}

// T-2: header shows the round/cap chip from the agent-graph loop state.
func TestStepsHeader_ShowsRoundChip(t *testing.T) {
	m := task322Model()
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "running", Round: 2, RoundCap: 3},
	})
	if m.flowLoopRound != 2 || m.flowLoopCap != 3 {
		t.Fatalf("round/cap not stored: %d/%d", m.flowLoopRound, m.flowLoopCap)
	}
	title := stripANSI(m.stepsSectionTitle())
	if title != "steps  round 2/3" {
		t.Fatalf("header chip wrong, got %q", title)
	}
}

// T-2: flow-engine Cap wins over RoundCap (AgentLoopState contract).
func TestStepsHeader_CapPreferredOverRoundCap(t *testing.T) {
	m := task322Model()
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "running", Round: 1, RoundCap: 5, Cap: 3},
	})
	if title := stripANSI(m.stepsSectionTitle()); title != "steps  round 1/3" {
		t.Fatalf("Cap must win over RoundCap, got %q", title)
	}
}

// T-2: unknown cap (0/0, e.g. hydrate error or synthetic 409 state) → plain header.
func TestStepsHeader_HiddenWithoutCap(t *testing.T) {
	m := task322Model()
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "running"},
	})
	if title := stripANSI(m.stepsSectionTitle()); title != "steps" {
		t.Fatalf("header must stay plain without cap, got %q", title)
	}
}
