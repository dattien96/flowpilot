// CP-81 Task-417: TUI lifecycle participant tests — lease register/heartbeat/
// release, three-choice close dialog, fenced turn-off, planned-restart
// reconnect, unplanned-loss notice. Additive only; nothing here edits the
// pre-CP-81 contract tests.
package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// lifecycleStub is a scripted lifecycle-aware runner for TUI tests.
type lifecycleStub struct {
	srv *httptest.Server

	registerCalls  atomic.Int32
	heartbeatCalls atomic.Int32
	releaseCalls   atomic.Int32
	shutdownCalls  atomic.Int32
	restartCalls   atomic.Int32

	// lastShutdownBody / lastReleaseBody capture the final request payloads.
	lastShutdownBody atomic.Value // map[string]any
	shutdownBodies   chan map[string]any

	// Controls.
	snapshot   client.LifecycleSnapshot
	shutdownSt int // first shutdown response status (0 → 202)
	confirmTok string
}

func newLifecycleStub(t *testing.T) *lifecycleStub {
	t.Helper()
	s := &lifecycleStub{shutdownBodies: make(chan map[string]any, 4)}
	s.snapshot = client.LifecycleSnapshot{
		RunnerInstanceID: "inst-1",
		Generation:       1,
		ProtocolVersion:  1,
		Mode:             "client-managed",
		Phase:            "ready",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/system/lifecycle", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.snapshot)
	})
	mux.HandleFunc("/system/clients/register", func(w http.ResponseWriter, r *http.Request) {
		s.registerCalls.Add(1)
		writeJSON(w, http.StatusOK, client.RegisterClientResponse{
			LeaseID:             "l-tui",
			LeaseToken:          "tok-1",
			RunnerInstanceID:    "inst-1",
			Generation:          7,
			HeartbeatIntervalMs: 5000,
			TTLMs:               15000,
			Snapshot:            s.snapshot,
		})
	})
	mux.HandleFunc("/system/clients/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			s.heartbeatCalls.Add(1)
			writeJSON(w, http.StatusOK, s.snapshot)
		case strings.HasSuffix(r.URL.Path, "/release"):
			s.releaseCalls.Add(1)
			out := s.snapshot
			out.Phase = "idle_grace"
			writeJSON(w, http.StatusOK, out)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/system/shutdown", func(w http.ResponseWriter, r *http.Request) {
		s.shutdownCalls.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.lastShutdownBody.Store(body)
		select {
		case s.shutdownBodies <- body:
		default:
		}
		if s.shutdownCalls.Load() == 1 && s.shutdownSt == http.StatusConflict {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]any{
					"code":         "lifecycle_confirmation_required",
					"message":      "runner is shared or busy",
					"confirmToken": s.confirmTok,
				},
				"snapshot": s.snapshot,
			})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
	})
	mux.HandleFunc("/system/restart", func(w http.ResponseWriter, r *http.Request) {
		s.restartCalls.Add(1)
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "runnerInstanceId": "inst-1"})
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// runCmds executes a cmd tree (handles tea.BatchMsg) and returns leaf msgs.
func runCmds(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	return flattenMsg(t, msg)
}

func flattenMsg(t *testing.T, msg tea.Msg) []tea.Msg {
	t.Helper()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			out = append(out, flattenMsg(t, c())...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func findMsg[T any](msgs []tea.Msg) (T, bool) {
	var zero T
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			return v, true
		}
	}
	return zero, false
}

// registeredModel returns a model whose lease is already registered against
// the stub.
func registeredModel(t *testing.T, s *lifecycleStub) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{}, s.srv.URL)
	m.lease = tuiLease{
		LeaseID:          "l-tui",
		Token:            "tok-1",
		RunnerInstanceID: "inst-1",
		Generation:       7,
		HeartbeatEvery:   5 * time.Second,
		TTL:              15 * time.Second,
		Registered:       true,
	}
	return m
}

func lastMessage(m *AppModel) string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].Content
}

// ── §7 required tests ───────────────────────────────────────────────────────

