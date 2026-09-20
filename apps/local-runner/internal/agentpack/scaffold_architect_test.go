package agentpack

import (
	"strings"
	"testing"
)

// CP-67 P-3 (Task-380) + P-4 (Task-381) agentpack contracts, additive.

func TestScaffoldArchitectPromptRendering(t *testing.T) {
	prompt := loadPackPrompt(t, "prompts/scaffold-contract-tdd.md")
	if prompt == "" {
		t.Fatal("scaffold prompt must render non-empty")
	}
	for _, want := range []string{
		"production stubs",                       // Step 2: stubs first
		"return nil, errors.New",                 // Go canonical stub
		"TODO(\"not implemented\")",              // Kotlin canonical stub
		"throw new Error(\"not implemented\")",   // TS/React canonical stub
		"std::runtime_error",                     // C++ canonical stub (B-11)
		"assert(0",                               // C canonical stub (B-11)
		"RED",                                    // the suite must run red
		"submit_scaffold_outcome",                // typed handover
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("scaffold prompt missing %q", want)
		}
	}
	// The persona resolves as a pack agent.
	spec, err := LoadAgentSpecFS(embeddedPackFS, "flow-pack/agents/scaffold-architect.md")
	if err != nil {
		t.Fatalf("LoadAgentSpecFS(scaffold-architect): %v", err)
	}
	if spec.Name != "scaffold-architect" {
		t.Fatalf("persona name = %q", spec.Name)
	}
	if !strings.Contains(spec.Description, "RED") {
		t.Fatalf("persona description must commit to the RED proof, got %q", spec.Description)
	}
}

func TestScaffoldNodeAllowsModelField(t *testing.T) {
	def := taskHarnessDefinition(t)
	node := findNodeByID(t, def, "test_signatures")
	if node.Model != "claude-sonnet-4-5" {
		t.Fatalf("test_signatures model = %q, want the high-reasoning tier", node.Model)
	}
	// B-5: the model allowlist is open for agent.scaffold — pack validation
	// must accept a scaffold node declaring a model.
	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition(task-harness with scaffold model) = %v", err)
	}
	vibe := loadVibeSprint(t)
	if nodeByID(vibe, "tdd").Model != "claude-sonnet-4-5" {
		t.Fatal("vibe-sprint tdd must declare the high-reasoning tier")
	}
	if err := ValidateFlowDefinition(vibe); err != nil {
		t.Fatalf("ValidateFlowDefinition(vibe-sprint) = %v", err)
	}
}

func TestImplementScaffoldBodyPromptRendering(t *testing.T) {
	prompt := loadPackPrompt(t, "prompts/implement-scaffold-body.md")
	if prompt == "" {
		t.Fatal("coder scaffold-body prompt must render non-empty")
	}
	for _, want := range []string{"bodies", "submit_coder_outcome"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("coder prompt missing %q", want)
		}
	}
}

func TestImplementScaffoldBodyPromptContainsBatchContract(t *testing.T) {
	// Task-381 AC-2: the FOUR prohibitions + the Accumulate & Batch contract.
	prompt := loadPackPrompt(t, "prompts/implement-scaffold-body.md")
	for _, want := range []string{
		"Do NOT edit the test files",    // 1. test files read-only
		"Do NOT change a signature",     // 2. signature lock
		"Do NOT write new tests",        // 3. no new tests
		"Do NOT add, remove, or rename", // 4. no added/removed functions (B-8.1)
		"Accumulate & Batch",            // the batching rule
		"renegotiate_signatures",        // canonical term
		"batch_signature_requests",      // the batch payload
		"rationale",                     // every row needs a reason
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("coder prompt missing the batch-contract element %q", want)
		}
	}
}
