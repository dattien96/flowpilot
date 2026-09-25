package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-474: a cloned/user-owned flow must be execution-equivalent to its source
// after save→reload. step_definitions has no columns for run/posture/
// contextProfile/config and workflows has none for tools/contextProfiles, so
// Upsert must persist a definition snapshot (workflows.definition_json) and
// recordFromWorkflowRow must restore the schema-less fields from it — with
// real columns keeping precedence over the snapshot exactly like the
// built-in/embedded-pack path.

func bug474SnapshotDef() agentpack.FlowDefinition {
	return agentpack.FlowDefinition{
		ID:          "cp-harness-smoke-copy",
		Description: "cloned harness",
		Tools:       []string{"flowpilot_submit_review_outcome"},
		ContextProfiles: map[string]agentpack.ContextProfile{
			"scout": {Name: "scout", CandidateSources: []string{"artifact_context"}, MaxTokens: 512},
		},
		Nodes: []agentpack.FlowNode{
			{
				ID:             "guard",
				Run:            "inline",
				Behavior:       "agent.delegate",
				Agent:          "agents/guard.md",
				Posture:        "read_only",
				ContextProfile: "scout",
				Config:         map[string]any{"max_attempts": 2, "candidates": []any{"a", "b"}},
			},
			{
				ID:       "writer",
				Run:      "delegate",
				Behavior: "agent.delegate",
				Agent:    "agents/coder.md",
				Model:    "pack-model",
			},
		},
		Edges: []agentpack.FlowEdge{{From: "guard", To: "writer", When: "done", Kind: "forward"}},
	}
}

func bug474RowForSnapshot(t *testing.T, snapshot agentpack.FlowDefinition) map[string]any {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return map[string]any{
		"id": "aaaaaaaa-1111-2222-3333-444444444444", "name": "Smoke Clone",
		"is_builtin": false, "editable": true, "cloneable": true,
		"edges_json": []any{
			map[string]any{"from": "guard", "to": "writer", "when": "done", "kind": "forward"},
		},
		"definition_json": json.RawMessage(raw),
		"workflow_steps": []map[string]any{
			{
				"step_type": "clone_guard", "order_index": 0,
				"step_definitions": map[string]any{
					"step_type": "clone_guard", "node_id": "guard",
					"behavior_id": "agent.delegate", "agent_ref": "agents/guard.md",
				},
			},
			{
				"step_type": "clone_writer", "order_index": 1,
				"step_definitions": map[string]any{
					"step_type": "clone_writer", "node_id": "writer",
					"behavior_id": "agent.delegate", "agent_ref": "agents/coder.md",
				},
			},
		},
	}
}

func TestBUG474_ClonedFlowRestoresExecutionFieldsFromSnapshot(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{bug474RowForSnapshot(t, bug474SnapshotDef())}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "aaaaaaaa-1111-2222-3333-444444444444")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	def := record.Definition

	nodes := map[string]agentpack.FlowNode{}
	for _, n := range def.Nodes {
		nodes[n.ID] = n
	}
	guard := nodes["guard"]
	if guard.Posture != "read_only" {
		t.Fatalf("guard.Posture = %q — snapshot restore dropped read_only posture (BUG-474)", guard.Posture)
	}
	if guard.ContextProfile != "scout" {
		t.Fatalf("guard.ContextProfile = %q, want scout", guard.ContextProfile)
	}
	if guard.Config == nil || guard.Config["max_attempts"] != float64(2) {
		t.Fatalf("guard.Config = %#v, want max_attempts=2", guard.Config)
	}
	if guard.Run != "inline" {
		t.Fatalf("guard.Run = %q — snapshot declared inline but reload derived delegate", guard.Run)
	}
	writer := nodes["writer"]
	if writer.Model != "pack-model" {
		t.Fatalf("writer.Model = %q — snapshot-declared model must restore when the row column is empty", writer.Model)
	}
	if len(def.Tools) != 1 || def.Tools[0] != "flowpilot_submit_review_outcome" {
		t.Fatalf("def.Tools = %#v — flow-level tools dropped", def.Tools)
	}
	if _, ok := def.ContextProfiles["scout"]; !ok {
		t.Fatalf("def.ContextProfiles = %#v — profile map dropped; node ref 'scout' is now dangling", def.ContextProfiles)
	}
}

