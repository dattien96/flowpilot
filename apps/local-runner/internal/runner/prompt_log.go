package runner

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// promptLogEnabled reports whether composed-prompt logging is on. It is ON BY
// DEFAULT so the composed prompt is always available for inspection; set
// FLOWPILOT_LOG_PROMPT to a falsey value (0/false/off/no) to turn it off when
// prompts carry sensitive content you don't want written/logged.
func promptLogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWPILOT_LOG_PROMPT"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// logComposedPrompt records the fully-composed per-turn prompt (feature history +
// prior discussion + mode prefix + raw user text — everything the prompt-assembly
// seam produced, before the provider adapter prepends skill content) so an E2E
// tester can inspect exactly what context was injected.
//
// On by default (disable with FLOWPILOT_LOG_PROMPT=0). It logs a one-line marker
// and writes the full prompt under the FlowPilot tool workspace (NOT the target
// project), namespaced by project id:
//
//	<toolWorkspace>/.flowpilot/runs/<projectID>/<runID>/prompt-<turnID>.txt
//	<toolWorkspace>/.flowpilot/runs/<projectID>/last-prompt.txt   (latest, for quick access)
func logComposedPrompt(toolWorkspace, projectID, runID, turnID, prompt string) {
	if !promptLogEnabled() {
		return
	}
	log.Printf("[prompt] project=%s run=%s turn=%s bytes=%d\n%s", projectID, runID, turnID, len(prompt), prompt)

	if strings.TrimSpace(toolWorkspace) == "" {
		return
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = "unknown-project"
	}
	projectDir := filepath.Join(toolWorkspace, ".flowpilot", "runs", projectID)
	runDir := filepath.Join(projectDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(runDir, "prompt-"+turnID+".txt"), []byte(prompt), 0o644)
	_ = os.WriteFile(filepath.Join(projectDir, "last-prompt.txt"), []byte(prompt), 0o644)
}
