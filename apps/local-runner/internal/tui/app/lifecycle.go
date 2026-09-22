// CP-81 Task-417: the TUI as a lifecycle participant instead of a runner
// owner. Register → heartbeat → release-on-close; ordinary quit never posts
// /system/shutdown — only the explicit "Turn off FlowPilot" choice does, and
// it goes through the fenced two-phase contract. Planned restarts reconnect;
// unplanned runner loss closes the TUI with a notice.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// ── state ───────────────────────────────────────────────────────────────────

// tuiLease is this process's generation-scoped runner lease (SD-28 D-2).
// Token is kept only in memory, never logged.
type tuiLease struct {
	LeaseID          string
	Token            string
	RunnerInstanceID string
	Generation       int
	HeartbeatEvery   time.Duration
	TTL              time.Duration
	Registered       bool
}

// reconnectState is active while the runner performs a planned restart:
// ordinary work is paused and /health is polled until a NEW runnerInstanceId
// appears or Deadline passes.
type reconnectState struct {
	RestartID string
	Deadline  time.Time
}

// CloseChoice enumerates the three-way close dialog (SD-28 §6.3).
type CloseChoice int

const (
	CloseCancel CloseChoice = iota
	CloseThisClientOnly
	CloseTurnOffFlowPilot
)

// closeDialogState is the modal shown when quitting while other clients or
// protected work exist.
type closeDialogState struct {
	Cursor   int
	Snapshot client.LifecycleSnapshot
}

// closeDialogChoices is the rendered order — cursor index maps to it.
var closeDialogChoices = []struct {
	Label  string
	Choice CloseChoice
}{
	{"Cancel", CloseCancel},
	{"Close TUI only", CloseThisClientOnly},
	{"Turn off FlowPilot", CloseTurnOffFlowPilot},
}

// ── messages (Task-417 §6) ───────────────────────────────────────────────────

// LifecycleSnapshotMsg carries the freshest lifecycle snapshot (heartbeat or
// explicit fetch).
type LifecycleSnapshotMsg struct{ Snapshot client.LifecycleSnapshot }

// LifecycleHeartbeatTickMsg fires the heartbeat command.
type LifecycleHeartbeatTickMsg struct{}

// LifecycleLostMsg ends the TUI — Planned marks a restart that never came
// back (deadline) versus an unplanned loss.
type LifecycleLostMsg struct {
	Reason  string
	Planned bool
}

// ExitIntentMsg starts the close flow; Source is "slash"|"ctrlc"|"load".
type ExitIntentMsg struct{ Source string }

// CloseChoiceMsg delivers the dialog selection.
type CloseChoiceMsg struct{ Choice CloseChoice }

// leaseRegisteredMsg stores the issued lease and starts heartbeats.
type leaseRegisteredMsg struct {
	res client.RegisterClientResponse
	err error
}

// exitDecisionMsg carries the snapshot evaluated for an exit intent — sent
// only when the close dialog must open (quit paths return QuitMsg inline).
type exitDecisionMsg struct {
	snap client.LifecycleSnapshot
}

// reconnectPollMsg is the planned-restart /health poll result.
type reconnectPollMsg struct {
	instanceID string
	alive      bool
}

// heartbeatErrMsg distinguishes typed lifecycle failures from transport loss.
type heartbeatErrMsg struct {
	err error
}

// ── registration + heartbeat ─────────────────────────────────────────────────

// clientInstanceID is stable for this TUI process (idempotent re-register).
func (m *AppModel) clientInstanceID() string {
	if m.cfg.ClientInstanceID != "" {
		return m.cfg.ClientInstanceID
	}
	return fmt.Sprintf("tui-%d", os.Getpid())
}

// cmdRegisterLease registers this TUI's lease against the runner (D-2). It
// runs after runner attach (ConnectedMsg) and on reconnect.
func (m *AppModel) cmdRegisterLease() tea.Cmd {
	runnerURL := m.runnerURL
	clientID := m.clientInstanceID()
	projectPath := m.cfg.ProjectPath
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		res, err := cl.RegisterLifecycleClient(ctx, client.RegisterClientRequest{
			Kind:             "tui",
			ClientInstanceID: clientID,
			PID:              os.Getpid(),
			Label:            fmt.Sprintf("TUI %d", os.Getpid()),
			ProjectPath:      projectPath,
		})
		return leaseRegisteredMsg{res: res, err: err}
	}
}

