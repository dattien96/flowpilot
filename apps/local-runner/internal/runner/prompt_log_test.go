package runner

import (
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