func TestTUI_RegisterLeaseAfterRunnerReady(t *testing.T) {
	s := newLifecycleStub(t)
	m := New(config.ChatConfig{ProjectPath: "/tmp/proj"}, s.srv.URL)

	msgs := runCmds(t, m.cmdRegisterLease())
	reg, ok := findMsg[leaseRegisteredMsg](msgs)
	if !ok {
		t.Fatalf("expected leaseRegisteredMsg, got %v", msgs)
	}
	if reg.err != nil {
		t.Fatalf("register err: %v", reg.err)
	}
	if s.registerCalls.Load() != 1 {
		t.Fatalf("register calls=%d want 1", s.registerCalls.Load())
	}
	m2, cmd := m.applyLeaseRegistered(reg)
	am := m2.(*AppModel)
	if !am.lease.Registered || am.lease.LeaseID != "l-tui" || am.lease.Token != "tok-1" {
		t.Fatalf("lease not stored: %+v", am.lease)
	}
	if am.lease.Generation != 7 || am.lease.RunnerInstanceID != "inst-1" {
		t.Fatalf("generation/instance mismatch: %+v", am.lease)
	}
	if am.lease.HeartbeatEvery != 5*time.Second || am.lease.TTL != 15*time.Second {
		t.Fatalf("timing mismatch: %+v", am.lease)
	}
	if cmd == nil {
		t.Fatal("heartbeat tick must be scheduled after registration")
	}
}

func TestTUI_HeartbeatKeepsLeaseAlive(t *testing.T) {
	s := newLifecycleStub(t)
	s.snapshot.Phase = "ready"
	m := registeredModel(t, s)

	_, cmd := m.Update(LifecycleHeartbeatTickMsg{})
	if cmd == nil {
		t.Fatal("heartbeat tick must produce a cmd when registered")
	}
	msgs := runCmds(t, cmd)
	if s.heartbeatCalls.Load() != 1 {
		t.Fatalf("heartbeat calls=%d want 1", s.heartbeatCalls.Load())
	}
	snap, ok := findMsg[LifecycleSnapshotMsg](msgs)
	if !ok {
		t.Fatalf("expected LifecycleSnapshotMsg, got %v", msgs)
	}
	m2, _ := m.Update(snap)
	am := m2.(*AppModel)
	if am.lifecyclePhase != "ready" {
		t.Fatalf("phase=%q want ready", am.lifecyclePhase)
	}
}

func TestTUI_HeartbeatFailurePlannedRestartReconnects(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.lifecyclePhase = "draining_restart"
	s.snapshot.Phase = "draining_restart"
	s.snapshot.Restart = &client.RestartInfo{
		RestartID: "rst-1",
		Deadline:  time.Now().Add(30 * time.Second),
	}

	m2, cmd := m.applyHeartbeatErr(heartbeatErrMsg{err: errors.New("connection refused")})
	am := m2.(*AppModel)
	if am.reconnect == nil {
		t.Fatal("planned restart must enter reconnect mode")
	}
	if am.statusMsg != "runner restarting…" {
		t.Fatalf("statusMsg=%q", am.statusMsg)
	}
	if cmd == nil {
		t.Fatal("reconnect must start a /health poll cmd")
	}
}

