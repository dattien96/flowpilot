package app

// Worktree start option + status badge (CP-71 P-3 / Task-409 T-4).
// Mirrors working_mode.go plumbing: a session flag armed via /worktree that
// flows into the next StartRunInput; mid-run pin is respected because the
// flag is only read at start. The badge shows the armed state or the live
// binding state echoed back on the run handle.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
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

// cmdWorktreeMerge resolves the run's worktree merge decision (Task-437).
// No-arg probes GetRun and reports the live binding state. With a mode it
// POSTs /worktree/resolve; the Update handler turns 409 requiresConfirm into
// a file list + --confirm escape hatch, and 409 conflict into fail-closed
// paths + patch-artifact guidance. Mirrors cmdGrokYoloPosture plumbing.
func (m *AppModel) cmdWorktreeMerge(args []string) tea.Cmd {
	if m.runHandle == nil {
		m.addMessage("system", "No active run — open a run bound to a worktree first.", "error")
		return nil
	}
	runID := m.runHandle.RunID
	runnerURL := m.runnerURL
	mode, confirm := "", false
	for _, a := range args {
		if a == "--confirm" {
			confirm = true
			continue
		}
		if mode != "" {
			m.addMessage("system", "Usage: /wt-merge [apply_patch|keep_branch|discard|archive|recreate_empty] [--confirm]", "error")
			return nil
		}
		mode = a
	}
	if mode != "" {
		switch mode {
		case "apply_patch", "keep_branch", "discard", "archive", "recreate_empty":
		default:
			m.addMessage("system", "Unknown mode "+mode+". Usage: /wt-merge [apply_patch|keep_branch|discard|archive|recreate_empty] [--confirm]", "error")
			return nil
		}
	}
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if mode == "" {
			snap, err := cl.GetRun(context.Background(), runID)
			if err != nil {
				return worktreeMergeMsg{Err: err}
			}
			return worktreeMergeMsg{Snap: &snap}
		}
		res, err := cl.ResolveWorktree(context.Background(), runID, mode, confirm)
		if err != nil {
			return worktreeMergeMsg{Err: err, Mode: mode}
		}
		return worktreeMergeMsg{Res: res, Mode: mode}
	}
}

// handleWorktreeMergeMsg renders the /wt-merge outcome. 409 bodies are read
// off APIError.Details: requiresConfirm lists the files at risk and names the
// --confirm retry; a merge conflict prints conflictPaths + patchArtifactRef
// and stays fail-closed (the runner never force-applies).
func (m *AppModel) handleWorktreeMergeMsg(msg worktreeMergeMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		var apiErr *client.APIError
		if errors.As(msg.Err, &apiErr) && apiErr.Status == 409 {
			if apiErr.Details["requiresConfirm"] == true {
				files := append(anyStrings(apiErr.Details["uncommitted"]), anyStrings(apiErr.Details["untracked"])...)
				m.addMessage("system", fmt.Sprintf("Worktree resolve needs confirmation — %d file(s) would be lost: %s. Re-run with /wt-merge %s --confirm to proceed.",
					len(files), strings.Join(files, ", "), msg.Mode), "error")
				return m, nil
			}
			// Wire-truth: the evidence body carries conflict:true, not a code
			// field — detect on details first (code kept for compatibility).
			if apiErr.Details["conflict"] == true || apiErr.Code == "worktree_merge_conflict" {
				paths := anyStrings(apiErr.Details["conflictPaths"])
				ref, _ := apiErr.Details["patchArtifactRef"].(string)
				m.addMessage("system", fmt.Sprintf("Merge conflict — fix %s in the main workspace, then re-run /wt-merge %s. Patch kept at %s",
					strings.Join(paths, ", "), msg.Mode, ref), "error")
				return m, nil
			}
		}
		m.addMessage("system", "Worktree resolve failed: "+msg.Err.Error(), "error")
		return m, nil
	}
	if msg.Snap != nil {
		wt := msg.Snap.Worktree
		if wt == nil || wt.State == "" {
			m.addMessage("system", "No worktree binding on this run. Usage: /wt-merge apply_patch|keep_branch|discard [--confirm]", "")
			return m, nil
		}
		m.noteWorktreeBinding(wt.State, wt.Slug)
		m.addMessage("system", fmt.Sprintf("Worktree %s — %s. Resolve: /wt-merge apply_patch|keep_branch|discard [--confirm]; lost binding: archive|recreate_empty",
			wt.State, wt.Path), "")
		return m, nil
	}
	res := msg.Res
	m.noteWorktreeBinding(res.WorktreeState, m.worktreeSlug)
	out := "Worktree → " + res.WorktreeState
	if res.Branch != "" {
		out += " (branch " + res.Branch + ")"
	}
	if res.Applied {
		out += " — patch applied to the main workspace"
	}
	if res.PriorChangesLost {
		out += " — prior worktree changes were lost"
	}
	if res.Archived {
		out += " — archived"
	}
	m.addMessage("system", out, "")
	return m, nil
}

func anyStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
