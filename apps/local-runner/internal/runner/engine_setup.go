package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/contextsync"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/reqscaffold"
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
	XcodeScheme      string `json:"xcodeScheme,omitempty"`
	XcodeDestination string `json:"xcodeDestination,omitempty"`
}

type EngineStatusResponse struct {
	ProjectID        string                        `json:"projectId"`
	WorkingDirectory string                        `json:"workingDirectory"`
	Initialized      bool                          `json:"initialized"`
	GateMode         string                        `json:"gateMode"`
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

func (s *InteractiveService) handleInstallLibreTranslate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	result := tooling.InstallLibreTranslate(ctx)

	type installResponse struct {
		Success bool                 `json:"success"`
		Output  string               `json:"output"`
		Error   string               `json:"error,omitempty"`
		Tooling []tooling.ToolStatus `json:"tooling"`
	}
	writeInteractiveJSON(w, http.StatusOK, installResponse{
		Success: result.Success,
		Output:  result.Output,
		Error:   result.Error,
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

	platform := strings.TrimSpace(request.Platform)
	xcodeScheme := strings.TrimSpace(request.XcodeScheme)
	xcodeDestination := strings.TrimSpace(request.XcodeDestination)

	// Write/update test-config.json for iOS projects when scheme and destination are
	// provided. This lets the runner auto-capture a baseline without the user needing
	// to create .flowpilot/settings/test-config.json manually. (Task-159)
	if platform == "ios" && xcodeScheme != "" && xcodeDestination != "" {
		dotFP := filepath.Join(workingDirectory, ".flowpilot")
		if err := writeIOSTestConfig(dotFP, xcodeScheme, xcodeDestination); err != nil {
			log.Printf("[engine] iOS test-config write error: %v", err)
		}
	}

	status, initState := s.runEngineInit(r.PathValue("projectId"), workingDirectory, platform, trigger)
	status.LastInit = initState
	writeInteractiveJSON(w, http.StatusOK, status)
}

// writeIOSTestConfig writes .flowpilot/settings/test-config.json with the
// xcodebuild command built from the project's scheme and destination. Existing
// test_dir and result_format values are preserved. (Task-159)
func writeIOSTestConfig(dotFP, xcodeScheme, xcodeDestination string) error {
	settingsDir := filepath.Join(dotFP, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(settingsDir, "test-config.json")
	cfg := map[string]interface{}{}
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	cfg["test_command"] = fmt.Sprintf(
		"xcodebuild test -scheme %s -destination '%s'",
		xcodeScheme, xcodeDestination,
	)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
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
		GateMode:         readGateMode(dotFlowpilotDir),
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

	// Always run an incremental ledger update on bind when .flowpilot already exists.
	// shouldSkipBindInit may short-circuit the full init, which would leave new commits
	// invisible to the oracle until the next manual re-init or post-commit hook fires.
	// changeledger.Build is cursor-based so this is cheap (only new commits processed).
	if trigger == engineInitTriggerBind {
		if _, statErr := os.Stat(dotFlowpilotDir); statErr == nil {
			_ = changeledger.Build(workingDirectory, dotFlowpilotDir)
			if l, lErr := changeledger.New(dotFlowpilotDir); lErr == nil {
				_, _ = featurecatalog.Build(workingDirectory, l, dotFlowpilotDir)
			}
		}
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

	// Install skill pack first so the skill_pack sentinel exists when CheckAll probes it.
	installResult, installErr := skillpack.Install(workingDirectory, platform)
	statuses, toolingErr := tooling.CheckAll(workingDirectory, dotFlowpilotDir)

	// Scaffold requirements/ folder structure with embedded FORMAT-REFERENCE files.
	// Runs on every full init (not skipped); existing files are never overwritten.
	// (CP-35 Task-112)
	scaffoldResult, scaffoldErr := reqscaffold.Scaffold(workingDirectory)
	scaffoldDetail := fmt.Sprintf("%d created, %d skipped, %d errors",
		len(scaffoldResult.Created), len(scaffoldResult.Skipped), len(scaffoldResult.Errors))

	steps := []EngineInitStepResult{
		buildEngineStep(
			"skillpack_install",
			installErr,
			fmt.Sprintf("%d installed, %d skipped, %d install errors", len(installResult.Installed), len(installResult.Skipped), len(installResult.Errors)),
		),
		buildEngineStep("tooling_check", toolingErr, fmt.Sprintf("%d tool entries refreshed", len(statuses))),
		buildEngineStep("req_scaffold", scaffoldErr, scaffoldDetail),
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

	// CP-35 P-4: write gate-config.json with gate_mode:"enforce" on first init.
	// Existing user config is never overwritten.
	writeDefaultGateConfig(dotFlowpilotDir)

	// CP-35: install the post-commit git hook so new commits are visible to the
	// oracle before the next AI turn — without waiting for a session restart.
	hookErr := changeledger.InstallPostCommitHook(workingDirectory)
	steps = append(steps, buildEngineStep("hook_install", hookErr, filepath.Join(workingDirectory, ".git", "hooks", "post-commit")))

	// P-8 (CP-35): create EngineStore subdirs, write local manifest, sync shared
	// files to the project's Drive `context-engine/` folder (best-effort).
	var syncErr error
	var syncDetail string
	if store, storeErr := contextsync.NewEngineStore(dotFlowpilotDir); storeErr != nil {
		syncErr = storeErr
		syncDetail = filepath.Join(dotFlowpilotDir, "manifest.json")
	} else if manifestErr := contextsync.WriteManifest(store); manifestErr != nil {
		syncErr = manifestErr
		syncDetail = filepath.Join(dotFlowpilotDir, "manifest.json")
	} else {
		syncer := s.buildEngineDriveSyncer(projectID)
		result := contextsync.SyncSharedFiles(context.Background(), store, syncer)
		if len(result.Synced) > 0 {
			syncDetail = fmt.Sprintf("manifest ok; %d files synced to drive, %d skipped", len(result.Synced), len(result.Skipped))
		} else {
			syncDetail = fmt.Sprintf("manifest ok; %d files skipped (drive not connected)", len(result.Skipped))
		}
	}
	steps = append(steps, buildEngineStep("contextsync_manifest", syncErr, syncDetail))

	initState := &EngineInitState{
		Trigger:          trigger,
		Status:           summarizeEngineInitStatus(toolingErr, installErr, installResult.Errors, scaffoldErr, ledgerErr, catalogErr, syncErr),
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
	scaffoldErr error,
	ledgerErr error,
	catalogErr error,
	syncErr error,
) string {
	if toolingErr == nil && installErr == nil && len(installResultErrors) == 0 && scaffoldErr == nil && ledgerErr == nil && catalogErr == nil && syncErr == nil {
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