func TestTUI_HeartbeatFailureUnplannedLossShowsNoticeAndQuits(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.lifecyclePhase = "ready"

	m2, cmd := m.applyHeartbeatErr(heartbeatErrMsg{err: errors.New("connection refused")})
	am := m2.(*AppModel)
	if !am.quitting {
		t.Fatal("unplanned loss must mark the model quitting")
	}
	if !strings.Contains(lastMessage(am), "Runner stopped unexpectedly") {
		t.Fatalf("missing loss notice, last=%q", lastMessage(am))
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestTUI_ExitAloneReleasesLeaseWithoutShutdown(t *testing.T) {
	s := newLifecycleStub(t)
	s.snapshot.Clients = []client.ClientLeaseView{
		{LeaseID: "l-tui", Kind: "tui", Label: "TUI 1"},
	}
	m := registeredModel(t, s)

	_, cmd := m.Update(ExitIntentMsg{Source: "ctrlc"})
	msgs := runCmds(t, cmd)
	if _, ok := findMsg[QuitMsg](msgs); !ok {
		t.Fatalf("sole-client exit must yield QuitMsg, got %v", msgs)
	}
	if s.releaseCalls.Load() != 1 {
		t.Fatalf("release calls=%d want 1", s.releaseCalls.Load())
	}
	if s.shutdownCalls.Load() != 0 || s.restartCalls.Load() != 0 {
		t.Fatalf("ordinary close must not hit system actions (shutdown=%d restart=%d)",
			s.shutdownCalls.Load(), s.restartCalls.Load())
	}
}

func TestTUI_ExitWithDesktopShowsThreeChoices(t *testing.T) {
	s := newLifecycleStub(t)
	s.snapshot.Clients = []client.ClientLeaseView{
		{LeaseID: "l-tui", Kind: "tui", Label: "TUI 1"},
		{LeaseID: "l-desk", Kind: "desktop", Label: "Desktop"},
	}
	m := registeredModel(t, s)
	m.quitting = true // quit intent pre-blanks the view

	_, cmd := m.Update(ExitIntentMsg{Source: "slash"})
	msgs := runCmds(t, cmd)
	dec, ok := findMsg[exitDecisionMsg](msgs)
	if !ok {
		t.Fatalf("shared exit must open the dialog, got %v", msgs)
	}
	m2, _ := m.applyExitDecision(dec)
	am := m2.(*AppModel)
	if am.closeDialog == nil {
		t.Fatal("dialog state must be open")
	}
	if am.quitting {
		t.Fatal("view must be visible while the dialog is open")
	}
	body := lastMessage(am)
	for _, want := range []string{"Cancel", "Close TUI only", "Turn off FlowPilot", "Desktop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dialog missing %q:\n%s", want, body)
		}
	}
}

func TestTUI_CloseThisClientOnlyLeavesRunner(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.closeDialog = &closeDialogState{Snapshot: s.snapshot}

	m2, cmd := m.applyCloseChoice(CloseThisClientOnly)
	am := m2.(*AppModel)
	if am.closeDialog != nil {
		t.Fatal("dialog must close after a choice")
	}
	msgs := runCmds(t, cmd)
	if _, ok := findMsg[QuitMsg](msgs); !ok {
		t.Fatalf("CloseThisClientOnly must quit, got %v", msgs)
	}
	if s.releaseCalls.Load() != 1 {
		t.Fatalf("release calls=%d want 1", s.releaseCalls.Load())
	}
	if s.shutdownCalls.Load() != 0 {
		t.Fatal("CloseThisClientOnly must leave the runner running")
	}
}

func TestTUI_CancelExitKeepsTUIAlive(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.quitting = true
	m.closeDialog = &closeDialogState{Snapshot: s.snapshot}

	m2, cmd := m.applyCloseChoice(CloseCancel)
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("cancel must not schedule a quit cmd")
	}
	if am.closeDialog != nil {
		t.Fatal("dialog must close on cancel")
	}
	if am.quitting {
		t.Fatal("cancel must restore the view")
	}
	if s.releaseCalls.Load() != 0 || s.shutdownCalls.Load() != 0 {
		t.Fatal("cancel must not call any lifecycle mutation")
	}
}

func TestTUI_TurnOffFlowPilotSendsConfirmedForce(t *testing.T) {
	s := newLifecycleStub(t)
	s.shutdownSt = http.StatusConflict
	s.confirmTok = "ct-1"
	m := registeredModel(t, s)

	msgs := runCmds(t, m.cmdShutdownAndQuit())
	if _, ok := findMsg[QuitMsg](msgs); !ok {
		t.Fatalf("turn-off must end in QuitMsg, got %v", msgs)
	}
	if s.shutdownCalls.Load() != 2 {
		t.Fatalf("expected fenced 2-phase shutdown, calls=%d", s.shutdownCalls.Load())
	}
	second := <-s.shutdownBodies
	<-s.shutdownBodies // drain order: bodies channel FIFO — first is the 409 attempt
	_ = second
	// The last recorded body is the confirmed replay.
	final, _ := s.lastShutdownBody.Load().(map[string]any)
	if final["confirm"] != true || final["confirmToken"] != "ct-1" {
		t.Fatalf("confirmed replay missing confirm fields: %v", final)
	}
	if final["requesterLeaseId"] != "l-tui" || final["expectedInstanceId"] != "inst-1" {
		t.Fatalf("fencing fields missing: %v", final)
	}
}

