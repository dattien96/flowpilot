package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-635 (live run-348382 / run-306526): a delegate leg running an
// agent.code / agent.delegate node on a flow-driven parent has a contract
// face — submit_review_outcome — that is never registered on its MCP tool
// surface. The leg's own prompt instructs the face for blocked /
// renegotiate_signatures paths, so without the tool the verdict stays in
// prose, the engine sees no machine verdict, re-drives the completed leg,
// and zero_delta_progress escalates the loop on the engine's own gap.
//
// Fix: every spawnable flow child is offered the face — the bridge still
// rejects settle attempts from non-cohort members, so the offer carries no
// settle authority for legs whose contract does not need it.

func TestBug635_DelegateChildOfferedReviewOutcomeTool(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				offers = append(offers, req.OfferReviewOutcomeTool)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "suite green"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].stepID = "coder"
	svc.runs[child.RunID].label = "coder"
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(child.RunID, TurnInput{StepID: "turn-1", Prompt: "implement + report outcome"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "child turn settled", 3*time.Second, func() bool {
		mu.Lock()
		n := len(offers)
		mu.Unlock()
		svc.mu.Lock()
		inflight := svc.runs[child.RunID].turnInFlight
		svc.mu.Unlock()
		return n == 1 && !inflight
	})
	mu.Lock()
	defer mu.Unlock()
	if !offers[0] {
		t.Fatal("delegate leg on a flow-driven parent must be offered submit_review_outcome — its contract expects a machine verdict face")
	}
}

// The negative arm stays: a child on a NON-flow-driven parent (plain chat
// spawn) must not see the verdict face — nothing upstream consumes it.
func TestBug635_NonFlowChildStillNotOffered(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				offers = append(offers, req.OfferReviewOutcomeTool)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(child.RunID, TurnInput{StepID: "turn-1", Prompt: "go"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "child turn settled", 3*time.Second, func() bool {
		mu.Lock()
		n := len(offers)
		mu.Unlock()
		svc.mu.Lock()
		inflight := svc.runs[child.RunID].turnInFlight
		svc.mu.Unlock()
		return n == 1 && !inflight
	})
	mu.Lock()
	defer mu.Unlock()
	if offers[0] {
		t.Fatal("plain-chat child (no flow engine) must not be offered submit_review_outcome")
	}
}
