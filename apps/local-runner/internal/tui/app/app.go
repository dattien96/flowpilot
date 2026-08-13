package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// ---- Styles -----------------------------------------------------------------
// Color tokens mirror apps/desktop-flowpilot/src/styles.css :root (Codex-like dark chat).

const (
	colorText       = "#ececec" // --text
	colorTextDim    = "#9b9b9b" // --text-dim
	colorPromptText = "#f3f3f3" // .bubble.prompt
	colorAccent     = "#4c8dff" // --accent
	colorWarn       = "#f0b429" // --warn
	colorAsk        = "#b07cff" // --ask (gate / questions)
	colorOK         = "#3fb950" // --ok
	colorErr        = "#f85149" // --err
	colorBg3        = "#1e1e1e" // --bg-3
	colorCodeBg     = "#252526" // fenced-code panel (lifted vs terminal / --bg-3)
)

var (
	styleUserLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleUser      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPromptText))
	styleAssistant = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleSystem    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleTool      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn))
	styleError     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	styleGate      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAsk))
	styleStatus    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleStatusOK  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))
	styleStatusErr = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	stylePrompt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	// Input stroke frame (Desktop accent / prompt-border — no neon wash).
	stylePromptFocus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleInputFocus  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleInputStroke = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	styleCursor      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorBg3)).Background(lipgloss.Color(colorAccent))
	styleSuggest     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleSuggestSel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)).Underline(true)
	styleLoading     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWarn)).Background(lipgloss.Color(colorBg3))
	styleLink        = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color(colorAccent))
	styleSelect      = lipgloss.NewStyle().Reverse(true)
	styleThinking    = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(colorTextDim))
	// Active workflow step (Desktop timeline “current” accent).
	styleStepRunning = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWarn)).Background(lipgloss.Color(colorBg3))
	styleStepDone    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOK))
	styleStepFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorErr))
)

// ---- New / Init -------------------------------------------------------------

