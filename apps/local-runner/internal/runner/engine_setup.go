package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/skillpack"
	"flowpilot-runner/internal/tooling"
)

const (
	engineInitTriggerManual = "manual"
	engineInitTriggerBind   = "bind"
)

type engineSetupRequest struct {
	WorkingDirectory string `json:"workingDirectory"`
	Trigger          string `json:"trigger,omitempty"`
	Platform         string `json:"platform,omitempty"`
}

type EngineStatusResponse struct {
	ProjectID        string                        `json:"projectId"`
	WorkingDirectory string                        `json:"workingDirectory"`
	Initialized      bool                          `json:"initialized"`
	Tooling          []tooling.ToolStatus          `json:"tooling"`
	Capability       tooling.CapabilityProfile     `json:"capability"`
	SkillPack        skillpack.PackStatus          `json:"skillPack"`
	LastInit         *EngineInitState              `json:"lastInit,omitempty"`
	Warnings         []string                      `json:"warnings,omitempty"`
}

type EngineGlobalToolingStatusResponse struct {
	Tooling []tooling.ToolStatus `json:"tooling"`
}

type EngineInitState struct {
	Trigger          string                  `json:"trigger"`
	Status           string                  `json:"status"`
	Skipped          bool                    `json:"skipped"`
	SkipReason       string                  `json:"skipReason,omitempty"`
	AttemptedAt      string                  `json:"attemptedAt"`
	CompletedAt      string                  `json:"completedAt"`
	WorkingDirectory string                  `json:"workingDirectory"`
	Install          EngineInstallSummary    `json:"install"`
	Steps            []EngineInitStepResult  `json:"steps"`
}

type EngineInstallSummary struct {
	InstalledPaths []string `json:"installedPaths"`
	SkippedPaths   []string `json:"skippedPaths"`
	Errors         []string `json:"errors"`
}

