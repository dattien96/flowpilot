package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// ---- Styles -----------------------------------------------------------------

var (
	// Bright colors — avoid faint/grey so chat stays readable on dark/light terminals.
	styleUser      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))  // bright cyan
	styleAssistant = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")) // bright white
	styleSystem    = lipgloss.NewStyle().Foreground(lipgloss.Color("159"))            // light cyan
	styleTool      = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))            // light yellow
	styleError     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")) // bright red
	styleGate      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")) // pink
	styleStatus    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))            // near-white
	styleStatusOK  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))  // bright green
	styleStatusErr = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	stylePrompt       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	// Input uses a stroke/frame (no full-row background highlight — hard to read).
	stylePromptFocus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	styleInputFocus  = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	styleInputStroke = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleCursor      = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(lipgloss.Color("231"))
	styleSuggest     = lipgloss.NewStyle().Foreground(lipgloss.Color("123"))
	styleSuggestSel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Underline(true)
	styleLoading     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226")).Background(lipgloss.Color("236"))
)

// ---- New / Init -------------------------------------------------------------

// New creates a new AppModel from ChatConfig and the runner URL.
func New(cfg config.ChatConfig, runnerURL string) *AppModel {
	m := &AppModel{
		cfg:             cfg,
		runnerURL:       runnerURL,
		client:          client.New(runnerURL),
		yolo:            cfg.Yolo,
		provider:        cfg.Provider,
		model:           cfg.Model,
		reasoningEffort: cfg.ReasoningEffort,
		projectPath:     cfg.ProjectPath,
		connStatus:      ConnConnecting,
		statusMsg:       "connecting...",
		width:           80,
		height:          32, // room for /help list + status/input without clipping top commands
		asciiMode:       isLegacyConsole(),
	}
	return m
}

// Init is the Bubble Tea Init function.
func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(m.cmdConnect(), tickCursor())
}

func tickCursor() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(time.Time) tea.Msg {
		return cursorTickMsg{}
	})
}

