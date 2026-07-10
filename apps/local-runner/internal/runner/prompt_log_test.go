package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogComposedPromptDefaultOnOptOut(t *testing.T) {
	tool := t.TempDir() // the FlowPilot tool workspace (NOT the target project)
	promptPath := filepath.Join(tool, ".flowpilot", "runs", "proj-1", "run-x", "prompt-turn-1.txt")
	lastPath := filepath.Join(tool, ".flowpilot", "runs", "proj-1", "last-prompt.txt")

	// On by default (env unset) → writes under the tool workspace, namespaced by project id.
	os.Unsetenv("FLOWPILOT_LOG_PROMPT")
	logComposedPrompt(tool, "proj-1", "run-x", "turn-1", "hello world")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("prompt file = %q, want %q", data, "hello world")
	}
	if _, err := os.Stat(lastPath); err != nil {
		t.Fatalf("expected last-prompt.txt: %v", err)
	}

	// Opt-out: FLOWPILOT_LOG_PROMPT=0 → no file written.
	tool2 := t.TempDir()
	t.Setenv("FLOWPILOT_LOG_PROMPT", "0")
	logComposedPrompt(tool2, "proj-1", "run-y", "turn-1", "secret")
	if _, err := os.Stat(filepath.Join(tool2, ".flowpilot", "runs", "proj-1", "run-y", "prompt-turn-1.txt")); !os.IsNotExist(err) {
		t.Fatal("expected no prompt file when FLOWPILOT_LOG_PROMPT=0")
	}
}

func TestLogTurnProviderParamsDefaultOnOptOut(t *testing.T) {
	tool := t.TempDir()
	paramsPath := filepath.Join(tool, ".flowpilot", "runs", "proj-1", "run-x", "turn-turn-1-params.json")
	lastPath := filepath.Join(tool, ".flowpilot", "runs", "proj-1", "last-turn-params.json")

	os.Unsetenv("FLOWPILOT_LOG_PROMPT")
	logTurnProviderParams(tool, "proj-1", "run-x", "turn-1", "grok", "grok-4.5", "high", "/repo", true)
	data, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read turn-params file: %v", err)
	}
	var got turnProviderParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal turn-params: %v", err)
	}
	want := turnProviderParams{RunID: "run-x", TurnID: "turn-1", ProjectID: "proj-1", Provider: "grok", Model: "grok-4.5", ReasoningEffort: "high", Cwd: "/repo", Yolo: true}
	got.LoggedAt = ""
	if got != want {
		t.Fatalf("turn-params = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(lastPath); err != nil {
		t.Fatalf("expected last-turn-params.json: %v", err)
	}

	// Opt-out: shares FLOWPILOT_LOG_PROMPT with logComposedPrompt (same gate).
	tool2 := t.TempDir()
	t.Setenv("FLOWPILOT_LOG_PROMPT", "0")
	logTurnProviderParams(tool2, "proj-1", "run-y", "turn-1", "codex", "gpt-5.4", "", "/repo", false)
	if _, err := os.Stat(filepath.Join(tool2, ".flowpilot", "runs", "proj-1", "run-y", "turn-turn-1-params.json")); !os.IsNotExist(err) {
		t.Fatal("expected no turn-params file when FLOWPILOT_LOG_PROMPT=0")
	}
}
