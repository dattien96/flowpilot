package runner

import (
	"context"
	"testing"
)

// Quota route events are runner-emitted routing decisions — provider
// transcripts have no concept of them, so excluding them from the flow-events
// sidecar loses the repin/stop/blocked evidence entirely on restart. They
// belong to the same durable-replay class as question/permission cards.
func TestQuotaRouteEventsPersistToFlowSidecar(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	for _, evType := range []ProviderEventType{
		EventQuotaRouteCommitted, EventQuotaRouteStopped, EventQuotaRouteBlocked,
	} {
		if err := store.AppendEvent(context.Background(), ProviderEvent{
			Type:          evType,
			WorkflowRunID: "run-quota",
		}); err != nil {
			t.Fatalf("AppendEvent %s: %v", evType, err)
		}
	}

	evs, err := store.LoadFlowEvents(context.Background(), "run-quota")
	if err != nil {
		t.Fatalf("LoadFlowEvents: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("LoadFlowEvents = %d events, want 3 route decisions replayed; got %+v", len(evs), evs)
	}
}