// ---- Update -----------------------------------------------------------------

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case cursorTickMsg:
		m.cursorOn = !m.cursorOn
		if m.sessionLoading {
			m.loadingFrame = (m.loadingFrame + 1) % 64
		}
		return m, tickCursor()

	case ErrMsg:
		m.err = msg.Err
		m.sessionLoading = false
		m.pendingPrompt = ""
		m.connStatus = ConnError
		m.statusMsg = "error"
		errText := msg.Err.Error()
		m.addMessage("system", "Error: "+errText, "error")
		return m, nil

	case ConnectedMsg:
		m.runnerURL = msg.RunnerURL
		m.connStatus = ConnIdle
		m.statusMsg = "connected"
		m.sessionLoading = true
		m.sessionPanel.RunnerURL = msg.RunnerURL
		m.sessionPanel.ProjectPath = m.cfg.ProjectPath
		m.sessionPanel.Collapsed = false
		cmds := []tea.Cmd{m.cmdLoadSessionDefaults(), m.cmdPrefetchFlows()}
		if m.cfg.ResumeRunID != "" {
			cmds = append(cmds, m.cmdResume(m.cfg.ResumeRunID))
		}
		return m, tea.Batch(cmds...)

	case SessionDefaultsMsg:
		if msg.Provider != "" {
			m.provider = msg.Provider
		}
		if msg.Model != "" {
			m.model = msg.Model
		}
		if msg.AccountLabel != "" {
			m.accountLabel = msg.AccountLabel
		}
		if len(msg.Providers) > 0 {
			m.providers = msg.Providers
		}
		if msg.ProviderAccounts != nil {
			m.providerAccounts = msg.ProviderAccounts
		}
		if len(msg.Projects) > 0 {
			m.projects = msg.Projects
		}
		if msg.Project != nil {
			m.project = msg.Project
		}
		if msg.Account != nil && (m.provider == "" || strings.EqualFold(msg.Account.ProviderKey, m.provider)) {
			m.account = msg.Account
			if m.accountLabel == "" {
				m.accountLabel = msg.Account.DisplayLabel
			}
		}
		m.bindActiveAccountForProvider()
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
		m.refreshSessionPanel()
		if msg.CatalogErr != "" {
			m.addMessage("system", "Project catalog unavailable: "+msg.CatalogErr+"\nChat needs Supabase catalog (same as Desktop). Fix .env / runner, then restart.", "error")
		}
		if m.project == nil && m.cfg.ProjectPath != "" {
			m.addMessage("system", formatMissingProjectHelp(m.cfg.ProjectPath, m.projects), "error")
		}
		m.applyAuthNotice(msg.CatalogErr)
		m.sessionLoading = false
		m.sessionDefaultsLoaded = true
		m.connStatus = ConnIdle
		m.statusMsg = "ready"
		m.addMessage("system", "Ready — type / for commands · F2 or /info toggles session panel.", "")
		cmds := []tea.Cmd{m.cmdRefreshProjectContext()}
		if m.project != nil && len(m.chatList) == 0 {
			cmds = append(cmds, m.cmdPrefetchChats())
		}
		return m, tea.Batch(cmds...)

	case ProjectContextMsg:
		if msg.Path != "" {
			m.projectPath = msg.Path
		}
		m.projectBranch = msg.Branch
		m.refreshSessionPanel()
		return m, nil

	case DesktopEnsureMsg:
		var authLine string
		if msg.AuthSync != "" {
			authLine = "\nAuth session synced for Desktop: " + msg.AuthSync
		} else if msg.AuthErr != "" {
			authLine = "\nAuth sync skipped: " + msg.AuthErr + " (Desktop may show Login — run /login in TUI first)"
		}
		if msg.Err != "" {
			m.addMessage("system", "Desktop ensure failed: "+msg.Err+"\n"+settingsBridgeBlurb(msg.URL)+authLine, "error")
			return m, nil
		}
		if msg.Reused {
			m.addMessage("system", "Desktop already running.\n"+settingsBridgeBlurb(msg.URL)+authLine, "")
		} else if msg.Launched {
			m.addMessage("system", "Starting Desktop app (detached)…\n"+settingsBridgeBlurb(msg.URL)+"\nLog: <workspace>/.flowpilot/cli-desktop.log"+authLine, "")
		} else {
			m.addMessage("system", settingsBridgeBlurb(msg.URL)+authLine, "")
		}
		return m, nil

	case ProviderConnectMsg:
		if msg.Err != "" {
			m.addMessage("system", fmt.Sprintf("Provider connect failed (%s): %s", msg.ProviderKey, msg.Err), "error")
			return m, nil
		}
		m.addMessage("system", fmt.Sprintf(
			"Provider %s: login terminal/browser requested. Finish auth there, then /provider or /status to refresh.",
			msg.ProviderKey,
		), "")
		return m, m.cmdLoadSessionDefaults()

	case ProviderInstallMsg:
		if len(msg.Providers) > 0 {
			m.providers = msg.Providers
		}
		if msg.Err != "" {
			m.addMessage("system", fmt.Sprintf("Provider install failed (%s): %s", msg.ProviderKey, msg.Err), "error")
			return m, nil
		}
		status := "install finished"
		if p := findProvider(m.providers, msg.ProviderKey); p != nil {
			if p.Installed {
				status = "installed"
				if ver := strings.TrimSpace(p.DetectedVersion); ver != "" {
					status = "installed · " + ver
				}
			} else {
				status = "not installed yet — check installer output / PATH, then /provider"
			}
		}
		m.addMessage("system", fmt.Sprintf(
			"Provider %s: %s. Next: /provider connect %s (same as Desktop after Install).",
			msg.ProviderKey, status, msg.ProviderKey,
		), "")
		return m, m.cmdLoadSessionDefaults()

	case ChatListMsg:
		if msg.Silent {
			if msg.Err == "" {
				m.chatList = msg.Items
			}
			return m, nil
		}
		if msg.Err != "" {
			m.addMessage("system", "Chat list failed: "+msg.Err, "error")
			return m, nil
		}
		m.chatList = msg.Items
		m.addMessage("system", formatChatList(msg.Items), "")
		return m, nil

	case ChatOpenedMsg:
		if msg.Err != "" {
			m.addMessage("system", msg.Err, "error")
			m.connStatus = ConnError
			m.statusMsg = "open failed"
			return m, nil
		}
		handle := msg.Handle
		m.runHandle = &handle
		m.stepID = handle.StepID
		m.pendingPrompt = ""
		m.firstTurnPending = false
		m.gate = nil
		m.approval = nil
		m.question = nil
		m.messages = nil
		if len(msg.Messages) > 0 {
			m.messages = append([]ChatMessage(nil), msg.Messages...)
		}
		if pk := strings.TrimSpace(handle.ProviderKey); pk != "" {
			m.provider = pk
			m.bindActiveAccountForProvider()
			m.refreshSessionPanel()
		}
		m.connStatus = ConnIdle
		m.statusMsg = fmt.Sprintf("opened %s", shortID(handle.RunID))
		m.addMessage("system", fmt.Sprintf("Opened chat %s — continue typing or /history|/open|/resume to switch.", handle.RunID), "")
		return m, nil

	case LoginResultMsg:
		m.authPhase = AuthNone
		m.authEmail = ""
		m.signedInEmail = msg.Email
		m.authNeedLogin = false
		line := fmt.Sprintf("Signed in as %s", orDash(msg.Email))
		if msg.UserID != "" {
			line += fmt.Sprintf(" (user %s)", msg.UserID)
		}
		if msg.PersistedTo != "" {
			line += "\nDesktop session saved — reopen Desktop to leave the login screen."
		} else if msg.PersistError != "" {
			line += "\nCould not write Desktop session file: " + msg.PersistError
			line += "\nLogin succeeded on runner; sign in on Desktop if it still shows Login."
		}
		m.addMessage("system", line, "")
		m.sessionLoading = true
		m.statusMsg = "loading session..."
		m.addMessage("system", "Reloading session after login…", "")
		return m, m.cmdLoadSessionDefaults()

	case FlowListMsg:
		m.flowBuiltins = msg.Builtins
		m.flowWorkflows = msg.Workflows
		if msg.Silent {
			return m, nil
		}
		var sb strings.Builder
		sb.WriteString("Built-in flows (bug mode):\n")
		if len(msg.Builtins) == 0 {
			sb.WriteString("  (none)\n")
		} else {
			for _, opt := range msg.Builtins {
				sb.WriteString(fmt.Sprintf("  %s\n    %s\n", opt.FlowRef, opt.Label))
				if opt.Description != "" {
					sb.WriteString(fmt.Sprintf("    %s\n", opt.Description))
				}
			}
		}
		sb.WriteString("Catalog workflows")
		if m.project != nil {
			sb.WriteString(fmt.Sprintf(" (project %s)", m.project.ID))
		}
		sb.WriteString(":\n")
		if msg.CatalogErr != "" {
			sb.WriteString(fmt.Sprintf("  (catalog unavailable: %s)\n", msg.CatalogErr))
			sb.WriteString("  Fix: /login if Desktop shows Login, or ensure runner can reach Supabase (.env), then restart.\n")
		} else {
			shown := 0
			for _, wf := range msg.Workflows {
				if m.project != nil && wf.ProjectID != "" && wf.ProjectID != m.project.ID {
					continue
				}
				sb.WriteString(fmt.Sprintf("  %s  %s\n", wf.ID, wf.Name))
				shown++
			}
			if shown == 0 {
				sb.WriteString("  (none — configure in Desktop → Settings)\n")
			}
		}
		sb.WriteString("Usage: /flow <flowRef>  (Tab / ↑↓ to pick while typing)")
		m.addMessage("system", sb.String(), "")
		return m, nil

	case RunStartedMsg:
		handle := msg.Handle
		m.runHandle = &handle
		if handle.StepID != "" {
			m.stepID = handle.StepID
		}
		if handle.ProviderKey != "" && m.provider == "" {
			m.provider = handle.ProviderKey
		}
		m.connStatus = ConnRunning
		m.statusMsg = fmt.Sprintf("run %s • %s", shortID(handle.RunID), handle.ProviderKey)
		prompt := m.pendingPrompt
		m.pendingPrompt = ""
		if prompt != "" {
			return m, m.cmdSendTurn(prompt)
		}
		if m.cfg.Print {
			return m, m.cmdStreamHeadless(handle.RunID, handle.LastEventSeq)
		}
		return m, nil

	case RunResumedMsg:
		handle := msg.Handle
		m.runHandle = &handle
		if handle.StepID != "" {
			m.stepID = handle.StepID
		}
		m.connStatus = ConnRunning
		m.addMessage("system", fmt.Sprintf("Resumed run %s", shortID(handle.RunID)), "")
		return m, m.cmdStreamRun(handle.RunID, handle.LastEventSeq)

	case EventMsg:
		return m.handleEvent(msg.Ev)

	case TokenUsageMsg:
		m.lastTokens = msg.Usage
		return m, nil

	case AgentGraphMsg:
		if msg.Graph != nil {
			m.agentRuns = msg.Graph.Runs
			if m.focusedAgentIdx >= len(m.agentRuns) {
				m.focusedAgentIdx = 0
			}
		}
		return m, nil

	case TurnDoneMsg:
		m.connStatus = ConnIdle
		m.statusMsg = "done"
		if msg.FinalMsg != "" && !(isStepCompleteStub(msg.FinalMsg) && m.hasAssistantContent()) {
			m.ensureAssistantMessage(msg.FinalMsg)
		}
		if m.cfg.Print {
			m.headlessOutput = msg.FinalMsg
			return m, tea.Quit
		}
		return m, nil

	case turnFinishedMsg:
		m.connStatus = ConnIdle
		m.statusMsg = "done"
		if msg.Usage != nil {
			m.lastTokens = msg.Usage
			if msg.Usage.ModelContextWindow != nil && *msg.Usage.ModelContextWindow > 0 {
				m.modelContextWin = *msg.Usage.ModelContextWindow
			}
		}
		if msg.FinalMsg != "" && !(isStepCompleteStub(msg.FinalMsg) && m.hasAssistantContent()) {
			m.ensureAssistantMessage(msg.FinalMsg)
		}
		if m.cfg.Print {
			m.headlessOutput = msg.FinalMsg
			return m, tea.Quit
		}
		return m, nil

	case TurnFailedMsg:
		m.connStatus = ConnError
		m.statusMsg = "turn failed: " + msg.Reason
		m.addMessage("system", "Turn failed: "+msg.Reason, "error")
		if m.cfg.Print {
			return m, tea.Quit
		}
		return m, nil

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *AppModel) handleEvent(ev client.ProviderEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "message_delta":
		m.appendAssistantDelta(ev.Text)

	case "message_completed":
		m.appendAssistantDelta(ev.Text)

	case "turn_completed":
		// Prefer already-streamed assistant text; ignore step-complete stubs.
		if ev.FinalMessage != "" && !m.hasAssistantContent() && !isStepCompleteStub(ev.FinalMessage) {
			m.ensureAssistantMessage(ev.FinalMessage)
		}
		m.connStatus = ConnIdle
		m.statusMsg = "done"
		final := chooseAssistantFinal(m.lastAssistantText(), ev.FinalMessage)
		return m, func() tea.Msg { return TurnDoneMsg{FinalMsg: final} }

	case "turn_failed":
		m.connStatus = ConnError
		m.statusMsg = "turn failed: " + ev.Error
		m.addMessage("system", "Turn failed: "+ev.Error, "error")
		if m.cfg.Print {
			return m, func() tea.Msg { return TurnFailedMsg{Reason: ev.Error} }
		}

	case "turn_started":
		m.connStatus = ConnRunning

	case "tool_started":
		if ev.ToolName != "" {
			m.addMessage("tool", fmt.Sprintf("→ %s", ev.ToolName), "tool")
		}

	case "tool_completed":
		// no separate rendering needed

	case "permission_required":
		if ev.ApprovalID != "" {
			m.approval = &ApprovalState{
				ID:    ev.ApprovalID,
				RunID: ev.WorkflowRunID,
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			if m.effectiveYolo() {
				return m, m.cmdAutoApprove(ev.ApprovalID, ev.WorkflowRunID)
			}
			m.addMessage("system", fmt.Sprintf("[APPROVAL] %s — type /approve or /deny", ev.ApprovalID), "approval")
		}

	case "user_question_required":
		if ev.QuestionID != "" {
			opts := make([]map[string]string, 0, len(ev.Options))
			for _, o := range ev.Options {
				opts = append(opts, o)
			}
			m.question = &QuestionState{
				ID:      ev.QuestionID,
				Prompt:  ev.Prompt,
				Options: opts,
				RunID:   ev.WorkflowRunID,
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "question"
			m.addMessage("system", fmt.Sprintf("[QUESTION] %s", ev.Prompt), "question")
			if len(opts) > 0 {
				var sb strings.Builder
				for i, o := range opts {
					sb.WriteString(fmt.Sprintf("  %d) %s\n", i+1, o["label"]))
				}
				m.addMessage("system", sb.String(), "")
			}
		}

	case "flow_gate_violation":
		m.gate = &GateState{
			Options:        ev.GateOptions,
			RegressedTests: ev.GateRegressedTests,
			RunID:          ev.WorkflowRunID,
		}
		m.connStatus = ConnWaiting
		m.statusMsg = "gate"
		m.addMessage("system", buildGateMessage(ev.GateOptions, ev.GateRegressedTests), "gate")

	case "token_usage_updated":
		if ev.TokenUsage != nil {
			m.lastTokens = ev.TokenUsage
			if ev.TokenUsage.ModelContextWindow != nil && *ev.TokenUsage.ModelContextWindow > 0 {
				m.modelContextWin = *ev.TokenUsage.ModelContextWindow
			}
		}

	case "agent_graph_updated":
		if ev.AgentGraph != nil {
			m.agentRuns = ev.AgentGraph.Runs
			if m.focusedAgentIdx >= len(m.agentRuns) {
				m.focusedAgentIdx = 0
			}
		}
		if m.agentsFocus {
			m.addMessage("system", fmt.Sprintf("[agents] %d agents active", len(m.agentRuns)), "")
		}
	}

	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While session/project catalogs load: chat prompts are blocked.
	// Slash commands (starting with '/') stay available so /help /clear /login work.
	if m.sessionLoading && !m.allowsKeyWhileLoading(msg) {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEscape:
			return m, func() tea.Msg { return QuitMsg{} }
		default:
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.runHandle != nil {
			_ = m.client.Interrupt(context.Background(), m.runHandle.RunID)
		}
		return m, func() tea.Msg { return QuitMsg{} }

	case tea.KeyF2:
		if m.authPhase != AuthNone {
			return m, nil
		}
		m.sessionPanel.Collapsed = !m.sessionPanel.Collapsed
		return m, nil

	case tea.KeyEscape:
		if m.authPhase != AuthNone {
			m.authPhase = AuthNone
			m.authEmail = ""
			m.inputValue = ""
			m.addMessage("system", "Login cancelled.", "")
			return m, nil
		}
		if m.runHandle != nil {
			_ = m.client.Interrupt(context.Background(), m.runHandle.RunID)
		}
		return m, func() tea.Msg { return QuitMsg{} }

	case tea.KeyTab:
		if m.authPhase != AuthNone {
			return m, nil
		}
		if items := m.collectSuggestions(); len(items) > 0 {
			m.applySuggestion(items)
			return m, m.cmdMaybePrefetchPickers()
		}
		// Cycle through agent runs when agents focus is active.
		if m.agentsFocus && len(m.agentRuns) > 0 {
			m.focusedAgentIdx = (m.focusedAgentIdx + 1) % len(m.agentRuns)
			agent := m.agentRuns[m.focusedAgentIdx]
			m.addMessage("system", fmt.Sprintf("Focused agent: %s (%s)", agent.AgentName, agent.Status), "")
		}
		return m, nil

	case tea.KeyUp:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx - 1 + n) % n
				return m, nil
			}
		}

	case tea.KeyDown:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx + 1) % n
				return m, nil
			}
		}

	case tea.KeyEnter:
		// When a suggestion list is open, Enter accepts the highlighted row
		// (same target as Tab) and runs it — no need for Tab-then-Enter.
		if m.authPhase == AuthNone {
			if items := m.collectSuggestions(); len(items) > 0 {
				it := items[m.suggIdx%len(items)]
				if cmd := suggestionAcceptValue(it); cmd != "" {
					// Action rows (e.g. /provider → connect) only expand the next picker.
					if it.kind == "provider-action" {
						m.inputValue = cmd
						m.suggIdx = 0
						return m, m.cmdMaybePrefetchPickers()
					}
					// Slash picks are allowed even while a turn is in progress.
					if m.sendBlocked() && !strings.HasPrefix(cmd, "/") {
						return m, nil
					}
					m.inputValue = ""
					m.suggIdx = 0
					return m.processInput(cmd)
				}
				// Placeholder (loading / no match): if the user already typed an
				// arg (e.g. /model custom-id), run the typed line instead of trapping Enter.
				if typed := strings.TrimSpace(m.inputValue); len(strings.Fields(typed)) >= 2 {
					if m.sendBlocked() && !strings.HasPrefix(typed, "/") {
						return m, nil
					}
					m.inputValue = ""
					m.suggIdx = 0
					return m.processInput(typed)
				}
				return m, nil
			}
		}
		input := strings.TrimSpace(m.inputValue)
		if input == "" {
			return m, nil
		}
		// In-progress turn: keep the draft next question; Enter does nothing.
		if m.sendBlocked() && !strings.HasPrefix(input, "/") && m.authPhase == AuthNone {
			m.statusMsg = "in progress — Enter disabled (keep typing)"
			return m, nil
		}
		m.inputValue = ""
		// Bare "/" with no suggestion rows left → help.
		if input == "/" {
			return m.handleSlashCommand("/help")
		}
		return m.processInput(input)

	case tea.KeyBackspace:
		if len(m.inputValue) > 0 {
			runes := []rune(m.inputValue)
			m.inputValue = string(runes[:len(runes)-1])
			m.suggIdx = 0
		}
		return m, m.cmdMaybePrefetchPickers()

	case tea.KeyRunes:
		m.inputValue += string(msg.Runes)
		m.suggIdx = 0
		return m, m.cmdMaybePrefetchPickers()
	}
	return m, nil
}

