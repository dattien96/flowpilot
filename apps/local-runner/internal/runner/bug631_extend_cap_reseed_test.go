package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-631 (live run-150388): an operator extend-cap grant is durable loop
// state (extendCap mutates + persists st.Cap), but startResolvedFlow's mount
// mutation unconditionally reseeds st.Cap = Definition.Policy.Cap — every
// remount (debate re-mount, sprint re-mount, post-restart flow start) wiped
// the granted headroom back to the yaml default. Observed live:
// ExtendCount=31 yet Cap=5 — thirty-one granted extensions, zero effect.
//
// The durable signal that an operator extended is ExtendCount > 0: once any
// grant exists, a remount must keep the granted cap, not reseat to policy.
// A larger policy cap still wins (reconfiguration = more headroom, never a
// shrink). ExtendBy/NegotiationCap remain config seeds, always refreshed.

func bug631FlowStore() *fakeFlowDefinitionStore {
	store := newFakeFlowDefinitionStore()
	store.byRef[bug631FlowRef] = FlowDefinitionRecord{
		FlowRef: bug631FlowRef,
		Name:    "Extendable Flow",
		Source:  "supabase_user_definition",
		Definition: agentpack.FlowDefinition{
			ID: "bug631-extendable",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
			},
			Policy: agentpack.FlowPolicy{Cap: 3, ExtendBy: 2},
		},
	}
	return store
}

const bug631FlowRef = "66666666-6666-6666-6666-666666666666"

// The core defect: extend → remount → cap must still be the granted 5, not
// the yaml default 3.
func TestBug631_ExtendCapSurvivesRemount(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.SetFlowDefinitionStore(bug631FlowStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, bug631FlowRef, "first mount")
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Cap != 3 {
		t.Fatalf("fresh mount Cap = %d, want 3 (policy default)", st.Cap)
	}

	if _, err := svc.extendCap(parent.RunID); err != nil {
		t.Fatalf("extendCap: %v", err)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Cap != 5 || st.ExtendCount != 1 {
		t.Fatalf("after extend: Cap=%d ExtendCount=%d, want 5/1", st.Cap, st.ExtendCount)
	}

	// Remount the same flow — the path every re-mount/restart takes.
	svc.startResolvedFlow(context.Background(), parent.RunID, bug631FlowRef, "remount")

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Cap != 5 {
		t.Fatalf("remount wiped the granted cap: Cap = %d, want 5 (operator grant must survive; policy default 3 must not reseat)", st.Cap)
	}
	if st.RoundCap != 5 {
		t.Fatalf("RoundCap after remount = %d, want 5 (mirrors the granted Cap)", st.RoundCap)
	}
	awaitTournamentChildIdle(t, svc, parent.RunID)
}

// Boundary: a remount with a RAISED policy cap still adopts the larger value
// — the guard prevents shrinking, never growth.
func TestBug631_RemountAdoptsLargerPolicyCap(t *testing.T) {
	svc, _ := newTestServer(t)
	store := bug631FlowStore()
	svc.SetFlowDefinitionStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, bug631FlowRef, "first mount")
	if _, err := svc.extendCap(parent.RunID); err != nil {
		t.Fatalf("extendCap: %v", err)
	}

	// Operator reconfigured the flow: policy cap now exceeds the grant.
	rec := store.byRef[bug631FlowRef]
	rec.Definition.Policy.Cap = 9
	store.byRef[bug631FlowRef] = rec

	svc.startResolvedFlow(context.Background(), parent.RunID, bug631FlowRef, "remount")

	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Cap != 9 {
		t.Fatalf("Cap after remount with raised policy = %d, want 9 (larger policy cap still wins)", st.Cap)
	}
	awaitTournamentChildIdle(t, svc, parent.RunID)
}
