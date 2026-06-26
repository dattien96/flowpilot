package runner

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// logComposedPrompt records the fully-composed per-turn prompt (feature history +
// prior discussion + mode prefix + raw user text — everything the prompt-assembly
// seam produced, before the provider adapter prepends skill content) so an E2E
// tester can inspect exactly what context was injected.
//
// Opt-in via FLOWPILOT_LOG_PROMPT (any non-empty value) because prompts can carry
// sensitive content. When enabled it logs a one-line marker and, when a workspace
// is known, writes the full prompt to `.flowpilot/runs/<runId>/prompt-<turnID>.txt`
// (and refreshes `.flowpilot/runs/last-prompt.txt` for quick access).
func logComposedPrompt(runID, turnID, cwd, prompt string) {
	if strings.TrimSpace(os.Getenv("FLOWPILOT_LOG_PROMPT")) == "" {
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
