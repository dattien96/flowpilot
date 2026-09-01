package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// Chat-posture support (Task-xxx / CA-xxx): Scan/Plan/Code mode switching.
//
// The posture document lives on the runner (SSOT). The TUI reads/writes it ONLY
// through the runner's GET/PUT /client/chat-posture endpoints, mirroring the
// Desktop, so both surfaces always agree.

// postureOrder is the full posture key order (used by /mode <name> and /mode-setup).
// "non" (CA-685) is the no-mode posture: no pins, the session keeps the user's
// last provider/model choice.
var postureOrder = []string{"scan", "plan", "code", "non"}

// tabPostureOrder is the Tab/cycle order: Plan → Code → Non → Plan. Scan is only
// reachable via explicit /mode scan — the user must opt into a read-only
// posture intentionally, not cycle into it accidentally.
var tabPostureOrder = []string{"plan", "code", "non"}

func validPosture(name string) bool {
	for _, p := range postureOrder {
		if name == p {
			return true
		}
	}
	return false
}

// postureLabel is the human label shown in /mode output.
func postureLabel(name string) string {
	switch name {
	case "scan":
		return "scan — read-only (reads auto-approve, writes auto-deny)"
	case "plan":
		return "plan — read-only (reads auto-approve, writes auto-deny)"
	case "non":
		return "non — no posture (keeps your last model choice)"
	default:
		return "code — normal approvals / YOLO"
	}
}

// activePosture returns the TUI's current posture ("" = non, the no-mode
// default per CA-685).
func (m *AppModel) activePosture() string {
	if m.chatPosture == "" {
		return "non"
	}
	return m.chatPosture
}

// cmdLoadChatPosture fetches the runner's chat-posture document (5s
// timeout so a cold runner never leaves Update blocked — the treo report
// after the chat input appears).
func (m *AppModel) cmdLoadChatPosture() tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := cl.GetChatPosture(ctx)
		return chatPostureMsg{Cfg: cfg, Err: err}
	}
}

// cmdSaveChatPosture PUTs the runner's chat-posture document (after a local
// edit, 5s timeout for the same reason as cmdLoadChatPosture).
func (m *AppModel) cmdSaveChatPosture(cfg client.ChatPostureConfig) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		saved, err := cl.SetChatPosture(ctx, cfg)
		return chatPostureMsg{Cfg: saved, Err: err}
	}
}

// chatPostureCmd applies the pending action stored in chatPosturePending after a
// successful load. Returns "" when nothing is pending.
func (m *AppModel) chatPostureCmdFromPending(cfg client.ChatPostureConfig) tea.Cmd {
	pending := m.chatPosturePending
	m.chatPosturePending = ""
	switch {
	case pending == "restore":
		// TUI restart restore (SessionDefaultsMsg firstLoad): apply the runner's
		// persisted Active and its profile pins without marking dirty — this is
		// a read-only restore, not a user switch that must PUT active back.
		active := strings.TrimSpace(cfg.Active)
		if !validPosture(active) {
			active = "non" // CA-685: no-mode default for unknown/empty active
		}
		m.restoreChatPostureProfile(cfg, active)
		m.chatPostureCfg = cfg
		m.chatPostureCfg.Active = active
		return nil
	case pending == "show":
		m.displayChatPosture(cfg)
		return nil
	case strings.HasPrefix(pending, "apply:"):
		name := strings.TrimPrefix(pending, "apply:")
		if validPosture(name) {
			// CP-59 Task-315 (BUG-330): a cross-provider posture pin on a live
			// chat routes through the switch endpoint instead of swapping the
			// model inside the old provider; the full profile re-applies on the
			// new leg via the queued-posture path (applyChatSwitched).
			if cmd := m.routePostureSwitch(cfg, name); cmd != nil {
				return cmd
			}
			// Tab while a switch or turn is still in flight must not fall
			// through to an in-place apply — that leaked a grok provider onto
			// an opencode leg after a 409 and needed a double-Tab to reach
			// code (B-4: 417944→417970). Treat it as C2-busy like the switch
			// path so the next Tab can reach code cleanly.
			if m.chatSwitchInFlight || m.turnLive || m.turnStream != nil || m.turnSendPending || m.question != nil || len(m.questions) > 0 || m.approval != nil || len(m.approvals) > 0 {
				m.addMessage("system", "Cannot switch provider/model while a question or approval is pending — please answer it first", "error")
				return func() tea.Msg { return detachedNoticeMsg{} }
			}
			// CA-685: the active posture's full profile (reasoning included)
			// re-applies; /new is a refresh, not an override (supersedes the
			// CA-641 reasoning-keep carve-out).
			m.applyChatPostureProfile(cfg, name)
			// Persist the active posture back to the runner (SSOT) so the
			// Desktop and a later /new read the same selection — mirrors the
			// Desktop tab switch which PUTs active.
			m.chatPostureCfg = cfg
			m.chatPostureCfg.Active = name
			m.chatPostureDirty = true
		}
		return nil
	case strings.HasPrefix(pending, "setup:"):
		parts := strings.SplitN(strings.TrimPrefix(pending, "setup:"), ":", 3)
		if len(parts) == 3 {
			m.editChatPostureProfile(cfg, parts[0], parts[1], parts[2])
		}
		return nil
	case strings.HasPrefix(pending, "modal:"):
		tab := strings.TrimPrefix(pending, "modal:")
		m.chatPostureCfg = cfg
		m.openModeSetupModal(cfg, tab)
		return nil
	default:
		return nil
	}
}