// New creates a new AppModel from ChatConfig and the runner URL.
func New(cfg config.ChatConfig, runnerURL string) *AppModel {
	provider := cfg.Provider
	model := cfg.Model
	reasoning := cfg.ReasoningEffort
	// Restore last TUI selection when flags omit provider/model.
	if saved, _, err := prefs.Load(); err == nil {
		if provider == "" {
			provider = saved.Provider
		}
		if model == "" {
			model = saved.Model
		}
		if reasoning == "" {
			reasoning = saved.ReasoningEffort
		}
	}
	if reasoning == "" {
		reasoning = "medium"
	}
	m := &AppModel{
		cfg:             cfg,
		runnerURL:       runnerURL,
		client:          client.New(runnerURL),
		inputCursor:     -1,
		yolo:            cfg.Yolo,
		provider:        provider,
		model:           model,
		reasoningEffort: reasoning,
		projectPath:     cfg.ProjectPath,
		connStatus:      ConnConnecting,
		statusMsg:       "connecting...",
		width:           80,
		height:          42, // room for /help + rounded input + 6-row status bar
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
		cmds := []tea.Cmd{tickCursor()}
		// While a flow turn/orchestration is live, poll steps-runtime so long
		// silent steps (e.g. grok-context / context.produce) stay visible.
		if m.shouldPollStepsRuntime() {
			m.stepsPollTicks++
			if m.stepsPollTicks%3 == 0 { // ~1.6s
				cmds = append(cmds, m.cmdRefreshStepsRuntime())
			}
		} else {
			m.stepsPollTicks = 0
		}
		return m, tea.Batch(cmds...)

	case ErrMsg:
		m.err = msg.Err
		m.sessionLoading = false
		m.pendingPrompt = ""
		m.connStatus = ConnError
		m.statusMsg = "error"
		errText := msg.Err.Error()
		m.addMessage("system", "Error: "+errText, "error")
		if strings.Contains(errText, "connection refused") || strings.Contains(errText, "dial tcp") ||
			strings.Contains(errText, "connection reset") || strings.Contains(errText, "i/o timeout") ||
			strings.Contains(errText, "dispatch_prepare_failed") {
			m.runHandle = nil
		}
		return m, nil

	case ApprovalResolvedMsg:
		shown := m.approval != nil && (msg.ID == "" || m.approval.ID == msg.ID)
		if shown {
			m.approval = nil
		}
		m.connStatus = ConnRunning
		m.statusMsg = "approved"
		if !shown {
			return m, nil
		}
		label := "Approved."
		if strings.EqualFold(msg.Decision, "deny") {
			label = "Denied."
		}
		m.addMessage("system", label, "")
		return m, nil

	case QuestionResolvedMsg:
		if m.question != nil && (msg.ID == "" || m.question.ID == msg.ID) {
			m.question = nil
		}
		m.connStatus = ConnRunning
		m.statusMsg = "answered"
		m.addMessage("system", "Answered: "+msg.Choice, "question")
		return m, nil

	case StoppedMsg:
		m.clearThinkingPlaceholder()
		m.pendingPrompt = ""
		m.stopOrchestrationStream()
		m.stopFocusStream()
		m.restoreMainTranscript()
		m.connStatus = ConnIdle
		m.statusMsg = "stopped"
		m.addMessage("system", "Stopped.", "")
		return m, nil

	case CopiedMsg:
		if msg.Err != "" {
			m.addMessage("system", "Copy failed: "+msg.Err, "error")
			return m, nil
		}
		m.addMessage("system", "Copied "+msg.Kind+".", "")
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
		firstLoad := !m.sessionDefaultsLoaded
		if m.provider == "" && msg.Provider != "" {
			m.provider = msg.Provider
		}
		if m.model == "" && msg.Model != "" {
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
			if label := strings.TrimSpace(msg.Account.DisplayLabel); label != "" {
				m.accountLabel = label
			}
		}
		if m.reasoningEffort == "" {
			m.reasoningEffort = "medium"
		}
		m.bindActiveAccountForProvider()
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
		if firstLoad {
			persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)
		}
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
		if firstLoad {
			m.addMessage("system", "Ready — type / for commands · F2/click session panel · F3/click skills chip.", "")
		}
		cmds := []tea.Cmd{m.cmdRefreshProjectContext(), m.cmdLoadSkills(false)}
		if firstLoad && m.project != nil && len(m.chatList) == 0 {
			cmds = append(cmds, m.cmdPrefetchChats())
		}
		// Task-291 / CP-56 D-15: --yolo on Grok must hit the same posture
		// endpoint as /yolo and Desktop. Do not rewrite startTurn.
		if firstLoad {
			if c := m.startupGrokYoloPostureCmd(); c != nil {
				cmds = append(cmds, c)
			}
		}
		return m, tea.Batch(cmds...)

	case SkillsListMsg:
		if msg.Err != "" {
			if msg.Show {
				m.addMessage("system", "Skills list failed: "+msg.Err, "error")
			}
			return m, nil
		}
		m.skillsCatalog = msg.Skills
		if msg.Show {
			m.addMessage("system", formatSkillsCatalog(m.skillsCatalog, m.selectedSkills, m.provider), "")
		}
		return m, nil

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

	case ActivatedAccountMsg:
		if msg.Err != nil {
			m.addMessage("system", fmt.Sprintf("Failed to activate account: %v", msg.Err), "error")
			return m, nil
		}
		if msg.Account != nil {
			m.bindActiveAccountForProvider()
			m.addMessage("system", fmt.Sprintf(
				"Activated account %q (%s) for provider %s.",
				msg.Account.DisplayLabel, msg.Account.ID, msg.Account.ProviderKey,
			), "")
			m.refreshSessionPanel()
			return m, m.cmdLoadSessionDefaults()
		}

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
		m.visiblePromptCount = 0
		m.historyLoadedAfterSeq = msg.HistoryLoadedAfterSeq
		m.historyChunkInFlight = false
		m.lastEventSeq = handle.LastEventSeq
		m.viewport.offset = 0
		if len(msg.Messages) > 0 {
			m.messages = append([]ChatMessage(nil), msg.Messages...)
			m.syncVisiblePromptCount()
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

	case HistoryChunkMsg:
		m.historyChunkInFlight = false
		if msg.Err != "" {
			m.addMessage("system", msg.Err, "error")
			return m, nil
		}
		if m.runHandle == nil || m.runHandle.RunID != msg.RunID {
			return m, nil
		}
		if m.historyLoadedAfterSeq > 0 && msg.NewLoadedAfterSeq >= m.historyLoadedAfterSeq {
			return m, nil
		}
		m.historyLoadedAfterSeq = msg.NewLoadedAfterSeq
		if len(msg.Messages) > 0 {
			m.messages = prependReplayMessages(msg.Messages, m.messages)
			m.syncVisiblePromptCount()
			m.visiblePromptCount = expandVisiblePromptCount(m.visiblePromptCount, countUserPrompts(m.messages), chatPromptPageSize)
		}
		m.rowCache = nil
		m.rowCacheSig = 0
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
		m.addMessage("system", fmt.Sprintf("Run %s started — streaming events…", shortID(handle.RunID)), "")
		prompt := m.pendingPrompt
		m.pendingPrompt = ""
		var cmds []tea.Cmd
		if m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep {
			cmds = append(cmds, m.cmdRefreshStepsRuntime())
		}
		if prompt != "" {
			cmds = append(cmds, m.cmdSendTurn(prompt))
		} else if m.cfg.Print {
			cmds = append(cmds, m.cmdStreamHeadless(handle.RunID, handle.LastEventSeq))
		}
		return m, tea.Batch(cmds...)

	case turnStreamOpenedMsg:
		m.turnStream = &turnStreamState{evCh: msg.EvCh, errCh: msg.ErrCh}
		m.connStatus = ConnRunning
		m.statusMsg = "streaming…"
		if strings.EqualFold(strings.TrimSpace(m.reasoningEffort), "high") {
			m.statusMsg = "thinking…"
		}
		return m, m.cmdPollTurnStream()

	case turnStreamEventMsg:
		if msg.Ev.Seq > m.lastEventSeq {
			m.lastEventSeq = msg.Ev.Seq
		}
		m2, cmd := m.handleEvent(msg.Ev)
		am := m2.(*AppModel)
		return am, tea.Batch(cmd, am.cmdPollTurnStream())

	case turnStreamClosedMsg:
		m.turnStream = nil
		if msg.Err != nil {
			m.connStatus = ConnError
			m.statusMsg = "turn failed"
			m.addMessage("system", "Send turn failed: "+msg.Err.Error(), "error")
			m.runHandle = nil
			return m, nil
		}
		if m.connStatus == ConnRunning {
			m.connStatus = ConnIdle
			m.statusMsg = "done"
		}
		cmds := []tea.Cmd{m.cmdRefreshStepsRuntime()}
		// Desktop startOrchestrationStream: keep listening after the turn so
		// hub/child step transitions (and late gate/approval events) surface.
		if m.shouldPollStepsRuntime() && m.orchStream == nil {
			cmds = append(cmds, m.cmdStartOrchestrationStream())
		}
		return m, tea.Batch(cmds...)

	case orchStreamOpenedMsg:
		m.stopOrchestrationStream()
		m.orchStream = &orchStreamState{evCh: msg.EvCh, cancel: msg.Cancel}
		return m, m.cmdPollOrchStream()

	case orchStreamEventMsg:
		if m.orchStream == nil {
			return m, nil
		}
		if msg.Ev.Seq > m.lastEventSeq {
			m.lastEventSeq = msg.Ev.Seq
		}
		if m.viewingChild() {
			switch msg.Ev.Type {
			case "message_delta", "message_completed", "turn_completed", "turn_failed", "tool_started":
				return m, m.cmdPollOrchStream()
			}
		}
		m2, cmd := m.handleEvent(msg.Ev)
		am := m2.(*AppModel)
		return am, tea.Batch(cmd, am.cmdPollOrchStream())

	case focusStreamOpenedMsg:
		if msg.RunID != m.focusRunID {
			if msg.Cancel != nil {
				msg.Cancel()
			}
			return m, nil
		}
		m.stopFocusStream()
		m.focusStream = &orchStreamState{evCh: msg.EvCh, cancel: msg.Cancel}
		return m, m.cmdPollFocusStream()

	case focusStreamEventMsg:
		if m.focusStream == nil {
			return m, nil
		}
		m.handleFocusEvent(msg.Ev)
		return m, m.cmdPollFocusStream()

	case focusStreamClosedMsg:
		m.stopFocusStream()
		return m, nil

	case orchStreamClosedMsg:
		m.stopOrchestrationStream()
		return m, m.cmdRefreshStepsRuntime()

	case StepsRuntimeMsg:
		if m.runHandle == nil || msg.RunID != m.runHandle.RunID {
			return m, nil
		}
		if msg.Err != "" {
			// Soft failure — catalog may not have steps yet.
			return m, nil
		}
		prevSteps := append([]client.WorkflowStepRuntime(nil), m.flowSteps...)
		prevActive := m.flowStepsActive
		m.flowSteps = msg.Steps
		m.flowStepsActive = activeStepName(msg.Steps)
		m.refreshSessionPanel()
		if m.flowStepsActive != "" && (m.connStatus == ConnRunning || m.connStatus == ConnWaiting) {
			m.statusMsg = "step: " + m.flowStepsActive
		}
		// Chat: only the current step line (full list lives in the top-right panel).
		for _, line := range formatStepChatNotices(prevSteps, msg.Steps, prevActive, m.flowStepsActive, m.lastTurnError) {
			m.addMessage("system", line, "steps")
		}
		return m, nil

	case ClipboardPasteMsg:
		if msg.Err != "" && msg.Attachment == nil && msg.Text == "" {
			m.addMessage("system", msg.Err, "error")
			return m, nil
		}
		if msg.Attachment != nil {
			m.pendingAttach = append(m.pendingAttach, *msg.Attachment)
			m.addMessage("system", fmt.Sprintf(
				"Attached image: %s (%d pending) — /image to list, /image open %d to view",
				msg.Attachment.OriginalName, len(m.pendingAttach), len(m.pendingAttach),
			), "")
			return m, nil
		}
		if msg.Text != "" {
			m.insertInputAtCursor(msg.Text)
			return m, nil
		}
		if msg.Err != "" {
			m.addMessage("system", msg.Err, "error")
		}
		return m, nil

	case AttachmentOpenMsg:
		if msg.Err != "" && msg.Path == "" {
			m.addMessage("system", "Open image failed: "+msg.Err, "error")
			return m, nil
		}
		if msg.Path != "" {
			line := "Opened image: " + msg.Path
			if msg.Err != "" {
				line += " (viewer: " + msg.Err + ")"
			}
			m.addMessage("system", line, "")
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
		m.runHandle = nil
		if m.cfg.Print {
			return m, tea.Quit
		}
		return m, nil

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m *AppModel) handleEvent(ev client.ProviderEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "message_delta":
		m.appendAssistantDelta(ev.Text)
		if m.connStatus == ConnRunning {
			m.statusMsg = "streaming…"
		}

	case "message_completed":
		m.appendAssistantDelta(ev.Text)

	case "turn_completed":
		// Prefer already-streamed assistant text; ignore step-complete stubs.
		// run-92955: a prior turn's assistant text made hasAssistantContent()
		// true, so thinking… on a follow-up was never replaced when tools
		// arrived after the placeholder. Always fill/remove thinking first.
		if m.thinkingIndex() >= 0 {
			fill := strings.TrimSpace(ev.FinalMessage)
			if fill == "" || isStepCompleteStub(fill) {
				fill = strings.TrimSpace(m.lastAssistantText())
			}
			m.ensureAssistantMessage(fill)
		} else if ev.FinalMessage != "" && !m.hasAssistantContent() && !isStepCompleteStub(ev.FinalMessage) {
			m.ensureAssistantMessage(ev.FinalMessage)
		}
		if m.shouldPollStepsRuntime() && (m.orchStream != nil || m.flowHasActiveAgents()) {
			m.connStatus = ConnWaiting
			m.statusMsg = "flow running…"
			return m, m.cmdRefreshStepsRuntime()
		}
		m.connStatus = ConnIdle
		m.statusMsg = "done"
		final := chooseAssistantFinal(m.lastAssistantText(), ev.FinalMessage)
		return m, func() tea.Msg { return TurnDoneMsg{FinalMsg: final} }

	case "turn_failed":
		m.connStatus = ConnError
		m.statusMsg = "turn failed: " + ev.Error
		if strings.TrimSpace(ev.Error) != "" {
			m.lastTurnError = strings.TrimSpace(ev.Error)
		}
		m.addMessage("system", "Turn failed: "+ev.Error, "error")
		if m.cfg.Print {
			return m, func() tea.Msg { return TurnFailedMsg{Reason: ev.Error} }
		}

	case "turn_started":
		m.connStatus = ConnRunning
		m.statusMsg = "turn running…"
		if m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep {
			return m, m.cmdRefreshStepsRuntime()
		}

	case "tool_started":
		if ev.ToolName != "" {
			m.addMessage("tool", fmt.Sprintf("→ %s", ev.ToolName), "tool")
		}

	case "tool_completed":
		// no separate rendering needed

	case "permission_required":
		if ev.ApprovalID != "" {
			if m.effectiveYolo() {
				// Runner should auto-approve under YOLO; if a card still arrives,
				// resolve it without showing the gate (Task-291).
				m.connStatus = ConnRunning
				m.statusMsg = "auto-approved"
				return m, m.cmdAutoApprove(ev.ApprovalID, ev.WorkflowRunID)
			}
			m.approval = &ApprovalState{
				ID:    ev.ApprovalID,
				RunID: ev.WorkflowRunID,
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			m.addMessage("system", formatApprovalWaitingLine(ev.ApprovalID, m.asciiMode), "approval")
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
			m.addMessage("system", formatQuestionMessage(ev.Prompt, opts), "question")
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
		// Desktop refreshes the step timeline on agent_graph_updated.
		return m, m.cmdRefreshStepsRuntime()
	}

	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While session/project catalogs load: chat prompts are blocked.
	// Slash commands (starting with '/') stay available so /help /clear /login work.
	if m.sessionLoading && !m.allowsKeyWhileLoading(msg) {
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, m.cmdShutdownAndQuit()
		default:
			return m, nil
		}
	}

	if m.authPhase == AuthNone && (isPromptNewlineKey(msg) || isModifiedEnterNewline(msg)) {
		m.inputValue += "\n"
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.turnIsActive() {
			return m, m.cmdStopTurn()
		}
		m.quitting = true
		return m, m.cmdShutdownAndQuit()

	case tea.KeyF2:
		if m.authPhase != AuthNone {
			return m, nil
		}
		m.sessionPanel.Collapsed = !m.sessionPanel.Collapsed
		return m, nil

	case tea.KeyF3:
		if m.authPhase != AuthNone {
			return m, nil
		}
		if len(attachedSkillNames(m.selectedSkills)) == 0 {
			return m, nil
		}
		m.statusSkillsExpanded = !m.statusSkillsExpanded
		return m, nil

	case tea.KeyF4:
		if m.authPhase != AuthNone {
			return m, nil
		}
		m.statusDetailsCollapsed = !m.statusDetailsCollapsed
		return m, nil

	case tea.KeyCtrlV:
		if m.authPhase != AuthNone {
			return m, nil
		}
		// Prefer image clipboard; falls back to text. On Windows Terminal, Ctrl+V
		// is often stolen for text-only paste — use /image paste or Alt+V then.
		return m, m.cmdClipboardPaste()

	case tea.KeyEscape:
		if m.authPhase != AuthNone {
			m.authPhase = AuthNone
			m.authEmail = ""
			m.inputValue = ""
			m.addMessage("system", "Login cancelled.", "")
			return m, nil
		}
		if !m.mouseSel.empty() {
			m.mouseSel = mouseSelect{}
			m.statusMsg = "selection cleared"
			return m, nil
		}
		if m.inputValue != "" {
			m.inputValue = ""
			m.suggIdx = 0
			m.statusMsg = "prompt cleared"
			return m, nil
		}
		m.statusMsg = "Ctrl-C or /exit to quit"
		return m, nil

	case tea.KeyTab:
		if m.authPhase != AuthNone {
			return m, nil
		}
		if items := m.collectSuggestions(); len(items) > 0 {
			it := items[m.suggIdx%len(items)]
			if it.kind == "skill" && strings.TrimSpace(it.value) != "" {
				m.toggleSkillByNameQuiet(it.value)
				m.retargetSkillSuggestion(it.value)
				return m, nil
			}
			m.applySuggestion(items)
			return m, m.cmdMaybePrefetchPickers()
		}
		if len(m.agentRuns) > 0 {
			runs := orderAgentsMainFirst(m.agentRuns)
			m.focusedAgentIdx = (m.focusedAgentIdx + 1) % len(runs)
			agent := runs[m.focusedAgentIdx]
			name := agent.AgentName
			if name == "" {
				name = agent.RunID
			}
			m.addMessage("system", fmt.Sprintf("Viewing agent: %s (%s)", name, agent.Status), "")
			return m, m.cmdFocusAgent(agent.RunID)
		}
		return m, nil

	case tea.KeyUp:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx - 1 + n) % n
				return m, nil
			}
			m.scrollTranscript(1)
			return m, nil
		}

	case tea.KeyDown:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx + 1) % n
				return m, nil
			}
			m.scrollTranscript(-1)
			return m, nil
		}

	case tea.KeyPgUp:
		if m.authPhase == AuthNone {
			m.scrollTranscript(m.pageScrollAmount())
			return m, nil
		}

	case tea.KeyPgDown:
		if m.authPhase == AuthNone {
			m.scrollTranscript(-m.pageScrollAmount())
			return m, nil
		}

	case tea.KeyEnter:
		// When a suggestion list is open, Enter accepts the highlighted row
		// (same target as Tab) and runs it — except skills: Tab ticks, Enter applies.
		if m.authPhase == AuthNone {
			if items := m.collectSuggestions(); len(items) > 0 {
				it := items[m.suggIdx%len(items)]
				if it.kind == "skill" {
					// Tab ticks; Enter applies (closes picker, keeps the tick set).
					m.inputValue = ""
					m.suggIdx = 0
					return m, nil
				}
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
			if m.approval == nil || !isApprovalDecisionInput(input) {
				m.statusMsg = "in progress — Enter disabled (keep typing)"
				return m, nil
			}
		}
		m.inputValue = ""
		// Bare "/" with no suggestion rows left → help.
		if input == "/" {
			return m.handleSlashCommand("/help")
		}
		return m.processInput(input)

	case tea.KeyLeft:
		m.moveInputCursor(-1)
		return m, nil

	case tea.KeyRight:
		m.moveInputCursor(1)
		return m, nil

	case tea.KeyHome, tea.KeyCtrlA:
		m.inputCursor = 0
		return m, nil

	case tea.KeyEnd, tea.KeyCtrlE:
		m.inputCursor = -1
		return m, nil

	case tea.KeyBackspace:
		m.deleteInputBeforeCursor()
		m.suggIdx = 0
		return m, m.cmdMaybePrefetchPickers()

	case tea.KeySpace, tea.KeyRunes:
		if msg.Type == tea.KeySpace {
			m.insertInputAtCursor(" ")
		} else {
			m.insertInputAtCursor(string(msg.Runes))
		}
		m.suggIdx = 0
		return m, m.cmdMaybePrefetchPickers()
	}
	// Alt+V / ctrl+shift+v: image paste when the terminal steals Ctrl+V.
	if m.authPhase == AuthNone {
		switch msg.String() {
		case "alt+v", "ctrl+shift+v":
			return m, m.cmdClipboardPaste()
		}
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
	if skillSugg := filterSkillSuggestions(m.inputValue, m.skillsCatalog, m.selectedSkills); len(skillSugg) > 0 {
		return skillSugg
	}
	if ok, _ := parseSlashArgPrefix(m.inputValue, "/skill"); ok {
		if len(m.skillsCatalog) == 0 && len(m.selectedSkills) == 0 {
			return []suggestItem{{value: "", detail: "loading skills…", kind: "skill"}}
		}
		return []suggestItem{{value: "", detail: "(no matching skills)", kind: "skill"}}
	}
	if ok, _ := parseSlashArgPrefix(m.inputValue, "/s"); ok {
		if len(m.skillsCatalog) == 0 && len(m.selectedSkills) == 0 {
			return []suggestItem{{value: "", detail: "loading skills…", kind: "skill"}}
		}
		return []suggestItem{{value: "", detail: "(no matching skills)", kind: "skill"}}
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
	} else if it.kind == "flow" || it.kind == "history" || it.kind == "model" || it.kind == "reasoning" || it.kind == "provider" || it.kind == "provider-connect" || it.kind == "provider-action" || it.kind == "provider-install" || it.kind == "provider-account" || it.kind == "skill" {
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
	case "skill":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/skill " + it.value
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
	case "provider-account":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/provider account " + it.value
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
// Gate/question answers remain allowed. Pending approval blocks chat turns
// (those 409) — use /approve, /deny, or the clickable chips.
func (m *AppModel) sendBlocked() bool {
	if m.gate != nil || m.question != nil {
		return false
	}
	if m.approval != nil {
		return true
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

func (m *AppModel) turnIsActive() bool {
	if m.pendingPrompt != "" {
		return true
	}
	if m.orchStream != nil || m.flowHasActiveAgents() {
		return true
	}
	if m.runHandle == nil {
		return false
	}
	switch m.connStatus {
	case ConnRunning, ConnWaiting, ConnConnecting:
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

	if m.approval != nil {
		if isApprovalDecisionInput(input) {
			return m.submitPendingApproval(approvalDecisionOf(input))
		}
		m.addMessage("system", "Pending approval — click Approve or Deny (or type them).", "approval")
		return m, nil
	}

	if m.question != nil {
		return m.submitQuestionAnswer(input)
	}

	// Gate blocked loop: if gate is active, input is treated as a gate decision.
	if m.gate != nil {
		return m.handleGateInput(input)
	}

	if !m.canSend() {
		m.addMessage("system", "Child transcript is read-only. Return to main (/agent main).", "error")
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

	m.viewport.offset = 0
	m.addMessage("user", input, "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.connStatus = ConnRunning
	m.statusMsg = "thinking…"

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

// dispatchImageCommand handles /image [paste|open|clear|list|<path>].
// Reserved subcommands must never fall through to os.ReadFile (that produced
// "read paste: open paste: invalid argument" when paste was treated as a path).
func (m *AppModel) dispatchImageCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.addMessage("system", formatPendingAttachments(m.pendingAttach)+
			"\nTip: Alt+V or /image paste (Windows Terminal often steals Ctrl+V). Images: codex/claude only (grok Vision=false).", "")
		return m, nil
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "list", "ls":
		m.addMessage("system", formatPendingAttachments(m.pendingAttach), "")
		return m, nil
	case "paste", "clip", "clipboard":
		return m, m.cmdClipboardPaste()
	case "clear":
		m.pendingAttach = nil
		m.addMessage("system", "Cleared pending images.", "")
		return m, nil
	case "open", "view", "show":
		idx := 1
		if len(args) > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil {
				idx = n
			} else {
				m.addMessage("system", "Usage: /image open <n>  (n = 1-based index from /image list)", "error")
				return m, nil
			}
		}
		return m, m.cmdOpenPendingAttachment(idx)
	}

	path := strings.TrimSpace(strings.Join(args, " "))
	if path == "" {
		return m, nil
	}
	// Belt-and-suspenders: never open reserved words as files.
	switch strings.ToLower(path) {
	case "paste", "clip", "clipboard", "list", "ls", "clear", "open", "view", "show":
		return m, m.cmdClipboardPaste()
	}
	if !client.SupportsImages(m.provider) {
		m.addMessage("system", client.ImagesUnsupportedReason(m.provider), "error")
		return m, nil
	}
	if len(m.pendingAttach) >= 6 {
		m.addMessage("system", "Maximum 6 images per turn.", "error")
		return m, nil
	}
	attachments, err := client.ValidateAttachments([]string{path}, m.provider)
	if err != nil {
		m.addMessage("system", err.Error(), "error")
		return m, nil
	}
	m.pendingAttach = append(m.pendingAttach, attachments...)
	m.addMessage("system", fmt.Sprintf("Attached image: %s (%d pending) — /image open %d to view",
		filepath.Base(path), len(m.pendingAttach), len(m.pendingAttach)), "")
	return m, nil
}

// handleSlashCommand dispatches slash commands.
func (m *AppModel) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])
	cmd = "/" + strings.TrimLeft(cmd, "/")
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
		m.viewport.offset = 0
		m.addMessage("system", "Conversation cleared.", "")

	case "/exit", "/quit":
		m.quitting = true
		return m, m.cmdShutdownAndQuit()

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
		if !m.agentsFocus {
			m.restoreMainTranscript()
			break
		}
		runs := orderAgentsMainFirst(m.agentRuns)
		if len(runs) == 0 {
			m.addMessage("system", "No agents yet. Tab / /agent once children spawn.", "")
			break
		}
		var b strings.Builder
		b.WriteString("Agents (Tab cycles, /agent <name> opens transcript):\n")
		for _, r := range runs {
			cur := ""
			if r.RunID == m.focusRunID || (m.focusRunID == "" && (strings.EqualFold(r.Role, "main") || r.RunID == m.mainRunID())) {
				cur = " *"
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s%s\n", r.AgentName, r.Status, r.RunID, cur))
		}
		m.addMessage("system", strings.TrimRight(b.String(), "\n"), "")

	case "/agent":
		target := "main"
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		runID, name, ok := m.resolveAgentFocusTarget(target)
		if !ok {
			m.addMessage("system", fmt.Sprintf("Agent %q not found. Try /agents.", target), "error")
			break
		}
		m.addMessage("system", fmt.Sprintf("Viewing agent: %s", name), "")
		return m, m.cmdFocusAgent(runID)

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

	case "/skill", "/s":
		if len(args) == 0 || (len(args) == 1 && (strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls"))) {
			// Desktop ChatInput skill picker dump — selected first, then catalog.
			m.addMessage("system", formatSkillsCatalog(m.skillsCatalog, m.selectedSkills, m.provider), "")
			showAfter := len(m.skillsCatalog) == 0
			return m, m.cmdLoadSkills(showAfter)
		}
		name := strings.Join(args, " ")
		if strings.EqualFold(name, "clear") {
			m.selectedSkills = nil
			m.addMessage("system", "Cleared selected skills.", "")
			break
		}
		m.toggleSkillByName(name)

	case "/image":
		return m.dispatchImageCommand(args)

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
			case "account", "switch", "activate", "acc":
				if len(args) < 2 {
					var sb strings.Builder
					sb.WriteString("Usage: /provider account <account-id>\n")
					sb.WriteString("Configured provider accounts:\n")
					for _, acc := range m.providerAccounts {
						activeMark := " "
						if acc.IsActive {
							activeMark = "*"
						}
						pathLabel := formatAccountPathLabel(acc.HomePath)
						pathStr := ""
						if pathLabel != "" {
							pathStr = fmt.Sprintf(" · path: %s", pathLabel)
						}
						sb.WriteString(fmt.Sprintf("  %s [%s] %s (%s)%s · id: %s\n", activeMark, acc.ProviderKey, acc.DisplayLabel, acc.AuthStatus, pathStr, acc.ID))
					}
					m.addMessage("system", sb.String(), "")
					break
				}
				accountID := strings.TrimSpace(args[1])
				m.addMessage("system", fmt.Sprintf("Switching active account to %s…", accountID), "")
				return m, m.cmdActivateAccount(accountID)
			}
		}
		if len(args) == 0 || (len(args) == 1 && strings.EqualFold(args[0], "list")) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Current provider: %s\n", orDash(m.provider)))
			sb.WriteString(fmt.Sprintf("Current model:    %s\n", orDash(m.model)))
			if len(m.providers) == 0 {
				sb.WriteString("No provider catalog loaded yet. Wait for connect, or restart chat.")
			} else {
				sb.WriteString("Providers & Accounts (Desktop Settings parity):\n")
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
					for _, acc := range m.providerAccounts {
						if strings.EqualFold(acc.ProviderKey, p.Key) {
							accMark := " "
							activeLabel := ""
							if acc.IsActive {
								accMark = "*"
								activeLabel = " [ACTIVE]"
							}
							pathLabel := formatAccountPathLabel(acc.HomePath)
							pathStr := ""
							if pathLabel != "" {
								pathStr = fmt.Sprintf(" · path: %s", pathLabel)
							}
							sb.WriteString(fmt.Sprintf("      %s Account: %s (%s)%s · id: %s%s\n", accMark, acc.DisplayLabel, acc.AuthStatus, pathStr, acc.ID, activeLabel))
						}
					}
				}
				sb.WriteString("Pick provider: /provider <key>  · Switch account: /provider account <account-id>  · Connect: /provider connect  · Install: /provider install")
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
				persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)
				m.skillsCatalog = nil // Desktop reloads skills when provider changes.
				msg := fmt.Sprintf("Provider set to: %s · model: %s (saved for next TUI /new)", m.provider, orDash(m.model))
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
				return m, m.cmdLoadSkills(false)
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
			persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)
			m.addMessage("system", fmt.Sprintf("Model set to: %s (next prompt uses this model)", m.model), "")
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
				persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)
				m.addMessage("system", fmt.Sprintf("Reasoning effort set to: %s (saved)", effort), "")
			default:
				m.addMessage("system", "Reasoning effort must be high, medium, or low.", "error")
			}
		}

	case "/new":
		m.stopOrchestrationStream()
		m.turnStream = nil
		m.runHandle = nil
		m.stepID = ""
		m.pendingPrompt = ""
		m.messages = nil
		m.visiblePromptCount = 0
		m.historyLoadedAfterSeq = 0
		m.mainHistoryLoadedAfterSeq = 0
		m.historyChunkInFlight = false
		m.viewport.offset = 0
		m.connStatus = ConnIdle
		m.statusMsg = "ready"
		m.selectedSkills = nil
		m.pendingAttach = nil
		m.gate = nil
		m.approval = nil
		m.question = nil
		m.flowSteps = nil
		m.flowStepsActive = ""
		m.lastEventSeq = 0
		m.lastTurnError = ""
		m.lastTokens = nil
		// Keep provider/model/reasoning + armed flow (clear flow with /chat).
		m.firstTurnPending = m.launch.IsBuiltin()
		persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)
		m.refreshSessionPanel()
		m.addMessage("system", fmt.Sprintf(
			"New conversation started — provider %s · model %s (latest selection kept).",
			orDash(m.provider), orDash(m.model),
		), "")

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
		sb.WriteString("(F2 or click session panel · F4 or click status row to fold details · F3 or click skills:N chip · /info also toggles the panel)")
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
			return m.submitPendingApproval("approve")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("approve")
		}
		m.addMessage("system", "No pending approval.", "")

	case "/deny":
		if m.approval != nil {
			return m.submitPendingApproval("deny")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("deny")
		}
		m.addMessage("system", "No pending approval.", "")

	case "/stop":
		if !m.turnIsActive() {
			m.addMessage("system", "Nothing to stop.", "")
			break
		}
		return m, m.cmdStopTurn()

	case "/copy":
		kind := "answer"
		if len(args) > 0 {
			kind = strings.ToLower(args[0])
		}
		return m, m.cmdCopyKind(kind)

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

	c := m.tuiChrome()
	var sb strings.Builder

	for _, l := range c.panelLines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}

	lines := m.renderMessages()
	m.clampViewport(len(lines), c.messagesHeight)
	lines = sliceViewport(lines, c.messagesHeight, m.viewport.offset)
	if !m.mouseSel.empty() {
		lines = applyMouseSelection(lines, m.mouseSel, c.panelH)
	}
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}

	for i := len(lines); i < c.messagesHeight; i++ {
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
	sb.WriteString("\n")
	w := m.width
	if w <= 0 {
		w = 80
	}
	if m.asciiMode {
		sb.WriteString(strings.Repeat("-", w))
	} else {
		sb.WriteString(strings.Repeat("─", w))
	}
	sb.WriteString("\n")
	sb.WriteString(c.statusBlock)
	sb.WriteString("\n")
	if len(c.sugg) > 0 {
		sb.WriteString(m.renderSuggestions(c.sugg))
		sb.WriteString("\n")
	}
	sb.WriteString(m.renderInputLine())

	return sb.String()
}

