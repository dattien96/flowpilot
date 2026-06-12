package runner

import (
	"strings"
	"testing"
)

func TestDeriveStepPromptBase(t *testing.T) {
	// with description → name + description frame
	got := DeriveStepPromptBase("plan", "Plan", "  Do the planning.  ")
	want := "You are executing the \"Plan\" workflow step.\n\nDo the planning."
	if got != want {
		t.Fatalf("with description:\n got=%q\nwant=%q", got, want)
	}

	// empty description → fallback frame with stepType
	got = DeriveStepPromptBase("plan", "Plan", "   ")
	want = "You are executing the \"Plan\" workflow step (plan). Produce the expected deliverable for this stage."
	if got != want {
		t.Fatalf("empty description:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildWorkflowStepPrompt(t *testing.T) {
	got := BuildWorkflowStepPrompt(WorkflowStepPromptInput{
		BeginPrompt:         "Build the cart feature",
		InputArtifactPaths:  []string{"in/spec.md"},
		Model:               "gpt-5.4",
		OutputArtifactPaths: nil,
		PromptBase:          "Base prompt",
		StepType:            "build",
		Subagent:            "",
		TeamRole:            "engineer",
		WorkingDirectory:    "/ws",
		RequiredSkills:      nil,
	})

	// structural golden checks (port of buildWorkflowStepPrompt)
	for _, want := range []string{
		"# Workflow Step Execution",
		"## Execution Contract",
		"- Step type: build",
		"- Model: gpt-5.4",
		"- Working directory: /ws",
		"- Team role: engineer",
		"- Subagent: not set", // empty subagent → "not set"
		"## Prompt Base\nBase prompt",
		"## Begin Prompt\nBuild the cart feature",
		"## Input Artifacts\n- in/spec.md",
		"## Required Skills\n- None",
		"## Output Targets\n- No artifact output configured for this step.",
		"Return the final result in Markdown",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q\n---\n%s", want, got)
		}
	}
}

func TestBuildWorkflowStepFollowUpPrompt_DropsEmptyOptionalSections(t *testing.T) {
	got := BuildWorkflowStepFollowUpPrompt("Tweak the copy", nil, nil, "/ws")
	if strings.Contains(got, "Artifacts To Review") || strings.Contains(got, "Related Input Artifacts") {
		t.Fatalf("empty optional sections should be dropped:\n%s", got)
	}
	if !strings.Contains(got, "## Execution Context\n- Working directory: /ws") {
		t.Fatalf("missing execution context:\n%s", got)
	}
}

func TestShouldAppendResultSummaryStep(t *testing.T) {
	if ShouldAppendResultSummaryStep([]string{"plan"}) {
		t.Fatal("one main step should not get a summary")
	}
	if !ShouldAppendResultSummaryStep([]string{"plan", "build"}) {
		t.Fatal("two distinct main steps should get a summary")
	}
	if ShouldAppendResultSummaryStep([]string{"plan", "result_summary"}) {
		t.Fatal("trailing summary step should not get another summary")
	}
}

func TestBuildResultSummaryPrompt(t *testing.T) {
	got := BuildResultSummaryPrompt("  Ship the cart  ", "Cart Workflow", "/ws", []ResultSummarySourceStep{
		{
			StepName: "Plan", StepType: "plan", OutputMarkdown: "Planned it.",
			ArtifactOutputPaths: []string{"out/plan.md"}, StartedAt: "t0", CompletedAt: "t1",
		},
		{
			StepName: "Build", StepType: "build", OutputMarkdown: "Built it.",
			ArtifactOutputPaths: nil, StartedAt: "t2", CompletedAt: "t3",
		},
	})

	for _, want := range []string{
		"Create a final workflow-level summary",
		"## Workflow\n- Name: Cart Workflow\n- Working directory: /ws",
		"## Original Goal\nShip the cart", // beginPrompt trimmed
		"### Step 1: Plan",
		"- Step type: plan",
		"#### Output Excerpt\nPlanned it.",
		"#### Artifact Output Paths\n- out/plan.md",
		"### Step 2: Build",
		"#### Artifact Output Paths\n- None", // no artifact paths → "- None"
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary prompt missing %q\n---\n%s", want, got)
		}
	}
}

func TestTrimForPromptTruncates(t *testing.T) {
	long := strings.Repeat("x", resultSummaryOutputLimit+50)
	out := trimForPrompt(long)
	if !strings.HasSuffix(out, "\n...[truncated]") {
		t.Fatalf("expected truncation marker, got suffix %q", out[len(out)-20:])
	}
	if len(out) != resultSummaryOutputLimit+len("\n...[truncated]") {
		t.Fatalf("truncated length = %d", len(out))
	}
}
