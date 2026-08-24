package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// Task-293: static node promptTemplate wiring (appendStaticNodePrompt).
// New file — no pre-existing test is modified.

func TestAppendStaticNodePromptNoopWithoutTemplate(t *testing.T) {
	got := appendStaticNodePrompt("base prompt", agentpack.FlowNode{ID: "plain"})
	if got != "base prompt" {
		t.Fatalf("prompt changed without promptTemplate: %q", got)
	}
}

func TestAppendStaticNodePromptAppendsStaticTemplate(t *testing.T) {
	got := appendStaticNodePrompt("base prompt", agentpack.FlowNode{
		ID:             "plan",
		PromptTemplate: "prompts/plan-safe-fix-contract.md",
	})
	if !strings.Contains(got, "base prompt") {
		t.Fatalf("base prompt lost: %q", got)
	}
	if !strings.Contains(got, "[FlowPilot safe-fix contract — plan step]") {
		t.Fatalf("static plan prompt not appended: %q", got)
	}
	if !strings.Contains(got, "FEATURE-KEYS.md") {
		t.Fatalf("plan prompt must mention feature-key/CA history rule: %q", got)
	}
}

func TestAppendStaticNodePromptSkipsRenderTemplates(t *testing.T) {
	// flow-context-handoff.md is a Go render template ({{ .RenderedContext }})
	// — it must never be injected raw into a provider prompt.
	got := appendStaticNodePrompt("base prompt", agentpack.FlowNode{
		ID:             "implement",
		PromptTemplate: "prompts/flow-context-handoff.md",
	})
	if got != "base prompt" {
		t.Fatalf("render template leaked into prompt: %q", got)
	}
}

func TestAppendStaticNodePromptSkipsMissingTemplate(t *testing.T) {
	got := appendStaticNodePrompt("base prompt", agentpack.FlowNode{
		ID:             "x",
		PromptTemplate: "prompts/does-not-exist.md",
	})
	if got != "base prompt" {
		t.Fatalf("missing template altered prompt: %q", got)
	}
}

func TestComposeFlowNodeAgentPromptAppendsStaticTemplate(t *testing.T) {
	dir := t.TempDir()
	got := composeFlowNodeAgentPrompt(dir, "base", agentpack.FlowNode{
		ID:             "reviewer",
		PromptTemplate: "prompts/review-safe-fix-contract.md",
	})
	if !strings.Contains(got, "safe-fix contract gate") {
		t.Fatalf("compose did not append review static prompt: %q", got)
	}
	if !strings.Contains(got, "base") {
		t.Fatalf("compose lost base prompt: %q", got)
	}
}

func TestSafeFixPromptsCoverHardRules(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"prompts/plan-safe-fix-contract.md", []string{"FEATURE-KEYS.md", "additive tests only", "Claude, Codex and Grok"}},
		{"prompts/test-signatures.md", []string{"UNIT TEST SIGNATURES ONLY", "Do NOT write any production code"}},
		{"prompts/implement-complete-tests.md", []string{"safe-fix contract", "no t.Skip without a stated reason"}},
		{"prompts/review-safe-fix-contract.md", []string{"R1", "R2", "R3", "submit_review_outcome"}},
	} {
		tmpl, ok, err := agentpack.LoadBuiltinPrompt(tc.path)
		if err != nil || !ok {
			t.Fatalf("%s not loadable: ok=%v err=%v", tc.path, ok, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(tmpl.Contents, w) {
				t.Fatalf("%s missing %q in contents", tc.path, w)
			}
		}
		if strings.Contains(tmpl.Contents, "{{") {
			t.Fatalf("%s must be a static markdown prompt, not a Go render template", tc.path)
		}
	}
}
