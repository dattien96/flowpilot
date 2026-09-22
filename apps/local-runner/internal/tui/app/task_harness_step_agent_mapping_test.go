package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// task-harness S3 (BUG-XXX): two flow nodes that share ONE catalog agent
// (plan_writer + implement both spawn from "coder", plan_reviewer + reviewer
// both from "reviewer") must map to their own child runs by node id, never by
// the shared agent name. New file — no pre-existing test is modified.

func sameCatalogChildModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 36
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	return m
}

func TestChildRunForStep_SameCatalogAgentPinsOwnNode(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sameCatalogChildModel(pk)
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-pw", AgentName: "coder", Label: "plan_writer", Status: "running"},
			}
			pw := client.WorkflowStepRuntime{StepID: "s1", NodeID: "plan_writer", AgentRef: "agents/coder.md", Status: "DONE"}
			impl := client.WorkflowStepRuntime{StepID: "s2", NodeID: "implement", AgentRef: "agents/coder.md", Status: "PENDING"}
			if r, ok := m.childRunForStep(pw); !ok || r.RunID != "run-pw" {
				t.Fatalf("%s: plan_writer must map to its own child, got %q ok=%v", pk, r.RunID, ok)
			}
			if r, ok := m.childRunForStep(impl); ok {
				t.Fatalf("%s: implement must NOT map to plan_writer's child (shared catalog name), got %q", pk, r.RunID)
			}
		})
	}
}

func TestChildRunForStep_ReviewerPairPinsOwnNode(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sameCatalogChildModel(pk)
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-pr", AgentName: "reviewer", Label: "plan_reviewer", Status: "running"},
			}
			pr := client.WorkflowStepRuntime{StepID: "s1", NodeID: "plan_reviewer", AgentRef: "agents/reviewer.md", Status: "RUNNING"}
			rev := client.WorkflowStepRuntime{StepID: "s2", NodeID: "reviewer", AgentRef: "agents/reviewer.md", Status: "PENDING"}
			if r, ok := m.childRunForStep(pr); !ok || r.RunID != "run-pr" {
				t.Fatalf("%s: plan_reviewer must map to its own child, got %q ok=%v", pk, r.RunID, ok)
			}
			if r, ok := m.childRunForStep(rev); ok {
				t.Fatalf("%s: reviewer must NOT map to plan_reviewer's child, got %q", pk, r.RunID)
			}
		})
	}
}

func TestFlowStepsPanel_SameCatalogAgentOnlyFocusedRowHighlighted(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sameCatalogChildModel(pk)
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-pw", AgentName: "coder", Label: "plan_writer", Status: "running"},
			}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "plan_writer", AgentRef: "agents/coder.md", Status: "DONE"},
				{StepID: "s2", NodeID: "implement", AgentRef: "agents/coder.md", Status: "PENDING"},
			}
			m.focusRunID = "run-pw"

			pwLine, implLine := stepRowLines(t, m.flowStepsPanelLines(), "plan_writer"), stepRowLines(t, m.flowStepsPanelLines(), "implement")
			if !strings.Contains(pwLine, ">") && !strings.Contains(pwLine, "▸") {
				t.Fatalf("%s: focused plan_writer row must carry the marker:\n%s", pk, pwLine)
			}
			if strings.Contains(implLine, ">") || strings.Contains(implLine, "▸") {
				t.Fatalf("%s: implement row must NOT be highlighted while viewing plan_writer:\n%s", pk, implLine)
			}
			if strings.Contains(implLine, "agent:") {
				t.Fatalf("%s: implement (no child yet) must not show an agent chip:\n%s", pk, implLine)
			}
		})
	}
}

func TestChildRunForStep_UnlabeledAgentNameFallbackStillWorks(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sameCatalogChildModel(pk)
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-c", AgentName: "coder", Status: "running"},
			}
			step := client.WorkflowStepRuntime{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"}
			if r, ok := m.childRunForStep(step); !ok || r.RunID != "run-c" {
				t.Fatalf("%s: unlabeled child must still map by agent name (rag-harness), got %q ok=%v", pk, r.RunID, ok)
			}
		})
	}
}

func TestChildRunForStep_FocusedPreferenceKept(t *testing.T) {
	m := sameCatalogChildModel("codex")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main"},
		{RunID: "run-a", AgentName: "my-reviewer"},
		{RunID: "run-b", AgentName: "my-reviewer"},
	}
	step := client.WorkflowStepRuntime{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer"}
	m.focusRunID = "run-b"
	if r, ok := m.childRunForStep(step); !ok || r.RunID != "run-b" {
		t.Fatalf("focused child must win the mapping (CA-529), got %q ok=%v", r.RunID, ok)
	}
	m.focusRunID = ""
	if r, ok := m.childRunForStep(step); !ok || r.RunID != "run-a" {
		t.Fatalf("without focus first match must win (CA-513/524), got %q ok=%v", r.RunID, ok)
	}
}

func stepRowLines(t *testing.T, lines []string, node string) string {
	t.Helper()
	for _, l := range lines {
		plain := stripANSI(l)
		if strings.Contains(plain, node) {
			return plain
		}
	}
	t.Fatalf("no step row for %q in:\n%s", node, strings.Join(lines, "\n"))
	return ""
}
