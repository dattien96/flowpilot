package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	// Opencode-style hierarchy: the whole window canvas is the DARKEST layer
	// (#0d0d0d) and the elevated panels (code blocks, right sidebar, chat bar)
	// are progressively lighter grays above it (CA-532).
	colorCanvas = "#0d0d0d" // --bg (opencode canvas)
	colorBg2    = "#161616" // --bg-2 (right sidebar / elevated panel)
	colorBg3    = "#1e1e1e" // --bg-3 (chat bar, loading, step-running)
	colorCodeBg = "#2e2e2e" // fenced-code panel (solid lifted card vs --bg)
	// Status-line exclusive values (not reused for model/YOLO/skills/open-back).
	colorStatusAgent = "#2dd4bf" // teal — agent:<name> value
	colorStatusFlow  = "#f472b6" // pink — flow name / active step value
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
	styleStatusHi  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)) // model, reason value, YOLO value, 7d, skills
	styleStatusOK  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))
	styleStatusErr = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	// agent:NAME and flow-name values — dedicated hues, not styleStatusHi/accent.
	styleStatusAgent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorStatusAgent))
	styleStatusFlow  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorStatusFlow))
	stylePrompt      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
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
	// F2 step [open]/[back] — distinct from step highlight (accent) and running (warn).
	styleStepAgentAction = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color(colorAsk))
	// Canvas + elevated-panel backgrounds (CA-532): whole window canvas is darkest,
	// the right sidebar and chat bar are lighter grays like opencode.
	styleCanvas  = lipgloss.NewStyle().Background(lipgloss.Color(colorCanvas))
	styleSidebar = lipgloss.NewStyle().Background(lipgloss.Color(colorBg2))
	styleChatBar = lipgloss.NewStyle().Background(lipgloss.Color(colorBg3))
)

// ---- New / Init -------------------------------------------------------------

