package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogComposedPromptOptIn(t *testing.T) {
	cwd := t.TempDir()
	promptPath := filepath.Join(cwd, ".flowpilot", "runs", "run-x", "prompt-turn-1.txt")

	// Disabled by default → no file written.
	os.Unsetenv("FLOWPILOT_LOG_PROMPT")
	logComposedPrompt("run-x", "turn-1", cwd, "hello")
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatal("expected no prompt file when FLOWPILOT_LOG_PROMPT is unset")
	}

	// Enabled → writes the per-turn file and the last-prompt pointer.
	t.Setenv("FLOWPILOT_LOG_PROMPT", "1")
	logComposedPrompt("run-x", "turn-1", cwd, "hello world")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("prompt file = %q, want %q", data, "hello world")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".flowpilot", "runs", "last-prompt.txt")); err != nil {
		t.Fatalf("expected last-prompt.txt: %v", err)
	}
}
