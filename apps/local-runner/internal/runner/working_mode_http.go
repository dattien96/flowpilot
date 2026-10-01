package runner

import (
	"context"
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
	if in.SpawnedInternally {
		// BUG-547: engine-internal child spawns carry the parent's workflowID
		// as lineage metadata — they are not user mounts. Skip the client and
		// start-family gates (both were validated when the parent started);
		// the inherited mode still normalizes/stamps above.
		return nil
	}
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
	// CA-1078: a Flow-mode picker launch sends the workflow row's catalog
	// UUID as WorkflowID, not the pack flow identity — a builtin mirror's
	// UUID (workflows.id → pack_flow_id=vibe-tasks) reads as an unknown id
	// and fails the vibe family gate with working_mode_flow_forbidden.
	// Resolve to the canonical pack ref before gating. An unresolvable ref
	// keeps its raw value: dev still admits plain admin workflows, vibe
	// still fails closed on unknown/system ids.
	s.mu.Lock()
	store := s.flowDefinitionStore
	s.mu.Unlock()
	if rec, rerr := NewFlowDefinitionResolver(store).
		ResolveFlowRef(context.Background(), flowID); rerr == nil &&
		strings.TrimSpace(rec.FlowRef) != "" {
		flowID = rec.FlowRef
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
