package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// Live turn streaming (Desktop consumeStream parity): previously cmdSendTurn
// buffered every SSE event until turn_completed, so long workflow runs looked hung
// on "Starting workflow run…".

type turnStreamOpenedMsg struct {
	EvCh  <-chan client.ProviderEvent
	ErrCh <-chan error
}

type turnStreamEventMsg struct {
	Ev client.ProviderEvent
}

type turnStreamClosedMsg struct {
	Err error
}

type turnStreamState struct {
	evCh  <-chan client.ProviderEvent
	errCh <-chan error
}

// Orchestration stream (Desktop startOrchestrationStream): continues after the
// user turn so hub/child agent_graph_updated and late gate events still apply.
type orchStreamOpenedMsg struct {
	EvCh   <-chan client.ProviderEvent
	Cancel context.CancelFunc
}

type orchStreamEventMsg struct {
	Ev client.ProviderEvent
}

type orchStreamClosedMsg struct{}

type orchStreamState struct {
	evCh   <-chan client.ProviderEvent
	cancel context.CancelFunc
}

func (m *AppModel) stopOrchestrationStream() {
	if m.orchStream == nil {
		return
	}
	if m.orchStream.cancel != nil {
		m.orchStream.cancel()
	}
	m.orchStream = nil
}

func (m *AppModel) cmdPollTurnStream() tea.Cmd {
	st := m.turnStream
	if st == nil || st.evCh == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-st.evCh
		if !ok {
			var err error
			if st.errCh != nil {
				err = <-st.errCh
			}
			return turnStreamClosedMsg{Err: err}
		}
		return turnStreamEventMsg{Ev: ev}
	}
}

func (m *AppModel) cmdStartOrchestrationStream() tea.Cmd {
	if m.runHandle == nil || m.orchStream != nil {
		return nil
	}
	runID := m.runHandle.RunID
	after := m.lastEventSeq
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := cl.StreamLive(ctx, runID, after)
		return orchStreamOpenedMsg{EvCh: ch, Cancel: cancel}
	}
}

func (m *AppModel) cmdPollOrchStream() tea.Cmd {
	st := m.orchStream
	if st == nil || st.evCh == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-st.evCh
		if !ok {
			return orchStreamClosedMsg{}
		}
		return orchStreamEventMsg{Ev: ev}
	}
}

// openTurnStream posts the turn and returns channels for live polling (does not buffer).
func (m *AppModel) openTurnStream(prompt string) tea.Cmd {
	if m.runHandle == nil {
		return nil
	}
	// A new user turn owns the SSE filter; stop prior orchestration listener.
	m.stopOrchestrationStream()
	runID := m.runHandle.RunID
	// Resolve the step id the same way Desktop does so a resumed/opened flow run
	// never POSTs an empty stepId (startTurn rejects that with 400, CA-519).
	stepID := m.resolveTurnStepID()
	// Raise the client per-run SSE cursor to the last event the model has seen
	// so the new turn streams from here instead of replaying the prior turn
	// (CA-520). m.lastEventSeq may have advanced past the /open snapshot via
	// the orchestration stream.
	m.client.NoteLastSeq(runID, m.lastEventSeq)
	cl := m.client
	yolo := m.effectiveYolo()
	model := m.model
	reasoningEffort := m.reasoningEffort
	skills := m.selectedSkills
	attachments := m.pendingAttach
	var (
		turnSubMode    string
		turnFlowRef    string
		turnChangeType string
	)
	if m.firstTurnPending {
		if sub, fr, ct, _, ok := m.launch.FirstTurnExtras(); ok {
			turnSubMode = sub
			turnFlowRef = fr
			turnChangeType = ct
		}
		m.firstTurnPending = false
	}
	catalogWorkflow := m.launch.IsCatalogWorkflow()
	// Release local pending temp files; base64 already copied into attachments.
	for _, att := range attachments {
		m.unlinkPendingLocal(att.ID)
	}
	m.pendingAttach = nil
	m.attachPanelOpen = false

	return func() tea.Msg {
		ctx := context.Background()
		turnIn := client.TurnInput{
			RunID:           runID,
			StepID:          stepID,
			Prompt:          prompt,
			ReasoningEffort: reasoningEffort,
			SelectedSkills:  skills,
			Attachments:     attachments,
			SubMode:         turnSubMode,
			FlowRef:         turnFlowRef,
			ChangeType:      turnChangeType,
		}
		if !catalogWorkflow {
			yoloCopy := yolo
			turnIn.YoloMode = &yoloCopy
			modelCopy := model
			turnIn.Model = &modelCopy
		} else {
			turnIn.ReasoningEffort = ""
		}
		evCh, errCh := cl.SendTurn(ctx, turnIn)
		return turnStreamOpenedMsg{EvCh: evCh, ErrCh: errCh}
	}
}

func (m *AppModel) cmdSendTurn(prompt string) tea.Cmd {
	return m.openTurnStream(prompt)
}

func formatFlowStepsBanner(steps []client.WorkflowStepRuntime) string {
	if len(steps) == 0 {
		return "Flow steps: (none yet — waiting for runner)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Flow steps (%d):\n", len(steps)))
	active := ""
	for i, s := range steps {
		name := strings.TrimSpace(s.NodeID)
		if name == "" {
			name = strings.TrimSpace(s.StepType)
		}
		if name == "" {
			name = shortID(s.StepID)
		}
		mark := " "
		st := strings.ToUpper(strings.TrimSpace(s.Status))
		switch st {
		case "RUNNING", "WAITING_USER_APPROVAL":
			mark = ">"
			if active == "" {
				active = name
			}
		case "DONE":
			mark = "+"
		case "FAILED":
			mark = "x"
		case "SKIPPED", "CANCELED":
			mark = "-"
		}
		line := fmt.Sprintf("  %s %2d. [%s] %s", mark, i+1, st, name)
		if s.RetryCount > 0 {
			line += fmt.Sprintf(" (retry %d)", s.RetryCount)
		}
		sb.WriteString(line + "\n")
	}
	if active != "" {
		sb.WriteString("In progress: " + active)
	} else {
		sb.WriteString("In progress: (idle)")
	}
	return sb.String()
}
