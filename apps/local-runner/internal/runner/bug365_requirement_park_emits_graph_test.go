package runner

import "testing"

// BUG-365: the BUG-363/364 requirement park set the loop state but never
// emitted an agent-graph event, so the TUI kept "Thinking" forever with no
// [Retry] card after the slicer parked (live run-646702: task_slicer DONE,
// main agent Thinking 3m29s). The park must reach the client.
func TestBUG365_RequirementParkEmitsGraph(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.parkVibeRequirement(parent.RunID,
		"task_slicer produced no Task files under requirements/08-Task/todo/; refusing to sprint from fallback")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if rs == nil {
		t.Fatal("run missing")
	}
	for i := len(rs.events) - 1; i >= 0; i-- {
		ev := rs.events[i]
		if ev.Type != EventAgentGraphUpdated || ev.AgentGraphSnapshot == nil {
			continue
		}
		if ev.AgentGraphSnapshot.LoopState.Status == "blocked" &&
			ev.AgentGraphSnapshot.LoopState.BlockReason == "requirement" {
			return
		}
	}
	t.Fatal("parkVibeRequirement must emit EventAgentGraphUpdated with blocked/requirement")
}
