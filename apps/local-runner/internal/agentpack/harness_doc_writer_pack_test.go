package agentpack

import (
	"strings"
	"testing"
)

// BUG-XXX (task-harness S3): the plan/document-writer nodes used the coder
// agent, so plan_writer kept implementing source + tests + CA notes instead of
// writing only its Task artifact. The doc nodes now use the dedicated
// doc-writer agent (no Edit/Bash); the real code writers stay on coder. New
// file — no pre-existing test is modified.

func TestHarnessDocWriterNodesUseDocWriterAgent(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	wantDoc := []struct{ flow, node string }{
		{"task-harness", "plan_writer"},
		{"cp-harness", "cp_plan_writer"},
		{"cp-harness", "task_splitter"},
		{"cp-harness-smoke", "cp_plan_writer"},
		{"cp-harness-smoke", "task_splitter"},
	}
	for _, w := range wantDoc {
		found := false
		for _, def := range pack.Flows {
			if def.ID != w.flow {
				continue
			}
			for _, n := range def.Nodes {
				if n.ID == w.node {
					found = true
					if n.Agent != "agents/doc-writer.md" {
						t.Fatalf("%s node %q agent = %q, want agents/doc-writer.md", w.flow, w.node, n.Agent)
					}
				}
			}
		}
		if !found {
			t.Fatalf("%s node %q missing from builtin pack", w.flow, w.node)
		}
	}
}

func TestHarnessCodeWritersStayOnCoder(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID != "task-harness" && def.ID != "cp-harness-smoke" {
			continue
		}
		for _, n := range def.Nodes {
			if n.ID == "implement" && n.Agent != "agents/coder.md" {
				t.Fatalf("%s implement agent = %q, want agents/coder.md (real code writer)", def.ID, n.Agent)
			}
			// CP-67 supersession: task-harness's test_signatures is the
			// scaffold architect node; cp-harness-smoke keeps the legacy
			// tester wiring (CP-67 does not touch the smoke flow).
			if def.ID == "task-harness" && n.ID == "test_signatures" && n.Agent != "agents/scaffold-architect.md" {
				t.Fatalf("%s test_signatures agent = %q, want agents/scaffold-architect.md", def.ID, n.Agent)
			}
		}
	}
}

func TestDocWriterAgentSpecIsDocumentOnly(t *testing.T) {
	spec, err := LoadAgentSpecFS(embeddedPackFS, "flow-pack/agents/doc-writer.md")
	if err != nil {
		t.Fatalf("LoadAgentSpecFS(doc-writer): %v", err)
	}
	for _, banned := range []string{"Edit", "Bash"} {
		for _, tool := range spec.Tools {
			if strings.EqualFold(tool, banned) {
				t.Fatalf("doc-writer must not expose %s: %v", banned, spec.Tools)
			}
		}
	}
	haveWrite := false
	for _, tool := range spec.Tools {
		if strings.EqualFold(tool, "Write") {
			haveWrite = true
		}
	}
	if !haveWrite {
		t.Fatalf("doc-writer must keep Write to author markdown artifacts: %v", spec.Tools)
	}
	if !strings.Contains(spec.SystemPrompt, "must NOT") {
		t.Fatal("doc-writer system prompt must carry an explicit must-NOT scope guard")
	}
	if !strings.Contains(spec.SystemPrompt, "change-audit") {
		t.Fatal("doc-writer system prompt must forbid writing change-audit notes")
	}
}

func TestHarnessDocWriterFlowsStillValidate(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID != "task-harness" && def.ID != "cp-harness" && def.ID != "cp-harness-smoke" {
			continue
		}
		if err := ValidateFlowDefinition(def); err != nil {
			t.Fatalf("ValidateFlowDefinition(%s) after doc-writer retarget = %v, want nil", def.ID, err)
		}
	}
}