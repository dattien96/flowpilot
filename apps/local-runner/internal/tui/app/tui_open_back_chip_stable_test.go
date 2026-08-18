package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-529 — the step [open]/[back] chip must stay stable while viewing a child,
// even when live graph + list hydrate polls replace/reorder agentRuns.

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

// TestOpenBackChip_StableAcrossHydrateReorder locks the reported flicker: the
// polled list puts a different same-named run first, yet the focused child keeps
// no [open] chip and the steps header keeps [back] instead of flipping (CA-542).
func TestOpenBackChip_StableAcrossHydrateReorder(t *testing.T) {
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

			joined := strings.Join(m.flowStepsPanelLines(), "\n")
			if strings.Contains(joined, "[open]") {
				t.Fatalf("%s: focused child row must not show [open]:\n%s", pk, joined)
			}
			if strings.Contains(joined, "[back]") {
				t.Fatalf("%s: [back] must live on the steps header, not the row (CA-542):\n%s", pk, joined)
			}
			if !strings.Contains(m.stepsSectionTitle(), "[back]") {
				t.Fatalf("%s: focused child must keep [back] on the steps header:\n%s", pk, m.stepsSectionTitle())
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
			joined2 := strings.Join(am.flowStepsPanelLines(), "\n")
			if strings.Contains(joined2, "[open]") {
				t.Fatalf("%s: reorder must not flip the focused row to [open]:\n%s", pk, joined2)
			}
			if !strings.Contains(am.stepsSectionTitle(), "[back]") {
				t.Fatalf("%s: [back] must survive hydrate reorder:\n%s", pk, am.stepsSectionTitle())
			}
		})
	}
}

// TestOpenBackChip_GraphReorderKeepsBack drives the same guarantee through the
// agent_graph_updated path, which replaces agentRuns authoritatively.
func TestOpenBackChip_GraphReorderKeepsBack(t *testing.T) {
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
	joined := strings.Join(am.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "[open]") {
		t.Fatalf("graph reorder must not flip the focused row to [open]:\n%s", joined)
	}
	if !strings.Contains(am.stepsSectionTitle(), "[back]") {
		t.Fatalf("graph reorder must keep [back] on the steps header:\n%s", am.stepsSectionTitle())
	}
}

// TestOpenBackChip_OpenWhenNotFocused keeps the CA-513/524 contract: without a
// focused child the mapped step still shows [open].
func TestOpenBackChip_OpenWhenNotFocused(t *testing.T) {
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
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if !strings.Contains(joined, "[open]") {
		t.Fatalf("unfocused child step must show [open]:\n%s", joined)
	}
	if strings.Contains(joined, "[back]") {
		t.Fatalf("unfocused must not show [back]:\n%s", joined)
	}
}
