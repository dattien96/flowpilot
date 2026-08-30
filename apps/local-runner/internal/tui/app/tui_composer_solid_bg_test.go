package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Composer must be solid #1e1e1e, transcript stays canvas #0d0d0d.
// The watchdog "input stalled ..." warning must not pollute top-left chrome
// and must appear as a single small line outside the composer frame at the
// bottom of the chat pane.

func TestComposer_SolidChatBarAndTranscriptCanvas(t *testing.T) {
	forceTrueColor(t)
	m := New(config.ChatConfig{Provider: "claude", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 24
	m.asciiMode = false
	m.addMessage("assistant", "hello world", "")
	m.inputValue = "abc [coding] def"
	m.inputCursor = -1
	m.selectedSkills = nil
	view := m.View()
	barSeq := bgANSI(colorBg3)
	canvasSeq := bgANSI(colorCanvas)
	lines := strings.Split(view, "\n")
	var sawComposerBar, sawCanvas bool
	var composerHasCanvasLeak bool
	for _, line := range lines {
		plain := stripANSI(line)
		// Composer top/bottom borders contain the rounded glyphs.
		isComposerBorder := strings.Contains(plain, "╭") || strings.Contains(plain, "╰") || strings.Contains(plain, "Chat:")
		isComposerBody := strings.Contains(plain, "abc [coding] def")
		if isComposerBorder || isComposerBody {
			if !strings.Contains(line, barSeq) {
				t.Fatalf("composer line missing bar bg %q: %q", barSeq, line)
			}
			if strings.Contains(line, canvasSeq) {
				composerHasCanvasLeak = true
			}
			sawComposerBar = true
		}
		// Transcript filler / empty area should be canvas (dark #0d0d0d).
		if strings.Contains(line, canvasSeq) {
			sawCanvas = true
		}
	}
	if !sawComposerBar {
		t.Fatalf("composer bar not found in view:\n%s", view)
	}
	if composerHasCanvasLeak {
		t.Fatalf("composer line leaked canvas bg (black hole) – must be solid bar:\n%s", view)
	}
	if !sawCanvas {
		t.Fatalf("transcript canvas bg not found (should be #0d0d0d):\n%s", view)
	}
}

func TestComposer_MentionHighlightsSurviveSolidBar(t *testing.T) {
	forceTrueColor(t)
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width = 80
	m.height = 24
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	m.inputValue = "abc [coding] def"
	m.inputCursor = -1
	view := m.View()
	if !strings.Contains(view, bgANSI(colorBg3)) {
		t.Fatalf("view missing bar bg")
	}
	if !strings.Contains(view, bgANSI(colorCanvas)) {
		t.Fatalf("view missing canvas bg")
	}
}

func TestChatFrameTitle_DoesNotLeakStalled(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.statusMsg = "input stalled: terminal is not delivering keys — close this window to exit (runner is cleaned up)"
	header := stripANSI(m.chatFrameTitle())
	if strings.Contains(strings.ToLower(header), "stalled") || strings.Contains(header, "close this window") {
		t.Fatalf("header must not contain stalled warning: %q", header)
	}
	if !strings.Contains(strings.ToLower(header), "chat") {
		t.Fatalf("header must still contain Chat: %q", header)
	}
}

func TestBottomNotice_ShowsStalledOutsideFrame(t *testing.T) {
	forceTrueColor(t)
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 24
	m.statusMsg = "input stalled: terminal is not delivering keys — close this window to exit (runner is cleaned up)"
	m.inputValue = ""
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "input stalled") {
		t.Fatalf("view must show stalled bottom notice: %q", plain)
	}
	lines := strings.Split(view, "\n")
	var idxBorder, idxNotice = -1, -1
	for i, l := range lines {
		p := stripANSI(l)
		if strings.Contains(p, "╰") && idxBorder == -1 {
			idxBorder = i
		}
		if strings.Contains(strings.ToLower(p), "input stalled") {
			idxNotice = i
		}
	}
	if idxBorder == -1 {
		t.Fatalf("composer bottom border not found")
	}
	if idxNotice == -1 {
		t.Fatalf("stalled notice not found")
	}
	if idxNotice <= idxBorder {
		t.Fatalf("stalled notice must be after composer frame (border=%d notice=%d)", idxBorder, idxNotice)
	}
	// Header must not contain it.
	if strings.Contains(stripANSI(m.chatFrameTitle()), "stalled") {
		t.Fatalf("header leaked stalled")
	}
}

func TestBottomNotice_ShowsCopiedOutsideFrame(t *testing.T) {
	forceTrueColor(t)
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m2, _ := m.Update(CopiedMsg{Kind: "selection"})
	am := m2.(*AppModel)
	view := am.View()
	if !strings.Contains(stripANSI(view), "Copied selection") {
		t.Fatalf("view must show Copied bottom notice: %q", view)
	}
	lines := strings.Split(view, "\n")
	var idxBorder, idxCopied = -1, -1
	for i, l := range lines {
		p := stripANSI(l)
		if strings.Contains(p, "╰") && idxBorder == -1 {
			idxBorder = i
		}
		if strings.Contains(p, "Copied selection") {
			idxCopied = i
		}
	}
	if idxBorder == -1 || idxCopied == -1 {
		t.Fatalf("border=%d copied=%d view=%q", idxBorder, idxCopied, view)
	}
	if idxCopied <= idxBorder {
		t.Fatalf("Copied notice must be after composer frame (border=%d copied=%d)", idxBorder, idxCopied)
	}
	// Top chrome must not contain Copied.
	if strings.Contains(stripANSI(am.chatFrameTitle()), "Copied") {
		t.Fatalf("header leaked Copied")
	}
	// renderStatusLine still returns it for old callers (oracle rule).
	if !strings.Contains(am.renderStatusLine(), "Copied selection") {
		t.Fatalf("renderStatusLine must still return Copied for legacy callers")
	}
	_ = client.SkillSelection{}
}
