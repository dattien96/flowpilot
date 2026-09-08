package agentpack

import "testing"

func TestPack_VibeCpValidatorHasContractPrompt(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def FlowDefinition
	for _, f := range pack.Flows {
		if f.ID == "vibe-cp-ingest" {
			def = f
			break
		}
	}
	if def.ID == "" {
		t.Fatal("missing vibe-cp-ingest")
	}
	got := ""
	for _, n := range def.Nodes {
		if n.ID == "cp_validator" {
			got = n.PromptTemplate
			break
		}
	}
	if got != "prompts/vibe-cp-validate.md" {
		t.Fatalf("cp_validator template=%q", got)
	}
}
