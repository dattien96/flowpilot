package agentpack

import "testing"

func TestPack_VibeIngestSSPromptsIgnoreCode(t *testing.T) {
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
	}
	if got["ingest_reader"] != "prompts/vibe-ss-ingest.md" {
		t.Fatalf("ingest_reader template=%q", got["ingest_reader"])
	}
	if got["ss_converter"] != "prompts/vibe-ss-ingest.md" {
		t.Fatalf("ss_converter template=%q", got["ss_converter"])
	}
	if got["ss_validator"] != "prompts/vibe-ss-validate.md" {
		t.Fatalf("ss_validator template=%q", got["ss_validator"])
	}
}
