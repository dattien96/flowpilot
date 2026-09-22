package app

// Worktree start option + status badge (CP-71 P-3 / Task-409 T-4).
// Mirrors working_mode.go plumbing: a session flag armed via /worktree that
// flows into the next StartRunInput; mid-run pin is respected because the
// flag is only read at start. The badge shows the armed state or the live
// binding state echoed back on the run handle.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// worktreeEnabled reports whether the next started run will be isolated.
func (m *AppModel) worktreeEnabled() bool {
	return m != nil && m.worktree
}

// toggleWorktree flips the arming flag. Disabling while a live binding exists
// is refused — the merge decision must be resolved first (SS-23 AC-10).
func (m *AppModel) toggleWorktree() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.worktree && (m.liveWorktree == "active" || m.liveWorktree == "merge_pending") {
		m.addMessage("system", "Merge or discard the worktree first — it is still bound to this chat.", "error")
		return nil
	}
	m.worktree = !m.worktree
	state := "OFF"
	if m.worktree {
		state = "ON"
	}
	m.addMessage("system", "Worktree isolation: "+state, "")
	return nil
}

// worktreeBadge renders the status-chrome chip: live binding state when the
// active run carries one, else the armed flag.
func (m *AppModel) worktreeBadge() string {
	if m == nil {
		return ""
	}
	if m.liveWorktree != "" {
		label := "wt:" + m.liveWorktree
		if s := strings.TrimSpace(m.worktreeSlug); s != "" {
			label += "(" + s + ")"
		}
		return label
	}
	if m.worktree {
		return "wt:armed"
	}
	return ""
}

// noteWorktreeBinding stamps the live binding from a run handle so the badge
// reflects the real state after start/resume.
func (m *AppModel) noteWorktreeBinding(state, slug string) {
	m.liveWorktree = strings.TrimSpace(state)
	m.worktreeSlug = strings.TrimSpace(slug)
	if m.liveWorktree != "" {
		m.worktree = true
	}
}