func TestBUG474_UpsertClonedFlowWritesDefinitionSnapshot(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var workflowBody []byte
	var sawWorkflowPost bool
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		switch {
		case method == http.MethodPost && strings.Contains(endpoint, "/workflows?") || method == http.MethodPost && strings.HasSuffix(endpoint, "/workflows"):
			sawWorkflowPost = true
			workflowBody = append([]byte(nil), body...)
			rows := []map[string]any{
				{"id": "bbbbbbbb-1111-2222-3333-444444444444", "name": "Smoke Clone",
					"is_builtin": false, "editable": true, "cloneable": true,
					"edges_json": []any{}, "workflow_steps": []any{}},
			}
			b, _ := json.Marshal(rows)
			return 201, b, nil
		case method == http.MethodPost && strings.Contains(endpoint, "/step_definitions"):
			return 201, []byte("[]"), nil
		case method == http.MethodPost && strings.Contains(endpoint, "/workflow_steps"):
			return 201, []byte("[]"), nil
		case method == http.MethodGet:
			rows := []map[string]any{
				{"id": "bbbbbbbb-1111-2222-3333-444444444444", "name": "Smoke Clone",
					"is_builtin": false, "editable": true, "cloneable": true,
					"edges_json": []any{}, "workflow_steps": []any{}},
			}
			b, _ := json.Marshal(rows)
			return 200, b, nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	_, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		Name: "Smoke Clone", Source: "supabase_user_definition",
		Editable: true, Cloneable: true,
		Definition: bug474SnapshotDef(),
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !sawWorkflowPost {
		t.Fatal("no workflows POST captured")
	}
	var payload map[string]any
	if err := json.Unmarshal(workflowBody, &payload); err != nil {
		t.Fatalf("decode workflows payload: %v", err)
	}
	raw, present := payload["definition_json"]
	if !present {
		t.Fatal("workflows upsert wrote no definition_json — clone snapshot not persisted (BUG-474)")
	}
	rawBytes, _ := json.Marshal(raw)
	var snap agentpack.FlowDefinition
	if err := json.Unmarshal(rawBytes, &snap); err != nil {
		t.Fatalf("definition_json not a FlowDefinition: %v", err)
	}
	if len(snap.Nodes) != 2 || snap.Nodes[0].Posture != "read_only" {
		t.Fatalf("snapshot nodes = %#v — posture lost in persisted snapshot", snap.Nodes)
	}
	if _, ok := snap.ContextProfiles["scout"]; !ok {
		t.Fatalf("snapshot ContextProfiles missing scout: %#v", snap.ContextProfiles)
	}
}

func TestBUG474_BuiltinRowIgnoresDefinitionSnapshot(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	// A builtin mirror row must keep the embedded pack as the authority for
	// schema-less fields; a stray definition_json on the row must not become
	// a second authority that can drift from the pack.
	bogus := agentpack.FlowDefinition{
		ID:    "review-loop",
		Nodes: []agentpack.FlowNode{{ID: "coder", Posture: "verdict_only"}},
	}
	raw, err := json.Marshal(bogus)
	if err != nil {
		t.Fatalf("marshal bogus snapshot: %v", err)
	}
	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{
			{
				"id": "cccccccc-1111-2222-3333-444444444444", "name": "Review Loop",
				"is_builtin": true, "editable": false, "cloneable": true,
				"pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "review-loop",
				"edges_json": []any{},
				"definition_json": json.RawMessage(raw),
				"workflow_steps": []map[string]any{
					{
						"step_type": "flowpilot_core_flow_pack__review_loop__coder", "order_index": 0,
						"step_definitions": map[string]any{
							"step_type": "flowpilot_core_flow_pack__review_loop__coder", "node_id": "coder",
							"behavior_id": "agent.delegate", "agent_ref": "agents/coder.md",
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
	for _, n := range record.Definition.Nodes {
		if n.Posture == "verdict_only" {
			t.Fatalf("builtin row honored definition_json snapshot — embedded pack must stay the sole authority")
		}
	}
}

func TestBUG474_LegacyCloneWithoutSnapshotDerivesRun(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	// Rows written before definition_json existed have already lost the
	// schema-less fields. The deterministic legacy contract is: keep the
	// derive-run reconstruction (delegate for agent.*, inline otherwise)
	// rather than fail the load or invent fields.
	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{
			{
				"id": "dddddddd-1111-2222-3333-444444444444", "name": "Old Clone",
				"is_builtin": false, "editable": true, "cloneable": true,
				"edges_json": []any{},
				"workflow_steps": []map[string]any{
					{
						"step_type": "old_guard", "order_index": 0,
						"step_definitions": map[string]any{
							"step_type": "old_guard", "node_id": "guard",
							"behavior_id": "agent.delegate", "agent_ref": "agents/guard.md",
						},
					},
					{
						"step_type": "old_marker", "order_index": 1,
						"step_definitions": map[string]any{
							"step_type": "old_marker", "node_id": "marker",
						},
					},
				},
			},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "dddddddd-1111-2222-3333-444444444444")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	runs := map[string]string{}
	for _, n := range record.Definition.Nodes {
		runs[n.ID] = n.Run
	}
	if runs["guard"] != "delegate" || runs["marker"] != "inline" {
		t.Fatalf("legacy derive-run contract changed: %#v", runs)
	}
}

func TestBUG474_SnapshotDoesNotOverrideStoredColumns(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	snap := bug474SnapshotDef()
	snap.Nodes[1].Model = "pack-model"
	snap.Nodes[1].ContextSources = []string{"snapshot-source"}
	row := bug474RowForSnapshot(t, snap)
	// Admin-editable columns stay authoritative over the snapshot: the stored
	// context_sources and model must win, exactly like embedded-pack
	// precedence for built-ins.
	row["workflow_steps"].([]map[string]any)[1]["step_definitions"].(map[string]any)["context_sources"] = []string{"admin-source"}
	row["workflow_steps"].([]map[string]any)[1]["step_definitions"].(map[string]any)["model"] = "admin-model"

	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		rows := []map[string]any{row}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "aaaaaaaa-1111-2222-3333-444444444444")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	for _, n := range record.Definition.Nodes {
		if n.ID != "writer" {
			continue
		}
		if n.Model != "admin-model" {
			t.Fatalf("writer.Model = %q — stored column must override snapshot", n.Model)
		}
		if len(n.ContextSources) != 1 || n.ContextSources[0] != "admin-source" {
			t.Fatalf("writer.ContextSources = %#v — stored column must override snapshot", n.ContextSources)
		}
	}
}
