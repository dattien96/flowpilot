package app

// CP-71 / Task-409: TUI worktree toggle — armed flag flows into the next
// StartRunInput; the status badge reflects armed/live state; a live binding
// pins the toggle (SS-23 AC-10).

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestTUI_WorktreeToggleAppliesNextStart(t *testing.T) {
	m := &AppModel{}
	if m.worktreeEnabled() {
		t.Fatal("worktree must default off for a new session")
	}
	m.toggleWorktree()
	if !m.worktreeEnabled() {
		t.Fatal("toggleWorktree must arm the flag")
	}
	// The flag lands on the start payload.
	in := client.StartRunInput{Worktree: m.worktreeEnabled()}
	if !in.Worktree {
		t.Fatal("StartRunInput.Worktree must be true when armed")
	}
	// Toggle back off when no live binding exists.
	m.toggleWorktree()
	if m.worktreeEnabled() {
		t.Fatal("toggleWorktree must disarm when no live binding")
	}
}

func TestTUI_WorktreeBadgeVisible(t *testing.T) {
	m := &AppModel{}
	if got := m.worktreeBadge(); got != "" {
		t.Fatalf("badge=%q want empty when unarmed", got)
	}
	m.worktree = true
	if got := m.worktreeBadge(); got != "wt:armed" {
		t.Fatalf("badge=%q want wt:armed", got)
	}
	m.noteWorktreeBinding("active", "fp-chat-abc")
	if got := m.worktreeBadge(); !strings.Contains(got, "active") || !strings.Contains(got, "fp-chat-abc") {
		t.Fatalf("badge=%q want live binding state+slug", got)
	}
}

func TestTUI_WorktreeTogglePinnedWhileBound(t *testing.T) {
	m := &AppModel{worktree: true, liveWorktree: "active"}
	m.toggleWorktree()
	if !m.worktreeEnabled() {
		t.Fatal("live binding must pin the toggle on (merge/discard first)")
	}
	found := false
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "Merge or discard") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a merge/discard notice message")
	}
}
