package app

// Chat switch surface (CP-59 Task-315): /provider <key> and /model <foreign>
// on a live chat route through the runner switch endpoint (Task-314).
// Adoption keeps the transcript (`m.messages` is never cleared), resets
// per-run stream state, and attaches the new leg's orchestration stream — the
// divider renders from the seed turn (single source, SD-26 D-7); clients never
// synthesize a divider on success.
//
// Slice 3: detached chats (restored, no active leg — SD26 §10) defer provider
// changes to the next prompt, which reattaches via startRun carrying the chat
// identity; /open backfills prior legs' turns from the chat timeline.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// Chat transcript record types (SD-26 §6.1, SD26-E-1..E-9) — mirrored as
// literals; parity with the runner is exercised by the timeline handler tests.
const (
	tuiRecTurnStarted      = "turn_started"
	tuiRecMessageCompleted = "message_completed"
	tuiRecProviderSwitch   = "chat_provider_switch"
)

// ChatSwitchedMsg carries the switch result back to the Update loop.
type ChatSwitchedMsg struct {
	Resp *client.ChatSwitchResponse
	Err  error
}

// ReattachedMsg reports a detached-chat reattach: a fresh leg was minted via
// startRun carrying the chat identity; the queued prompt sends on it.
type ReattachedMsg struct {
	Handle client.RunHandle
	Err    error
}

// detachedNoticeMsg is a no-op marker so a detached provider/model change can
// return a non-nil command (apply + notice) without a runner round-trip.
type detachedNoticeMsg struct{}

// chatTimelineBackfillMsg carries the prior legs' transcript records for /open
// restore-by-chat rendering.
type chatTimelineBackfillMsg struct {
	Records  []client.ChatTranscriptRecord
	Current  string
	Err      error
	Detached bool
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
	// Detached chat (restored, no active leg — SD26 §10): selections apply
	// locally and ride the next prompt's reattach; no switch endpoint call.
	if m.chatDetached && m.runHandle.RunKind == "chat" && strings.TrimSpace(m.runHandle.ChatID) != "" {
		if targetProvider != "" {
			m.provider = targetProvider
			m.bindActiveAccountForProvider()
			m.skillsCatalog = nil
		}
		if model != "" {
			m.model = model
		}
		m.addMessage("system", fmt.Sprintf("%s %s will apply when the chat reattaches on your next prompt", targetProvider, orDash(model)), "")
		m.refreshSessionPanel()
		return func() tea.Msg { return detachedNoticeMsg{} }
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
	// Carry the handoff stats for the seed-envelope divider (D-7): the seed
	// event arrives on the attached stream and formats with these stats.
	st := msg.Resp.Handoff
	m.lastSwitchStats = &st
	m.lastSwitchTarget = strings.TrimSpace(m.provider + " · " + m.model)
}

// cmdReattachChat mints a fresh local leg for a detached chat via startRun
// carrying the chat identity (SD26 §10); the queued prompt sends on it after
// ReattachedMsg adopts the handle. Runs on the CURRENT provider/model
// selection — detached provider/model changes were applied locally already.
func (m *AppModel) cmdReattachChat() tea.Cmd {
	if m.runHandle == nil || strings.TrimSpace(m.runHandle.ChatID) == "" {
		return nil
	}
	chatID := strings.TrimSpace(m.runHandle.ChatID)
	switchFrom := m.runHandle.RunID
	projectID := ""
	cwd := m.cfg.ProjectPath
	if m.project != nil {
		if m.project.Path != "" {
			cwd = m.project.Path
		}
		projectID = m.project.ID
	}
	in := client.StartRunInput{
		ProjectID:       projectID,
		ChatMode:        "normal_chat",
		ProviderKey:     m.provider,
		Model:           m.model,
		ReasoningEffort: m.reasoningEffort,
		Cwd:             cwd,
		ChatID:          chatID,
		SwitchFromRunID: switchFrom,
	}
	in.YoloMode = m.effectiveYolo()
	cl := m.client
	return func() tea.Msg {
		handle, err := cl.StartRun(context.Background(), in)
		return ReattachedMsg{Handle: handle, Err: err}
	}
}

// cmdBackfillChatTimeline fetches the chat timeline for /open restore-by-chat:
// prior legs' turns render into the transcript and the detached flag derives
// from the legs (no active leg = detached, SD26 §10).
func (m *AppModel) cmdBackfillChatTimeline(handle client.RunHandle) tea.Cmd {
	if strings.TrimSpace(handle.ChatID) == "" {
		return nil
	}
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := cl.GetChatTimeline(ctx, handle.ChatID, 0, 0)
		if err != nil {
			return chatTimelineBackfillMsg{Current: handle.RunID, Err: err}
		}
		detached := true
		for _, leg := range resp.Legs {
			if leg.LegState == "active" {
				detached = false
			}
		}
		return chatTimelineBackfillMsg{Records: resp.Records, Current: handle.RunID, Err: nil, Detached: detached}
	}
}

// renderChatTimelineBackfill maps prior legs' records into the transcript:
// user/assistant text turns oldest-first plus one divider per provider switch
// (stats from the E-9 payload). The current leg's own records are skipped —
// its history renders from the run replay. Idempotent per chat: a guard flag
// prevents double backfill when /open fires twice.
func (m *AppModel) renderChatTimelineBackfill(msg chatTimelineBackfillMsg) {
	if m.chatBackfillDone {
		return
	}
	m.chatBackfillDone = true
	for _, rec := range msg.Records {
		if rec.LegRunID == msg.Current {
			continue // current leg renders from the run replay
		}
		switch rec.Type {
		case tuiRecTurnStarted:
			var p struct {
				Prompt string `json:"prompt"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && strings.TrimSpace(p.Prompt) != "" {
				m.addMessage("user", p.Prompt, "")
			}
		case tuiRecMessageCompleted:
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && strings.TrimSpace(p.Text) != "" {
				m.addMessage("assistant", p.Text, "")
			}
		case tuiRecProviderSwitch:
			var p struct {
				ToProvider        string `json:"toProvider"`
				ToModel           string `json:"toModel"`
				HandoffMode       string `json:"handoffMode"`
				IncludedTurnCount int    `json:"includedTurnCount"`
				OmittedTurnCount  int    `json:"omittedTurnCount"`
				Truncated         bool   `json:"truncated"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil {
				carried := fmt.Sprintf("%d turns", p.IncludedTurnCount)
				if p.Truncated && p.OmittedTurnCount > 0 {
					carried = fmt.Sprintf("%d of %d turns", p.IncludedTurnCount, p.IncludedTurnCount+p.OmittedTurnCount)
				}
				m.addMessage("system", fmt.Sprintf("⇄ switched to %s · %s — carried %s (%s)", p.ToProvider, p.ToModel, carried, p.HandoffMode), "")
			}
		}
	}
}
