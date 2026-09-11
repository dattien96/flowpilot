package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestBUG369_AgentsPickerShowsTaskXY(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:0")
			m.asciiMode = true
			m.mode = ModeFlow
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "completed"},
				{RunID: "run-p1", Label: "preflight_contract_plan", AgentName: "contract-planner", Status: "completed", VibeTaskIndex: 1, VibeTaskTotal: 3, VibeTaskName: "Task-910.md"},
				{RunID: "run-p2", Label: "preflight_contract_plan", AgentName: "contract-planner", Status: "completed", VibeTaskIndex: 2, VibeTaskTotal: 3, VibeTaskName: "Task-911.md"},
				{RunID: "run-c2", Label: "coder", AgentName: "coder", Status: "running", VibeTaskIndex: 2, VibeTaskTotal: 3, VibeTaskName: "Task-911.md"},
			}
			m.inputValue = "/agents"
			items := m.collectSuggestions()
			var details []string
			for _, it := range items {
				if it.kind != "agent" {
					continue
				}
				details = append(details, it.detail)
			}
			blob := strings.Join(details, "\n")
			if !strings.Contains(blob, "task 1/3 Task-910.md") {
				t.Fatalf("%s: missing task 1/3: %q", pk, blob)
			}
			if !strings.Contains(blob, "task 2/3 Task-911.md") {
				t.Fatalf("%s: missing task 2/3: %q", pk, blob)
			}
		})
	}
}

func TestBUG369_AgentsDumpShowsTaskXY(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:0")
	m.asciiMode = true
	m.mode = ModeFlow
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-c", AgentName: "coder", Status: "running", VibeTaskIndex: 3, VibeTaskTotal: 3, VibeTaskName: "Task-912.md"},
	}
	m2, _ := m.handleSlashCommand("/agents")
	blob := ""
	for _, msg := range m2.(*AppModel).messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "task 3/3 Task-912.md") {
		t.Fatalf("dump missing task chip: %s", blob)
	}
}

func TestBUG369_AgentsPickerOmitsChipWithoutStamp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:0")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-c", AgentName: "coder", Label: "coder", Status: "running"},
	}
	m.inputValue = "/agents"
	items := m.collectSuggestions()
	for _, it := range items {
		if strings.Contains(it.detail, "task ") {
			t.Fatalf("non-vibe child must not show task chip: %+v", it)
		}
	}
}
