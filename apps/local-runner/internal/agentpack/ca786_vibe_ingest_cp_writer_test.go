package agentpack

import "testing"

func TestPack_VibeIngestWritesCPThenJoinsCpIngest(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def FlowDefinition
	for _, f := range pack.Flows {
		if f.ID == "vibe-ingest" {
			def = f
			break
		}
	}
	if def.ID == "" {
		t.Fatal("missing vibe-ingest")
	}
	got := map[string]string{}
	for _, n := range def.Nodes {
		got[n.ID] = n.PromptTemplate
		if n.ID == "sprint_slicer" {
			t.Fatal("sprint_slicer must not remain on vibe-ingest")
		}
	}
	if got["cp_writer"] != "prompts/vibe-cp-from-ss.md" {
		t.Fatalf("cp_writer template=%q", got["cp_writer"])
	}
	found := false
	for _, e := range def.Edges {
		if e.From == "ss_lock" && e.To == "cp_writer" && e.When == "done" {
			found = true
		}
		if e.From == "ss_lock" && e.To == "sprint_slicer" {
			t.Fatal("ss_lock must not edge to sprint_slicer")
		}
	}
	if !found {
		t.Fatal("ss_lock --done--> cp_writer missing")
	}
}