func (m *AppModel) applyLeaseRegistered(msg leaseRegisteredMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Runner predates lifecycle (404) or refused — stay attached without a
		// lease; ordinary close then just quits (no shutdown).
		tuiLog("lifecycle register failed (unmanaged runner?): %v", msg.err)
		return m, nil
	}
	m.lease = tuiLease{
		LeaseID:          msg.res.LeaseID,
		Token:            msg.res.LeaseToken,
		RunnerInstanceID: msg.res.RunnerInstanceID,
		Generation:       msg.res.Generation,
		HeartbeatEvery:   time.Duration(msg.res.HeartbeatIntervalMs) * time.Millisecond,
		TTL:              time.Duration(msg.res.TTLMs) * time.Millisecond,
		Registered:       true,
	}
	if m.lease.HeartbeatEvery <= 0 {
		m.lease.HeartbeatEvery = 5 * time.Second
	}
	m.applyLifecycleSnapshot(msg.res.Snapshot)
	return m, tea.Tick(m.lease.HeartbeatEvery, func(time.Time) tea.Msg {
		return LifecycleHeartbeatTickMsg{}
	})
}

// cmdHeartbeat refreshes the lease and surfaces the post-eval snapshot.
func (m *AppModel) cmdHeartbeat() tea.Cmd {
	if !m.lease.Registered || m.reconnect != nil {
		return nil
	}
	runnerURL := m.runnerURL
	lease := m.lease
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		snap, err := cl.HeartbeatLifecycleClient(ctx, lease.LeaseID, client.LeaseMutation{
			LeaseToken:       lease.Token,
			RunnerInstanceID: lease.RunnerInstanceID,
			Generation:       lease.Generation,
		})
		if err != nil {
			return heartbeatErrMsg{err: err}
		}
		return LifecycleSnapshotMsg{Snapshot: snap}
	}
}

// applyHeartbeatErr decides between silent re-register (stale lease), planned
// reconnect (draining_restart), and unplanned loss.
func (m *AppModel) applyHeartbeatErr(msg heartbeatErrMsg) (tea.Model, tea.Cmd) {
	var ae *client.APIError
	if errors.As(msg.err, &ae) && (ae.Code == "lease_unknown" || ae.Code == "stale_generation" || ae.Code == "invalid_lease_token") {
		// Lease expired or generation rotated — re-register once to rejoin.
		m.lease.Registered = false
		return m, m.cmdRegisterLease()
	}
	if m.lifecyclePhase == "draining_restart" {
		return m.enterReconnect(m.reconnectDeadline())
	}
	return m.runnerLost("runner unreachable")
}

// applyLifecycleSnapshot folds a snapshot into status + reconnect detection.
func (m *AppModel) applyLifecycleSnapshot(snap client.LifecycleSnapshot) {
	m.lifecyclePhase = snap.Phase
	m.lifecycleClients = len(snap.Clients)
	m.lifecycleWork = snap.Workload.ActiveCount()
	m.updatePending = snap.UpdatePending
	if snap.Phase == "draining_restart" && snap.Restart != nil && m.reconnect == nil {
		m.reconnect = &reconnectState{RestartID: snap.Restart.RestartID, Deadline: snap.Restart.Deadline}
	}
}

// ── reconnect (planned restart) ──────────────────────────────────────────────

func (m *AppModel) reconnectDeadline() time.Time {
	if m.reconnect != nil && !m.reconnect.Deadline.IsZero() {
		return m.reconnect.Deadline
	}
	return time.Now().Add(60 * time.Second)
}

// enterReconnect marks the model as waiting for the restarted runner and
// starts the /health poll.
func (m *AppModel) enterReconnect(deadline time.Time) (tea.Model, tea.Cmd) {
	if m.reconnect == nil {
		m.reconnect = &reconnectState{Deadline: deadline}
	} else {
		m.reconnect.Deadline = deadline
	}
	m.statusMsg = "runner restarting…"
	return m, m.cmdReconnectPoll()
}

