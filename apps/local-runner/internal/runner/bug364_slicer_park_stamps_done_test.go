package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-364: live run-640953 completed the task_slicer child (zero Task files)
// and the BUG-363 park fired — but the flow step stayed RUNNING forever.
// tryAdvanceFlowFromNode calls onVibeCpNodeDone (:1160) BEFORE its
// loop-liveness gate (:1166) and DONE writes (:1203+), so the park trips its
// own downstream gate. The fix stamps the completed node DONE before
// parking. This test drives completion through the real settle→advance
// ordering (direct onVibeCpNodeDone calls, as in bug363 tests, cannot see
// it): it FAILS on CA-817 code and PASSES with the stamp.

// Ordering: linear (non-cohort) slicer child settles to DONE + parked loop.
func TestBUG364_SettleSlicerCompletionStampsDoneAndParks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(spec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec, "SS-snake-mvp.md"), []byte("# SS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	const childID = "run-slicer-linear"
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	rs.workspaceCwd = dir
	// Overlaid vibe-cp-ingest topology (slicer→done), as live after the
	// maybeStartVibeCpIngest overlay.
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "task_slicer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "task_slicer", To: "done", When: "done", Kind: "forward"},
	}
	svc.runs[childID] = &interactiveRun{id: childID, parentRunID: parent.RunID,
		label: "task_slicer", agentName: "doc-writer", flowCohortId: "",
		status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, rs.activeFlowNodes)
	svc.agentOrchestrator.setLoop(parent.RunID,
		AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})

	// Caller holds s.mu for settleFlowChildTurnCompletedLocked.
	svc.mu.Lock()
	svc.settleFlowChildTurnCompletedLocked(svc.runs[childID], "no tasks",
		ProviderEvent{Type: EventTurnCompleted, FinalMessage: "no tasks"})
	svc.mu.Unlock()

	// advanceOrNotifyHub is async — poll for the settled terminal state.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if flowStepStatus(t, svc, parent.RunID, "task_slicer") == StepStatusDone {
			st := svc.agentOrchestrator.loopStateFor(parent.RunID)
			if st.Status == "blocked" && st.BlockReason == "requirement" &&
				strings.Contains(st.GateReason, "Task") {
				return // fixed: DONE + parked with reason
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("step=%q loop=%+v want DONE + blocked/requirement with Task reason",
		flowStepStatus(t, svc, parent.RunID, "task_slicer"),
		svc.agentOrchestrator.loopStateFor(parent.RunID))
}