func (m *AppModel) collectSuggestions() []suggestItem {
	projectID := ""
	if m.project != nil {
		projectID = m.project.ID
	}
	if flows := filterFlowSuggestions(m.inputValue, m.flowBuiltins, m.flowWorkflows, projectID); len(flows) > 0 {
		return flows
	}
	// While `/flow ` is open but catalog still loading, show a placeholder row.
	if ok, _ := parseFlowArgPrefix(m.inputValue); ok {
		if len(m.flowBuiltins) == 0 && len(m.flowWorkflows) == 0 {
			return []suggestItem{{value: "", detail: "loading flows…", kind: "flow"}}
		}
		return []suggestItem{{value: "", detail: "(no matching flows)", kind: "flow"}}
	}
	if chats := filterHistorySuggestions(m.inputValue, m.chatList); len(chats) > 0 {
		return chats
	}
	if cmd, _, ok := parseChatOpenArgPrefix(m.inputValue); ok {
		if len(m.chatList) == 0 {
			return []suggestItem{{value: "", detail: "loading chats…", kind: "history", slash: cmd}}
		}
		return []suggestItem{{value: "", detail: "(no matching chats)", kind: "history", slash: cmd}}
	}
	if providerSugg := filterProviderSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider); len(providerSugg) > 0 {
		return providerSugg
	}
	if mode, _, ok := parseProviderPicker(m.inputValue); ok {
		kind := "provider"
		switch mode {
		case "connect":
			kind = "provider-connect"
		case "install":
			kind = "provider-install"
		}
		if len(m.providers) == 0 {
			return []suggestItem{{value: "", detail: "loading providers…", kind: kind}}
		}
		return []suggestItem{{value: "", detail: "(no matching providers)", kind: kind}}
	}
	models := modelsForProvider(m.providers, m.provider)
	if modelSugg := filterModelSuggestions(m.inputValue, models, m.model); len(modelSugg) > 0 {
		return modelSugg
	}
	if ok, _ := parseSlashArgPrefix(m.inputValue, "/model"); ok {
		if len(models) == 0 {
			return []suggestItem{{value: "", detail: "no models — set /provider first", kind: "model"}}
		}
		return []suggestItem{{value: "", detail: "(no matching models)", kind: "model"}}
	}
	if reasonSugg := filterReasoningSuggestions(m.inputValue, m.reasoningEffort); len(reasonSugg) > 0 {
		return reasonSugg
	}
	if ok, _ := parseSlashArgPrefix(m.inputValue, "/reasoning"); ok {
		return []suggestItem{{value: "", detail: "(no matching effort)", kind: "reasoning"}}
	}
	cmds := filterSlashSuggestions(m.inputValue)
	out := make([]suggestItem, 0, len(cmds))
	for _, sc := range cmds {
		out = append(out, suggestItem{value: sc.name, detail: sc.description, kind: "cmd"})
	}
	return out
}

func (m *AppModel) applySuggestion(items []suggestItem) {
	if len(items) == 0 {
		return
	}
	idx := m.suggIdx % len(items)
	it := items[idx]
	if cmd := suggestionAcceptValue(it); cmd == "" {
		return
	} else if it.kind == "flow" || it.kind == "history" || it.kind == "model" || it.kind == "reasoning" || it.kind == "provider" || it.kind == "provider-connect" || it.kind == "provider-action" {
		m.inputValue = cmd
	} else {
		// Tab fills the command token and leaves a trailing space for args.
		m.inputValue = it.value + " "
	}
	m.suggIdx = (idx + 1) % len(items)
}

// suggestionAcceptValue returns the slash line to run for Enter (or empty if not actionable).
func suggestionAcceptValue(it suggestItem) string {
	switch it.kind {
	case "flow":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/flow " + it.value
	case "history":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		slash := it.slash
		if slash == "" {
			slash = "/history"
		}
		return slash + " " + it.value
	case "model":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/model " + it.value
	case "reasoning":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/reasoning " + it.value
	case "provider":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/provider " + it.value
	case "provider-action":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		// Trailing space opens the connect-mode provider picker.
		return "/provider " + strings.TrimSpace(it.value) + " "
	case "provider-connect":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/provider connect " + it.value
	case "provider-install":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/provider install " + it.value
	case "cmd":
		return strings.TrimSpace(it.value)
	default:
		return strings.TrimSpace(it.value)
	}
}

func (m *AppModel) allowsKeyWhileLoading(msg tea.KeyMsg) bool {
	if strings.HasPrefix(m.inputValue, "/") {
		return true
	}
	if msg.Type == tea.KeyRunes && len(m.inputValue) == 0 && len(msg.Runes) > 0 && msg.Runes[0] == '/' {
		return true
	}
	return false
}

