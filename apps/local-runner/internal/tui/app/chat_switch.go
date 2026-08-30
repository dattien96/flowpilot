package app

// Chat switch surface (CP-59 Task-315): /provider <key> and /model <foreign>
// on a live chat route through the runner switch endpoint (Task-314).
// Adoption keeps the transcript (`m.messages` is never cleared), resets
// per-run stream state, and attaches the new leg's orchestration stream — the
// divider renders from the seed turn (single source, SD-26 D-7); clients never
// synthesize a divider on success.

import (
	"context"
	"fmt"
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

// pinnedProviderFor derives a posture profile's provider (Task-315 T-2):
// explicit pin, else the model catalog (which covers exactly the ids the
// runner serves — a separate prefix mirror would drift from it).
func (m *AppModel) pinnedProviderFor(prof client.ChatPostureProfile) string {
	if p := strings.TrimSpace(prof.Provider); p != "" {
		return p
	}
	return providerForModel(m.providers, prof.Model)
}

// routePostureSwitch routes a cross-provider posture Tab to the switch
// endpoint — the BUG-330 fix at the TUI entry point. A bare-model pin derives
// its provider once and persists the derived provider (CP-59 P-6 / Q-2).
// Returns nil when the apply should proceed in place (no chat, workflow run,
// same-provider pin, switch already in flight).
func (m *AppModel) routePostureSwitch(cfg client.ChatPostureConfig, name string) tea.Cmd {
	prof, ok := cfg.Profiles[name]
	if !ok {
		return nil
	}
	pinned := m.pinnedProviderFor(prof)
	if pinned == "" || m.runHandle == nil || m.chatSwitchInFlight {
		return nil
	}
	if m.runHandle.RunKind != "chat" || strings.TrimSpace(m.runHandle.ChatID) == "" {
		return nil
	}
	if strings.EqualFold(pinned, m.provider) {
		return nil
	}
	cmd := m.cmdSwitchChatProvider(pinned, prof.Model)
	if cmd == nil {
		return nil
	}
	if strings.TrimSpace(prof.Provider) == "" {
		// Derive-once: stamp the derived provider into the profile and persist
		// it back to the runner SSOT with a one-time warning.
		prof.Provider = pinned
		cfg.Profiles[name] = prof
		m.chatPostureCfg = cfg
		m.chatPostureDirty = true
		m.addMessage("system", fmt.Sprintf("Posture %s pin had no provider — derived %q from model %q (persisting)", name, pinned, prof.Model), "")
		return tea.Batch(cmd, m.cmdSaveChatPosture(cfg))
	}
	m.chatSwitchQueuedPosture = name
	return cmd
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
		if validPosture(queued) {
			// Full profile re-application on the new leg (CA-685 semantics):
			// reasoning/yolo/posture pins land here; the model pin matches the
			// switched-to model by construction.
			m.applyChatPostureProfile(m.chatPostureCfg, queued)
		}
	}
}