// cmdReconnectPoll probes /health until a new runnerInstanceId appears or the
// reconnect deadline expires.
func (m *AppModel) cmdReconnectPoll() tea.Cmd {
	runnerURL := m.runnerURL
	oldInstance := m.lease.RunnerInstanceID
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h, err := cl.Health(ctx)
		if err != nil {
			return reconnectPollMsg{alive: false}
		}
		return reconnectPollMsg{instanceID: h.RunnerInstanceID, alive: h.RunnerInstanceID != "" && h.RunnerInstanceID != oldInstance}
	}
}

func (m *AppModel) applyReconnectPoll(msg reconnectPollMsg) (tea.Model, tea.Cmd) {
	if m.reconnect == nil {
		return m, nil
	}
	if time.Now().After(m.reconnect.Deadline) {
		return m.runnerLost("runner restart exceeded reconnect deadline")
	}
	if msg.alive {
		// New instance is up: re-register (new generation) and reattach the
		// session — runs/artifacts are durable runner-side.
		m.reconnect = nil
		m.lease.Registered = false
		return m, tea.Batch(m.cmdRegisterLease(), m.cmdConnect())
	}
	return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return reconnectPollMsg{instanceID: "", alive: false}
	})
}

// runnerLost handles unplanned runner death (or restart deadline expiry):
// clear notice, then quit.
func (m *AppModel) runnerLost(reason string) (tea.Model, tea.Cmd) {
	m.addMessage("system", "Runner stopped unexpectedly; TUI will close.", "error")
	if reason != "" {
		m.statusMsg = reason
	}
	m.quitting = true
	return m, func() tea.Msg { return QuitMsg{} }
}

// ── exit intent + close dialog ───────────────────────────────────────────────

// cmdExitIntent evaluates the lifecycle snapshot to decide between a silent
// release+quit (sole client, no work) and the three-choice dialog. The
// release happens inline so a plain quit still yields a QuitMsg — the
// pre-CP-81 contract tests pin that shape.
func (m *AppModel) cmdExitIntent(source string) tea.Cmd {
	runnerURL := m.runnerURL
	lease := m.lease
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		snap, err := cl.GetLifecycleSnapshot(ctx)
		if err != nil {
			// Unmanaged/legacy runner or already gone — release best-effort
			// (no-op when unmanaged) then quit; never a global shutdown.
			releaseLease(ctx, cl, lease)
			return QuitMsg{}
		}
		otherClients := 0
		for _, c := range snap.Clients {
			if c.LeaseID != lease.LeaseID {
				otherClients++
			}
		}
		if otherClients == 0 && snap.Workload.ActiveCount() == 0 {
			releaseLease(ctx, cl, lease)
			return QuitMsg{}
		}
		return exitDecisionMsg{snap: snap}
	}
}

// releaseLease is the cmd-context release helper (shared by the exit-intent
// inline path and cmdReleaseAndQuit).
func releaseLease(ctx context.Context, cl *client.Client, lease tuiLease) {
	if !lease.Registered {
		return
	}
	_, _ = cl.ReleaseLifecycleClient(ctx, lease.LeaseID, client.LeaseMutation{
		LeaseToken:       lease.Token,
		RunnerInstanceID: lease.RunnerInstanceID,
		Generation:       lease.Generation,
	})
}

func (m *AppModel) applyExitDecision(msg exitDecisionMsg) (tea.Model, tea.Cmd) {
	// The quit intent pre-set m.quitting to blank the view; reopen it so the
	// dialog's system card is visible while the user decides.
	m.quitting = false
	m.closeDialog = &closeDialogState{Snapshot: msg.snap}
	// Render as a system card (repo convention for prompts like delete-
	// pending); keys are intercepted by handleCloseDialogKey while open.
	m.addMessage("system", m.renderCloseDialog(), "gate")
	return m, nil
}

// cmdReleaseAndQuit releases this lease (idempotent server-side) then quits —
// the ordinary close path that never touches global shutdown.
func (m *AppModel) cmdReleaseAndQuit() tea.Cmd {
	runnerURL := m.runnerURL
	lease := m.lease
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		releaseLease(ctx, cl, lease)
		return QuitMsg{}
	}
}