func (m *AppModel) loadingBannerText() string {
	return renderFlowpilotLoader(m.loadingFrame, m.asciiMode)
}

func suggestionVisibleLimit(sugg []suggestItem) int {
	if len(sugg) > 0 && (sugg[0].kind == "history" || sugg[0].kind == "skill") {
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
		case "skill":
			kind = "skills"
		}
	}
	sel := 0
	if len(sugg) > 0 {
		sel = m.suggIdx % len(sugg)
	}
	start, end := suggestionWindow(len(sugg), sel, limit)
	var sb strings.Builder
	sb.WriteString(styleSuggest.Render(kind + ":"))
	if kind == "skills" {
		sb.WriteString(styleSuggest.Render("  Tab tick · Enter apply"))
	}
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
	if kind == "skills" {
		hint := "  Tab tick to select · Enter apply"
		if above > 0 || below > 0 {
			hint = fmt.Sprintf("  Tab tick to select · Enter apply  · %d above · %d below [%d/%d]", above, below, sel+1, len(sugg))
		}
		sb.WriteString(styleSuggest.Render(hint))
	} else if above > 0 || below > 0 {
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
	rows := m.chatRows()
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}

type chatRow struct {
	Text        string
	MsgIdx      int
	Copy        bool
	CopyText    string
	FenceIdx    int
	LoadEarlier bool
}

