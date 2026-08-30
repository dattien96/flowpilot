package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-332 (operator regression report): opening /mode-setup ("Configure
// postures") clipped the modal to its header + tab row — the Model/Reasoning/
// YOLO fields, Save/Cancel buttons and the composer below were pushed past the
// terminal bottom. Root cause: tuiChrome budgeted messagesHeight WITHOUT the
// modal block, and renderChatPane appends the modal after the transcript, so
// the frame overflowed the terminal height and the bottom got cut. The modal
// height must be part of the height budget (transcript shrinks while the modal
// is open) and the modal must render from the same cached block.

func bug332ModalModel(t *testing.T) *AppModel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.width = 100
	m.height = 20
	m.openModeSetupModal(client.ChatPostureConfig{}, "code")
	if !m.modeSetupModalOpen || m.modeSetupModalDraft == nil {
		t.Fatal("mode setup modal must be open with a draft")
	}
	return m
}

func TestBug332ModalHeightIsBudgeted(t *testing.T) {
	m := bug332ModalModel(t)
	c := m.tuiChrome()
	if c.modalH == 0 {
		t.Fatal("tuiChrome must budget the modal block height while it is open")
	}
	want := m.height - c.inputH - c.attachPanelH - c.suggLines - c.bannerLines - c.panelH - c.chatSepH - c.bottomNoticeH - c.modalH
	if want < 1 {
		want = 1
	}
	if c.messagesHeight != want {
		t.Fatalf("messagesHeight = %d, want %d (must subtract modalH=%d)", c.messagesHeight, want, c.modalH)
	}
}

func TestBug332ModalFrameFitsTerminalHeight(t *testing.T) {
	m := bug332ModalModel(t)
	chat := m.renderChatPane(m.chatWidth(), m.height)
	lines := strings.Split(chat, "\n")
	if len(lines) > m.height {
		t.Fatalf("chat pane overflows the terminal with the modal open: %d lines > height %d (bottom rows clipped on screen)", len(lines), m.height)
	}
	if !strings.Contains(chat, "Model:") {
		t.Fatal("modal fields row missing — tab content lost (operator report)")
	}
	if !strings.Contains(chat, "╰") {
		t.Fatal("modal bottom border clipped — Save/Cancel unreachable")
	}
	if !strings.Contains(chat, "Configure postures") {
		t.Fatal("modal title missing")
	}
}

func TestBug332ModalClosedLayoutUnchanged(t *testing.T) {
	m := bug332ModalModel(t)
	m.modeSetupModalOpen = false
	m.modeSetupModalDraft = nil
	c := m.tuiChrome()
	if c.modalH != 0 || c.modalBlock != "" {
		t.Fatalf("closed modal must not occupy budget, got modalH=%d block=%q", c.modalH, c.modalBlock)
	}
	want := m.height - c.inputH - c.attachPanelH - c.suggLines - c.bannerLines - c.panelH - c.chatSepH - c.bottomNoticeH
	if c.messagesHeight != want {
		t.Fatalf("closed-modal messagesHeight = %d, want legacy formula %d", c.messagesHeight, want)
	}
}
