package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA815_BootScanSkipsFlowReconstruct(t *testing.T) {
	svc, _ := newTestServer(t)
	store, ok := svc.workflowStore.(interface {
		UpsertProviderSession(context.Context, ProviderSessionState) error
	})
	if !ok {
		t.Fatal("store must upsert sessions")
	}
	st := ProviderSessionState{
		RunID:       "run-flow-boot",
		ProjectID:   "proj",
		ProviderKey: ProviderKeyCodex,
		RunKind:     "chat",
		WorkingMode: workingmode.Vibe,
		Status:      RunStatusCancelled,
		StartedAt:   "2026-09-09T00:00:00Z",
		UpdatedAt:   "2026-09-09T00:05:00Z",
		ActiveFlowNodes: []agentpack.FlowNode{
			{ID: "audit", Behavior: "artifact.audit_draft"},
		},
	}
	if err := store.UpsertProviderSession(context.Background(), st); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	svc.ScanPersistedChatsForSummaries(context.Background())
	svc.mu.Lock()
	_, live := svc.runs["run-flow-boot"]
	svc.mu.Unlock()
	if live {
		t.Fatal("boot summary scan must not reconstruct flow sessions")
	}
}
