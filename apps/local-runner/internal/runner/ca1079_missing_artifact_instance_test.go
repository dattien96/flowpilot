package runner

// CA-1079: builtinHarnessArtifactInstanceIDs maps slot tdd_signatures →
// instance ...0005, but no migration ever seeded that artifact_instances
// row — a remote missing it 409s the WHOLE seed batch with FK 23503
// (live: "Key (artifact_instance_id)=(...0005) is not present"), starving
// the flow's other bindings. The seed must drop only the rows whose
// instance is missing and retry — degrade-soft, same class as CA-1075's
// unapplied-migration handling.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// ca1079FKBody mimics PostgREST's 23503 detail shape exactly.
const ca1079FKBody = `{"code":"23503","details":"Key (artifact_instance_id)=(` +
	builtinHarnessTddSignaturesInstanceID + `) is not present in table \"artifact_instances\".","hint":null,` +
	`"message":"insert or update on table \"step_artifact_bindings\" violates foreign key constraint \"step_artifact_bindings_artifact_instance_id_fkey\""}`

func ca1079Record(bindings ...agentpack.FlowArtifactBinding) FlowDefinitionRecord {
	return FlowDefinitionRecord{
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "vibe-sprint",
		FlowRef:    "flowpilot-core-flow-pack/vibe-sprint",
		Definition: agentpack.FlowDefinition{
			ID: "vibe-sprint",
			Nodes: []agentpack.FlowNode{{
				ID:               "tdd",
				Behavior:         "agent.scaffold",
				ArtifactBindings: bindings,
			}},
		},
	}
}

// Scenario: one missing instance must not kill sibling bindings in the
// same batch.
// Input: tdd node binding cp_md (exists) + tdd_signatures (remote lacks
// ...0005); first POST 409s 23503 naming ...0005.
// Expect: retry POST without the ...0005 row; nil error; 2 POSTs total.
func TestCA1079_SeedDropsMissingInstanceAndRetries(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var postBodies [][]byte
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if method != http.MethodPost || !strings.Contains(endpoint, "/step_artifact_bindings") {
			t.Fatalf("unexpected request: %s %s", method, endpoint)
		}
		postBodies = append(postBodies, append([]byte(nil), body...))
		if len(postBodies) == 1 {
			return http.StatusConflict, []byte(ca1079FKBody), nil
		}
		return http.StatusCreated, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "k")
	rec := ca1079Record(
		agentpack.FlowArtifactBinding{Direction: "input", SlotName: "cp_in", ArtifactInstanceID: "cp_md", Required: true},
		agentpack.FlowArtifactBinding{Direction: "output", SlotName: "sigs", ArtifactInstanceID: "tdd_signatures", Required: true},
	)
	if err := store.SeedBuiltinHarnessArtifactBindings(context.Background(), rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(postBodies) != 2 {
		t.Fatalf("POSTs = %d, want 2 (first 23503, retry without the missing instance)", len(postBodies))
	}
	var retryRows []map[string]any
	if err := json.Unmarshal(postBodies[1], &retryRows); err != nil {
		t.Fatalf("decode retry body: %v", err)
	}
	if len(retryRows) != 1 {
		t.Fatalf("retry rows = %d, want 1 (only the existing cp_md binding)", len(retryRows))
	}
	if got := retryRows[0]["artifact_instance_id"]; got != builtinHarnessCpMdInstanceID {
		t.Fatalf("retry artifact_instance_id = %v, want %s", got, builtinHarnessCpMdInstanceID)
	}
}

// Scenario: a flow whose ONLY binding points at the missing instance
// degrades to a no-op instead of surfacing the 23503.
// Expect: nil error; exactly one POST (no retry — nothing left to send).
func TestCA1079_SeedAllRowsMissingIsNoOp(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	posts := 0
	httpRequestFn = func(_ context.Context, method, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		if method == http.MethodPost {
			posts++
			return http.StatusConflict, []byte(ca1079FKBody), nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "k")
	rec := ca1079Record(
		agentpack.FlowArtifactBinding{Direction: "output", SlotName: "sigs", ArtifactInstanceID: "tdd_signatures", Required: true},
	)
	if err := store.SeedBuiltinHarnessArtifactBindings(context.Background(), rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if posts != 1 {
		t.Fatalf("POSTs = %d, want 1 (drop the missing row, then nothing left to send)", posts)
	}
}

// Scenario: a non-FK failure must still propagate — the retry path is
// scoped to 23503 artifact_instance misses only.
// Expect: error surfaced, single POST.
func TestCA1079_NonFKFailureStillPropagates(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	posts := 0
	httpRequestFn = func(_ context.Context, method, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		if method == http.MethodPost {
			posts++
			return http.StatusInternalServerError, []byte(`{"code":"boom"}`), nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "k")
	rec := ca1079Record(
		agentpack.FlowArtifactBinding{Direction: "output", SlotName: "sigs", ArtifactInstanceID: "tdd_signatures", Required: true},
	)
	if err := store.SeedBuiltinHarnessArtifactBindings(context.Background(), rec); err == nil {
		t.Fatal("want error on non-23503 failure, got nil")
	}
	if posts != 1 {
		t.Fatalf("POSTs = %d, want 1 (no retry on non-FK failure)", posts)
	}
}