// New creates a new AppModel from ChatConfig and the runner URL.
func New(cfg config.ChatConfig, runnerURL string) *AppModel {
	provider := cfg.Provider
	model := cfg.Model
	reasoning := cfg.ReasoningEffort
	// Restore last TUI selection when flags omit provider/model; restore mode/flow/yolo.
	var savedPrefs prefs.Session
	var haveSaved bool
	if saved, _, err := prefs.Load(); err == nil {
		haveSaved = true
		savedPrefs = saved
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
	yolo := cfg.Yolo
	// TestMain sets FLOWPILOT_TUI_SKIP_MODE_RESTORE so shared session-file
	// pollution from /flow or /yolo tests cannot force mode/yolo on every New().
	skipSessionUX := strings.TrimSpace(os.Getenv("FLOWPILOT_TUI_SKIP_MODE_RESTORE")) != ""
	// --yolo flag wins; otherwise restore chat-mode YOLO preference from disk.
	if !cfg.Yolo && haveSaved && !skipSessionUX && savedPrefs.Yolo != nil {
		yolo = *savedPrefs.Yolo
	}
	m := &AppModel{
		cfg:             cfg,
		runnerURL:       runnerURL,
		client:          client.New(runnerURL),
		inputCursor:     -1,
		yolo:            yolo,
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
	if haveSaved && !skipSessionUX {
		// Defer flow-mode arm until project catalog binds (tryApplyPendingFlowRestore).
		// Immediate ModeFlow restore on cold start left the TUI unusable when catalog
		// was slow/empty (operator report: chat mode OK, restored flow mode hangs).
		stashPendingFlowRestore(m, savedPrefs)
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

// cmdThinkingTick schedules the 90ms spinner step while a thinking row is live.
func cmdThinkingTick() tea.Cmd {
	return tea.Tick(thinkingTickInterval, func(time.Time) tea.Msg {
		return thinkingTickMsg{}
	})
}

// ---- Update -----------------------------------------------------------------

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fullWidth = msg.Width
		return m, nil

	case cursorTickMsg:
		m.cursorOn = !m.cursorOn
		if m.sessionLoading {
			m.loadingFrame = (m.loadingFrame + 1) % 64
		}
		cmds := []tea.Cmd{tickCursor()}
		// Start the 90ms spinner ticker whenever a chat turn or flow step is live
		// (CA-537): the spinner animates on the status line + F2 RUNNING step, not
		// in the chat timeline. Frame resets on the idle→live edge so the elapsed
		// clock does not race ahead while idle. The ticker self-cancels on its own
		// tick once work is no longer live.
		if m.workIsLive() {
			if !m.thinkingTickerActive {
				m.thinkingTickerActive = true
				m.thinkingFrame = 0
				cmds = append(cmds, cmdThinkingTick())
			}
		} else {
			m.thinkingTickerActive = false
		}
		// While a flow turn/orchestration is live, poll steps-runtime so long
		// silent steps (e.g. grok-context / context.produce) stay visible.
		// Also re-hydrate agent graph so F2 [open] appears as soon as a child
		// agent starts (do not wait for run complete or a missed SSE).
		// Gated by shouldPollStepsRuntime so idle/terminal + dead-runner pause.
		if m.shouldPollStepsRuntime() {
			m.stepsPollTicks++
			if m.stepsPollTicks%3 == 0 { // ~1.6s
				cmds = append(cmds, m.cmdRefreshStepsRuntime())
				if m.runHandle != nil && m.runnerPollFailStreak < runnerPollFailPauseAfter {
					cmds = append(cmds, m.cmdHydrateAgentRuns(m.runHandle.RunID))
				}
			}
		} else {
			m.stepsPollTicks = 0
		}
		return m, tea.Batch(cmds...)

	case thinkingTickMsg:
		m.thinkingFrame++
		if m.workIsLive() {
			return m, cmdThinkingTick()
		}
		m.thinkingTickerActive = false
		return m, nil

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
		m.flowLoopStatus = "stopped"
		m.addMessage("system", "Stopped.", "")
		return m, nil

	case CopiedMsg:
		if msg.Err != "" {
			m.addMessage("system", "Copy failed: "+msg.Err, "error")
			return m, nil
		}
		// Transient toast — do not pollute the chat timeline (CA-511).
		kind := strings.TrimSpace(msg.Kind)
		if kind == "" {
			kind = "selection"
		}
		return m, m.showFlashToast("Copied " + kind + ".")

	case toastClearMsg:
		if msg.ID == m.flashToastID {
			m.flashToast = ""
		}
		return m, nil

	case ConnectedMsg:
		m.runnerURL = msg.RunnerURL
		m.connStatus = ConnIdle
		m.statusMsg = "connected"
		m.sessionLoading = true
		m.sessionPanel.RunnerURL = msg.RunnerURL
		m.sessionPanel.ProjectPath = m.cfg.ProjectPath
		m.sessionPanel.Collapsed = false
		cmds := []tea.Cmd{
			m.cmdLoadSessionDefaults(),
			m.cmdPrefetchFlows(),
			// The FlowPilot banner stays up until SessionDefaultsMsg decides the
			// catalog (project bound or failed) — typing stays interactive, send
			// stays blocked (CA-514). The 45s safety net is the only bail-out.
			tea.Tick(45*time.Second, func(time.Time) tea.Msg { return sessionLoadTimeoutMsg{} }),
		}
		if m.cfg.ResumeRunID != "" {
			cmds = append(cmds, m.cmdResume(m.cfg.ResumeRunID))
		}
		return m, tea.Batch(cmds...)

	case sessionKeysUnlockMsg:
		// Deprecated since CA-514 fourth pass: no longer scheduled. The FlowPilot
		// banner stays up until SessionDefaultsMsg decides the catalog. Kept as a
		// no-op so a stale delivery can never clear the banner early.
		return m, nil

	case sessionLoadTimeoutMsg:
		// Safety net only if defaults never arrived (runner truly stuck).
		// Do NOT mark sessionDefaultsLoaded: a late SessionDefaultsMsg must still
		// count as first load so it persists provider/model and restores flow.
		if m.sessionDefaultsLoaded {
			return m, nil
		}
		m.sessionLoading = false
		m.connStatus = ConnError
		m.statusMsg = "session load failed"
		m.addMessage("system",
			"Session did not load from "+m.runnerURL+" within 45s.\n"+
				"Check runner logs / .env (Supabase). UI stays usable: /help /login /status /provider",
			"error",
		)
		m.refreshSessionPanel()
		return m, nil

	case ProjectsCatalogMsg:
		if msg.Err != "" {
			// Soft: keep UI usable; offer one more retry path via /login or restart.
			if m.project == nil {
				m.addMessage("system",
					"Project catalog still unavailable: "+msg.Err+"\n"+
						"Runner /health can be fine while Supabase catalog is slow — try /login or restart runner.",
					"error",
				)
				if hint := m.pendingFlowRestoreHint(); hint != "" {
					m.addMessage("system", hint, "")
				}
			}
			return m, nil
		}
		if len(msg.Projects) > 0 {
			m.projects = msg.Projects
		}
		if msg.Project != nil {
			m.project = msg.Project
		} else if m.project == nil && m.cfg.ProjectPath != "" && len(m.projects) > 0 {
			m.project = matchProjectByPath(m.projects, m.cfg.ProjectPath)
		}
		m.refreshSessionPanel()
		if m.project != nil {
			m.statusMsg = "ready"
			m.addMessage("system", fmt.Sprintf("Project bound: %s — you can chat now.", m.project.Name), "")
			if notice := m.tryApplyPendingFlowRestore(); notice != "" {
				m.addMessage("system", notice, "")
			}
			var cmds []tea.Cmd
			if len(m.chatList) == 0 {
				cmds = append(cmds, m.cmdPrefetchChats())
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

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
			m.persistSessionPrefs()
		}
		m.refreshSessionPanel()
		m.sessionLoading = false
		m.sessionDefaultsLoaded = true
		m.connStatus = ConnIdle
		m.statusMsg = "ready"
		// Never show a false "ready": when the catalog produced no project the
		// operator must see why chat is disabled (CA-514).
		if m.project == nil {
			switch {
			case runnerDialDeadErr(msg.CatalogErr):
				m.statusMsg = "catalog unavailable"
			case msg.CatalogErr != "":
				m.statusMsg = "loading catalog…"
			case len(m.projects) > 0:
				m.statusMsg = "no project match"
			default:
				m.statusMsg = "no project"
			}
		}
		var cmds []tea.Cmd
		// Catalog timeout ≠ runner offline: /health can pass while Supabase is slow.
		// Only a dial-level failure is the runner being dead; a ctx deadline from
		// the catalog budget is slow-Supabase → schedule a background retry.
		if msg.CatalogErr != "" {
			if runnerDialDeadErr(msg.CatalogErr) {
				m.addMessage("system", "Project catalog unavailable: "+msg.CatalogErr+"\nChat needs Supabase catalog (same as Desktop). Fix .env / runner, then restart.", "error")
			} else {
				m.addMessage("system",
					"Project catalog slow/timeout: "+msg.CatalogErr+"\n"+
						"Runner is up; waiting on Supabase catalog — retrying in background…",
					"error",
				)
				cmds = append(cmds, m.cmdLoadProjectsCatalog())
			}
		}
		// Chat is disabled without a bound project_id: always surface the help
		// (also when the catalog error path already added a message above).
		if m.project == nil && m.cfg.ProjectPath != "" {
			m.addMessage("system", formatMissingProjectHelp(m.cfg.ProjectPath, m.projects), "error")
		}
		m.applyAuthNotice(msg.CatalogErr)
		if firstLoad {
			m.addMessage("system", "Ready — type / for commands · F2/click session panel · F3/click skills chip.", "")
		}
		// Restore flow mode only after project_id exists (cold-start arm is unsafe).
		if m.project != nil {
			if notice := m.tryApplyPendingFlowRestore(); notice != "" {
				m.addMessage("system", notice, "")
			}
		} else if firstLoad {
			if hint := m.pendingFlowRestoreHint(); hint != "" {
				m.addMessage("system", hint, "")
			}
		}
		cmds = append(cmds, m.cmdRefreshProjectContext(), m.cmdLoadSkills(false))
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
			// Do not leave a half-open live poll arming on a failed open.
			if runnerUnreachableErr(msg.Err) {
				m.runnerPollFailStreak = runnerPollFailPauseAfter
			}
			return m, nil
		}
		m.stopOrchestrationStream()
		handle := msg.Handle
		m.runnerPollFailStreak = 0
		m.stepsPollInFlight = false
		m.agentsHydrateInFlight = false
		m.agentHydrateRetries = 0
		// Prefer server snapshot / history row status so terminal opens do not
		// arm [stop] via flow orch listener (empty resume status looked "live").
		if st := strings.TrimSpace(msg.Snapshot.Status); st != "" {
			handle.Status = st
		} else if st := strings.TrimSpace(msg.HistoryMeta.Status); st != "" && strings.TrimSpace(handle.Status) == "" {
			handle.Status = st
		}
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
		// Seed the client per-run SSE cursor so a later continue turn streams
		// from the resume snapshot instead of replaying the whole old turn
		// (CA-520). Desktop already seeds this via consumeHistoryReplayStream.
		if handle.RunID != "" {
			m.client.NoteLastSeq(handle.RunID, handle.LastEventSeq)
		}
		m.viewport.offset = 0
		if len(msg.Messages) > 0 {
			m.messages = append([]ChatMessage(nil), msg.Messages...)
			m.syncVisiblePromptCount()
		}
		if pk := strings.TrimSpace(handle.ProviderKey); pk != "" {
			m.provider = pk
			m.bindActiveAccountForProvider()
		}
		m.refreshSessionPanel()
		// Restore flow chrome from resume handle (and history list as fallback).
		m.applyOpenedRunFlowChrome(handle, msg.HistoryMeta)
		// A resumed workflow/flow run may carry no StepID on the handle (runner
		// only mints "chat-<runId>" for normal chat, T-7). Resolve the launch
		// fallback now (workflow id / synthetic chat id) so continue turns after
		// /open never POST an empty stepId — startTurn rejects that with 400
		// (CA-519).
		if strings.TrimSpace(m.stepID) == "" {
			m.stepID = m.resolveTurnStepID()
		}
		m.connStatus = ConnIdle
		// Per-run token usage (CA-540): /open must not carry another chat's ctx/
		// token numbers. Clear, then seed from the run's own last usage event.
		m.lastTokens = msg.TokenUsage
		if msg.TokenUsage != nil && msg.TokenUsage.ModelContextWindow != nil && *msg.TokenUsage.ModelContextWindow > 0 {
			m.modelContextWin = *msg.TokenUsage.ModelContextWindow
		}
		m.statusMsg = fmt.Sprintf("opened %s", shortID(handle.RunID))
		kind := "chat"
		if m.mode == ModeFlow || m.mode == ModeStep || m.launch.IsCatalogWorkflow() {
			kind = "flow"
		}
		openLabel := handle.RunID
		if kind == "flow" {
			if n := m.launch.StatusLabel(); n != "" && n != "flow" {
				openLabel = n + " · " + handle.RunID
			}
		}
		m.addMessage("system", fmt.Sprintf("Opened %s %s — continue typing or /history|/open|/resume to switch.", kind, openLabel), "")
		hydrate := m.applyPendingFromSnapshot(msg.Snapshot)
		var cmds []tea.Cmd
		if hydrate != nil {
			cmds = append(cmds, hydrate)
		}
		if m.runHandle != nil && m.orchStream == nil {
			cmds = append(cmds, m.cmdStartOrchestrationStream())
		}
		// One-shot steps fetch always fires for flow opens so the F2 step
		// timeline renders even for completed runs (run-189839). The cursor
		// auto-poll cadence is still gated by shouldPollStepsRuntime (CA-508/514).
		if kind == "flow" && m.runHandle != nil {
			cmds = append(cmds, m.cmdRefreshStepsRuntime())
		}
		// Hydrate sub-agents so /agent Tab and step [open] work after /open.
		if kind == "flow" && m.runHandle != nil {
			cmds = append(cmds, m.cmdHydrateAgentRuns(m.runHandle.RunID))
			// One-shot graph fetch seeds loop state (done/blocked/running) so an
			// opened blocked flow shows the awaiting-user banner instead of arming
			// [stop] on the stale live handle (BUG-231 run-189839 parity with
			// Desktop refreshAgentGraph on history open).
			cmds = append(cmds, m.cmdHydrateAgentGraph(m.runHandle.RunID))
			cmds = append(cmds, m.cmdHydrateDispatchAttention(m.runHandle.RunID))
		}
		// Catalog may still be loading — refresh flow list so status label can use name.
		if kind == "flow" && len(m.flowWorkflows) == 0 && len(m.flowBuiltins) == 0 {
			cmds = append(cmds, m.cmdPrefetchFlows())
		}
		return m, tea.Batch(cmds...)

	case agentRunsHydratedMsg:
		m.agentsHydrateInFlight = false
		if msg.Err != "" {
			m.noteRunnerPollResult(msg.Err)
			// Dead runner: stop retrying (CA-514). Soft errors may still recover
			// on a fresh hydrate (CA-528).
			if runnerUnreachableErr(msg.Err) {
				return m, nil
			}
			return m, m.cmdHydrateAgentRunsIfNeeded()
		}
		if m.runHandle == nil || m.runHandle.RunID != msg.ParentRunID {
			return m, nil
		}
		m.noteRunnerPollResult("")
		// Merge instead of clobber so a main-only/empty list cannot erase children
		// a faster agent_graph_updated already mapped (CA-528).
		m.adoptAgentRuns(msg.Runs)
		if m.hasChildAgentRuns() {
			m.agentHydrateRetries = 0
		}
		// Steps may still be missing a child [open] chip — re-arm a bounded retry
		// so the chip appears without waiting for the next steps transition.
		return m, m.cmdHydrateAgentRunsIfNeeded()

	case hydrateAgentRunsIfNeededMsg:
		if m.runHandle == nil {
			return m, nil
		}
		if m.stepsNeedChildOpenChip() {
			return m, m.cmdHydrateAgentRuns(m.runHandle.RunID)
		}
		return m, nil

	case runSnapshotMsg:
		if msg.Err != "" {
			return m, nil
		}
		return m, m.applyPendingFromSnapshot(msg.Snap)

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
		// Same banner-until-catalog contract as cold start: typing stays free,
		// send stays blocked until SessionDefaultsMsg (or the 45s safety net).
		return m, tea.Batch(
			m.cmdLoadSessionDefaults(),
			tea.Tick(45*time.Second, func(time.Time) tea.Msg { return sessionLoadTimeoutMsg{} }),
		)

	case FlowListMsg:
		m.flowBuiltins = msg.Builtins
		m.flowWorkflows = msg.Workflows
		// After catalog load: re-resolve armed arm + upgrade UUID labels.
		// Also apply deferred prefs flow restore if project is already bound.
		if m.project != nil {
			if notice := m.tryApplyPendingFlowRestore(); notice != "" && !msg.Silent {
				m.addMessage("system", notice, "")
			}
		}
		if m.mode == ModeFlow || m.mode == ModeStep {
			m.refineLaunchFromCatalog()
		}
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
		m.runnerPollFailStreak = 0
		m.stepsPollInFlight = false
		m.agentsHydrateInFlight = false
		m.agentHydrateRetries = 0
		if handle.StepID != "" {
			m.stepID = handle.StepID
		}
		if handle.ProviderKey != "" && m.provider == "" {
			m.provider = handle.ProviderKey
		}
		m.connStatus = ConnRunning
		m.statusMsg = fmt.Sprintf("run %s • %s", shortID(handle.RunID), handle.ProviderKey)
		m.addMessage("system", fmt.Sprintf("Run %s started — streaming events…", shortID(handle.RunID)), "")
		m.refreshSessionPanel()
		prompt := m.pendingPrompt
		m.pendingPrompt = ""
		var cmds []tea.Cmd
		if m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep {
			cmds = append(cmds, m.cmdRefreshStepsRuntime())
			// Seed agent list early so first child spawn can show [open] on next hydrate.
			cmds = append(cmds, m.cmdHydrateAgentRuns(handle.RunID))
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
		if m.isFlowChrome() {
			if m.flowStepsActive != "" {
				m.statusMsg = "step: " + m.flowStepsActive
			} else {
				m.statusMsg = "flow running…"
			}
		} else if strings.EqualFold(strings.TrimSpace(m.reasoningEffort), "high") {
			m.statusMsg = "thinking…"
		}
		return m, m.cmdPollTurnStream()

	case turnStreamEventMsg:
		if msg.Ev.Seq > m.lastEventSeq {
			m.lastEventSeq = msg.Ev.Seq
		}
		if m.runHandle != nil {
			m.client.NoteLastSeq(m.runHandle.RunID, msg.Ev.Seq)
		}
		m2, cmd := m.handleEvent(msg.Ev)
		am := m2.(*AppModel)
		return am, tea.Batch(cmd, am.cmdPollTurnStream())

	case turnStreamClosedMsg:
		m.turnStream = nil
		if msg.Err != nil {
			// BUG-231 (run-189839 parity): a freeform turn against a blocked flow
			// answers 409 flow_awaiting_user. That is a deliberate parked state,
			// not a failure — park the flow (keep the handle) so the user can
			// /continue or /stop instead of being dropped to a dead error state.
			if client.IsFlowAwaitingUserError(msg.Err) {
				m.clearThinkingPlaceholder()
				m.flowLoopStatus = "blocked"
				if m.flowBlockReason == "" {
					m.flowBlockReason = "awaiting_user"
				}
				m.showBlockedBanner(client.AgentLoopState{
					Status:      "blocked",
					BlockReason: m.flowBlockReason,
					Round:       0,
					RoundCap:    0,
				})
				var cmds []tea.Cmd
				if m.runHandle != nil {
					cmds = append(cmds, m.cmdHydrateAgentGraph(m.runHandle.RunID))
					cmds = append(cmds, m.cmdHydrateAgentRuns(m.runHandle.RunID))
					cmds = append(cmds, m.cmdRefreshStepsRuntime())
					cmds = append(cmds, m.cmdHydrateDispatchAttention(m.runHandle.RunID))
				}
				return m, tea.Batch(cmds...)
			}
			m.connStatus = ConnError
			m.statusMsg = "turn failed"
			m.addMessage("system", "Send turn failed: "+msg.Err.Error(), "error")
			m.runHandle = nil
			m.clearThinkingPlaceholder()
			return m, nil
		}
		if m.connStatus == ConnRunning {
			m.connStatus = ConnIdle
			m.statusMsg = "done"
		}
		// run-96217: stream closed after tools while thinking… still present
		// (missed message_delta / empty FinalMessage). Never leave the placeholder.
		if m.thinkingIndex() >= 0 {
			if fill := strings.TrimSpace(m.lastAssistantText()); fill != "" {
				m.ensureAssistantMessage(fill)
			} else {
				m.clearThinkingPlaceholder()
				m.addMessage("system", "Turn finished but no assistant text arrived on the live stream (check runner log / /history).", "error")
			}
		}
		cmds := []tea.Cmd{m.cmdRefreshStepsRuntime()}
		// Desktop startOrchestrationStream: keep listening after the user turn
		// so late gate/approval events (run-97624 chat-mode reprompt) surface.
		// Do not reuse shouldPollStepsRuntime — that predicate is flow/step UI only.
		if m.runHandle != nil && m.orchStream == nil {
			cmds = append(cmds, m.cmdStartOrchestrationStream())
		}
		cmds = append(cmds, m.cmdHydratePendingFromSnapshot())
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
		if m.runHandle != nil {
			m.client.NoteLastSeq(m.runHandle.RunID, msg.Ev.Seq)
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
		if msg.Err != "" {
			// Total failure (runner unreachable / child not resumable): leave the
			// main transcript visible instead of a stuck empty child chrome.
			m.restoreMainTranscript()
			m.addMessage("system", "Open child transcript failed: "+msg.Err, "error")
			return m, nil
		}
		m.stopFocusStream()
		// Keep the system banner; append seeded history after it.
		if len(msg.Messages) > 0 {
			m.messages = append(m.messages, msg.Messages...)
			m.syncVisiblePromptCount()
			m.viewport.offset = 0
		} else if msg.Fallback != "" {
			m.addMessage("system", msg.Fallback, "steps")
		} else {
			m.addMessage("system", "(no transcript events for this agent yet)", "")
		}
		if msg.EvCh != nil {
			m.focusStream = &orchStreamState{evCh: msg.EvCh, cancel: msg.Cancel}
			return m, m.cmdPollFocusStream()
		}
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return m, nil

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
		m.stepsPollInFlight = false
		if m.runHandle == nil || msg.RunID != m.runHandle.RunID {
			return m, nil
		}
		if msg.Err != "" {
			// Soft failure — catalog may not have steps yet; track dead-runner streak.
			m.noteRunnerPollResult(msg.Err)
			return m, nil
		}
		m.noteRunnerPollResult("")
		prevSteps := append([]client.WorkflowStepRuntime(nil), m.flowSteps...)
		prevActive := m.flowStepsActive
		m.flowSteps = msg.Steps
		m.flowStepsActive = activeStepName(msg.Steps)
		m.refreshSessionPanel()
		if m.flowStepsActive != "" && (m.connStatus == ConnRunning || m.connStatus == ConnWaiting) {
			m.statusMsg = "step: " + m.flowStepsActive
		}
		// All steps now terminal + loop done + no active agents → finished flow:
		// drop [stop] and any stale running/streaming chrome (run-189839).
		m.settleFlowIfDone()
		// Step progress lines only on main hub view — never while reading a child
		// transcript (would interleave flow banners into sub-agent chat).
		if !m.viewingChild() {
			for _, line := range formatStepChatNotices(prevSteps, msg.Steps, prevActive, m.flowStepsActive, m.lastTurnError) {
				m.addMessage("system", line, "steps")
			}
		}
		// Agent-bearing step became active / finished → re-hydrate so [open] appears live.
		var cmds []tea.Cmd
		if stepsSuggestChildAgentOpen(prevSteps, msg.Steps) {
			cmds = append(cmds, m.cmdHydrateAgentRuns(m.runHandle.RunID))
		}
		// Already have children mapped → keep F2 expanded for open/back.
		m.expandSessionPanelForChildAgents()
		// Steps still missing a child [open] chip (e.g. a slower/empty hydrate) →
		// re-arm a bounded retry so the chip appears without waiting for the next
		// steps transition (CA-528).
		cmds = append(cmds, m.cmdHydrateAgentRunsIfNeeded())
		return m, tea.Batch(cmds...)

	case ClipboardPasteMsg:
		if msg.Err != "" && msg.Attachment == nil && msg.Text == "" {
			m.addMessage("system", msg.Err, "error")
			return m, nil
		}
		if msg.Attachment != nil {
			m.appendPendingAttachment(*msg.Attachment)
			m.addMessage("system", fmt.Sprintf(
				"Attached image: %s (%d pending) — click [%d img] to manage, /image open %d to view",
				msg.Attachment.OriginalName, len(m.pendingAttach), len(m.pendingAttach), len(m.pendingAttach),
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
		// Stale-parent guard (Desktop run-63960): an agent graph for a different
		// run must never overwrite the current loop state / agent runs, or a late
		// blocked refresh could clobber a newer Continue/Stop/switch-run result.
		if msg.Graph != nil {
			if m.runHandle != nil && strings.TrimSpace(msg.Graph.ParentRunID) != "" &&
				strings.TrimSpace(msg.Graph.ParentRunID) != m.runHandle.RunID {
				return m, nil
			}
			m.applyAgentGraph(msg.Graph)
		}
		return m, nil

	case AgentGraphHydratedMsg:
		if msg.Err != "" {
			m.noteRunnerPollResult(msg.Err)
			return m, nil
		}
		if m.runHandle == nil || m.runHandle.RunID != msg.ParentRunID {
			return m, nil
		}
		if msg.Graph != nil {
			m.applyAgentGraph(msg.Graph)
		}
		return m, nil

	case AttentionLoadedMsg:
		if m.runHandle == nil || m.runHandle.RunID != msg.RunID {
			return m, nil
		}
		if msg.Err != nil {
			m.attentionErr = msg.Err.Error()
			m.noteRunnerPollResult(msg.Err.Error())
			return m, nil
		}
		m.applyAttention(msg.Items)
		return m, nil

	case AttentionInspectedMsg:
		if m.runHandle == nil || m.runHandle.RunID != msg.RunID {
			return m, nil
		}
		if msg.Err != nil {
			m.attentionErr = msg.Err.Error()
			return m, nil
		}
		if msg.Result != nil {
			if m.attentionInspect == nil {
				m.attentionInspect = map[string]*client.DispatchInspectResult{}
			}
			m.attentionInspect[attentionKey(msg.RunID, msg.TurnID)] = msg.Result
		}
		return m, nil

	case AttentionResolvedMsg:
		if m.runHandle == nil || m.runHandle.RunID != msg.RunID {
			return m, nil
		}
		if msg.Err != nil {
			m.attentionErr = msg.Err.Error()
			m.addMessage("system", "Dispatch resolution failed: "+msg.Err.Error(), "error")
			return m, m.cmdHydrateDispatchAttention(m.runHandle.RunID)
		}
		m.addMessage("system", fmt.Sprintf("Dispatch %s resolved (%s).", msg.Kind, msg.Outcome), "")
		// Drop the resolved item + refresh to reflect the committed settlement.
		return m, m.cmdHydrateDispatchAttention(m.runHandle.RunID)

	case TurnDoneMsg:
		m.connStatus = ConnIdle
		m.statusMsg = "done"
		// A completed turn means the flow moved on — a still-armed gate is stale
		// (CA-536). It must not keep swallowing input.
		m.gate = nil
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
		// A completed turn means the flow moved on — a still-armed gate is stale
		// (CA-536). It must not keep swallowing input.
		m.gate = nil
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
		// Flow chrome is quiet (CA-511): progress lives on F2 steps + status line,
		// so a delta must not overwrite "step: X"/"flow running…" with "streaming…"
		// — that masked a finished flow behind a fake live spinner (run-189839).
		if m.connStatus == ConnRunning && !m.isFlowChrome() {
			m.statusMsg = "streaming…"
		}

	case "message_completed":
		m.appendAssistantDelta(ev.Text)

	case "turn_completed":
		// A completed turn means the flow moved on — a still-armed gate is stale
		// (CA-536). It must not keep swallowing input.
		m.gate = nil
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
		if m.isFlowChrome() {
			if m.flowStepsActive != "" {
				m.statusMsg = "step: " + m.flowStepsActive
			} else {
				m.statusMsg = "flow running…"
			}
		} else {
			m.statusMsg = "turn running…"
		}
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
		// CA-536 (run-103672): only a genuine block with a decision card may
		// lock the composer. The runner emits this event for warn/reprompt
		// verdicts too (and for blocks without r-reg options), which carry
		// empty GateOptions — arming the gate then made every keystroke
		// re-print "Gate options:" with nothing to match, permanently freezing
		// chat. Non-block / option-less verdicts surface as info instead.
		blocking := strings.EqualFold(ev.Status, "block") && len(ev.GateOptions) > 0
		m.addMessage("system", buildGateMessage(ev.GateOptions, ev.GateRegressedTests), "gate")
		if blocking {
			m.gate = &GateState{
				Options:        ev.GateOptions,
				RegressedTests: ev.GateRegressedTests,
				RunID:          ev.WorkflowRunID,
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "gate"
		}

	case "token_usage_updated":
		if ev.TokenUsage != nil {
			m.lastTokens = ev.TokenUsage
			if ev.TokenUsage.ModelContextWindow != nil && *ev.TokenUsage.ModelContextWindow > 0 {
				m.modelContextWin = *ev.TokenUsage.ModelContextWindow
			}
		}

	case "agent_graph_updated":
		if ev.AgentGraph != nil {
			// Stale-parent guard (Desktop run-63960): ignore graphs for a
			// different run so a late blocked refresh cannot overwrite a newer
			// Continue/Stop result.
			if m.runHandle == nil || strings.TrimSpace(ev.AgentGraph.ParentRunID) == "" ||
				strings.TrimSpace(ev.AgentGraph.ParentRunID) == m.runHandle.RunID {
				m.applyAgentGraph(ev.AgentGraph)
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
	// Never hard-block keyboard while loading — a hung runner previously made
	// the TUI feel fully frozen. processInput still rejects non-slash *send*.
	if m.sessionLoading && msg.Type == tea.KeyCtrlC {
		// Allow quit during load without waiting for catalog.
		m.quitting = true
		return m, m.cmdShutdownAndQuit()
	}

	if m.authPhase == AuthNone && (isPromptNewlineKey(msg) || isModifiedEnterNewline(msg)) && !m.viewingChild() {
		m.inputValue += "\n"
		return m, nil
	}

	// Desktop parity (CA-519): a focused sub-agent transcript is read-only. Only
	// navigation, [back], agent-cycle, and slash commands are allowed; chat text
	// input is dropped so the user cannot keep typing into the child view.
	if m.viewingChild() && !m.allowsKeyWhileViewingChild(msg) {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		// Drag-select highlight: copy instead of quit (in-app selection replaces
		// native terminal select while mouse tracking is on).
		if !m.mouseSel.empty() {
			text := m.selectionPlainText()
			m.mouseSel = mouseSelect{}
			if strings.TrimSpace(text) == "" {
				m.statusMsg = "nothing to copy"
				return m, nil
			}
			return m, m.cmdCopyText(text, "selection")
		}
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
			m.clearInputValue()
			m.addMessage("system", "Login cancelled.", "")
			return m, nil
		}
		if m.attachPanelOpen {
			m.closeAttachPanel()
			m.statusMsg = "image panel closed"
			return m, nil
		}
		if !m.mouseSel.empty() {
			m.mouseSel = mouseSelect{}
			m.statusMsg = "selection cleared"
			return m, nil
		}
		if m.inputValue != "" {
			m.clearInputValue()
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
			return m, m.cmdFocusAgent(agent.RunID)
		}
		return m, nil

	case tea.KeyUp:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx - 1 + n) % n
				return m, nil
			}
			// No picker open: Up/Down recall sent prompts (bash-style), never
			// scroll the transcript — scroll is PgUp/PgDown + mouse wheel.
			m.navigatePromptHistory(1)
			m.suggIdx = 0
			return m, nil
		}

	case tea.KeyDown:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx + 1) % n
				return m, nil
			}
			m.navigatePromptHistory(-1)
			m.suggIdx = 0
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
					// Tab ticks; Enter applies (closes picker, keeps ticks on the
					// status chip). Strip only /skill… so draft prompt is preserved
					// (Desktop pickSkill keeps pre-slash text; runner injects via SelectedSkills).
					m.inputValue = stripActiveSlashCommand(m.inputValue, m.inputCaretIndex())
					m.inputCursor = -1
					m.suggIdx = 0
					if n := len(attachedSkillNames(m.selectedSkills)); n > 0 {
						m.statusMsg = fmt.Sprintf("skills:%d attached — prompt kept", n)
					} else {
						m.statusMsg = "skill picker closed"
					}
					return m, nil
				}
				if cmd := suggestionAcceptValue(it); cmd != "" {
					// Action rows only expand the next picker (provider connect, /image open|rm).
					if it.kind == "provider-action" || it.kind == "image-sub-next" {
						m.setInputPreservingDraftPrefix(cmd)
						m.suggIdx = 0
						return m, m.cmdMaybePrefetchPickers()
					}
					// Slash picks are allowed even while a turn is in progress.
					if m.sendBlocked() && !strings.HasPrefix(cmd, "/") {
						return m, nil
					}
					m.clearInputValue()
					return m.processInput(cmd)
				}
				// Placeholder (loading / no match): if the user already typed an
				// arg (e.g. /model custom-id), run the typed line instead of trapping Enter.
				if typed := strings.TrimSpace(m.inputValue); len(strings.Fields(typed)) >= 2 {
					if m.sendBlocked() && !strings.HasPrefix(typed, "/") {
						return m, nil
					}
					m.clearInputValue()
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
		m.clearInputValue()
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
		// Bubble Tea delivers Alt+letter as KeyRunes+Alt (String() == "alt+v").
		// Handle image-paste chords before inserting the bare rune — otherwise
		// Alt+V becomes a literal "v" and never reaches the chord handler below.
		if m.authPhase == AuthNone && msg.Type == tea.KeyRunes {
			switch msg.String() {
			case "alt+v", "ctrl+shift+v":
				return m, m.cmdClipboardPaste()
			}
			// Windows Terminal often steals Ctrl+V and injects bracketed paste
			// (KeyRunes+Paste). Prefer clipboard image/path, then text; fall
			// back to the bracketed-paste runes when the clipboard is empty.
			if msg.Paste {
				return m, m.cmdClipboardPasteWithFallback(string(msg.Runes))
			}
		}
		if msg.Type == tea.KeySpace {
			m.insertInputAtCursor(" ")
		} else {
			m.insertInputAtCursor(string(msg.Runes))
		}
		m.suggIdx = 0
		return m, m.cmdMaybePrefetchPickers()
	}
	// Alt+V / ctrl+shift+v when not delivered as KeyRunes (some terminals).
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
	in := m.slashSuggestLine()
	if flows := filterFlowSuggestions(in, m.flowBuiltins, m.flowWorkflows, projectID); len(flows) > 0 {
		return flows
	}
	// While `/flow ` is open but catalog still loading, show a placeholder row.
	if ok, _ := parseFlowArgPrefix(in); ok {
		if len(m.flowBuiltins) == 0 && len(m.flowWorkflows) == 0 {
			return []suggestItem{{value: "", detail: "loading flows…", kind: "flow"}}
		}
		return []suggestItem{{value: "", detail: "(no matching flows)", kind: "flow"}}
	}
	if chats := filterHistorySuggestions(in, m.chatList); len(chats) > 0 {
		return chats
	}
	if cmd, _, ok := parseChatOpenArgPrefix(in); ok {
		if len(m.chatList) == 0 {
			return []suggestItem{{value: "", detail: "loading chats…", kind: "history", slash: cmd}}
		}
		return []suggestItem{{value: "", detail: "(no matching chats)", kind: "history", slash: cmd}}
	}
	if providerSugg := filterProviderSuggestions(in, m.providers, m.providerAccounts, m.provider); len(providerSugg) > 0 {
		return providerSugg
	}
	if mode, _, ok := parseProviderPicker(in); ok {
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
	if modelSugg := filterModelSuggestions(in, models, m.model); len(modelSugg) > 0 {
		return modelSugg
	}
	if ok, _ := parseSlashArgPrefix(in, "/model"); ok {
		if len(models) == 0 {
			return []suggestItem{{value: "", detail: "no models — set /provider first", kind: "model"}}
		}
		return []suggestItem{{value: "", detail: "(no matching models)", kind: "model"}}
	}
	if reasonSugg := filterReasoningSuggestions(in, m.reasoningEffort); len(reasonSugg) > 0 {
		return reasonSugg
	}
	if ok, _ := parseSlashArgPrefix(in, "/reasoning"); ok {
		return []suggestItem{{value: "", detail: "(no matching effort)", kind: "reasoning"}}
	}
	if skillSugg := filterSkillSuggestions(in, m.skillsCatalog, m.selectedSkills); len(skillSugg) > 0 {
		return skillSugg
	}
	if ok, _ := parseSlashArgPrefix(in, "/skill"); ok {
		if len(m.skillsCatalog) == 0 && len(m.selectedSkills) == 0 {
			return []suggestItem{{value: "", detail: "loading skills…", kind: "skill"}}
		}
		return []suggestItem{{value: "", detail: "(no matching skills)", kind: "skill"}}
	}
	if ok, _ := parseSlashArgPrefix(in, "/s"); ok {
		if len(m.skillsCatalog) == 0 && len(m.selectedSkills) == 0 {
			return []suggestItem{{value: "", detail: "loading skills…", kind: "skill"}}
		}
		return []suggestItem{{value: "", detail: "(no matching skills)", kind: "skill"}}
	}
	if agentSugg := filterAgentSuggestions(in, m.agentRuns); len(agentSugg) > 0 {
		return agentSugg
	}
	// `/agent ` or `/agents ` (space after cmd): show empty picker row.
	// Bare `/agents` keeps slash-cmd list so Enter still toggles Agents focus.
	if _, ok, argSlot := parseAgentPicker(in); ok && argSlot {
		detail := "(no agents yet — wait for children, or /open a flow with sub-agents)"
		if m.runHandle != nil && (m.mode == ModeFlow || m.mode == ModeStep) {
			detail = "(no child agents on this run yet)"
		}
		return []suggestItem{{value: "", detail: detail, kind: "agent"}}
	}
	if imgSugg := filterImageSuggestions(in, m.pendingAttach); len(imgSugg) > 0 {
		return imgSugg
	}
	if _, _, ok := parseImagePicker(in); ok {
		return []suggestItem{{value: "", detail: "(no matching /image option)", kind: "image-sub"}}
	}
	cmds := filterSlashSuggestions(in)
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
	} else if it.kind == "flow" || it.kind == "history" || it.kind == "model" || it.kind == "reasoning" || it.kind == "provider" || it.kind == "provider-connect" || it.kind == "provider-action" || it.kind == "provider-install" || it.kind == "provider-account" || it.kind == "skill" || it.kind == "agent" || it.kind == "image-sub" || it.kind == "image-sub-next" || it.kind == "image-open" || it.kind == "image-rm" {
		// Nested pickers: only replace the active /… fragment (keep pre-slash draft).
		m.setInputPreservingDraftPrefix(cmd)
	} else {
		// Tab fills the command token and leaves a trailing space for args.
		// Mid-draft "abc /sk" → "abc /skill " (do not wipe the draft).
		m.setInputPreservingDraftPrefix(it.value + " ")
	}
	m.suggIdx = (idx + 1) % len(items)
}

// setInputPreservingDraftPrefix writes a slash line over the active / token only.
func (m *AppModel) setInputPreservingDraftPrefix(slashLine string) {
	m.inputValue = replaceActiveSlashWith(m.inputValue, m.inputCaretIndex(), slashLine)
	m.inputCursor = -1
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
	case "agent":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/agent " + it.value
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
	case "image-sub":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		// paste/list/clear run as full commands on Enter.
		return "/image " + strings.TrimSpace(it.value)
	case "image-sub-next":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		// open/rm need a trailing space so the index picker appears (like provider connect).
		return "/image " + strings.TrimSpace(it.value) + " "
	case "image-open":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/image open " + strings.TrimSpace(it.value)
	case "image-rm":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/image rm " + strings.TrimSpace(it.value)
	case "cmd":
		return strings.TrimSpace(it.value)
	default:
		return strings.TrimSpace(it.value)
	}
}

func (m *AppModel) allowsKeyWhileLoading(msg tea.KeyMsg) bool {
	// Chrome + navigation always available (avoids full freeze during catalog load).
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc, tea.KeyF2, tea.KeyF3, tea.KeyF4,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
		tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd,
		tea.KeyTab, tea.KeyShiftTab:
		return true
	case tea.KeyEnter:
		return strings.HasPrefix(strings.TrimSpace(m.inputValue), "/") ||
			strings.HasPrefix(strings.TrimSpace(m.slashSuggestLine()), "/")
	case tea.KeyBackspace, tea.KeyDelete:
		// Edit only when already composing a slash command.
		return strings.HasPrefix(strings.TrimSpace(m.inputValue), "/")
	}
	if _, _, ok := activeSlashLine(m.inputValue, m.inputCaretIndex()); ok {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(m.inputValue), "/") {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] == '/' {
		return true
	}
	return false
}

// allowsKeyWhileViewingChild is the child-view twin of allowsKeyWhileLoading: a
// focused sub-agent transcript is read-only, so only navigation, agent-cycle,
// [back]/Esc, copy, and slash commands are allowed. Plain chat text (including
// Enter-to-send) is dropped — continue happens on the main run only (CA-519).
func (m *AppModel) allowsKeyWhileViewingChild(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc, tea.KeyF2, tea.KeyF3, tea.KeyF4,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
		tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd,
		tea.KeyTab, tea.KeyShiftTab, tea.KeyCtrlV:
		return true
	case tea.KeyEnter:
		return strings.HasPrefix(strings.TrimSpace(m.inputValue), "/") ||
			strings.HasPrefix(strings.TrimSpace(m.slashSuggestLine()), "/")
	case tea.KeyBackspace, tea.KeyDelete:
		return strings.HasPrefix(strings.TrimSpace(m.inputValue), "/")
	}
	if _, _, ok := activeSlashLine(m.inputValue, m.inputCaretIndex()); ok {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(m.inputValue), "/") {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] == '/' {
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
	// Dispatch operator attention (CP-51 Task-256): a new turn must not be sent
	// until every uncertain/repair item is resolved (Desktop blocked dispatch).
	if m.hasUnresolvedAttention() {
		return true
	}
	switch m.connStatus {
	case ConnRunning, ConnWaiting:
		return true
	default:
		return false
	}
}

// runStatusIsTerminal is true when the run is finished and [stop] must not arm
// solely because an orch SSE listener is attached (e.g. /open of completed flow).
func runStatusIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done", "failed", "error",
		"cancelled", "canceled", "stopped", "aborted":
		return true
	default:
		return false
	}
}

func (m *AppModel) turnIsActive() bool {
	if m.pendingPrompt != "" {
		return true
	}
	if m.flowHasActiveAgents() {
		return true
	}
	// Dispatch operator attention (CP-51 Task-256): while an uncertain turn /
	// open repair awaits a decision, the flow is parked — [stop] must not arm
	// against the very user the flow is waiting on.
	if m.hasUnresolvedAttention() {
		return false
	}
	// A blocked loop (awaiting-user pause, BUG-231) with no live child is parked,
	// not running: [stop] must not arm against the very user the flow is waiting
	// on to Continue/Stop. run-189839 showed [stop] on a parked blocked flow.
	if m.flowLoopBlocked() {
		return false
	}
	// Flow/step orch SSE can mean live work — but after /open we also attach orch
	// as a late-event listener on finished runs (same idea as plain-chat run-97624).
	// Do not arm [stop] when the handle is known-terminal and no agent is active.
	if m.shouldPollStepsRuntime() && m.orchStream != nil {
		if m.runHandle == nil || !runStatusIsTerminal(m.runHandle.Status) {
			return true
		}
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

// workIsLive reports whether the chat turn or the flow is actively working, so
// the status-line + F2 RUNNING-step spinner animates whenever there is real work
// (CA-537). It must NOT animate while the user is deciding (gate/question/
// approval), when the flow is parked on operator attention, or when a blocked
// loop is paused — in those states the work is waiting on the user, not running.
// ConnConnecting/session-loading is deliberately not "live": those phases already
// surface their own "connecting…"/"loading…" labels.
func (m *AppModel) workIsLive() bool {
	// Decision states first: an agent waiting on the user reports a
	// "waiting_user_approval" status, so flowHasActiveAgents must not win here.
	if m.gate != nil || m.question != nil || m.approval != nil {
		return false
	}
	if m.pendingPrompt != "" {
		return true
	}
	if m.flowHasActiveAgents() {
		return true
	}
	if m.hasUnresolvedAttention() {
		return false
	}
	if m.flowLoopBlocked() {
		return false
	}
	if m.connStatus == ConnRunning {
		return true
	}
	if m.turnStream != nil || m.focusedChildLive() {
		return true
	}
	// Flow/step chrome: a live (non-terminal) handle is still working even on
	// ConnIdle; reuse the poll predicate so the spinner only stops once the run
	// is terminal.
	if m.shouldPollStepsRuntime() {
		return true
	}
	return false
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

	// Never arm a run without a project_id: cmdStartRun re-fetches the catalog
	// with an unbounded context and hung the TUI when the catalog was slow/empty
	// (CA-514). When defaults are loaded but no project is bound, chat is
	// disabled — record the draft line, clear it, and refuse fast. A late
	// ProjectsCatalogMsg can still bind the project so the user can re-send.
	if m.sessionDefaultsLoaded && m.project == nil && m.runHandle == nil {
		if m.bindProjectIfPossible() {
			m.refreshSessionPanel()
		} else {
			m.statusMsg = "chat disabled — no project_id"
			m.addMessage("user", input, "")
			m.addMessage("system", formatMissingProjectHelp(m.cfg.ProjectPath, m.projects), "error")
			return m, nil
		}
	}

	m.viewport.offset = 0
	m.addMessage("user", input, "")
	m.recordPromptHistory(input)
	// CA-537: no "Thinking" row in the chat timeline — the spinner animates on
	// the status line + F2 RUNNING step instead (workIsLive starts the ticker).
	m.statusMsg = "thinking…"
	m.connStatus = ConnRunning

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

// isFlowChrome is true for catalog/step/flow runs where the status line favors
// step/flow progress (F2 steps) over generic "turn running…" labels.
func (m *AppModel) isFlowChrome() bool {
	if m == nil {
		return false
	}
	return m.mode == ModeFlow || m.mode == ModeStep || m.launch.IsCatalogWorkflow()
}

// showFlashToast sets a 1s status-area toast (not a chat timeline message).
func (m *AppModel) showFlashToast(text string) tea.Cmd {
	m.flashToastID++
	id := m.flashToastID
	m.flashToast = strings.TrimSpace(text)
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return toastClearMsg{ID: id}
	})
}

// handleGateInput interprets user input when a flow_gate_violation is pending.
// Accepts option number (1-based) or option name (case-insensitive).
func (m *AppModel) handleGateInput(input string) (tea.Model, tea.Cmd) {
	// CA-536 escape hatch: a gate armed with no decision options can never be
	// answered. Clear it so the next Enter reaches chat instead of looping the
	// empty "Gate options:" prompt forever.
	if m.gate == nil || len(m.gate.Options) == 0 {
		m.gate = nil
		return m, nil
	}
	lowInput := strings.TrimSpace(strings.ToLower(input))
	for i, opt := range m.gate.Options {
		if lowInput == fmt.Sprintf("%d", i+1) || strings.EqualFold(lowInput, opt) {
			runID := m.gate.RunID
			m.gate = nil
			m.connStatus = ConnRunning
			// CA-537: the flow keeps working after a gate decision — the spinner
			// animates on the status line + F2 RUNNING step, not as a chat row.
			m.statusMsg = "thinking…"
			return m, m.cmdSubmitGateDecision(runID, opt)
		}
	}
	opts := strings.Join(m.gate.Options, ", ")
	m.addMessage("system", fmt.Sprintf("Gate options: %s (enter number or name)", opts), "gate")
	return m, nil
}

// dispatchImageCommand handles /image [paste|open|clear|rm|list|<path>].
// Reserved subcommands must never fall through to os.ReadFile (that produced
// "read paste: open paste: invalid argument" when paste was treated as a path).
func (m *AppModel) dispatchImageCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		if len(m.pendingAttach) > 0 {
			m.openAttachPanel()
		}
		m.addMessage("system", formatPendingAttachments(m.pendingAttach)+
			"\nTip: click [N img] to manage · Alt+V / /image paste · /image rm <n> · /image clear", "")
		return m, nil
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "list", "ls", "panel", "manage":
		if len(m.pendingAttach) > 0 {
			m.openAttachPanel()
		}
		m.addMessage("system", formatPendingAttachments(m.pendingAttach), "")
		return m, nil
	case "paste", "clip", "clipboard":
		return m, m.cmdClipboardPaste()
	case "clear":
		n := m.clearPendingAttachments()
		m.addMessage("system", fmt.Sprintf("Cleared %d pending image(s).", n), "")
		return m, nil
	case "rm", "remove", "del", "delete", "x":
		if len(args) < 2 {
			m.addMessage("system", "Usage: /image rm <n>  (1-based index from /image list)", "error")
			return m, nil
		}
		idx, err := strconv.Atoi(strings.TrimSpace(args[1]))
		if err != nil {
			m.addMessage("system", "Usage: /image rm <n>  (1-based index)", "error")
			return m, nil
		}
		name, ok := m.removePendingAttachment(idx)
		if !ok {
			m.addMessage("system", fmt.Sprintf("image index %d out of range (1-%d)", idx, len(m.pendingAttach)+1), "error")
			return m, nil
		}
		m.addMessage("system", fmt.Sprintf("Removed pending image: %s (%d left)", name, len(m.pendingAttach)), "")
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
	case "paste", "clip", "clipboard", "list", "ls", "clear", "open", "view", "show",
		"rm", "remove", "del", "delete", "x", "panel", "manage":
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
	for _, att := range attachments {
		m.appendPendingAttachment(att)
	}
	m.addMessage("system", fmt.Sprintf("Attached image: %s (%d pending) — click [%d img] to manage",
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
		m.persistSessionPrefs()
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
		if len(args) > 0 {
			target := strings.Join(args, " ")
			runID, name, ok := m.resolveAgentFocusTarget(target)
			if !ok {
				m.addMessage("system", fmt.Sprintf("Agent %q not found. Try /agents.", target), "error")
				break
			}
			_ = name
			return m, m.cmdFocusAgent(runID)
		}
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
		b.WriteString("Agents (Tab cycles, /agent <name> or step [open] opens transcript):\n")
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
		_ = name
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
		m.persistSessionPrefs()
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
		if m.runHandle != nil && m.mode != ModeChat {
			m.addMessage("system", "Cannot switch to chat while a run is open. Use /new first.", "error")
			break
		}
		m.mode = ModeChat
		m.launch = LaunchArm{}
		m.firstTurnPending = false
		m.persistSessionPrefs()
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
				m.persistSessionPrefs()
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
			m.persistSessionPrefs()
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
				m.persistSessionPrefs()
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
		_ = m.clearPendingAttachments()
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
		m.persistSessionPrefs()
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
		// A blocked (awaiting-user) flow has turnIsActive()==false but Stop is
		// still valid — the user can end the parked loop (Desktop FlowAwaitingUser
		// Stop parity, BUG-231).
		if !m.turnIsActive() && !m.flowLoopBlocked() {
			m.addMessage("system", "Nothing to stop.", "")
			break
		}
		return m, m.cmdStopTurn()

	case "/continue":
		// Desktop continueFlow parity (BUG-231): unblock a parked blocked flow.
		if !m.flowLoopBlocked() {
			m.addMessage("system", "No parked (blocked) flow to continue.", "")
			break
		}
		if m.runHandle == nil {
			m.addMessage("system", "No run to continue.", "")
			break
		}
		return m, m.cmdContinueFlow(m.runHandle.RunID)

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

	fullW := m.width
	if fullW <= 0 {
		fullW = 80
	}
	m.fullWidth = fullW
	useSide := m.useRightSidebar()

	var sideLines []string
	var sideW, sideX int
	if useSide {
		sideW = m.sideWidth()
		sideX = fullW - sideW
		sideLines = m.renderRightSidebar(m.height)
	}

	c := m.tuiChrome()
	var rows []string

	rows = append(rows, c.panelLines...)

	lines := m.renderMessages()
	// Freeze the viewport while the user is mid-select: clamping against a line
	// count that changed (width flip / live stream) would yank the text out from
	// under the cursor / reset a top-of-history drag to the bottom (CA-526).
	if !m.selecting() {
		m.clampViewport(len(lines), c.messagesHeight)
	}
	lines = sliceViewport(lines, c.messagesHeight, m.viewport.offset)
	if !m.mouseSel.empty() {
		lines = applyMouseSelection(lines, m.mouseSel, c.panelH)
	}
	rows = append(rows, lines...)

	for i := len(lines); i < c.messagesHeight; i++ {
		rows = append(rows, "")
	}

	if m.sessionLoading {
		for _, line := range strings.Split(m.loadingBannerText(), "\n") {
			rows = append(rows, styleLoading.Render(line))
		}
	} else if m.authNeedLogin && m.authPhase == AuthNone {
		banner := "SIGN IN REQUIRED — Desktop is signed out. Type /login (or /login you@email.com)"
		if !m.asciiMode {
			banner = "! " + banner
		}
		rows = append(rows, styleError.Render(banner))
	}
	rows = append(rows, "")
	w := m.chatWidth()
	if w <= 0 {
		w = 80
	}
	if m.asciiMode {
		rows = append(rows, strings.Repeat("-", w))
	} else {
		rows = append(rows, strings.Repeat("─", w))
	}
	// flashToast is rendered on the project/git status row (see status_bar.go).
	rows = append(rows, strings.Split(c.statusBlock, "\n")...)
	if len(c.sugg) > 0 {
		rows = append(rows, strings.Split(m.renderSuggestions(c.sugg), "\n")...)
	}
	if c.attachPanelBlock != "" {
		rows = append(rows, strings.Split(c.attachPanelBlock, "\n")...)
	}
	inputStart := len(rows)
	rows = append(rows, strings.Split(m.renderInputLine(), "\n")...)

	// CA-532: paint the dark canvas across the whole chat column and give the chat
	// bar a lighter elevated background. The right sidebar column is painted in
	// joinRightSidebar. Every row is padded so the background fills the row; the
	// chat-bar (input) rows keep one free last column (safeTermWidth) so Windows
	// Terminal never wraps the composer.
	rows = padLinesTo(rows, m.height)
	barW := safeTermWidth(w)
	for i, r := range rows {
		st := styleCanvas
		rw := w
		if i >= inputStart {
			st = styleChatBar
			rw = barW
		}
		rows[i] = paintRow(r, rw, st)
	}

	if useSide {
		return joinRightSidebar(rows, sideLines, sideW, sideX, m.asciiMode)
	}

	return strings.Join(rows, "\n")
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
	// ToolGroupKey is non-empty for the summary row of a collapsed run of 2+
	// consecutive tool calls (CA-525). Clicking it toggles the run expansion.
	ToolGroupKey string
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
	_, _ = h.Write([]byte(strconv.Itoa(m.chatWidth())))
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
	// CA-537: no thinking row renders in the chat timeline, so the chat cache is
	// independent of thinkingFrame (the status line + F2 spinner re-render via
	// the tick message, not through chatRows).
	_, _ = h.Write([]byte(strconv.Itoa(m.visiblePromptCount)))
	_, _ = h.Write([]byte(strconv.FormatInt(m.historyLoadedAfterSeq, 10)))
	keys := make([]string, 0, len(m.expandedToolGroups))
	for k, v := range m.expandedToolGroups {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = h.Write([]byte{2})
		_, _ = h.Write([]byte(k))
	}
	return h.Sum64()
}

// toolGroupKey derives a stable identity for a run of consecutive tool calls from
// its content alone, so the expanded/collapsed state survives thinking-placeholder
// reordering (replaceThinkingAt moves messages, which shifts their indices).
func toolGroupKey(tools []ChatMessage) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, strings.TrimSpace(strings.TrimPrefix(t.Content, "→ ")))
	}
	return strings.Join(names, "\x1f")
}

