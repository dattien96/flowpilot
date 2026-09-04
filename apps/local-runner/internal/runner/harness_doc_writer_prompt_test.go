package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-XXX (task-harness S3): plan_writer used to run as the coder agent and
// implement source + tests + CA notes instead of writing only its Task
// artifact. The composed node prompt now carries an ONLY-these-files scope
// guard and the spawn resolves the doc-writer agent definition (no Edit/Bash).
// New file — no pre-existing test is modified.

func TestHarnessDocWriterComposedPromptOnlyGuard(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID != "task-harness" {
			continue
		}
		for _, n := range def.Nodes {
			if n.ID != "plan_writer" {
				continue
			}
			composed := composeFlowNodeAgentPrompt(t.TempDir(), "plan base", n)
			if !strings.Contains(composed, "and ONLY these files") {
				t.Fatal("plan_writer composed prompt must carry the ONLY-these-files scope guard")
			}
			if !strings.Contains(composed, "do NOT run commands or tests") {
				t.Fatal("plan_writer composed prompt must forbid running commands/tests")
			}
			if !strings.Contains(composed, "Templated file outputs (write contract)") {
				t.Fatal("plan_writer composed prompt missing the templated write contract")
			}
			return
		}
	}
	t.Fatal("task-harness plan_writer node missing from builtin pack")
}

func TestResolvePackAgentDefinitionDocWriter(t *testing.T) {
	def, ok := resolvePackAgentDefinition("doc-writer")
	if !ok {
		t.Fatal("resolvePackAgentDefinition(doc-writer) must resolve from the embedded pack")
	}
	for _, banned := range []string{"Bash", "Edit"} {
		for _, tool := range def.Tools {
			if strings.EqualFold(tool, banned) {
				t.Fatalf("doc-writer agent must not expose %s: %v", banned, def.Tools)
			}
		}
	}
	if !strings.Contains(def.SystemPrompt, "must NOT") {
		t.Fatal("doc-writer agent system prompt must carry the must-NOT scope guard")
	}
}