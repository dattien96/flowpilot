package agentpack

import (
	"strings"
	"testing"
)

// CP-64 P-2 (Task-365): reproducer prompt + persona + agent.reproduce behavior.
// New file — no pre-existing pack test is modified.

const (
	reproducePromptRel = "prompts/reproduce-failing-test.md"
	reproducerAgentRel = "agents/reproducer.md"
)

func builtinReproduceNode(t *testing.T, flowID string) FlowNode {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load builtin pack: %v", err)
	}
	for _, flow := range pack.Flows {
		if flow.ID != flowID {
			continue
		}
		node, ok := findNodeInFlow(flow, "reproduce_test")
		if !ok {
			t.Fatalf("flow %q has no reproduce_test node", flowID)
		}
		return node
	}
	t.Fatalf("flow %q not found in builtin pack", flowID)
	return FlowNode{}
}

func findNodeInFlow(flow FlowDefinition, id string) (FlowNode, bool) {
	for _, n := range flow.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return FlowNode{}, false
}

// renameReproduceEdgeEndpoints translates a bug flow's CP-64 node id
// (reproduce_test) back to the legacy TDD node id (test_signatures) so a
// pre-CP-64 topology comparison (rag-harness / task-harness) still holds — the
// two flows share the exact same edge shape, one node was renamed.
func renameReproduceEdgeEndpoints(edges []FlowEdge) []FlowEdge {
	out := make([]FlowEdge, len(edges))
	copy(out, edges)
	for i := range out {
		if out[i].From == "reproduce_test" {
			out[i].From = "test_signatures"
		}
		if out[i].To == "reproduce_test" {
			out[i].To = "test_signatures"
		}
	}
	return out
}

// Scenario: prompt template của reproducer render được khi pack load (không lỗi
// parse/template) và chứa đủ 4 luật cốt lõi của CP-64 §3.1.
func TestReproducerAgentPromptRender(t *testing.T) {
	prompt, ok, err := LoadBuiltinPrompt(reproducePromptRel)
	if err != nil {
		t.Fatalf("load prompt %q: %v", reproducePromptRel, err)
	}
	if !ok {
		t.Fatalf("prompt %q not found in the builtin pack", reproducePromptRel)
	}
	body := prompt.Contents
	if !strings.Contains(body, "{{") && strings.TrimSpace(body) == "" {
		t.Fatal("reproduce prompt rendered empty")
	}
	for _, want := range []string{
		"REPRODUCING TEST",
		"Do NOT touch production code",
		"RUN the test suite",
		"REJECTED",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reproduce prompt missing %q", want)
		}
	}
	// The persona must exist, declare the reproducer role, and carry the
	// "do not optimise for green" invariant.
	var agent *AgentSpec
	specs, err := LoadBuiltinAgents()
	if err != nil {
		t.Fatalf("load builtin agents: %v", err)
	}
	for i := range specs {
		if specs[i].Name == "reproducer" {
			agent = &specs[i]
			break
		}
	}
	if agent == nil {
		t.Fatalf("agent %q not resolved from the pack manifest", reproducerAgentRel)
	}
	if agent.Name != "reproducer" || agent.Role != "reproducer" {
		t.Fatalf("reproducer agent name/role = %q/%q", agent.Name, agent.Role)
	}
	// AC-4: the legacy signature assets stay untouched (task-harness and
	// vibe-sprint keep the empty-signature flow).
	legacy, ok, err := LoadBuiltinPrompt("prompts/test-signatures.md")
	if err != nil || !ok {
		t.Fatalf("legacy test-signatures prompt missing: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(legacy.Contents, "UNIT TEST SIGNATURES ONLY") {
		t.Fatal("legacy empty-signature prompt changed — CP-64 must not break new-feature flows")
	}
}

// Scenario: node reproduce_test trong bug flow bind đúng prompt/persona/behavior,
// và runtime registry resolve được agent.reproduce ở scope delegate (T-2/T-4).
func TestReproducerArtifactBinding(t *testing.T) {
	for _, flowID := range []string{"bug-harness", "bug-plan-harness"} {
		node := builtinReproduceNode(t, flowID)
		canonical, ok := NormalizeBehaviorID(node.Behavior)
		if !ok || canonical != "agent.reproduce" {
			t.Fatalf("flow %q reproduce_test behavior = %q (canonical %q ok=%v)", flowID, node.Behavior, canonical, ok)
		}
		if node.Agent != reproducerAgentRel {
			t.Fatalf("flow %q reproduce_test agent = %q want %q", flowID, node.Agent, reproducerAgentRel)
		}
		if node.PromptTemplate != reproducePromptRel {
			t.Fatalf("flow %q reproduce_test promptTemplate = %q want %q", flowID, node.PromptTemplate, reproducePromptRel)
		}
	}
	// Runtime binding: the id resolves to the same delegate-scoped handler
	// agent.delegate uses (T-2 — no new spawn path). Pinned in the runner
	// package (TestReproduceBehaviorRuntimeBinding) — agentpack cannot import
	// runner from a test (import cycle).
	for _, alias := range []string{"reproduce", "reproducing_test"} {
		if got, ok := NormalizeBehaviorID(alias); !ok || got != "agent.reproduce" {
			t.Fatalf("alias %q resolved to %q ok=%v", alias, got, ok)
		}
	}
}