// sendBlocked is true while a start/turn is in flight (chat Enter must not send).
// Gate/approval/question answers remain allowed.
func (m *AppModel) sendBlocked() bool {
	if m.gate != nil || m.question != nil || m.approval != nil {
		return false
	}
	if m.pendingPrompt != "" {
		return true
	}
	switch m.connStatus {
	case ConnRunning, ConnWaiting:
		return true
	default:
		return false
	}
}

func (m *AppModel) processInput(input string) (tea.Model, tea.Cmd) {
	if m.sessionLoading && !strings.HasPrefix(strings.TrimSpace(input), "/") {
		m.addMessage("system", "Still loading session — chat is disabled until ready.", "error")
		return m, nil
	}
	if m.authPhase == AuthEmail {
		email := strings.TrimSpace(input)
		if email == "" || !strings.Contains(email, "@") {
			m.addMessage("system", "Enter a valid email (or Esc to cancel).", "error")
			return m, nil
		}
		m.authEmail = email
		m.authPhase = AuthPassword
		m.addMessage("system", "Password:", "")
		return m, nil
	}
	if m.authPhase == AuthPassword {
		if input == "" {
			m.addMessage("system", "Password required (or Esc to cancel).", "error")
			return m, nil
		}
		email := m.authEmail
		m.authPhase = AuthNone
		m.addMessage("system", "Signing in…", "")
		return m, m.cmdLogin(email, input)
	}
	if strings.HasPrefix(input, "/") {
		return m.handleSlashCommand(input)
	}

	// Gate blocked loop: if gate is active, input is treated as a gate decision.
	if m.gate != nil {
		return m.handleGateInput(input)
	}

	if !m.canSend() {
		m.addMessage("system", "Send is disabled while focused on a child agent. Focus main to send.", "error")
		return m, nil
	}

	if m.sendBlocked() {
		m.addMessage("system", "A turn is already in progress — wait for it to finish (draft kept in the input).", "error")
		return m, nil
	}

	// After session defaults load, refuse empty provider (avoids silent fake Codex).
	if m.sessionDefaultsLoaded && strings.TrimSpace(m.provider) == "" {
		m.addMessage("system", "No provider selected — use /provider <key> (then /model).", "error")
		return m, nil
	}

	m.addMessage("user", input, "")
	m.connStatus = ConnRunning
	m.statusMsg = "sending..."

	// First message: start run, then send turn (desktop sendPrompt parity).
	if m.runHandle == nil {
		m.pendingPrompt = input
		startMsg := fmt.Sprintf("Starting chat run (%s · %s)…", m.provider, orDash(m.model))
		if m.launch.IsCatalogWorkflow() {
			startMsg = fmt.Sprintf("Starting workflow run (%s · %s · %s)…", m.launch.StatusLabel(), m.provider, orDash(m.model))
		} else if m.launch.IsBuiltin() {
			startMsg = fmt.Sprintf("Starting chat run with %s (%s · %s)…", m.launch.StatusLabel(), m.provider, orDash(m.model))
		}
		m.addMessage("system", startMsg, "")
		return m, m.cmdStartRun()
	}
	return m, m.cmdSendTurn(input)
}

// handleGateInput interprets user input when a flow_gate_violation is pending.
// Accepts option number (1-based) or option name (case-insensitive).
func (m *AppModel) handleGateInput(input string) (tea.Model, tea.Cmd) {
	lowInput := strings.TrimSpace(strings.ToLower(input))
	for i, opt := range m.gate.Options {
		if lowInput == fmt.Sprintf("%d", i+1) || strings.EqualFold(lowInput, opt) {
			runID := m.gate.RunID
			m.gate = nil
			m.connStatus = ConnRunning
			return m, m.cmdSubmitGateDecision(runID, opt)
		}
	}
	opts := strings.Join(m.gate.Options, ", ")
	m.addMessage("system", fmt.Sprintf("Gate options: %s (enter number or name)", opts), "gate")
	return m, nil
}

