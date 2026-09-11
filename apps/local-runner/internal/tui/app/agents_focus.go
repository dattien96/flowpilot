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

// focusedChildLive reports whether the focused sub-agent run is still actually
// working. cmdFocusAgent always opens a listen-only StreamLive on a child even
// when that child already finished (it is the only way to keep the transcript
// readable), so a bare focusStream must NOT count as live work — otherwise
// opening a completed sub-agent (run-101411 grok-coder) keeps the Thinking
// spinner + elapsed clock running forever.
func (m *AppModel) focusedChildLive() bool {
	if m.focusStream == nil {
		return false
	}
	id := strings.TrimSpace(m.focusRunID)
	if id == "" || id == m.mainRunID() {
		return false
	}
	for _, r := range m.agentRuns {
		if strings.TrimSpace(r.RunID) != id {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(r.Status))
		if st == "waiting_user_approval" {
			return false
		}
		return !runStatusIsTerminal(r.Status)
	}
	// Focused run not in the hydrate snapshot yet: fall back to parent flow/turn
	// liveness so the spinner does not drop mid-open.
	return m.connStatus == ConnRunning || m.flowHasActiveAgents()
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

// hasLiveWorkingChild reports a child that is truly still working, not just
// stamped WAITING_USER_APPROVAL for an escalate/cap park. BUG-231 stamp must not
// hide the [Continue]/[Stop] bar nor keep Thinking on (run-136749).
func (m *AppModel) hasLiveWorkingChild() bool {
	mainID := m.mainRunID()
	for _, r := range m.agentRuns {
		if (r.RunID != "" && r.RunID == mainID) || isMainAgentRun(r) {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(r.Status))
		switch st {
		case "running", "spawned":
			return true
		case "waiting_approval", "waiting_question":
			// YOLO/ask_user gate not yet mounted as a clickable card — keep
			// [Continue] hidden until the user can actually Approve/Deny.
			if m.approval == nil && m.question == nil && m.gate == nil {
				return true
			}
		case "waiting_user_approval":
			// Park stamp (escalate/cap/delegate_failed) — not live work.
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

// hasChildAgentRun reports whether any run in the given list is a child agent
// (not the synthetic main row).
func hasChildAgentRun(runs []client.AgentRunSummary) bool {
	for _, r := range runs {
		if r.RunID != "" && !isMainAgentRun(r) {
			return true
		}
	}
	return false
}

// adoptAgentRuns applies a fresh agent-runs snapshot without clobbering known
// children (CA-528). GET …/agents can return only the synthetic main row (or an
// empty list) while a faster agent_graph_updated already populated children;
// replacing wholesale would hide the step [open] chips until the next poll.
func (m *AppModel) adoptAgentRuns(incoming []client.AgentRunSummary) {
	if len(incoming) == 0 {
		return // never drop known agents on an empty hydrate
	}
	if !hasChildAgentRun(incoming) && m.hasChildAgentRuns() {
		return // main-only snapshot must not erase known children
	}
	m.agentRuns = incoming
	m.afterAgentRunsAdopted()
}

// afterAgentRunsAdopted keeps agent display state consistent after agentRuns
// changed: clamp the focused index and re-settle the flow chrome if the loop
// finished. (Task-311: no panel-expand — the sidebar is width-reactive.)
func (m *AppModel) afterAgentRunsAdopted() {
	if m.focusedAgentIdx >= len(m.agentRuns) {
		m.focusedAgentIdx = 0
	}
	m.settleFlowIfDone()
}

// agentHydrateRetryLimit caps the automatic hydrate re-arm ladder so a slow or
// dead runner is not flooded (CA-528; CA-514 keeps the poll pause intact).
const agentHydrateRetryLimit = 3

// stepsNeedChildOpenChip is true when a flow step that can host a child agent is
// in an [open]-eligible state but no child run is mapped to it yet — the F2 step
// list is missing its [open] button and a fresh hydrate may fix it (CA-528).
func (m *AppModel) stepsNeedChildOpenChip() bool {
	if !(m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep) {
		return false
	}
	for _, s := range m.flowSteps {
		if !stepMayHostChildAgent(s) {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(s.Status)) {
		case "RUNNING", "WAITING_USER_APPROVAL", "DONE", "FAILED":
		default:
			continue
		}
		if _, ok := m.childRunForStep(s); !ok {
			return true
		}
	}
	return false
}

// hydrateAgentRunsIfNeededMsg re-arms a one-shot agent hydrate after a short
// tick when steps still need their child [open] chip (CA-528).
type hydrateAgentRunsIfNeededMsg struct{}

// cmdHydrateAgentRunsIfNeeded returns a bounded retry tick when a flow step is
// still missing its child [open] chip and no hydrate is in flight. The in-flight
// guard plus the retry cap keep this from flooding a slow or dead runner.
func (m *AppModel) cmdHydrateAgentRunsIfNeeded() tea.Cmd {
	if !m.stepsNeedChildOpenChip() {
		return nil
	}
	if m.agentsHydrateInFlight {
		return nil
	}
	if m.agentHydrateRetries >= agentHydrateRetryLimit {
		return nil
	}
	m.agentHydrateRetries++
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return hydrateAgentRunsIfNeededMsg{}
	})
}

// (Task-311: expandSessionPanelForChildAgents removed — the sidebar is
// width-reactive, there is no collapsed panel to expand.)

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
	// Task-311: no panel expansion — the sidebar is width-reactive; [open]/[back]
	// controls are visible whenever the terminal is wide enough.
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

// agentTaskDetail is the /agents picker description for vibe-sprint children
// (BUG-369): "task 2/3 Task-911-….md". Empty when the child was not stamped.
func agentTaskDetail(r client.AgentRunSummary) string {
	if r.VibeTaskTotal <= 0 || r.VibeTaskIndex <= 0 {
		return ""
	}
	s := fmt.Sprintf("task %d/%d", r.VibeTaskIndex, r.VibeTaskTotal)
	if n := strings.TrimSpace(r.VibeTaskName); n != "" {
		s += " " + n
	}
	return s
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
// The child that is currently focused wins over any other run that also matches
// the step keys: live graph + list hydrate polls replace agentRuns and can
// reorder same-named live/historical runs, which would otherwise flip the step's
// [open]/[back] chip under a stable focus (CA-529).
//
// Matching is step-identity-first, agent-name-last (task-harness S3 BUG-XXX):
// spawnChildRun sets Label = node.ID, so a step whose child exists is pinned by
// its own node id — two nodes that share ONE catalog agent (task-harness
// plan_writer + implement both spawn from "coder", plan_reviewer + reviewer
// both from "reviewer") must NOT map both rows to the same child. The catalog
// agent name is only a fallback for unlabeled runs (e.g. rag-harness nodes
// whose id equals the agent name), and never overrides a step-identity match.
func (m *AppModel) childRunForStep(s client.WorkflowStepRuntime) (client.AgentRunSummary, bool) {
	agentRef := strings.TrimSpace(s.AgentRef)
	nodeID := strings.TrimSpace(s.NodeID)
	stepType := strings.TrimSpace(s.StepType)
	mainID := m.mainRunID()
	matches := func(r client.AgentRunSummary) bool {
		if r.RunID == "" || r.RunID == mainID || isMainAgentRun(r) {
			return false
		}
		label := strings.TrimSpace(r.Label)
		if label != "" {
			// A labeled child is pinned to its own node id/step type only.
			return strings.EqualFold(label, nodeID) || strings.EqualFold(label, stepType)
		}
		// Unlabeled child: fall back to run id / agent name against the step's
		// own node identity, then the shared catalog name as a last resort.
		if strings.EqualFold(r.RunID, nodeID) || strings.EqualFold(r.RunID, stepType) {
			return true
		}
		if strings.EqualFold(r.AgentName, nodeID) || strings.EqualFold(r.AgentName, stepType) {
			return true
		}
		return agentRef != "" && strings.EqualFold(r.AgentName, agentRef)
	}
	if m.viewingChild() {
		for _, r := range m.agentRuns {
			if strings.EqualFold(strings.TrimSpace(r.RunID), strings.TrimSpace(m.focusRunID)) && matches(r) {
				return r, true
			}
		}
	}
	for _, r := range m.agentRuns {
		if matches(r) {
			return r, true
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
		if task := agentTaskDetail(r); task != "" {
			if detail != "" {
				detail = task + " · " + detail
			} else {
				detail = task
			}
		}
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
