package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

type gateConfigPayload struct {
	GateMode string `json:"gateMode"`
}

type setGateConfigRequest struct {
	WorkingDirectory string `json:"workingDirectory"`
	GateMode         string `json:"gate_mode"`
}

// readGateMode reads gate_mode from .flowpilot/settings/gate-config.json.
// When the file is absent and .flowpilot/ already exists, it auto-creates the
// file with gate_mode:"enforce" so the mode is always persisted after first read.
// Returns "enforce" for any absent/unreadable/unrecognised value.
func readGateMode(dotFP string) string {
	data, err := os.ReadFile(filepath.Join(dotFP, "settings", "gate-config.json"))
	if err != nil {
		// Auto-create only when the .flowpilot dir is already initialised.
		if info, statErr := os.Stat(dotFP); statErr == nil && info.IsDir() {
			_ = writeGateMode(dotFP, "enforce")
		}
		return "enforce"
	}
	var cfg struct {
		GateMode string `json:"gate_mode"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "enforce"
	}
	if cfg.GateMode == "warn" {
		return "warn"
	}
	return "enforce"
}

// writeGateMode persists gate_mode to .flowpilot/settings/gate-config.json.
func writeGateMode(dotFP string, mode string) error {
	if mode != "enforce" && mode != "warn" {
		mode = "enforce"
	}
	settingsDir := filepath.Join(dotFP, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]string{"gate_mode": mode}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(settingsDir, "gate-config.json"), data, 0o644)
}

// writeDefaultGateConfig writes gate-config.json with gate_mode:"enforce" only
// when the file does not exist yet (first init). Existing user choices are kept.
func writeDefaultGateConfig(dotFP string) {
	path := filepath.Join(dotFP, "settings", "gate-config.json")
	if _, err := os.Stat(path); err == nil {
		return // already present — do not overwrite
	}
	_ = writeGateMode(dotFP, "enforce")
}

// handleGetEngineGateConfig handles GET /client/projects/{projectId}/engine/gate-config
func (s *InteractiveService) handleGetEngineGateConfig(w http.ResponseWriter, r *http.Request) {
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(r.URL.Query().Get("workingDirectory"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	writeInteractiveJSON(w, http.StatusOK, gateConfigPayload{GateMode: readGateMode(dotFP)})
}

// handleSetEngineGateConfig handles POST /client/projects/{projectId}/engine/gate-config
func (s *InteractiveService) handleSetEngineGateConfig(w http.ResponseWriter, r *http.Request) {
	var req setGateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(req.WorkingDirectory)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	if err := writeGateMode(dotFP, req.GateMode); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "write_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, gateConfigPayload{GateMode: readGateMode(dotFP)})
}
