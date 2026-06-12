package runner

import (
	"fmt"
	"strings"
)

// Phase 5 (04-05): workflow prompt construction, ported from the Admin Web server
// tier (deriveStepPromptBase in domain/model/entity/workflow-engine.ts;
// buildWorkflowStepPrompt / buildWorkflowStepFollowUpPrompt in
// workflow-start-runtime.ts; the result-summary builders in
// workflow-result-summary.ts). These are pure string builders and are golden-tested
// against the TS outputs so the runner-side prompt assembly (Phase 3) and the turn
// (Phase 4) feed identical prompts to the provider.

const (
	resultSummaryStepType     = "result_summary"
	resultSummaryStepName     = "Summary"
	resultSummaryMinMainSteps = 2
	resultSummaryOutputLimit  = 1600
)

// DeriveStepPromptBase builds the base prompt frame from a step definition when the
// step has no custom prompt_base (port of deriveStepPromptBase).
func DeriveStepPromptBase(stepType, name, description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed != "" {
		return fmt.Sprintf("You are executing the %q workflow step.\n\n%s", name, trimmed)
	}
	return fmt.Sprintf("You are executing the %q workflow step (%s). Produce the expected deliverable for this stage.", name, stepType)
}

// buildPromptSection mirrors the TS helper: a "## Title" header, the lines, and a
// trailing blank line.
func buildPromptSection(title string, lines []string) string {
	return strings.Join(append([]string{"## " + title}, append(lines, "")...), "\n")
}

// WorkflowStepPromptInput carries the fields buildWorkflowStepPrompt needs.
type WorkflowStepPromptInput struct {
	BeginPrompt         string
	InputArtifactPaths  []string
	Model               string
	OutputArtifactPaths []string
	PromptBase          string
	StepType            string
	Subagent            string // "" == null → "not set"
	TeamRole            string // "" == null → "not set"
	WorkingDirectory    string
	RequiredSkills      []string
}

func orNotSet(v string) string {
	if v == "" {
		return "not set"
	}
	return v
}

func bulletsOr(paths []string, empty string) []string {
	if len(paths) == 0 {
		return []string{empty}
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = "- " + p
	}
	return out
}

// BuildWorkflowStepPrompt assembles the full step execution prompt (port of
// buildWorkflowStepPrompt).
func BuildWorkflowStepPrompt(in WorkflowStepPromptInput) string {
	sections := []string{
		"# Workflow Step Execution",
		"",
		buildPromptSection("Execution Contract", []string{
			"- Step type: " + in.StepType,
			"- Model: " + in.Model,
			"- Working directory: " + in.WorkingDirectory,
			"- Team role: " + orNotSet(in.TeamRole),
			"- Subagent: " + orNotSet(in.Subagent),
		}),
		buildPromptSection("Prompt Base", []string{in.PromptBase}),
		buildPromptSection("Begin Prompt", []string{in.BeginPrompt}),
		buildPromptSection("Input Artifacts", bulletsOr(in.InputArtifactPaths, "- None")),
		buildPromptSection("Required Skills", bulletsOr(in.RequiredSkills, "- None")),
		buildPromptSection("Output Targets", bulletsOr(in.OutputArtifactPaths, "- No artifact output configured for this step.")),
		"Return the final result in Markdown and write any requested deliverables to the listed output targets when appropriate.",
	}
	return strings.Join(sections, "\n")
}

// BuildWorkflowStepFollowUpPrompt builds a revision prompt (port of
// buildWorkflowStepFollowUpPrompt). Empty optional sections are dropped, matching
// the TS `.filter(Boolean)`.
func BuildWorkflowStepFollowUpPrompt(followUpPrompt string, inputArtifactPaths, outputArtifactPaths []string, workingDirectory string) string {
	sections := []string{
		strings.TrimSpace(followUpPrompt),
		"",
	}
	if len(outputArtifactPaths) > 0 {
		sections = append(sections, buildPromptSection("Artifacts To Review", bulletsOr(outputArtifactPaths, "")))
	}
	if len(inputArtifactPaths) > 0 {
		sections = append(sections, buildPromptSection("Related Input Artifacts", bulletsOr(inputArtifactPaths, "")))
	}
	sections = append(sections,
		buildPromptSection("Execution Context", []string{"- Working directory: " + workingDirectory}),
		"Revise the current artifact according to the follow-up prompt and return the updated final result in Markdown.",
	)
	// drop empties (the "" placeholder line is intentional spacing, kept by TS only
	// when followUpPrompt is non-empty); replicate `.filter(Boolean)`.
	out := make([]string, 0, len(sections))
	for _, s := range sections {
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

// ---- result summary --------------------------------------------------------

func normalizeStepType(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// IsResultSummaryStepType reports whether a step type is the built-in summary step.
func IsResultSummaryStepType(stepType string) bool {
	return normalizeStepType(stepType) == resultSummaryStepType
}

// ShouldAppendResultSummaryStep reports whether the auto-summary step should be
// appended: at least RESULT_SUMMARY_MIN_MAIN_STEPS main steps, and the last one is
// not already a summary (port of shouldAppendResultSummaryStep).
func ShouldAppendResultSummaryStep(mainStepTypes []string) bool {
	if len(mainStepTypes) < resultSummaryMinMainSteps {
		return false
	}
	last := mainStepTypes[len(mainStepTypes)-1]
	return !IsResultSummaryStepType(last)
}

// trimForPrompt clamps an output excerpt to the prompt limit (port of trimForPrompt).
func trimForPrompt(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= resultSummaryOutputLimit {
		return trimmed
	}
	return trimmed[:resultSummaryOutputLimit] + "\n...[truncated]"
}

// ResultSummarySourceStep is one completed step rendered into the summary prompt.
type ResultSummarySourceStep struct {
	StepName            string
	StepType            string
	OutputMarkdown      string
	ArtifactOutputPaths []string
	StartedAt           string
	CompletedAt         string
}

// BuildResultSummaryPrompt synthesizes the final workflow-summary prompt from the
// completed steps (port of buildResultSummaryPrompt).
func BuildResultSummaryPrompt(beginPrompt, workflowName, workingDirectory string, steps []ResultSummarySourceStep) string {
	rendered := make([]string, len(steps))
	for i, step := range steps {
		artifactPaths := "- None"
		if len(step.ArtifactOutputPaths) > 0 {
			bl := make([]string, len(step.ArtifactOutputPaths))
			for j, p := range step.ArtifactOutputPaths {
				bl[j] = "- " + p
			}
			artifactPaths = strings.Join(bl, "\n")
		}
		rendered[i] = strings.Join([]string{
			fmt.Sprintf("### Step %d: %s", i+1, step.StepName),
			"- Step type: " + step.StepType,
			"- Started at: " + step.StartedAt,
			"- Completed at: " + step.CompletedAt,
			"#### Output Excerpt",
			trimForPrompt(step.OutputMarkdown),
			"#### Artifact Output Paths",
			artifactPaths,
		}, "\n")
	}

	return strings.Join([]string{
		"Create a final workflow-level summary based on the completed steps below.",
		"Return concise Markdown that explains the overall result of the workflow.",
		"Include: overall objective, completed work, key decisions, final deliverables, and remaining risks or follow-up items.",
		"Do not summarize this summary step itself. Summarize only the earlier completed steps.",
		"",
		"## Workflow",
		"- Name: " + workflowName,
		"- Working directory: " + workingDirectory,
		"",
		"## Original Goal",
		strings.TrimSpace(beginPrompt),
		"",
		"## Completed Step Results",
		strings.Join(rendered, "\n\n"),
	}, "\n")
}
