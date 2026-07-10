package runner

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// turnProviderParams is the on-disk shape written by logTurnProviderParams.
type turnProviderParams struct {
	RunID           string `json:"run_id"`
	TurnID          string `json:"turn_id"`
	ProjectID       string `json:"project_id"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Cwd             string `json:"cwd"`
	Yolo            bool   `json:"yolo"`
	LoggedAt        string `json:"logged_at"`
}

// logTurnProviderParams records the actual provider/model/reasoning/cwd/yolo
// values passed to the adapter for this turn — previously only available for
// Flow mode (flowDiagLog); chat-mode runs had no equivalent, which made
// diagnosing "which model/cwd did the provider actually get" (e.g. the Grok
// "Path not found" cwd bug) guesswork. Provider-neutral: called once per turn
// for every ChatMode/FlowMode run alike, right where TurnRequest is built.
//
// Written alongside logComposedPrompt's prompt file under the same per-turn
// run directory (same on/off gate, FLOWPILOT_LOG_PROMPT) so both are found
// together:
//
//	<toolWorkspace>/.flowpilot/runs/<projectID>/<runID>/turn-<turnID>-params.json
//	<toolWorkspace>/.flowpilot/runs/<projectID>/last-turn-params.json   (latest)
func logTurnProviderParams(toolWorkspace, projectID, runID, turnID, provider, model, reasoningEffort, cwd string, yolo bool) {
	if !promptLogEnabled() {
		return
	}
	params := turnProviderParams{
		RunID:           runID,
		TurnID:          turnID,
		ProjectID:       projectID,
		Provider:        provider,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		Cwd:             cwd,
		Yolo:            yolo,
		LoggedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return
	}
	log.Printf("[turn-params] run=%s turn=%s provider=%s model=%s reasoning=%s cwd=%s yolo=%t",
		runID, turnID, provider, model, reasoningEffort, cwd, yolo)

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
	_ = os.WriteFile(filepath.Join(runDir, "turn-"+turnID+"-params.json"), data, 0o644)
	_ = os.WriteFile(filepath.Join(projectDir, "last-turn-params.json"), data, 0o644)
}