// toolGroupRows renders a run of 2+ consecutive tool calls (CA-525) as a single
// collapsible summary row. Collapsed by default; the individual → tool lines are
// emitted only after the user clicks the summary (toggleToolGroup).
func (m *AppModel) toolGroupRows(tools []ChatMessage) []chatRow {
	key := toolGroupKey(tools)
	expanded := m.expandedToolGroups[key]
	marker, markerOpen := "▸", "▾"
	if m.asciiMode {
		marker, markerOpen = "+", "-"
	}
	if expanded {
		marker = markerOpen
	}
	label := fmt.Sprintf("%d tool call", len(tools))
	if len(tools) > 1 {
		label += "s"
	}
	text := styleTool.Render(marker+" "+label) + styleLink.Render(" · click")
	rows := []chatRow{{Text: text, MsgIdx: -1, ToolGroupKey: key}}
	if expanded {
		for _, t := range tools {
			rows = append(rows, chatRow{Text: styleTool.Render(t.Content), MsgIdx: -1})
		}
	}
	return rows
}

func (m *AppModel) toggleToolGroup(key string) {
	if m.expandedToolGroups == nil {
		m.expandedToolGroups = map[string]bool{}
	}
	expanding := !m.expandedToolGroups[key]
	m.expandedToolGroups[key] = expanding
	m.rowCache = nil
	m.rowCacheSig = 0
	// CA-527: the transcript is bottom-anchored (offset = rows from the bottom),
	// so expanding a group inserts N tool rows after the summary without moving
	// the anchor — the list would push UP and scroll off-screen. Shift the offset
	// by ±N so the summary (and everything above it) stays pinned and the group
	// expands downward. Skipped while the user is mid-drag (CA-526 freeze).
	if !m.selecting() {
		n := strings.Count(key, "\x1f") + 1
		if expanding {
			m.viewport.offset += n
		} else {
			m.viewport.offset -= n
		}
		if m.viewport.offset < 0 {
			m.viewport.offset = 0
		}
	}
}

