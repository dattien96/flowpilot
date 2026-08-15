package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

type focusStreamOpenedMsg struct {
	RunID    string
	Messages []ChatMessage // history after ResumeRun seed (empty if live-only)
	AfterSeq int64         // stream live events after this seq
	EvCh     <-chan client.ProviderEvent
	Cancel   context.CancelFunc
	Fallback string // resume failed → live-only fallback in effect (soft note)
	Err      string // total failure (runner unreachable / child not resumable)
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

// hasChildAgentRuns is true when the agent graph has a non-main child with a run id.
func (m *AppModel) hasChildAgentRuns() bool {
	mainID := m.mainRunID()
	for _, r := range m.agentRuns {
		if r.RunID == "" || r.RunID == mainID || isMainAgentRun(r) {
			continue
		}
		return true
	}
	return false
}

// expandSessionPanelForChildAgents opens the F2 panel so step [open] is visible
// as soon as a child agent exists (live or after hydrate).
func (m *AppModel) expandSessionPanelForChildAgents() {
	if m.hasChildAgentRuns() {
		m.sessionPanel.Collapsed = false
	}
}

// stepsSuggestChildAgentOpen is true when a step that can host a child agent
// just became active/finished (or newly appeared) — trigger agent-graph hydrate.
func stepsSuggestChildAgentOpen(prev, next []client.WorkflowStepRuntime) bool {
	prevByID := make(map[string]client.WorkflowStepRuntime, len(prev))
	for _, s := range prev {
		prevByID[s.StepID] = s
	}
	for _, s := range next {
		if !stepMayHostChildAgent(s) {
			continue
		}
		st := strings.ToUpper(strings.TrimSpace(s.Status))
		switch st {
		case "RUNNING", "WAITING_USER_APPROVAL", "DONE", "FAILED":
			// ok
		default:
			continue
		}
		old, ok := prevByID[s.StepID]
		if !ok {
			return true
		}
		oldSt := strings.ToUpper(strings.TrimSpace(old.Status))
		if oldSt != st || strings.TrimSpace(old.AgentRef) != strings.TrimSpace(s.AgentRef) {
			return true
		}
	}
	return false
}

func stepMayHostChildAgent(s client.WorkflowStepRuntime) bool {
	if strings.TrimSpace(s.AgentRef) != "" {
		return true
	}
	// Common agent step types even when AgentRef is still empty on the DTO.
	st := strings.ToLower(strings.TrimSpace(s.StepType))
	switch st {
	case "agent", "coder", "reviewer", "orchestrator", "worker":
		return true
	}
	node := strings.ToLower(strings.TrimSpace(s.NodeID))
	if node == "" || node == "main" {
		return false
	}
	// Heuristic: non-context structural nodes often map to child agents.
	if strings.Contains(node, "review") || strings.Contains(node, "coder") ||
		strings.Contains(node, "agent") || strings.Contains(node, "worker") {
		return true
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

// formatAgentViewStatus is a compact status-line label: agent:main or agent:grok-coder.
// Prefix is dim; the name uses styleStatusAgent (teal), not model/YOLO accent.
// Open/back live on the F2 steps panel.
func (m *AppModel) formatAgentViewStatus() string {
	name := ""
	switch {
	case m.viewingChild():
		name = m.agentNameForRun(m.focusRunID)
		if name == "" {
			name = shortID(m.focusRunID)
		}
	case m.mode == ModeFlow || m.mode == ModeStep || len(m.agentRuns) > 0:
		name = "main"
	default:
		return ""
	}
	return styleStatus.Render("agent:") + styleStatusAgent.Render(name)
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
		m.mainHistoryLoadedAfterSeq = m.historyLoadedAfterSeq
	}
	m.stopFocusStream()
	m.focusRunID = runID
	// Keep focusedAgentIdx in sync for Tab cycle.
	for i, r := range orderAgentsMainFirst(m.agentRuns) {
		if r.RunID == runID {
			m.focusedAgentIdx = i
			break
		}
	}
	// Expand F2 panel so step [open]/[back] controls are visible.
	// No "Viewing agent:" chat spam — status agent:name + F2 highlight suffice.
	m.sessionPanel.Collapsed = false
	m.messages = nil
	m.visiblePromptCount = 0
	m.historyLoadedAfterSeq = 0
	m.viewport.offset = 0
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithCancel(context.Background())
		// Resume seeds durable transcript (turn log / Grok JSONL) into the
		// runner event buffer — StreamLive alone on a cold child returns empty
		// (run-98158 grok-coder after parent /open). CA-504.
		handle, resumeErr := resumeChildRunForFocus(cl, runID)
		if resumeErr == nil {
			until := handle.LastEventSeq
			var collected []client.ProviderEvent
			if until > 0 {
				collected = collectReplayEvents(ctx, cl, runID, 0, until, chatReplayMaxEvents)
			}
			trimmed := trimEventsFromTurnStart(collected)
			msgs := replayChildHistoryMessages(trimmed)
			ch := cl.StreamLive(ctx, runID, until)
			return focusStreamOpenedMsg{
				RunID: runID, Messages: msgs, AfterSeq: until,
				EvCh: ch, Cancel: cancel,
			}
		}
		// Resume failed. Distinguish dial-level failure (runner genuinely
		// unreachable — the stream would fail too) from a server-side error
		// (child cold / session unavailable — the live stream may still serve an
		// in-memory child). Dial failure is a total failure: Err → restore main.
		if runnerDialDeadErr(resumeErr.Error()) {
			cancel()
			return focusStreamOpenedMsg{RunID: runID, Err: resumeErr.Error(), Cancel: cancel}
		}
		// Server-side resume failure: do not hard-fail the child view, fall back
		// to the pre-CA-504 live-only stream so an in-memory child still renders
		// its transcript (run-193749). StreamLive never errors — a not-found run
		// just closes the channel, so the handler shows the fallback note
		// instead of a red error and a stuck empty chrome.
		ch := cl.StreamLive(ctx, runID, 0)
		return focusStreamOpenedMsg{
			RunID: runID, EvCh: ch, Cancel: cancel,
			Fallback: fmt.Sprintf("child resume failed (%s); showing live events", resumeErr.Error()),
		}
	}
}

// resumeChildRunForFocus POSTs /resume for a child transcript, retrying once
// when the runner looked momentarily unreachable (supervisor restart / port
// handoff) so a transient dial failure does not drop the seed (CA-517).
func resumeChildRunForFocus(cl *client.Client, runID string) (client.RunHandle, error) {
	var handle client.RunHandle
	var err error
	for attempt := 0; attempt <= 1; attempt++ {
		resumeCtx, resumeCancel := context.WithTimeout(context.Background(), 45*time.Second)
		handle, err = cl.ResumeRun(resumeCtx, runID)
		resumeCancel()
		if err == nil {
			return handle, nil
		}
		if attempt == 0 && runnerDialDeadErr(err.Error()) {
			select {
			case <-time.After(300 * time.Millisecond):
			}
			continue
		}
		break
	}
	return handle, err
}

func (m *AppModel) agentNameForRun(runID string) string {
	for _, r := range m.agentRuns {
		if r.RunID == runID {
			return agentDisplayName(r)
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
		m.historyLoadedAfterSeq = m.mainHistoryLoadedAfterSeq
		m.visiblePromptCount = 0
		m.syncVisiblePromptCount()
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
	case "turn_started":
		if p := childFacingUserPrompt(ev.Prompt); p != "" {
			m.addMessage("user", p, "")
		}
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

// replayChildHistoryMessages rebuilds a child agent transcript for /agent focus.
// Spawn-composed prompts are reduced to the human task; system frames are skipped.
func replayChildHistoryMessages(evs []client.ProviderEvent) []ChatMessage {
	var out []ChatMessage
	for _, ev := range evs {
		switch ev.Type {
		case "turn_started":
			if p := childFacingUserPrompt(ev.Prompt); p != "" {
				out = append(out, ChatMessage{Role: "user", Content: p})
			}
		case "message_delta":
			if ev.Text == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1].Role == "assistant" {
				out[len(out)-1].Content += ev.Text
			} else {
				out = append(out, ChatMessage{Role: "assistant", Content: ev.Text})
			}
		case "message_completed":
			out = applyReplayAssistantText(out, ev.Text)
		case "turn_completed":
			out = applyReplayAssistantText(out, ev.FinalMessage)
		case "tool_started":
			if ev.ToolName != "" {
				out = append(out, ChatMessage{Role: "tool", Content: "→ " + ev.ToolName, FormatHint: "tool"})
			}
		}
	}
	return out
}

// childFacingUserPrompt returns a short user-facing task from a child spawn
// prompt, or "" when the frame is pure orchestration / system.
func childFacingUserPrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return ""
	}
	// Full agent composition: keep the trailing human task after identity/context.
	if strings.Contains(p, "[FlowPilot sub-agent") ||
		strings.Contains(p, "You are the implementation agent") ||
		strings.Contains(p, "You are the review agent") ||
		strings.Contains(p, "[flow-engine]") {
		// Prefer text after the last FCP / context marker.
		for _, sep := range []string{
			"[Context: use sections above as feature truth. Stay in scope.]\n\n",
			"[Context: use sections above as feature truth. Stay in scope.]\n",
			"]\n\n",
		} {
			if i := strings.LastIndex(p, sep); i >= 0 {
				rest := strings.TrimSpace(p[i+len(sep):])
				// Drop write-contract appendices for display.
				if j := strings.Index(rest, "\n\n## Required file outputs"); j >= 0 {
					rest = strings.TrimSpace(rest[:j])
				}
				if rest != "" && len([]rune(rest)) < 2000 {
					return rest
				}
			}
		}
		// Fallback: last non-empty short paragraph.
		parts := strings.Split(p, "\n")
		for i := len(parts) - 1; i >= 0; i-- {
			line := strings.TrimSpace(parts[i])
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") ||
				strings.HasPrefix(line, "Before you finish") || strings.HasPrefix(line, "For `") {
				continue
			}
			if len([]rune(line)) > 8 && len([]rune(line)) < 400 {
				return line
			}
		}
		return ""
	}
	return p
}

func agentDisplayName(r client.AgentRunSummary) string {
	if n := strings.TrimSpace(r.Label); n != "" {
		return n
	}
	if n := strings.TrimSpace(r.AgentName); n != "" {
		return n
	}
	return shortID(r.RunID)
}

func (m *AppModel) resolveAgentFocusTarget(raw string) (runID, name string, ok bool) {
	want := strings.TrimSpace(raw)
	if want == "" || strings.EqualFold(want, "main") {
		return m.mainRunID(), "main", true
	}
	runs := orderAgentsMainFirst(m.agentRuns)
	for _, r := range runs {
		if strings.EqualFold(r.RunID, want) ||
			strings.EqualFold(r.AgentName, want) ||
			strings.EqualFold(r.Label, want) {
			return r.RunID, agentDisplayName(r), true
		}
	}
	return "", "", false
}

// agentRunsHydratedMsg is GET …/workflow-runs/{id}/agents after /open.
type agentRunsHydratedMsg struct {
	ParentRunID string
	Runs        []client.AgentRunSummary
	Err         string
}

// AgentGraphHydratedMsg carries a one-shot GET /agent-graph result used to seed
// loop state (done/blocked/running) on /open of a flow run (BUG-231 parity with
// Desktop refreshAgentGraph on history open).
type AgentGraphHydratedMsg struct {
	ParentRunID string
	Graph       *client.AgentGraphSnapshot
	Err         string
}

// cmdHydrateAgentGraph is a one-shot graph fetch on flow open so the TUI can
// show the awaiting-user banner / settle chrome without waiting for a live
// agent_graph_updated event. Fires even on completed/blocked opens.
func (m *AppModel) cmdHydrateAgentGraph(parentRunID string) tea.Cmd {
	parentRunID = strings.TrimSpace(parentRunID)
	if parentRunID == "" {
		return nil
	}
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		g, err := cl.GetAgentGraph(ctx, parentRunID)
		if err != nil {
			return AgentGraphHydratedMsg{ParentRunID: parentRunID, Err: err.Error()}
		}
		return AgentGraphHydratedMsg{ParentRunID: parentRunID, Graph: g}
	}
}

