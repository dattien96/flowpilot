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
	// Posture chip colors — each posture gets its own hue on the status line.
	colorPostureScan = "#38bdf8" // sky — scan (read-only)
	colorPosturePlan = "#f59e0b" // amber — plan (read-only)
	colorPostureCode = "#3fb950" // green — code (normal gated chat)
)

var (
	styleUserLabel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleUser        = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPromptText))
	styleAssistant   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleSystem      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleTool        = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn))
	styleError       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	styleGate        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAsk))
	styleStatus      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleStatusHi    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)) // model, reason value, YOLO value, 7d, skills
	styleMention     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))     // skill tokens in prompt
	styleMentionFile = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)) // @file paths in prompt
	styleStatusOK    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))
	styleStatusErr   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	// agent:NAME and flow-name values — dedicated hues, not styleStatusHi/accent.
	styleStatusAgent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorStatusAgent))
	styleStatusFlow  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorStatusFlow))
	// Posture chip colors — dedicated hues for scan/plan/code on the status line.
	stylePostureScan = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorPostureScan))
	stylePosturePlan = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorPosturePlan))
	stylePostureCode = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorPostureCode))
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
	initTUILog()
	tuiLog("New() provider=%q model=%q reasoning=%q runner=%s project=%q", cfg.Provider, cfg.Model, cfg.ReasoningEffort, runnerURL, cfg.ProjectPath)
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
	tuiLog("New() done provider=%q model=%q yolo=%v mode=%q width=%d", m.provider, m.model, m.yolo, m.mode.String(), m.width)
	return m
}

