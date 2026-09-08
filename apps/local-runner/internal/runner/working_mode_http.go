package runner

import (
	"errors"
	"net/http"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

func mapWorkingModeError(err error) *apiErr {
	if err == nil {
		return nil
	}
	var we *workingmode.Error
	if errors.As(err, &we) {
		status := http.StatusBadRequest
		switch we.Code {
		case workingmode.CodeClientForbidden:
			status = http.StatusForbidden
		case workingmode.CodePinned:
			status = http.StatusConflict
		}
		return newAPIErr(status, we.Code, we.Msg)
	}
	return newAPIErr(http.StatusBadRequest, "invalid_request", err.Error())
}

// enforceWorkingModeStart applies Task-326 T-1/T-3 at createRun (user startKind).
func (s *InteractiveService) enforceWorkingModeStart(in *StartRunInput) *apiErr {
	if in == nil {
		return nil
	}
	mode, err := workingmode.Normalize(in.WorkingMode)
	if err != nil {
		return mapWorkingModeError(err)
	}
	in.WorkingMode = mode
	if err := workingmode.CheckClient(in.Client, mode); err != nil {
		return mapWorkingModeError(err)
	}
	if strings.TrimSpace(in.RunID) != "" {
		return mapWorkingModeError(workingmode.PinnedError())
	}
	flowID := strings.TrimSpace(in.FlowRef)
	if flowID == "" {
		flowID = strings.TrimSpace(in.WorkflowID)
	}
	if flowID == "" {
		return nil
	}
	if err := workingmode.FlowAllowedForWorkingMode(mode, flowID, "user"); err != nil {
		return mapWorkingModeError(err)
	}
	if strings.TrimSpace(in.FlowRef) != "" && in.ChatMode == "" && strings.TrimSpace(in.WorkflowID) == "" {
		in.ChatMode = "normal_chat"
	}
	return nil
}

func (s *InteractiveService) handleFlowPickerOptions(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("workingMode")
	ids := workingmode.FlowPickerOptions(mode)
	opts := make([]BuiltinFlowOption, 0, len(ids))
	for _, id := range ids {
		opts = append(opts, BuiltinFlowOption{FlowRef: id, Label: id})
	}
	writeInteractiveJSON(w, http.StatusOK, opts)
}