func (m *AppModel) chatRows() []chatRow {
	sig := m.chatRowsSig()
	if m.rowCache != nil && m.rowCacheSig == sig {
		return m.rowCache
	}
	rows := m.buildChatRows()
	if rows == nil {
		rows = []chatRow{}
	}
	m.rowCache = rows
	m.rowCacheSig = sig
	return rows
}

func (m *AppModel) chatRowsSig() uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.Itoa(m.width)))
	if m.asciiMode {
		_, _ = h.Write([]byte{1})
	}
	for _, msg := range m.messages {
		_, _ = h.Write([]byte(msg.Role))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(msg.Content))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(msg.FormatHint))
		_, _ = h.Write([]byte{1})
	}
	_, _ = h.Write([]byte(strconv.Itoa(m.visiblePromptCount)))
	_, _ = h.Write([]byte(strconv.FormatInt(m.historyLoadedAfterSeq, 10)))
	return h.Sum64()
}

func (m *AppModel) buildChatRows() []chatRow {
	width := safeTermWidth(m.width)
	if width < 1 {
		if m.width > 0 {
			width = m.width
		} else {
			width = 80
		}
	}
	var rows []chatRow
	if hidden := m.hiddenPromptCountBeforeWindow(); hidden > 0 || m.hasMoreHistoryOnServer() {
		label := loadEarlierPromptLabel(hidden, m.hasMoreHistoryOnServer() && hidden == 0)
		rows = append(rows, chatRow{
			Text:        styleLink.Render(label),
			LoadEarlier: true,
		})
	}
	start := m.windowStartIndex()
	for mi := start; mi < len(m.messages); mi++ {
		msg := m.messages[mi]
		prefix := ""
		prefixStyle := styleSystem
		style := styleSystem
		rightAlign := false
		switch msg.Role {
		case "user":
			prefixStyle = styleUserLabel
			style = styleUser
			rightAlign = true
		case "assistant":
			style = styleAssistant
			if msg.FormatHint == "thinking" {
				style = styleThinking
			}
		case "tool":
			style = styleTool
		case "system":
			switch msg.FormatHint {
			case "error":
				style = styleError
			case "gate", "approval":
				style = styleGate
			case "steps":
				style = styleSystem
			default:
				style = styleSystem
			}
			prefixStyle = style
		}
		contentWidth := width
		if rightAlign {
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
		showCopy := (msg.Role == "user" || msg.Role == "assistant") && msg.FormatHint != "thinking" && strings.TrimSpace(msg.Content) != ""
		boxed := msg.Role == "user"
		if showCopy && boxed {
			contentWidth -= len([]rune(copyChip))
			if contentWidth < 8 {
				contentWidth = 8
			}
		}
		if boxed {
			contentWidth -= 4
			if contentWidth < 8 {
				contentWidth = 8
			}
		}
		mdLines := textsToMD(trimEmptyEdges(wrapText(msg.Content, contentWidth)))
		if msg.Role == "assistant" && msg.FormatHint == "" {
			mdLines = renderMarkdownRows(msg.Content, contentWidth, m.asciiMode)
		}
		var msgRows []chatRow
		fenceN := 0
		for i, ml := range mdLines {
			line := ml.Text
			lineStyle := style
			if msg.FormatHint == "steps" {
				lineStyle = styleForStepBannerLine(line)
			}
			var rendered string
			if msg.FormatHint == "approval" {
				rendered = renderApprovalText(line)
			} else if msg.FormatHint == "question" {
				rendered = renderQuestionText(line)
			} else if i == 0 && prefix != "" {
				rendered = prefixStyle.Render(prefix) + lineStyle.Render(stripANSI(line))
			} else if prefix != "" {
				pad := strings.Repeat(" ", len([]rune(prefix)))
				rendered = pad + lineStyle.Render(stripANSI(line))
			} else if msg.Role == "assistant" && msg.FormatHint == "" {
				rendered = line
			} else {
				rendered = lineStyle.Render(stripANSI(line))
			}
			copyFence := ml.CopyCode != ""
			copyOn := showCopy && i == len(mdLines)-1
			if copyOn && !boxed && !copyFence {
				rendered = rendered + styleLink.Render(copyChip)
			}
			if rightAlign && !boxed {
				rendered = rightAlignPlain(rendered, width)
			}
			row := chatRow{Text: rendered, MsgIdx: mi, Copy: copyOn || copyFence}
			if copyFence {
				row.CopyText = ml.CopyCode
				row.FenceIdx = fenceN
				fenceN++
			}
			msgRows = append(msgRows, row)
		}
		if boxed {
			msgRows = strokeChatRows(msgRows, width, true, m.asciiMode)
		}
		if mi > 0 && chatGapBefore(m.messages[mi-1], msg) {
			rows = append(rows, chatRow{})
		}
		rows = append(rows, msgRows...)
	}
	return rows
}

func chatGapBefore(prev, cur ChatMessage) bool {
	return isChatBubble(prev) && isChatBubble(cur)
}

func isChatBubble(msg ChatMessage) bool {
	if msg.Role == "user" {
		return true
	}
	return msg.Role == "assistant" && msg.FormatHint == ""
}

// formatApprovalWaitingLine is the transcript line for a live gate. Approve/Deny
// live in the input bar; this copy must not say "click" so the response row
// is not mistaken for the control.
func formatApprovalWaitingLine(approvalID string, ascii bool) string {
	wait := "⏳ "
	suffix := "Waiting user…"
	if ascii {
		wait = "... "
		suffix = "Waiting user..."
	}
	return wait + fmt.Sprintf("[APPROVAL] %s %s", approvalID, suffix)
}

func renderApprovalText(line string) string {
	stripped := stripANSI(line)
	var b strings.Builder
	rest := stripped
	for {
		next, token := nextHighlightToken(rest, []string{"/approve", "/deny", "Approve", "Deny"})
		if next < 0 {
			b.WriteString(styleGate.Render(rest))
			return b.String()
		}
		b.WriteString(styleGate.Render(rest[:next]))
		b.WriteString(styleLink.Render(token))
		rest = rest[next+len(token):]
	}
}

func renderQuestionText(line string) string {
	stripped := stripANSI(line)
	var b strings.Builder
	rest := stripped
	tokens := []string{"/approve", "/deny", "Approve", "Deny"}
	for i := 1; i <= 9; i++ {
		tokens = append(tokens, strconv.Itoa(i)+")")
	}
	for {
		next, token := nextHighlightToken(rest, tokens)
		if next < 0 {
			b.WriteString(styleGate.Render(rest))
			return b.String()
		}
		b.WriteString(styleGate.Render(rest[:next]))
		b.WriteString(styleLink.Render(token))
		rest = rest[next+len(token):]
	}
}

func nextHighlightToken(s string, tokens []string) (int, string) {
	best, tok := -1, ""
	for _, t := range tokens {
		i := strings.Index(s, t)
		if i < 0 {
			continue
		}
		if best < 0 || i < best || (i == best && len(t) > len(tok)) {
			best, tok = i, t
		}
	}
	return best, tok
}

func renderQuestionBar(left, mid string, q *QuestionState, width int) string {
	var b strings.Builder
	b.WriteString(left)
	b.WriteString(" ")
	b.WriteString(styleGate.Render("question"))
	b.WriteString(" ")
	b.WriteString(mid)
	b.WriteString(" ")
	for i, o := range q.Options {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(styleLink.Render(strconv.Itoa(i+1) + ")"))
		b.WriteString(" ")
		b.WriteString(styleLink.Render(questionOptionLabel(o)))
	}
	if len(q.Options) == 0 {
		b.WriteString(styleSystem.Render("type an answer"))
	} else {
		b.WriteString("  ")
		b.WriteString(styleSystem.Render("click or type"))
	}
	line := b.String()
	plain := stripANSI(line)
	if width > 1 && len([]rune(plain)) > width {
		return styleGate.Render(fitStatusWidth(plain, width))
	}
	return line
}

func styleForStepBannerLine(line string) lipgloss.Style {
	u := strings.ToUpper(line)
	switch {
	case strings.Contains(u, "[RUNNING]") || strings.Contains(u, "[WAITING_USER_APPROVAL]") || strings.HasPrefix(strings.TrimSpace(line), ">"):
		return styleStepRunning
	case strings.Contains(u, "[DONE]"):
		return styleStepDone
	case strings.Contains(u, "[FAILED]"):
		return styleStepFailed
	case strings.HasPrefix(strings.TrimSpace(line), "IN PROGRESS:"):
		return styleStepRunning
	default:
		return styleSystem
	}
}

func fitStatusWidth(s string, w int) string {
	return truncateVisual(s, w)
}

func (m *AppModel) renderInputLine() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	w = safeTermWidth(w)
	if m.sessionLoading && !strings.HasPrefix(m.inputValue, "/") {
		frames := []string{"|", "/", "-", "\\"}
		spin := frames[m.loadingFrame%len(frames)]
		msg := fmt.Sprintf(" %s please wait… (chat disabled) ", spin)
		return styleLoading.Render(truncateVisual(msg, w))
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
	left := styleInputStroke.Render("┃")
	mid := styleInputStroke.Render("│")
	label := strings.TrimSpace(prefix)
	attach := styleLink.Render("[+img] ")
	if n := len(m.pendingAttach); n > 0 {
		attach = stylePromptFocus.Render(fmt.Sprintf("[%d img] ", n))
	}
	innerW := w - 2
	if innerW < 1 {
		innerW = 1
	}
	// Drop the attach chip before it forces the box past the terminal width.
	if lipgloss.Width(stripANSI(attach))+8 > innerW {
		attach = ""
	}
	fixedWidth := 1 + lipgloss.Width(stripANSI(attach)) + 1 // leading space + caret
	availWidth := innerW - fixedWidth
	if availWidth < 1 {
		availWidth = 1
	}

	bodyLines := strings.Split(body, "\n")
	const maxVis = 6
	if len(bodyLines) > maxVis {
		bodyLines = bodyLines[len(bodyLines)-maxVis:]
	}

	var inner []string
	if m.approval != nil {
		inner = append(inner, styleGate.Render("approval")+"  "+
			styleLink.Render("Approve")+"  "+styleLink.Render("Deny")+"  "+
			styleSystem.Render("click or type"))
	}
	if m.question != nil {
		inner = append(inner, renderQuestionBar(left, mid, m.question, innerW))
	}
	caretAt := m.inputCaretIndex()
	off := 0
	for i, bl := range bodyLines {
		runes := []rune(bl)
		lineStart, lineEnd := off, off+len(runes)
		onLine := caretAt >= lineStart && (caretAt < lineEnd || (caretAt == lineEnd && i == len(bodyLines)-1))
		caretCol := caretAt - lineStart
		if !onLine {
			caretCol = -1
		}
		shown := runes
		if len(runes) > availWidth {
			col := caretCol
			if col < 0 {
				col = len(runes)
			}
			var vis int
			shown, vis = windowRunesAround(runes, col, availWidth)
			if caretCol >= 0 {
				caretCol = vis
			}
		}
		var styled string
		if onLine {
			if caretCol < 0 || caretCol > len(shown) {
				caretCol = len(shown)
			}
			styled = styleInputFocus.Render(string(shown[:caretCol])) + caret +
				styleInputFocus.Render(string(shown[caretCol:]))
		} else {
			styled = styleInputFocus.Render(string(shown))
		}
		if i == 0 {
			inner = append(inner, attach+styled)
		} else {
			inner = append(inner, styled)
		}
		off = lineEnd + 1
	}
	footer := strings.TrimSpace(m.model)
	return frameInput(inner, w, label, footer, m.asciiMode)
}

// ---- Helpers ----------------------------------------------------------------

func (m *AppModel) addMessage(role, content, hint string) {
	m.messages = append(m.messages, ChatMessage{
		Role:       role,
		Content:    content,
		FormatHint: hint,
	})
	if role == "user" {
		m.syncVisiblePromptCount()
	}
}

func (m *AppModel) appendAssistantDelta(text string) {
	if idx := m.thinkingIndex(); idx >= 0 {
		m.replaceThinkingAt(idx, text)
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content += text
		return
	}
	m.addMessage("assistant", text, "")
}

func (m *AppModel) ensureAssistantMessage(text string) {
	if isStepCompleteStub(text) && m.hasAssistantContent() && m.thinkingIndex() < 0 {
		return
	}
	if idx := m.thinkingIndex(); idx >= 0 {
		if strings.TrimSpace(text) == "" {
			m.messages = append(m.messages[:idx], m.messages[idx+1:]...)
			return
		}
		m.replaceThinkingAt(idx, text)
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		last := &m.messages[len(m.messages)-1]
		if last.Content == "" {
			last.Content = text
			last.FormatHint = ""
		}
	} else if text != "" {
		m.addMessage("assistant", text, "")
	}
}

func (m *AppModel) thinkingIndex() int {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "assistant" && m.messages[i].FormatHint == "thinking" {
			return i
		}
	}
	return -1
}

