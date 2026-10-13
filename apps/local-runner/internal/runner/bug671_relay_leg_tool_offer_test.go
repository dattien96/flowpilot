package runner

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-671 (live run-752742 / chat cht_3078718bf299): when the cohort parent
// is a reattach/relay leg (flowEngineDriven inherited, activeFlowNodes
// empty — BUG-669), every verdict-offer predicate resolves on EMPTY
// topology: flowNodeForRunLocked(parent), flowNodePostureFor, and the
// flowRequires* gates all see nothing, so submit_review_outcome is never
// advertised — on Devin and Grok legs alike (provider-agnostic gap).
// Fix: flow-semantic reads resolve through the chat's topology owner
// (launcher leg) via flowTopologySourceLocked.

func bug671ServiceWithLauncherRelay(t *testing.T, offers *[]bool, mu *sync.Mutex) (*InteractiveService, string, string, string) {
	t.Helper()
	// Isolate the durable quota ledger — the host's real state may carry
	// blocked accounts (live quota events), which veto startTurn upstream of
	// the offer ladder under test.
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_STATE_FILE", filepath.Join(t.TempDir(), "quota-state.json"))
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				*offers = append(*offers, req.OfferReviewOutcomeTool)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	launcher, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun launcher: %v", err)
	}
	relay, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun relay: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	// Launcher leg: legSeq 1, carries the REAL flow topology.
	launcherRs := svc.runs[launcher.RunID]
	launcherRs.chatID = "cht-bug671"
	launcherRs.legState = LegStateActive
	launcherRs.legSeq = 1
	launcherRs.flowEngineDriven = true
	launcherRs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	launcherRs.activeFlowAcceptanceNodes = []string{"synthesis"}
	// Relay leg: legSeq 2, inherited latch, NO topology (reattach inherit).
	relayRs := svc.runs[relay.RunID]
	relayRs.chatID = "cht-bug671"
	relayRs.legState = LegStateActive
	relayRs.legSeq = 2
	relayRs.flowEngineDriven = true
	// Child spawned under the RELAY leg (live: leg5 spawned the review cohort).
	childRs := svc.runs[child.RunID]
	childRs.parentRunID = relay.RunID
	childRs.chatID = "cht-bug671"
	childRs.stepID = "coder"
	childRs.label = "coder"
	// Test env pins real host accounts (possibly quota-blocked) — the offer
	// ladder under test is upstream of account routing; clear the pin.
	launcherRs.providerAccountID = svc.activeAccountForProvider(launcherRs.providerKey)
	relayRs.providerAccountID = svc.activeAccountForProvider(relayRs.providerKey)
	childRs.providerAccountID = svc.activeAccountForProvider(childRs.providerKey)
	svc.mu.Unlock()
	return svc, launcher.RunID, relay.RunID, child.RunID
}

// Cohort/delegate child spawned under a relay-leg parent must still be
// offered the verdict face — the parent's empty topology must resolve to the
// launcher leg's real graph.
func TestBug671_DelegateChildOnRelayParentOfferedOutcomeTool(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	svc, _, _, childID := bug671ServiceWithLauncherRelay(t, &offers, &mu)

	if _, apiErr := svc.startTurn(childID, TurnInput{StepID: "turn-1", Prompt: "implement + report outcome"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "child turn settled", 3*time.Second, func() bool {
		mu.Lock()
		n := len(offers)
		mu.Unlock()
		svc.mu.Lock()
		inflight := svc.runs[childID].turnInFlight
		svc.mu.Unlock()
		return n == 1 && !inflight
	})
	mu.Lock()
	defer mu.Unlock()
	if !offers[0] {
		t.Fatal("delegate child of a relay-leg parent must be offered submit_review_outcome — topology resolves on the launcher leg, not the relay's empty graph")
	}
}

// The relay leg hosting the hub turn itself must see the launcher's
// verdict-bearing topology (flowHasVerdictOnlyNode on the owner's nodes).
func TestBug671_RelayHubLegOfferedVerdictToolFromOwnerTopology(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_STATE_FILE", filepath.Join(t.TempDir(), "quota-state.json"))
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
	launcher, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun launcher: %v", err)
	}
	relay, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun relay: %v", err)
	}
	svc.mu.Lock()
	launcherRs := svc.runs[launcher.RunID]
	launcherRs.chatID = "cht-bug671b"
	launcherRs.legState = LegStateActive
	launcherRs.legSeq = 1
	launcherRs.flowEngineDriven = true
	launcherRs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate", Behavior: "agent.delegate", Posture: PostureVerdictOnly},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	relayRs := svc.runs[relay.RunID]
	relayRs.chatID = "cht-bug671b"
	relayRs.legState = LegStateActive
	relayRs.legSeq = 2
	relayRs.flowEngineDriven = true
	relayRs.turnCount = 1 // turn 2 is the hub post-join turn
	launcherRs.providerAccountID = svc.activeAccountForProvider(launcherRs.providerKey)
	relayRs.providerAccountID = svc.activeAccountForProvider(relayRs.providerKey)
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(relay.RunID, TurnInput{StepID: "turn-2", Prompt: "join cohort + submit outcome"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn relay: %s", apiErr.msg)
	}
	waitLoop(t, "relay turn settled", 3*time.Second, func() bool {
		mu.Lock()
		n := len(offers)
		mu.Unlock()
		return n == 1
	})
	mu.Lock()
	defer mu.Unlock()
	if !offers[0] {
		t.Fatal("relay hub leg must be offered submit_review_outcome — the launcher's topology declares a verdict_only node")
	}
}
