package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CP-55 P-1 review pass 2 (Task-263/CA-424): agentpack.FlowDefinition.
// AcceptanceNodes must survive the Supabase-backed user-flow store, not just
// an in-memory load from the embedded pack — otherwise a builtin mirror sync,
// a clone, a user edit, or a fetch/reload silently drops the declared
// acceptance boundary and the P-8 flow migration would fail after reload.
// New file — no pre-existing test is modified.

func TestRecordFromWorkflowRowRestoresAcceptanceNodes(t *testing.T) {
	row := dbWorkflowRow{
		ID:                  "11111111-1111-1111-1111-111111111111",
		Name:                "Writer Flow",
		Editable:            true,
		Cloneable:           true,
		AcceptanceNodesJSON: []string{"validate", "audit"},
		EdgesJSON:           []dbFlowEdgeRow{},
	}
	rec := recordFromWorkflowRow(row)
	if len(rec.Definition.AcceptanceNodes) != 2 || rec.Definition.AcceptanceNodes[0] != "validate" || rec.Definition.AcceptanceNodes[1] != "audit" {
		t.Fatalf("Definition.AcceptanceNodes = %#v, want [validate audit] restored from acceptance_nodes_json", rec.Definition.AcceptanceNodes)
	}
}

// TestRecordFromWorkflowRowNilAcceptanceNodesJSONYieldsEmptyDefinition covers
// a row whose acceptance_nodes_json the JSON decoder left as a nil slice
// (e.g. a legacy row shape in a test fixture that omits the field entirely) —
// this must decode to an empty AcceptanceNodes, not panic or synthesize a
// value, matching every pre-CP-55 flow that never declares one.
func TestRecordFromWorkflowRowNilAcceptanceNodesJSONYieldsEmptyDefinition(t *testing.T) {
	row := dbWorkflowRow{
		ID:        "22222222-2222-2222-2222-222222222222",
		Name:      "Legacy Flow",
		Editable:  true,
		Cloneable: true,
		EdgesJSON: []dbFlowEdgeRow{},
	}
	rec := recordFromWorkflowRow(row)
	if len(rec.Definition.AcceptanceNodes) != 0 {
		t.Fatalf("Definition.AcceptanceNodes = %#v, want empty when acceptance_nodes_json is absent", rec.Definition.AcceptanceNodes)
	}
}

