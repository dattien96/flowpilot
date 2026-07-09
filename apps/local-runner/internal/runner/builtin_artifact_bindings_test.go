package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func builtinContextFlowRecordFixture() FlowDefinitionRecord {
	return FlowDefinitionRecord{
		FlowRef:     canonicalFlowRef("flowpilot-core-flow-pack", builtinContextCodingReviewSynthesisFlowID),
		PackID:      "flowpilot-core-flow-pack",
		PackFlowID:  builtinContextCodingReviewSynthesisFlowID,
		PackVersion: "0.1.0",
		Definition: agentpack.FlowDefinition{
			ID: builtinContextCodingReviewSynthesisFlowID,
			Nodes: []agentpack.FlowNode{
				{ID: "context", Behavior: "context.produce"},
				{ID: "coder", Behavior: "agent.delegate"},
				{ID: "reviewer_correctness", Behavior: "agent.delegate"},
				{ID: "reviewer_security", Behavior: "agent.delegate"},
				{ID: "synthesis", Behavior: "hub.inline"},
			},
		},
	}
}

// TestSeedBuiltinContextArtifactBindingsBindsOutputOnContextAndInputElsewhere
// verifies Task-205's core claim: the context node's binding is OUTPUT (SD-23
// D-5) and every other node's binding to the same built-in instance is INPUT.
func TestSeedBuiltinContextArtifactBindingsBindsOutputOnContextAndInputElsewhere(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	type gotRow struct {
		StepDefinitionID   string `json:"step_definition_id"`
		Direction          string `json:"direction"`
		ArtifactInstanceID string `json:"artifact_instance_id"`
		Required           bool   `json:"required"`
	}
	var gotRows []gotRow
	var gotEndpoint string
	httpRequestFn = func(_ context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		gotEndpoint = endpoint
		if method != http.MethodPost {
			t.Fatalf("expected POST, got %s", method)
		}
		if headers["Prefer"] != "resolution=merge-duplicates,return=minimal" {
			t.Fatalf("expected merge-duplicates upsert Prefer header, got %q", headers["Prefer"])
		}
		if err := json.Unmarshal(body, &gotRows); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return 201, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "test-key")
	if err := store.SeedBuiltinContextArtifactBindings(context.Background(), builtinContextFlowRecordFixture()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotEndpoint, "/step_artifact_bindings") {
		t.Fatalf("expected step_artifact_bindings endpoint, got %q", gotEndpoint)
	}
	if !strings.Contains(gotEndpoint, "on_conflict=step_definition_id,direction,artifact_instance_id") {
		t.Fatalf("expected on_conflict upsert clause, got %q", gotEndpoint)
	}
	if len(gotRows) != 5 {
		t.Fatalf("expected 5 binding rows (one per node), got %d", len(gotRows))
	}

	byStepSuffix := make(map[string]gotRow)
	for _, row := range gotRows {
		byStepSuffix[row.StepDefinitionID] = row
	}

	var contextRow, coderRow gotRow
	for stepType, row := range byStepSuffix {
		// sanitizeStepType (supabase_workflow_flow_store.go) collapses the
		// "__" separator flowNodeStepType builds with down to a single "_",
		// so the resolved step_type ends in "_context"/"_coder", not "__...".
		if strings.HasSuffix(stepType, "_context") {
			contextRow = row
		}
		if strings.HasSuffix(stepType, "_coder") {
			coderRow = row
		}
		if row.ArtifactInstanceID != builtinContextArtifactInstanceID {
			t.Fatalf("row %q bound to wrong instance %q", stepType, row.ArtifactInstanceID)
		}
	}
	if contextRow.Direction != "output" {
		t.Fatalf("expected context node binding direction=output, got %q", contextRow.Direction)
	}
	if !contextRow.Required {
		t.Fatal("expected context node (output) binding to be required")
	}
	if coderRow.Direction != "input" {
		t.Fatalf("expected coder node binding direction=input, got %q", coderRow.Direction)
	}
}

// TestEnsureBuiltinArtifactBindingsWithStoreOnlyActsOnTargetFlow verifies
// EnsureBuiltinArtifactBindingsWithStore skips every synced flow except
// context-coding-review-synthesis (e.g. review-loop, rag-harness never get
// spurious bindings seeded).
func TestEnsureBuiltinArtifactBindingsWithStoreOnlyActsOnTargetFlow(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	calls := 0
	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		calls++
		return 201, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "test-key")
	synced := []FlowDefinitionRecord{
		{PackFlowID: "review-loop", Definition: agentpack.FlowDefinition{Nodes: []agentpack.FlowNode{{ID: "coder"}}}},
		{PackFlowID: "rag-harness", Definition: agentpack.FlowDefinition{Nodes: []agentpack.FlowNode{{ID: "context"}}}},
	}
	if err := EnsureBuiltinArtifactBindingsWithStore(context.Background(), store, synced); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no HTTP calls for non-target flows, got %d", calls)
	}

	synced = append(synced, builtinContextFlowRecordFixture())
	if err := EnsureBuiltinArtifactBindingsWithStore(context.Background(), store, synced); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 HTTP call for the target flow, got %d", calls)
	}
}

// TestEnsureBuiltinArtifactBindingsWithStoreNoopsOnUnsupportedStore verifies
// a store without the seeder capability (e.g. a test fake) is a safe no-op,
// mirroring how SyncBuiltins already treats builtinStaleMirrorReclaimer as
// optional.
func TestEnsureBuiltinArtifactBindingsWithStoreNoopsOnUnsupportedStore(t *testing.T) {
	if err := EnsureBuiltinArtifactBindingsWithStore(context.Background(), nil, []FlowDefinitionRecord{builtinContextFlowRecordFixture()}); err != nil {
		t.Fatalf("expected nil store to no-op, got %v", err)
	}
}
