package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/skillpack"
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

	// CP-68 single-command contract (Task-385 T-4): a successful `/init` with the
	// default kind "all" checks the platform's scaffold capability and, when a
	// verified recipe exists, auto-runs the AI Scaffold Turn — the user types
	// nothing else. `/init skill` never reaches this branch.
	if strings.ToLower(strings.TrimSpace(msg.Kind)) != "all" {
		return m, nil
	}
	platform := ""
	if m.project != nil {
		platform = strings.TrimSpace(m.project.Platform)
	}
	if !skillpack.HasScaffoldCapability(platform) {
		m.addMessage("system", "scaffold: skipped (no verified recipe)", "")
		return m, nil
	}
	label := strings.TrimSpace(msg.Kind)
	if m.project != nil && strings.TrimSpace(m.project.Name) != "" {
		label = strings.TrimSpace(m.project.Name)
	}
	m.scaffoldBusy = true
	m.scaffoldPhase = ""
	m.scaffoldProgressSeq = 0
	m.scaffoldProgressInFlight = false
	m.addMessage("system", fmt.Sprintf("Starting AI Scaffold turn for %s…", label), "")
	// CA-916: the blocking dispatch POST is unchanged; /scaffold/progress polling
	// piggybacks on the thinking ticker (which runs while scaffoldBusy) so the AI
	// turn's output streams into the chat like a normal run.
	return m, m.cmdDispatchScaffold()
}

// EngineScaffoldMsg is the result of the auto-triggered AI Scaffold Turn that
// follows a successful `/init` on a scaffold-capable platform.
type EngineScaffoldMsg struct {
	ProjectID string
	Result    *client.ScaffoldResult
	Err       error
}

// cmdDispatchScaffold sends POST /client/projects/{id}/scaffold in the
// background; the result arrives as EngineScaffoldMsg and is rendered by
// handleEngineScaffoldMsg.
func (m *AppModel) cmdDispatchScaffold() tea.Cmd {
	if m.project == nil || strings.TrimSpace(m.project.ID) == "" {
		return nil
	}
	projectID := strings.TrimSpace(m.project.ID)
	platform := strings.TrimSpace(m.project.Platform)
	cwd := strings.TrimSpace(m.boundProjectPath())
	if cwd == "" {
		cwd = strings.TrimSpace(m.cfg.ProjectPath)
	}
	if cwd == "" {
		cwd = strings.TrimSpace(m.project.Path)
	}
	cl := m.client
	provider := strings.TrimSpace(m.provider)
	model := strings.TrimSpace(m.model)
	return func() tea.Msg {
		ctx := context.Background()
		res, err := cl.DispatchScaffold(ctx, projectID, cwd, platform, provider, model)
		return EngineScaffoldMsg{ProjectID: projectID, Result: res, Err: err}
	}
}

// handleEngineScaffoldMsg renders the scaffold outcome in the chat timeline.
func (m *AppModel) handleEngineScaffoldMsg(msg EngineScaffoldMsg) (tea.Model, tea.Cmd) {
	m.scaffoldBusy = false
	m.scaffoldPhase = ""
	if msg.Err != nil {
		m.addMessage("system", fmt.Sprintf("scaffold: failed: %v", msg.Err), "error")
		return m, nil
	}
	if msg.Result == nil {
		m.addMessage("system", "scaffold: empty response.", "error")
		return m, nil
	}

	text := strings.TrimSpace(msg.Result.Message)
	if text == "" {
		text = fmt.Sprintf("scaffold: %s", msg.Result.Status)
	}
	style := ""
	switch msg.Result.Status {
	case "error":
		style = "error"
	case "done":
		if msg.Result.CompilerGate != nil {
			verdict := "FAIL"
			if msg.Result.CompilerGate.Passed {
				verdict = "PASS"
			}
			text += fmt.Sprintf(" — Compiler Gate: %s (exit %d)", verdict, msg.Result.CompilerGate.ExitCode)
		}
	}
	m.addMessage("system", text, style)
	// CA-916: one final progress fetch flushes events that landed between the
	// last poll's `after` cursor and the POST result; the ticker has stopped by
	// now so nothing else will fetch again. Always issue — the seq cursor dedupes
	// against any poll still in flight.
	m.scaffoldProgressInFlight = true
	return m, m.cmdScaffoldProgress()
}

// EngineScaffoldProgressMsg carries one poll of the CA-916 scaffold progress
// feed; output events render as a chat-style assistant stream.
type EngineScaffoldProgressMsg struct {
	Snapshot *client.ScaffoldProgressSnapshot
	Err      error
}

// cmdScaffoldProgress fetches the scaffold feed after the rendered cursor. The
// thinking ticker (90ms, live while scaffoldBusy) re-issues it every ~720ms.
func (m *AppModel) cmdScaffoldProgress() tea.Cmd {
	if m.project == nil || strings.TrimSpace(m.project.ID) == "" {
		return nil
	}
	projectID := strings.TrimSpace(m.project.ID)
	after := m.scaffoldProgressSeq
	cl := m.client
	return func() tea.Msg {
		snap, err := cl.ScaffoldProgress(context.Background(), projectID, after)
		return EngineScaffoldProgressMsg{Snapshot: snap, Err: err}
	}
}

// handleEngineScaffoldProgressMsg renders scaffold feed events like a chat run:
// output deltas append to the assistant stream, phase milestones update the
// busy-line label. Polling resumes via the thinking ticker, not here.
func (m *AppModel) handleEngineScaffoldProgressMsg(msg EngineScaffoldProgressMsg) (tea.Model, tea.Cmd) {
	m.scaffoldProgressInFlight = false
	if msg.Err != nil || msg.Snapshot == nil {
		return m, nil
	}
	snap := msg.Snapshot
	for _, ev := range snap.Events {
		// Overlapping fetches (ticker poll + final flush) can return the same
		// events — the seq cursor is the dedupe guard so output never doubles.
		if ev.Seq <= m.scaffoldProgressSeq {
			continue
		}
		m.scaffoldProgressSeq = ev.Seq
		switch ev.Kind {
		case "output":
			m.appendAssistantDelta(ev.Text)
		case "phase":
			if ev.Phase != "" {
				m.scaffoldPhase = ev.Phase
			}
		}
	}
	return m, nil
}