func (m *AppModel) cmdHydrateAgentRuns(parentRunID string) tea.Cmd {
	parentRunID = strings.TrimSpace(parentRunID)
	if parentRunID == "" {
		return nil
	}
	if m.agentsHydrateInFlight {
		return nil
	}
	// Auto-poll path pauses when runner is dead; one-shot hydrate from /open still
	// runs when streak is high only if caller clears streak first (ChatOpenedMsg).
	m.agentsHydrateInFlight = true
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		runs, err := cl.ListAgentRuns(ctx, parentRunID)
		if err != nil {
			return agentRunsHydratedMsg{ParentRunID: parentRunID, Err: err.Error()}
		}
		// Ensure main is present for Tab cycle / picker.
		hasMain := false
		for _, r := range runs {
			if strings.EqualFold(r.Role, "main") || strings.EqualFold(r.AgentName, "main") || r.RunID == parentRunID {
				hasMain = true
				break
			}
		}
		if !hasMain {
			runs = append([]client.AgentRunSummary{{
				RunID: parentRunID, AgentName: "main", Label: "main", Role: "main", Status: "completed",
			}}, runs...)
		}
		return agentRunsHydratedMsg{ParentRunID: parentRunID, Runs: runs}
	}
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

func isMainAgentRun(r client.AgentRunSummary) bool {
	return strings.EqualFold(r.Role, "main") || strings.EqualFold(r.AgentName, "main")
}