// TestSupabaseWorkflowFlowStoreGetByRefRestoresAcceptanceNodes exercises the
// real fetch path (GetByRef -> fetchOne -> recordFromWorkflowRow), not just
// the unexported restore function directly, using the same fake-transport
// pattern as supabase_workflow_flow_store_test.go's other GetByRef tests.
func TestSupabaseWorkflowFlowStoreGetByRefRestoresAcceptanceNodes(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{
			{
				"id": "33333333-3333-3333-3333-333333333333", "name": "Writer Flow",
				"is_builtin": false, "editable": true, "cloneable": true,
				"edges_json":            []any{},
				"acceptance_nodes_json": []string{"validate"},
			},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	if len(record.Definition.AcceptanceNodes) != 1 || record.Definition.AcceptanceNodes[0] != "validate" {
		t.Fatalf("Definition.AcceptanceNodes = %#v, want [validate] restored through the real fetch path", record.Definition.AcceptanceNodes)
	}
}

// TestSupabaseWorkflowFlowStoreUpsertSendsAcceptanceNodesJSON is the
// write-side counterpart: Upsert's /workflows payload must carry
// acceptance_nodes_json, mirroring how selectable_in_json/chat_sub_modes_json
// are already sent.
func TestSupabaseWorkflowFlowStoreUpsertSendsAcceptanceNodesJSON(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var workflowBody []byte
	row := map[string]any{
		"id": "44444444-4444-4444-4444-444444444444", "name": "Writer Flow",
		"is_builtin": true, "pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "writer-flow",
		"editable": false, "cloneable": true, "edges_json": []any{},
		"acceptance_nodes_json": []string{"validate"},
	}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/step_definitions") {
			return 201, nil, nil
		}
		if strings.Contains(endpoint, "/workflow_steps") {
			return 201, []byte("[]"), nil
		}
		if method == http.MethodPost && strings.Contains(endpoint, "/workflows") {
			workflowBody = body
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	saved, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		PackID: "flowpilot-core-flow-pack", PackFlowID: "writer-flow", Name: "Writer Flow", Editable: false, Cloneable: true,
		Definition: agentpack.FlowDefinition{
			AcceptanceNodes: []string{"validate"},
			Nodes: []agentpack.FlowNode{
				{ID: "freeze", Behavior: "contract.freeze"},
				{ID: "writer", Behavior: "agent.code"},
				{ID: "validate", Behavior: "command.validate"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if workflowBody == nil {
		t.Fatal("expected a POST to /workflows")
	}
	var sent map[string]any
	if err := json.Unmarshal(workflowBody, &sent); err != nil {
		t.Fatalf("decode workflow body: %v", err)
	}
	got, ok := sent["acceptance_nodes_json"].([]any)
	if !ok || len(got) != 1 || got[0] != "validate" {
		t.Fatalf("sent acceptance_nodes_json = %#v, want [\"validate\"]", sent["acceptance_nodes_json"])
	}

	// Upsert re-fetches the saved row (fetchOne) before returning, so the
	// returned record's own AcceptanceNodes proves the full write-then-read
	// round trip through one call, not just the raw outbound payload.
	if len(saved.Definition.AcceptanceNodes) != 1 || saved.Definition.AcceptanceNodes[0] != "validate" {
		t.Fatalf("saved.Definition.AcceptanceNodes = %#v, want [validate] after the post-upsert re-fetch", saved.Definition.AcceptanceNodes)
	}
}

// TestSupabaseWorkflowFlowStoreUpsertSendsEmptyAcceptanceNodesAsJSONArray
// proves nonNilStrings' existing nil-to-[]-normalization also covers
// AcceptanceNodes: acceptance_nodes_json is `not null default '[]'::jsonb`
// (20260730121000_add_workflow_acceptance_nodes.sql), so a Go JSON `null`
// would violate that constraint against a real database — every legacy flow
// with no declared acceptance boundary must send `[]`, not `null`.
func TestSupabaseWorkflowFlowStoreUpsertSendsEmptyAcceptanceNodesAsJSONArray(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var workflowBody []byte
	row := map[string]any{
		"id": "55555555-5555-5555-5555-555555555555", "name": "Legacy Flow",
		"is_builtin": false, "editable": true, "cloneable": true, "edges_json": []any{},
	}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/workflow_steps") {
			return 201, []byte("[]"), nil
		}
		if method == http.MethodPost && strings.Contains(endpoint, "/workflows") {
			workflowBody = body
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{Name: "Legacy Flow", Editable: true, Cloneable: true})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if workflowBody == nil {
		t.Fatal("expected a POST to /workflows")
	}
	if strings.Contains(string(workflowBody), `"acceptance_nodes_json":null`) {
		t.Fatalf("acceptance_nodes_json must never be sent as JSON null (violates the not-null jsonb column default), got: %s", workflowBody)
	}
	var sent map[string]any
	if err := json.Unmarshal(workflowBody, &sent); err != nil {
		t.Fatalf("decode workflow body: %v", err)
	}
	got, ok := sent["acceptance_nodes_json"].([]any)
	if !ok || len(got) != 0 {
		t.Fatalf("sent acceptance_nodes_json = %#v, want an empty JSON array", sent["acceptance_nodes_json"])
	}
}

// TestBuiltinRecordFromFlowPreservesAcceptanceNodes proves the whole-struct
// Definition copy builtinRecordFromFlow already does (Definition: def)
// carries AcceptanceNodes through untouched. This is the exact function both
// ResolveBuiltin's embedded-pack fallback and FlowMirrorSyncService.SyncBuiltins
// use to build the record that then flows into Upsert, so CloneBuiltin's
// clone.Definition = source.Definition and UpdateUserFlow's pass-through
// record both preserve AcceptanceNodes for the same reason, with no separate
// code change required in any of the three.
func TestBuiltinRecordFromFlowPreservesAcceptanceNodes(t *testing.T) {
	manifest := agentpack.Manifest{ID: "flowpilot-core-flow-pack", Version: "0.1.0"}
	def := agentpack.FlowDefinition{
		ID:              "writer-flow",
		AcceptanceNodes: []string{"validate"},
		Nodes: []agentpack.FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
	}
	rec := builtinRecordFromFlow(manifest, def, []byte("raw yaml content"))
	if len(rec.Definition.AcceptanceNodes) != 1 || rec.Definition.AcceptanceNodes[0] != "validate" {
		t.Fatalf("builtinRecordFromFlow Definition.AcceptanceNodes = %#v, want [validate] preserved from the source FlowDefinition", rec.Definition.AcceptanceNodes)
	}
}