func (m *AppModel) buildChatRows() []chatRow {
	width := safeTermWidth(m.chatWidth())
	if width < 1 {
		if m.chatWidth() > 0 {
			width = m.chatWidth()
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
		// CA-537: the animated spinner lives on the status line + F2 RUNNING step,
		// never as a chat-timeline row. Any stray thinking placeholder (legacy
		// replay) is skipped so it stays invisible in the chat.
		if msg.FormatHint == "thinking" {
			continue
		}
		// CA-525: a run of 2+ consecutive tool calls renders as one collapsible
		// summary row (click to expand) instead of N stacked tool lines.
		if msg.Role == "tool" {
			end := mi + 1
			for end < len(m.messages) && m.messages[end].Role == "tool" {
				end++
			}
			if end-mi > 1 {
				rows = append(rows, m.toolGroupRows(m.messages[mi:end])...)
				mi = end - 1
				continue
			}
		}
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
	w := m.chatWidth()
	if w <= 0 {
		w = 80
	}
	w = safeTermWidth(w)
	if m.viewingChild() {
		// Desktop parity: a focused sub-agent transcript is read-only — chat may
		// only continue on the main run. Render a locked banner instead of an
		// editable composer (CA-519).
		msg := " Child transcript is read-only — chat continues on main (/agent main or [back]) "
		return styleSystem.Render(truncateVisual(msg, w))
	}
	if m.sessionLoading && !strings.HasPrefix(strings.TrimSpace(m.slashSuggestLine()), "/") {
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
	// Only show when images are pending — bare "[+img]" looked like an attachment.
	attachPlain := m.inputAttachChipPlain()
	attach := ""
	if attachPlain != "" {
		attach = stylePromptFocus.Render(attachPlain)
	}
	innerW := w - 2
	if innerW < 1 {
		innerW = 1
	}
	// Drop the attach chip before it forces the box past the terminal width.
	if attach != "" && lipgloss.Width(stripANSI(attach))+8 > innerW {
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
	if bar := m.renderAttentionBar(); bar != "" {
		inner = append(inner, strings.Split(bar, "\n")...)
	}
	if bar := m.renderBlockedBar(); bar != "" {
		inner = append(inner, strings.Split(bar, "\n")...)
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
		// Highlight attached skill mentions [name] in the draft (chip + inject still separate).
		hiSel := m.selectedSkills
		if m.authPhase != AuthNone {
			hiSel = nil // never style password/email
		}
		if onLine {
			if caretCol < 0 || caretCol > len(shown) {
				caretCol = len(shown)
			}
			styled = styleInputBodyWithSkillTokens(string(shown[:caretCol]), hiSel) + caret +
				styleInputBodyWithSkillTokens(string(shown[caretCol:]), hiSel)
		} else {
			styled = styleInputBodyWithSkillTokens(string(shown), hiSel)
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
	// A fresh thinking placeholder restarts the elapsed spinner at 0.
	if hint == "thinking" {
		m.thinkingFrame = 0
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
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
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
		// Start the slow catalog fetch FIRST so it overlaps the account/provider
		// path during the FlowPilot banner phase. The banner already says
		// "loading session · project · providers — chat locked", so the project
		// catalog belongs to that phase — not a post-banner retry (CA-514).
		type catalogResult struct {
			projects []client.Project
			err      error
		}
		catalogCh := make(chan catalogResult, 1)
		go func() {
			cl := client.New(runnerURL)
			ctxProjects, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ps, err := cl.ListProjects(ctxProjects)
			catalogCh <- catalogResult{projects: ps, err: err}
		}()

		// Provider/account path. Accounts are a local config read (fast); the
		// providers scan can spawn CLI probes on the runner, so it gets a short
		// independent budget instead of the shared fast-path window — a cold
		// probe scan must never hold the session unlock for the full window
		// (CA-535). The runner's /providers handler also honors r.Context()
		// cancel and caches results, so in practice this resolves in ms.
		cl := client.New(runnerURL)
		ctxFast, cancelFast := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancelFast()
		accounts, accErr := cl.ListProviderAccounts(ctxFast)
		ctxProvs, cancelProvs := context.WithTimeout(context.Background(), 2*time.Second)
		providers, provErr := cl.ListProviders(ctxProvs)
		cancelProvs()
		provider, model, label := pickActiveSessionDefaults(flagProvider, flagModel, accounts, providers)

		cat := <-catalogCh
		var (
			projects   []client.Project
			project    *client.Project
			account    *client.ProviderAccountSummary
			catalogErr string
		)
		if cat.err == nil {
			projects = cat.projects
			project = matchProjectByPath(projects, projectPath)
		} else {
			catalogErr = cat.err.Error()
		}
		// Prefer a dial-level failure as the catalog message when projects is empty
		// (a ctx deadline on the account path is not runner-dead evidence).
		if catalogErr == "" {
			if accErr != nil && runnerDialDeadErr(accErr.Error()) {
				catalogErr = accErr.Error()
			} else if provErr != nil && runnerDialDeadErr(provErr.Error()) {
				catalogErr = provErr.Error()
			}
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

// cmdLoadProjectsCatalog retries GET /client/projects with a long timeout.
// Used when the first session load hit a catalog timeout while the runner is up.
func (m *AppModel) cmdLoadProjectsCatalog() tea.Cmd {
	runnerURL := m.runnerURL
	projectPath := m.cfg.ProjectPath
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		ps, err := cl.ListProjects(ctx)
		if err != nil {
			return ProjectsCatalogMsg{Err: err.Error()}
		}
		return ProjectsCatalogMsg{
			Projects: ps,
			Project:  matchProjectByPath(ps, projectPath),
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
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
	line := m.slashSuggestLine()
	ok, _ := parseFlowArgPrefix(line)
	if !ok && !strings.EqualFold(strings.TrimSpace(line), "/flow") {
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
				// Bound this fallback: an unbounded ListProjects re-fetch is what
				// hung the TUI in ConnRunning when the catalog was slow (CA-514).
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if ps, err := cl.ListProjects(ctx); err == nil {
					projects = ps
				}
				cancel()
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

func (m *AppModel) cmdContinueFlow(runID string) tea.Cmd {
	runnerURL := m.runnerURL
	parentID := runID
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		g, err := cl.ContinueFlow(ctx, parentID)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return AgentGraphHydratedMsg{ParentRunID: parentID, Graph: g}
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