func TestTUI_LastClientExitStartsIdleGrace(t *testing.T) {
	s := newLifecycleStub(t)
	s.snapshot.Clients = []client.ClientLeaseView{
		{LeaseID: "l-tui", Kind: "tui", Label: "TUI 1"},
	}
	m := registeredModel(t, s)

	_, cmd := m.Update(ExitIntentMsg{Source: "ctrlc"})
	msgs := runCmds(t, cmd)
	if _, ok := findMsg[QuitMsg](msgs); !ok {
		t.Fatalf("last-client exit must quit, got %v", msgs)
	}
	// Release is the only mutation: the runner enters idle_grace server-side.
	if s.releaseCalls.Load() != 1 {
		t.Fatalf("release calls=%d want 1", s.releaseCalls.Load())
	}
	if s.shutdownCalls.Load() != 0 {
		t.Fatal("last-client exit must not force shutdown — idle grace applies")
	}
}

func TestTUI_RunnerRestartReattachReRegistersLease(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.lease.RunnerInstanceID = "inst-old"
	m.reconnect = &reconnectState{Deadline: time.Now().Add(30 * time.Second)}

	m2, cmd := m.applyReconnectPoll(reconnectPollMsg{instanceID: "inst-new", alive: true})
	am := m2.(*AppModel)
	if am.reconnect != nil {
		t.Fatal("reattach must clear reconnect state")
	}
	if am.lease.Registered {
		t.Fatal("old-generation lease must be dropped before re-register")
	}
	if cmd == nil {
		t.Fatal("reattach must re-register + reconnect")
	}
	runCmds(t, cmd)
	if s.registerCalls.Load() != 1 {
		t.Fatalf("re-register calls=%d want 1", s.registerCalls.Load())
	}
}

func TestTUI_ReconnectDeadlineExpiresCloses(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)
	m.reconnect = &reconnectState{Deadline: time.Now().Add(-time.Second)}

	m2, cmd := m.applyReconnectPoll(reconnectPollMsg{alive: false})
	am := m2.(*AppModel)
	if !am.quitting {
		t.Fatal("expired reconnect deadline must close the TUI")
	}
	if !strings.Contains(lastMessage(am), "Runner stopped unexpectedly") {
		t.Fatalf("missing notice, last=%q", lastMessage(am))
	}
	if _, ok := cmd().(QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestTUI_StatusShowsSharedClientsAndUpdatePending(t *testing.T) {
	s := newLifecycleStub(t)
	m := registeredModel(t, s)

	m.lifecycleClients = 2
	m.updatePending = true
	chip := m.lifecycleStatusChip()
	if !strings.Contains(chip, "shared:2") || !strings.Contains(chip, "update pending") {
		t.Fatalf("chip=%q want shared count + update pending", chip)
	}

	m.reconnect = &reconnectState{Deadline: time.Now().Add(time.Minute)}
	if chip := m.lifecycleStatusChip(); chip != "runner restarting…" {
		t.Fatalf("reconnect chip=%q", chip)
	}
}

func TestTUI_ExitDoesNotCallKillRunnerByURL(t *testing.T) {
	s := newLifecycleStub(t)
	s.snapshot.Clients = []client.ClientLeaseView{
		{LeaseID: "l-tui", Kind: "tui", Label: "TUI 1"},
	}
	m := registeredModel(t, s)

	// Ordinary close: only lifecycle read + release, never a system action or
	// a process kill (the app package does not import runnerboot at all).
	_, cmd := m.Update(ExitIntentMsg{Source: "slash"})
	runCmds(t, cmd)
	if s.shutdownCalls.Load() != 0 || s.restartCalls.Load() != 0 {
		t.Fatal("ordinary exit must not hit /system/shutdown or /system/restart")
	}
	if s.releaseCalls.Load() != 1 {
		t.Fatalf("release calls=%d want 1", s.releaseCalls.Load())
	}
}
