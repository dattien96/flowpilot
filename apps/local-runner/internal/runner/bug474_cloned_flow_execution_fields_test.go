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
			"scout": {Name: "scout", CandidateSources: []string{"artifact_context"}, MaxEstPromptTokens: 512},
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

// Live-path regression found during the BUG-474 live leg: the Desktop clone
// path (supabaseAdminRepository.cloneWorkflow) INSERTs the workflows row via
// PostgREST directly — it never passes through the runner's Upsert, so the
// row lands with cloned_from set but definition_json NULL. A read must heal
// the schema-less fields from the clone's provenance: cloned_from → source
// row → its definition_json (user source) or its embedded pack flow
// (built-in mirror source) — then backfill definition_json once so later
// reads are cheap.
func TestBUG474_DesktopCloneRowHealsFromBuiltinSource(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	const cloneID = "eeeeeeee-1111-2222-3333-444444444444"
	const mirrorID = "ffffffff-1111-2222-3333-444444444444"
	cloneRow := map[string]any{
		"id": cloneID, "name": "Vibe Sprint Copy",
		"is_builtin": false, "editable": true, "cloneable": true,
		"cloned_from": mirrorID,
		"edges_json": []any{
			map[string]any{"from": "preflight_contract_plan", "to": "preflight_contract_freeze", "when": "done", "kind": "forward"},
		},
		"workflow_steps": []map[string]any{
			{
				"step_type": cloneID + "__plan", "order_index": 0,
				"step_definitions": map[string]any{
					"step_type": cloneID + "__plan", "node_id": "preflight_contract_plan",
					"behavior_id": "agent.delegate", "agent_ref": "agents/contract-planner.md",
				},
			},
		},
	}
	sourceRow := map[string]any{
		"id": mirrorID, "name": "vibe-sprint",
		"is_builtin": true, "editable": false, "cloneable": true,
		"pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "vibe-sprint",
		"edges_json": []any{}, "workflow_steps": []any{},
	}

	var patched map[string]any
	var patchTarget string
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		switch {
		case method == http.MethodGet && strings.Contains(endpoint, "id=eq."+mirrorID):
			b, _ := json.Marshal([]map[string]any{sourceRow})
			return 200, b, nil
		case method == http.MethodGet:
			b, _ := json.Marshal([]map[string]any{cloneRow})
			return 200, b, nil
		case method == http.MethodPatch && strings.Contains(endpoint, "/workflows"):
			patchTarget = endpoint
			_ = json.Unmarshal(body, &patched)
			return 200, []byte("[]"), nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), cloneID)
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	var plan agentpack.FlowNode
	for _, n := range record.Definition.Nodes {
		if n.ID == "preflight_contract_plan" {
			plan = n
		}
	}
	if plan.Posture != "read_only" {
		t.Fatalf("clone node posture = %q — Desktop-clone row still drops pack posture (live BUG-474 gap)", plan.Posture)
	}
	if plan.ContextProfile != "scout" {
		t.Fatalf("clone node contextProfile = %q, want scout", plan.ContextProfile)
	}
	if _, ok := record.Definition.ContextProfiles["scout"]; !ok {
		t.Fatalf("clone ContextProfiles = %#v — flow-level profiles not healed", record.Definition.ContextProfiles)
	}
	// One-time backfill: the healed snapshot must be persisted onto the clone
	// row so subsequent reads take the cheap definition_json branch.
	if patched == nil {
		t.Fatal("no definition_json backfill PATCH issued — heal must persist once")
	}
	if !strings.Contains(patchTarget, cloneID) {
		t.Fatalf("backfill patched wrong row: %s", patchTarget)
	}
}

// The mirror row for a NON-builtin clone source carries its own
// definition_json (written by Upsert): a clone-of-clone heals from that
// snapshot instead of the pack.
func TestBUG474_DesktopCloneRowHealsFromUserSourceSnapshot(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	const cloneID = "12121212-1111-2222-3333-444444444444"
	const parentID = "34343434-1111-2222-3333-444444444444"
	parentSnap := bug474SnapshotDef()
	rawParent, _ := json.Marshal(parentSnap)
	cloneRow := map[string]any{
		"id": cloneID, "name": "Smoke Clone Of Clone",
		"is_builtin": false, "editable": true, "cloneable": true,
		"cloned_from": parentID,
		"edges_json":   []any{},
		"workflow_steps": []map[string]any{
			{
				"step_type": cloneID + "__guard", "order_index": 0,
				"step_definitions": map[string]any{
					"step_type": cloneID + "__guard", "node_id": "guard",
					"behavior_id": "agent.delegate", "agent_ref": "agents/guard.md",
				},
			},
		},
	}
	sourceRow := map[string]any{
		"id": parentID, "name": "Smoke Clone",
		"is_builtin": false, "editable": true, "cloneable": true,
		"definition_json": json.RawMessage(rawParent),
		"edges_json":      []any{}, "workflow_steps": []any{},
	}

	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		switch {
		case method == http.MethodGet && strings.Contains(endpoint, "id=eq."+parentID):
			b, _ := json.Marshal([]map[string]any{sourceRow})
			return 200, b, nil
		case method == http.MethodGet:
			b, _ := json.Marshal([]map[string]any{cloneRow})
			return 200, b, nil
		case method == http.MethodPatch:
			return 200, []byte("[]"), nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), cloneID)
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	for _, n := range record.Definition.Nodes {
		if n.ID == "guard" && n.Posture != "read_only" {
			t.Fatalf("clone-of-clone posture = %q — parent's snapshot must heal it", n.Posture)
		}
	}
}

