package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-504 (live run-3688 / run-22241): the machine-verdict face
// (submit_review_outcome) was not reliably available on gated children.
//
// Two runner-side defects produced the live symptoms:
//
//  1. offerReviewOutcomeTool enumerated only mapped hub cohorts
//     (plan/review) and locked coders — a child executing a node whose
//     declared posture is verdict_only (vibe-owner-debate's owner_1 /
//     owner_2, cohort "owner_debate") never had the tool registered, so
//     grok correctly reported it absent and invented a
//     /tmp/submit_review_outcome.json file-drop nobody ingests.
//  2. The same offer gap hit the PARENT session hosting the owner-debate
//     flow's inline nodes (debate_trigger/debate_synthesis): a
//     non-autoOrchestrate flow host's turns never offered the face even
//     though the mounted flow declares verdict-bearing nodes.
//
// Additionally, FlowPilot's own posture policy already treats the verdict
// face as read-only-compatible (chat_posture_policy.go: "runner-hosted and
// enforced at SubmitFlowControl — a read_only reviewer MUST be able to
// submit it"), yet the tools/list descriptor carried no readOnlyHint — so
// providers whose session mode filters non-readOnlyHint MCP calls (devin
// ask/accept-edits) refused the call upstream of the runner bridge.

// TestBug504_VerdictOnlyChildOfferedReviewOutcomeTool pins that a child run
// executing a verdict_only flow node is offered submit_review_outcome —
// the only MCP surface that posture allows.
func TestBug504_VerdictOnlyChildOfferedReviewOutcomeTool(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
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
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1", Behavior: "agent.delegate", Cohort: "owner_debate", Posture: PostureVerdictOnly},
		{ID: "owner_2", Behavior: "agent.delegate", Cohort: "owner_debate", Posture: PostureVerdictOnly},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].stepID = "owner_1"
	svc.runs[child.RunID].label = "owner_1"
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(child.RunID, TurnInput{StepID: "turn-1", Prompt: "give your verdict"}, "", ""); apiErr != nil {
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
		t.Fatal("verdict_only child must be offered submit_review_outcome — it is the only action the posture allows")
	}
}

// TestBug504_VerdictFlowHostOffersToolOnInlineTurn pins that the parent
// session hosting an inline node of a flow that declares verdict_only
// children is offered the verdict face — the synthesis/trigger turns run on
// this session and must be able to submit the debate outcome.
func TestBug504_VerdictFlowHostOffersToolOnInlineTurn(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
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
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// The run hosts a mounted flow containing verdict_only nodes — no
	// autoOrchestrate flag set (the live vibe-owner-debate host shape).
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1", Behavior: "agent.delegate", Cohort: "owner_debate", Posture: PostureVerdictOnly},
		{ID: "owner_2", Behavior: "agent.delegate", Cohort: "owner_debate", Posture: PostureVerdictOnly},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "turn-1", Prompt: "synthesize"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "host turn settled", 3*time.Second, func() bool {
		mu.Lock()
		n := len(offers)
		mu.Unlock()
		svc.mu.Lock()
		rs := svc.runs[parent.RunID]
		inflight := rs.turnInFlight
		pendingGate := rs.pendingFlowGateSettle || rs.postTurnGateCancel != nil
		svc.mu.Unlock()
		return n == 1 && !inflight && !pendingGate
	})
	mu.Lock()
	defer mu.Unlock()
	if !offers[0] {
		t.Fatal("flow host running a flow that declares verdict_only nodes must offer submit_review_outcome to its inline turns")
	}
}

// TestBug504_VerdictToolDefsCarryReadOnlyHint pins the tools/list
// descriptor: FlowPilot-hosted interaction tools that never mutate the
// session environment (submit_review_outcome, vibe-requirement-outcome,
// ask_user, approve) must advertise readOnlyHint so provider-side session
// modes that refuse non-readOnlyHint MCP calls (devin ask/accept-edits —
// live run-1) do not wall them off upstream of the runner bridge.
// spawn_agent deliberately stays unannotated: it has a real side effect.
func TestBug504_VerdictToolDefsCarryReadOnlyHint(t *testing.T) {
	assertHint := func(t *testing.T, defs []any, name string, want bool) {
		t.Helper()
		for _, d := range defs {
			m, ok := d.(map[string]any)
			if !ok || m["name"] != name {
				continue
			}
			ann, _ := m["annotations"].(map[string]any)
			got, _ := ann["readOnlyHint"].(bool)
			if got != want {
				t.Fatalf("tool %q readOnlyHint=%v, want %v (def=%+v)", name, got, want, m)
			}
			return
		}
		t.Fatalf("tool %q missing from defs %+v", name, defs)
	}

	defs := claudeMCPToolDefsWithVibe(true, true)
	assertHint(t, defs, "submit_review_outcome", true)
	assertHint(t, defs, "vibe-requirement-outcome", true)
	assertHint(t, defs, "ask_user", true)
	assertHint(t, defs, "approve", true)
	assertHint(t, defs, "spawn_agent", false)
}