// postureModelPinWins decides whether a posture profile's pinned model may
// override the current session model. Superseded by CA-685 (operator decision):
// the posture semantics are now explicit —
//
//   - "non" (ChatPostureNon): NO mode. No pins are applied, ever; the session
//     keeps the user's last provider/model choice across restarts.
//   - scan/plan/code: the pinned profile IS the posture's config. Restart
//     restore and /new re-apply always show the pinned model; the pin itself
//     only changes via /mode-setup or the Desktop settings. Mid-session /model
//     still works for the current chat, it just does not survive a restart
//     while a real posture is active.

// posturePinProvider resolves the provider a posture pin demands: the explicit
// provider pin when present, else the provider inferred from the pinned model
// (BUG-330 guard — a grok-4.5 pin must not stamp an opencode session).
func (m *AppModel) posturePinProvider(prof client.ChatPostureProfile) string {
	if p := strings.TrimSpace(prof.Provider); p != "" {
		return p
	}
	if prof.Model != "" {
		if inferred := providerForModel(m.providers, prof.Model); inferred != "" {
			return inferred
		}
	}
	return ""
}

// applyChatPostureProfile switches the session to a posture and applies its
// pinned profile fields (provider/model/reasoning/yolo) when set.
// CA-685: "non" applies nothing — the session keeps the user's last choice.
// In scan/plan/code the FULL profile pins apply, reasoning included (operator
// decision 2026-08-29 — supersedes the CA-641 re-apply carve-out; keeping a
// personal reasoning choice across restarts is what the "non" posture is for).
func (m *AppModel) applyChatPostureProfile(cfg client.ChatPostureConfig, name string) {
	m.chatPosture = name
	if name == "non" {
		// No pins — provider/model/reasoning stay exactly as the user left them.
		m.persistSessionPrefs()
		m.refreshSessionPanel()
		m.addMessage("system", fmt.Sprintf("Mode: %s", postureLabel(name)), "")
		return
	}
	// A profile switching to scan/plan also flips the read-only expectation; the
	// runner enforces it via ChatPosture on the turn.
	prof := cfg.Profiles[name]
	if pinProvider := m.posturePinProvider(prof); pinProvider != "" {
		m.setPostureProvider(pinProvider)
	}
	if prof.Model != "" {
		m.model = prof.Model
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
	}
	if prof.ReasoningEffort != "" {
		m.reasoningEffort = prof.ReasoningEffort
	}
	m.clampReasoningForCurrentModel() // CA-686: reasoning is dynamic per model
	if prof.Yolo != nil {
		m.yolo = *prof.Yolo
		if strings.ToLower(m.provider) == "grok" {
			// keep in sync with the runner's Grok posture endpoint
			m.postureGrokSync = *prof.Yolo
			m.postureGrokSyncSet = true
		}
	}
	m.persistSessionPrefs()
	m.refreshSessionPanel()
	m.addMessage("system", fmt.Sprintf("Mode: %s", postureLabel(name)), "")
}

// restoreChatPostureProfile is the restart-restore variant of
// applyChatPostureProfile: it applies the persisted Active's pinned fields
// without marking dirty and without the "Mode: …" banner — the TUI is just
// resuming where the user left off.
// CA-685 semantics: scan/plan/code always come back with their pinned model
// AND reasoning (mid-session /model or /reasoning changes do not survive a
// restart in a real posture — the pin only changes via /mode-setup or Desktop
// settings); "non" applies nothing, so the user's persisted choices survive.
// (Operator decision 2026-08-29 supersedes CA-638's reasoning-keep rule.)
func (m *AppModel) restoreChatPostureProfile(cfg client.ChatPostureConfig, name string) {
	m.chatPosture = name
	if name == "non" {
		m.persistSessionPrefs()
		m.refreshSessionPanel()
		return
	}
	prof := cfg.Profiles[name]
	if pinProvider := m.posturePinProvider(prof); pinProvider != "" {
		m.setPostureProvider(pinProvider)
	}
	if prof.Model != "" {
		m.model = prof.Model
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
	}
	if prof.ReasoningEffort != "" {
		// CA-685: the posture's reasoning pin loads with the profile (e.g. the
		// scan posture's xhigh) — /reasoning remains free mid-session.
		m.reasoningEffort = prof.ReasoningEffort
	}
	m.clampReasoningForCurrentModel() // CA-686: reasoning is dynamic per model
	if prof.Yolo != nil {
		m.yolo = *prof.Yolo
		if strings.ToLower(m.provider) == "grok" {
			m.postureGrokSync = *prof.Yolo
			m.postureGrokSyncSet = true
		}
	}
	m.persistSessionPrefs()
	m.refreshSessionPanel()
}

