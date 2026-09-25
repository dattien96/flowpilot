package agentpack

import (
	"strings"
	"testing"
	"testing/fstest"
)

// Task-441 (CP-86 P-2): contextProfiles.*.maxTokens is renamed to
// maxEstPromptTokens — the value is an ESTIMATED prompt-length budget
// (bytes/4 heuristic), not provider-reported usage. Hard rename, no alias:
// a legacy maxTokens key must fail flow load loudly so an out-of-repo flow
// author discovers the schema change instead of silently losing the cap.

const task441FlowPrefix = `id: t441-flow
contextProfiles:
  scout:
    candidateSources: [canonical.head]
`

func TestTask441_MaxEstPromptTokens_Parses(t *testing.T) {
	fsys := fstest.MapFS{
		"pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents: []
flows:
  - path: flows/a.yaml
tools: []
contexts: []
prompts: []
`)},
		"pack/flows/a.yaml": &fstest.MapFile{Data: []byte(task441FlowPrefix +
			"    maxEstPromptTokens: 7777\n" +
			"nodes:\n  - id: n1\n    run: inline\n")},
	}
	pack, err := LoadPackFS(fsys, "pack")
	if err != nil {
		t.Fatalf("LoadPackFS: %v", err)
	}
	profile, ok := pack.Flows[0].ContextProfiles["scout"]
	if !ok {
		t.Fatal("scout profile missing")
	}
	if profile.MaxEstPromptTokens != 7777 {
		t.Fatalf("MaxEstPromptTokens = %d, want 7777", profile.MaxEstPromptTokens)
	}
}

func TestTask441_LegacyMaxTokens_FailsFlowLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents: []
flows:
  - path: flows/a.yaml
tools: []
contexts: []
prompts: []
`)},
		"pack/flows/a.yaml": &fstest.MapFile{Data: []byte(task441FlowPrefix +
			"    maxTokens: 6000\n" +
			"nodes:\n  - id: n1\n    run: inline\n")},
	}
	_, err := LoadPackFS(fsys, "pack")
	if err == nil {
		t.Fatal("legacy maxTokens key must fail flow load, got nil error")
	}
	if !strings.Contains(err.Error(), "maxEstPromptTokens") {
		t.Fatalf("error = %v, want it to name the replacement key maxEstPromptTokens", err)
	}
}

func TestTask441_AllBuiltinFlows_ParseClean(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	found := false
	for _, flow := range pack.Flows {
		for _, profile := range flow.ContextProfiles {
			if profile.MaxEstPromptTokens <= 0 {
				t.Fatalf("flow %s profile %q: MaxEstPromptTokens = %d, want > 0",
					flow.ID, profile.Name, profile.MaxEstPromptTokens)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no contextProfiles parsed from builtin pack — rename may have silently dropped the budget")
	}
}