// Old-schema remote (no definition_json migration applied): the clone read
// still heals from the builtin source's embedded pack — the source select
// degrades to a column-free retry, and the backfill PATCH is skipped once the
// store learns the remote lacks the column.
func TestBUG474_DesktopCloneRowHealsOnUnmigratedRemote(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	const cloneID = "56565656-1111-2222-3333-444444444444"
	const mirrorID = "78787878-1111-2222-3333-444444444444"
	undefinedCol := []byte(`{"code":"42703","message":"column workflows.definition_json does not exist"}`)
	var selects, patches int
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		if method == http.MethodGet && strings.Contains(endpoint, "id=eq."+mirrorID) {
			selects++
			if strings.Contains(endpoint, "definition_json") {
				return 400, undefinedCol, nil
			}
			b, _ := json.Marshal([]map[string]any{{
				"id": mirrorID, "is_builtin": true,
				"pack_id": "flowpilot-core-flow-pack", "pack_flow_id": "vibe-sprint",
			}})
			return 200, b, nil
		}
		if method == http.MethodGet {
			b, _ := json.Marshal([]map[string]any{{
				"id": cloneID, "is_builtin": false, "cloned_from": mirrorID,
				"edges_json": []any{},
				"workflow_steps": []map[string]any{{
					"step_type": cloneID + "__plan", "order_index": 0,
					"step_definitions": map[string]any{
						"step_type": cloneID + "__plan", "node_id": "preflight_contract_plan",
						"behavior_id": "agent.delegate", "agent_ref": "agents/contract-planner.md",
					},
				}},
			}})
			return 200, b, nil
		}
		if method == http.MethodPatch {
			patches++
			return 400, undefinedCol, nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), cloneID)
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok {
		t.Fatal("expected record found")
	}
	for _, n := range record.Definition.Nodes {
		if n.ID == "preflight_contract_plan" && n.Posture != "read_only" {
			t.Fatalf("unmigrated remote: builtin-source heal must still restore posture, got %q", n.Posture)
		}
	}
	// After the first 42703 the store must stop paying snapshot round-trips:
	// a second read reuses the column-free select and skips the PATCH.
	if err := func() error {
		_, _, e := store.GetByRef(context.Background(), cloneID)
		return e
	}(); err != nil {
		t.Fatalf("second GetByRef: %v", err)
	}
	for _, e := range []struct {
		got, max int
		name     string
	}{{selects, 3, "source selects"}, {patches, 1, "backfill PATCHes"}} {
		if e.got > e.max {
			t.Fatalf("expected bounded %s on unmigrated remote, got %d (max %d)", e.name, e.got, e.max)
		}
	}
}

// Upsert on an unmigrated remote retries without the snapshot key instead of
// hard-failing the flow save.
func TestBUG474_UpsertDegradesOnUnmigratedRemote(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	undefinedCol := []byte(`{"code":"42703","message":"column workflows.definition_json does not exist"}`)
	var posts []map[string]any
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, body []byte) (int, []byte, error) {
		switch {
		case method == http.MethodPost && strings.Contains(endpoint, "/workflows"):
			var p map[string]any
			_ = json.Unmarshal(body, &p)
			posts = append(posts, p)
			if _, has := p["definition_json"]; has {
				return 400, undefinedCol, nil
			}
			return 200, []byte(`[{"id":"9abc9abc-1111-2222-3333-444444444444"}]`), nil
		case method == http.MethodGet:
			// Post-upsert fetchOne.
			b, _ := json.Marshal([]map[string]any{{
				"id": "9abc9abc-1111-2222-3333-444444444444", "is_builtin": false,
				"edges_json": []any{}, "workflow_steps": []any{},
			}})
			return 200, b, nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	rec, err := store.Upsert(context.Background(), FlowDefinitionRecord{
		Name:      "User Flow",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID:    "user-flow",
			Nodes: []agentpack.FlowNode{{ID: "n1", Behavior: "agent.delegate", Run: "delegate"}},
		},
	})
	if err != nil {
		t.Fatalf("Upsert on unmigrated remote must degrade, got: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected initial post + stripped retry, got %d posts", len(posts))
	}
	if _, has := posts[0]["definition_json"]; !has {
		t.Fatal("first post must attempt the snapshot (BUG-474 contract)")
	}
	if _, has := posts[1]["definition_json"]; has {
		t.Fatal("retry must strip definition_json for unmigrated remote")
	}
	if rec.FlowRef != "9abc9abc-1111-2222-3333-444444444444" {
		t.Fatalf("FlowRef = %q", rec.FlowRef)
	}
}

// A clone whose source row is gone (deleted) keeps the deterministic legacy
// path — no heal target, no failure.
func TestBUG474_DesktopCloneWithMissingSourceKeepsLegacyPath(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		if method == http.MethodGet && strings.Contains(endpoint, "id=eq.deadbeef") {
			return 200, []byte("[]"), nil // source gone
		}
		rows := []map[string]any{
			{
				"id": "abababab-1111-2222-3333-444444444444", "name": "Orphan Clone",
				"is_builtin": false, "editable": true, "cloneable": true,
				"cloned_from": "deadbeef", "edges_json": []any{},
				"workflow_steps": []map[string]any{
					{"step_type": "s1", "order_index": 0,
						"step_definitions": map[string]any{"step_type": "s1", "node_id": "w",
							"behavior_id": "agent.delegate"}},
				},
			},
		}
		b, _ := json.Marshal(rows)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	record, ok, err := store.GetByRef(context.Background(), "abababab-1111-2222-3333-444444444444")
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if !ok || len(record.Definition.Nodes) != 1 || record.Definition.Nodes[0].Run != "delegate" {
		t.Fatalf("orphan clone must still resolve via legacy derive-run: %#v", record.Definition.Nodes)
	}
}