// Init is the Bubble Tea Init function.
func (m *AppModel) Init() tea.Cmd {
	tuiLog("Init() -> cmdConnect + tickCursor")
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
	// Log startup-relevant messages (skip high-frequency ticks to keep log readable).
	switch v := msg.(type) {
	case cursorTickMsg, thinkingTickMsg, tea.WindowSizeMsg:
	default:
		if km, ok := msg.(tea.KeyMsg); ok {
			tuiLog("Update KeyMsg Type=%v String=%q Paste=%v Runes=%q", km.Type, km.String(), km.Paste, string(km.Runes))
		} else {
			tuiLog("Update %T %v", msg, v)
		}
	}
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
		// Drive sync/restore spinner (CA-551): same always-on cursor tick driver
		// as the thinking ticker, but for the /sync and /restore batches.
		if m.driveSync != nil || m.restoreBatch != nil {
			if !m.driveSyncTickerActive {
				m.driveSyncTickerActive = true
				m.driveSyncFrame = 0
				cmds = append(cmds, cmdDriveSyncTick())
			}
		} else {
			m.driveSyncTickerActive = false
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

	case driveSyncTickMsg:
		m.driveSyncFrame++
		if m.driveSync != nil || m.restoreBatch != nil {
			return m, cmdDriveSyncTick()
		}
		m.driveSyncTickerActive = false
		return m, nil

	case ErrMsg:
		m.err = msg.Err
		m.sessionLoading = false
		m.pendingPrompt = ""
		m.turnSendPending = false
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

	case chatPostureMsg:
		if msg.Err != nil {
			if m.chatPostureSaving {
				posture := m.chatPostureSavingPosture
				m.chatPostureSaving = false
				m.chatPostureSavingPosture = ""
				m.chatPosturePending = ""
				m.addMessage("system", fmt.Sprintf("Posture %s save failed: %s", posture, msg.Err.Error()), "error")
				return m, nil
			}
			m.chatPosturePending = ""
			m.addMessage("system", "Chat posture error: "+msg.Err.Error(), "error")
			return m, nil
		}
		cmd := m.chatPostureCmdFromPending(msg.Cfg)
		if m.chatPostureDirty {
			m.chatPostureDirty = false
			return m, tea.Batch(m.cmdSaveChatPosture(m.chatPostureCfg), cmd)
		}
		// PUT completed for /mode-setup edit — replace the "saving…" banner.
		if m.chatPostureSaving {
			posture := m.chatPostureSavingPosture
			m.chatPostureSaving = false
			m.chatPostureSavingPosture = ""
			m.chatPostureCfg = msg.Cfg
			if posture != "" {
				m.addMessage("system", fmt.Sprintf("Posture %s saved.", posture), "")
			}
			return m, cmd
		}
		return m, cmd

	case grokSyncFailedMsg:
		// Grok YOLO posture sync failed — re-enable the flag so the next
		// turn retries. The turn already sent (sync failure does not block
		// the conversation), but Grok's runtime config may be stale.
		m.postureGrokSync = msg.Yolo
		m.postureGrokSyncSet = true
		return m, nil

	case ApprovalResolvedMsg:
		shown := m.pendingApprovalShown(msg.ID)
		m.removeApproval(msg.ID)
		m.connStatus = ConnRunning
		m.statusMsg = "approved"
		if !shown {
			return m, nil
		}
		label := "Approved."
		if strings.EqualFold(msg.Decision, "deny") {
			label = "Denied."
		}
		if m.approval != nil {
			// More cards in the queue: stay waiting so the next head can be
			// resolved (BUG-157/158 parallel approvals).
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			m.addMessage("system", fmt.Sprintf("%s %d more pending — /approve-all resolves the rest.", label, len(m.approvals)), "")
			return m, nil
		}
		m.addMessage("system", label, "")
		return m, nil

	case QuestionResolvedMsg:
		shown := m.pendingQuestionShown(msg.ID)
		m.removeQuestion(msg.ID)
		m.connStatus = ConnRunning
		m.statusMsg = "answered"
		m.addMessage("system", "Answered: "+msg.Choice, "question")
		if shown && m.question != nil {
			// More cards in the queue: keep answering the next head.
			m.connStatus = ConnWaiting
			m.statusMsg = "question"
			m.addMessage("system", fmt.Sprintf("%d question(s) still pending.", len(m.questions)), "question")
		}
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
		// Freeze liveness so the status bar does not keep "Thinking Ns" and
		// the spinner/elapsed clock stops. The backend run is interrupted by
		// user — the local handle and its agents/steps are still marked
		// "running" until the next poll, which would keep workIsLive() and
		// shouldPollStepsRuntime() true and the 27s timer ticking.
		if m.runHandle != nil {
			m.runHandle.Status = "stopped"
		}
		for i := range m.agentRuns {
			if !runStatusIsTerminal(m.agentRuns[i].Status) {
				m.agentRuns[i].Status = "stopped"
			}
		}
		for i := range m.flowSteps {
			st := strings.ToUpper(strings.TrimSpace(m.flowSteps[i].Status))
			if st == "RUNNING" || st == "WAITING_USER_APPROVAL" {
				m.flowSteps[i].Status = "CANCELLED"
			}
		}
		m.flowStepsActive = ""
		m.turnStream = nil
		m.thinkingTickerActive = false
		m.thinkingFrame = 0
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
		// A successful selection copy clears the drag highlight so the UI returns
		// to normal mode instead of staying in "copied" state (CA-543).
		if kind == "selection" {
			m.mouseSel = mouseSelect{}
		}
		return m, m.showFlashToast("Copied " + kind + ".")

	case toastClearMsg:
		if msg.ID == m.flashToastID {
			m.flashToast = ""
		}
		return m, nil

	case ConnectedMsg:
		tuiLog("ConnectedMsg runner=%s -> sessionLoading=true", msg.RunnerURL)
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
		tuiLog("SessionDefaultsMsg firstLoad=%v providers=%d accounts=%d projects=%d catalogErr=%q", !m.sessionDefaultsLoaded, len(msg.Providers), len(msg.ProviderAccounts), len(msg.Projects), msg.CatalogErr)
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
		// CP-56 restart restore: TUI must resume the runner's persisted active
		// posture (scan/plan/code) and its pinned profile after reopen — the
		// runner is SSOT, the TUI default is code, so without this GET the mode
		// looks lost. Not a user switch, so the restore does not PUT.
		if firstLoad {
			m.chatPosturePending = "restore"
			cmds = append(cmds, m.cmdLoadChatPosture())
		}
		return m, tea.Batch(cmds...)

	case WorkspaceFilesMsg:
		query, start, ok := activeAtFragment(m.inputValue, m.inputCaretIndex())
		if !ok || isAgentAtMention(start, query, m.agentRuns) || msg.Query != query {
			return m, nil
		}
		if msg.Err != "" {
			m.workspaceFiles = []string{}
			m.workspaceFilesQuery = msg.Query
			return m, nil
		}
		m.workspaceFiles = msg.Paths
		if m.workspaceFiles == nil {
			m.workspaceFiles = []string{}
		}
		m.workspaceFilesQuery = msg.Query
		return m, nil

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
				m.chatList = mergeChatListSyncStatus(m.chatList, msg.Items)
			}
			return m, nil
		}
		if msg.Err != "" {
			m.addMessage("system", "Chat list failed: "+msg.Err, "error")
			return m, nil
		}
		m.chatList = mergeChatListSyncStatus(m.chatList, msg.Items)
		m.addMessage("system", formatChatListWithRemote(msg.Items, m.remoteChatList), "")
		return m, nil

	case DriveSyncBatchMsg:
		// G2 /sync progress: update the badge for the finished row, then either
		// start the next upload or print the batch summary (Desktop Navigator
		// "Synced n/m" parity). One HTTP per Update — the batch never freezes the
		// composer and a single failure does not abort the rest.
		st := m.driveSync
		if st == nil {
			// Stale message (batch already finished/reset): surface the result
			// without touching counters.
			if msg.Err != nil {
				m.addMessage("system", formatDriveSyncErr(msg.RunID, msg.Err), "error")
			}
			return m, nil
		}
		m.markChatSyncStatus(msg.RunID, msg.Err, msg.Result)
		st.done++
		if msg.Err != nil {
			st.failed++
		}
		if len(st.queue) == 0 {
			m.driveSync = nil
			m.addMessage("system", formatDriveSyncSummary(st), "")
			return m, nil
		}
		next := st.queue[0]
		st.queue = st.queue[1:]
		return m, m.cmdSyncRun(next, st.projectID)

	case EngineInitMsg:
		return m.handleEngineInitMsg(msg)

	case RemoteChatListMsg:
		// G3 /restore index (silent refresh after a batch, loud bare dump).
		if msg.Err != "" {
			m.addMessage("system", "Remote chat list failed: "+msg.Err, "error")
			return m, nil
		}
		m.remoteChatList = msg.Items
		if !msg.Silent {
			m.addMessage("system", formatRemoteChatList(msg.Items), "")
		}
		return m, nil

	case RestoreBatchMsg:
		// G3 /restore progress: sequential queue; single restore opens the
		// restored chat on success, /restore all stays silent and refreshes
		// both lists afterwards.
		st := m.restoreBatch
		if st == nil {
			if msg.Err != nil {
				m.addMessage("system", formatRestoreErr(msg.SourceKey, msg.Err), "error")
			}
			return m, nil
		}
		st.done++
		if msg.Err != nil {
			st.failed++
		}
		if len(st.queue) == 0 {
			m.restoreBatch = nil
			if st.openAfter {
				if msg.Err == nil && msg.Result != nil && strings.TrimSpace(msg.Result.RunID) != "" {
					m.addMessage("system", fmt.Sprintf("Restored %s from Drive.", msg.SourceKey), "")
					return m, m.cmdOpenChat(msg.Result.RunID)
				}
				m.addMessage("system", formatRestoreErr(msg.SourceKey, msg.Err), "error")
				return m, nil
			}
			m.addMessage("system", formatRestoreSummary(st), "")
			return m, tea.Batch(m.cmdFetchChats(true), m.cmdFetchRemoteChats(true))
		}
		next := st.queue[0]
		st.queue = st.queue[1:]
		return m, m.cmdRestoreOne(next, st.projectID, st.cwd)

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
		m.clearPendingDecisions()
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
		m.turnSendPending = false
		m.connStatus = ConnRunning
		m.statusMsg = "streaming…"
		if m.isFlowChrome() {
			if m.flowStepsActive != "" {
				m.statusMsg = "step: " + m.flowStepsActive
			} else if m.flowLoopDone() {
				// run-107774: after a flow is done, a follow-up turn is plain hub
				// chat, not a re-run — keep the chat label ("thinking…") instead of
				// claiming the flow restarted.
				m.statusMsg = "thinking…"
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
		m.turnSendPending = false
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

	case pasteBurstSettleMsg:
		// A raw-paste burst finished quietly — collapse it to a token without
		// waiting for the next keystroke.
		if !m.pasteBurst.active {
			return m, nil
		}
		if pasteNow().Sub(m.pasteBurst.lastRuneAt) >= burstSettle {
			m.collapsePasteBurst()
			tuiLog("burst collapse active=false inputLen=%d", len([]rune(m.inputValue)))
		} else {
			// Fired early (e.g. 129ms <150ms due to 15 runes each scheduling a tick) — reschedule.
			tuiLog("burst settle early, reschedule active=true")
			return m, cmdPasteBurstSettle()
		}
		return m, nil

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
			// Same paste-summary handling as bracketed paste: long blocks show a
			// token and expand on submit. NUL/control bytes from the clipboard
			// are stripped so the message copies cleanly (run-117747).
			msg.Text = sanitizePasteText(msg.Text)
			if needsPasteSummary(msg.Text) {
				m.insertPasteSummary(msg.Text)
			} else {
				m.insertInputAtCursor(msg.Text)
			}
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
		// Post-done follow-up chat (run-107774) is plain hub chat, so deltas may
		// surface "streaming…" there again.
		if m.connStatus == ConnRunning && (!m.isFlowChrome() || m.flowLoopDone()) {
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
		// run-107774: once the loop is done, a completed follow-up turn must not
		// re-arm "flow running…" — the flow is finished, settle to done directly.
		if !m.flowLoopDone() && m.shouldPollStepsRuntime() && (m.orchStream != nil || m.flowHasActiveAgents()) {
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
			} else if m.flowLoopDone() {
				// run-107774: post-done follow-up is a plain hub chat turn, not a
				// flow restart — use the chat label instead of "flow running…".
				m.statusMsg = "turn running…"
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
			if ev.Decision != "" {
				// Replay of an already-resolved approval (BUG-ApprovalReplay-Restart):
				// render read-only instead of re-mounting an interactive card the
				// run no longer waits on.
				m.connStatus = ConnRunning
				m.statusMsg = "approval resolved"
				m.addMessage("system", fmt.Sprintf("[APPROVAL] %s already resolved: %s", ev.ApprovalID, ev.Decision), "approval")
				return m, nil
			}
			st := ApprovalState{
				ID:    ev.ApprovalID,
				RunID: ev.WorkflowRunID,
			}
			if d := ev.Details; d != nil {
				st.Command = d.Command
				st.Cwd = d.Cwd
				st.Reason = d.Reason
				st.Kind = d.Kind
				st.Decisions = d.Decisions
			}
			added := m.pushApproval(st)
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			if added {
				m.addMessage("system", formatApprovalWaitingLineDetailed(ev.ApprovalID, m.asciiMode, st), "approval")
			}
		}

	case "user_question_required":
		if ev.QuestionID != "" {
			if len(ev.Answer) > 0 {
				// Replay of an already-resolved question: render read-only, never
				// mount an interactive card the run no longer waits on (G3 parity).
				m.connStatus = ConnRunning
				m.statusMsg = "question answered"
				m.addMessage("system", fmt.Sprintf("[QUESTION] %s already answered: %s", ev.QuestionID, strings.Join(ev.Answer, ", ")), "question")
				break
			}
			opts := make([]map[string]string, 0, len(ev.Options))
			for _, o := range ev.Options {
				opts = append(opts, o)
			}
			added := m.pushQuestion(QuestionState{
				ID:          ev.QuestionID,
				Prompt:      ev.Prompt,
				Options:     opts,
				MultiSelect: ev.MultiSelect,
				RunID:       ev.WorkflowRunID,
			})
			m.connStatus = ConnWaiting
			m.statusMsg = "question"
			if added {
				m.addMessage("system", formatQuestionMessage(ev.Prompt, opts, ev.MultiSelect), "question")
			}
		}

	case "flow_gate_violation":
		// CA-536 (run-103672): only a genuine block with a decision card may
		// lock the composer. The runner emits this event for warn/reprompt
		// verdicts too (and for blocks without r-reg options), which carry
		// empty GateOptions — arming the gate then made every keystroke
		// re-print "Gate options:" with nothing to match, permanently freezing
		// chat. Non-block / option-less verdicts surface as info instead.
		// CA-545 (run-204658): reprompt/warn and block-without-options must not
		// render a misleading "Options:" line — they auto-reprompt or are info
		// only. buildGateMessage now varies by status + error.
		blocking := strings.EqualFold(ev.Status, "block") && len(ev.GateOptions) > 0
		m.addMessage("system", buildGateMessage(ev.Status, ev.Error, ev.GateOptions, ev.GateRegressedTests), "gate")
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
	if msg.Paste {
		tuiLog("handleKey Paste runes=%d type=%v", len(msg.Runes), msg.Type)
	}
	// Never hard-block keyboard while loading — a hung runner previously made
	// the TUI feel fully frozen. processInput still rejects non-slash *send*.
	if m.sessionLoading && msg.Type == tea.KeyCtrlC {
		// Allow quit during load without waiting for catalog.
		m.quitting = true
		return m, m.cmdShutdownAndQuit()
	}

	if m.authPhase == AuthNone && !m.viewingChild() {
		now := pasteNow()
		if m.pasteBurst.active && now.Sub(m.pasteBurst.lastRuneAt) >= burstSettle {
			m.collapsePasteBurst()
		}
		// Raw (non-bracketed) paste arrives as a flood of key events where every
		// line break is a plain Enter. Swallow those Enters as newlines while the
		// flood is active so pasting never auto-submits per line.
		if isBurstNewlineKey(msg) && m.handleBurstNewline(now) {
			return m, cmdPasteBurstSettle()
		}
		if isPromptNewlineKey(msg) || isModifiedEnterNewline(msg) {
			m.inputValue += "\n"
			return m, nil
		}
	}

	// Modal takes precedence over all other key handling.
	if m.modeSetupModalOpen {
		return m.handleModeSetupModalKey(msg)
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
		// Wizard back navigation for /mode-setup
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(m.inputValue)), "/mode-setup") {
			in := strings.TrimSpace(m.inputValue)
			parts := strings.Fields(in)
			if len(parts) <= 1 {
				if m.modeSetupDraftDirty {
					m.modeSetupDraft = nil
					m.modeSetupDraftDirty = false
					m.clearInputValue()
					m.addMessage("system", "Posture draft discarded.", "")
					return m, nil
				}
				m.clearInputValue()
				m.statusMsg = "prompt cleared"
				return m, nil
			}
			if len(parts) == 2 {
				m.setInputPreservingDraftPrefix("/mode-setup ")
				m.suggIdx = 0
				return m, m.cmdMaybePrefetchPickers()
			}
			if len(parts) == 3 {
				m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " ")
				m.suggIdx = 0
				return m, m.cmdMaybePrefetchPickers()
			}
			m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " " + parts[2] + " ")
			m.suggIdx = 0
			return m, m.cmdMaybePrefetchPickers()
		}
		if m.inputValue != "" {
			m.clearInputValue()
			m.statusMsg = "prompt cleared"
			return m, nil
		}
		m.statusMsg = "Ctrl-C or /exit to quit"
		return m, nil

	case tea.KeyShiftTab:
		// Wizard back: Shift-Tab goes up one level in /mode-setup
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(m.inputValue)), "/mode-setup") {
			in := strings.TrimSpace(m.inputValue)
			parts := strings.Fields(in)
			if len(parts) <= 1 {
				if m.modeSetupDraftDirty {
					m.modeSetupDraft = nil
					m.modeSetupDraftDirty = false
					m.clearInputValue()
					m.addMessage("system", "Posture draft discarded.", "")
					return m, nil
				}
				m.clearInputValue()
				return m, nil
			}
			if len(parts) == 2 {
				m.setInputPreservingDraftPrefix("/mode-setup ")
				m.suggIdx = 0
				return m, m.cmdMaybePrefetchPickers()
			}
			if len(parts) == 3 {
				m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " ")
				m.suggIdx = 0
				return m, m.cmdMaybePrefetchPickers()
			}
			m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " " + parts[2] + " ")
			m.suggIdx = 0
			return m, m.cmdMaybePrefetchPickers()
		}
		if m.authPhase != AuthNone {
			return m, nil
		}
		if items := m.collectSuggestions(); len(items) > 0 {
			// Shift-Tab cycles backwards
			if len(items) > 0 {
				m.suggIdx = (m.suggIdx - 1 + len(items)) % len(items)
				m.applySuggestion(items)
				return m, m.cmdMaybePrefetchPickers()
			}
		}
		return m, nil

	case tea.KeyTab:
		if m.authPhase != AuthNone {
			return m, nil
		}
		if items := m.collectSuggestions(); len(items) > 0 {
			it := items[m.suggIdx%len(items)]
			if it.kind == "file" && strings.TrimSpace(it.value) != "" {
				m.applyFileMention(it.value)
				return m, nil
			}
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
		// Empty input + no picker/agents: Tab cycles the chat posture
		// (plan ↔ code). Scan is only reachable via explicit /mode scan.
		// Guard with sessionDefaultsLoaded so a stray Tab right after the chat
		// input appears cannot fire a GET /client/chat-posture before the
		// session is ready (the "treo" report — F2 lúc được lúc không).
		if m.mode == ModeChat && m.sessionDefaultsLoaded && m.inputValue == "" {
			next := m.activePosture()
			found := false
			for i, p := range tabPostureOrder {
				if p == next {
					next = tabPostureOrder[(i+1)%len(tabPostureOrder)]
					found = true
					break
				}
			}
			if !found {
				// Current posture (e.g. scan) is not in the Tab cycle;
				// default to the first entry (plan).
				next = tabPostureOrder[0]
			}
			m.chatPosturePending = "apply:" + next
			return m, m.cmdLoadChatPosture()
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
				if it.kind == "file" {
					if strings.TrimSpace(it.value) != "" {
						m.applyFileMention(it.value)
					}
					return m, nil
				}
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
				// Wizard save/cancel/back rows
				if it.kind == "mode-setup-save" {
					if m.modeSetupDraft == nil || !m.modeSetupDraftDirty {
						m.addMessage("system", "Nothing to save.", "")
						m.clearInputValue()
						return m, nil
					}
					// Save draft in one PUT (copy draft, clear wizard)
					draft := *m.modeSetupDraft
					m.modeSetupDraft = nil
					m.modeSetupDraftDirty = false
					m.chatPostureCfg = draft
					m.chatPostureSaving = true
					m.chatPostureSavingPosture = draft.Active
					m.addMessage("system", "Saving posture draft…", "")
					m.clearInputValue()
					return m, m.cmdSaveChatPosture(draft)
				}
				if it.kind == "mode-setup-cancel" {
					m.modeSetupDraft = nil
					m.modeSetupDraftDirty = false
					m.clearInputValue()
					m.addMessage("system", "Posture draft discarded.", "")
					return m, nil
				}
				if it.kind == "mode-setup-back" {
					// Go up one wizard level based on current input
					in := strings.TrimSpace(m.inputValue)
					parts := strings.Fields(in)
					// in is like "/mode-setup", "/mode-setup plan", "/mode-setup plan model", "/mode-setup plan model opus"
					if len(parts) <= 1 {
						m.clearInputValue()
						return m, nil
					}
					if len(parts) == 2 {
						// at posture level -> back to root
						m.setInputPreservingDraftPrefix("/mode-setup ")
						m.suggIdx = 0
						return m, m.cmdMaybePrefetchPickers()
					}
					if len(parts) == 3 {
						// at field level -> back to posture
						// keep posture, drop field
						m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " ")
						m.suggIdx = 0
						return m, m.cmdMaybePrefetchPickers()
					}
					// at value level -> back to field
					m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " " + parts[2] + " ")
					m.suggIdx = 0
					return m, m.cmdMaybePrefetchPickers()
				}
				if it.kind == "mode-setup-value" {
					// Wizard staging: parse "posture field value" and stage into draft, then return to field picker
					parts := strings.Fields(strings.TrimSpace(it.value))
					if len(parts) >= 3 {
						posture := strings.ToLower(parts[0])
						field := strings.ToLower(parts[1])
						val := strings.Join(parts[2:], " ")
						if validPosture(posture) {
							if m.modeSetupDraft == nil {
								// Snapshot current cfg (or empty) as draft base
								cfgCopy := m.chatPostureCfg
								if cfgCopy.Profiles == nil {
									cfgCopy.Profiles = map[string]client.ChatPostureProfile{}
								} else {
									// deep copy map
									newMap := make(map[string]client.ChatPostureProfile, len(cfgCopy.Profiles))
									for k, v := range cfgCopy.Profiles {
										newMap[k] = v
									}
									cfgCopy.Profiles = newMap
								}
								m.modeSetupDraft = &cfgCopy
							}
							prof := m.modeSetupDraft.Profiles[posture]
							switch field {
							case "provider":
								prof.Provider = strings.TrimSpace(val)
							case "model":
								prof.Model = strings.TrimSpace(val)
								if prov := providerForModel(m.providers, prof.Model); prov != "" {
									prof.Provider = prov
								}
							case "reasoning", "reason":
								prof.ReasoningEffort = strings.ToLower(strings.TrimSpace(val))
							case "yolo":
								switch strings.ToLower(val) {
								case "on", "true", "1":
									on := true
									prof.Yolo = &on
								case "off", "false", "0":
									off := false
									prof.Yolo = &off
								case "clear", "-":
									prof.Yolo = nil
								}
							case "clear":
								prof = client.ChatPostureProfile{}
							}
							if m.modeSetupDraft.Profiles == nil {
								m.modeSetupDraft.Profiles = map[string]client.ChatPostureProfile{}
							}
							m.modeSetupDraft.Profiles[posture] = prof
							m.modeSetupDraftDirty = true
							m.addMessage("system", fmt.Sprintf("Staged %s %s = %s (not yet saved — pick another field or ✓ save)", posture, field, orDash(val)), "")
							// Return to field picker for same posture
							m.setInputPreservingDraftPrefix("/mode-setup " + posture + " ")
							m.suggIdx = 0
							return m, m.cmdMaybePrefetchPickers()
						}
					}
					// Fallback to old immediate PUT if parsing failed
				}
				if cmd := suggestionAcceptValue(it); cmd != "" {
					// Action rows only expand the next picker (provider connect, /image open|rm, mode-setup steps).
					if it.kind == "provider-action" || it.kind == "image-sub-next" || it.kind == "mode-setup-posture" || (it.kind == "mode-setup-field" && !strings.HasSuffix(strings.ToLower(strings.TrimSpace(it.value)), " clear")) {
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
				if typed := strings.TrimSpace(m.expandPasteTokens(m.inputValue)); len(strings.Fields(typed)) >= 2 {
					if m.sendBlocked() && !strings.HasPrefix(typed, "/") {
						return m, nil
					}
					m.clearInputValue()
					return m.processInput(typed)
				}
				return m, nil
			}
		}
		input := strings.TrimSpace(m.expandPasteTokens(m.inputValue))
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
			// (KeyRunes+Paste). The bracketed-paste runes already carry the
			// pasted text — insert it directly. Reading the system clipboard on
			// the typing hot path is what froze the composer: a locked
			// clipboard (native read or PowerShell GetText) blocked forever, so
			// typed/pasted characters never appeared while F2/F4 still worked.
			// The clipboard is only consulted when the paste carries no text
			// (image-only clipboard) or looks like a copied image file path.
			if msg.Paste {
				pasted := sanitizePasteText(string(msg.Runes))
				if strings.TrimSpace(pasted) == "" {
					return m, m.cmdClipboardPasteWithFallback("")
				}
if path := imagePathFromClipboardText(pasted); path != "" {
				return m, m.cmdAttachImagePath(path)
			}
			// Long/multi-line pastes collapse to a "[Pasted N lines · C chars]"
			// token so the composer never shows a cut block and the pasted line
			// breaks can never auto-submit. The full text is expanded on send.
			if needsPasteSummary(pasted) {
				m.insertPasteSummary(pasted)
			} else {
				m.insertInputAtCursor(pasted)
			}
			m.suggIdx = 0
			return m, m.cmdMaybePrefetchPickers()
			}
		}
		s := " "
		if msg.Type == tea.KeyRunes {
			s = string(msg.Runes)
		}
		// Track raw (non-bracketed) paste floods so their Enters become newlines
		// instead of submits; bracketed pastes are handled above.
		if m.authPhase == AuthNone && !msg.Paste && !msg.Alt {
			m.noteBurstRune(s)
		}
		m.insertInputAtCursor(s)
		m.suggIdx = 0
		// Auto-collapse the burst ~burstSettle after it stops (no keystroke needed).
		if m.pasteBurst.active {
			return m, tea.Batch(m.cmdMaybePrefetchPickers(), cmdPasteBurstSettle())
		}
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
	if query, start, ok := activeAtFragment(m.inputValue, m.inputCaretIndex()); ok && !isAgentAtMention(start, query, m.agentRuns) {
		if m.workspaceFilesQuery != query {
			return filterFileMentionSuggestions(query, nil)
		}
		return filterFileMentionSuggestions(query, m.workspaceFiles)
	}
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
	if chats := filterHistorySuggestionsWithRemote(in, m.chatList, m.remoteChatList); len(chats) > 0 {
		return chats
	}
	if syncSugg := filterSyncSuggestionsWithRemote(in, m.syncableChats(), m.remoteChatList); len(syncSugg) > 0 {
		return syncSugg
	}
	if restoreSugg := filterRestoreSuggestions(in, m.remoteChatList); len(restoreSugg) > 0 {
		return restoreSugg
	}
	// CA-554: /restore Tab picker must always open — show a loading row while
	// the Drive index is in flight and an empty-state row once it is confirmed
	// empty (mirrors the /history loading row below).
	if ok, _ := parseSlashArgPrefix(in, "/restore"); ok {
		if m.remoteChatList == nil {
			return []suggestItem{{value: "", detail: "loading Drive chats…", kind: "restore", slash: "/restore"}}
		}
		return []suggestItem{{value: "", detail: "(no Drive-backed chats to restore)", kind: "restore", slash: "/restore"}}
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
	// /model now lists every supported model across all providers (catalog ∪
	// defaults) with provider as detail — picking a model auto-switches provider
	// per FlowPilot rule. Mirrors /mode-setup model behavior.
	if ok, q := parseSlashArgPrefix(in, "/model"); ok {
		all := allModelsAcrossProviders(m.providers, m.provider, m.model)
		qLower := strings.ToLower(strings.TrimSpace(q))
		out := make([]suggestItem, 0, len(all))
		for _, e := range all {
			if qLower != "" && !strings.Contains(strings.ToLower(e.id), qLower) {
				continue
			}
			detail := e.provider
			if detail == "" {
				detail = "model"
			}
			if strings.EqualFold(e.id, m.model) {
				if detail == "model" {
					detail = "current"
				} else {
					detail = detail + " · current"
				}
			}
			out = append(out, suggestItem{value: e.id, detail: detail, kind: "model"})
		}
		if len(out) > 0 {
			return out
		}
		if len(all) == 0 {
			if !m.sessionDefaultsLoaded && len(m.providers) == 0 {
				return []suggestItem{{value: "", detail: "loading models…", kind: "model"}}
			}
			return []suggestItem{{value: "", detail: "no models in catalog", kind: "model"}}
		}
		return []suggestItem{{value: "", detail: "(no matching models)", kind: "model"}}
	}
	if reasonSugg := filterReasoningSuggestions(in, m.reasoningEffort); len(reasonSugg) > 0 {
		return reasonSugg
	}
	if ok, _ := parseSlashArgPrefix(in, "/reasoning"); ok {
		return []suggestItem{{value: "", detail: "(no matching effort)", kind: "reasoning"}}
	}
	// /mode-setup now uses modal on Enter, TAB picker disabled per user request
	// (previously wizard posture→field→value). Keep typed 3-arg via handleSlashCommand.
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(in)), "/mode-setup") {
		// No TAB suggestions for this command; Enter opens modal
		// Fall through to let slash command handling decide (bare shows command, with args no picker)
	} else if modeSetupSugg := filterModeSetupSuggestions(in, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft); len(modeSetupSugg) > 0 {
		// Legacy wizard path kept for typed power-user but not exposed via TAB
		// (kept for backward compat if needed, but currently unreachable due to guard above)
		return modeSetupSugg
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
	// /mode <name> picker — scan/plan/code
	if ok, q := parseSlashArgPrefix(in, "/mode"); ok {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(in)), "/mode-setup") {
			qLower := strings.ToLower(strings.TrimSpace(q))
			out := make([]suggestItem, 0, 3)
			for _, p := range postureOrder {
				if qLower != "" && !strings.Contains(strings.ToLower(p), qLower) {
					continue
				}
				out = append(out, suggestItem{value: p, detail: postureLabel(p), kind: "mode"})
			}
			if len(out) > 0 {
				return out
			}
			return []suggestItem{{value: "", detail: "(no matching posture)", kind: "mode"}}
		}
	}
	if initSugg := filterInitSuggestions(in); len(initSugg) > 0 {
		return initSugg
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
	// Wizard save/cancel/back via Tab should behave like Enter
	if it.kind == "mode-setup-save" {
		if m.modeSetupDraft != nil && m.modeSetupDraftDirty {
			draft := *m.modeSetupDraft
			m.modeSetupDraft = nil
			m.modeSetupDraftDirty = false
			m.chatPostureCfg = draft
			m.chatPostureSaving = true
			m.chatPostureSavingPosture = draft.Active
			m.addMessage("system", "Saving posture draft…", "")
			m.clearInputValue()
			// Note: Tab caller will still do cmdMaybePrefetchPickers, but save cmd needs to be returned
			// For now, just set input; Enter is the primary save trigger. Tab on save is no-op.
		}
		return
	}
	if it.kind == "mode-setup-cancel" {
		m.modeSetupDraft = nil
		m.modeSetupDraftDirty = false
		m.clearInputValue()
		m.addMessage("system", "Posture draft discarded.", "")
		return
	}
	if it.kind == "mode-setup-back" {
		in := strings.TrimSpace(m.inputValue)
		parts := strings.Fields(in)
		if len(parts) <= 1 {
			m.clearInputValue()
		} else if len(parts) == 2 {
			m.setInputPreservingDraftPrefix("/mode-setup ")
		} else if len(parts) == 3 {
			m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " ")
		} else {
			m.setInputPreservingDraftPrefix("/mode-setup " + parts[1] + " " + parts[2] + " ")
		}
		m.suggIdx = 0
		return
	}
	if cmd := suggestionAcceptValue(it); cmd == "" {
		return
	} else if it.kind == "file" {
		m.applyFileMention(it.value)
	} else if it.kind == "flow" || it.kind == "history" || it.kind == "model" || it.kind == "reasoning" || it.kind == "provider" || it.kind == "provider-connect" || it.kind == "provider-action" || it.kind == "provider-install" || it.kind == "provider-account" || it.kind == "skill" || it.kind == "agent" || it.kind == "image-sub" || it.kind == "image-sub-next" || it.kind == "image-open" || it.kind == "image-rm" || it.kind == "mode-setup-posture" || it.kind == "mode-setup-field" || it.kind == "mode-setup-value" || it.kind == "mode" || it.kind == "init" {
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
	case "sync":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		slash := it.slash
		if slash == "" {
			slash = "/sync"
		}
		return slash + " " + it.value
	case "restore":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		slash := it.slash
		if slash == "" {
			slash = "/restore"
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
	case "file":
		return strings.TrimSpace(it.value)
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
	case "mode-setup-posture":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/mode-setup " + strings.TrimSpace(it.value) + " "
	case "mode-setup-field":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		v := strings.TrimSpace(it.value)
		if strings.HasSuffix(strings.ToLower(v), " clear") {
			return "/mode-setup " + v
		}
		return "/mode-setup " + v + " "
	case "mode-setup-value":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/mode-setup " + strings.TrimSpace(it.value)
	case "mode":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/mode " + strings.TrimSpace(it.value)
	case "init":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		slash := it.slash
		if slash == "" {
			slash = "/init"
		}
		return slash + " " + strings.TrimSpace(it.value)
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
		msg := "Still loading session — chat is disabled until ready. (F2/F4 still work)"
		if len(m.providers) == 0 {
			msg = "Still loading session — providers not ready yet, please wait 2-3s then retry."
		}
		m.addMessage("system", msg, "error")
		tuiLog("processInput blocked: sessionLoading providers=%d", len(m.providers))
		return m, m.showFlashToast(msg)
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
		msg := "A turn is already in progress — wait for it to finish (draft kept in the input)."
		if m.pendingPrompt != "" {
			msg = "A turn is already in progress (pendingPrompt) — wait for it to finish."
		} else if m.approval != nil {
			msg = "Pending approval — click Approve/Deny first."
		} else if m.hasUnresolvedAttention() {
			msg = "Flow needs your decision — check dispatch attention above."
		}
		m.addMessage("system", msg, "error")
		tuiLog("send blocked: %s", msg)
		// Also flash to status bar so it's visible even if viewport is scrolled.
		return m, m.showFlashToast(msg)
	}

	// After session defaults load, refuse empty provider (avoids silent fake Codex).
	if m.sessionDefaultsLoaded && strings.TrimSpace(m.provider) == "" {
		m.addMessage("system", "No provider selected — use /provider <key> (then /model).", "error")
		return m, nil
	}
	if m.sessionDefaultsLoaded && len(m.providers) == 0 {
		// Cold start: providers not yet loaded (log showed 2s timeout). Don't
		// silently block — log and let the run attempt proceed; the runner
		// will return a proper error if the provider is truly unavailable.
		tuiLog("send with providers empty (cold start) provider=%q", m.provider)
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
		m.turnSendPending = true
		startMsg := fmt.Sprintf("Starting chat run (%s · %s)…", m.provider, orDash(m.model))
		if m.launch.IsCatalogWorkflow() {
			startMsg = fmt.Sprintf("Starting workflow run (%s · %s · %s)…", m.launch.StatusLabel(), m.provider, orDash(m.model))
		} else if m.launch.IsBuiltin() {
			startMsg = fmt.Sprintf("Starting chat run with %s (%s · %s)…", m.launch.StatusLabel(), m.provider, orDash(m.model))
		}
		m.addMessage("system", startMsg, "")
		return m, m.cmdStartRun()
	}
	// run-107774: mark the turn send in flight so a steps poll landing before
	// turnStreamOpenedMsg cannot settle the finished loop (see settleFlowIfDone).
	m.turnSendPending = true
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

	case "/scan":
		if m.mode != ModeChat {
			m.addMessage("system", "Scan/Plan/Code postures apply to chat mode only.", "")
			break
		}
		m.chatPosturePending = "apply:scan"
		return m, m.cmdLoadChatPosture()

	case "/mode":
		if m.mode != ModeChat {
			m.addMessage("system", "Scan/Plan/Code postures apply to chat mode only.", "")
			break
		}
		// /mode <name> applies a posture; /mode cycles scan→plan→code.
		next := m.activePosture()
		if len(args) > 0 {
			want := strings.ToLower(args[0])
			if !validPosture(want) {
				m.addMessage("system", "Mode must be scan, plan, or code.", "error")
				break
			}
			next = want
		} else {
			// /mode without args cycles plan ↔ code. Scan requires
			// explicit /mode scan so the user opts in intentionally.
			found := false
			for i, p := range tabPostureOrder {
				if p == next {
					next = tabPostureOrder[(i+1)%len(tabPostureOrder)]
					found = true
					break
				}
			}
			if !found {
				next = tabPostureOrder[0]
			}
		}
		m.chatPosturePending = "apply:" + next
		return m, m.cmdLoadChatPosture()

	case "/mode-setup":
		if m.mode != ModeChat {
			m.addMessage("system", "Scan/Plan/Code postures apply to chat mode only.", "")
			break
		}
		if len(args) == 0 {
			m.chatPosturePending = "modal:"
			return m, m.cmdLoadChatPosture()
		}
		if len(args) == 1 && validPosture(strings.ToLower(args[0])) {
			m.chatPosturePending = "modal:" + strings.ToLower(args[0])
			return m, m.cmdLoadChatPosture()
		}
		// /mode-setup <posture> <field> <value>  (clear needs no value)
		if len(args) < 2 {
			m.addMessage("system", "Usage: /mode-setup <scan|plan|code> <provider|model|reasoning|yolo|clear> <value>", "error")
			break
		}
		posture := strings.ToLower(args[0])
		field := strings.ToLower(args[1])
		if field == "clear" {
			if len(args) != 2 {
				m.addMessage("system", "Usage: /mode-setup <scan|plan|code> clear (no value)", "error")
				break
			}
			m.chatPosturePending = "setup:" + posture + ":" + field + ":"
			return m, m.cmdLoadChatPosture()
		}
		if len(args) < 3 {
			m.addMessage("system", "Usage: /mode-setup <scan|plan|code> <provider|model|reasoning|yolo|clear> <value>", "error")
			break
		}
		value := strings.Join(args[2:], " ")
		m.chatPosturePending = "setup:" + posture + ":" + field + ":" + value
		return m, m.cmdLoadChatPosture()

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
			all := allModelsAcrossProviders(m.providers, m.provider, m.model)
			if len(all) == 0 {
				sb.WriteString("No models listed. Check catalog.")
			} else {
				sb.WriteString("Available models (provider → model):\n")
				for _, e := range all {
					mark := " "
					if strings.EqualFold(e.id, m.model) {
						mark = "*"
					}
					prov := e.provider
					if prov == "" {
						prov = orDash(m.provider)
					}
					sb.WriteString(fmt.Sprintf("  %s %s · %s\n", mark, prov, e.id))
				}
				sb.WriteString("Pick: type /model  then ↑↓ · Tab · Enter (provider auto-switches)")
			}
			m.addMessage("system", sb.String(), "")
		} else {
			want := strings.TrimSpace(strings.Join(args, " "))
			all := allModelsAcrossProviders(m.providers, m.provider, m.model)
			var matched *modelEntry
			for _, e := range all {
				if strings.EqualFold(e.id, want) {
					tmp := e
					matched = &tmp
					break
				}
			}
			providerSwitched := false
			if matched != nil {
				if matched.provider != "" && !strings.EqualFold(matched.provider, m.provider) {
					m.provider = matched.provider
					m.bindActiveAccountForProvider()
					m.skillsCatalog = nil
					providerSwitched = true
				}
				m.model = matched.id
			} else {
				if prov := providerForModel(m.providers, want); prov != "" && !strings.EqualFold(prov, m.provider) {
					m.provider = prov
					m.bindActiveAccountForProvider()
					m.skillsCatalog = nil
					providerSwitched = true
				}
				m.model = want
			}
			m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
			m.persistSessionPrefs()
			if matched != nil && matched.provider != "" {
				m.addMessage("system", fmt.Sprintf("Model set to: %s · provider: %s (next prompt uses this model)", m.model, m.provider), "")
			} else {
				m.addMessage("system", fmt.Sprintf("Model set to: %s (next prompt uses this model)", m.model), "")
			}
			m.refreshSessionPanel()
			if providerSwitched {
				return m, m.cmdLoadSkills(false)
			}
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
		m.clearPendingDecisions()
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
		if !m.sessionDefaultsLoaded {
			break
		}
		// Reload the posture profile from the runner (SSOT) so /new applies
		// the active posture's pinned provider/model/reasoning/yolo — the
		// same pins that a fresh Desktop session reads on mount.
		m.chatPosturePending = "apply:" + m.activePosture()
		return m, m.cmdLoadChatPosture()

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
		m.sessionPanel.DriveStatus = m.driveIndicatorLine()
		m.sessionPanel.DriveBadge = m.openChatDriveBadge()
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

	case "/approve forever", "/deny forever":
		// BUG-246: "don't ask again" for the exec approval. A deny is never
		// persisted (desktop parity); the remember flag rides only on approve.
		decision := "approve"
		if strings.HasPrefix(input, "/deny") {
			decision = "deny"
		}
		if m.approval != nil {
			remember := decision == "approve" && approvalRememberable(m.approval)
			return m.submitPendingApprovalRemember(decision, remember)
		}
		m.addMessage("system", "No pending approval.", "")

	case "/approve-all", "/deny-all":
		// BUG-157/158: a turn can fan out several parallel approvals; resolve
		// every queued card at once (Desktop bulk-approve parity).
		decision := "approve"
		if strings.HasPrefix(input, "/deny") {
			decision = "deny"
		}
		return m.resolveAllApprovals(decision)

	case "/submit":
		// G3 multiSelect question: send the toggled selection set as an array.
		return m.submitQuestionSubmit()

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

	case "/init":
		if len(args) == 0 {
			m.addMessage("system", "Usage: /init <skill|all>  — Tab shows skill (flow-pack) or all (full engine).", "")
			break
		}
		kind := strings.ToLower(strings.TrimSpace(args[0]))
		if kind != "skill" && kind != "all" {
			m.addMessage("system", fmt.Sprintf("Unknown /init kind %q — use /init skill or /init all (Tab).", args[0]), "error")
			break
		}
		return m, m.cmdInitEngine(kind)

	case "/sync":
		return m.runSyncDispatch(args)

	case "/restore":
		return m.runRestoreDispatch(args)

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
	viewStart := time.Now()

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
	if len(c.sugg) > 0 && !m.modeSetupModalOpen {
		rows = append(rows, strings.Split(m.renderSuggestions(c.sugg), "\n")...)
	}
	if m.modeSetupModalOpen {
		rows = append(rows, strings.Split(m.renderModeSetupModal(w), "\n")...)
	}
	if c.attachPanelBlock != "" {
		rows = append(rows, strings.Split(c.attachPanelBlock, "\n")...)
	}
	inputStart := len(rows)
	if m.sessionLoading {
		// Hide normal chat input until loading is done — show a clear loading
		// placeholder instead so the user doesn't feel the UI is hung.
		// F2/F4 still work via allowsKeyWhileLoading.
		frames := []string{"|", "/", "-", "\\"}
		spin := frames[m.loadingFrame%len(frames)]
		msg := fmt.Sprintf(" %s Loading session · project · providers — chat locked (F2/F4 still work) ", spin)
		rows = append(rows, styleLoading.Render(truncateVisual(msg, w)))
	} else {
		rows = append(rows, strings.Split(m.renderInputLine(), "\n")...)
	}

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
		out := joinRightSidebar(rows, sideLines, sideW, sideX, m.asciiMode)
		if d := time.Since(viewStart); d > 100*time.Millisecond {
			tuiLog("View slow dur=%v width=%d height=%d side=%v", d, m.width, m.height, useSide)
		}
		return out
	}

	out := strings.Join(rows, "\n")
	if d := time.Since(viewStart); d > 100*time.Millisecond {
		tuiLog("View slow dur=%v width=%d height=%d side=%v", d, m.width, m.height, useSide)
	}
	return out
}

