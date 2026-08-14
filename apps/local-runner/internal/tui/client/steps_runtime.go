package client

import (
	"context"
	"net/url"
)

// WorkflowStepRuntime mirrors GET /client/workflow-runs/{runId}/steps-runtime step rows
// (Desktop FlowTimelineSidebar / WorkflowStepRuntimeDTO).
type WorkflowStepRuntime struct {
	StepID           string `json:"stepId"`
	StepType         string `json:"stepType"`
	Status           string `json:"status"`
	RetryCount       int    `json:"retryCount"`
	RejectionNote    string `json:"rejectionNote,omitempty"`
	StartedAt        string `json:"startedAt,omitempty"`
	FinishedAt       string `json:"finishedAt,omitempty"`
	RequiresApproval bool   `json:"requiresApproval"`
	BehaviorID       string `json:"behaviorId,omitempty"`
	NodeID           string `json:"nodeId,omitempty"`
	AgentRef         string `json:"agentRef,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	YoloMode         bool   `json:"yoloMode,omitempty"`
}

// WorkflowStepsRuntimeSnapshot mirrors the steps-runtime endpoint body.
type WorkflowStepsRuntimeSnapshot struct {
	RunID    string                `json:"runId"`
	Steps    []WorkflowStepRuntime `json:"steps"`
	Provider string                `json:"provider,omitempty"`
	Model    string                `json:"model,omitempty"`
	YoloMode bool                  `json:"yoloMode,omitempty"`
}

// GetWorkflowStepsRuntime fetches GET /client/workflow-runs/{runId}/steps-runtime.
func (c *Client) GetWorkflowStepsRuntime(ctx context.Context, runID string) (WorkflowStepsRuntimeSnapshot, error) {
	var snap WorkflowStepsRuntimeSnapshot
	err := c.getJSON(ctx, "/client/workflow-runs/"+url.PathEscape(runID)+"/steps-runtime", &snap)
	return snap, err
}
