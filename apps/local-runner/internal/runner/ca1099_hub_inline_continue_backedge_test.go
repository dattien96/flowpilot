package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1099: vibe-owner-debate's continue back-edge is
// debate_synthesis --continue--> debate_trigger, and debate_trigger is a
// hub.inline node — executed by the PARENT hub session, never a child run.
// Live (fp-debate-live run-31884, 2026-10-02): synthesis submitted "continue"
// (round 1/5, looping), the step timeline stamped debate_trigger RUNNING, and
// maybeReinvokeCoderForContinue resolved the target correctly but had no
// branch for hub.inline targets: reinvoke lifecycle skipped spawnContinueChild,
// non-agent.delegate skipped the delegate-reuse path, and the final
// reinvokeMatchingFlowChild hunted a child labeled "debate_trigger" that can
// never exist — silent no-op, the debate wedged at round 1 forever.
//
// The fix: a continue back-edge targeting a hub.inline node must re-invoke
// the parent hub session on that node (activeHubNodeID re-pointed so the next
// verdict resolves debate_trigger's own edges), mirroring dispatchHubNotifyNode.

// ca1099DebateFixture is the live vibe-owner-debate topology.
func ca1099DebateFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "debate_trigger", To: "owner_1", When: "done", Kind: "forward"},
		{From: "debate_trigger", To: "owner_2", When: "done", Kind: "forward"},
		{From: "owner_1", To: "debate_synthesis", When: "done", Kind: "forward"},
		{From: "owner_2", To: "debate_synthesis", When: "done", Kind: "forward"},
		{From: "debate_synthesis", To: "debate_trigger", When: "continue", Kind: "back"},
		{From: "debate_synthesis", To: "done", When: "done", Kind: "forward"},
		{From: "debate_synthesis", To: "ask_user", When: "escalate", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "owner_1", Behavior: "agent.delegate", Agent: "agents/owner.md", Lifecycle: "spawn", Cohort: "owner_debate", Join: "all"},
		{ID: "owner_2", Behavior: "agent.delegate", Agent: "agents/owner.md", Lifecycle: "spawn", Cohort: "owner_debate", Join: "all"},
		{ID: "debate_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	return edges, nodes
}

// TestCA1099ContinueBackEdgeToInlineHubReinvokesParent pins that a "continue"
// verdict on debate_synthesis re-invokes the PARENT hub run on debate_trigger —
// a hub turn (parent RunID), not a child re-dispatch and not silence.
func TestCA1099ContinueBackEdgeToInlineHubReinvokesParent(t *testing.T) {
	var mu sync.Mutex
	type captured struct {
		runID  string
		prompt string
	}
	var turns []captured
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				turns = append(turns, captured{runID: req.RunID, prompt: req.Prompt})
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	edges, nodes := ca1099DebateFixture()

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "debate_synthesis" // hub is parked on synthesis when it submits continue
	svc.mu.Unlock()

	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "round 1 contested — re-deliberate",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	// The back-edge target debate_trigger is hub.inline — its re-entry must be
	// a turn on the PARENT hub session, so its "done" resolves the forward
	// edges that re-spawn owner_1/owner_2.
	waitLoop(t, "parent hub reinvoked on debate_trigger for the new debate round", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, tr := range turns {
			if tr.runID == parent.RunID {
				return true
			}
		}
		return false
	})
	// And the re-pointed hub node must be debate_trigger so its verdict
	// resolves owner-spawning edges, not synthesis's own.
	svc.mu.Lock()
	gotHub := svc.runs[parent.RunID].activeHubNodeID
	svc.mu.Unlock()
	if gotHub != "debate_trigger" {
		t.Fatalf("activeHubNodeID = %q, want debate_trigger re-pointed for the re-entered inline node", gotHub)
	}
}

// TestCA1099ContinueBackEdgeInlineHubDoesNotSpawnChild pins that the re-entry
// does NOT spawn a fresh child run for the inline node — debate_trigger runs
// on the parent hub; a child labeled debate_trigger would orphan.
func TestCA1099ContinueBackEdgeInlineHubDoesNotSpawnChild(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	edges, nodes := ca1099DebateFixture()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "debate_synthesis"
	svc.mu.Unlock()

	if _, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "continue", Summary: "loop"}); fcErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", fcErr)
	}
	// Give any (wrong) child spawn a moment to fire.
	time.Sleep(300 * time.Millisecond)
	svc.mu.Lock()
	var bad []string
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && strings.EqualFold(run.label, "debate_trigger") {
			bad = append(bad, run.id)
		}
	}
	svc.mu.Unlock()
	if len(bad) > 0 {
		t.Fatalf("continue to inline hub node spawned child run(s) %v — hub.inline targets must re-invoke the parent hub, not a child", bad)
	}
}
