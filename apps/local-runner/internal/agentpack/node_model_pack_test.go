package agentpack

import (
	"strings"
	"testing"
)

// TestModelProviderKeyPrefixes pins the single-source model→provider table
// (Task-320): the runner's providerKeyFromModel delegates to this function,
// so any drift breaks the parity test on the runner side.
func TestModelProviderKeyPrefixes(t *testing.T) {
	cases := []struct {
		model string
		want  string
		ok    bool
	}{
		{"gpt-5.4-mini", "codex", true},
		{"GPT-5.4", "codex", true},
		{"  gpt-5.4  ", "codex", true},
		{"gemini-2.5-flash", "gemini", true},
		{"auto-gemini-2.5", "gemini", true},
		{"claude-sonnet-4-5", "claude", true},
		{"grok-4-5", "grok", true},
		{"grok-build", "grok", true},
		{"opencode/grok-code-fast-1", "opencode", true},
		{"opencode-go/claude-opus-4-6", "opencode", true},
		{"some-unknown-model", "", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, tc := range cases {
		got, ok := ModelProviderKey(tc.model)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ModelProviderKey(%q) = (%q, %v), want (%q, %v)", tc.model, got, ok, tc.want, tc.ok)
		}
	}
}

// modelValidationFlow builds a minimal valid flow with one node carrying the
// given behavior/model for ValidateFlowDefinition model-tier checks.
func modelValidationFlow(behavior, model string) FlowDefinition {
	return FlowDefinition{
		ID:   "model-tier-check",
		Mode: "flow",
		Nodes: []FlowNode{
			{ID: "entry", Run: "delegate", Lifecycle: "once", Behavior: behavior, Agent: "agents/coder.md", Model: model},
			{ID: "audit", Run: "inline", Lifecycle: "once", Behavior: "artifact.audit_draft"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "audit", When: "done", Kind: "forward"},
			{From: "audit", To: "done", When: "done", Kind: "forward"},
		},
	}
}

// TestFlowNodeModelValidation (Task-320 T-3) pins fail-closed pack validation:
// only agent.delegate may declare model, and the model must map to a known
// provider — anything else breaks the load instead of silently inheriting.
func TestFlowNodeModelValidation(t *testing.T) {
	if err := ValidateFlowDefinition(modelValidationFlow("agent.delegate", "grok-4-5")); err != nil {
		t.Fatalf("delegate + known model must validate, got %v", err)
	}
	if err := ValidateFlowDefinition(modelValidationFlow("agent.delegate", "")); err != nil {
		t.Fatalf("delegate without model must validate, got %v", err)
	}
	for _, behavior := range []string{"agent.code", "hub.inline", "contract.freeze", "context.produce", "command.validate", "artifact.audit_draft", ""} {
		err := ValidateFlowDefinition(modelValidationFlow(behavior, "grok-4-5"))
		if err == nil || !strings.Contains(err.Error(), "never consumes a model") {
			t.Errorf("behavior %q + model must fail with never-consumes-a-model, got %v", behavior, err)
		}
	}
	err := ValidateFlowDefinition(modelValidationFlow("agent.delegate", "not-a-real-model-xyz"))
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Errorf("delegate + unknown model must fail with unknown-model, got %v", err)
	}
}
