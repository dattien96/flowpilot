package cli

import (
	"testing"
)

// TestChatCommand_RegistersOnRoot verifies flowpilot chat is on the real root command.
func TestChatCommand_RegistersOnRoot(t *testing.T) {
	root := NewRootCommand()
	chatCmd, _, err := root.Find([]string{"chat"})
	if err != nil {
		t.Fatalf("Find('chat'): %v", err)
	}
	if chatCmd == nil || chatCmd.Use != "chat" {
		t.Fatalf("chat command not found")
	}
	wantFlags := []string{
		"runner-url", "no-start-runner", "project", "provider", "model",
		"reasoning", "yolo", "print", "prompt", "resume", "timeout",
	}
	for _, flag := range wantFlags {
		if chatCmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s missing on chat command", flag)
		}
	}
}
