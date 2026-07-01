package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// BuiltinFlowOption is one entry a Chat Mode "Built-in orchestration" picker
// can render for the active sub-mode.
type BuiltinFlowOption struct {
	FlowRef     string `json:"flowRef"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// BuiltinOrchestrationOptions returns the built-in flow options Chat Mode
// should offer for subMode, computed entirely from pack metadata (CP-42 P-9,
// Task-177 T-4):
//   - the flow's builtin.selectableIn must contain "chat"
//   - the flow must not be the chatBaseline (baseline is always-on, never a
//     picker option)
//   - the flow's builtin.chatSubModes must contain subMode
//
// An empty subMode or one with no matching flow returns an empty, non-nil
// slice rather than an error, so a caller can render "no picker" for
// sub-modes like normal/task without special-casing them here — adding a
// picker for a new sub-mode is a pack-data change, not a code change
// (Task-177 T-6).
func BuiltinOrchestrationOptions(subMode string) ([]BuiltinFlowOption, error) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return nil, fmt.Errorf("builtin orchestration options: %w", err)
	}
	subMode = strings.ToLower(strings.TrimSpace(subMode))
	opts := make([]BuiltinFlowOption, 0)
	if subMode == "" {
		return opts, nil
	}
	for _, def := range pack.Flows {
		if def.Builtin.ChatBaseline {
			continue
		}
		if !containsFold(def.Builtin.SelectableIn, "chat") {
			continue
		}
		if !containsFold(def.Builtin.ChatSubModes, subMode) {
			continue
		}
		opts = append(opts, BuiltinFlowOption{
			FlowRef:     canonicalFlowRef(pack.Manifest.ID, def.ID),
			Label:       flowOptionLabel(def.ID),
			Description: def.Description,
		})
	}
	return opts, nil
}

// validateChatOrchestrationSelection checks that flowRef, if provided, is one
// of the built-in options offered for subMode. An empty flowRef (normal chat,
// no orchestration) is always valid regardless of subMode. This validates the
// request contract only — resolving and executing the selected flow into the
// run loop is a separate step not wired into startTurn yet (Task-177).
func validateChatOrchestrationSelection(subMode, flowRef string) error {
	flowRef = strings.TrimSpace(flowRef)
	if flowRef == "" {
		return nil
	}
	opts, err := BuiltinOrchestrationOptions(subMode)
	if err != nil {
		return fmt.Errorf("validate orchestration selection: %w", err)
	}
	for _, opt := range opts {
		if opt.FlowRef == flowRef {
			return nil
		}
	}
	return fmt.Errorf("flowRef %q is not a valid built-in orchestration option for subMode %q", flowRef, subMode)
}

// containsFold reports whether values contains target, case-insensitively.
func containsFold(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}

// flowOptionLabel turns a kebab-case flow ID like "review-loop" into a
// display label like "Review Loop". The pack schema has no dedicated label
// field yet, so this is a display-only derivation, not a semantic lookup.
func flowOptionLabel(flowID string) string {
	parts := strings.Split(flowID, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