func (m *AppModel) replaceThinkingAt(idx int, text string) {
	msg := m.messages[idx]
	msg.Content = text
	msg.FormatHint = ""
	m.messages = append(append(m.messages[:idx], m.messages[idx+1:]...), msg)
}

func (m *AppModel) clearThinkingPlaceholder() {
	if idx := m.thinkingIndex(); idx >= 0 {
		m.messages = append(m.messages[:idx], m.messages[idx+1:]...)
	}
}

func (m *AppModel) hasAssistantContent() bool {
	return strings.TrimSpace(m.lastAssistantText()) != ""
}

func (m *AppModel) lastAssistantText() string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "assistant" && m.messages[i].FormatHint != "thinking" {
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
	for _, p := range []string{"run-", "turn-", "step-", "wf-"} {
		if !strings.HasPrefix(id, p) {
			continue
		}
		rest := id[len(p):]
		if len(rest) <= 10 {
			return id
		}
		return p + rest[:6] + "…"
	}
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
	if len(args) == 0 && m.signedInEmail != "" && !m.authNeedLogin {
		m.addMessage("system", fmt.Sprintf("Signed in as %s (use /login [email] [password] to switch accounts).", m.signedInEmail), "")
		return m, nil
	}
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

func (m *AppModel) cmdShutdownAndQuit() tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		// Always stop the local runner on TUI exit, including a reused process
		// (CA-445 skip-kill leaked runners across just chat-dev sessions).
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cl.ShutdownStack(ctx)
		_ = killRunnerByURL(runnerURL)
		return QuitMsg{}
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
	// Prefer in-memory values already set on the model (from New / prefs).
	if flagProvider == "" {
		flagProvider = m.provider
	}
	if flagModel == "" {
		flagModel = m.model
	}
	// Disk prefs win over "first active account" when flags/memory are empty.
	if saved, _, err := prefs.Load(); err == nil {
		if flagProvider == "" {
			flagProvider = saved.Provider
		}
		if flagModel == "" {
			flagModel = saved.Model
		}
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
	return tea.Batch(m.cmdMaybePrefetchFlows(), m.cmdMaybePrefetchHistory(), m.cmdMaybePrefetchSkills())
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

// turnFinishedMsg is an internal completion message that also carries usage
// (kept for tests / print-mode paths; interactive turns use live turnStream* msgs).
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
	return m.cmdApprove(approvalID, "approve")
}

func (m *AppModel) submitPendingApproval(decision string) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		m.addMessage("system", "No pending approval.", "")
		return m, nil
	}
	return m, m.cmdApprove(m.approval.ID, decision)
}

