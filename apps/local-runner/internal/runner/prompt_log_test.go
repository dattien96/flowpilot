package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogComposedPromptDefaultOnOptOut(t *testing.T) {
	cwd := t.TempDir()
	promptPath := filepath.Join(cwd, ".flowpilot", "runs", "run-x", "prompt-turn-1.txt")

	// On by default (env unset) → writes the per-turn file and the last-prompt pointer.
	os.Unsetenv("FLOWPILOT_LOG_PROMPT")
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

	// Opt-out: FLOWPILOT_LOG_PROMPT=0 → no file written.
	cwd2 := t.TempDir()
	t.Setenv("FLOWPILOT_LOG_PROMPT", "0")
	logComposedPrompt("run-y", "turn-1", cwd2, "secret")
	if _, err := os.Stat(filepath.Join(cwd2, ".flowpilot", "runs", "run-y", "prompt-turn-1.txt")); !os.IsNotExist(err) {
		t.Fatal("expected no prompt file when FLOWPILOT_LOG_PROMPT=0")
	}
}
