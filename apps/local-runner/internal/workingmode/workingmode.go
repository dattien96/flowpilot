// Package workingmode is the fail-closed SSOT for CP-60 P-1 / Task-326.
// TUI may import this package; it must not import internal/runner.
package workingmode

import "strings"

const (
	Dev  = "dev"
	Vibe = "vibe"

	CodeInvalidMode     = "invalid_working_mode"
	CodeFlowForbidden   = "working_mode_flow_forbidden"
	CodeInvalidFlowRef  = "invalid_flow_ref"
	CodeClientForbidden = "working_mode_client_forbidden"
	CodePinned          = "working_mode_pinned"

	PackPrefix = "flowpilot-core-flow-pack/"
)

// DevHarnessFive is the user-start allowlist in working_mode=dev (stable order).
var DevHarnessFive = []string{
	"task-harness",
	"bug-harness",
	"bug-plan-harness",
	"cp-harness",
	"context-coding-review-synthesis",
}

var hiddenFlowIDs = map[string]struct{}{
	"review-loop":      {},
	"rag-harness":      {},
	"cp-harness-smoke": {},
}

var harnessFlowIDs = map[string]struct{}{
	"task-harness":                     {},
	"bug-harness":                      {},
	"bug-plan-harness":                 {},
	"cp-harness":                       {},
	"context-coding-review-synthesis":  {},
}

var vibeUserFlowIDs = map[string]struct{}{
	"vibe-ingest":    {},
	"vibe-cp-ingest": {},
}

var vibeSystemFlowIDs = map[string]struct{}{
	"vibe-sprint":        {},
	"vibe-owner-debate":  {},
}

// Error is a frozen-code gate failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg == "" {
		return e.Code
	}
	return e.Code + ": " + e.Msg
}

func errCode(code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

// Normalize returns dev|vibe. Empty → dev. "normal" and unknowns are invalid.
func Normalize(mode string) (string, error) {
	m := strings.TrimSpace(mode)
	if m == "" {
		return Dev, nil
	}
	if m == Dev || m == Vibe {
		return m, nil
	}
	return "", errCode(CodeInvalidMode, "working_mode must be dev or vibe")
}

// BareFlowID strips an optional pack prefix.
func BareFlowID(flowID string) string {
	id := strings.TrimSpace(flowID)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, PackPrefix) {
		return strings.TrimSpace(id[len(PackPrefix):])
	}
	if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
		pack := id[:i]
		rest := id[i+1:]
		if pack == strings.TrimSuffix(PackPrefix, "/") {
			return rest
		}
	}
	return id
}

func isHidden(id string) bool {
	_, ok := hiddenFlowIDs[id]
	return ok
}

func isHarness(id string) bool {
	_, ok := harnessFlowIDs[id]
	return ok
}

func isVibeUser(id string) bool {
	_, ok := vibeUserFlowIDs[id]
	return ok
}

func isVibeSystem(id string) bool {
	_, ok := vibeSystemFlowIDs[id]
	return ok
}

func isVibeFamily(id string) bool {
	if isVibeUser(id) || isVibeSystem(id) {
		return true
	}
	return strings.HasPrefix(id, "vibe-")
}

// LooksLikeVibeFlow is true for vibe-* ids and catalog labels like "Vibe Cp Ingest".
func LooksLikeVibeFlow(idOrName string) bool {
	id := BareFlowID(idOrName)
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return isVibeFamily(id)
}

// SkipChatOrchestrationCheck is true for /flow pack builtins (harness + vibe).
// Those are not Chat Mode subMode picker options; TUI still may send subMode=bug.
func SkipChatOrchestrationCheck(flowRef string) bool {
	id := BareFlowID(flowRef)
	return isHarness(id) || isVibeFamily(id)
}

func isTrackedFlow(id string) bool {
	return isHidden(id) || isHarness(id) || isVibeFamily(id)
}

// FlowAllowedForWorkingMode is the start-family gate (Task-326 T-3).
// Empty startKind is user. Empty flowID is invalid_flow_ref.
func FlowAllowedForWorkingMode(mode, flowID, startKind string) error {
	norm, err := Normalize(mode)
	if err != nil {
		return err
	}
	id := BareFlowID(flowID)
	if id == "" {
		return errCode(CodeInvalidFlowRef, "flow id is required")
	}
	if isHidden(id) {
		return errCode(CodeInvalidFlowRef, "flow is not startable")
	}
	kind := strings.TrimSpace(startKind)
	if kind == "" {
		kind = "user"
	}

	switch kind {
	case "user":
		if norm == Vibe {
			if isVibeUser(id) {
				return nil
			}
			return errCode(CodeFlowForbidden, "working_mode_flow_forbidden")
		}
		// dev user: harness five only among tracked ids; catalog UUIDs pass.
		if isHarness(id) {
			return nil
		}
		if isVibeFamily(id) || isTrackedFlow(id) {
			return errCode(CodeFlowForbidden, "working_mode_flow_forbidden")
		}
		return nil
	case "system":
		if norm == Vibe {
			if isVibeSystem(id) {
				return nil
			}
			return errCode(CodeFlowForbidden, "working_mode_flow_forbidden")
		}
		// dev system cannot start vibe system ids
		if isVibeSystem(id) || isVibeFamily(id) {
			return errCode(CodeFlowForbidden, "working_mode_flow_forbidden")
		}
		return nil
	default:
		return errCode(CodeFlowForbidden, "working_mode_flow_forbidden")
	}
}

// FlowPickerOptions returns user-startable ids. Empty mode lists as dev.
func FlowPickerOptions(mode string) []string {
	norm, err := Normalize(mode)
	if err != nil {
		return nil
	}
	if norm == Vibe {
		return []string{"vibe-ingest"}
	}
	out := make([]string, len(DevHarnessFive))
	copy(out, DevHarnessFive)
	return out
}

// CheckClient enforces X-Client: desktop|tui for vibe. Admin/missing + vibe → 403.
func CheckClient(clientHeader, mode string) error {
	norm, err := Normalize(mode)
	if err != nil {
		return err
	}
	if norm != Vibe {
		return nil
	}
	c := strings.ToLower(strings.TrimSpace(clientHeader))
	if c == "desktop" || c == "tui" {
		return nil
	}
	return errCode(CodeClientForbidden, "vibe is Desktop/TUI only")
}

// PinnedError is returned when a request would mutate a live run's mode.
func PinnedError() error {
	return errCode(CodePinned, "working_mode is immutable on a live run")
}
