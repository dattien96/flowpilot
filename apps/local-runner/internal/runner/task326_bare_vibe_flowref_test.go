package runner

import (
	"context"
	"testing"
)

func TestResolveFlowRef_BareVibeIngest(t *testing.T) {
	rec, err := NewFlowDefinitionResolver(nil).ResolveFlowRef(context.Background(), "vibe-ingest")
	if err != nil {
		t.Fatalf("ResolveFlowRef vibe-ingest: %v", err)
	}
	if rec.Definition.ID != "vibe-ingest" && rec.PackFlowID != "vibe-ingest" {
		t.Fatalf("record=%+v", rec)
	}
}

func TestExplicitFlowRefResolves_BareVibeIngest(t *testing.T) {
	svc := NewInteractiveService()
	if !svc.explicitFlowRefResolves(context.Background(), "vibe-ingest") {
		t.Fatal("bare vibe-ingest must resolve so first turn is not demoted to chat")
	}
}
