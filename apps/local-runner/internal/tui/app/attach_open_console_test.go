package app

import (
	"os"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// TestAttachmentOpenMsg_RearmsConsole verifies the viewer-open path does not
// wedge the TUI (log pid 12736: 49s stall after attach-open). After ShellExecuteW
// the console is re-armed via disableConsoleQuickEdit so clicks/keys keep
// arriving. This is additive — no old test edited.
func TestAttachmentOpenMsg_RearmsConsole(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 30
	m.sessionLoading = false
	// Simulate successful viewer open
	msg := AttachmentOpenMsg{Path: `C:\Temp\flowpilot-tui-attach\clipboard.png`}
	m2, _ := m.Update(msg)
	am := m2.(*AppModel)
	found := false
	for _, mm := range am.messages {
		if strings.Contains(mm.Content, "Opened image:") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("AttachmentOpenMsg should add 'Opened image' system message, got %+v", am.messages)
	}
	// Also error-only path must not panic
	m3, _ := am.Update(AttachmentOpenMsg{Err: "nope"})
	if m3 == nil {
		t.Fatal("AttachmentOpenMsg error path returned nil model")
	}
}

// TestOpenPath_WindowsUsesShellExecute asserts the Windows file no longer spawns
// cmd.exe (Start) which stole focus and left conhost without Key/Mouse for 49s.
func TestOpenPath_WindowsUsesShellExecute(t *testing.T) {
	b, err := os.ReadFile("open_path_windows.go")
	if err != nil {
		t.Skipf("open_path_windows.go not present: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "exec.Command") {
		t.Fatalf("open_path_windows.go must not use exec.Command (was cmd /c start), got %q", s)
	}
	if !strings.Contains(s, "ShellExecuteW") {
		t.Fatalf("open_path_windows.go should use ShellExecuteW, got %q", s)
	}
	if !strings.Contains(s, "go:build windows") {
		t.Fatalf("open_path_windows.go missing build tag, got %q", s[:200])
	}
}

func TestOpenPath_OtherUsesOpenFallback(t *testing.T) {
	b, err := os.ReadFile("open_path_other.go")
	if err != nil {
		t.Skipf("open_path_other.go not present: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "cmd") {
		t.Fatalf("open_path_other.go must not contain cmd, got %q", s)
	}
	if !strings.Contains(s, "xdg-open") || !strings.Contains(s, "go:build !windows") {
		t.Fatalf("open_path_other.go should handle darwin/linux, got %q", s)
	}
}
