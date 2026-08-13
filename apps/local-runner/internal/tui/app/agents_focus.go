package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

type focusStreamOpenedMsg struct {
	RunID  string
	EvCh   <-chan client.ProviderEvent
	Cancel context.CancelFunc
}

type focusStreamEventMsg struct {
	Ev client.ProviderEvent
}

type focusStreamClosedMsg struct{}

func (m *AppModel) mainRunID() string {
	if m.runHandle == nil {
		return ""
	}
	return m.runHandle.RunID
}

func (m *AppModel) viewingChild() bool {
	id := strings.TrimSpace(m.focusRunID)
	return id != "" && id != m.mainRunID()
}

func (m *AppModel) flowHasActiveAgents() bool {
	for _, r := range m.agentRuns {
		st := strings.ToLower(strings.TrimSpace(r.Status))
		switch st {
		case "running", "waiting_approval", "waiting_question", "spawned", "waiting_user_approval":
			return true
		}
	}
	return false
}

func (m *AppModel) stopFocusStream() {
	if m.focusStream == nil {
		return
	}
	if m.focusStream.cancel != nil {
		m.focusStream.cancel()
	}
	m.focusStream = nil
}

func (m *AppModel) formatAgentsChip(ascii bool) string {
	runs := orderAgentsMainFirst(m.agentRuns)
	if len(runs) == 0 {
		return ""
	}
	sep := " "
	var b strings.Builder
	b.WriteString("agents:")
	focus := strings.TrimSpace(m.focusRunID)
	mainID := m.mainRunID()
	for i, r := range runs {
		name := strings.TrimSpace(r.AgentName)
		if name == "" {
			name = shortID(r.RunID)
		}
		mark := "○"
		if ascii {
			mark = "o"
		}
		st := strings.ToLower(strings.TrimSpace(r.Status))
		if st == "running" || st == "spawned" || strings.Contains(st, "waiting") {
			mark = "●"
			if ascii {
				mark = "*"
			}
		}
		star := ""
		if focus == r.RunID || (focus == "" && r.RunID == mainID && i == 0) {
			star = "*"
		}
		if focus == "" && i == 0 && (strings.EqualFold(r.Role, "main") || strings.EqualFold(r.AgentName, "main")) {
			star = "*"
		}
		b.WriteString(sep)
		b.WriteString(name)
		b.WriteString(star)
		b.WriteString(mark)
	}
	return b.String()
}

func (m *AppModel) cmdFocusAgent(runID string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	mainID := m.mainRunID()
	if runID == "" || runID == mainID {
		m.restoreMainTranscript()
		return nil
	}
	if !m.viewingChild() {
		m.mainTranscript = append([]ChatMessage(nil), m.messages...)
	}
	m.stopFocusStream()
	m.focusRunID = runID
	m.messages = nil
	m.viewport.offset = 0
	name := m.agentNameForRun(runID)
	m.addMessage("system", fmt.Sprintf("Child transcript: %s — /agent main or Tab to return", name), "")
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := cl.StreamLive(ctx, runID, 0)
		return focusStreamOpenedMsg{RunID: runID, EvCh: ch, Cancel: cancel}
	}
}

func (m *AppModel) agentNameForRun(runID string) string {
	for _, r := range m.agentRuns {
		if r.RunID == runID {
			if n := strings.TrimSpace(r.AgentName); n != "" {
				return n
			}
			return shortID(r.RunID)
		}
	}
	if runID == "" {
		return "main"
	}
	return shortID(runID)
}

func (m *AppModel) restoreMainTranscript() {
	m.stopFocusStream()
	if m.viewingChild() && m.mainTranscript != nil {
		m.messages = append([]ChatMessage(nil), m.mainTranscript...)
	}
	m.focusRunID = ""
	m.mainTranscript = nil
	m.viewport.offset = 0
}

func (m *AppModel) cmdPollFocusStream() tea.Cmd {
	st := m.focusStream
	if st == nil || st.evCh == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-st.evCh
		if !ok {
			return focusStreamClosedMsg{}
		}
		return focusStreamEventMsg{Ev: ev}
	}
}

func (m *AppModel) handleFocusEvent(ev client.ProviderEvent) {
	switch ev.Type {
	case "message_delta", "message_completed":
		m.appendAssistantDelta(ev.Text)
	case "turn_completed":
		if ev.FinalMessage != "" && !m.hasAssistantContent() && !isStepCompleteStub(ev.FinalMessage) {
			m.ensureAssistantMessage(ev.FinalMessage)
		}
	case "tool_started":
		if ev.ToolName != "" {
			m.addMessage("tool", fmt.Sprintf("→ %s", ev.ToolName), "tool")
		}
	case "turn_failed":
		if strings.TrimSpace(ev.Error) != "" {
			m.addMessage("system", "Turn failed: "+ev.Error, "error")
		}
	}
}

func (m *AppModel) resolveAgentFocusTarget(raw string) (runID, name string, ok bool) {
	want := strings.TrimSpace(raw)
	if want == "" || strings.EqualFold(want, "main") {
		return m.mainRunID(), "main", true
	}
	runs := orderAgentsMainFirst(m.agentRuns)
	for _, r := range runs {
		if strings.EqualFold(r.RunID, want) || strings.EqualFold(r.AgentName, want) {
			return r.RunID, r.AgentName, true
		}
	}
	return "", "", false
}

func (m *AppModel) cycleFocusedAgent() (runID, name string, ok bool) {
	runs := orderAgentsMainFirst(m.agentRuns)
	if len(runs) == 0 {
		return "", "", false
	}
	cur := strings.TrimSpace(m.focusRunID)
	if cur == "" {
		cur = m.mainRunID()
	}
	idx := 0
	for i, r := range runs {
		if r.RunID == cur {
			idx = i
			break
		}
	}
	next := runs[(idx+1)%len(runs)]
	return next.RunID, next.AgentName, true
}

func (m *AppModel) childRunIDs() []string {
	mainID := m.mainRunID()
	var out []string
	for _, r := range m.agentRuns {
		if r.RunID != "" && r.RunID != mainID {
			out = append(out, r.RunID)
		}
	}
	return out
}
