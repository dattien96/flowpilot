package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-569 (live run-100368): a "continue" flow_control on synthesis correctly
// resolved the back-edge to coder and stamped the step RUNNING — but
// maybeReinvokeCoderForContinue's final child match required
// child.label == "coder" exactly. The live coder legs were hub-adhoc-spawned
// with task-scoped labels ("task025_coder", "task025_coder_r2"), so the match
// silently found nothing and the step sat RUNNING with no live leg.
//
// The fix: the back-edge target match must also accept a task-scoped label
// (<prefix>_<nodeID>, with an optional trailing _rN round suffix) and, failing
// that, the node's declared agent identity.

// bug569SprintFixture is the minimal vibe-sprint slice around the
// synthesis--continue-->coder back-edge.
func bug569SprintFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	return edges, nodes
}

func TestBUG569ContinueBackEdgeRedrivesTaskScopedCoderChild(t *testing.T) {
	var mu sync.Mutex
	var turns []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				turns = append(turns, req.RunID)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	edges, nodes := bug569SprintFixture()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "synthesis"
	// The live leg was hub-adhoc-spawned with a task-scoped label and already
	// settled — it is the child the back-edge must re-drive.
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "task025_coder"
	crs.agentName = "coder"
	crs.status = RunStatusCompleted
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)

	if _, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "reviewer requested changes — rework coder",
	}); fcErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", fcErr)
	}

	waitLoop(t, "task-scoped coder leg re-driven on synthesis continue back-edge", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range turns {
			if id == child.RunID {
				return true
			}
		}
		return false
	})
}
