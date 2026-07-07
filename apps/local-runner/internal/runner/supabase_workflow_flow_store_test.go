package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestSupabaseWorkflowFlowStoreGetByRefBuiltinPackFlowID(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotURL string
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		gotURL = endpoint
		rows := []map[string]any{
			{
				"id": "11111111-1111-1111-1111-111111111111", "name": "Review Loop",
				"description": "review until clean", "is_builtin": true, "editable": false,
				"cloneable": true, "pack_id": "flowpilot-core-flow-pack", "pack_version": "0.1.0",
				"pack_flow_id": "review-loop", "pack_hash": "abc123",
				"selectable_in_json": []string{"chat", "flow"}, "chat_baseline": false,
				"chat_sub_modes_json": []string{"bug"}, "policy_cap": 3, "policy_on_cap": "escalate",
				"policy_extend_by": 2, "policy_extend_max": 2, "edges_json": []any{},
				"workflow_steps": []map[string]any{
					{
						"step_type": "flowpilot_core_flow_pack__review_loop__coder", "order_index": 0,
						"step_definitions": map[string]any{
							"step_type": "flowpilot_core_flow_pack__review_loop__coder", "node_id": "coder",
							"node_lifecycle": "reinvoke", "behavior_id": "agent.delegate",
							"agent_ref": "agents/coder.md", "depends_on_json": []string{},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "flowpilot-core-flow-pack/review-loop")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	if !strings.Contains(gotURL, "pack_id=eq.flowpilot-core-flow-pack") || !strings.Contains(gotURL, "pack_flow_id=eq.review-loop") {
		t.Fatalf("expected pack-identity lookup for non-UUID flowRef, got %s", gotURL)
	}
	if record.Source != "supabase_builtin_mirror" {
		t.Fatalf("source = %q, want supabase_builtin_mirror", record.Source)
	}
	if record.FlowRef != "flowpilot-core-flow-pack/review-loop" {
		t.Fatalf("flowRef = %q, want stable pack ref, not the row's raw uuid", record.FlowRef)
	}
	if len(record.Definition.Nodes) != 1 || record.Definition.Nodes[0].ID != "coder" {
		t.Fatalf("unexpected nodes: %#v", record.Definition.Nodes)
	}
	if record.Definition.Nodes[0].Behavior != "agent.delegate" || record.Definition.Nodes[0].Agent != "agents/coder.md" {
		t.Fatalf("unexpected node fields: %#v", record.Definition.Nodes[0])
	}
	if record.Definition.Nodes[0].Lifecycle != "reinvoke" {
		t.Fatalf("node lifecycle = %q, want reinvoke", record.Definition.Nodes[0].Lifecycle)
	}
	// BUG-NOTE-CP42 #21: Definition.ID must be the flow's own semantic id
	// ("review-loop"), matching what the embedded-pack path
	// (builtinRecordFromFlow) produces for the exact same flow — not the
	// row's raw UUID ("11111111-...").
	if record.Definition.ID != "review-loop" {
		t.Fatalf("Definition.ID = %q, want review-loop (matching the embedded-pack normalized shape, not the row's raw uuid)", record.Definition.ID)
	}
	if record.Definition.Policy.Cap != 3 {
		t.Fatalf("policy cap = %d, want 3", record.Definition.Policy.Cap)
	}
}

// TestSupabaseWorkflowFlowStoreGetByRefDecodesInputsOutputsAndContexts is the
// regression test for BUG-NOTE-CP42 #10: rag-harness has flow-level context
// bindings and per-node inputs/outputs, but the store used to only round-trip
// step/node identity fields, silently dropping this data on every mirror
// read. Nothing reads Definition.Contexts/FlowNode.Inputs/Outputs at runtime
// yet, so this bug was previously invisible — a mirrored flow simply lost the
// data with no error anywhere — but it corrupts a mirror's fidelity from the
// source agentpack YAML on every sync.
func TestSupabaseWorkflowFlowStoreGetByRefDecodesInputsOutputsAndContexts(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{
			{
				"id": "44444444-4444-4444-4444-444444444444", "name": "RAG Harness",
				"is_builtin": true, "editable": false, "cloneable": true,
				"pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "rag-harness",
				"edges_json":    []any{},
				"contexts_json": map[string]any{"main_context": map[string]any{"ref": "contexts/flow-context-package.yaml"}},
				"workflow_steps": []map[string]any{
					{
						"step_type": "flowpilot_core_flow_pack__rag_harness__implement", "order_index": 0,
						"step_definitions": map[string]any{
							"step_type": "flowpilot_core_flow_pack__rag_harness__implement", "node_id": "implement",
							"behavior_id": "agent.delegate", "agent_ref": "agents/coder.md", "depends_on_json": []string{"context"},
							"inputs_json":  map[string]any{"main_context": "flow_context_package.v1"},
							"outputs_json": map[string]any{},
						},
					},
					{
						"step_type": "flowpilot_core_flow_pack__rag_harness__context", "order_index": 1,
						"step_definitions": map[string]any{
							"step_type": "flowpilot_core_flow_pack__rag_harness__context", "node_id": "context",
							"behavior_id": "context.produce", "depends_on_json": []string{},
							"inputs_json":  map[string]any{},
							"outputs_json": map[string]any{"main_context": "flow_context_package.v1"},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "flowpilot-core-flow-pack/rag-harness")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}

	binding, ok := record.Definition.Contexts["main_context"]
	if !ok || binding.Ref != "contexts/flow-context-package.yaml" {
		t.Fatalf("Definition.Contexts[main_context] = %#v, ok=%v; want ref contexts/flow-context-package.yaml", binding, ok)
	}

	var implementInputs, contextOutputs map[string]string
	for i := range record.Definition.Nodes {
		n := record.Definition.Nodes[i]
		if n.ID == "implement" {
			implementInputs = n.Inputs
		}
		if n.ID == "context" {
			contextOutputs = n.Outputs
		}
	}
	if implementInputs["main_context"] != "flow_context_package.v1" {
		t.Fatalf("implement node inputs = %#v, want main_context -> flow_context_package.v1", implementInputs)
	}
	if contextOutputs["main_context"] != "flow_context_package.v1" {
		t.Fatalf("context node outputs = %#v, want main_context -> flow_context_package.v1", contextOutputs)
	}
}

func TestSupabaseWorkflowFlowStoreGetByRefUUIDLookup(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotURL string
	httpRequestFn = func(_ context.Context, _ string, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		gotURL = endpoint
		rows := []map[string]any{
			{"id": "22222222-2222-2222-2222-222222222222", "name": "My Flow", "is_builtin": false, "editable": true, "cloneable": true, "edges_json": []any{}},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	if !strings.Contains(gotURL, "id=eq.22222222-2222-2222-2222-222222222222") {
		t.Fatalf("expected id-based lookup for a UUID flowRef, got %s", gotURL)
	}
	if record.Source != "supabase_user_definition" {
		t.Fatalf("source = %q, want supabase_user_definition", record.Source)
	}
}

func TestSupabaseWorkflowFlowStoreUpsertBuiltinUsesPackConflictTarget(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotMethod, gotURL string
	var gotBody []byte
	row := map[string]any{"id": "11111111-1111-1111-1111-111111111111", "name": "Review Loop", "is_builtin": true, "pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "review-loop", "editable": false, "cloneable": true, "edges_json": []any{}}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/workflow_steps") {
			return 201, nil, nil // DELETE existing steps, POST new steps
		}
		if method == http.MethodPost {
			// The one upsert call to /workflows.
			gotMethod, gotURL, gotBody = method, endpoint, body
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		PackID: "flowpilot-core-flow-pack", PackFlowID: "review-loop", Name: "Review Loop", Editable: false, Cloneable: true,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if !strings.Contains(gotURL, "on_conflict=pack_id,pack_flow_id") {
		t.Fatalf("expected pack-identity conflict target, got %s", gotURL)
	}
	var sent map[string]any
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent["is_builtin"] != true || sent["pack_id"] != "flowpilot-core-flow-pack" {
		t.Fatalf("sent body = %#v", sent)
	}
}

// TestSupabaseWorkflowFlowStoreUpsertSendsContextsAndStepDefinitionNodeData is
// the write-side half of BUG-NOTE-CP42 #10 and BUG-236: Upsert's workflows
// payload must carry contexts_json, node definition data must be written to
// step_definitions, and workflow_steps must remain only the relation/order rows.
func TestSupabaseWorkflowFlowStoreUpsertSendsContextsAndNodeInputsOutputs(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var workflowBody, stepDefinitionsBody, stepsBody []byte
	row := map[string]any{"id": "55555555-5555-5555-5555-555555555555", "name": "RAG Harness", "is_builtin": true, "pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "rag-harness", "editable": false, "cloneable": true, "edges_json": []any{}}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/step_definitions") {
			if method == http.MethodPost {
				stepDefinitionsBody = body
			}
			return 201, nil, nil
		}
		if strings.Contains(endpoint, "/workflow_steps") {
			switch method {
			case http.MethodPost:
				stepsBody = body
				var sent []map[string]any
				_ = json.Unmarshal(body, &sent)
				inserted := make([]map[string]any, len(sent))
				for i := range sent {
					inserted[i] = map[string]any{"id": fmt.Sprintf("step-%d", i)}
				}
				b, _ := json.Marshal(inserted)
				return 201, b, nil
			default: // DELETE (supersede old rows) / PATCH (renormalize order_index)
				return 200, nil, nil
			}
		}
		if method == http.MethodPost && strings.Contains(endpoint, "/workflows") {
			workflowBody = body
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		PackID: "flowpilot-core-flow-pack", PackFlowID: "rag-harness", Name: "RAG Harness", Editable: false, Cloneable: true,
		Definition: agentpack.FlowDefinition{
			Contexts: map[string]agentpack.FlowContextBinding{
				"main_context": {Ref: "contexts/flow-context-package.yaml"},
			},
			Nodes: []agentpack.FlowNode{
				{ID: "context", Behavior: "context.produce", Lifecycle: "once", Outputs: map[string]string{"main_context": "flow_context_package.v1"}},
				{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md", DependsOn: []string{"context"},
					Lifecycle: "reinvoke", Inputs: map[string]string{"main_context": "flow_context_package.v1"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if workflowBody == nil {
		t.Fatal("expected a POST to /workflows")
	}
	var sentWorkflow map[string]any
	if err := json.Unmarshal(workflowBody, &sentWorkflow); err != nil {
		t.Fatalf("decode workflow body: %v", err)
	}
	contextsJSON, _ := sentWorkflow["contexts_json"].(map[string]any)
	mainCtx, _ := contextsJSON["main_context"].(map[string]any)
	if mainCtx["ref"] != "contexts/flow-context-package.yaml" {
		t.Fatalf("workflow payload contexts_json = %#v, want main_context.ref set", sentWorkflow["contexts_json"])
	}

	if stepDefinitionsBody == nil {
		t.Fatal("expected a POST to /step_definitions")
	}
	var sentDefinitions []map[string]any
	if err := json.Unmarshal(stepDefinitionsBody, &sentDefinitions); err != nil {
		t.Fatalf("decode step definitions body: %v", err)
	}
	var sawImplementInputs, sawContextOutputs, sawImplementLifecycle, sawContextLifecycle bool
	for _, s := range sentDefinitions {
		if s["node_id"] == "implement" {
			inputs, _ := s["inputs_json"].(map[string]any)
			sawImplementInputs = inputs["main_context"] == "flow_context_package.v1"
			sawImplementLifecycle = s["node_lifecycle"] == "reinvoke"
		}
		if s["node_id"] == "context" {
			outputs, _ := s["outputs_json"].(map[string]any)
			sawContextOutputs = outputs["main_context"] == "flow_context_package.v1"
			sawContextLifecycle = s["node_lifecycle"] == "once"
		}
	}
	if !sawImplementInputs {
		t.Fatalf("implement step definition missing inputs_json.main_context: %#v", sentDefinitions)
	}
	if !sawContextOutputs {
		t.Fatalf("context step definition missing outputs_json.main_context: %#v", sentDefinitions)
	}
	if !sawImplementLifecycle || !sawContextLifecycle {
		t.Fatalf("step definitions missing node_lifecycle mirror: %#v", sentDefinitions)
	}

	if stepsBody == nil {
		t.Fatal("expected a POST to /workflow_steps")
	}
	var sentSteps []map[string]any
	if err := json.Unmarshal(stepsBody, &sentSteps); err != nil {
		t.Fatalf("decode steps body: %v", err)
	}
	for _, s := range sentSteps {
		for _, forbidden := range []string{"node_id", "behavior_id", "agent_ref", "node_lifecycle", "inputs_json", "outputs_json"} {
			if _, exists := s[forbidden]; exists {
				t.Fatalf("workflow_steps payload must not carry node definition field %q: %#v", forbidden, sentSteps)
			}
		}
	}
}

// TestSupabaseWorkflowFlowStoreReplaceStepsInsertsBeforeDeleting is the
// regression test for BUG-NOTE-CP42 #18: replaceSteps used to DELETE every
// existing workflow_steps row before inserting the replacements, so an
// insert failure (FK/unique/network error) permanently lost the old steps —
// the delete had already committed. This asserts the request ordering
// directly: the POST to insert new rows must happen before any DELETE.
func TestSupabaseWorkflowFlowStoreReplaceStepsInsertsBeforeDeleting(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var callOrder []string
	row := map[string]any{"id": "66666666-6666-6666-6666-666666666666", "name": "RAG Harness", "is_builtin": true, "pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "rag-harness", "editable": false, "cloneable": true, "edges_json": []any{}}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/step_definitions") {
			return 201, nil, nil
		}
		if strings.Contains(endpoint, "/workflow_steps") {
			callOrder = append(callOrder, method)
			if method == http.MethodPost {
				var sent []map[string]any
				_ = json.Unmarshal(body, &sent)
				inserted := make([]map[string]any, len(sent))
				for i := range sent {
					inserted[i] = map[string]any{"id": fmt.Sprintf("step-%d", i)}
				}
				b, _ := json.Marshal(inserted)
				return 201, b, nil
			}
			return 200, nil, nil
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		PackID: "flowpilot-core-flow-pack", PackFlowID: "rag-harness", Name: "RAG Harness", Editable: false, Cloneable: true,
		Definition: agentpack.FlowDefinition{
			Nodes: []agentpack.FlowNode{{ID: "context", Behavior: "context.produce"}, {ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"}},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if len(callOrder) == 0 || callOrder[0] != http.MethodPost {
		t.Fatalf("workflow_steps call order = %v, want the first call to be POST (insert new steps before deleting old ones)", callOrder)
	}
	sawDeleteAfterPost := false
	for _, m := range callOrder {
		if m == http.MethodPost {
			continue
		}
		if m == http.MethodDelete {
			sawDeleteAfterPost = true
		}
	}
	if !sawDeleteAfterPost {
		t.Fatalf("expected a DELETE to supersede the old steps after the insert, call order = %v", callOrder)
	}
}

// TestSupabaseWorkflowFlowStoreReplaceStepsInsertFailureLeavesNoDeleteCall
// proves the actual data-loss fix: when the insert fails, replaceSteps must
// return an error WITHOUT ever issuing a DELETE — the pre-fix code deleted
// unconditionally first, so an insert failure meant the existing steps were
// already gone by the time the error surfaced.
func TestSupabaseWorkflowFlowStoreReplaceStepsInsertFailureLeavesNoDeleteCall(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var sawDelete bool
	row := map[string]any{"id": "77777777-7777-7777-7777-777777777777", "name": "RAG Harness", "is_builtin": true, "pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "rag-harness", "editable": false, "cloneable": true, "edges_json": []any{}}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/step_definitions") {
			return 201, nil, nil
		}
		if strings.Contains(endpoint, "/workflow_steps") {
			if method == http.MethodDelete {
				sawDelete = true
			}
			if method == http.MethodPost {
				return 500, []byte("simulated FK violation"), nil
			}
			return 200, nil, nil
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		PackID: "flowpilot-core-flow-pack", PackFlowID: "rag-harness", Name: "RAG Harness", Editable: false, Cloneable: true,
		Definition: agentpack.FlowDefinition{
			Nodes: []agentpack.FlowNode{{ID: "context", Behavior: "context.produce"}},
		},
	})
	if err == nil {
		t.Fatal("expected Upsert to fail when the step insert fails")
	}
	if sawDelete {
		t.Fatal("replaceSteps must not delete existing steps when the insert of new steps failed — the old steps would be permanently lost")
	}
}

func TestSupabaseWorkflowFlowStoreUpsertNewUserRowOmitsID(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotURL string
	var gotBody []byte
	row := map[string]any{"id": "33333333-3333-3333-3333-333333333333", "name": "My Flow", "is_builtin": false, "editable": true, "cloneable": true, "edges_json": []any{}}
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "/workflow_steps") {
			return 201, nil, nil
		}
		if method == http.MethodPost {
			gotURL, gotBody = endpoint, body
		}
		b, _ := json.Marshal([]map[string]any{row})
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	saved, err := store.Upsert(context.Background(), FlowDefinitionRecord{Name: "My Flow", Editable: true, Cloneable: true})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if strings.Contains(gotURL, "on_conflict") {
		t.Fatalf("brand-new row must not request a conflict target, got %s", gotURL)
	}
	var sent map[string]any
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if _, hasID := sent["id"]; hasID {
		t.Fatalf("brand-new row payload must not set id, got %#v", sent)
	}
	if saved.FlowRef != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("saved.FlowRef = %q, want the store-assigned id", saved.FlowRef)
	}
}

func TestFlowDefinitionStoreForNilRunnerYieldsNil(t *testing.T) {
	if store := FlowDefinitionStoreFor(nil); store != nil {
		t.Fatalf("expected nil store for nil runner, got %#v", store)
	}
}

func TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil(t *testing.T) {
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store := FlowDefinitionStoreFor(r); store != nil {
		t.Fatalf("expected nil store for an unconfigured runner, got %#v", store)
	}
}

func TestFlowDefinitionStoreForConfiguredRunnerYieldsSupabaseStore(t *testing.T) {
	tmp := t.TempDir()
	r, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.secretStore = newMemorySecretStore()
	if err := r.secretStore.Set(supabaseServiceRoleSecretKey, "service-role-key"); err != nil {
		t.Fatalf("seed service role key: %v", err)
	}
	cfg := SupabaseWorkspaceConfig{Version: 1, APIURL: "https://proj.supabase.co", AnonKey: "anon-key"}
	raw, _ := json.Marshal(cfg)
	dir := filepath.Join(tmp, ".flowpilot", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "supabase-config.json"), raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, ok := FlowDefinitionStoreFor(r).(*SupabaseWorkflowFlowStore); !ok {
		t.Fatalf("expected SupabaseWorkflowFlowStore, got %T", FlowDefinitionStoreFor(r))
	}
}

// TestFlowDefinitionStoreForAnonKeyOnlyYieldsNil is the regression test for
// BUG-NOTE-CP42 #19: workflows/workflow_steps' RLS policies are scoped `to
// authenticated`, but a bare anon key authenticates as Postgres role `anon`,
// not `authenticated` — every write through an anon-key-only store would be
// silently rejected by RLS. FlowDefinitionStoreFor must require a real
// service role key rather than falling back to a key that can't actually
// write these tables.
func TestFlowDefinitionStoreForAnonKeyOnlyYieldsNil(t *testing.T) {
	tmp := t.TempDir()
	r, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.secretStore = newMemorySecretStore() // no service role key seeded
	cfg := SupabaseWorkspaceConfig{Version: 1, APIURL: "https://proj.supabase.co", AnonKey: "anon-key"}
	raw, _ := json.Marshal(cfg)
	dir := filepath.Join(tmp, ".flowpilot", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "supabase-config.json"), raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if store := FlowDefinitionStoreFor(r); store != nil {
		t.Fatalf("expected nil store when only an anon key is configured (no service role key), got %#v", store)
	}
}

func TestEnsureBuiltinFlowMirrorsWithStoreNilStoreIsNoOp(t *testing.T) {
	synced, err := EnsureBuiltinFlowMirrorsWithStore(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil store, got %v", err)
	}
	if synced != nil {
		t.Fatalf("expected nil result for nil store, got %#v", synced)
	}
}