func (m *AppModel) cmdApprove(approvalID, decision string) tea.Cmd {
	runnerURL := m.runnerURL
	id := approvalID
	dec := decision
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitApproval(context.Background(), id, dec, false); err != nil {
			return ErrMsg{Err: err}
		}
		return ApprovalResolvedMsg{ID: id, Decision: dec}
	}
}

func (m *AppModel) cmdStopTurn() tea.Cmd {
	if m.runHandle == nil {
		return func() tea.Msg { return StoppedMsg{} }
	}
	m.stopFocusStream()
	m.restoreMainTranscript()
	runnerURL := m.runnerURL
	parentID := m.runHandle.RunID
	childIDs := m.childRunIDs()
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = cl.StopAgentLoop(ctx, parentID)
		_ = cl.Interrupt(ctx, parentID)
		for _, id := range childIDs {
			_ = cl.Interrupt(ctx, id)
		}
		return StoppedMsg{}
	}
}

func (m *AppModel) cmdCopyKind(kind string) tea.Cmd {
	idx := -1
	wantUser := kind == "prompt" || kind == "user"
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.FormatHint == "thinking" {
			continue
		}
		if wantUser && msg.Role == "user" {
			idx = i
			break
		}
		if !wantUser && msg.Role == "assistant" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return func() tea.Msg { return CopiedMsg{Kind: kind, Err: "nothing to copy"} }
	}
	return m.cmdCopyMessage(idx)
}

