package runner

import (
	"strings"
	"testing"
)

// CP-57 OC-08 gap closure: the opencode prompt hook must inject selected
// skills through the SHARED injector with byte-identical content/order versus
// the Claude hook (provider-neutral parity), appending only the opencode
// reinforcement suffix. Mirrors TestGrokPromptPrepAppendsAskUserReinforcement
// but exercises the REAL live-registry promptPrep closure (provider_registry.go
// opencode block) rather than a hand-written lookalike. Additive.

func TestOpencodeRegistryPromptPrepInjectsSkillsLikeClaude(t *testing.T) {
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "1")
	defer mockOpencodeInitProcess(t)()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapterIface, err := reg.Adapter(ProviderKeyOpencode, "opencode/muse-spark-1.2-contributor-free", "medium")
	if err != nil {
		t.Fatalf("live opencode adapter: %v", err)
	}
	adapter, ok := adapterIface.(*opencodeAdapter)
	if !ok {
		t.Fatalf("expected *opencodeAdapter, got %T", adapterIface)
	}
	t.Cleanup(r.closeAllOpencodeProcesses)

	req := TurnRequest{
		RunID:          "run-skill-oc",
		Prompt:         "need a choice",
		SelectedSkills: []SkillSelection{{Name: "audit-logging", Source: "catalog"}},
	}
	got := adapter.preparePrompt(req)

	// Exactness vs the shared injector: the opencode prompt must be exactly
	// injectSelectedSkills(workspace, prompt, skills) + opencodeToolReinforcements.
	want := r.injectSelectedSkills("/tmp", req.Prompt, req.SelectedSkills) + opencodeToolReinforcements
	if got != want {
		t.Fatalf("opencode promptPrep drift:\n got  %q\n want %q", got, want)
	}
	if !strings.Contains(got, "audit-logging") {
		t.Fatalf("selected skill content must reach the prompt, got %q", got)
	}
	if !strings.Contains(got, opencodeAskUserReinforcement) || !strings.Contains(got, opencodeSpawnAgentReinforcement) {
		t.Fatalf("ask_user/spawn_agent reinforcements missing, got %q", got)
	}

	// Claude parity: the SAME shared injector output is the Claude prompt body —
	// only the reinforcement suffix differs. (claude hook: registry.go:385-390.)
	claudeBody := r.injectSelectedSkills("/tmp", req.Prompt, req.SelectedSkills) + claudeAskUserReinforcement
	if !strings.HasPrefix(got, r.injectSelectedSkills("/tmp", req.Prompt, req.SelectedSkills)) {
		t.Fatal("opencode prompt must start with the identical shared skill injection as Claude")
	}
	if strings.HasPrefix(claudeBody, got) && claudeAskUserReinforcement == opencodeToolReinforcements {
		t.Fatal("sanity: reinforcement suffixes unexpectedly identical")
	}

	// No selection → prompt + reinforcement only, untouched body.
	plain := adapter.preparePrompt(TurnRequest{RunID: "run-skill-oc", Prompt: "plain"})
	if plain != "plain"+opencodeToolReinforcements {
		t.Fatalf("no-selection prompt drift, got %q", plain)
	}
}
