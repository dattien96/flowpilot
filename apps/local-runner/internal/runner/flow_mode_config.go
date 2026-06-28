package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const flowModeConfigFileName = "flow-mode-config.json"

type FlowModeConfig struct {
	ValidationCommand          string `json:"validationCommand,omitempty"`
	ValidationWorkingDirectory string `json:"validationWorkingDirectory,omitempty"`
	MaxRetries                 int    `json:"maxRetries,omitempty"`
	MaxContextPackageBytes     int    `json:"maxContextPackageBytes,omitempty"`
}

func loadFlowModeConfig(dotFlowpilotDir string) (FlowModeConfig, error) {
	config := FlowModeConfig{
		MaxRetries:             3,
		MaxContextPackageBytes: flowContextDefaultMaxBytes,
	}
	path := filepath.Join(dotFlowpilotDir, "settings", flowModeConfigFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, nil
		}
		return FlowModeConfig{}, err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return FlowModeConfig{}, err
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.MaxContextPackageBytes <= 0 {
		config.MaxContextPackageBytes = flowContextDefaultMaxBytes
	}
	config.ValidationCommand = strings.TrimSpace(config.ValidationCommand)
	config.ValidationWorkingDirectory = strings.TrimSpace(config.ValidationWorkingDirectory)
	return config, nil
}
