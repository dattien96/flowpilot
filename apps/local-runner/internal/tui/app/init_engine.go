package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// EngineInitMsg is the result of a TUI /init dispatch.
type EngineInitMsg struct {
	Kind   string
	Result *client.EngineInitResult
	Err    error
}

func (m *AppModel) cmdInitEngine(kind string) tea.Cmd {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "skill" && kind != "all" {
		kind = "all"
	}
	if m.project == nil || strings.TrimSpace(m.project.ID) == "" {
		m.addMessage("system", "No project bound — bind a project first (check runner projects catalog).", "error")
		return nil
	}
	cwd := strings.TrimSpace(m.boundProjectPath())
	if cwd == "" {
		cwd = strings.TrimSpace(m.cfg.ProjectPath)
	}
	if cwd == "" && m.project != nil {
		cwd = strings.TrimSpace(m.project.Path)
	}
	if cwd == "" {
		m.addMessage("system", "No working directory — project has no path and TUI was not given a project directory.", "error")
		return nil
	}
	projectID := m.project.ID
	platform := ""
	if m.project != nil {
		platform = strings.TrimSpace(m.project.Platform)
	}
	cl := m.client
	m.addMessage("system", fmt.Sprintf("Init %s for %s (%s)…", kind, projectID, cwd), "")
	return func() tea.Msg {
		ctx := context.Background()
		res, err := cl.InitEngine(ctx, projectID, cwd, platform, kind)
		return EngineInitMsg{Kind: kind, Result: res, Err: err}
	}
}

func (m *AppModel) handleEngineInitMsg(msg EngineInitMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.addMessage("system", fmt.Sprintf("Init %s failed: %v", msg.Kind, msg.Err), "error")
		return m, nil
	}
	if msg.Result == nil {
		m.addMessage("system", fmt.Sprintf("Init %s: empty response.", msg.Kind), "error")
		return m, nil
	}
	var detail string
	if msg.Result.LastInit != nil {
		installed := len(msg.Result.LastInit.Install.InstalledPaths)
		skipped := len(msg.Result.LastInit.Install.SkippedPaths)
		errs := len(msg.Result.LastInit.Install.Errors)
		status := msg.Result.LastInit.Status
		if status == "" {
			status = "ok"
		}
		detail = fmt.Sprintf("%s — %d installed, %d skipped, %d errors", status, installed, skipped, errs)
		if len(msg.Result.LastInit.Install.Errors) > 0 {
			detail += " — " + strings.Join(msg.Result.LastInit.Install.Errors, "; ")
		}
		if len(msg.Result.Warnings) > 0 {
			detail += " — warnings: " + strings.Join(msg.Result.Warnings, "; ")
		}
	} else if len(msg.Result.Warnings) > 0 {
		detail = strings.Join(msg.Result.Warnings, "; ")
	}
	if detail == "" {
		detail = "done"
	}
	m.addMessage("system", fmt.Sprintf("Init %s: %s", msg.Kind, detail), "")
	return m, nil
}
