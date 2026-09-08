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
	var builtins []client.BuiltinFlowOption
	for _, opt := range m.flowBuiltins {
		if _, ok := allow[workingmode.BareFlowID(opt.FlowRef)]; ok {
			builtins = append(builtins, opt)
		}
	}
	if m.workingMode == workingmode.Vibe {
		if len(builtins) == 0 {
			builtins = []client.BuiltinFlowOption{{FlowRef: "vibe-ingest", Label: "vibe-ingest"}}
		}
		return builtins, nil
	}
	return builtins, m.flowWorkflows
}

func (m *AppModel) setWorkingMode(mode string) {
	m.workingMode = mode
	m.persistSessionPrefs()
}

func (m *AppModel) workingModeChip() string {
	if m != nil && strings.EqualFold(m.workingMode, workingmode.Vibe) {
		return "VIBE"
	}
	return ""
}
