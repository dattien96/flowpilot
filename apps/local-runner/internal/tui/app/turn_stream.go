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
// RunID is the leg the stream was opened for — captured when the cmd was
// created. A provider/chat switch landing between issue and delivery makes the
// open stale: Update must drop+cancel it instead of letting it tear down the
// NEW leg's stream and attach the dead one (BUG-450). Empty RunID is only
// reachable from test-injected messages, which are always processed.
type orchStreamOpenedMsg struct {
	RunID  string
	EvCh   <-chan client.ProviderEvent
	Cancel context.CancelFunc
}

// st identifies the stream generation that produced the message. A poll
// in flight when its stream is replaced (provider switch, new user turn)
// still delivers one final event/close — the tag lets Update drop it instead
// of letting a dead stream's close tear down the live one (BUG-428).
// St is nil only for test-injected messages, which are always processed.
type orchStreamEventMsg struct {
	Ev client.ProviderEvent
	st *orchStreamState
}

type orchStreamClosedMsg struct {
	st *orchStreamState
}

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
		return orchStreamOpenedMsg{RunID: runID, EvCh: ch, Cancel: cancel}
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
			return orchStreamClosedMsg{st: st}
		}
		return orchStreamEventMsg{Ev: ev, st: st}
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
	posture := m.chatPosture
	skills := m.selectedSkills
	attachments := m.pendingAttach
	// Snapshot any pending Grok YOLO posture sync off the cmd goroutine so the
	// values are read/cleared on the model goroutine only (no data race with
	// later posture switches), then applied before the turn posts below.
	grokSyncSet := m.postureGrokSyncSet
	grokSyncYolo := m.postureGrokSync
	m.postureGrokSyncSet = false
	var (
		turnSubMode     string
		turnFlowRef     string
		turnChangeType  string
		turnSourceDocID string
	)
	if m.firstTurnPending {
		if sub, fr, ct, src, ok := m.launch.FirstTurnExtras(); ok {
			turnSubMode = sub
			turnFlowRef = fr
			turnChangeType = ct
			turnSourceDocID = src
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

	// Grok sync: separate cmd so a failure re-enables the flag (via
	// grokSyncFailedMsg) while the turn is still sent (via turnCmd).
	var cmds []tea.Cmd
	if grokSyncSet {
		cmds = append(cmds, func() tea.Msg {
			if err := cl.ApplyGrokYoloPosture(context.Background(), grokSyncYolo); err != nil {
				return grokSyncFailedMsg{Yolo: grokSyncYolo}
			}
			return nil
		})
	}
	cmds = append(cmds, func() tea.Msg {
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
			SourceDocID:     turnSourceDocID,
			ChatPosture:     posture,
		}
		if !catalogWorkflow {
			yoloCopy := yolo
			turnIn.YoloMode = &yoloCopy
			modelCopy := model
			turnIn.Model = &modelCopy
		} else {
			turnIn.ReasoningEffort = ""
		}
		// Observability for BUG-339 F4 / BUG-329: log the model actually sent.
		tuiLog("openTurnStream sendTurn run=%s model=%q yolo=%v posture=%s step=%s", runID, model, yolo, posture, stepID)
		evCh, errCh := cl.SendTurn(ctx, turnIn)
		return turnStreamOpenedMsg{EvCh: evCh, ErrCh: errCh}
	})
	return tea.Batch(cmds...)
}

func (m *AppModel) cmdSendTurn(prompt string) tea.Cmd {
	// CP-59 Task-315 slice 3 (SD26 §10): a detached chat (restored, no active
	// leg) reattaches on the first prompt — a fresh leg mints via startRun
	// carrying the chat identity, then the prompt sends on it.
	if m.chatDetached && m.runHandle != nil && strings.TrimSpace(m.runHandle.ChatID) != "" {
		m.pendingPrompt = prompt
		return m.cmdReattachChat()
	}
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