// handleSlashCommand dispatches slash commands.
func (m *AppModel) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/help":
		var sb strings.Builder
		sb.WriteString("Available commands:\n")
		for _, sc := range knownSlashCommands {
			sb.WriteString(fmt.Sprintf("  %-14s %s\n", sc.name, sc.description))
		}
		m.addMessage("system", sb.String(), "")

	case "/clear":
		m.messages = nil
		m.addMessage("system", "Conversation cleared.", "")

	case "/exit", "/quit":
		return m, func() tea.Msg { return QuitMsg{} }

	case "/yolo":
		if m.mode != ModeChat || m.launch.IsArmed() {
			m.addMessage("system", "YOLO is auto-on in flow mode. Switch to /chat to toggle.", "")
			break
		}
		m.yolo = !m.yolo
		state := "OFF"
		if m.yolo {
			state = "ON"
		}
		m.addMessage("system", fmt.Sprintf("YOLO mode: %s", state), "")
		// Grok: sync server-side posture after the local toggle.
		if strings.ToLower(m.provider) == "grok" {
			return m, m.cmdGrokYoloPosture(m.yolo)
		}

	case "/agents":
		m.agentsFocus = !m.agentsFocus
		state := "OFF"
		if m.agentsFocus {
			state = "ON"
		}
		m.addMessage("system", fmt.Sprintf("Agents focus: %s", state), "")

	case "/agent":
		if len(args) == 0 {
			m.addMessage("system", "Usage: /agent <name>", "")
		} else {
			name := strings.Join(args, " ")
			for i, a := range m.agentRuns {
				if strings.EqualFold(a.AgentName, name) {
					m.focusedAgentIdx = i
					m.agentsFocus = true
					m.addMessage("system", fmt.Sprintf("Focused agent: %s", a.AgentName), "")
					return m, nil
				}
			}
			m.addMessage("system", fmt.Sprintf("Agent %q not found.", name), "error")
		}

	case "/flow":
		if len(args) == 0 || args[0] == "list" {
			return m, m.cmdListFlows()
		}
		if m.runHandle != nil {
			m.addMessage("system", "Cannot change flow after a run has started. Use /new first.", "error")
			break
		}
		projectID := ""
		if m.project != nil {
			projectID = m.project.ID
		}
		arm, err := resolveFlowLaunch(m.flowBuiltins, m.flowWorkflows, projectID, strings.Join(args, " "))
		if err != nil {
			m.addMessage("system", err.Error(), "error")
			break
		}
		m.launch = arm
		m.mode = ModeFlow
		m.firstTurnPending = arm.IsBuiltin()
		if arm.IsBuiltin() {
			m.addMessage("system", fmt.Sprintf(
				"Flow armed: %s (builtin · %s). Send a prompt to start.",
				arm.StatusLabel(), arm.FlowRef,
			), "")
		} else {
			m.addMessage("system", fmt.Sprintf(
				"Flow armed: %s (catalog workflow). Send a prompt to start.",
				arm.StatusLabel(),
			), "")
		}

	case "/chat":
		m.mode = ModeChat
		m.launch = LaunchArm{}
		m.firstTurnPending = false
		m.addMessage("system", "Switched to chat mode.", "")

	case "/skill":
		if len(args) == 0 {
			if len(m.selectedSkills) == 0 {
				m.addMessage("system", "No skills selected. Use /skill <name> to toggle.", "")
			} else {
				names := make([]string, len(m.selectedSkills))
				for i, s := range m.selectedSkills {
					names[i] = s.Name
				}
				m.addMessage("system", fmt.Sprintf("Active skills: %s", strings.Join(names, ", ")), "")
			}
		} else {
			name := strings.Join(args, " ")
			// Toggle: remove if present, add if absent.
			found := false
			for i, s := range m.selectedSkills {
				if strings.EqualFold(s.Name, name) {
					m.selectedSkills = append(m.selectedSkills[:i], m.selectedSkills[i+1:]...)
					m.addMessage("system", fmt.Sprintf("Skill removed: %s", name), "")
					found = true
					break
				}
			}
			if !found {
				m.selectedSkills = append(m.selectedSkills, client.SkillSelection{Name: name, Source: "user"})
				m.addMessage("system", fmt.Sprintf("Skill added: %s", name), "")
			}
		}

	case "/image":
		if len(args) == 0 {
			m.addMessage("system", "Usage: /image <path> — attach image to next turn (codex/claude only).", "")
		} else {
			path := strings.Join(args, " ")
			if !client.SupportsImages(m.provider) {
				m.addMessage("system", fmt.Sprintf("Provider %q does not support images (codex/claude only).", m.provider), "error")
			} else if len(m.pendingAttach) >= 6 {
				m.addMessage("system", "Maximum 6 images per turn.", "error")
			} else {
				attachments, err := client.ValidateAttachments([]string{path}, m.provider)
				if err != nil {
					m.addMessage("system", err.Error(), "error")
				} else {
					m.pendingAttach = append(m.pendingAttach, attachments...)
					m.addMessage("system", fmt.Sprintf("Attached image: %s (%d pending)", filepath.Base(path), len(m.pendingAttach)), "")
				}
			}
		}

	case "/provider":
		if len(args) > 0 {
			switch strings.ToLower(args[0]) {
			case "connect", "config":
				key := strings.TrimSpace(m.provider)
				if len(args) > 1 {
					key = strings.TrimSpace(args[1])
				}
				if key == "" {
					m.addMessage("system", "Usage: /provider connect <key> — or type /provider connect  then ↑↓ Tab Enter\n(Desktop Settings → Connect New Account parity)", "")
					break
				}
				if p := findProvider(m.providers, key); p != nil && !p.Installed {
					m.addMessage("system", fmt.Sprintf(
						"Provider %s is not installed. Run /provider install %s first (Desktop disables Connect until installed).",
						key, key,
					), "error")
					break
				}
				m.addMessage("system", fmt.Sprintf(
					"Connecting provider %s… Complete login in the opened terminal/browser (same as Desktop Settings).",
					key,
				), "")
				return m, m.cmdConnectProvider(key)
			case "install":
				key := strings.TrimSpace(m.provider)
				if len(args) > 1 {
					key = strings.TrimSpace(args[1])
				}
				if key == "" {
					m.addMessage("system", "Usage: /provider install <key> — or type /provider install  then ↑↓ Tab Enter\n(Runner POST /providers/install; Desktop Settings Install button is Gemini-only in UI, API supports all)", "")
					break
				}
				if p := findProvider(m.providers, key); p != nil && p.Installed {
					m.addMessage("system", fmt.Sprintf("Provider %s is already installed.", key), "")
					break
				}
				m.addMessage("system", fmt.Sprintf("Installing provider CLI %s… (may take a minute)", key), "")
				return m, m.cmdInstallProvider(key)
			}
		}
		if len(args) == 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Current provider: %s\n", orDash(m.provider)))
			sb.WriteString(fmt.Sprintf("Current model:    %s\n", orDash(m.model)))
			if len(m.providers) == 0 {
				sb.WriteString("No provider catalog loaded yet. Wait for connect, or restart chat.")
			} else {
				sb.WriteString("Providers (Desktop ChatInput readiness: installed + active connected account):\n")
				for _, p := range m.providers {
					mark := " "
					if strings.EqualFold(p.Key, m.provider) {
						mark = "*"
					}
					code, status := providerReadiness(p, m.providerAccounts)
					readyMark := " "
					if code == "ready" {
						readyMark = "+"
					} else {
						readyMark = "-"
					}
					sb.WriteString(fmt.Sprintf("  %s%s %s  %s\n", mark, readyMark, p.Key, status))
				}
				sb.WriteString("Pick: /provider  · Connect: /provider connect  · Install: /provider install  (↑↓ Tab Enter)")
			}
			m.addMessage("system", sb.String(), "")
		} else if m.runHandle != nil {
			m.addMessage("system", "Cannot change provider after a run has started. Use /new to start fresh.", "error")
		} else {
			want := strings.ToLower(args[0])
			found := false
			var selected *client.Provider
			for i := range m.providers {
				if strings.EqualFold(m.providers[i].Key, want) {
					m.provider = m.providers[i].Key
					models := modelsForProvider(m.providers, m.provider)
					if len(models) > 0 {
						m.model = models[0]
					} else {
						m.model = ""
					}
					selected = &m.providers[i]
					found = true
					break
				}
			}
			if !found && len(m.providers) > 0 {
				m.addMessage("system", fmt.Sprintf("Unknown provider %q. Try /provider to list.", args[0]), "error")
			} else {
				if !found {
					m.provider = args[0]
				}
				m.bindActiveAccountForProvider()
				msg := fmt.Sprintf("Provider set to: %s · model: %s", m.provider, orDash(m.model))
				if acc := m.activeProviderAccountLabel(); acc != "" {
					msg += " · account: " + acc
				}
				if selected != nil {
					if code, status := providerReadiness(*selected, m.providerAccounts); code != "ready" {
						msg += "\nNot ready (" + status + ") — Desktop disables this provider chip until ready."
					}
				}
				m.addMessage("system", msg, "")
				m.refreshSessionPanel()
			}
		}

	case "/model":
		if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Provider: %s\n", orDash(m.provider)))
			sb.WriteString(fmt.Sprintf("Current model: %s\n", orDash(m.model)))
			models := modelsForProvider(m.providers, m.provider)
			if len(models) == 0 {
				sb.WriteString("No models listed for this provider. Check Desktop provider install, or /provider first.")
			} else {
				sb.WriteString("Available models:\n")
				for _, id := range models {
					mark := " "
					if id == m.model {
						mark = "*"
					}
					sb.WriteString(fmt.Sprintf("  %s %s\n", mark, id))
				}
				sb.WriteString("Pick: type /model  then ↑↓ · Tab · Enter")
			}
			m.addMessage("system", sb.String(), "")
		} else {
			want := strings.Join(args, " ")
			models := modelsForProvider(m.providers, m.provider)
			if len(models) > 0 {
				ok := false
				for _, id := range models {
					if strings.EqualFold(id, want) {
						m.model = id
						ok = true
						break
					}
				}
				if !ok {
					m.addMessage("system", fmt.Sprintf("Model %q not in catalog for %s. Try /model to list.", want, orDash(m.provider)), "error")
					break
				}
			} else {
				m.model = want
			}
			m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
			m.addMessage("system", fmt.Sprintf("Model set to: %s", m.model), "")
			m.refreshSessionPanel()
		}

	case "/reasoning":
		if len(args) == 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Current reasoning effort: %s\n", orDash(m.reasoningEffort)))
			sb.WriteString("Options: high · medium · low\n")
			sb.WriteString("Pick: type /reasoning  then ↑↓ · Tab · Enter")
			m.addMessage("system", sb.String(), "")
		} else {
			effort := strings.ToLower(args[0])
			switch effort {
			case "high", "medium", "low", "":
				m.reasoningEffort = effort
				m.addMessage("system", fmt.Sprintf("Reasoning effort set to: %s", effort), "")
			default:
				m.addMessage("system", "Reasoning effort must be high, medium, or low.", "error")
			}
		}

	case "/new":
		m.runHandle = nil
		m.stepID = ""
		m.pendingPrompt = ""
		m.messages = nil
		m.connStatus = ConnIdle
		m.statusMsg = "ready"
		m.selectedSkills = nil
		m.pendingAttach = nil
		m.gate = nil
		m.approval = nil
		m.question = nil
		// Keep armed flow so /new can restart the same target; clear with /chat.
		m.firstTurnPending = m.launch.IsBuiltin()
		m.addMessage("system", "New conversation started.", "")

	case "/step":
		if len(args) == 0 {
			m.addMessage("system", "Usage: /step <step-id>", "")
		} else {
			m.addMessage("system", fmt.Sprintf("Step selected: %s (will apply on next run start)", args[0]), "")
		}

	case "/status":
		auth := "(not signed in)"
		if m.signedInEmail != "" {
			auth = m.signedInEmail
		} else if m.authNeedLogin {
			auth = "SIGN-IN REQUIRED — /login"
		}
		m.refreshSessionPanel()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Status: %s | Mode: %s | %s | Provider: %s | Model: %s | Auth: %s\n",
			m.connStatus, m.mode, m.yoloStatusLabel(), m.provider, m.model, auth))
		sb.WriteString("Active provider account: " + orDash(m.activeProviderAccountLabel()) + "\n")
		for _, line := range m.sessionPanel.lines() {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("(F2 or /info toggles the top-right session panel)")
		m.addMessage("system", strings.TrimRight(sb.String(), "\n"), "")

	case "/info":
		m.sessionPanel.Collapsed = !m.sessionPanel.Collapsed
		m.refreshSessionPanel()
		state := "expanded"
		if m.sessionPanel.Collapsed {
			state = "collapsed"
		}
		m.addMessage("system", "Session panel "+state+" (F2 or /info to toggle).", "")

	case "/login":
		return m.beginLogin(args)

	case "/settings", "/setting":
		host := strings.TrimSpace(m.cfg.DesktopHost)
		if host == "" {
			host = "127.0.0.1"
		}
		port := m.cfg.DesktopPort
		if port <= 0 {
			port = 5173
		}
		m.addMessage("system", settingsBridgeBlurb(fmt.Sprintf("http://%s:%d", host, port)), "")
		return m, m.cmdEnsureDesktop()

	case "/history", "/chats", "/open", "/resume":
		if len(args) > 0 {
			runID, err := resolveChatOpenTarget(args, m.chatList)
			if err != nil {
				m.addMessage("system", err.Error()+" — type /history  (or /open / /resume ) for the picker", "error")
				break
			}
			m.addMessage("system", fmt.Sprintf("Opening chat %s…", runID), "")
			m.connStatus = ConnConnecting
			m.statusMsg = "opening chat…"
			return m, m.cmdOpenChat(runID)
		}
		m.addMessage("system", "Loading chat history…", "")
		return m, m.cmdListChats()

	case "/approve":
		if m.approval != nil {
			id := m.approval.ID
			m.approval = nil
			return m, m.cmdApprove(id, "approve")
		}
		m.addMessage("system", "No pending approval.", "")

	case "/deny":
		if m.approval != nil {
			id := m.approval.ID
			m.approval = nil
			return m, m.cmdApprove(id, "deny")
		}
		m.addMessage("system", "No pending approval.", "")

	default:
		m.addMessage("system", fmt.Sprintf("Unknown command: %s. Type /help for list.", cmd), "error")
	}

	return m, nil
}