// handleCloseDialogKey routes keys while the three-choice dialog is open:
// arrows+enter navigate, c/t/esc are shortcuts (Q-1 copy: "Close TUI only").
func (m *AppModel) handleCloseDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.closeDialog
	switch msg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		m.closeDialog = nil
		m.quitting = false
		m.addMessage("system", "Close cancelled.", "")
		return m, nil
	case tea.KeyUp:
		if d.Cursor > 0 {
			d.Cursor--
		}
		return m, nil
	case tea.KeyDown:
		if d.Cursor < len(closeDialogChoices)-1 {
			d.Cursor++
		}
		return m, nil
	case tea.KeyEnter:
		return m.applyCloseChoice(closeDialogChoices[d.Cursor].Choice)
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'c', 'C':
				return m.applyCloseChoice(CloseThisClientOnly)
			case 't', 'T':
				return m.applyCloseChoice(CloseTurnOffFlowPilot)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) applyCloseChoice(choice CloseChoice) (tea.Model, tea.Cmd) {
	m.closeDialog = nil
	switch choice {
	case CloseThisClientOnly:
		m.quitting = true
		return m, m.cmdReleaseAndQuit()
	case CloseTurnOffFlowPilot:
		m.quitting = true
		return m, m.cmdShutdownAndQuit()
	default:
		m.quitting = false
		m.addMessage("system", "Close cancelled.", "")
		return m, nil
	}
}

// releaseLeaseSync best-effort releases the lease outside the event loop —
// the Run() epilogue for quit paths that never reach cmdReleaseAndQuit
// (terminal close, headless exit, panic-recovery). Server-side release is
// idempotent; TTL expiry covers SIGKILL.
func (m *AppModel) releaseLeaseSync() {
	if !m.lease.Registered {
		return
	}
	cl := client.New(m.runnerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, _ = cl.ReleaseLifecycleClient(ctx, m.lease.LeaseID, client.LeaseMutation{
		LeaseToken:       m.lease.Token,
		RunnerInstanceID: m.lease.RunnerInstanceID,
		Generation:       m.lease.Generation,
	})
	m.lease.Registered = false
}

// registerLeaseSync is the headless (--print) counterpart of cmdRegisterLease:
// a lease keeps the client-managed runner out of idle_grace between turns.
func (m *AppModel) registerLeaseSync() {
	cl := client.New(m.runnerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	res, err := cl.RegisterLifecycleClient(ctx, client.RegisterClientRequest{
		Kind:             "tui",
		ClientInstanceID: m.clientInstanceID(),
		PID:              os.Getpid(),
		Label:            fmt.Sprintf("TUI %d", os.Getpid()),
		ProjectPath:      m.cfg.ProjectPath,
	})
	if err != nil {
		return
	}
	m.lease = tuiLease{
		LeaseID:          res.LeaseID,
		Token:            res.LeaseToken,
		RunnerInstanceID: res.RunnerInstanceID,
		Generation:       res.Generation,
		HeartbeatEvery:   time.Duration(res.HeartbeatIntervalMs) * time.Millisecond,
		TTL:              time.Duration(res.TTLMs) * time.Millisecond,
		Registered:       true,
	}
}

// ── status chip (T-5) ────────────────────────────────────────────────────────

// lifecycleStatusChip renders the compact lifecycle fragment for the status
// line: shared clients, idle countdown, update pending, restart banner.
func (m *AppModel) lifecycleStatusChip() string {
	if m.reconnect != nil || m.lifecyclePhase == "draining_restart" {
		return "runner restarting…"
	}
	var chip string
	if m.lifecycleClients > 1 {
		chip = fmt.Sprintf("shared:%d", m.lifecycleClients)
	}
	if m.updatePending {
		if chip != "" {
			chip += " "
		}
		chip += "update pending"
	}
	return chip
}

// renderCloseDialog draws the compact three-choice close prompt.
func (m *AppModel) renderCloseDialog() string {
	d := m.closeDialog
	lines := []string{"Runner is shared or busy:"}
	for _, c := range d.Snapshot.Clients {
		if c.LeaseID != m.lease.LeaseID {
			lines = append(lines, fmt.Sprintf("  client: %s", c.Label))
		}
	}
	for _, it := range d.Snapshot.Workload.Items {
		lines = append(lines, fmt.Sprintf("  work: %s %s", it.Kind, it.RunID))
	}
	for i, ch := range closeDialogChoices {
		cursor := "  "
		if i == d.Cursor {
			cursor = "> "
		}
		lines = append(lines, cursor+ch.Label)
	}
	lines = append(lines, "(↑/↓+Enter, c=close only, t=turn off, Esc=cancel)")
	return joinLines(lines)
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