type EngineInitStepResult struct {
	Step         string `json:"step"`
	Outcome      string `json:"outcome"`
	Detail       string `json:"detail,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

func (s *InteractiveService) handleGetEngineStatus(w http.ResponseWriter, r *http.Request) {
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(r.URL.Query().Get("workingDirectory"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}

	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	status := s.buildEngineStatusResponse(r.PathValue("projectId"), workingDirectory, platform, nil, nil)
	writeInteractiveJSON(w, http.StatusOK, status)
}

func (s *InteractiveService) handleGetGlobalEngineToolingStatus(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, EngineGlobalToolingStatusResponse{
		Tooling: tooling.CheckGlobal(),
	})
}

func (s *InteractiveService) handleInitEngine(w http.ResponseWriter, r *http.Request) {
	var request engineSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err.Error() != "EOF" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}

	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(request.WorkingDirectory)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}

	trigger := strings.TrimSpace(request.Trigger)
	if trigger == "" {
		trigger = engineInitTriggerManual
	}

	status, initState := s.runEngineInit(r.PathValue("projectId"), workingDirectory, strings.TrimSpace(request.Platform), trigger)
	status.LastInit = initState
	writeInteractiveJSON(w, http.StatusOK, status)
}

func (s *InteractiveService) resolveEngineWorkingDirectory(value string) (string, *apiErr) {
	if s.runner == nil {
		return "", newAPIErr(http.StatusBadGateway, "runner_unavailable", "runner is unavailable")
	}

	workingDirectory := strings.TrimSpace(value)
	if workingDirectory == "" {
		return "", newAPIErr(http.StatusBadRequest, "working_directory_required", "workingDirectory is required")
	}

	resolved, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", newAPIErr(http.StatusBadRequest, "invalid_working_directory", "workingDirectory could not be resolved")
	}

	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", newAPIErr(http.StatusBadRequest, "working_directory_unavailable", "workingDirectory must point to an existing directory")
	}

	resolved = strings.TrimSpace(filepath.Clean(resolved))
	return resolved, nil
}

func (s *InteractiveService) buildEngineStatusResponse(
	projectID string,
	workingDirectory string,
	platform string,
	precomputedTooling []tooling.ToolStatus,
	extraWarnings []string,
) EngineStatusResponse {
	dotFlowpilotDir := filepath.Join(workingDirectory, ".flowpilot")
	warnings := append([]string(nil), extraWarnings...)
	toolStatuses := append([]tooling.ToolStatus(nil), precomputedTooling...)
	skillPackState, err := skillpack.Status(workingDirectory, platform)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("skill-pack status warning: %v", err))
	}
	if len(toolStatuses) == 0 {
		toolStatuses = append(toolStatuses, tooling.CheckGlobal()...)
		toolStatuses = append(toolStatuses, buildSkillPackToolStatus(skillPackState))
	}

	var initialized bool
	if info, statErr := os.Stat(dotFlowpilotDir); statErr == nil && info.IsDir() {
		initialized = true
	}

	lastInit, err := loadEngineInitState(dotFlowpilotDir)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("engine-init state warning: %v", err))
	}

	return EngineStatusResponse{
		ProjectID:        projectID,
		WorkingDirectory: workingDirectory,
		Initialized:      initialized,
		Tooling:          toolStatuses,
		Capability:       tooling.ComputeCapabilityProfile(workingDirectory, toolStatuses),
		SkillPack:        skillPackState,
		LastInit:         lastInit,
		Warnings:         warnings,
	}
}

func (s *InteractiveService) runEngineInit(
	projectID string,
	workingDirectory string,
	platform string,
	trigger string,
) (EngineStatusResponse, *EngineInitState) {
	dotFlowpilotDir := filepath.Join(workingDirectory, ".flowpilot")
	attemptedAt := time.Now().UTC().Format(time.RFC3339)
	warnings := []string{}

	currentSkillPack, err := skillpack.Status(workingDirectory, platform)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("skill-pack status warning: %v", err))
	}

	if shouldSkipBindInit(trigger, dotFlowpilotDir, currentSkillPack) {
		initState := &EngineInitState{
			Trigger:          trigger,
			Status:           "skipped",
			Skipped:          true,
			SkipReason:       "engine already initialized for current skill-pack version",
			AttemptedAt:      attemptedAt,
			CompletedAt:      attemptedAt,
			WorkingDirectory: workingDirectory,
			Install:          EngineInstallSummary{},
			Steps: []EngineInitStepResult{
				{Step: "bind_gate", Outcome: "skipped", Detail: "current skill pack and tooling status already present"},
			},
		}
		if err := saveEngineInitState(dotFlowpilotDir, initState); err != nil {
			warnings = append(warnings, fmt.Sprintf("engine-init state warning: %v", err))
		}
		return s.buildEngineStatusResponse(projectID, workingDirectory, platform, nil, warnings), initState
	}

	statuses, toolingErr := tooling.CheckAll(workingDirectory, dotFlowpilotDir)
	installResult, installErr := skillpack.Install(workingDirectory, platform)

	steps := []EngineInitStepResult{
		buildEngineStep("tooling_check", toolingErr, fmt.Sprintf("%d tool entries refreshed", len(statuses))),
		buildEngineStep(
			"skillpack_install",
			installErr,
			fmt.Sprintf("%d installed, %d skipped, %d install errors", len(installResult.Installed), len(installResult.Skipped), len(installResult.Errors)),
		),
	}

	ledgerErr := changeledger.Build(workingDirectory, dotFlowpilotDir)
	steps = append(steps, buildEngineStep("changeledger_build", ledgerErr, filepath.Join(dotFlowpilotDir, "ledger", "feature_history.ndjson")))

	var catalogErr error
	if ledgerErr == nil {
		ledger, err := changeledger.New(dotFlowpilotDir)
		if err != nil {
			catalogErr = err
		} else {
			_, catalogErr = featurecatalog.Build(workingDirectory, ledger, dotFlowpilotDir)
		}
	} else {
		catalogErr = ledgerErr
	}
	steps = append(steps, buildEngineStep("featurecatalog_build", catalogErr, filepath.Join(dotFlowpilotDir, "catalog", "features.ndjson")))

	initState := &EngineInitState{
		Trigger:          trigger,
		Status:           summarizeEngineInitStatus(toolingErr, installErr, installResult.Errors, ledgerErr, catalogErr),
		Skipped:          false,
		AttemptedAt:      attemptedAt,
		CompletedAt:      time.Now().UTC().Format(time.RFC3339),
		WorkingDirectory: workingDirectory,
		Install: EngineInstallSummary{
			InstalledPaths: append([]string(nil), installResult.Installed...),
			SkippedPaths:   append([]string(nil), installResult.Skipped...),
			Errors:         append([]string(nil), installResult.Errors...),
		},
		Steps: steps,
	}

	if err := saveEngineInitState(dotFlowpilotDir, initState); err != nil {
		warnings = append(warnings, fmt.Sprintf("engine-init state warning: %v", err))
	}

	return s.buildEngineStatusResponse(projectID, workingDirectory, platform, statuses, warnings), initState
}

func buildEngineStep(step string, err error, detail string) EngineInitStepResult {
	if err != nil {
		return EngineInitStepResult{
			Step:         step,
			Outcome:      "failed",
			Detail:       detail,
			ErrorMessage: err.Error(),
		}
	}
	return EngineInitStepResult{
		Step:    step,
		Outcome: "ok",
		Detail:  detail,
	}
}

func summarizeEngineInitStatus(
	toolingErr error,
	installErr error,
	installResultErrors []string,
	ledgerErr error,
	catalogErr error,
) string {
	if toolingErr == nil && installErr == nil && len(installResultErrors) == 0 && ledgerErr == nil && catalogErr == nil {
		return "success"
	}
	return "partial"
}

func shouldSkipBindInit(trigger string, dotFlowpilotDir string, skillPackStatus skillpack.PackStatus) bool {
	if trigger != engineInitTriggerBind {
		return false
	}
	if info, err := os.Stat(dotFlowpilotDir); err != nil || !info.IsDir() {
		return false
	}
	if !skillPackStatus.Current {
		return false
	}
	_, err := os.Stat(filepath.Join(dotFlowpilotDir, "engine-init.json"))
	return err == nil
}

func buildSkillPackToolStatus(status skillpack.PackStatus) tooling.ToolStatus {
	toolStatus := tooling.ToolStatus{
		Tool:      "skill_pack",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	switch {
	case status.Current:
		toolStatus.Status = "ok"
	case status.Installed:
		toolStatus.Status = "stale"
	default:
		toolStatus.Status = "missing"
	}
	return toolStatus
}

func loadEngineInitState(dotFlowpilotDir string) (*EngineInitState, error) {
	data, err := os.ReadFile(filepath.Join(dotFlowpilotDir, "engine-init.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state EngineInitState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveEngineInitState(dotFlowpilotDir string, state *EngineInitState) error {
	if err := os.MkdirAll(dotFlowpilotDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dotFlowpilotDir, "engine-init.json"), data, 0o644)
}

func isSameOrWithinPath(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