// ---- View -------------------------------------------------------------------

func (m *AppModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	var sugg []suggestItem
	if m.authPhase == AuthNone && (!m.sessionLoading || strings.HasPrefix(m.inputValue, "/")) {
		sugg = m.collectSuggestions()
	}
	suggLines := 0
	if len(sugg) > 0 {
		limit := suggestionVisibleLimit(sugg)
		if len(sugg) < limit {
			limit = len(sugg)
		}
		suggLines = limit + 2 // header + hint
	}
	bannerLines := 0
	if m.sessionLoading {
		bannerLines = loadingBannerHeight
	} else if m.authNeedLogin && m.authPhase == AuthNone {
		bannerLines = 1
	}
	panelLines := m.renderSessionPanelOverlay()
	panelH := len(panelLines)

	// 2 status lines + 1 input row.
	messagesHeight := m.height - 4 - suggLines - bannerLines - panelH
	if messagesHeight < 1 {
		messagesHeight = 1
	}

	for _, l := range panelLines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}

	lines := m.renderMessages()
	if len(lines) > messagesHeight {
		lines = lines[len(lines)-messagesHeight:]
	}
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}

	for i := len(lines); i < messagesHeight; i++ {
		sb.WriteString("\n")
	}

	if m.sessionLoading {
		for _, line := range strings.Split(m.loadingBannerText(), "\n") {
			sb.WriteString(styleLoading.Render(line))
			sb.WriteString("\n")
		}
	} else if m.authNeedLogin && m.authPhase == AuthNone {
		banner := "SIGN IN REQUIRED — Desktop is signed out. Type /login (or /login you@email.com)"
		if !m.asciiMode {
			banner = "! " + banner
		}
		sb.WriteString(styleError.Render(banner))
		sb.WriteString("\n")
	}
	sb.WriteString(m.renderStatusLine())
	sb.WriteString("\n")
	if len(sugg) > 0 {
		sb.WriteString(m.renderSuggestions(sugg))
		sb.WriteString("\n")
	}
	sb.WriteString(m.renderInputLine())

	return sb.String()
}

func (m *AppModel) loadingBannerText() string {
	return renderFlowpilotLoader(m.loadingFrame, m.asciiMode)
}

func suggestionVisibleLimit(sugg []suggestItem) int {
	if len(sugg) > 0 && sugg[0].kind == "history" {
		return 12
	}
	return 8
}

func (m *AppModel) renderSuggestions(sugg []suggestItem) string {
	limit := suggestionVisibleLimit(sugg)
	kind := "commands"
	if len(sugg) > 0 {
		switch sugg[0].kind {
		case "flow":
			kind = "flows"
		case "history":
			kind = "chats"
		case "model":
			kind = "models"
		case "reasoning":
			kind = "reasoning"
		case "provider", "provider-connect", "provider-action", "provider-install":
			kind = "providers"
		}
	}
	sel := 0
	if len(sugg) > 0 {
		sel = m.suggIdx % len(sugg)
	}
	start, end := suggestionWindow(len(sugg), sel, limit)
	var sb strings.Builder
	sb.WriteString(styleSuggest.Render(kind + ":"))
	for i := start; i < end; i++ {
		sb.WriteString("\n")
		label := sugg[i].value
		if label == "" {
			label = "—"
		}
		line := fmt.Sprintf("  %-28s %s", label, sugg[i].detail)
		if i == sel {
			marker := "> "
			if !m.asciiMode {
				marker = "▸ "
			}
			sb.WriteString(styleSuggestSel.Render(marker + strings.TrimLeft(line, " ")))
		} else {
			sb.WriteString(styleSuggest.Render(line))
		}
	}
	above := start
	below := len(sugg) - end
	sb.WriteString("\n")
	if above > 0 || below > 0 {
		sb.WriteString(styleSuggest.Render(fmt.Sprintf("  … %d above · %d below (↑↓ scrolls · Tab · Enter)  [%d/%d]", above, below, sel+1, len(sugg))))
	} else {
		sb.WriteString(styleSuggest.Render("  (↑↓ · Tab fill · Enter run)"))
	}
	return sb.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func formatMissingProjectHelp(path string, projects []client.Project) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project path %s is not in the Desktop catalog — chat cannot start without project_id.\n", path))
	if len(projects) == 0 {
		sb.WriteString("Catalog is empty right now (often signed out or Supabase unreachable).\n")
		sb.WriteString("Fix:\n")
		sb.WriteString("  1) Type /login to sign in here (same as Desktop Login screen).\n")
		sb.WriteString("  2) Confirm Desktop can list projects (same runner).\n")
		sb.WriteString("  3) Check .env.dev Supabase URL/key, then restart runner.\n")
		sb.WriteString("  4) Add/open this folder in Desktop → Projects, retry chat-dev.\n")
		return sb.String()
	}
	sb.WriteString("Add/open this folder in Desktop → Projects, then retry.\n")
	sb.WriteString("Known projects:\n")
	limit := 8
	if len(projects) < limit {
		limit = len(projects)
	}
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("  %s  %s\n    %s\n", projects[i].ID, projects[i].Name, projects[i].Path))
	}
	if len(projects) > limit {
		sb.WriteString(fmt.Sprintf("  … %d more\n", len(projects)-limit))
	}
	return sb.String()
}

func (m *AppModel) renderMessages() []string {
	width := m.width
	if width < 20 {
		width = 80
	}
	var lines []string
	for _, msg := range m.messages {
		prefix := ""
		style := styleSystem
		rightAlign := false
		switch msg.Role {
		case "user":
			prefix = "You: "
			style = styleUser
			rightAlign = true
		case "assistant":
			style = styleAssistant
		case "tool":
			style = styleTool
		case "system":
			switch msg.FormatHint {
			case "error":
				style = styleError
			case "gate":
				style = styleGate
			default:
				style = styleSystem
			}
		}
		// Wrap plain text first (ANSI from lipgloss must not be mid-wrapped).
		contentWidth := width
		if rightAlign {
			// User bubbles sit on the right (~70% width) to contrast AI on the left.
			contentWidth = width * 7 / 10
			if contentWidth < 16 {
				contentWidth = width
			}
			if prefix != "" {
				contentWidth -= len([]rune(prefix))
				if contentWidth < 8 {
					contentWidth = width - len([]rune(prefix))
				}
			}
		} else if prefix != "" {
			contentWidth = width - len([]rune(prefix))
			if contentWidth < 8 {
				contentWidth = width
				prefix = ""
			}
		}
		wrapped := wrapText(msg.Content, contentWidth)
		for i, line := range wrapped {
			var rendered string
			if i == 0 && prefix != "" {
				rendered = style.Render(prefix) + style.Render(line)
			} else if prefix != "" {
				pad := strings.Repeat(" ", len([]rune(prefix)))
				rendered = pad + style.Render(line)
			} else {
				rendered = style.Render(line)
			}
			if rightAlign {
				rendered = rightAlignPlain(rendered, width)
			}
			lines = append(lines, rendered)
		}
	}
	return lines
}

func (m *AppModel) renderStatusLine() string {
	sep := " │ "
	if m.asciiMode {
		sep = " | "
	}

	var parts []string

	parts = append(parts, fmt.Sprintf("[%s]", m.mode))
	if m.mode == ModeFlow || m.mode == ModeStep {
		if label := m.launch.StatusLabel(); label != "" {
			// Keep statusline compact for long UUID/refs.
			if len([]rune(label)) > 28 {
				r := []rune(label)
				label = string(r[:25]) + "…"
			}
			parts = append(parts, label)
		}
	}

	// Provider · model (active provider-account label — not Supabase email).
	providerDisplay := m.provider
	if providerDisplay == "" {
		providerDisplay = "default"
	}
	if m.model != "" {
		providerDisplay = fmt.Sprintf("%s · %s", providerDisplay, m.model)
	}
	if acc := m.activeProviderAccountLabel(); acc != "" {
		providerDisplay = fmt.Sprintf("%s (%s)", providerDisplay, acc)
	}
	parts = append(parts, providerDisplay)

	// Always show YOLO; flow/step arms force ON (chat toggle only).
	parts = append(parts, m.yoloStatusLabel())

	// Account rate-limit remaining (desktop parity).
	if lim := formatAccountLimits(m.account); lim != "" {
		parts = append(parts, lim)
	}

	// Focused agent indicator.
	if m.agentsFocus && len(m.agentRuns) > 0 {
		var agentName string
		if m.focusedAgentIdx < len(m.agentRuns) {
			agentName = m.agentRuns[m.focusedAgentIdx].AgentName
		}
		if agentName != "" {
			parts = append(parts, fmt.Sprintf("@%s", agentName))
		}
	}

	// Context window remaining + last turn (desktop ChatInput parity).
	if ctxLine := formatContextLimits(m.lastTokens, m.modelContextWin); ctxLine != "" {
		parts = append(parts, ctxLine)
	}

	// Supabase identity stays out of the statusline; only warn when signed out.
	if m.authNeedLogin {
		parts = append(parts, styleStatusErr.Render("SIGN-IN"))
	}

	// Connection status.
	statusStyle := styleStatus
	switch m.connStatus {
	case ConnRunning, ConnConnecting:
		statusStyle = styleStatusOK
	case ConnError:
		statusStyle = styleStatusErr
	}
	if m.sessionLoading {
		statusStyle = styleStatusOK
	}

	statusStr := m.connStatus.String()
	if m.sessionLoading {
		// Keep "connected" visible for status parity; append loading marker.
		if m.statusMsg != "" {
			statusStr = m.statusMsg + " · loading…"
		} else {
			statusStr = "loading…"
		}
	} else if m.statusMsg != "" {
		statusStr = m.statusMsg
	}
	parts = append(parts, statusStyle.Render(statusStr))

	line1 := styleStatus.Render(strings.Join(parts, sep))
	line2 := styleStatus.Render(m.projectStatusLabel())
	return line1 + "\n" + line2
}