// childRunForStep maps a flow step to a spawned child agent (never main).
func (m *AppModel) childRunForStep(s client.WorkflowStepRuntime) (client.AgentRunSummary, bool) {
	keys := []string{
		strings.TrimSpace(s.AgentRef),
		strings.TrimSpace(s.NodeID),
		strings.TrimSpace(s.StepType),
	}
	mainID := m.mainRunID()
	for _, r := range m.agentRuns {
		if r.RunID == "" || r.RunID == mainID || isMainAgentRun(r) {
			continue
		}
		for _, k := range keys {
			if k == "" {
				continue
			}
			if strings.EqualFold(r.RunID, k) ||
				strings.EqualFold(r.AgentName, k) ||
				strings.EqualFold(r.Label, k) {
				return r, true
			}
		}
	}
	return client.AgentRunSummary{}, false
}

func parseAgentPicker(input string) (query string, ok bool, argSlot bool) {
	s := strings.TrimLeft(input, " \t")
	for _, cmd := range []string{"/agents", "/agent"} {
		if len(s) < len(cmd) || !strings.EqualFold(s[:len(cmd)], cmd) {
			continue
		}
		rest := s[len(cmd):]
		if rest == "" {
			return "", true, false
		}
		if rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		return strings.TrimSpace(rest), true, true
	}
	return "", false, false
}

func filterAgentSuggestions(input string, runs []client.AgentRunSummary) []suggestItem {
	query, ok, _ := parseAgentPicker(input)
	if !ok {
		return nil
	}
	ordered := orderAgentsMainFirst(runs)
	q := strings.ToLower(strings.TrimSpace(query))
	var out []suggestItem
	for _, r := range ordered {
		name := agentDisplayName(r)
		label := strings.TrimSpace(r.Label)
		agent := strings.TrimSpace(r.AgentName)
		if q != "" &&
			!strings.Contains(strings.ToLower(name), q) &&
			!strings.Contains(strings.ToLower(label), q) &&
			!strings.Contains(strings.ToLower(agent), q) &&
			!strings.Contains(strings.ToLower(r.RunID), q) {
			continue
		}
		detail := strings.TrimSpace(r.Status)
		if agent != "" && !strings.EqualFold(agent, name) {
			if detail != "" {
				detail += " · "
			}
			detail += agent
		}
		if r.RunID != "" {
			if detail != "" {
				detail += " · "
			}
			detail += shortID(r.RunID)
		}
		out = append(out, suggestItem{value: name, detail: detail, kind: "agent"})
	}
	return out
}
