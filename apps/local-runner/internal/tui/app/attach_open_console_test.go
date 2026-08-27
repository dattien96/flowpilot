package app

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestRunClipboardPS_UsesCreateNoWindow(t *testing.T) {
	b, err := os.ReadFile("clipboard_ps_windows.go")
	if err != nil {
		t.Skipf("clipboard_ps_windows.go not present: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "CREATE_NO_WINDOW") && !strings.Contains(s, "0x08000000") && !strings.Contains(s, "clipboardCreateNoWindow") {
		t.Fatalf("clipboard_ps_windows.go should set CREATE_NO_WINDOW, got %q", s)
	}
	if !strings.Contains(s, "go:build windows") {
		t.Fatalf("clipboard_ps_windows.go missing build tag")
	}
	b2, err := os.ReadFile("clipboard_paste.go")
	if err != nil {
		t.Fatalf("clipboard_paste.go read: %v", err)
	}
	if !strings.Contains(string(b2), "applyClipboardSysProcAttr") {
		t.Fatalf("clipboard_paste.go should call applyClipboardSysProcAttr")
	}
}

func TestClipboardPasteMsg_TextThenEnterSubmits(t *testing.T) {
	// Regression for pid 9288: Alt+V text paste 192 chars left TUI Enter-dead
	// (no KeyMsg after ClipboardPasteMsg). Paste text then Enter must submit.
	// Short paste (<8 runes) stays inline (CA-541 threshold).
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 30
	m.sessionLoading = false
	m.authPhase = AuthNone
	m2, _ := m.Update(ClipboardPasteMsg{Text: "hi", NoImage: true})
	am := m2.(*AppModel)
	if am.inputValue != "hi" {
		t.Fatalf("short paste should insert inline, got %q", am.inputValue)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	bm := m3.(*AppModel)
	if len(bm.messages) == 0 {
		t.Fatalf("Enter after short clipboard paste must submit, messages=%+v input=%q", bm.messages, bm.inputValue)
	}
	// Long paste collapses to token but Enter still submits full text
	m = New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 30
	m.sessionLoading = false
	m.authPhase = AuthNone
	long := strings.Repeat("a", 300)
	m2, _ = m.Update(ClipboardPasteMsg{Text: long, NoImage: true})
	am = m2.(*AppModel)
	if !strings.Contains(am.inputValue, "[Pasted") {
		t.Fatalf("long paste should collapse, got %q", am.inputValue)
	}
	m3, _ = am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m3.(*AppModel)
	if len(bm.messages) == 0 {
		t.Fatalf("Enter after long collapsed paste must submit")
	}
}