func (m *AppModel) renderInputLine() string {
	if m.sessionLoading && !strings.HasPrefix(m.inputValue, "/") {
		frames := []string{"|", "/", "-", "\\"}
		spin := frames[m.loadingFrame%len(frames)]
		return styleLoading.Render(fmt.Sprintf(" %s please wait… (chat disabled) ", spin))
	}
	var prefix, body string
	switch m.authPhase {
	case AuthEmail:
		prefix = " email "
		body = m.inputValue
	case AuthPassword:
		prefix = " password "
		body = strings.Repeat("*", len([]rune(m.inputValue)))
	default:
		switch {
		case m.sessionLoading:
			prefix = " wait "
		case m.sendBlocked():
			prefix = " next "
		default:
			prefix = " chat "
		}
		body = m.inputValue
	}
	caret := " "
	if m.cursorOn {
		if m.asciiMode {
			caret = styleCursor.Render("_")
		} else {
			caret = styleCursor.Render("▌")
		}
	}
	// Stroke frame (┃ label │ text) — no full-width background wash.
	left := styleInputStroke.Render("┃")
	mid := styleInputStroke.Render("│")
	label := stylePromptFocus.Render(strings.TrimSpace(prefix))
	return left + " " + label + " " + mid + styleInputFocus.Render(" "+body+caret)
}

// ---- Helpers ----------------------------------------------------------------

func (m *AppModel) addMessage(role, content, hint string) {
	m.messages = append(m.messages, ChatMessage{
		Role:       role,
		Content:    content,
		FormatHint: hint,
	})
}

func (m *AppModel) appendAssistantDelta(text string) {
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content += text
	} else {
		m.addMessage("assistant", text, "")
	}
}

func (m *AppModel) ensureAssistantMessage(text string) {
	if isStepCompleteStub(text) && m.hasAssistantContent() {
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		if m.messages[len(m.messages)-1].Content == "" {
			m.messages[len(m.messages)-1].Content = text
		}
	} else if text != "" {
		m.addMessage("assistant", text, "")
	}
}

func (m *AppModel) hasAssistantContent() bool {
	return strings.TrimSpace(m.lastAssistantText()) != ""
}

func (m *AppModel) lastAssistantText() string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "assistant" {
			return m.messages[i].Content
		}
	}
	return ""
}

func buildGateMessage(opts []string, regressed []string) string {
	var sb strings.Builder
	sb.WriteString("[GATE] Flow gate triggered.\n")
	if len(regressed) > 0 {
		sb.WriteString(fmt.Sprintf("  Regressed tests: %s\n", strings.Join(regressed, ", ")))
	}
	sb.WriteString("  Options: ")
	sb.WriteString(strings.Join(opts, ", "))
	return sb.String()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// isLegacyConsole returns true when stdout appears to be a legacy Windows
// console that does not support full ANSI / Unicode box-drawing characters.
func isLegacyConsole() bool {
	if os.Getenv("TERM") == "dumb" {
		return true
	}
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") != "" ||
			os.Getenv("TERM_PROGRAM") != "" ||
			os.Getenv("ConEmuANSI") == "ON" ||
			os.Getenv("TERM") != "" {
			return false
		}
		return true
	}
	return false
}

func (m *AppModel) applyAuthNotice(catalogErr string) {
	session, _, err := client.LoadDesktopAuthSession()
	if err == nil && session != nil {
		switch {
		case session.Email != nil && strings.TrimSpace(*session.Email) != "":
			m.signedInEmail = strings.TrimSpace(*session.Email)
		case strings.TrimSpace(session.UserID) != "":
			m.signedInEmail = strings.TrimSpace(session.UserID)
		}
		m.authNeedLogin = false
		return
	}

	// No Desktop session file → same state as Desktop Login screen.
	m.authNeedLogin = true
	m.signedInEmail = ""
	reason := "No Desktop Supabase session on this PC (same as Desktop Login screen)."
	if catalogErr != "" {
		reason = "Catalog unavailable and no Desktop session — sign in to continue."
	} else if m.cfg.ProjectPath != "" && m.project == nil && len(m.projects) == 0 {
		reason = "Project catalog empty and no Desktop session — sign in, then retry."
	}
	m.addMessage("system", reason+"\nType /login (or /login you@email.com). Esc cancels the prompt.", "error")
}

func (m *AppModel) beginLogin(args []string) (tea.Model, tea.Cmd) {
	switch len(args) {
	case 0:
		m.authPhase = AuthEmail
		m.authEmail = ""
		m.addMessage("system", "Supabase login — enter email (Esc to cancel):", "")
		return m, nil
	case 1:
		email := strings.TrimSpace(args[0])
		if !strings.Contains(email, "@") {
			m.addMessage("system", "Usage: /login [email] [password]", "error")
			return m, nil
		}
		m.authEmail = email
		m.authPhase = AuthPassword
		m.addMessage("system", "Password:", "")
		return m, nil
	default:
		email := strings.TrimSpace(args[0])
		password := strings.Join(args[1:], " ")
		m.authPhase = AuthNone
		m.addMessage("system", "Signing in…", "")
		return m, m.cmdLogin(email, password)
	}
}

// ---- Commands (Tea.Cmd factories) -------------------------------------------

func (m *AppModel) cmdConnect() tea.Cmd {
	return func() tea.Msg {
		return ConnectedMsg{RunnerURL: m.runnerURL}
	}
}

func (m *AppModel) cmdLogin(email, password string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		res, err := cl.LoginSupabase(ctx, email, password)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("login failed: %w", err)}
		}
		out := LoginResultMsg{
			Email:  strings.TrimSpace(res.Email),
			UserID: res.UserID,
		}
		if out.Email == "" {
			out.Email = strings.TrimSpace(email)
		}
		clientKey := ""
		if cfg, cfgErr := cl.GetSupabaseConfig(ctx); cfgErr == nil {
			clientKey = client.ClientKeyForConfig(cfg)
		}
		emailCopy := out.Email
		path, perr := client.PersistDesktopAuthSession(client.DesktopAuthSession{
			ClientKey:    clientKey,
			AccessToken:  res.AccessToken,
			RefreshToken: res.RefreshToken,
			UserID:       res.UserID,
			Email:        &emailCopy,
		})
		if perr != nil {
			out.PersistError = perr.Error()
		} else {
			out.PersistedTo = path
		}
		return out
	}
}

func (m *AppModel) cmdLoadSessionDefaults() tea.Cmd {
	runnerURL := m.runnerURL
	flagProvider := m.cfg.Provider
	flagModel := m.cfg.Model
	projectPath := m.cfg.ProjectPath
	// Prefer in-memory values already set on the model (from New).
	if flagProvider == "" {
		flagProvider = m.provider
	}
	if flagModel == "" {
		flagModel = m.model
	}
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		accounts, _ := cl.ListProviderAccounts(ctx)
		providers, _ := cl.ListProviders(ctx)
		provider, model, label := pickActiveSessionDefaults(flagProvider, flagModel, accounts, providers)
		var (
			projects   []client.Project
			project    *client.Project
			account    *client.ProviderAccountSummary
			catalogErr string
		)
		if ps, err := cl.ListProjects(ctx); err == nil {
			projects = ps
			project = matchProjectByPath(ps, projectPath)
		} else {
			catalogErr = err.Error()
		}
		for i := range accounts {
			if accounts[i].IsActive && (provider == "" || strings.EqualFold(accounts[i].ProviderKey, provider)) {
				a := accounts[i]
				account = &a
				if label == "" {
					label = a.DisplayLabel
				}
				break
			}
		}
		if account == nil {
			for i := range accounts {
				if strings.EqualFold(accounts[i].ProviderKey, provider) {
					a := accounts[i]
					account = &a
					break
				}
			}
		}
		return SessionDefaultsMsg{
			Provider:         provider,
			Model:            model,
			AccountLabel:     label,
			Providers:        providers,
			ProviderAccounts: accounts,
			Projects:         projects,
			Project:          project,
			Account:          account,
			CatalogErr:       catalogErr,
		}
	}
}

func (m *AppModel) cmdListFlows() tea.Cmd {
	return m.cmdFetchFlows(false)
}

func (m *AppModel) cmdPrefetchFlows() tea.Cmd {
	return m.cmdFetchFlows(true)
}

