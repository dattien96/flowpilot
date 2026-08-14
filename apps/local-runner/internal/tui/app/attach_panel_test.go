package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func samplePendingAtt(id, name string) client.PromptAttachment {
	// minimal valid-looking base64 (not a real image — materialize still writes bytes)
	return client.PromptAttachment{
		ID:           id,
		Kind:         "image",
		OriginalName: name,
		MimeType:     "image/png",
		Data:         "aGVsbG8=", // "hello"
		SizeBytes:    5,
		Width:        10,
		Height:       10,
	}
}

func TestAttachPanel_OpenAndRemoveByClick(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 100, 40
	m.sessionLoading = false
	m.asciiMode = true
	m.appendPendingAttachment(samplePendingAtt("id-a", "a.png"))
	m.appendPendingAttachment(samplePendingAtt("id-b", "b.png"))
	if len(m.pendingLocalPaths) != 2 {
		t.Fatalf("expected 2 local paths, got %d", len(m.pendingLocalPaths))
	}
	pathA := m.pendingLocalPaths["id-a"]
	if pathA == "" {
		t.Fatal("missing path for id-a")
	}
	if _, err := os.Stat(pathA); err != nil {
		t.Fatalf("temp file missing: %v", err)
	}

	m.openAttachPanel()
	c := m.tuiChrome()
	if c.attachPanelH < 3 {
		t.Fatalf("attach panel height=%d block=%q", c.attachPanelH, c.attachPanelBlock)
	}
	// Find first [x] (remove) — not [open].
	lines := strings.Split(c.attachPanelBlock, "\n")
	var y, x int
	found := false
	for i, line := range lines {
		plain := stripANSI(line)
		if j := strings.Index(plain, "[x]"); j >= 0 {
			y = c.attachPanelY + i
			x = lipgloss.Width(plain[:j])
			found = true
			action, idx := hitAttachPanelAction(c, x, y)
			if action != "rm" || idx != 1 {
				t.Fatalf("hitAttachPanelAction=%s/%d want rm/1 (y=%d x=%d line=%q)", action, idx, y, x, plain)
			}
			break
		}
	}
	if !found {
		t.Fatalf("no [x] in panel:\n%s", c.attachPanelBlock)
	}
	target := m.clickTargetAt(x, y)
	if target != "attach-rm:1" {
		t.Fatalf("clickTargetAt=%q want attach-rm:1", target)
	}
	m2, _ := m.dispatchMouseClick(x, y)
	am := m2.(*AppModel)
	if len(am.pendingAttach) != 1 {
		t.Fatalf("pending=%d want 1 (target was hit)", len(am.pendingAttach))
	}
	if am.pendingAttach[0].ID != "id-b" {
		t.Fatalf("remaining id=%q want id-b", am.pendingAttach[0].ID)
	}
	if _, err := os.Stat(pathA); !os.IsNotExist(err) {
		t.Fatalf("temp file should be deleted: %v path=%s", err, pathA)
	}
}

func TestAttachPanel_ChipClickToggles(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 30
	m.sessionLoading = false
	m.asciiMode = true
	m.appendPendingAttachment(samplePendingAtt("z1", "z.png"))
	_ = m.View()
	c := m.tuiChrome()
	// Click chip in input row
	stripped := stripANSI(strings.Split(c.inputBlock, "\n")[1])
	i := strings.Index(stripped, "[1 img]")
	if i < 0 {
		t.Fatalf("chip missing in input:\n%s", stripped)
	}
	m2, _ := m.dispatchMouseClick(i, c.inputY+1)
	am := m2.(*AppModel)
	if !am.attachPanelOpen {
		t.Fatal("expected panel open")
	}
	m3, _ := am.dispatchMouseClick(i, c.inputY+1)
	if m3.(*AppModel).attachPanelOpen {
		t.Fatal("second chip click should close panel")
	}
}

func TestImageRmCommand_RemovesAndDeletesTemp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.appendPendingAttachment(samplePendingAtt("rm1", "rm.png"))
	path := m.pendingLocalPaths["rm1"]
	m2, _ := m.dispatchImageCommand([]string{"rm", "1"})
	am := m2.(*AppModel)
	if len(am.pendingAttach) != 0 {
		t.Fatalf("pending=%d", len(am.pendingAttach))
	}
	if path != "" {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("temp should be gone: %s", path)
		}
	}
}

func TestEscClosesAttachPanel(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.appendPendingAttachment(samplePendingAtt("e1", "e.png"))
	m.openAttachPanel()
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m2.(*AppModel).attachPanelOpen {
		t.Fatal("esc should close panel")
	}
}

func TestHitAttachChipToken_FullChipNotOnlyBracket(t *testing.T) {
	// Old bug: only "[" was clickable (1 cell). Whole "[2 img]" must hit.
	line := "┃ [2 img] hello world"
	// Visual start of chip after "┃ "
	chip := "[2 img]"
	i := strings.Index(line, chip)
	if i < 0 {
		t.Fatal("chip missing")
	}
	start := lipgloss.Width(line[:i])
	// Click middle of "2 img"
	mid := start + 3
	if !hitAttachChipToken(line, mid) {
		t.Fatalf("mid-chip x=%d should hit on %q", mid, line)
	}
	// Click on last char of chip
	end := start + lipgloss.Width(chip) - 1
	if !hitAttachChipToken(line, end) {
		t.Fatalf("end-chip x=%d should hit", end)
	}
	// Far into prompt body should miss
	if hitAttachChipToken(line, start+lipgloss.Width(chip)+5) {
		t.Fatal("prompt body should not open attach panel")
	}
}

func TestHitAttachPanel_OpenAndRemoveChips(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 30
	m.sessionLoading = false
	m.asciiMode = true
	m.appendPendingAttachment(samplePendingAtt("w1", "w.png"))
	m.openAttachPanel()
	c := m.tuiChrome()
	y := c.attachPanelY + 2
	lines := strings.Split(c.attachPanelBlock, "\n")
	if y-c.attachPanelY >= len(lines) {
		t.Fatal("no data row")
	}
	plain := stripANSI(lines[y-c.attachPanelY])
	// Body / name → open
	if action, idx := hitAttachPanelAction(c, 5, y); action != "open" || idx != 1 {
		t.Fatalf("body hit=%s/%d want open/1 line=%q", action, idx, plain)
	}
	// [open] chip
	if j := strings.Index(plain, "[open]"); j >= 0 {
		x := lipgloss.Width(plain[:j]) + 2
		if action, idx := hitAttachPanelAction(c, x, y); action != "open" || idx != 1 {
			t.Fatalf("[open] hit=%s/%d", action, idx)
		}
	} else {
		t.Fatalf("missing [open] in %q", plain)
	}
	// [x] chip
	if j := strings.Index(plain, "[x]"); j >= 0 {
		x := lipgloss.Width(plain[:j])
		if action, idx := hitAttachPanelAction(c, x, y); action != "rm" || idx != 1 {
			t.Fatalf("[x] hit=%s/%d", action, idx)
		}
	} else {
		t.Fatalf("missing [x] in %q", plain)
	}
}

func TestClearPending_CleansTempDirFiles(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.appendPendingAttachment(samplePendingAtt("c1", "c.png"))
	p := m.pendingLocalPaths["c1"]
	n := m.clearPendingAttachments()
	if n != 1 || len(m.pendingAttach) != 0 {
		t.Fatalf("n=%d pending=%d", n, len(m.pendingAttach))
	}
	if p != "" {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("uncleared temp %s", p)
		}
	}
	// pending root may still exist empty — fine
	_ = filepath.Dir(p)
}
