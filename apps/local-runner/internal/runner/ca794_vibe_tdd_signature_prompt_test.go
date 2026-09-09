package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestComposeVibeTddPromptIsSignatureOnly(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var tdd agentpack.FlowNode
	found := false
	for _, def := range pack.Flows {
		if def.ID != "vibe-sprint" {
			continue
		}
		for _, n := range def.Nodes {
			if n.ID == "tdd" {
				tdd = n
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("vibe-sprint tdd node missing")
	}
	got := composeFlowNodeAgentPrompt(t.TempDir(), "tdd base from locked SS/CP", tdd)
	for _, want := range []string{
		"tdd base from locked SS/CP",
		"UNIT TEST SIGNATURES ONLY",
		"Do NOT write any production code",
		"Do NOT fill in test bodies",
		"tdd-signatures.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("composed tdd prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Write and run tests for the requested change") {
		t.Fatal("tester persona default must not leak into composed tdd prompt")
	}
}