func (m *AppModel) cmdFetchFlows(silent bool) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		builtins, _ := cl.ListBuiltinOrchestrationOptions(ctx, "bug")
		workflows, err := cl.ListWorkflows(ctx)
		msg := FlowListMsg{Builtins: builtins, Workflows: workflows, Silent: silent}
		if err != nil {
			msg.CatalogErr = err.Error()
		}
		return msg
	}
}

func (m *AppModel) cmdMaybePrefetchFlows() tea.Cmd {
	ok, _ := parseFlowArgPrefix(m.inputValue)
	if !ok && !strings.EqualFold(strings.TrimSpace(m.inputValue), "/flow") {
		return nil
	}
	if len(m.flowBuiltins) > 0 || len(m.flowWorkflows) > 0 {
		return nil
	}
	return m.cmdPrefetchFlows()
}

func (m *AppModel) cmdMaybePrefetchPickers() tea.Cmd {
	return tea.Batch(m.cmdMaybePrefetchFlows(), m.cmdMaybePrefetchHistory())
}

func (m *AppModel) cmdStartRun() tea.Cmd {
	cfg := m.cfg
	runnerURL := m.runnerURL
	yolo := m.effectiveYolo()
	provider := m.provider
	model := m.model
	reasoning := m.reasoningEffort
	launch := m.launch
	projectID := ""
	cwd := cfg.ProjectPath
	if m.project != nil {
		projectID = m.project.ID
		if m.project.Path != "" {
			cwd = m.project.Path
		}
	}
	knownProjects := m.projects

	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		if projectID == "" {
			projects := knownProjects
			if len(projects) == 0 {
				if ps, err := cl.ListProjects(ctx); err == nil {
					projects = ps
				}
			}
			if matched := matchProjectByPath(projects, cwd); matched != nil {
				projectID = matched.ID
				if matched.Path != "" {
					cwd = matched.Path
				}
			}
			if projectID == "" {
				return ErrMsg{Err: fmt.Errorf("%s", formatMissingProjectHelp(cwd, projects))}
			}
		}
		if strings.TrimSpace(provider) == "" {
			return ErrMsg{Err: fmt.Errorf("provider required — set /provider before chatting")}
		}
		input := launch.ToStartRunInput(projectID, provider, model, reasoning, cwd, yolo)
		handle, err := cl.StartRun(ctx, input)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("start run: %w", err)}
		}
		return RunStartedMsg{Handle: handle}
	}
}

func (m *AppModel) cmdResume(runID string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		handle, err := cl.ResumeRun(context.Background(), runID)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return RunResumedMsg{Handle: handle}
	}
}

func (m *AppModel) cmdSendTurn(prompt string) tea.Cmd {
	if m.runHandle == nil {
		return nil
	}
	runID := m.runHandle.RunID
	stepID := m.stepID
	if stepID == "" {
		stepID = m.runHandle.StepID
	}
	runnerURL := m.runnerURL
	yolo := m.effectiveYolo()
	model := m.model
	reasoningEffort := m.reasoningEffort
	skills := m.selectedSkills
	attachments := m.pendingAttach
	// Snapshot + consume first-turn builtin extras before the async cmd runs.
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
	// Catalog workflow / plain chat: never send flowRef/subMode on the turn.

	// Clear pending attachments (consumed by this turn).
	m.pendingAttach = nil

	return func() tea.Msg {
		cl := client.New(runnerURL)
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

		// Per-turn model/YOLO overrides match Desktop normal_chat only (BUG-063).
		if !catalogWorkflow {
			yoloCopy := yolo
			turnIn.YoloMode = &yoloCopy
			modelCopy := model
			turnIn.Model = &modelCopy
		} else {
			turnIn.ReasoningEffort = ""
		}

		evCh, errCh := cl.SendTurn(ctx, turnIn)

		var finalMsg strings.Builder
		var completed string
		var usage *client.TokenUsageSnapshot
		for ev := range evCh {
			switch ev.Type {
			case "message_delta":
				finalMsg.WriteString(ev.Text)
			case "token_usage_updated":
				if ev.TokenUsage != nil {
					usage = ev.TokenUsage
				}
			case "turn_completed":
				if ev.FinalMessage != "" {
					completed = ev.FinalMessage
				}
				if ev.TokenUsage != nil {
					usage = ev.TokenUsage
				}
			case "turn_failed":
				return TurnFailedMsg{Reason: ev.Error}
			}
		}
		if err := <-errCh; err != nil {
			return ErrMsg{Err: fmt.Errorf("send turn: %w", err)}
		}
		out := chooseAssistantFinal(finalMsg.String(), completed)
		return turnFinishedMsg{FinalMsg: out, Usage: usage}
	}
}

// turnFinishedMsg is an internal completion message that also carries usage.
type turnFinishedMsg struct {
	FinalMsg string
	Usage    *client.TokenUsageSnapshot
}

func (m *AppModel) cmdStreamRun(runID string, afterSeq int64) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		ch := cl.StreamRun(ctx, runID, afterSeq)
		for ev := range ch {
			if ev.Type == "turn_completed" {
				return TurnDoneMsg{FinalMsg: ev.FinalMessage}
			}
		}
		return TurnDoneMsg{}
	}
}

func (m *AppModel) cmdStreamHeadless(runID string, afterSeq int64) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx := context.Background()
		ch := cl.StreamRun(ctx, runID, afterSeq)
		var finalMsg string
		for ev := range ch {
			if ev.FinalMessage != "" {
				finalMsg = ev.FinalMessage
			}
			if ev.Type == "turn_failed" {
				return TurnFailedMsg{Reason: ev.Error}
			}
			if ev.Type == "turn_completed" {
				break
			}
		}
		return TurnDoneMsg{FinalMsg: finalMsg}
	}
}

func (m *AppModel) cmdAutoApprove(approvalID, runID string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitApproval(context.Background(), approvalID, "approve", false); err != nil {
			return ErrMsg{Err: err}
		}
		return nil
	}
}

func (m *AppModel) cmdApprove(approvalID, decision string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitApproval(context.Background(), approvalID, decision, false); err != nil {
			return ErrMsg{Err: err}
		}
		return nil
	}
}

func (m *AppModel) cmdSubmitGateDecision(runID, decision string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitGateDecision(context.Background(), runID, decision); err != nil {
			return ErrMsg{Err: err}
		}
		return nil
	}
}

func (m *AppModel) cmdGrokYoloPosture(yolo bool) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.ApplyGrokYoloPosture(context.Background(), yolo); err != nil {
			return ErrMsg{Err: err}
		}
		return nil
	}
}

// ---- Run (entrypoint) -------------------------------------------------------

// Run starts the Bubble Tea program. In headless/print mode it runs
// the model loop and prints the final response to stdout, then exits.
func Run(cfg config.ChatConfig, runnerURL string) error {
	m := New(cfg, runnerURL)

	if cfg.Print {
		return runHeadless(m, cfg.Prompt)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// runHeadless runs without a TUI: sends one prompt and prints the response.
// Exits with non-zero when turn_failed or a gate blocks completion (CP-56 §7).
func runHeadless(m *AppModel, prompt string) error {
	ctx := context.Background()
	cl := m.client

	input := client.StartRunInput{
		ProviderKey:     m.cfg.Provider,
		Model:           m.cfg.Model,
		ReasoningEffort: m.cfg.ReasoningEffort,
		YoloMode:        m.yolo,
		Cwd:             m.cfg.ProjectPath,
		ChatMode:        "normal_chat",
	}
	if input.Cwd == "" {
		projects, err := cl.ListProjects(ctx)
		if err == nil && len(projects) > 0 {
			input.ProjectID = projects[0].ID
			input.Cwd = projects[0].Path
		}
	}

	var handle client.RunHandle
	var err error
	if m.cfg.ResumeRunID != "" {
		handle, err = cl.ResumeRun(ctx, m.cfg.ResumeRunID)
	} else {
		handle, err = cl.StartRun(ctx, input)
	}
	if err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	yoloCopy := m.yolo
	turnIn := client.TurnInput{
		RunID:           handle.RunID,
		Prompt:          prompt,
		ReasoningEffort: m.cfg.ReasoningEffort,
		YoloMode:        &yoloCopy,
	}
	if m.cfg.Model != "" {
		model := m.cfg.Model
		turnIn.Model = &model
	}

	evCh, errCh := cl.SendTurn(ctx, turnIn)

	var finalMsg string
	turnFailed := false
	for ev := range evCh {
		switch ev.Type {
		case "message_delta":
			finalMsg += ev.Text
		case "turn_completed":
			if ev.FinalMessage != "" {
				finalMsg = ev.FinalMessage
			}
		case "turn_failed":
			turnFailed = true
		case "flow_gate_violation":
			// In headless mode, gate violations are fatal.
			return fmt.Errorf("gate violation blocked headless run (options: %s)", strings.Join(ev.GateOptions, ", "))
		}
	}
	if err := <-errCh; err != nil {
		return fmt.Errorf("turn: %w", err)
	}
	if turnFailed {
		return fmt.Errorf("turn failed")
	}

	fmt.Fprintln(os.Stdout, finalMsg)
	return nil
}
