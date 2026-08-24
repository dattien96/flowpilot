package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-293: rag-harness synthesis edges — continue loops back to implement
// (review findings feed the coder immediately), approved advances to audit.
// New file — no pre-existing test is modified.

func synthesisBackEdgeFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "implement", When: "continue", Kind: "back"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

func TestSynthesisContinueReinvokesImplementWithFindings(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	edges, nodes := synthesisBackEdgeFixture()

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "implement first round", Label: "implement", Wait: false,
	}); err != nil {
		t.Fatalf("spawn implement: %v", err)
	}
	waitLoop(t, "implement first turn completed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 1
	})

	// Hub synthesis decides changes_requested -> continue back-edge.
	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "Review findings: fix the nil deref and cover the restart case",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	// The hub's own auto-reinvoke turn (agent-results-ready) also flows through
	// this adapter, so wait for the implement RE-ENTRY prompt specifically
	// (coder-reentry base + the continue summary) rather than prompts[len-1].
	waitLoop(t, "implement reinvoked with review findings", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "Feedback received") && strings.Contains(p, "fix the nil deref") {
				return true
			}
		}
		return false
	})
	mu.Lock()
	var second string
	for _, p := range prompts {
		if strings.Contains(p, "Feedback received") && strings.Contains(p, "fix the nil deref") {
			second = p
		}
	}
	mu.Unlock()
	if !strings.Contains(second, "fix the nil deref") {
		t.Fatalf("implement re-entry prompt missing the review findings: %q", second)
	}
	if !strings.Contains(second, "Feedback received") {
		t.Fatalf("implement re-entry prompt missing the coder-reentry base: %q", second)
	}
	// Reuse, not a fresh child: review-loop-style continue keeps the coder's
	// session across rounds.
	svc.mu.Lock()
	children := 0
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label == "implement" {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 1 {
		t.Fatalf("implement child count = %d, want 1 (session reuse across review rounds)", children)
	}
}

func TestSynthesisApprovedAdvancesToAudit(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := synthesisBackEdgeFixture()
	dir := t.TempDir()
	initGitRepoForAuditFixture(t, dir)

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	// Hub approves via submit_review_outcome -> synthesis --done--> audit must
	// dispatch the inline audit node instead of settling the flow at synthesis.
	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "review approved"})
	if !handled {
		t.Fatal("expected advanceHubDoneThroughEdge to take over synthesis->audit")
	}
	if res.Status == "" {
		t.Fatal("expected a flow result status from the synthesis->audit dispatch")
	}
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got != StepStatusRunning && got != StepStatusWaitingUserApr {
		t.Fatalf("audit step status = %v, want RUNNING or WAITING_USER_APPROVAL after synthesis approved", got)
	}
}
