package runner

import (
	"context"
	"strings"
)

// flowContextHandoffPrefix is the leading marker of every flow context package
// prompt. injectFeatureHistoryPrompt checks for it to avoid double-injection.
const flowContextHandoffPrefix = "[FlowPilot flow context package]"

// isFlowContextHandoff reports whether a prompt already carries a prepended
// FlowContextPackage, preventing double-injection by injectFeatureHistoryPrompt.
func isFlowContextHandoff(prompt string) bool {
	return strings.HasPrefix(strings.TrimSpace(prompt), flowContextHandoffPrefix)
}

// isCodingStepType returns true for step types that represent a Coding step
// in a Flow Mode workflow.
func isCodingStepType(stepType string) bool {
	switch strings.ToLower(strings.TrimSpace(stepType)) {
	case "coding", "implementation", "code":
		return true
	}
	return false
}

// isPlanStepType returns true for step types that represent a Plan step.
func isPlanStepType(stepType string) bool {
	switch strings.ToLower(strings.TrimSpace(stepType)) {
	case "plan", "planning", "design":
		return true
	}
	return false
}

// findPlanStepID returns the ID of the most-recent Plan step that precedes
// codingStepID in the steps slice. Returns ("", false) when none is found.
func findPlanStepID(steps []RuntimeWorkflowStep, codingStepID string) (string, bool) {
	codingIdx := -1
	for i, s := range steps {
		if s.ID == codingStepID {
			codingIdx = i
			break
		}
	}
	if codingIdx <= 0 {
		return "", false
	}
	for i := codingIdx - 1; i >= 0; i-- {
		if isPlanStepType(steps[i].StepType) {
			return steps[i].ID, true
		}
	}
	return "", false
}

// FindFlowContextPackage scans the event list newest-first for an
// EventFlowContextPackage tied to planStepID. When planStepID is empty every
// package event is a candidate. Returns the package and true when found.
func FindFlowContextPackage(events []ProviderEvent, planStepID string) (*FlowContextPackage, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == EventFlowContextPackage && ev.FlowContextPackage != nil {
			if planStepID == "" || ev.WorkflowStepRunID == planStepID {
				cp := *ev.FlowContextPackage
				return &cp, true
			}
		}
	}
	return nil, false
}

// ComposeFlowCodingPrompt prepends the rendered FlowContextPackage plus a brief
// "use this as context" instruction before the Coding step's user instruction.
// The flowContextHandoffPrefix sentinel prevents injectFeatureHistoryPrompt from
// injecting a duplicate feature block (T-5, Task-169).
func ComposeFlowCodingPrompt(pkg FlowContextPackage, codingInstruction string) string {
	var sb strings.Builder
	sb.WriteString(flowContextHandoffPrefix + "\n\n")
	sb.WriteString(RenderFlowContextPackage(pkg))
	sb.WriteString("\n---\n\n")
	sb.WriteString("[Context use instructions: Use the Flow Context Package above as " +
		"the source of truth for prior work on this feature. " +
		"Do not broaden retrieval unless explicitly instructed. " +
		"Preserve source references when explaining changes.]\n\n")
	sb.WriteString(codingInstruction)
	return sb.String()
}

// injectFlowContextIfCoding prepends a FlowContextPackage to providerPrompt
// when stepID is a Coding step in a multi-step workflow run. The package is
// built once and cached on rs.planContextPackage; subsequent calls (retries)
// return the cached package unchanged (T-3, Task-169).
//
// The raw user prompt (rawPrompt = in.Prompt before mode-prefix assembly) is
// used as the feature resolution hint so the catalog scores on the user's
// intent rather than the assembled prompt text.
//
// Returns providerPrompt unchanged when:
//   - workflowStore is nil or has no steps for this run
//   - the step is not a Coding step
//   - BuildFlowContextPackage returns an error
func (s *InteractiveService) injectFlowContextIfCoding(
	ctx context.Context,
	rs *interactiveRun,
	stepID, rawPrompt, providerPrompt string,
) string {
	if s.workflowStore == nil || stepID == "" {
		return providerPrompt
	}
	steps, err := s.workflowStore.LoadRunSteps(ctx, rs.id)
	if err != nil || len(steps) == 0 {
		return providerPrompt
	}

	var thisStepType string
	for _, st := range steps {
		if st.ID == stepID {
			thisStepType = st.StepType
			break
		}
	}
	if !isCodingStepType(thisStepType) {
		return providerPrompt
	}
	planStepID, _ := findPlanStepID(steps, stepID)

	// Fast path: cached package survives retries without rebuilding.
	s.mu.Lock()
	cached := rs.planContextPackage
	if cached == nil {
		if found, ok := FindFlowContextPackage(rs.events, planStepID); ok {
			cached = found
			rs.planContextPackage = cached
		}
	}
	s.mu.Unlock()

	if cached != nil {
		return ComposeFlowCodingPrompt(*cached, providerPrompt)
	}

	// Slow path: build a fresh package.
	hints := FlowContextHints{
		WorkflowRunID: rs.id,
		PlanStepRunID: planStepID,
		UserPrompt:    rawPrompt,
		SourceDocID:   rs.sourceDocID,
	}
	built, buildErr := BuildFlowContextPackage(rs.workspaceCwd, hints)
	if buildErr != nil {
		return providerPrompt
	}
	if planStepID == "" {
		built.Warnings = append(built.Warnings,
			"no_plan_step: flow context assembled without a prior plan step")
	}

	pkg := &built
	s.mu.Lock()
	if rs.planContextPackage == nil {
		rs.planContextPackage = pkg
		s.emitLocked(rs, ProviderEvent{
			Type:               EventFlowContextPackage,
			WorkflowRunID:      rs.id,
			WorkflowStepRunID:  planStepID,
			FlowContextPackage: pkg,
		})
	} else {
		// Another path (unlikely — single-threaded turn flow) already set it.
		pkg = rs.planContextPackage
	}
	s.mu.Unlock()

	return ComposeFlowCodingPrompt(*pkg, providerPrompt)
}

// maybeClearPlanContextForPlanStep clears rs.planContextPackage when stepID
// belongs to a Plan step. Called at the start of each turn so a Plan rerun
// forces the next Coding step to rebuild the package (T-3, Task-169).
func (s *InteractiveService) maybeClearPlanContextForPlanStep(ctx context.Context, runID string, rs *interactiveRun, stepID string) {
	if s.workflowStore == nil || stepID == "" {
		return
	}
	steps, err := s.workflowStore.LoadRunSteps(ctx, runID)
	if err != nil {
		return
	}
	for _, st := range steps {
		if st.ID == stepID && isPlanStepType(st.StepType) {
			s.mu.Lock()
			rs.planContextPackage = nil
			s.mu.Unlock()
			return
		}
	}
}
