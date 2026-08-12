package app

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/tui/client"
)

// LaunchArm is the armed launch target for /flow or /step (Task-283).
// Builtin pack refs use normal_chat + first-turn subMode/flowRef.
// Catalog workflows use StartRun workflowId (runner resolves first step).
type LaunchArm struct {
	Mode        Mode
	FlowRef     string // builtin pack ref only (e.g. pack/review-loop)
	SubMode     string // "bug" for builtin orchestration
	ChangeType  string // "bugfix" for bug subMode first turn
	SourceDocID string
	WorkflowID  string // catalog workflow id
	StepID      string // optional; empty → runner picks first step
	Label       string // human name for statusline / messages
}

func (a LaunchArm) IsArmed() bool {
	return strings.TrimSpace(a.FlowRef) != "" ||
		strings.TrimSpace(a.WorkflowID) != "" ||
		strings.TrimSpace(a.StepID) != ""
}

func (a LaunchArm) IsBuiltin() bool {
	return strings.TrimSpace(a.FlowRef) != "" && strings.TrimSpace(a.WorkflowID) == ""
}

func (a LaunchArm) IsCatalogWorkflow() bool {
	return strings.TrimSpace(a.WorkflowID) != ""
}

func (a LaunchArm) StatusLabel() string {
	if s := strings.TrimSpace(a.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.FlowRef); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.WorkflowID); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.StepID); s != "" {
		return s
	}
	return ""
}

// ToStartRunInput maps the arm to the desktop-equivalent start payload.
func (a LaunchArm) ToStartRunInput(projectID, provider, model, reasoning, cwd string, yolo bool) client.StartRunInput {
	in := client.StartRunInput{
		ProjectID:       projectID,
		ProviderKey:     provider,
		Model:           model,
		ReasoningEffort: reasoning,
		YoloMode:        yolo,
		Cwd:             cwd,
	}
	if a.IsCatalogWorkflow() || (a.Mode == ModeStep && a.StepID != "") {
		in.WorkflowID = a.WorkflowID
		in.StepID = a.StepID
		// No chatMode — runner treats this as a workflow run and resolves steps.
		return in
	}
	in.ChatMode = "normal_chat"
	return in
}

// FirstTurnExtras returns builtin orchestration fields for the first chat turn only.
// Catalog workflow arms return ok=false (extras must not be sent).
func (a LaunchArm) FirstTurnExtras() (subMode, flowRef, changeType, sourceDocID string, ok bool) {
	if !a.IsBuiltin() {
		return "", "", "", "", false
	}
	return a.SubMode, a.FlowRef, a.ChangeType, a.SourceDocID, true
}

// resolveFlowLaunch picks builtin pack ref first, then catalog workflow (Task-283 Q-1).
func resolveFlowLaunch(
	builtins []client.BuiltinFlowOption,
	workflows []client.Workflow,
	projectID, query string,
) (LaunchArm, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return LaunchArm{}, fmt.Errorf("flow ref required — try /flow list")
	}

	for _, opt := range builtins {
		if strings.EqualFold(strings.TrimSpace(opt.FlowRef), q) {
			return builtinArm(opt), nil
		}
	}
	for _, opt := range builtins {
		if strings.EqualFold(strings.TrimSpace(opt.Label), q) {
			return builtinArm(opt), nil
		}
	}

	var projectScoped []client.Workflow
	for _, wf := range workflows {
		if projectID != "" && wf.ProjectID != "" && wf.ProjectID != projectID {
			continue
		}
		projectScoped = append(projectScoped, wf)
	}
	for _, wf := range projectScoped {
		if strings.EqualFold(strings.TrimSpace(wf.ID), q) {
			return catalogArm(wf), nil
		}
	}
	for _, wf := range projectScoped {
		if strings.EqualFold(strings.TrimSpace(wf.Name), q) {
			return catalogArm(wf), nil
		}
	}

	return LaunchArm{}, fmt.Errorf("flow %q not found — configure in Desktop → Settings", query)
}

func builtinArm(opt client.BuiltinFlowOption) LaunchArm {
	label := strings.TrimSpace(opt.Label)
	if label == "" {
		label = opt.FlowRef
	}
	return LaunchArm{
		Mode:       ModeFlow,
		FlowRef:    opt.FlowRef,
		SubMode:    "bug",
		ChangeType: "bugfix",
		Label:      label,
	}
}

func catalogArm(wf client.Workflow) LaunchArm {
	label := strings.TrimSpace(wf.Name)
	if label == "" {
		label = wf.ID
	}
	return LaunchArm{
		Mode:       ModeFlow,
		WorkflowID: wf.ID,
		Label:      label,
	}
}

// resolveFlowRef is the legacy helper used by existing A4.1 tests.
// Builtin → (flowRef, subMode); catalog → (workflowID, "").
func resolveFlowRef(builtins []client.BuiltinFlowOption, workflows []client.Workflow, query string) (flowRef, subMode string, err error) {
	arm, err := resolveFlowLaunch(builtins, workflows, "", query)
	if err != nil {
		return "", "", err
	}
	if arm.IsBuiltin() {
		return arm.FlowRef, arm.SubMode, nil
	}
	return arm.WorkflowID, "", nil
}
