package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-529 follow-up — the focused child stays stable (no chip flicker) while live
// graph + list hydrate polls replace/reorder agentRuns. Chips are gone; the
// focused child is highlighted with a teal marker instead.

func TestChildRunForStep_PrefersFocusedRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main"},
		{RunID: "run-a", AgentName: "my-reviewer"},
		{RunID: "run-b", AgentName: "my-reviewer"},
	}
	step := client.WorkflowStepRuntime{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer"}
	m.focusRunID = "run-b"
	if r, ok := m.childRunForStep(step); !ok || r.RunID != "run-b" {
		t.Fatalf("focused child must win the mapping, got %q ok=%v", r.RunID, ok)
	}
	m.focusRunID = ""
	if r, ok := m.childRunForStep(step); !ok || r.RunID != "run-a" {
		t.Fatalf("without focus first match must win (CA-513/524), got %q ok=%v", r.RunID, ok)
	}
}

// TestChildHighlight_StableAcrossHydrateReorder locks the reported flicker: the
// polled list puts a different same-named run first, yet the focused child keeps
// no [open]/[back] chip and the teal marker stays stable.
func TestChildHighlight_StableAcrossHydrateReorder(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
			}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-b", AgentName: "my-reviewer", Status: "running"},
				{RunID: "run-a", AgentName: "my-reviewer", Status: "running"},
			}
			m.focusRunID = "run-b"

			joined := stripANSI(strings.Join(m.flowStepsPanelLines(), "\n"))
			if strings.Contains(joined, "[open]") {
				t.Fatalf("%s: focused child row must not show [open]:\n%s", pk, joined)
			}
			if strings.Contains(joined, "[back]") {
				t.Fatalf("%s: focused row must not hold [back]:\n%s", pk, joined)
			}
			if !strings.Contains(joined, ">") && !strings.Contains(joined, "▸") {
				t.Fatalf("%s: focused child must show selection marker: %s", pk, joined)
			}
			if strings.Contains(m.stepsSectionTitle(), "[back]") {
				t.Fatalf("%s: steps header must not host [back]:\n%s", pk, m.stepsSectionTitle())
			}

			// Hydrate reorder: same-named run-a now first.
			next, _ := m.Update(agentRunsHydratedMsg{
				ParentRunID: "run-main",
				Runs: []client.AgentRunSummary{
					{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
					{RunID: "run-a", AgentName: "my-reviewer", Status: "running"},
					{RunID: "run-b", AgentName: "my-reviewer", Status: "running"},
				},
			})
			am := next.(*AppModel)
			joined2 := stripANSI(strings.Join(am.flowStepsPanelLines(), "\n"))
			if strings.Contains(joined2, "[open]") {
				t.Fatalf("%s: reorder must not flip the focused row to [open]:\n%s", pk, joined2)
			}
			if strings.Contains(joined2, "[back]") {
				t.Fatalf("%s: reorder must not add [back]:\n%s", pk, joined2)
			}
			if !strings.Contains(joined2, ">") && !strings.Contains(joined2, "▸") {
				t.Fatalf("%s: marker must survive hydrate reorder: %s", pk, joined2)
			}
		})
	}
}

// TestChildHighlight_GraphReorder keeps the highlight stable through the
// agent_graph_updated path, which replaces agentRuns authoritatively.
func TestChildHighlight_GraphReorder(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-c2", AgentName: "coder", Status: "running"},
	}
	m.focusRunID = "run-c2"
	next, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
		Type: "agent_graph_updated",
		AgentGraph: &client.AgentGraphSnapshot{
			ParentRunID: "run-main",
			Runs: []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-c1", AgentName: "coder", Status: "running"},
				{RunID: "run-c2", AgentName: "coder", Status: "running"},
			},
		},
	}})
	am := next.(*AppModel)
	joined := stripANSI(strings.Join(am.flowStepsPanelLines(), "\n"))
	if strings.Contains(joined, "[open]") {
		t.Fatalf("graph reorder must not flip the focused row to [open]:\n%s", joined)
	}
	if strings.Contains(joined, "[back]") {
		t.Fatalf("graph reorder must not add [back]:\n%s", joined)
	}
	if !strings.Contains(joined, ">") && !strings.Contains(joined, "▸") {
		t.Fatalf("marker must survive graph reorder: %s", joined)
	}
}

// TestChildStep_NoOpenChipWhenNotFocused keeps the CA-513/524 mapping contract:
// without a focused child the mapped step still highlights via the teal marker
// once focused, but never renders an [open] chip.
func TestChildStep_NoOpenChipWhenNotFocused(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
	}
	joined := stripANSI(strings.Join(m.flowStepsPanelLines(), "\n"))
	if strings.Contains(joined, "[open]") {
		t.Fatalf("child step must NOT show [open] chip:\n%s", joined)
	}
	if strings.Contains(joined, "[back]") {
		t.Fatalf("must not show [back]:\n%s", joined)
	}
}
