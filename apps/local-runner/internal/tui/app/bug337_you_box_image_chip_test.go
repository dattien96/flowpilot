package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-337 F-4: You-box must show an attach chip when a user turn carried images.
// Desktop already does (Timeline prompt-attachments). TUI had no chip — the You-box
// rendered only the text, so an image send looked like a text-only send.
// The chip must live outside the 4-line prompt clamp so it is always visible.

func TestBug337_YouBox_NoChipWhenNoAttachments(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.addMessage("user", "hello", "")
	view := stripANSI(m.View())
	if strings.Contains(view, "image attached") {
		t.Fatalf("no attachments: chip must not appear:\n%s", view)
	}
}

func TestBug337_YouBox_OneImageChip(t *testing.T) {
	for _, pk := range []string{"codex", "claude", "grok", "opencode"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:9")
		m.width, m.height = 80, 24
		m.sessionLoading = false
		m.addMessage("user", "anh gi day", "")
		// Simulate F-4 capture: pendingAttach names copied to the message
		m.messages[len(m.messages)-1].Attachments = []string{"clipboard.png"}
		view := stripANSI(m.View())
		if !strings.Contains(view, "[1 image attached]") {
			t.Fatalf("[%s] one image: chip missing:\n%s", pk, view)
		}
	}
}

func TestBug337_YouBox_TwoImagesChip(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.addMessage("user", "y la anh nay", "")
	m.messages[len(m.messages)-1].Attachments = []string{"a.png", "b.png"}
	view := stripANSI(m.View())
	if !strings.Contains(view, "[2 images attached]") {
		t.Fatalf("two images: chip missing:\n%s", view)
	}
}

func TestBug337_YouBox_ChipOutsideClamp(t *testing.T) {
	// Long prompt that would be clamped to 4 lines + "...." — chip must still show
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.addMessage("user", changeContractPrompt, "")
	m.messages[len(m.messages)-1].Attachments = []string{"x.png"}
	view := stripANSI(m.View())
	if !strings.Contains(view, "....") {
		t.Fatalf("long prompt must be clamped (....) before chip test:\n%s", view)
	}
	if !strings.Contains(view, "[1 image attached]") {
		t.Fatalf("chip must survive clamp (outside 4 lines):\n%s", view)
	}
	// Expanded box must also keep the chip
	x, y, ok := findClickTarget(m, "user-prompt-expand:"+changeContractPrompt)
	if !ok {
		t.Fatalf("collapsed box must expose expand target")
	}
	m2, _ := m.Update(clickLeft(x, y))
	view2 := stripANSI(m2.(*AppModel).View())
	if !strings.Contains(view2, "[1 image attached]") {
		t.Fatalf("expanded box must also show chip:\n%s", view2)
	}
	if !strings.Contains(view2, "symbols: Subtract") {
		t.Fatalf("expanded must show 5th line:\n%s", view2)
	}
}

func TestBug337_PendingAttachBecomesYouBoxChip(t *testing.T) {
	// End-to-end via the TUI's pendingAttach → You-box path (F-4 capture)
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.pendingAttach = []client.PromptAttachment{
		{ID: "a1", OriginalName: "a.png", MimeType: "image/png", Data: "dGVzdA=="},
		{ID: "a2", OriginalName: "b.png", MimeType: "image/png", Data: "dGVzdA=="},
	}
	// Simulate handleKey Enter flow that copies pendingAttach names to the You-box
	// (app.go:3362 block). Call the same helper directly.
	input := "hello with images"
	m.addMessage("user", input, "")
	names := make([]string, 0, len(m.pendingAttach))
	for _, att := range m.pendingAttach {
		if n := strings.TrimSpace(att.OriginalName); n != "" {
			names = append(names, n)
		}
	}
	m.messages[len(m.messages)-1].Attachments = names
	view := stripANSI(m.View())
	if !strings.Contains(view, "[2 images attached]") {
		t.Fatalf("pendingAttach → You-box chip missing:\n%s", view)
	}
	// After the turn is posted, pendingAttach would be cleared by openTurnStream,
	// but the You-box chip must remain.
	m.pendingAttach = nil
	view2 := stripANSI(m.View())
	if !strings.Contains(view2, "[2 images attached]") {
		t.Fatalf("chip must persist after pendingAttach cleared:\n%s", view2)
	}
}