func (m *AppModel) cmdCopyMessage(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.messages) {
		return func() tea.Msg { return CopiedMsg{Err: "nothing to copy"} }
	}
	text := m.messages[idx].Content
	kind := m.messages[idx].Role
	if kind == "assistant" {
		kind = "answer"
	}
	if kind == "user" {
		kind = "prompt"
	}
	return m.cmdCopyText(text, kind)
}

func (m *AppModel) cmdCopyFence(msgIdx, fenceIdx int) tea.Cmd {
	for _, r := range m.chatRows() {
		if r.MsgIdx == msgIdx && r.CopyText != "" && r.FenceIdx == fenceIdx {
			return m.cmdCopyText(r.CopyText, "code")
		}
	}
	return func() tea.Msg { return CopiedMsg{Kind: "code", Err: "nothing to copy"} }
}

func (m *AppModel) cmdCopyText(text, kind string) tea.Cmd {
	return func() tea.Msg {
		if err := writeClipboardText(text); err != nil {
			return CopiedMsg{Kind: kind, Err: err.Error()}
		}
		return CopiedMsg{Kind: kind}
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

// startupGrokYoloPostureCmd applies the existing Grok YOLO endpoint when the
// TUI started with --yolo (local flag only). /yolo already calls
// cmdGrokYoloPosture; chat turns still send yoloMode (Task-291).
func (m *AppModel) startupGrokYoloPostureCmd() tea.Cmd {
	if m == nil || !m.yolo || !strings.EqualFold(m.provider, "grok") {
		return nil
	}
	return m.cmdGrokYoloPosture(true)
}

// ---- Run (entrypoint) -------------------------------------------------------

// Run starts the Bubble Tea program. In headless/print mode it runs
// the model loop and prints the final response to stdout, then exits.
func Run(cfg config.ChatConfig, runnerURL string) error {
	m := New(cfg, runnerURL)

	if cfg.Print {
		return runHeadless(m, cfg.Prompt)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
