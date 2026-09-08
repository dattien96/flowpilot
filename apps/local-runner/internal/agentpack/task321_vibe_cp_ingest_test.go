package agentpack

import "testing"

func TestPack_VibeCpIngestTopology(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def *FlowDefinition
	for i := range pack.Flows {
		if pack.Flows[i].ID == "vibe-cp-ingest" {
			def = &pack.Flows[i]
			break
		}
	}
	if def == nil {
		t.Fatal("missing vibe-cp-ingest")
	}
	if len(def.Builtin.SelectableIn) != 0 {
		t.Fatalf("selectableIn=%v, want empty", def.Builtin.SelectableIn)
	}
	got := map[string]string{}
	for _, n := range def.Nodes {
		got[n.ID] = n.Behavior
	}
	if got["cp_lock"] != "user.confirm" {
		t.Fatalf("cp_lock behavior=%q, want user.confirm", got["cp_lock"])
	}
	if got["cp_reader"] != "agent.delegate" {
		t.Fatalf("cp_reader behavior=%q", got["cp_reader"])
	}
	slicer := nodeByID(*def, "task_slicer")
	if slicer.Agent != "agents/doc-writer.md" {
		t.Fatalf("task_slicer agent=%q", slicer.Agent)
	}
	if slicer.PromptTemplate != "prompts/task-splitter.md" {
		t.Fatalf("task_slicer prompt=%q", slicer.PromptTemplate)
	}
	hasIn, hasOut := false, false
	for _, b := range slicer.ArtifactBindings {
		if b.SlotName == "cp_md" && b.Direction == "input" {
			hasIn = true
		}
		if b.SlotName == "task_md" && b.Direction == "output" {
			hasOut = true
		}
	}
	if !hasIn || !hasOut {
		t.Fatalf("task_slicer bindings in=%v out=%v", hasIn, hasOut)
	}
	back := 0
	for _, e := range def.Edges {
		if e.Kind == "back" && e.When == "continue" {
			back++
			if e.From != "cp_lock" || e.To != "cp_reader" {
				t.Fatalf("continue back-edge %s → %s, want cp_lock → cp_reader", e.From, e.To)
			}
		}
	}
	if back != 1 {
		t.Fatalf("continue back-edges=%d, want 1", back)
	}
	if err := ValidateFlowDefinition(*def); err != nil {
		t.Fatalf("ValidateFlowDefinition: %v", err)
	}
	if err := ValidateFlowSafetyTopology(*def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology: %v", err)
	}
}
