package tooling

import (
	"context"
	"os/exec"
	"strings"
)

// InstallResult is returned by InstallLibreTranslate.
type InstallResult struct {
	Output  string `json:"output"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// InstallLibreTranslate runs pip install libretranslate with combined output captured.
// Callers must supply a long-timeout context (10+ minutes is recommended).
func InstallLibreTranslate(ctx context.Context) InstallResult {
	for _, pip := range []string{"pip", "pip3"} {
		if _, err := exec.LookPath(pip); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, pip, "install", "libretranslate").CombinedOutput()
		output := strings.TrimSpace(string(out))
		if err == nil {
			return InstallResult{Output: output, Success: true}
		}
		return InstallResult{Output: output, Success: false, Error: err.Error()}
	}
	return InstallResult{
		Success: false,
		Error:   "pip or pip3 not found — install Python 3.8+ first",
	}
}
