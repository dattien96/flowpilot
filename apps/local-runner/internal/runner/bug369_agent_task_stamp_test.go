package runner

import (
	"testing"
)

func TestBUG369_UpsertSummaryKeepsVibeTaskChip(t *testing.T) {
	o := newAgentOrchestrator()
	o.upsertSummary("parent", AgentRunSummary{
		RunID: "child-1", AgentName: "coder", Label: "coder", Status: RunStatusRunning,
		VibeTaskIndex: 2, VibeTaskTotal: 3, VibeTaskName: "Task-911.md",
	})
	o.upsertSummary("parent", AgentRunSummary{
		RunID: "child-1", AgentName: "coder", Label: "coder", Status: RunStatusRunning,
	})
	got, ok := o.currentSummary("parent", "child-1")
	if !ok {
		t.Fatal("summary missing")
	}
	if got.Status != RunStatusRunning {
		t.Fatalf("status=%q", got.Status)
	}
	if got.VibeTaskIndex != 2 || got.VibeTaskTotal != 3 || got.VibeTaskName != "Task-911.md" {
		t.Fatalf("vibe chip dropped: %+v", got)
	}
}

func TestBUG369_VibeTaskProgressForSpawn(t *testing.T) {
	plan := []string{"requirements/08-Task/todo/Task-910.md", "requirements/08-Task/todo/Task-911.md"}
	idx, total, name := vibeTaskProgress(plan, 2)
	if idx != 2 || total != 2 || name != "Task-911.md" {
		t.Fatalf("got %d/%d %q", idx, total, name)
	}
}