func (m *AppModel) loadingBannerText() string {
	return renderFlowpilotLoader(m.loadingFrame, m.asciiMode)
}

func suggestionVisibleLimit(sugg []suggestItem) int {
	if len(sugg) > 0 && (sugg[0].kind == "history" || sugg[0].kind == "skill" || sugg[0].kind == "file") {
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
		case "file":
			kind = "files"
		case "sync":
			kind = "sync"
		case "restore":
			kind = "restore"
		case "mode":
			kind = "postures"
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
	if kind == "files" {
		sb.WriteString(styleSuggest.Render("  Tab/Enter insert path"))
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
	// PromptExpandKey is non-empty for rows of a user prompt bubble that exceeds
	// the 4-line clamp. Clicking any such row toggles the bubble expansion.
	PromptExpandKey string
}

// maxUserPromptLines caps boxed user prompt bubbles; longer prompts collapse to
// this many wrapped lines with a "...." tail until the bubble is expanded.
const maxUserPromptLines = 4

// userPromptEllipsis marks a truncated user prompt bubble.
const userPromptEllipsis = "...."

func (m *AppModel) userPromptExpanded(content string) bool {
	return m.expandedUserPrompts[content]
}

func (m *AppModel) toggleUserPrompt(content string) {
	if m.expandedUserPrompts == nil {
		m.expandedUserPrompts = map[string]bool{}
	}
	m.expandedUserPrompts[content] = !m.expandedUserPrompts[content]
	m.rowCache = nil
	m.rowCacheSig = 0
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
	upKeys := make([]string, 0, len(m.expandedUserPrompts))
	for k, v := range m.expandedUserPrompts {
		if v {
			upKeys = append(upKeys, k)
		}
	}
	sort.Strings(upKeys)
	for _, k := range upKeys {
		_, _ = h.Write([]byte{3})
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
		wrapped := wrapText(msg.Content, contentWidth)
		mdLines := textsToMD(trimEmptyEdges(wrapped))
		if msg.Role == "assistant" && msg.FormatHint == "" {
			mdLines = renderMarkdownRows(msg.Content, contentWidth, m.asciiMode)
		}
		var userPainted []string
		userTruncated := false
		userTruncatable := false
		if msg.Role == "user" {
			userTruncatable = len(mdLines) > maxUserPromptLines
			if !m.userPromptExpanded(msg.Content) && userTruncatable {
				mdLines = mdLines[:maxUserPromptLines]
				userTruncated = true
			}
			userPainted = paintWrappedMentions(msg.Content, attachedSkillNames(m.selectedSkills), mdTexts(mdLines), styleUser)
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
			} else if msg.Role == "user" && i < len(userPainted) {
				rendered = userPainted[i]
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
			if userTruncated && i == len(mdLines)-1 {
				ellipsisW := lipgloss.Width(userPromptEllipsis)
				if lipgloss.Width(stripANSI(rendered))+ellipsisW > contentWidth {
					target := max(0, contentWidth-ellipsisW)
					// Cut visual width without adding "…" (truncateVisual adds one).
					plain := stripANSI(rendered)
					w := 0
					cut := 0
					for idx, r := range []rune(plain) {
						rw := lipgloss.Width(string(r))
						if w+rw > target {
							break
						}
						w += rw
						cut = idx + 1
					}
					plainCut := string([]rune(plain)[:cut])
					// Preserve user style for the truncated tail.
					rendered = styleUser.Render(plainCut)
				}
				rendered += styleStatus.Render(userPromptEllipsis)
			}
			row := chatRow{Text: rendered, MsgIdx: mi, Copy: copyOn || copyFence}
			if msg.Role == "user" && userTruncatable {
				row.PromptExpandKey = msg.Content
			}
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

// formatApprovalWaitingLineDetailed extends the live-gate transcript line with
// the approval kind + command so the operator sees exactly what they are being
// asked to approve (BUG-246). Cards without details keep the legacy copy.
func formatApprovalWaitingLineDetailed(approvalID string, ascii bool, a ApprovalState) string {
	base := formatApprovalWaitingLine(approvalID, ascii)
	if strings.TrimSpace(a.Command) == "" && strings.TrimSpace(a.Reason) == "" {
		return base
	}
	var parts []string
	if k := strings.TrimSpace(a.Kind); k != "" {
		parts = append(parts, k)
	}
	if c := strings.TrimSpace(a.Command); c != "" {
		parts = append(parts, c)
	}
	line := base + " · " + strings.Join(parts, ": ")
	if r := strings.TrimSpace(a.Reason); r != "" {
		line += " — " + r
	}
	return line
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
		if q.MultiSelect {
			mark := "[ ]"
			sel := ""
			for _, v := range q.Selected {
				if v == questionOptionToken(o) {
					mark = "[x]"
					break
				}
			}
			sel = mark + " "
			b.WriteString(styleLink.Render(sel + questionOptionLabel(o)))
		} else {
			b.WriteString(styleLink.Render(strconv.Itoa(i+1) + ")"))
			b.WriteString(" ")
			b.WriteString(styleLink.Render(questionOptionLabel(o)))
		}
	}
	if len(q.Options) == 0 {
		b.WriteString(styleSystem.Render("type an answer"))
	} else if q.MultiSelect {
		b.WriteString("  ")
		b.WriteString(styleSystem.Render("toggle, then /submit or click Submit"))
		b.WriteString("  ")
		b.WriteString(styleLink.Render("[Submit]"))
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
	// No composer clamp: pastes collapse to one "[Pasted N chars]" token and
	// manually typed multi-line prompts render fully (CA-560). Clamping also
	// misaligned caret offsets once the input grew past the window, losing the
	// cursor entirely.

	var inner []string
	if m.approval != nil {
		head := "approval"
		if n := len(m.approvals); n > 1 {
			head = fmt.Sprintf("approval %d/%d", 1, n)
		}
		chips := approvalDecisionChips(m.approval)
		if len(m.approvals) > 1 {
			chips += "  " + styleLink.Render("Approve all") + "  " + styleLink.Render("Deny all")
		}
		inner = append(inner, styleGate.Render(head)+"  "+chips+"  "+styleSystem.Render("click or type"))
	}
	if m.question != nil {
		qbar := renderQuestionBar(left, mid, m.question, innerW)
		if n := len(m.questions); n > 1 {
			qbar = styleGate.Render(fmt.Sprintf("(%d/%d)", 1, n)) + " " + qbar
		}
		inner = append(inner, qbar)
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

func buildGateMessage(status, errMsg string, opts []string, regressed []string) string {
	var sb strings.Builder
	lowStatus := strings.ToLower(strings.TrimSpace(status))
	hasOpts := len(opts) > 0
	hasRegressed := len(regressed) > 0
	// Only a real block with a decision card keeps the legacy detailed card.
	// Legacy tests emit GateOptions without Status — treat empty status + opts as block for view compat.
	if hasOpts && (lowStatus == "block" || lowStatus == "") {
		sb.WriteString("[GATE] Flow gate blocked.\n")
		if hasRegressed {
			sb.WriteString(fmt.Sprintf("  Regressed tests: %s\n", strings.Join(regressed, ", ")))
		}
		sb.WriteString("  Options: ")
		sb.WriteString(strings.Join(opts, ", "))
		return sb.String()
	}
	switch lowStatus {
	case "reprompt":
		sb.WriteString("[GATE] auto-reprompt")
	case "warn":
		sb.WriteString("[GATE] warn")
	case "block":
		sb.WriteString("[GATE] blocked")
	default:
		if lowStatus == "" {
			sb.WriteString("[GATE] Flow gate triggered.")
		} else {
			sb.WriteString("[GATE] " + lowStatus)
		}
	}
	if msg := strings.TrimSpace(errMsg); msg != "" {
		sb.WriteString(" — " + msg)
	} else if lowStatus == "reprompt" {
		sb.WriteString(" — gate is re-applying fixes (no action needed)")
	}
	if hasRegressed {
		sb.WriteString(fmt.Sprintf("\n  Regressed tests: %s", strings.Join(regressed, ", ")))
	}
	// Intentionally no "Options:" line when hasOpts == false.
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
	tuiLog("cmdConnect() start runner=%s", m.runnerURL)
	return func() tea.Msg {
		tuiLog("cmdConnect() -> ConnectedMsg runner=%s", m.runnerURL)
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
		tuiLog("cmdLoadSessionDefaults() start flagProvider=%q flagModel=%q project=%q", flagProvider, flagModel, projectPath)
		start := time.Now()
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
			t0 := time.Now()
			tuiLog("cmdLoadSessionDefaults catalog fetch start")
			cl := client.New(runnerURL)
			ctxProjects, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ps, err := cl.ListProjects(ctxProjects)
			tuiLog("cmdLoadSessionDefaults catalog fetch done dur=%v err=%v n=%d", time.Since(t0), err, len(ps))
			catalogCh <- catalogResult{projects: ps, err: err}
		}()

		// Provider/account path. Accounts are a local config read (fast); the
		// providers scan can spawn CLI probes on the runner, so it gets a short
		// independent budget instead of the shared fast-path window — a cold
		// probe scan must never hold the session unlock for the full window
		// (CA-535). The runner's /providers handler also honors r.Context()
		// cancel and caches results, so in practice this resolves in ms.
		cl := client.New(runnerURL)
		// Grok quota needs 2× billing HTTP (credits + fallback) per home; 4 homes
		// serial is up to ~20s cold. Desktop shows 7d fine; TUI's 8s budget
		// truncated it to Team UUID fallback. Give it a dedicated 20s.
		ctxFast, cancelFast := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancelFast()
		t0Provs := time.Now()
		accounts, accErr := cl.ListProviderAccounts(ctxFast)
		tuiLog("cmdLoadSessionDefaults accounts done dur=%v err=%v n=%d", time.Since(t0Provs), accErr, len(accounts))
		t0Provs2 := time.Now()
		ctxProvs, cancelProvs := context.WithTimeout(context.Background(), 8*time.Second)
		providers, provErr := cl.ListProviders(ctxProvs)
		cancelProvs()
		tuiLog("cmdLoadSessionDefaults providers done dur=%v err=%v n=%d", time.Since(t0Provs2), provErr, len(providers))
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
		tuiLog("cmdLoadSessionDefaults() done dur=%v accounts=%d providers=%d projects=%d catalogErr=%q accErr=%v provErr=%v", time.Since(start), len(accounts), len(providers), len(projects), catalogErr, accErr, provErr)
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
	return tea.Batch(m.cmdMaybePrefetchFlows(), m.cmdMaybePrefetchHistory(), m.cmdMaybePrefetchSkills(), m.cmdMaybePrefetchWorkspaceFiles())
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

// submitPendingApprovalDecision resolves the head approval with a specific
// decision value offered by the runner (BUG-246: decisions other than the
// default approve/deny, e.g. approve_for_session).
func (m *AppModel) submitPendingApprovalDecision(decision string) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		m.addMessage("system", "No pending approval.", "")
		return m, nil
	}
	return m, m.cmdApprove(m.approval.ID, decision)
}

// submitPendingApprovalRemember resolves the head approval carrying the
// "don't ask again" remember flag (BUG-246 desktop parity — only an approve
// decision is ever persisted; deny always passes remember=false).
func (m *AppModel) submitPendingApprovalRemember(decision string, remember bool) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		m.addMessage("system", "No pending approval.", "")
		return m, nil
	}
	return m, m.cmdApproveWithRemember(m.approval.ID, decision, remember)
}

func (m *AppModel) cmdApprove(approvalID, decision string) tea.Cmd {
	return m.cmdApproveWithRemember(approvalID, decision, false)
}

func (m *AppModel) cmdApproveWithRemember(approvalID, decision string, remember bool) tea.Cmd {
	runnerURL := m.runnerURL
	id := approvalID
	dec := decision
	rem := remember
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitApproval(context.Background(), id, dec, rem); err != nil {
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

// tuiProgramOpts returns Bubble Tea program options. On Windows the conhost
// ReadConsoleInput path with ENABLE_MOUSE_INPUT (WithMouseCellMotion) shares
// a 64-event queue with keys; a WT paste flood fills it with coninput mouse
// + key records so keys stick until a click (log 18936 10:13:31 Enable-only
// did not unstick, 22964 needed a real click). Disable mouse on Windows so
// Enter/F2/F4/Alt+V/Ctrl+V stay live — click affordances fall back to keys.
// The helper is exported for tests to assert the platform split.
func tuiProgramOpts() []tea.ProgramOption {
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if runtime.GOOS != "windows" {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	return opts
}

// ---- Run (entrypoint) -------------------------------------------------------

 // Run starts the Bubble Tea program. In headless/print mode it runs
// the model loop and prints the final response to stdout, then exits.
func Run(cfg config.ChatConfig, runnerURL string) error {
	initTUILog()
	tuiLog("Run() start print=%v runner=%s", cfg.Print, runnerURL)
	m := New(cfg, runnerURL)

	if cfg.Print {
		err := runHeadless(m, cfg.Prompt)
		tuiLog("Run() headless done err=%v", err)
		tuiLogClose()
		return err
	}

	p := tea.NewProgram(m, tuiProgramOpts()...)
	_, err := p.Run()
	tuiLog("Run() exit err=%v", err)
	tuiLogClose()
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
		ChatPosture:     m.activePosture(),
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
