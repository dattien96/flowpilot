package runner

import (
	"context"
	"strings"
)

func (s *InteractiveService) workflowStepByID(runID, stepID string) (RuntimeWorkflowStep, bool) {
	if s == nil || s.workflowStore == nil || runID == "" || stepID == "" {
		return RuntimeWorkflowStep{}, false
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		return RuntimeWorkflowStep{}, false
	}
	for _, step := range steps {
		if step.ID == stepID {
			return step, true
		}
	}
	return RuntimeWorkflowStep{}, false
}

func (s *InteractiveService) latestCompletedStepByType(runID, stepType string) (RuntimeWorkflowStep, bool) {
	if s == nil || s.workflowStore == nil || runID == "" || stepType == "" {
		return RuntimeWorkflowStep{}, false
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		return RuntimeWorkflowStep{}, false
	}
	var found RuntimeWorkflowStep
	for _, step := range steps {
		if !strings.EqualFold(step.StepType, stepType) {
			continue
		}
		if step.Status != StepStatusDone {
			continue
		}
		found = step
	}
	if found.ID == "" {
		return RuntimeWorkflowStep{}, false
	}
	return found, true
}

func (s *InteractiveService) firstStepByType(runID, stepType string) (RuntimeWorkflowStep, bool) {
	if s == nil || s.workflowStore == nil || runID == "" || stepType == "" {
		return RuntimeWorkflowStep{}, false
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		return RuntimeWorkflowStep{}, false
	}
	for _, step := range steps {
		if strings.EqualFold(step.StepType, stepType) {
			return step, true
		}
	}
	return RuntimeWorkflowStep{}, false
}