// setPostureProvider mirrors /provider <key> application.
func (m *AppModel) setPostureProvider(want string) {
	found := false
	for i := range m.providers {
		if strings.EqualFold(m.providers[i].Key, want) {
			m.provider = m.providers[i].Key
			models := modelsForProvider(m.providers, m.provider)
			if len(models) > 0 {
				m.model = models[0]
			} else {
				m.model = ""
			}
			found = true
			break
		}
	}
	if !found {
		m.provider = want
		m.model = ""
	}
	m.bindActiveAccountForProvider()
	m.skillsCatalog = nil
}

// providerForModel returns the provider key that owns a model ID, or "".
func providerForModel(providers []client.Provider, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	for _, p := range providers {
		for _, m := range p.Models {
			if strings.EqualFold(m.ModelID(), modelID) {
				return p.Key
			}
		}
	}
	return ""
}

// editChatPostureProfile applies a profile edit from /mode-setup and saves.
func (m *AppModel) editChatPostureProfile(cfg client.ChatPostureConfig, posture, field, value string) {
	if !validPosture(posture) {
		m.addMessage("system", fmt.Sprintf("Unknown posture %q. Use /mode-setup to list.", posture), "error")
		return
	}
	prof := cfg.Profiles[posture]
	lowerField := strings.ToLower(field)
	switch lowerField {
	case "provider":
		prof.Provider = strings.TrimSpace(value)
	case "model":
		prof.Model = strings.TrimSpace(value)
		// FlowPilot rule: picking a model auto-pins its provider (like /model
		// lists all models across providers). Infer provider from catalog.
		if prof.Model != "" {
			if inferred := providerForModel(m.providers, prof.Model); inferred != "" {
				prof.Provider = inferred
			}
		}
	case "reasoning", "reason":
		eff := strings.ToLower(strings.TrimSpace(value))
		if eff != "" && eff != "high" && eff != "medium" && eff != "low" {
			m.addMessage("system", "Reasoning must be high, medium, or low.", "error")
			return
		}
		prof.ReasoningEffort = eff
	case "yolo":
		switch strings.ToLower(value) {
		case "on", "true", "1":
			on := true
			prof.Yolo = &on
		case "off", "false", "0":
			off := false
			prof.Yolo = &off
		case "clear", "-":
			prof.Yolo = nil
		default:
			m.addMessage("system", "YOLO must be on or off (or clear).", "error")
			return
		}
	case "clear":
		prof = client.ChatPostureProfile{}
	default:
		m.addMessage("system", "Field must be provider, model, reasoning, yolo, or clear.", "error")
		return
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]client.ChatPostureProfile{}
	}
	cfg.Profiles[posture] = prof
	m.chatPostureCfg = cfg
	m.chatPostureDirty = true
	m.chatPostureSaving = true
	m.chatPostureSavingPosture = posture
	m.addMessage("system", fmt.Sprintf("Posture %s updated (saving…)", posture), "")
}

// displayChatPosture prints the runner's posture document and profiles.
func (m *AppModel) displayChatPosture(cfg client.ChatPostureConfig) {
	var sb strings.Builder
	sb.WriteString("Chat postures (Tab cycles when input is empty, /mode <name> applies):\n")
	active := cfg.Active
	if m.chatPosture != "" {
		active = m.chatPosture
	}
	for _, name := range postureOrder {
		mark := " "
		if name == active {
			mark = "*"
		}
		p := cfg.Profiles[name]
		prov := orDash(p.Provider)
		model := orDash(p.Model)
		reason := orDash(p.ReasoningEffort)
		yolo := "(inherit)"
		if p.Yolo != nil {
			if *p.Yolo {
				yolo = "on"
			} else {
				yolo = "off"
			}
		}
		sb.WriteString(fmt.Sprintf("  %s %-5s provider=%s model=%s reasoning=%s yolo=%s\n",
			mark, name, prov, model, reason, yolo))
	}
	sb.WriteString("Setup: /mode-setup <scan|plan|code> <provider|model|reasoning|yolo|clear> <value>")
	m.addMessage("system", strings.TrimRight(sb.String(), "\n"), "")
}
