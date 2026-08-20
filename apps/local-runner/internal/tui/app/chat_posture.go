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
var postureOrder = []string{"scan", "plan", "code"}

// tabPostureOrder is the Tab/cycle order: Plan ↔ Code only. Scan is only
// reachable via explicit /mode scan — the user must opt into a read-only
// posture intentionally, not cycle into it accidentally.
var tabPostureOrder = []string{"plan", "code"}

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
	default:
		return "code — normal approvals / YOLO"
	}
}

// activePosture returns the TUI's current posture ("" = code).
func (m *AppModel) activePosture() string {
	if m.chatPosture == "" {
		return "code"
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
			active = "code"
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
	default:
		return nil
	}
}

// applyChatPostureProfile switches the session to a posture and applies its
// pinned profile fields (provider/model/reasoning/yolo) when set.
func (m *AppModel) applyChatPostureProfile(cfg client.ChatPostureConfig, name string) {
	m.chatPosture = name
	// A profile switching to scan/plan also flips the read-only expectation; the
	// runner enforces it via ChatPosture on the turn.
	prof := cfg.Profiles[name]
	if prof.Provider != "" {
		m.setPostureProvider(prof.Provider)
	}
	if prof.Model != "" {
		m.model = prof.Model
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
	}
	if prof.ReasoningEffort != "" {
		m.reasoningEffort = prof.ReasoningEffort
	}
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
func (m *AppModel) restoreChatPostureProfile(cfg client.ChatPostureConfig, name string) {
	m.chatPosture = name
	prof := cfg.Profiles[name]
	if prof.Provider != "" {
		m.setPostureProvider(prof.Provider)
	}
	if prof.Model != "" {
		m.model = prof.Model
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
	}
	if prof.ReasoningEffort != "" {
		m.reasoningEffort = prof.ReasoningEffort
	}
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