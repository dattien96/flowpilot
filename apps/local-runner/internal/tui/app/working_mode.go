package app

import (
	"strings"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/prefs"
	"flowpilot-runner/internal/workingmode"
)

func savedWorkingMode(haveSaved, skip bool, saved prefs.Session) string {
	if !haveSaved || skip {
		return ""
	}
	return strings.TrimSpace(saved.WorkingMode)
}

func (m *AppModel) flowCatalogForWorkingMode() ([]client.BuiltinFlowOption, []client.Workflow) {
	if m == nil {
		return nil, nil
	}
	if m.workingMode != workingmode.Dev && m.workingMode != workingmode.Vibe {
		return m.flowBuiltins, m.flowWorkflows
	}
	allow := map[string]struct{}{}
	for _, id := range workingmode.FlowPickerOptions(m.workingMode) {
		allow[id] = struct{}{}
	}
	seen := map[string]struct{}{}
	var builtins []client.BuiltinFlowOption
	for _, opt := range m.flowBuiltins {
		id := workingmode.BareFlowID(opt.FlowRef)
		if _, ok := allow[id]; !ok {
			continue
		}
		builtins = append(builtins, opt)
		seen[id] = struct{}{}
	}
	if m.workingMode == workingmode.Vibe {
		for _, id := range workingmode.FlowPickerOptions(workingmode.Vibe) {
			if _, ok := seen[id]; ok {
				continue
			}
			builtins = append(builtins, client.BuiltinFlowOption{FlowRef: id, Label: id})
		}
		return builtins, nil
	}
	var workflows []client.Workflow
	for _, wf := range m.flowWorkflows {
		if workingmode.LooksLikeVibeFlow(wf.Name) || workingmode.LooksLikeVibeFlow(wf.ID) {
			continue
		}
		workflows = append(workflows, wf)
	}
	return builtins, workflows
}

func (m *AppModel) setWorkingMode(mode string) {
	m.workingMode = mode
	if m.mode != ModeChat || m.launch.IsArmed() {
		m.mode = ModeChat
		m.launch = LaunchArm{}
		m.firstTurnPending = false
	}
	m.persistSessionPrefs()
}

// isVibeCpSourcedFlow reports whether a picker/flow id takes a CP-*.md
// `@path` source at launch — vibe-cp-ingest slices it; vibe-tasks (CP-90)
// sprints its existing parented Task files. Same arming contract.
func isVibeCpSourcedFlow(ref string) bool {
	id := workingmode.BareFlowID(ref)
	return id == "vibe-cp-ingest" || id == "vibe-tasks"
}

func (m *AppModel) workingModeChip() string {
	if m != nil && strings.EqualFold(m.workingMode, workingmode.Vibe) {
		return "VIBE"
	}
	return ""
}
