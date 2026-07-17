package runner

import (
	"context"
	"testing"
)

// CP-51 Task-252 (DOD-I4 + durable provenance). These tests exercise the
// actual decision surface — isFlowContextHandoffWithSecret(secret, prompt,
// allowedIDs...) — directly, rather than through injectFeatureHistoryPromptCtx's
// full body (which needs a real .flowpilot feature catalog on disk to show an
// observable difference; the marker decision itself doesn't).

func markerPrompt(secret []byte, id string) string {
	return "Continue the task.\n" + flowContextTrustedMarkerWith(secret, id) + "\ncarry on"
}

func TestFCPMarker_OwnRun_Suppresses(t *testing.T) {
	secret := []byte("service-secret")
	prompt := markerPrompt(secret, "run-a")
	if !isFlowContextHandoffWithSecret(secret, prompt, allowedFCPMarkerIDs(&interactiveRun{id: "run-a"})...) {
		t.Fatal("a marker bound to rs.id must suppress on the outer AND inner check")
	}
}

func TestFCPMarker_SameServiceForeignRun_DoesNotSuppress(t *testing.T) {
	// DOD-I4: a marker minted by THIS service for run-b must not suppress
	// history when verifying for run-a — same secret, different id, no
	// recorded provenance binding run-b to run-a.
	secret := []byte("service-secret")
	prompt := markerPrompt(secret, "run-b")
	rs := &interactiveRun{id: "run-a"} // no provenance recorded
	if isFlowContextHandoffWithSecret(secret, prompt, allowedFCPMarkerIDs(rs)...) {
		t.Fatal("same-service foreign-run marker must NOT suppress run-a's history")
	}
}

func TestFCPMarker_SiblingWithoutProvenance_DoesNotSuppress(t *testing.T) {
	// Topology alone (parentRunID matching the marker's id) must never be
	// trusted — only a recorded provenance stamp may.
	secret := []byte("service-secret")
	prompt := markerPrompt(secret, "hub-1")
	child := &interactiveRun{id: "child-x", parentRunID: "hub-1"} // sibling/child of hub-1, but NOT stamped
	if isFlowContextHandoffWithSecret(secret, prompt, allowedFCPMarkerIDs(child)...) {
		t.Fatal("parentRunID topology alone must not suppress — only recorded provenance may")
	}
}

func TestFCPMarker_RecordedProvenance_Suppresses(t *testing.T) {
	// The legitimate Task-224/BUG-277 handoff case: provenance WAS recorded
	// (as spawnChildRun now stamps via FCPMarkerProvenanceRunID), so the same
	// marker DOES suppress.
	secret := []byte("service-secret")
	prompt := markerPrompt(secret, "hub-1")
	child := &interactiveRun{id: "child-x", parentRunID: "hub-1", markerProvenanceRunIDs: []string{"hub-1"}}
	if !isFlowContextHandoffWithSecret(secret, prompt, allowedFCPMarkerIDs(child)...) {
		t.Fatal("recorded provenance (hub-1) must suppress the legitimate handoff")
	}
}

func TestFCPMarker_ForeignSecret_NeverSuppresses(t *testing.T) {
	// R20-2 guard: a marker minted with a DIFFERENT service's secret must never
	// suppress, even for an otherwise-allowed id — HMAC verification must fail.
	foreignSecret := []byte("other-service-secret")
	prompt := markerPrompt(foreignSecret, "run-a")
	if isFlowContextHandoffWithSecret([]byte("service-secret"), prompt, allowedFCPMarkerIDs(&interactiveRun{id: "run-a"})...) {
		t.Fatal("a foreign-secret marker must never suppress, regardless of id match")
	}
}

// TestSpawnChildRun_StampsFCPMarkerProvenance is the end-to-end mint-time
// stamping proof: spawnChildRun with FCPMarkerProvenanceRunID set records it
// on the new child, and the child's own allowedFCPMarkerIDs (hence its future
// turns' handoff verification) includes it.
func TestSpawnChildRun_StampsFCPMarkerProvenance(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, _ TurnRequest, _ TurnBridge) error {
				<-ctx.Done()
				return ctx.Err()
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}

	result, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:                    "coder",
		Prompt:                   "do the task",
		Provider:                 "codex",
		FCPMarkerProvenanceRunID: parent.RunID,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}

	svc.mu.Lock()
	child := svc.runs[result.RunID]
	svc.mu.Unlock()
	if child == nil {
		t.Fatal("spawned child not found")
	}
	ids := allowedFCPMarkerIDs(child)
	found := false
	for _, id := range ids {
		if id == parent.RunID {
			found = true
		}
	}
	if !found {
		t.Fatalf("child's allowedFCPMarkerIDs %v must include the spawn-time provenance (parent %s)", ids, parent.RunID)
	}
}

// TestProvenance_RoundTripsAcrossRestart proves the recorded provenance
// survives a REAL disk round-trip (both markerProvenanceRunIDs and the
// Pending*ProvenanceRunID fields) and is still honored after reconstruction —
// closing the "encoder never writes them" gap found in the 2026-07-17 audit.
func TestProvenance_RoundTripsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	rs := &interactiveRun{
		id:                                 "child-x",
		parentRunID:                        "hub-1",
		providerKey:                        ProviderKeyCodex,
		markerProvenanceRunIDs:             []string{"hub-1"},
		pendingRestartProvenanceRunID:      "hub-1",
		pendingGateRepromptProvenanceRunID: "hub-2",
	}
	snap := sessionStateOf(rs)
	snap.RunID = "child-x"
	snap.ProjectID = "p"
	if err := store.UpsertProviderSession(context.Background(), snap); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	loaded, found, err := store2.GetProviderSession(context.Background(), "child-x")
	if err != nil || !found {
		t.Fatalf("GetProviderSession: found=%v err=%v", found, err)
	}
	if len(loaded.MarkerProvenanceRunIDs) != 1 || loaded.MarkerProvenanceRunIDs[0] != "hub-1" {
		t.Fatalf("MarkerProvenanceRunIDs did not survive round-trip: %v", loaded.MarkerProvenanceRunIDs)
	}
	if loaded.PendingRestartProvenanceRunID != "hub-1" {
		t.Fatalf("PendingRestartProvenanceRunID did not survive round-trip: %q", loaded.PendingRestartProvenanceRunID)
	}
	if loaded.PendingGateRepromptProvenanceRunID != "hub-2" {
		t.Fatalf("PendingGateRepromptProvenanceRunID did not survive round-trip: %q", loaded.PendingGateRepromptProvenanceRunID)
	}

	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	reconstructed, apiErr := svc.reconstructRun(loaded)
	if apiErr != nil || reconstructed == nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	ids := allowedFCPMarkerIDs(reconstructed)
	found2 := false
	for _, id := range ids {
		if id == "hub-1" {
			found2 = true
		}
	}
	if !found2 {
		t.Fatalf("reconstructed run's allowedFCPMarkerIDs %v must still include recorded provenance after restart", ids)
	}
}
