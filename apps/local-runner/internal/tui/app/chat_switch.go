package app

// Chat switch surface (CP-59 Task-315): /provider <key> and /model <foreign>
// on a live chat route through the runner switch endpoint (Task-314).
// Adoption keeps the transcript (`m.messages` is never cleared), resets
// per-run stream state, and attaches the new leg's orchestration stream — the
// divider renders from the seed turn (single source, SD-26 D-7); clients never
// synthesize a divider on success.

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// ChatSwitchedMsg carries the switch result back to the Update loop.
type ChatSwitchedMsg struct {
	Resp *client.ChatSwitchResponse
	Err  error
}

// routeProviderSwitch decides whether a provider change on a live chat goes
// through the switch endpoint (Task-315 T-3 routing rule). Returns a command
// when routed; nil when the caller must fall back to the legacy path (no chat
// run, workflow/flow run, same-provider model change, or a switch already in
// flight). targetProvider/model come from /provider <key> or /model <id>.
func (m *AppModel) routeProviderSwitch(targetProvider, model string) tea.Cmd {
	if targetProvider == "" || m.runHandle == nil || m.chatSwitchInFlight {
		return nil
	}
	if m.runHandle.RunKind != "chat" || strings.TrimSpace(m.runHandle.ChatID) == "" {
		return nil
	}
	if strings.EqualFold(targetProvider, m.provider) {
		return nil // same-provider continuity stays in-place (CP-59 P-7)
	}
	return m.cmdSwitchChatProvider(targetProvider, model)
}

// cmdSwitchChatProvider calls the runner switch endpoint (fire-and-return
// seed server-side) and reports via ChatSwitchedMsg. Guard: one in-flight
// switch; rapid re-routes during flight are dropped (single-leg guarantee,
// CP-59 CS-05; Desktop parity via providerSwitchLoading).
func (m *AppModel) cmdSwitchChatProvider(targetProvider, model string) tea.Cmd {
	if m.chatSwitchInFlight || m.runHandle == nil {
		return nil
	}
	chatID := strings.TrimSpace(m.runHandle.ChatID)
	if chatID == "" {
		return nil
	}
	m.chatSwitchInFlight = true
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		resp, err := cl.SwitchChatProvider(ctx, chatID, client.ChatSwitchInput{
			TargetProviderKey: targetProvider,
			Model:             model,
			ReasoningEffort:   m.reasoningEffort,
		})
		return ChatSwitchedMsg{Resp: &resp, Err: err}
	}
}

// applyChatSwitched handles ChatSwitchedMsg (the RunStartedMsg template,
// app.go:1221 — which never clears messages). On failure the chat continues on
// the source leg with an error line; on success the transcript is kept, the
// new handle adopted, per-run stream state reset, and the new leg's
// orchestration stream attached (seed turn renders live through it).
func (m *AppModel) applyChatSwitched(msg ChatSwitchedMsg) {
	m.chatSwitchInFlight = false
	if msg.Err != nil || msg.Resp == nil {
		m.addMessage("system", "Provider switch failed: "+msg.Err.Error()+" — chat continues on "+m.provider, "error")
		return
	}
	h := msg.Resp.Handle
	m.runHandle = &h
	if h.ProviderKey != "" {
		m.provider = string(h.ProviderKey)
	}
	if msg.Resp.Model != "" {
		m.model = msg.Resp.Model
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
	}
	m.lastEventSeq = h.LastEventSeq
	m.turnStream = nil
	m.turnSendPending = false
	m.client.NoteLastSeq(h.RunID, h.LastEventSeq)
	m.bindActiveAccountForProvider()
	m.skillsCatalog = nil
	m.refreshSessionPanel()
	if queued := m.chatSwitchQueuedPosture; queued != "" {
		m.chatSwitchQueuedPosture = ""
		m.addMessage("system", "Queued posture "+queued+" applies on the next prompt", "")
	}
}
