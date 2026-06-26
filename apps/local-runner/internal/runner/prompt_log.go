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
// and, when a workspace is known, writes the full prompt to
// `.flowpilot/runs/<runId>/prompt-<turnID>.txt` (and refreshes
// `.flowpilot/runs/last-prompt.txt` for quick access).
func logComposedPrompt(runID, turnID, cwd, prompt string) {
	if !promptLogEnabled() {
		return
	}
	log.Printf("[prompt] run=%s turn=%s bytes=%d\n%s", runID, turnID, len(prompt), prompt)

	if strings.TrimSpace(cwd) == "" {
		return
	}
	runsDir := filepath.Join(cwd, ".flowpilot", "runs")
	dir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "prompt-"+turnID+".txt"), []byte(prompt), 0o644)
	_ = os.WriteFile(filepath.Join(runsDir, "last-prompt.txt"), []byte(prompt), 0o644)
}
