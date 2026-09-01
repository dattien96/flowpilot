package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CP-58 Task-307 runner-side contracts for the templated harness plan
// artifacts. New file — no pre-existing test is modified.

// TestHarnessTemplatedPlanOutputPromptContract proves the plan_writer node's
// composed prompt carries the templated write contract (Task-307), and — the
// load-bearing half — that the template never enters the flowgate's concrete
// required-path list, or every task-harness run would reprompt forever on a
// literal "Task-{{idx}}-{{slug}}.md" file that can never exist.
func TestHarnessTemplatedPlanOutputPromptContract(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var planWriter agentpack.FlowNode
	found := false
	for _, def := range pack.Flows {
		if def.ID != "task-harness" {
			continue
		}
		for _, n := range def.Nodes {
			if n.ID == "plan_writer" {
				planWriter = n
				found = true
			}
		}
	}
	if !found {
		t.Fatal("task-harness plan_writer node missing from builtin pack")
	}

	if got := requiredFileArtifactOutputPaths(planWriter); len(got) != 0 {
		t.Fatalf("templated OUTPUT leaked into gated concrete paths: %v", got)
	}

	composed := composeFlowNodeAgentPrompt(t.TempDir(), "plan base", planWriter)
	if !strings.Contains(composed, "Templated file outputs (write contract)") {
		t.Fatal("plan_writer composed prompt missing the templated output write contract")
	}
	if !strings.Contains(composed, "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md") {
		t.Fatal("plan_writer composed prompt missing the Task plan pathTemplate")
	}
}

// TestHarnessTemplatedInputMentionForPlanReviewer proves the plan_reviewer is
// told where to find the plan artifact (Task-307 INPUT mention) without the
// concrete-path read block (a template has no concrete path to paste).
func TestHarnessTemplatedInputMentionForPlanReviewer(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var planReviewer agentpack.FlowNode
	found := false
	for _, def := range pack.Flows {
		if def.ID != "task-harness" {
			continue
		}
		for _, n := range def.Nodes {
			if n.ID == "plan_reviewer" {
				planReviewer = n
				found = true
			}
		}
	}
	if !found {
		t.Fatal("task-harness plan_reviewer node missing from builtin pack")
	}

	if got := resolveInputArtifactPrompt(t.TempDir(), planReviewer); got != "" {
		t.Fatalf("templated INPUT leaked into the concrete-path mention block: %q", got)
	}

	composed := composeFlowNodeAgentPrompt(t.TempDir(), "review base", planReviewer)
	if !strings.Contains(composed, "Bound input artifacts (locate and read)") {
		t.Fatal("plan_reviewer composed prompt missing the templated input mention")
	}
	if !strings.Contains(composed, "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md") {
		t.Fatal("plan_reviewer composed prompt missing the plan pathTemplate")
	}
}
