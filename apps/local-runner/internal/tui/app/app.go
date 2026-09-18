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
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
	"flowpilot-runner/internal/workingmode"
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
	styleUserLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleUser      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPromptText))
	styleAssistant = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleSystem    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleTool      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn))
	styleError     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
	styleGate      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAsk))
	// Question-card hierarchy (CA-643): the [QUESTION] head, the prompt body,
	// the option rows and the "Answered:" confirmation used to be one flat
	// --ask purple block. Each part now has its own hue so the card is scannable:
	// amber head + option indexes, light body, accent option labels, green answer.
	styleQuestionHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWarn))
	styleQuestionBody = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleQuestionOpt  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	styleQuestionDesc = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleAnswer       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))
	// Question and Answer card box styles with solid grey background (colorBg3).
	styleQuestionBoxBg   = lipgloss.NewStyle().Background(lipgloss.Color(colorBg3))
	styleQuestionBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn)).Background(lipgloss.Color(colorBg3))
	styleQuestionHeadBox = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWarn)).Background(lipgloss.Color(colorBg3))
	styleQuestionBodyBox = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorBg3))
	styleQuestionOptBox  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Background(lipgloss.Color(colorBg3))
	styleQuestionDescBox = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim)).Background(lipgloss.Color(colorBg3))
	styleAnswerBorder    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOK)).Background(lipgloss.Color(colorBg3))
	styleAnswerHeadBox   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK)).Background(lipgloss.Color(colorBg3))
	styleStatus          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleStatusHi        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)) // model, reason value, YOLO value, 7d, skills
	styleMention         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))     // skill tokens in prompt
	styleMentionFile     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)) // @file paths in prompt
	styleStatusOK        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorOK))
	styleStatusErr       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErr))
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
	// BUG-333 UX: the selected action in an action ring (Approve/Deny, gate
	// options, question options, attention/blocked actions) renders as a
	// FILLED chip — same selection language as the /mode-setup tab row — so
	// Tab/arrows moves are impossible to miss (accent-colored text on both
	// states was nearly invisible).
	styleRingSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62"))
	styleSelect       = lipgloss.NewStyle().Reverse(true)
	styleThinking     = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(colorTextDim))
	// Active workflow step (Desktop timeline “current” accent).
	styleStepRunning = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWarn)).Background(lipgloss.Color(colorBg3))
	styleStepDone    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOK))
	styleStepFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorErr))
	// F2 step [open]/[back] — distinct from step highlight (accent) and running (warn).
	styleStepAgentAction = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color(colorAsk))
	// Selected step row (sidebar steps view) — same filled-chip selection
	// language as the action ring / mode-setup tab row (BUG-333): bold white on
	// the 62 blue background. The old teal text (styleStatusAgent) did not
	// stand out against the dim unselected rows.
	styleStepSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62"))
	// Canvas + elevated-panel backgrounds (CA-532): whole window canvas is darkest,
	// the right sidebar and chat bar are lighter grays like opencode.
	styleCanvas  = lipgloss.NewStyle().Background(lipgloss.Color(colorCanvas))
	styleSidebar = lipgloss.NewStyle().Background(lipgloss.Color(colorBg2))
	styleChatBar = lipgloss.NewStyle().Background(lipgloss.Color(colorBg3))
)

// isStalledStatus reports the watchdog banner that must not pollute the input
// frame chrome (user request: that long warning goes to the bottom line outside
// the composer instead of top-left "Chat: ... | stalled ...").
func isStalledStatus(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(low, "input stalled") || strings.Contains(low, "close this window to exit")
}

// chatBarBg returns s with the composer #1e1e1e background. Composer inner
// segments (title, body, attach) must carry the bar bg themselves so a lipgloss
// reset inside the segment does not punch a black hole in the solid gray frame
// (see renderChatPane / frameInput).
func chatBarBg(s lipgloss.Style) lipgloss.Style {
	return s.Background(lipgloss.Color(colorBg3))
}

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
		textarea:        newChatTextArea(80),
		textareaReady:   true,
		yolo:            yolo,
		workingMode:     savedWorkingMode(haveSaved, skipSessionUX, savedPrefs),
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
const (
	decawmOff = "\x1b[?7l" // disable autowrap — macOS Terminal.app/Ghostty/iTerm wrap the last column and desync bubbletea diff (CA-585)
	decawmOn  = "\x1b[?7h" // restore
)

func cmdSetAutoWrap(on bool) tea.Cmd {
	return func() tea.Msg {
		seq := decawmOff
		if on {
			seq = decawmOn
		}
		_, _ = os.Stdout.WriteString(seq)
		return nil
	}
}

func (m *AppModel) Init() tea.Cmd {
	tuiLog("Init() -> disable autowrap + wheel/drag mouse (1000h wheel + 1002h drag motion, no 1003 hover) + cmdConnect + tickCursor")
	return tea.Sequence(
		cmdSetAutoWrap(false),
		cmdEnableWheelMouse(),
		tea.Batch(m.cmdConnect(), tickCursor(), cmdInputWatchdog()),
	)
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
	m.syncActionRingCard()
	// Log startup-relevant messages (skip high-frequency ticks to keep log readable).
	switch v := msg.(type) {
	case cursorTickMsg, thinkingTickMsg, tea.WindowSizeMsg, inputWatchdogMsg:
	default:
		if _, ok := msg.(tea.MouseMsg); ok {
			// Mouse motion is filtered by tuiMsgFilter; logging every motion here
			// would still open/write/close the log file for each of the 60+ motion
			// events per second and reintroduce the stall the filter fixed.
		} else if km, ok := msg.(tea.KeyMsg); ok {
			tuiLog("Update KeyMsg Type=%v String=%q Paste=%v Runes=%q", km.Type, km.String(), km.Paste, string(km.Runes))
		} else {
			// Large payloads (chat history, skills, catalog) must not dump entire
			// bodies into the log — that I/O stalls the event loop and drops keys
			// (CA-621, View slow 100-800ms on open with sidebar).
			switch x := v.(type) {
			case ChatListMsg:
				tuiLog("Update ChatListMsg n=%d err=%q silent=%v", len(x.Items), x.Err, x.Silent)
			case FlowListMsg:
				tuiLog("Update FlowListMsg builtins=%d workflows=%d err=%q silent=%v", len(x.Builtins), len(x.Workflows), x.CatalogErr, x.Silent)
			case SessionDefaultsMsg:
				tuiLog("Update SessionDefaultsMsg providers=%d accounts=%d projects=%d err=%q", len(x.Providers), len(x.ProviderAccounts), len(x.Projects), x.CatalogErr)
			case ProvidersCatalogMsg:
				tuiLog("Update ProvidersCatalogMsg providers=%d err=%q", len(x.Providers), x.Err)
			case SkillsListMsg:
				tuiLog("Update SkillsListMsg n=%d err=%q show=%v", len(x.Skills), x.Err, x.Show)
			case ChatOpenedMsg:
				tuiLog("Update ChatOpenedMsg run=%s msgs=%d err=%q", x.Handle.RunID, len(x.Messages), x.Err)
			case ChatDeletedMsg:
				tuiLog("Update ChatDeletedMsg run=%s err=%q", x.RunID, x.Err)
			case ProjectContextMsg:
				tuiLog("Update ProjectContextMsg path=%q branch=%q", x.Path, x.Branch)
			case ClipboardPasteMsg:
				tuiLog("Update ClipboardPasteMsg textLen=%d hasAtt=%v err=%q noImage=%v", len([]rune(x.Text)), x.Attachment != nil, x.Err, x.NoImage)
			default:
				tuiLog("Update %T %v", msg, v)
			}
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w := msg.Width - 2
		if w < 40 {
			w = msg.Width
		}
		m.width = w
		m.height = msg.Height
		m.fullWidth = w
		if m.mirrorReady() {
			m.textarea.SetWidth(w - 2)
		}
		return m, tea.Batch(tea.ClearScreen, cmdSetAutoWrap(false))

	case cursorTickMsg:
		if m.sessionLoading {
			m.loadingFrame = (m.loadingFrame + 1) % 64
		}
		// CA-633: blink only while the caret is meaningful (draft, live turn,
		// blocked bar, loading, selection). Truly idle frames pin a steady
		// caret so the rendered chat pane stays byte-identical across ticks —
		// the composeCellBuf merge below then hits its pure-function cache
		// instead of rebuilding the 126×50 cellbuf (log 24144: 0 KeyMsg after
		// ready while every tick rebuilt with the sidebar open).
		if m.cursorBlinkRelevant() {
			m.cursorOn = !m.cursorOn
		} else if !m.cursorOn {
			m.cursorOn = true
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
		errText := msg.Err.Error()
		if m.projectWizardOpen {
			m.projectWizardBusy = false
			m.projectWizardErr = errText
			return m, nil
		}
		if m.loginModalOpen {
			m.loginModalBusy = false
			m.loginModalErr = errText
			return m, nil
		}
		if m.supabaseSetupModalOpen {
			m.supabaseSetupModalBusy = false
			m.supabaseSetupModalErr = errText
			return m, nil
		}
		m.err = msg.Err
		m.sessionLoading = false
		m.pendingPrompt = ""
		m.turnSendPending = false
		m.connStatus = ConnError
		m.statusMsg = "error"
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
		// Task-311: the sidebar is width-reactive (>= tuiSidebarMinWidth) and
		// starts visible on wide terminals; no Collapsed state exists anymore.
		// BUG-351: mark the session-start prefetch in-flight so a later Tab
		// refresh does not double-fire while it is still running.
		m.flowListInflight = true
		m.flowListFetchedAt = time.Now()
		cmds := []tea.Cmd{
			m.cmdLoadSessionDefaults(),
			m.cmdPrefetchFlows(), // The FlowPilot banner stays up until SessionDefaultsMsg decides the
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
			if m.chatWaitPending {
				// Defaults arrived but startup chats never settled (slow
				// endpoint, missed budget): same degraded pass as the chat timer.
				m.passChatGateDegraded("Chat list is taking too long — continuing without it. /open retries on each keypress.")
				return m, nil
			}
			tuiLog("sessionLoadTimeoutMsg ignored (defaults already loaded)")
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

	case chatLoadTimeoutMsg:
		// Startup chat fetch never settled within budget: pass degraded so
		// a slow chat endpoint cannot hold init-loading hostage; the picker
		// retries on each keypress.
		if !m.chatWaitPending {
			return m, nil
		}
		m.passChatGateDegraded("Chat list is taking too long — continuing without it. /open retries on each keypress.")
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
				m.chatListInflight = true
				cmds = append(cmds, m.cmdPrefetchChats())
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case ProvidersCatalogMsg:
		if msg.Err != "" {
			tuiLog("ProvidersCatalogMsg retry err=%q", msg.Err)
			if len(m.providers) == 0 {
				m.addMessage("system",
					"Provider catalog still empty after retry: "+msg.Err+"\n"+
						"/provider list needs GET /providers — try /status or restart chat.",
					"error",
				)
			}
			return m, nil
		}
		if len(msg.Providers) == 0 {
			return m, nil
		}
		m.providers = msg.Providers
		if m.model == "" && m.provider != "" {
			if models := modelsForProvider(m.providers, m.provider); len(models) > 0 {
				m.model = models[0]
				m.clampReasoningForCurrentModel() // CA-686
			}
		}
		m.bindActiveAccountForProvider()
		m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
		m.refreshSessionPanel()
		n := opencodeCatalogModelCount(m.providers)
		if n > 0 && n < minOpencodeCatalogModels {
			m.addMessage("system", fmt.Sprintf("OpenCode catalog still warming (%d models) — retrying in background…", n), "")
		} else {
			m.addMessage("system", fmt.Sprintf("Provider catalog loaded (%d). /provider to list.", len(m.providers)), "")
		}
		var cmds []tea.Cmd
		cmds = append(cmds, m.cmdLoadSkills(false))
		if cmd := m.scheduleOpencodeCatalogRetryCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.maybeFetchOpencodeVariants(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case ProvidersWarmRetryMsg:
		m.providersWarmRetries++
		if !needsOpencodeCatalogWarmRetry(m.providers) {
			return m, nil
		}
		return m, m.cmdLoadProvidersCatalog()

	case opencodeVariantsMsg:
		if msg.Err != "" {
			tuiLog("opencodeVariantsMsg err model=%q err=%q", msg.Model, msg.Err)
			return m, nil
		}
		for pi := range m.providers {
			if !strings.EqualFold(m.providers[pi].Key, "opencode") {
				continue
			}
			for mi := range m.providers[pi].Models {
				if strings.EqualFold(m.providers[pi].Models[mi].ModelID(), msg.Model) {
					m.providers[pi].Models[mi].SupportedReasoningEfforts = msg.Efforts
					m.providers[pi].Models[mi].DefaultReasoningEffort = msg.Default
				}
			}
		}
		if strings.EqualFold(m.provider, "opencode") && strings.EqualFold(m.model, msg.Model) {
			m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
			m.clampReasoningForCurrentModel()
			m.refreshSessionPanel()
			m.addMessage("system", fmt.Sprintf("Reasoning options for %s: %s", msg.Model, strings.Join(msg.Efforts, " · ")), "")
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
			m.inputExpectedSince = time.Now()
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
		if m.project != nil && strings.TrimSpace(m.project.Path) != "" && (firstLoad || m.lspStatusPath != m.project.Path) {
			// Bound project (re)loaded: refresh the LSP sidebar hint.
			m.lspStatusPath = m.project.Path
			m.lspStatus = nil
			cmds = append(cmds, m.cmdFetchLSPStatus(m.project.Path))
		}
		if firstLoad && m.project != nil && len(m.chatList) == 0 {
			// Cold start with a bound project: the history picker renders
			// from m.chatList, so init-loading must not report ready before
			// the first list settles — a slow/failed silent fetch otherwise
			// leaves /open stuck on "loading chats…" with no error.
			m.chatWaitPending = true
			m.chatListInflight = true
			m.statusMsg = "loading chats…"
			cmds = append(cmds, m.cmdFetchChats(true), tea.Tick(chatStartupWaitTimeout, func(time.Time) tea.Msg { return chatLoadTimeoutMsg{} }))
		}
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
			m.handleOnboardingAfterSession(msg, true)
		} else if m.supabaseJustConfigured {
			// F-2: a first-run Supabase save re-arms the onboarding chain —
			// the follow-up SessionDefaultsMsg is not firstLoad, but the user
			// still needs login (then project binding).
			m.handleOnboardingAfterSession(msg, false)
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
		if firstLoad && len(m.providers) == 0 {
			// CA-535: session unlock must not wait on a slow /providers scan.
			// CA-657: empty catalog after that budget is sticky — backfill like
			// the project-catalog retry, without holding sessionLoading.
			m.addMessage("system", "Provider catalog still loading in background… /provider will fill when ready.", "")
			cmds = append(cmds, m.cmdLoadProvidersCatalog())
		} else if firstLoad && needsOpencodeCatalogWarmRetry(m.providers) {
			m.addMessage("system", fmt.Sprintf("OpenCode models still loading (%d) — will refresh in background…", opencodeCatalogModelCount(m.providers)), "")
			if cmd := m.scheduleOpencodeCatalogRetryCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
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
		// BUG-355 F1: always clear the background-refresh in-flight flag;
		// stamp the fetch time only on success so a failed refresh retries
		// on the next picker keypress instead of sticking the error for the
		// whole interval (same policy as the BUG-351 flow picker).
		m.chatListInflight = false
		waitedChats := m.chatWaitPending
		m.chatWaitPending = false
		if msg.Silent {
			if msg.Err == "" {
				m.chatList = mergeChatListSyncStatus(m.chatList, msg.Items)
				m.chatListFetchedAt = time.Now()
				if waitedChats {
					m.passChatGate()
				}
			} else if waitedChats && len(m.chatList) == 0 {
				// Startup fetch failed before any list ever arrived: the
				// picker would otherwise sit on "loading chats…" forever.
				// Pass degraded but loud; the picker retries per keypress.
				m.passChatGate()
				m.addMessage("system", "Chat list failed: "+msg.Err, "error")
			}
			return m, nil
		}
		if msg.Err != "" {
			m.addMessage("system", "Chat list failed: "+msg.Err, "error")
			if waitedChats {
				m.passChatGate()
			}
			return m, nil
		}
		m.chatList = mergeChatListSyncStatus(m.chatList, msg.Items)
		m.chatListFetchedAt = time.Now()
		if waitedChats {
			m.passChatGate()
		}
		m.addMessage("system", formatChatListWithRemote(msg.Items, m.remoteChatList), "")
		return m, nil

	case ChatDeletedMsg:
		return m.handleChatDeleted(msg)

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
		m.chatBackfillDone = false
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
		if kind == "flow" && m.gate == nil {
			cmds = append(cmds, m.cmdHydratePendingFromSnapshot())
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
		// Hydrate sub-agents so /agent Tab, step [open] and the sidebar agents
		// section work after /open — chat runs too (BUG-335: spawned children
		// were invisible after a TUI/runner restart because the hydrate only
		// ran for flow opens).
		if m.runHandle != nil {
			cmds = append(cmds, m.cmdHydrateAgentRuns(m.runHandle.RunID))
			// One-shot graph fetch seeds loop state (done/blocked/running) so an
			// opened blocked flow shows the awaiting-user banner instead of arming
			// [stop] on the stale live handle (BUG-231 run-189839 parity with
			// Desktop refreshAgentGraph on history open).
			cmds = append(cmds, m.cmdHydrateAgentGraph(m.runHandle.RunID))
			cmds = append(cmds, m.cmdHydrateDispatchAttention(m.runHandle.RunID))
		}
		// Catalog may still be loading — refresh flow list so status label can use name.
		if kind == "flow" && len(m.flowWorkflows) == 0 && len(m.flowBuiltins) == 0 && !m.flowListInflight {
			m.flowListInflight = true
			m.flowListFetchedAt = time.Now()
			cmds = append(cmds, m.cmdPrefetchFlows())
		}
		// CP-59 F4 / CA-699 Task-315 slice 3: chat history via chatTimeline
		// (provider-agnostic join — no providerKey branch, chatId only). Best
		// effort: timeline fetch failures keep the current-leg replay intact.
		// Fallback to HistoryMeta ChatID when ResumeRun's handle lacks it
		// (BUG-338 session predates chat_id column — list already stamps via
		// transcriptLegIndex, resume must not lose the chat).
		effectiveHandle := handle
		effectiveChatID := strings.TrimSpace(effectiveHandle.ChatID)
		if effectiveChatID == "" {
			effectiveChatID = strings.TrimSpace(msg.HistoryMeta.ChatID)
			effectiveHandle.ChatID = effectiveChatID
		}
		if effectiveChatID != "" && isChatHandle(&effectiveHandle) {
			if cmd := m.cmdBackfillChatTimeline(effectiveHandle); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else if strings.TrimSpace(effectiveHandle.ChatID) == "" && effectiveHandle.RunKind == "workflow" {
			// BUG-355 F2: chat-less workflow runs restore their transcript
			// from the run-scoped timeline (persisted under the run id).
			// Separate cmd — BUG-338 pins the chat backfill to stay nil here.
			if cmd := m.cmdBackfillRunTimeline(effectiveHandle); cmd != nil {
				cmds = append(cmds, cmd)
			}
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
		m.loginModalOpen = false
		m.loginModalBusy = false
		m.loginModalPassword = ""
		m.loginModalErr = ""
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
		if m.project == nil && m.cfg.ProjectPath != "" && !m.projectWizardOpen {
			// F-2: after a successful login the D-2 chain continues — an
			// unbound project path still needs the onboarding wizard.
			m.openProjectWizard(m.cfg.ProjectPath)
		}
		m.sessionLoading = true
		m.statusMsg = "loading session..."
		m.addMessage("system", "Reloading session after login…", "")
		// Same banner-until-catalog contract as cold start: typing stays free,
		// send stays blocked until SessionDefaultsMsg (or the 45s safety net).
		return m, tea.Batch(
			m.cmdLoadSessionDefaults(),
			tea.Tick(45*time.Second, func(time.Time) tea.Msg { return sessionLoadTimeoutMsg{} }),
		)

	case ProjectCreatedMsg:
		m.projectWizardOpen = false
		m.projectWizardBusy = false
		m.projectWizardErr = ""
		m.project = &msg.Project
		m.projects = append(m.projects, msg.Project)
		m.statusMsg = "ready"
		m.addMessage("system", fmt.Sprintf("Project created and bound: %s (%s) — you can chat now.", msg.Project.Name, msg.Project.Platform), "")
		if notice := m.tryApplyPendingFlowRestore(); notice != "" {
			m.addMessage("system", notice, "")
		}
		var cmds []tea.Cmd
		if len(m.chatList) == 0 {
			m.chatListInflight = true
			cmds = append(cmds, m.cmdPrefetchChats())
		}
		cmds = append(cmds, m.cmdRefreshProjectContext())
		if strings.TrimSpace(msg.Project.Path) != "" {
			m.lspStatusPath = msg.Project.Path
			m.lspStatus = nil
			cmds = append(cmds, m.cmdFetchLSPStatus(msg.Project.Path))
		}
		return m, tea.Batch(cmds...)

	case LSPStatusMsg:
		// Drop stale responses for a previously bound project.
		if msg.Path != "" && m.lspStatusPath != "" && msg.Path != m.lspStatusPath {
			return m, nil
		}
		if msg.Path != "" {
			m.lspStatusPath = msg.Path
		}
		m.lspStatus = msg.Status
		return m, nil

	case SupabaseConfigSavedMsg:
		m.supabaseSetupModalOpen = false
		m.supabaseSetupModalBusy = false
		m.supabaseSetupModalErr = ""
		m.addMessage("system", "Supabase workspace credentials saved successfully!", "")
		m.supabaseJustConfigured = true
		m.sessionLoading = true
		m.statusMsg = "reloading..."
		cmds := []tea.Cmd{
			m.cmdLoadSessionDefaults(),
			tea.Tick(45*time.Second, func(time.Time) tea.Msg { return sessionLoadTimeoutMsg{} }),
		}
		return m, tea.Batch(cmds...)

	case FlowListMsg:
		m.flowListInflight = false
		// BUG-351: a failed background refresh must not wipe a good cache —
		// keep showing the last good list until a fetch succeeds.
		if msg.CatalogErr == "" {
			m.flowBuiltins = msg.Builtins
			m.flowWorkflows = msg.Workflows
			m.flowListFetchedAt = time.Now()
		}
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

	case ChatSwitchedMsg:
		// CP-59 Task-315 (SD-26 D-7): adopt in place — transcript kept, no
		// client-synthesized divider on success (the seed turn carries it).
		m.applyChatSwitched(msg)
		if msg.Err == nil && msg.Resp != nil {
			cmds := []tea.Cmd{m.cmdStartOrchestrationStream()}
			if m.chatPostureDirty {
				m.chatPostureDirty = false
				cmds = append(cmds, m.cmdSaveChatPosture(m.chatPostureCfg))
			}
			return m, tea.Batch(cmds...)
		}
		if m.chatPostureDirty {
			m.chatPostureDirty = false
			return m, m.cmdSaveChatPosture(m.chatPostureCfg)
		}
		return m, nil

	case ReattachedMsg:
		// CP-59 Task-315 slice 3: a detached chat reattached — a fresh local
		// leg exists; the queued prompt sends on it via the normal path.
		m.chatDetached = false
		m.chatBackfillDone = false
		if msg.Err != nil {
			m.addMessage("system", "Reattach failed: "+msg.Err.Error()+" — the prompt was not sent; try again or /new", "error")
			return m, nil
		}
		h := msg.Handle
		m.runHandle = &h
		if h.ProviderKey != "" {
			m.provider = string(h.ProviderKey)
		}
		m.lastEventSeq = h.LastEventSeq
		m.client.NoteLastSeq(h.RunID, h.LastEventSeq)
		m.refreshSessionPanel()
		if prompt := m.pendingPrompt; prompt != "" {
			m.pendingPrompt = ""
			return m, m.cmdSendTurn(prompt)
		}
		return m, nil

	case chatTimelineBackfillMsg:
		// CP-59 Task-315 slice 3: /open restore-by-chat — prior legs' turns
		// render from the chat timeline; the detached flag derives from legs.
		if msg.Err != nil {
			m.chatDetached = msg.Detached
			m.chatBackfillDone = true
			note := "Chat history unavailable: " + msg.Err.Error() + " — showing current leg only"
			if msg.RunScoped {
				note = "Run transcript unavailable: " + msg.Err.Error() + " — steps panel still shows the run timeline"
			}
			m.addMessage("system", note, "error")
			return m, nil
		}
		m.chatDetached = msg.Detached
		m.renderChatTimelineBackfill(msg)
		return m, nil

	case detachedNoticeMsg:
		return m, nil

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
			m.turnLive = false
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
		// BUG-341: 417944 first turn ended on tool-only output with a blank
		// assistant until the next prompt's replay. If the turn is still live
		// (turn_completed not yet seen via any stream), keep it busy and let
		// the orch stream deliver the terminal event — do not settle to "done"
		// prematurely and do not clear the thinking placeholder yet.
		if m.turnLive {
			if m.connStatus == ConnRunning {
				// keep "turn running…" / "thinking…" — do not flip to done
			} else if m.thinkingIndex() >= 0 {
				m.connStatus = ConnRunning
				m.statusMsg = "thinking…"
			} else {
				m.connStatus = ConnRunning
				m.statusMsg = "turn running…"
			}
			cmds := []tea.Cmd{m.cmdRefreshStepsRuntime()}
			if m.runHandle != nil && m.orchStream == nil {
				cmds = append(cmds, m.cmdStartOrchestrationStream())
			}
			cmds = append(cmds, m.cmdHydratePendingFromSnapshot())
			return m, tea.Batch(cmds...)
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
		// Task-322: run-level provider/model posture (per-step fallback).
		m.flowStepsProvider = strings.TrimSpace(msg.Provider)
		m.flowStepsModel = strings.TrimSpace(msg.Model)
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
		// Steps still missing a child [open] chip (e.g. a slower/empty hydrate) →
		// re-arm a bounded retry so the chip appears without waiting for the next
		// steps transition (CA-528).
		cmds = append(cmds, m.cmdHydrateAgentRunsIfNeeded())
		// run-127174: step already WAITING but SSE graph used agentGraphSnapshot (TUI decoded only agentGraph) → loop still "running" and Thinking spins. Hydrate graph when any step is WAITING and no child is live.
		hasWaiting := false
		for _, s := range msg.Steps {
			if strings.EqualFold(strings.TrimSpace(string(s.Status)), "WAITING_USER_APPROVAL") {
				hasWaiting = true
				break
			}
		}
		if hasWaiting && !m.flowLoopBlocked() && !m.flowHasActiveAgents() && m.runHandle != nil {
			cmds = append(cmds, m.cmdHydrateAgentGraph(m.runHandle.RunID))
		}
		return m, tea.Batch(cmds...)

	case pasteBurstSettleMsg:
		// A raw-paste burst finished quietly — collapse it to a token without
		// waiting for the next keystroke.
		m.pasteBurst.settlePending = false
		if !m.pasteBurst.active && !m.pasteBurst.rejectArmed {
			return m, nil
		}
		if pasteNow().Sub(m.pasteBurst.lastRuneAt) >= burstSettle {
			wasActive := m.pasteBurst.active
			wasRejectArmed := m.pasteBurst.rejectArmed
			if wasActive && m.rejectWindowsRawPaste {
				if wasRejectArmed {
					if runes := []rune(m.inputValue); len(runes) > m.pasteBurst.start {
						if m.pasteBurst.start == 0 || len(runes) >= m.pasteBurst.start {
							m.inputValue = string(runes[:m.pasteBurst.start])
							m.setInputCaret(m.pasteBurst.start)
						}
					}
				}
				m.resetPasteBurst()
				m.syncTextareaValue()
				tuiLog("burst collapse (windows reject) active=false inputLen=%d", len([]rune(m.inputValue)))
				return m, nil
			}
			m.collapsePasteBurst()
			tuiLog("burst collapse active=false inputLen=%d", len([]rune(m.inputValue)))
			if wasActive && !m.pasteCtrlVHintShown {
				m.pasteCtrlVHintShown = true
				hint := "Use Alt+V for paste (text + image)"
				m.addMessage("system", hint, "gate")
				return m, m.showFlashToast(hint)
			}
		} else {
			// Fired early — reschedule.
			tuiLog("burst settle early, reschedule active=true")
			return m, m.cmdPasteBurstSettleOnce()
		}
		return m, nil

	case ClipboardPasteMsg:
		// PowerShell clipboard helpers on Windows previously attached to the
		// TUI console (no CREATE_NO_WINDOW) and left conhost without keys
		// after Alt+V text paste (pid 9288: 47s stall). Re-arm here for all
		// ClipboardPasteMsg branches; applyClipboardSysProcAttr prevents the
		// attach for future pastes. Keep wheel-only (1000h) so scroll survives
		// paste (was ensureMouseTrackingOff which killed wheel).
		ensureWheelMouseOn()
		// Reset any active burst state so clipboard paste and subsequent typing stay clean.
		m.resetPasteBurst()
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
		// Re-arm console input after ShellExecuteW / openPath: the viewer may
		// have stolen focus or cmd/start left the console in QuickEdit. This
		// restores mouse delivery that was lost for 49s in log pid 12736.
		disableConsoleQuickEdit()
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
		// CP-59 Task-315 slice 3: restore-by-chat — prior legs' turns backfill
		// from the chat timeline; best effort (silent on transport errors).
		return m, tea.Batch(m.cmdStreamRun(handle.RunID, handle.LastEventSeq), m.cmdBackfillChatTimeline(handle))

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
		m.turnLive = false
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
		m.turnLive = false
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
		m.turnLive = false
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

	case wheelFlushMsg:
		return m.flushPendingWheel()

	case tea.KeyMsg:
		m.markInputAlive()
		return m.handleKey(msg)

	case tea.MouseMsg:
		m.markInputAlive()
		return m.handleMouse(msg)

	case inputWatchdogMsg:
		return m.checkInputWatchdog(msg.at)
	}

	return m, nil
}

func (m *AppModel) handleEvent(ev client.ProviderEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "message_delta":
		// BUG-347: the post-switch seed turn's reply is noise — the divider
		// already rendered on switch commit; drop the seed's assistant output.
		if m.seedTurnActive {
			return m, nil
		}
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
		if m.seedTurnActive {
			return m, nil
		}
		m.appendAssistantDelta(ev.Text)

	case "turn_completed":
		// BUG-347: the seed turn ended — its FinalMessage repeats the envelope
		// reply that was already dropped, so never re-append it here.
		wasSeed := m.seedTurnActive
		m.seedTurnActive = false
		// A completed turn means the flow moved on — a still-armed gate is stale
		// (CA-536). It must not keep swallowing input.
		m.gate = nil
		m.turnLive = false
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
		} else if ev.FinalMessage != "" && !m.hasAssistantContent() && !isStepCompleteStub(ev.FinalMessage) && !wasSeed {
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
		m.seedTurnActive = false
		m.turnLive = false
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
		// BUG-347: a real user turn (non-empty, non-envelope prompt) on the
		// new leg disarms the seed guard even if turn_completed was missed.
		if m.seedTurnActive && ev.Prompt != "" && !strings.HasPrefix(ev.Prompt, client.HandoffPromptPrefix) {
			m.seedTurnActive = false
		}
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

	case "user_decision_card_requested":
		// CP-62 P-3 (Task-345): structured escalation card. Arm the decision
		// state; the next input submits the chosen option id as parked-run
		// feedback (Task-346 matches it back). A payload without usable
		// options keeps the prose card (Q-1 wrap-around).
		if ev.DecisionCard != nil && len(ev.DecisionCard.Options) > 0 {
			m.decisionCard = &DecisionCardState{
				RunID:       ev.WorkflowRunID,
				Question:    ev.DecisionCard.Question,
				Options:     ev.DecisionCard.Options,
				Recommended: ev.DecisionCard.Recommended,
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "decision"
			m.addMessage("system", formatDecisionCardMessage(ev.DecisionCard), "decision")
		} else {
			m.addMessage("system", "request_user_decision arrived without a usable card — answer in prose.", "gate")
		}

	case "token_usage_updated":
		if ev.TokenUsage != nil {
			m.lastTokens = ev.TokenUsage
			if ev.TokenUsage.ModelContextWindow != nil && *ev.TokenUsage.ModelContextWindow > 0 {
				m.modelContextWin = *ev.TokenUsage.ModelContextWindow
			}
		}

	case "agent_graph_updated":
		if g := ev.EffectiveAgentGraph(); g != nil {
			// Stale-parent guard (Desktop run-63960): ignore graphs for a
			// different run so a late blocked refresh cannot overwrite a newer
			// Continue/Stop result.
			if m.runHandle == nil || strings.TrimSpace(g.ParentRunID) == "" ||
				strings.TrimSpace(g.ParentRunID) == m.runHandle.RunID {
				m.applyAgentGraph(g)
			}
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
	// Flush a held SS3 'O' before any non-rune key (Backspace, Esc, VK F-keys
	// from Windows Terminal, …) so the user's typed 'O' is never lost when
	// the ConPTY SS3 suffix never arrives.
	if m.ss3.waiting && msg.Type != tea.KeyRunes {
		m.ss3.waiting = false
		m.insertInputAtCursor("O")
		m.suggIdx = 0
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
		// CA-612: once reject is armed, swallow any newline until settle
		// (prevents leaked \n from mid-paste gap, even if burst chain resets).
		if m.rejectWindowsRawPaste && m.pasteBurst.rejectArmed && isBurstNewlineKey(msg) {
			m.pasteBurst.lastRuneAt = now
			return m, m.cmdPasteBurstSettleOnce()
		}
		// When raw paste reject is active, reject raw flood Enters.
		if m.rejectWindowsRawPaste && isBurstNewlineKey(msg) {
			b := &m.pasteBurst
			if b.active || (b.chainLen >= 1 && now.Sub(b.lastRuneAt) < burstRuneGap && !strings.HasPrefix(m.inputValue, "/")) {
				wasActive := b.active
				m.pasteBurst.lastRuneAt = now
				// Hint once per flood (on arm), not per Enter.
				if !wasActive && b.chainLen >= 1 {
					hint := "Use Alt+V for paste (text + image)"
					m.addMessage("system", hint, "gate")
					m.pasteCtrlVHintShown = true
					return m, tea.Batch(m.showFlashToast(hint), m.cmdPasteBurstSettleOnce())
				}
				return m, m.cmdPasteBurstSettleOnce()
			}
		}
		// Raw (non-bracketed) paste arrives as a flood of key events where every
		// line break is a plain Enter. Swallow those Enters as newlines while the
		// flood is active so pasting never auto-submits per line.
		if isBurstNewlineKey(msg) && m.handleBurstNewline(now) {
			return m, m.cmdPasteBurstSettleOnce()
		}
		if isPromptNewlineKey(msg) || isModifiedEnterNewline(msg) {
			m.inputValue += "\n"
			m.syncTextareaValue()
			return m, nil
		}
	}

	// Modal takes precedence over all other key handling.
	if m.projectWizardOpen {
		return m.handleProjectWizardKey(msg)
	}
	if m.loginModalOpen {
		return m.handleLoginModalKey(msg)
	}
	if m.supabaseSetupModalOpen {
		return m.handleSupabaseSetupModalKey(msg)
	}
	if m.modeSetupModalOpen {
		return m.handleModeSetupModalKey(msg)
	}
	if m.authPhase == AuthNone && m.attachPanelOpen {
		if handled, model, cmd := m.handleAttachPanelKey(msg); handled {
			return model, cmd
		}
	}
	// Desktop parity (CA-519): a focused sub-agent transcript is read-only. Only
	// navigation, [back], agent-cycle, and slash commands are allowed; chat text
	// input is dropped so the user cannot keep typing into the child view.
	if m.viewingChild() && !m.allowsKeyWhileViewingChild(msg) {
		return m, nil
	}
	// Task-318: pending /delete confirm swallows normal input until y/n.
	if m.deletePendingRunID != "" || len(m.deletePendingIDs) > 0 {
		switch msg.Type {
		case tea.KeyEnter:
			var ids []string
			if len(m.deletePendingIDs) > 0 {
				ids = m.deletePendingIDs
			} else if m.deletePendingRunID != "" {
				ids = []string{m.deletePendingRunID}
			}
			if len(ids) == 0 {
				m.deletePendingRunID = ""
				m.deletePendingIDs = nil
				m.deletePendingLabel = ""
				return m, nil
			}
			first := ids[0]
			remaining := ids[1:]
			m.deleteBatchTotal = len(ids)
			m.deleteBatchQueue = remaining
			m.deletePendingRunID = ""
			m.deletePendingIDs = nil
			m.deletePendingLabel = ""
			m.deleteSelected = nil
			m.clearInputValue()
			if len(ids) == 1 {
				m.addMessage("system", fmt.Sprintf("Deleting chat %s…", shortID(first)), "")
			} else {
				m.addMessage("system", fmt.Sprintf("Deleting %d chats…", len(ids)), "")
			}
			return m, m.cmdDeleteChat(first)
		case tea.KeyEscape:
			m.deletePendingRunID = ""
			m.deletePendingIDs = nil
			m.deletePendingLabel = ""
			m.deleteBatchQueue = nil
			m.deleteBatchTotal = 0
			m.addMessage("system", "Delete cancelled.", "")
			m.clearInputValue()
			return m, nil
		case tea.KeyCtrlC:
			// Allow quit even while pending — mirror top-level Ctrl+C.
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
			if m.inputValue != "" {
				m.clearInputValue()
				m.statusMsg = "prompt cleared"
				return m, nil
			}
			m.quitting = true
			return m, m.cmdShutdownAndQuit()
		case tea.KeyRunes:
			if len(msg.Runes) == 1 {
				r := msg.Runes[0]
				if r == 'y' || r == 'Y' {
					var ids []string
					if len(m.deletePendingIDs) > 0 {
						ids = m.deletePendingIDs
					} else if m.deletePendingRunID != "" {
						ids = []string{m.deletePendingRunID}
					}
					if len(ids) == 0 {
						m.deletePendingRunID = ""
						m.deletePendingIDs = nil
						m.deletePendingLabel = ""
						return m, nil
					}
					first := ids[0]
					remaining := ids[1:]
					m.deleteBatchTotal = len(ids)
					m.deleteBatchQueue = remaining
					m.deletePendingRunID = ""
					m.deletePendingIDs = nil
					m.deletePendingLabel = ""
					m.deleteSelected = nil
					m.clearInputValue()
					if len(ids) == 1 {
						m.addMessage("system", fmt.Sprintf("Deleting chat %s…", shortID(first)), "")
					} else {
						m.addMessage("system", fmt.Sprintf("Deleting %d chats…", len(ids)), "")
					}
					return m, m.cmdDeleteChat(first)
				}
				if r == 'n' || r == 'N' {
					m.deletePendingRunID = ""
					m.deletePendingIDs = nil
					m.deletePendingLabel = ""
					m.deleteBatchQueue = nil
					m.deleteBatchTotal = 0
					m.addMessage("system", "Delete cancelled.", "")
					m.clearInputValue()
					return m, nil
				}
			}
			return m, nil
		default:
			return m, nil
		}
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
		if m.inputValue != "" {
			m.clearInputValue()
			m.statusMsg = "prompt cleared"
			return m, nil
		}
		m.quitting = true
		return m, m.cmdShutdownAndQuit()

	case tea.KeyF2:
		// Task-311: F2 is the /info alias — prints the session/status dump as a
		// chat message. The sidebar itself is width-reactive, no toggle.
		if m.authPhase != AuthNone {
			return m, nil
		}
		return m.infoDump()

	case tea.KeyF3:
		// Task-311: F3 is a no-op — skills details live in the sidebar.
		return m, nil

	case tea.KeyF4:
		// Task-311: F4 is a no-op — status details live in the sidebar.
		return m, nil

	case tea.KeyCtrlV:
		if m.authPhase != AuthNone {
			return m, nil
		}
		if m.rejectWindowsRawPaste || runtime.GOOS == "windows" {
			if !m.pasteBurst.rejectArmed {
				m.pasteBurst.rejectArmed = true
				if !m.pasteBurst.active {
					m.pasteBurst.active = true
					m.pasteBurst.start = len([]rune(m.inputValue))
					m.pasteBurst.buf = nil
					m.pasteBurst.lastRuneAt = pasteNow()
				}
			}
			hint := "Use Alt+V for paste (text + image)"
			m.addMessage("system", hint, "gate")
			m.pasteCtrlVHintShown = true
			return m, tea.Batch(m.showFlashToast(hint), m.cmdPasteBurstSettleOnce())
		}
		// Prefer image clipboard; falls back to text.
		return m, m.cmdClipboardPaste()

	case tea.KeyEscape:
		if m.authPhase != AuthNone {
			m.authPhase = AuthNone
			m.authEmail = ""
			m.clearInputValue()
			m.addMessage("system", "Login cancelled.", "")
			return m, nil
		}
		if m.gate != nil && m.gate.AwaitingCustom {
			m.gate.AwaitingCustom = false
			m.statusMsg = "custom gate cancelled"
			return m, nil
		}
		if m.actionRingFocus {
			m.actionRingFocus = false
			m.statusMsg = "action ring unfocused"
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
		// Return from a focused sub-agent transcript to main (Esc, per user
		// request). /agents <name> is the only way to enter a child view.
		if m.viewingChild() {
			m.restoreMainTranscript()
			m.addMessage("system", "Returned to main transcript (Esc).", "")
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
			wasDelete := strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.inputValue)), "/delete")
			m.clearInputValue()
			if wasDelete {
				m.deleteSelected = nil
			}
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
			it := items[(m.suggIdx-1+len(items))%len(items)]
			if it.kind == "delete" && strings.TrimSpace(it.value) != "" {
				m.suggIdx = (m.suggIdx - 1 + len(items)) % len(items)
				m.toggleDeleteSelection(it.value)
				m.retargetDeleteSuggestion(it.value)
				return m, nil
			}
			// Shift-Tab cycles backwards
			if len(items) > 0 {
				m.suggIdx = (m.suggIdx - 1 + len(items)) % len(items)
				m.applySuggestion(items)
				return m, m.cmdMaybePrefetchPickers()
			}
		}
		if m.authPhase == AuthNone && m.actionRingKeysActive() {
			if handled, model, cmd := m.handleActionRingKey(tea.KeyMsg{Type: tea.KeyLeft}); handled {
				return model, cmd
			}
		}
		return m, nil

	case tea.KeyTab:
		if m.authPhase != AuthNone {
			return m, nil
		}
		// BUG-333: with a blocking card pending (approval/question/gate) the
		// ring owns Tab BEFORE any passive suggestion list — the operator saw
		// a mounted gate whose Tab did nothing while a suggestion source was
		// alive. Diagnostic log eases the next live repro.
		if m.approval != nil || m.question != nil || m.gate != nil {
			tuiLog("tab-ring: active=%v sugg=%d input=%q idx=%d focus=%v",
				m.actionRingKeysActive(), len(m.collectSuggestions()), m.inputValue, m.actionRingIdx, m.actionRingFocus)
		}
		if m.actionRingKeysActive() {
			if handled, model, cmd := m.handleActionRingKey(tea.KeyMsg{Type: tea.KeyRight}); handled {
				return model, cmd
			}
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
			if it.kind == "delete" && strings.TrimSpace(it.value) != "" {
				// Skill-like multi-select: Tab toggles tick, picker stays open
				m.toggleDeleteSelection(it.value)
				m.retargetDeleteSuggestion(it.value)
				return m, nil
			}
			m.applySuggestion(items)
			return m, m.cmdMaybePrefetchPickers()
		}
		// Empty input + no picker/ring: Tab cycles the chat posture
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
			now := time.Now()
			// Wheel on Windows with mouse off is rapid KeyUp burst (3+ in 150ms).
			// Two rapid Ups are still history (test TestPromptHistory_UpDownRecall
			// does 2 Ups back-to-back); three within 150ms → wheel → scroll.
			if m.lastHistoryKeyType == tea.KeyUp && !m.lastHistoryKeyAt.IsZero() && !m.prevHistoryKeyAt.IsZero() && now.Sub(m.prevHistoryKeyAt) < 150*time.Millisecond {
				if m.lastHistoryWasNav {
					m.navigatePromptHistory(-1)
				}
				// Also undo the previous history if it was nav (second in burst)
				// The prev was already counted, but we only did one nav so far.
				// For 3-burst, we need to undo both previous navs if they were nav.
				// Simpler: if we are in burst, ensure we are back to draft and scroll.
				if m.promptHistIdx != -1 {
					// If still browsing after undo, reset to draft
					m.promptHistIdx = -1
					m.inputValue = m.promptDraft
					m.inputCursor = -1
					if m.mirrorReady() {
						m.syncTextareaValue()
					}
				}
				m.scrollTranscript(3)
				m.prevHistoryKeyAt = m.lastHistoryKeyAt
				m.lastHistoryKeyAt = now
				m.lastHistoryKeyType = tea.KeyUp
				m.lastHistoryWasNav = false
				m.suggIdx = 0
				return m, nil
			}
			m.navigatePromptHistory(1)
			m.prevHistoryKeyAt = m.lastHistoryKeyAt
			m.lastHistoryKeyAt = now
			m.lastHistoryKeyType = tea.KeyUp
			m.lastHistoryWasNav = true
			m.suggIdx = 0
			return m, nil
		}

	case tea.KeyDown:
		if m.authPhase == AuthNone {
			if n := len(m.collectSuggestions()); n > 0 {
				m.suggIdx = (m.suggIdx + 1) % n
				return m, nil
			}
			now := time.Now()
			if m.lastHistoryKeyType == tea.KeyDown && !m.lastHistoryKeyAt.IsZero() && !m.prevHistoryKeyAt.IsZero() && now.Sub(m.prevHistoryKeyAt) < 150*time.Millisecond {
				if m.lastHistoryWasNav {
					m.navigatePromptHistory(1)
				}
				if m.promptHistIdx != -1 {
					m.promptHistIdx = -1
					m.inputValue = m.promptDraft
					m.inputCursor = -1
					if m.mirrorReady() {
						m.syncTextareaValue()
					}
				}
				m.scrollTranscript(-3)
				m.prevHistoryKeyAt = m.lastHistoryKeyAt
				m.lastHistoryKeyAt = now
				m.lastHistoryKeyType = tea.KeyDown
				m.lastHistoryWasNav = false
				m.suggIdx = 0
				return m, nil
			}
			m.navigatePromptHistory(-1)
			m.prevHistoryKeyAt = m.lastHistoryKeyAt
			m.lastHistoryKeyAt = now
			m.lastHistoryKeyType = tea.KeyDown
			m.lastHistoryWasNav = true
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
					if m.mirrorReady() {
						m.syncTextareaValue()
					}
					m.suggIdx = 0
					if n := len(attachedSkillNames(m.selectedSkills)); n > 0 {
						m.statusMsg = fmt.Sprintf("skills:%d attached — prompt kept", n)
					} else {
						m.statusMsg = "skill picker closed"
					}
					return m, nil
				}
				if it.kind == "delete" {
					// Enter on /delete picker: delete ticked set, or highlighted, or all
					var ids []string
					if len(m.deleteSelected) > 0 {
						for _, ch := range m.chatList {
							id := strings.TrimSpace(ch.RunID)
							if id != "" && m.deleteSelected[id] {
								ids = append(ids, id)
							}
						}
					} else if strings.EqualFold(strings.TrimSpace(it.value), "all") {
						for _, ch := range m.chatList {
							id := strings.TrimSpace(ch.RunID)
							if id != "" {
								ids = append(ids, id)
							}
						}
					} else if strings.TrimSpace(it.value) != "" {
						ids = []string{strings.TrimSpace(it.value)}
					}
					if len(ids) == 0 {
						return m, nil
					}
					m.deletePendingIDs = ids
					m.deletePendingRunID = ids[0]
					m.deletePendingLabel = ""
					m.clearInputValue()
					m.suggIdx = 0
					if len(ids) == 1 {
						label := chatLabelForRunID(m.chatList, ids[0])
						m.addMessage("system", fmt.Sprintf("Delete %q (%s)? [y]es / [n]o  (Enter=y, Esc=n)", label, shortID(ids[0])), "")
					} else {
						m.addMessage("system", fmt.Sprintf("Delete %d chats? [y]es / [n]o  (Enter=y, Esc=n) — cannot undo", len(ids)), "")
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
					// Action rows only expand the next picker (provider connect, /image open|rm, mode-setup steps) —
					// except immediate-execution actions like /provider refresh|reload (CA-687), which run right away.
					expandsPicker := it.kind == "provider-action" || it.kind == "image-sub-next" || it.kind == "mode-setup-posture" || (it.kind == "mode-setup-field" && !strings.HasSuffix(strings.ToLower(strings.TrimSpace(it.value)), " clear")) || (it.kind == "flow" && isVibeCpIngestFlow(it.value))
					if expandsPicker && !(it.kind == "provider-action" && providerImmediateAction(it.value)) {
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
		if m.authPhase == AuthNone && m.actionRingEnterActivates() {
			return m.activateHighlightedAction()
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
		if handled, model, cmd := m.handleActionRingKey(msg); handled {
			return model, cmd
		}
		m.moveInputCursor(-1)
		return m, nil

	case tea.KeyRight:
		if handled, model, cmd := m.handleActionRingKey(msg); handled {
			return model, cmd
		}
		m.moveInputCursor(1)
		return m, nil

	case tea.KeyHome, tea.KeyCtrlA:
		m.inputCursor = 0
		return m, nil

	case tea.KeyEnd, tea.KeyCtrlE:
		m.inputCursor = -1
		return m, nil

	case tea.KeyBackspace, tea.KeyCtrlH:
		m.deleteInputBeforeCursor()
		m.suggIdx = 0
		// IME Telex rewrite (dd->đ) sends Backspace+runes in 3-5ms. The
		// Backspace must break the raw-paste chain, otherwise the 2 runes
		// after it are mistaken for a WT Ctrl+V flood (CA-611, log 4332).
		m.resetPasteBurst()
		return m, m.cmdMaybePrefetchPickers()

	case tea.KeyDelete:
		m.deleteInputAfterCursor()
		m.suggIdx = 0
		m.resetPasteBurst()
		return m, m.cmdMaybePrefetchPickers()

	case tea.KeySpace, tea.KeyRunes:
		// Windows ConPTY delivers SS3 F-keys ('\x1bOQ' = F2) as two rune
		// records 'O','Q' with the ESC dropped. Rebuild F1-F4 before anything
		// else — otherwise the pair lands in the composer (tui.log pid 18400)
		// and trips the raw-paste guard.
		if msg.Type == tea.KeyRunes {
			if repl, flush, consumed := m.ss3FKey(msg); consumed {
				if flush != "" {
					m.insertInputAtCursor(flush)
					m.suggIdx = 0
				}
				if repl.Type != tea.KeyRunes {
					return m.handleKey(repl)
				}
				if flush == "" {
					// Held 'O' or swallowed pair member: nothing to insert.
					return m, nil
				}
				msg = repl
			}
		}
		// Drop NUL and raw control noise universally across all platforms/terminals (only for non-bracketed paste)
		if msg.Type == tea.KeyRunes && !msg.Paste {
			if len(msg.Runes) == 0 {
				return m, nil
			}
			if msg.String() == "alt+\x00" || msg.String() == "\x00" {
				return m, nil
			}
			isOnlyNul := true
			for _, r := range msg.Runes {
				if r != 0 {
					isOnlyNul = false
					break
				}
			}
			if isOnlyNul {
				return m, nil
			}
		}
		if m.authPhase == AuthNone && msg.Type == tea.KeySpace {
			if handled, model, cmd := m.handleActionRingKey(msg); handled {
				return model, cmd
			}
		}
		if m.authPhase == AuthNone {
			if handled, model, cmd := m.handleF2StepPickerKey(msg); handled {
				return model, cmd
			}
			if handled, model, cmd := m.handleActionRingKey(msg); handled {
				return model, cmd
			}
		}
		// Bubble Tea delivers Alt+letter as KeyRunes+Alt (String() == "alt+v").
		// Handle image-paste chords before inserting the bare rune — otherwise
		// Alt+V becomes a literal "v" and never reaches the chord handler below.
		if m.authPhase == AuthNone && msg.Type == tea.KeyRunes {
			switch msg.String() {
			case "alt+v", "ctrl+shift+v":
				return m, m.cmdClipboardPaste()
			case "ctrl+v", "\x16":
				if m.rejectWindowsRawPaste || runtime.GOOS == "windows" {
					if !m.pasteBurst.rejectArmed {
						m.pasteBurst.rejectArmed = true
						if !m.pasteBurst.active {
							m.pasteBurst.active = true
							m.pasteBurst.start = len([]rune(m.inputValue))
							m.pasteBurst.buf = nil
							m.pasteBurst.lastRuneAt = pasteNow()
						}
					}
					hint := "Use Alt+V for paste (text + image)"
					m.addMessage("system", hint, "gate")
					m.pasteCtrlVHintShown = true
					return m, tea.Batch(m.showFlashToast(hint), m.cmdPasteBurstSettleOnce())
				}
				return m, m.cmdClipboardPaste()
			}
			// Bracketed paste (KeyRunes+Paste).
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
			s = strings.ReplaceAll(s, "\x00", "")
			if s == "" {
				return m, nil
			}
		}
		// Track raw (non-bracketed) paste floods so their Enters become newlines
		// instead of submits; bracketed pastes are handled above.
		if m.authPhase == AuthNone && !msg.Paste && !msg.Alt {
			m.noteBurstRune(s)
			if m.rejectWindowsRawPaste && m.pasteBurst.rejectArmed {
				m.pasteBurst.lastRuneAt = pasteNow()
				return m, m.cmdPasteBurstSettleOnce()
			}
			if m.rejectWindowsRawPaste && m.pasteBurst.active && m.pasteBurst.chainLen >= pasteCollapseMinRunes && !isSingleRepeatedRuneChain(m.pasteBurst.chainBuf) {
				m.pasteBurst.rejectArmed = true
				if m.pasteBurst.chainLen == pasteCollapseMinRunes {
					if runes := []rune(m.inputValue); len(runes) > m.pasteBurst.start {
						m.inputValue = string(runes[:m.pasteBurst.start])
						m.setInputCaret(m.pasteBurst.start)
					}
					m.syncTextareaValue()
					hint := "Use Alt+V for paste (text + image)"
					m.addMessage("system", hint, "gate")
					m.pasteCtrlVHintShown = true
					return m, tea.Batch(m.showFlashToast(hint), m.cmdPasteBurstSettleOnce())
				}
				return m, m.cmdPasteBurstSettleOnce()
			}
		}
		m.insertInputAtCursor(s)
		m.suggIdx = 0
		// Auto-collapse the burst ~burstSettle after it stops (no keystroke needed).
		if m.pasteBurst.active {
			return m, tea.Batch(m.cmdMaybePrefetchPickers(), m.cmdPasteBurstSettleOnce())
		}
		return m, m.cmdMaybePrefetchPickers()
	}
	// Alt+V / ctrl+shift+v when not delivered as KeyRunes (some terminals).
	if m.authPhase == AuthNone {
		switch msg.String() {
		case "alt+v", "ctrl+shift+v":
			return m, m.cmdClipboardPaste()
		case "ctrl+v", "\x16":
			if m.rejectWindowsRawPaste || runtime.GOOS == "windows" {
				hint := "Use Alt+V for paste (text + image)"
				m.addMessage("system", hint, "gate")
				m.pasteCtrlVHintShown = true
				return m, m.showFlashToast(hint)
			}
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
	flowBuiltins, flowWorkflows := m.flowCatalogForWorkingMode()
	if flows := filterFlowSuggestions(in, flowBuiltins, flowWorkflows, projectID); len(flows) > 0 {
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
	if del := filterDeleteSuggestionsWithSelected(in, m.chatList, m.remoteChatList, m.deleteSelected); len(del) > 0 {
		return del
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
	if ok, _ := parseDeleteArgPrefix(in); ok {
		if len(m.chatList) == 0 {
			return []suggestItem{{value: "", detail: "loading chats…", kind: "delete", slash: "/delete"}}
		}
		return []suggestItem{{value: "", detail: "(no matching chats)", kind: "delete", slash: "/delete"}}
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
	if reasonSugg := filterReasoningSuggestions(in, m.reasoningEffort, m.modelReasoningEfforts()); len(reasonSugg) > 0 {
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
	if vibeSugg := filterVibeArgSuggestions(in); len(vibeSugg) > 0 {
		return vibeSugg
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
	} else if it.kind == "flow" || it.kind == "history" || it.kind == "model" || it.kind == "reasoning" || it.kind == "provider" || it.kind == "provider-connect" || it.kind == "provider-action" || it.kind == "provider-install" || it.kind == "provider-account" || it.kind == "skill" || it.kind == "agent" || it.kind == "image-sub" || it.kind == "image-sub-next" || it.kind == "image-open" || it.kind == "image-rm" || it.kind == "mode-setup-posture" || it.kind == "mode-setup-field" || it.kind == "mode-setup-value" || it.kind == "mode" || it.kind == "init" || it.kind == "vibe" {
		m.setInputPreservingDraftPrefix(cmd)
	} else {
		m.setInputPreservingDraftPrefix(it.value + " ")
	}
	m.suggIdx = (idx + 1) % len(items)
}

// setInputPreservingDraftPrefix writes a slash line over the active / token only.
func (m *AppModel) setInputPreservingDraftPrefix(slashLine string) {
	m.inputValue = replaceActiveSlashWith(m.inputValue, m.inputCaretIndex(), slashLine)
	m.inputCursor = -1
	if m.mirrorReady() {
		m.syncTextareaValue()
	}
}

// suggestionAcceptValue returns the slash line to run for Enter (or empty if not actionable).
func suggestionAcceptValue(it suggestItem) string {
	switch it.kind {
	case "flow":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		if isVibeCpIngestFlow(it.value) {
			return "/flow vibe-cp-ingest @"
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
	case "vibe":
		if strings.TrimSpace(it.value) == "" {
			return ""
		}
		return "/vibe " + strings.TrimSpace(it.value)
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
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH:
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
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH:
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
	// Parked blocked loop (escalate/cap/delegate_failed) is waiting on the user
	// to Continue/Stop — chat input must stay enabled for /continue. A WAITING
	// stamp alone must not block input (run-136749 regression).
	if m.flowLoopBlocked() {
		return false
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
	// Composer [stop] is live-work only. A loop that reports done with no
	// RUNNING child (screenshot: "done" + leftover [stop], possibly with a
	// stale flowStepsActive) is not a turn the operator should Stop
	// (BUG-371). A done loop with a still-RUNNING child still arms [stop].
	if strings.EqualFold(strings.TrimSpace(m.flowLoopStatus), "done") && !m.hasLiveWorkingChild() {
		return false
	}
	if m.question != nil || m.approval != nil || m.gate != nil {
		return false
	}
	if m.pendingPrompt != "" {
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
	// Check before flowHasActiveAgents: WAITING_USER_APPROVAL is a park stamp
	// (run-136749), not live work.
	if m.flowLoopBlocked() {
		return false
	}
	if m.flowHasActiveAgents() {
		return true
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
	// A completed flow loop is not live work — stop spinner / thinking immediately.
	if m.flowLoopDone() {
		return false
	}
	// Decision states first: an agent waiting on the user reports a
	// "waiting_user_approval" status, so flowHasActiveAgents must not win here.
	if m.gate != nil || m.question != nil || m.approval != nil {
		return false
	}
	if m.pendingPrompt != "" {
		return true
	}
	if m.hasUnresolvedAttention() {
		return false
	}
	// Blocked park (escalate/cap/delegate_failed) is waiting on the user, not
	// live work — check before flowHasActiveAgents: WAITING_USER_APPROVAL is a
	// park stamp (run-136749), not a running child.
	if m.flowLoopBlocked() {
		return false
	}
	if m.hasLiveWorkingChild() {
		return true
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
	if (m.sessionLoading || m.chatWaitPending) && !strings.HasPrefix(strings.TrimSpace(input), "/") {
		msg := "Still loading session — chat is disabled until ready. (F2/F4 still work)"
		if !m.sessionLoading && m.chatWaitPending {
			msg = "Still loading chats — chat is disabled until ready. (F2/F4 still work)"
		} else if len(m.providers) == 0 {
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

	// CP-62 P-3 (Task-345): an armed decision card consumes the next input —
	// an option number/id/label submits that option; any other text is the
	// Q-1 prose fallback sent verbatim to the parked run.
	if m.decisionCard != nil {
		return m.handleDecisionCardInput(input)
	}

	if !m.canSend() {
		m.addMessage("system", "Child transcript is read-only. Return to main (/agent main).", "error")
		return m, nil
	}

	// Task-325 UX (live request run-577686): plain text typed while a flow is
	// parked IS the feedback — no "/continue" prefix needed. (A chat turn
	// would just 409 flow_awaiting_user server-side, so plain chat is useless
	// while parked.) Slash lines keep their commands (handled above);
	// approval/question/gate cards keep precedence (handled above).
	if m.flowLoopBlocked() && m.runHandle != nil && !strings.HasPrefix(strings.TrimSpace(input), "/") {
		feedback := strings.TrimSpace(input)
		if feedback == "" {
			return m, nil
		}
		m.viewport.offset = 0
		m.addMessage("user", input, "")
		m.recordPromptHistory(input)
		return m, m.cmdContinueFlowWithFeedback(m.runHandle.RunID, feedback)
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
			if len(m.pendingAttach) > 0 {
				names := make([]string, 0, len(m.pendingAttach))
				for _, att := range m.pendingAttach {
					if n := strings.TrimSpace(att.OriginalName); n != "" {
						names = append(names, n)
					} else {
						names = append(names, "image")
					}
				}
				m.messages[len(m.messages)-1].Attachments = names
			}
			m.addMessage("system", formatMissingProjectHelp(m.cfg.ProjectPath, m.projects), "error")
			return m, nil
		}
	}

	m.viewport.offset = 0
	m.addMessage("user", input, "")
	if len(m.pendingAttach) > 0 {
		names := make([]string, 0, len(m.pendingAttach))
		for _, att := range m.pendingAttach {
			if n := strings.TrimSpace(att.OriginalName); n != "" {
				names = append(names, n)
			} else {
				names = append(names, "image")
			}
		}
		m.messages[len(m.messages)-1].Attachments = names
	}
	m.recordPromptHistory(input)
	// CA-537: no "Thinking" row in the chat timeline — the spinner animates on
	// the status line + F2 RUNNING step instead (workIsLive starts the ticker).
	m.statusMsg = "thinking…"
	m.connStatus = ConnRunning
	m.turnLive = true

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
// Accepts option number (1-based) or option name (case-insensitive). CA-650:
// "custom <text>" submits the custom decision with text; a bare "custom"/"3"
// (or the [Custom] chip) arms AwaitingCustom so the next Enter submits the
// typed reason — the runner rejects option=custom without customText.
func (m *AppModel) handleGateInput(input string) (tea.Model, tea.Cmd) {
	// CA-536 escape hatch: a gate armed with no decision options can never be
	// answered. Clear it so the next Enter reaches chat instead of looping the
	// empty "Gate options:" prompt forever.
	if m.gate == nil || len(m.gate.Options) == 0 {
		m.gate = nil
		return m, nil
	}
	lowInput := strings.TrimSpace(strings.ToLower(input))

	// [Custom] chip armed: the whole input is the custom reason.
	if m.gate.AwaitingCustom {
		text := strings.TrimSpace(input)
		if text == "" {
			m.addMessage("system", "Custom gate decision: type your reason, then press Enter (or Esc/backspace to cancel).", "gate")
			return m, nil
		}
		return m.submitGateCustom(text)
	}

	// "custom <text>" typed directly.
	if strings.HasPrefix(lowInput, "custom ") {
		text := strings.TrimSpace(input[len("custom"):])
		if text == "" {
			return m.armGateCustom()
		}
		return m.submitGateCustom(text)
	}

	for i, opt := range m.gate.Options {
		if lowInput == fmt.Sprintf("%d", i+1) || strings.EqualFold(lowInput, opt) {
			if opt == "custom" {
				return m.armGateCustom()
			}
			runID := m.gate.RunID
			m.gate = nil
			m.connStatus = ConnRunning
			// CA-537: the flow keeps working after a gate decision — the spinner
			// animates on the status line + F2 RUNNING step, not as a chat row.
			m.statusMsg = "thinking…"
			return m, m.cmdSubmitGateDecision(runID, opt)
		}
	}
	opts := strings.Join(optionChips(m.gate.Options), " ")
	m.addMessage("system", fmt.Sprintf("Gate options: %s (click a chip or type number/name)", opts), "gate")
	return m, nil
}

// handleDecisionCardInput interprets user input while a CP-62 P-3 (Task-345)
// decision card is armed. An option number (1-based), option id, or option
// label submits that option id as parked-run feedback (Task-346 matches it
// back to the card); any other non-empty text is the Q-1 prose fallback sent
// verbatim. Empty input is ignored.
func (m *AppModel) handleDecisionCardInput(input string) (tea.Model, tea.Cmd) {
	card := m.decisionCard
	if card == nil {
		return m, nil
	}
	text := strings.TrimSpace(input)
	if text == "" {
		m.addMessage("system", "Decision card: type the option number/name, or your own answer.", "decision")
		return m, nil
	}
	runID := card.RunID
	for i, opt := range card.Options {
		if strings.EqualFold(text, fmt.Sprintf("%d", i+1)) ||
			strings.EqualFold(text, opt.ID) ||
			strings.EqualFold(text, opt.Label) {
			m.decisionCard = nil
			m.connStatus = ConnRunning
			m.statusMsg = "thinking…"
			m.addMessage("user", opt.Label, "")
			return m, m.cmdContinueFlowWithFeedback(runID, opt.ID)
		}
	}
	m.decisionCard = nil
	m.connStatus = ConnRunning
	m.statusMsg = "thinking…"
	m.addMessage("user", text, "")
	return m, m.cmdContinueFlowWithFeedback(runID, text)
}

// formatDecisionCardMessage renders the armed card as a chat message: the
// question, numbered options with consequences (the recommended one marked),
// and the evidence citations.
func formatDecisionCardMessage(card *client.DecisionCardData) string {
	var b strings.Builder
	b.WriteString("Decision needed: " + card.Question)
	if card.Detail != "" {
		b.WriteString("\n" + card.Detail)
	}
	for i, opt := range card.Options {
		marker := ""
		if card.Recommended != "" && opt.ID == card.Recommended {
			marker = " [recommended]"
		}
		b.WriteString(fmt.Sprintf("\n  %d. %s%s — %s", i+1, opt.Label, marker, opt.Consequence))
	}
	if len(card.Evidence) > 0 {
		b.WriteString("\nEvidence:")
		for _, ev := range card.Evidence {
			ref := ev.Path
			if ev.Line > 0 {
				ref += fmt.Sprintf(":%d", ev.Line)
			}
			if ev.Excerpt != "" {
				ref += " — " + ev.Excerpt
			}
			b.WriteString("\n  - " + ref)
		}
	}
	b.WriteString("\nReply with the option number/name, or type your own answer.")
	return b.String()
}

// armGateCustom arms the [Custom] chip flow: the next Enter submits the typed
// text as the custom gate decision.
func (m *AppModel) armGateCustom() (tea.Model, tea.Cmd) {
	m.gate.AwaitingCustom = true
	m.inputValue = ""
	m.setInputCaret(0)
	if m.mirrorReady() {
		m.syncTextareaValue()
	}
	m.statusMsg = "gate custom: type your reason then Enter"
	m.addMessage("system", "Custom gate decision — type your remediation instruction, then press Enter.", "gate")
	return m, nil
}

// submitGateCustom posts the custom gate decision with the typed text and
// clears the gate card (runner requires customText for option=custom).
func (m *AppModel) submitGateCustom(text string) (tea.Model, tea.Cmd) {
	runID := m.gate.RunID
	m.gate = nil
	m.connStatus = ConnRunning
	m.statusMsg = "thinking…"
	return m, m.cmdSubmitGateDecisionCustom(runID, text)
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
	if !m.chatSupportsImages() {
		m.addMessage("system", m.imagesUnsupportedReason(), "error")
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

	case "/standardize":
		// Task-333 (CP-49): delegate to the runner's /standardize API; logic
		// lives in standardize.go (conformance scan / reverse-doc + SS-Lock).
		return m.cmdStandardize(args)

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

	case "/vibe":
		arg := ""
		if len(args) > 0 {
			arg = strings.ToLower(strings.TrimSpace(args[0]))
		}
		switch arg {
		case "off", "normal", "dev":
			m.setWorkingMode(workingmode.Dev)
			m.addMessage("system", "Working mode: normal (dev)", "")
		default:
			m.setWorkingMode(workingmode.Vibe)
			rest := strings.TrimSpace(strings.Join(args, " "))
			if rest == "" || arg == "on" {
				m.addMessage("system", "Working mode: vibe", "")
				break
			}
			if m.runHandle != nil {
				m.addMessage("system", "Cannot change flow after a run has started. Use /new first.", "error")
				break
			}
			flowID, source := workingmode.DetectVibeEntry(rest)
			if flowID == "vibe-cp-ingest" {
				if err := workingmode.RejectNonCP(source, ""); err != nil {
					m.addMessage("system", err.Error(), "error")
					break
				}
			}
			m.launch = LaunchArm{
				Mode:        ModeFlow,
				FlowRef:     flowID,
				Label:       flowID,
				SourceDocID: source,
			}
			m.mode = ModeFlow
			m.firstTurnPending = true
			m.persistSessionPrefs()
			if flowID == "vibe-cp-ingest" {
				m.addMessage("system", fmt.Sprintf("CP locked entry armed: %s. Send a prompt to start vibe-cp-ingest.", source), "")
			} else {
				m.addMessage("system", fmt.Sprintf("SS ingest armed: %s. Send a prompt to start vibe-ingest.", rest), "")
			}
		}

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
		b.WriteString("Agents (/agents <name> to open transcript · Esc returns to main):\n")
		for _, r := range runs {
			cur := ""
			if r.RunID == m.focusRunID || (m.focusRunID == "" && (strings.EqualFold(r.Role, "main") || r.RunID == m.mainRunID())) {
				cur = " *"
			}
			line := fmt.Sprintf("  %s  %s  %s%s", r.AgentName, r.Status, r.RunID, cur)
			if task := agentTaskDetail(r); task != "" {
				line = fmt.Sprintf("  %s  %s  %s  %s%s", r.AgentName, task, r.Status, r.RunID, cur)
			}
			b.WriteString(line + "\n")
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
			m.flowListInflight = true
			m.flowListFetchedAt = time.Now()
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
		flowID := args[0]
		if m.workingMode == workingmode.Dev || m.workingMode == workingmode.Vibe {
			if err := workingmode.FlowAllowedForWorkingMode(m.workingMode, flowID, "user"); err != nil {
				m.addMessage("system", err.Error(), "error")
				break
			}
		}
		if isVibeCpIngestFlow(flowID) {
			rest := strings.TrimSpace(strings.Join(args[1:], " "))
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "@"))
			if rest == "" {
				m.setInputPreservingDraftPrefix("/flow vibe-cp-ingest @")
				return m, m.cmdMaybePrefetchWorkspaceFiles()
			}
			if err := workingmode.RejectNonCP(rest, ""); err != nil {
				m.addMessage("system", err.Error(), "error")
				break
			}
			m.launch = LaunchArm{
				Mode:        ModeFlow,
				FlowRef:     "vibe-cp-ingest",
				Label:       "vibe-cp-ingest",
				SourceDocID: rest,
			}
			m.mode = ModeFlow
			m.firstTurnPending = true
			m.persistSessionPrefs()
			m.addMessage("system", fmt.Sprintf("CP locked entry armed: %s. Send a prompt to start vibe-cp-ingest.", rest), "")
			break
		}
		query := strings.Join(args, " ")
		flowBuiltins, flowWorkflows := m.flowCatalogForWorkingMode()
		arm, err := resolveFlowLaunch(flowBuiltins, flowWorkflows, projectID, query)
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
				if p := findProvider(m.providers, key); p != nil && (!p.Installed || providerCLIUnusable(*p)) {
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
				if p := findProvider(m.providers, key); p != nil && p.Installed && !providerCLIUnusable(*p) {
					m.addMessage("system", fmt.Sprintf("Provider %s is already installed.", key), "")
					break
				}
				m.addMessage("system", fmt.Sprintf("Installing provider CLI %s… (may take a minute)", key), "")
				return m, m.cmdInstallProvider(key)
			case "refresh", "reload":
				// CA-687: the TUI has no Desktop "Detect models" button — this is
				// its equivalent. Re-fetches GET /providers (fresh model
				// detection: new grok/opencode/... entries land without a
				// restart) and keeps the current provider/model selection.
				m.addMessage("system", "Refreshing provider catalog and models…", "")
				return m, m.cmdLoadProvidersCatalog()
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
				sb.WriteString("No provider catalog loaded yet. Wait for connect, or run /provider refresh.")
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
					if code == "not_installed" {
						status = "not installed"
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
				sb.WriteString("Pick provider: /provider <key>  · Switch account: /provider account <account-id>  · Connect: /provider connect  · Install: /provider install  · Refresh models: /provider refresh")
			}
			m.addMessage("system", sb.String(), "")
		} else if m.runHandle != nil {
			// CP-59 Task-315: a chat run routes provider changes through the
			// runner switch endpoint (in-place adoption, transcript kept).
			if cmd := m.routeProviderSwitch(strings.ToLower(args[0]), ""); cmd != nil {
				return m, cmd
			}
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
					m.clampReasoningForCurrentModel() // CA-686: per-model efforts
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
				sb.WriteString("Pick: type /model  then ↑↓ · Tab · Enter (provider auto-switches)\nMissing a new model? /provider refresh reloads the catalog")
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
					// CP-59 Task-315: a foreign-provider model on a live chat
					// routes through the switch endpoint instead of swapping
					// m.model inside the old provider (BUG-330 class).
					if cmd := m.routeProviderSwitch(matched.provider, matched.id); cmd != nil {
						return m, cmd
					}
					if m.runHandle != nil {
						if isChatHandle(m.runHandle) {
							m.addMessage("system", fmt.Sprintf("Cannot switch to %s \u00b7 %s on this chat (missing chat identity) \u2014 use /new", matched.provider, matched.id), "error")
						} else {
							m.addMessage("system", "Cannot change provider after a run has started. Use /new to start fresh.", "error")
						}
						return m, nil
					}
					m.provider = matched.provider
					m.bindActiveAccountForProvider()
					m.skillsCatalog = nil
					providerSwitched = true
				}
				m.model = matched.id
			} else {
				if prov := providerForModel(m.providers, want); prov != "" && !strings.EqualFold(prov, m.provider) {
					if cmd := m.routeProviderSwitch(prov, want); cmd != nil {
						return m, cmd
					}
					if m.runHandle != nil {
						if isChatHandle(m.runHandle) {
							m.addMessage("system", fmt.Sprintf("Cannot switch to %s \u00b7 %s on this chat (missing chat identity) \u2014 use /new", prov, want), "error")
						} else {
							m.addMessage("system", "Cannot change provider after a run has started. Use /new to start fresh.", "error")
						}
						return m, nil
					}
					m.provider = prov
					m.bindActiveAccountForProvider()
					m.skillsCatalog = nil
					providerSwitched = true
				}
				m.model = want
			}
			m.modelContextWin = contextWindowForModel(m.providers, m.provider, m.model)
			m.clampReasoningForCurrentModel() // CA-686: reasoning is dynamic per model
			m.persistSessionPrefs()
			if matched != nil && matched.provider != "" {
				m.addMessage("system", fmt.Sprintf("Model set to: %s · provider: %s (next prompt uses this model)", m.model, m.provider), "")
			} else {
				m.addMessage("system", fmt.Sprintf("Model set to: %s (next prompt uses this model)", m.model), "")
			}
			m.refreshSessionPanel()
			// CA-689c: correct reasoning options at selection time — fetch the
			// model's real effort list if the catalog only has the guess.
			if fetch := m.maybeFetchOpencodeVariants(); fetch != nil {
				if providerSwitched {
					return m, tea.Batch(m.cmdLoadSkills(false), fetch)
				}
				return m, fetch
			}
			if providerSwitched {
				return m, m.cmdLoadSkills(false)
			}
		}

	case "/reasoning":
		if len(args) == 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Current reasoning effort: %s\n", orDash(m.reasoningEffort)))
			sb.WriteString(reasoningEffortHelp(m) + "\n")
			sb.WriteString("Pick: type /reasoning  then ↑↓ · Tab · Enter")
			m.addMessage("system", sb.String(), "")
		} else {
			effort := strings.ToLower(strings.TrimSpace(args[0]))
			if reasoningEffortAllowed(m, effort) {
				m.reasoningEffort = effort
				m.persistSessionPrefs()
				m.addMessage("system", fmt.Sprintf("Reasoning effort set to: %s (saved)", effort), "")
			} else {
				m.addMessage("system", "Reasoning effort not supported by "+orDash(m.model)+". "+reasoningEffortHelp(m), "error")
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
		// Task-322: drop run posture + loop round with the steps.
		m.flowStepsProvider = ""
		m.flowStepsModel = ""
		m.flowLoopRound = 0
		m.flowLoopCap = 0
		m.vibeTaskIndex = 0
		m.vibeTaskTotal = 0
		m.vibeTaskName = ""
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

	case "/dumpview":
		path := filepath.Join(os.TempDir(), "flowpilot-you-view.txt")
		if err := writeYouViewDump(m, path); err != nil {
			m.addMessage("system", "dump failed: "+err.Error(), "error")
			break
		}
		return m, m.showFlashToast("dumped " + youBoxLayoutRev + " → " + path)

	case "/status":
		return m.infoDump()

	case "/info":
		// Task-311: /info prints the session/status dump (F2 aliases it). The
		// sidebar has no toggle anymore — it follows terminal width.
		return m.infoDump()

	case "/login":
		return m.beginLogin(args)

	case "/project":
		dir := m.cfg.ProjectPath
		if len(args) > 1 && args[0] == "add" {
			dir = strings.Join(args[1:], " ")
		}
		m.openProjectWizard(dir)
		return m, nil

	case "/setup", "/supabase":
		m.openSupabaseSetupModal()
		return m, nil

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

	case "/delete":
		if len(args) == 0 {
			if m.runHandle == nil || strings.TrimSpace(m.runHandle.RunID) == "" {
				m.addMessage("system", "Usage: /delete <n|runId|all> — type /delete  then Tab tick · Enter del", "")
				break
			}
			runID := strings.TrimSpace(m.runHandle.RunID)
			label := chatLabelForRunID(m.chatList, runID)
			m.deletePendingIDs = []string{runID}
			m.deletePendingRunID = runID
			m.deletePendingLabel = label
			m.addMessage("system", fmt.Sprintf("Delete %q (%s)? [y]es / [n]o  (Enter=y, Esc=n)", label, shortID(runID)), "")
			break
		}
		if len(args) == 1 && strings.EqualFold(strings.TrimSpace(args[0]), "all") {
			if len(m.chatList) == 0 {
				m.addMessage("system", "No chats to delete.", "")
				break
			}
			ids := make([]string, 0, len(m.chatList))
			for _, ch := range m.chatList {
				if id := strings.TrimSpace(ch.RunID); id != "" {
					ids = append(ids, id)
				}
			}
			m.deletePendingIDs = ids
			m.deletePendingRunID = ids[0]
			m.deletePendingLabel = ""
			m.addMessage("system", fmt.Sprintf("Delete ALL %d chats? [y]es / [n]o  (Enter=y, Esc=n) — cannot undo", len(ids)), "")
			break
		}
		runID, err := resolveChatOpenTarget(args, m.chatList)
		if err != nil {
			m.addMessage("system", err.Error()+" — type /delete  for the picker", "error")
			break
		}
		label := chatLabelForRunID(m.chatList, runID)
		m.deletePendingIDs = []string{runID}
		m.deletePendingRunID = runID
		m.deletePendingLabel = label
		m.addMessage("system", fmt.Sprintf("Delete %q (%s)? [y]es / [n]o  (Enter=y, Esc=n)", label, shortID(runID)), "")
		break

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
		// Trailing text rides as human feedback (Task-325: plan_approval
		// feedback re-enters the writer; bare /continue keeps legacy body).
		if !m.flowLoopBlocked() {
			m.addMessage("system", "No parked (blocked) flow to continue.", "")
			break
		}
		if m.runHandle == nil {
			m.addMessage("system", "No run to continue.", "")
			break
		}
		if feedback := continueFeedbackFromArgs(args); feedback != "" {
			return m, m.cmdContinueFlowWithFeedback(m.runHandle.RunID, feedback)
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

func (m *AppModel) renderChatPane(w, h int) string {
	debug := viewDebugEnabled()
	stage := time.Now()
	c := m.tuiChrome()
	var rows []string

	rows = append(rows, c.panelLines...)

	tChrome := time.Since(stage)
	stage = time.Now()
	lines := m.renderMessages()
	tMsgs := time.Since(stage)
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
	// Small padding separates transcript from the input frame; the horizontal
	// rule is removed per user request. Transcript stays on the dark canvas
	// (#0d0d0d) — only the composer frame is the elevated gray (#1e1e1e) so no
	// black/gray mix inside the input.
	rows = append(rows, "")
	rows = append(rows, "")
	if len(c.sugg) > 0 && !m.hasModalOpen() {
		rows = append(rows, strings.Split(m.renderSuggestions(c.sugg), "\n")...)
	}
	if m.hasModalOpen() && c.modalBlock != "" {
		// Render the block cached by tuiChrome — same string its
		// modalH was measured from, so the height budget is always exact.
		rows = append(rows, strings.Split(c.modalBlock, "\n")...)
	}
	if c.attachPanelBlock != "" {
		rows = append(rows, strings.Split(c.attachPanelBlock, "\n")...)
	}
	_ = len(rows)
	inputStart, inputEnd := -1, -1
	if m.sessionLoading {
		// Hide normal chat input until loading is done — show a clear loading
		// placeholder instead so the user doesn't feel the UI is hung.
		// Slash commands and F2 (/info alias) still work via allowsKeyWhileLoading.
		frames := []string{"|", "/", "-", "\\"}
		spin := frames[m.loadingFrame%len(frames)]
		msg := fmt.Sprintf(" %s Loading session · project · providers — chat locked (F2/F4 still work) ", spin)
		rows = append(rows, styleLoading.Render(truncateVisual(msg, w)))
	} else {
		inputStart = len(rows)
		rows = append(rows, strings.Split(m.renderInputLine(), "\n")...)
		inputEnd = len(rows)
	}
	// Bottom notice (Copied / error-warning) — one small line outside the
	// composer frame, at the bottom of the chat pane. The watchdog
	// "input stalled ..." banner is moved here so top-left chrome stays clean
	// (user request). Only one line, canvas bg, truncated to width.
	if n := strings.TrimSpace(c.bottomNoticeBlock); n != "" {
		rows = append(rows, strings.Split(n, "\n")...)
	}
	// Legacy flash toast above the composer is kept for old callers
	// (renderStatusLine) but is no longer rendered here — the bottom notice is
	// the single visible location. If we rendered statusBlock here too the View
	// would show the toast twice.

	// CA-532 hierarchy restored: transcript = canvas, composer = chatBar.
	stage = time.Now()
	rows = padLinesTo(rows, h)
	barW := safeTermWidth(w)
	for i, r := range rows {
		rw := barW
		if inputStart >= 0 && i >= inputStart && i < inputEnd {
			rows[i] = paintComposerRow(r, rw)
			continue
		}
		if isYouBoxRow(r) {
			rows[i] = padYouBoxRow(r, rw)
			continue
		}
		rows[i] = paintRow(r, rw, styleCanvas)
	}

	body := strings.Join(rows, "\n")
	// Do not force outer Width/Height — rows are already painted to w/barW
	// and padded to h. Forcing Width(w) would pad chat-bar rows (barW) back to
	// w and break the safeTermWidth gutter (narrow test expects <w).
	if debug {
		tuiLog("viewDbg chatPane w=%d h=%d tuiChrome=%v messages=%v paint=%v", w, h, tChrome, tMsgs, time.Since(stage))
	}
	return body
}

// ---- View -------------------------------------------------------------------

var viewDebugOnce sync.Once
var viewDebug bool

// viewDebugEnabled gates the FLOWPILOT_DEBUG_VIEW per-frame render breakdown.
// The env var is read once (process-start) so the hot path never re-parses it.
func viewDebugEnabled() bool {
	viewDebugOnce.Do(func() {
		viewDebug = strings.TrimSpace(os.Getenv("FLOWPILOT_DEBUG_VIEW")) != ""
	})
	return viewDebug
}

// cursorBlinkRelevant reports whether the input caret should keep blinking:
// anything live or active makes the caret meaningful. When false the caret is
// pinned steady and the composed frame is cached across cursor ticks (CA-633).
func (m *AppModel) cursorBlinkRelevant() bool {
	if m.sessionLoading || m.authPhase != AuthNone || m.hasModalOpen() || m.viewingChild() {
		return true
	}
	if m.workIsLive() || m.flowLoopBlocked() {
		return true
	}
	if m.driveSync != nil || m.restoreBatch != nil {
		return true
	}
	if strings.TrimSpace(m.inputValue) != "" || !m.mouseSel.empty() {
		return true
	}
	return false
}

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
	h := m.height
	if h < 1 {
		h = 1
	}
	chatW := m.chatWidth()
	chatStart := time.Now()
	chatRaw := m.renderChatPane(chatW, h)
	chatPaneDur := time.Since(chatStart)
	chat := chatRaw
	if m.useRightSidebar() {
		side := m.renderSidebarPane(m.sideWidth(), h)
		// CA-633: composeCellBuf is a pure function of its inputs — skip the
		// expensive cellbuf merge when nothing changed (idle cursor ticks keep
		// chatRaw byte-identical via the pinned caret, so idle frames cost ~0).
		if m.lastComposeChat == chatRaw && m.lastComposeSide == side &&
			m.lastComposeFullW == fullW && m.lastComposeChatW == chatW &&
			m.lastComposeSideW == m.sideWidth() && m.lastComposeH == h {
			chat = m.composeOut
		} else {
			m.composeBuilds++
			m.lastComposeChat, m.lastComposeSide = chatRaw, side
			m.lastComposeFullW, m.lastComposeChatW, m.lastComposeSideW, m.lastComposeH = fullW, chatW, m.sideWidth(), h
			m.composeOut = composeCellBuf(chatRaw, side, fullW, chatW, m.sideWidth(), h)
			chat = m.composeOut
		}
	}
	d := time.Since(viewStart)
	if d > 100*time.Millisecond {
		// CA-621: throttle View slow log so chat history open does not spam tui.log and drop keys
		if time.Since(m.lastViewSlowLog) > 5*time.Second {
			m.lastViewSlowLog = time.Now()
			tuiLog("View slow dur=%v width=%d height=%d side=%v", d, m.width, m.height, m.useRightSidebar())
		}
	}
	if viewDebugEnabled() && d > 30*time.Millisecond {
		tuiLog("viewDbg View total=%v chatPane=%v composeBuilds=%d width=%d height=%d side=%v",
			d, chatPaneDur, m.composeBuilds, m.width, m.height, m.useRightSidebar())
	}
	return chat
}

func (m *AppModel) loadingBannerText() string {
	return renderFlowpilotLoader(m.loadingFrame, m.asciiMode)
}

func suggestionVisibleLimit(sugg []suggestItem) int {
	if len(sugg) > 0 && (sugg[0].kind == "history" || sugg[0].kind == "delete" || sugg[0].kind == "skill" || sugg[0].kind == "file") {
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
		case "delete":
			kind = "delete"
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
		case "vibe":
			kind = "vibe"
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
		notInstalled := strings.Contains(strings.ToLower(sugg[i].detail), "not installed")
		if i == sel {
			marker := "> "
			if !m.asciiMode {
				marker = "▸ "
			}
			if notInstalled {
				sb.WriteString(styleError.Underline(true).Render(marker + strings.TrimLeft(line, " ")))
			} else {
				sb.WriteString(styleSuggestSel.Render(marker + strings.TrimLeft(line, " ")))
			}
		} else if notInstalled {
			sb.WriteString(styleError.Render(line))
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
	// the 4-line clamp (CA-559/CA-607). Clicking any such row toggles the
	// bubble expansion. All box rows carry the key, so the whole box is
	// clickable; the [copy] chip is hit-tested first.
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

// clampPromptLines collapses wrapped prompt lines to maxUserPromptLines and
// appends the "...." ellipsis to the last kept line, cut so the tail never
// exceeds innerW (youBox hard-slices rows at the inner width, so the tail must
// fit before framing).
func clampPromptLines(lines []string, innerW int) []string {
	if len(lines) <= maxUserPromptLines {
		return lines
	}
	kept := append([]string(nil), lines[:maxUserPromptLines]...)
	last := kept[len(kept)-1]
	ellipsisW := lipgloss.Width(userPromptEllipsis)
	target := innerW - ellipsisW
	if target < 0 {
		target = 0
	}
	if lipgloss.Width(last) > target {
		w := 0
		cut := 0
		for i, r := range []rune(last) {
			rw := lipgloss.Width(string(r))
			if w+rw > target {
				break
			}
			w += rw
			cut = i + 1
		}
		last = string([]rune(last)[:cut])
	}
	kept[len(kept)-1] = last + userPromptEllipsis
	return kept
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
	if m.useRightSidebar() {
		_, _ = h.Write([]byte{9})
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
	if m.approval != nil {
		_, _ = h.Write([]byte{4, 1})
		_, _ = h.Write([]byte(m.approval.ID))
		_, _ = h.Write([]byte(m.approval.Command))
		_, _ = h.Write([]byte(strconv.Itoa(len(m.approvals))))
	}
	if m.question != nil {
		_, _ = h.Write([]byte{4, 2})
		_, _ = h.Write([]byte(m.question.ID))
		_, _ = h.Write([]byte(strconv.Itoa(len(m.questions))))
		for _, opt := range m.question.Options {
			for k, v := range opt {
				_, _ = h.Write([]byte(k + ":" + v))
			}
		}
	}
	if m.flowLoopBlocked() {
		_, _ = h.Write([]byte{4, 3})
		_, _ = h.Write([]byte(m.flowBlockReason))
	}
	if len(m.attention) > 0 {
		_, _ = h.Write([]byte{4, 4})
		_, _ = h.Write([]byte(strconv.Itoa(len(m.attention))))
		for _, it := range m.attention {
			_, _ = h.Write([]byte(it.RunID + ":" + it.Kind + ":" + it.Reason))
		}
	}
	// BUG-333 (operator report: approval gate mounted, Tab/arrows looked dead):
	// the interactive card rows (approval/gate/question bar) render from LIVE
	// ring state — actionRingIdx, focus, keys-active, gate custom mode,
	// question selections — that is NOT part of the message content hashed
	// above. The tui.log proved actionRingIdx cycled 0↔1 while the row cache
	// kept serving the bar with a frozen highlight. Hash the ring state so any
	// keyboard move repaints the card row.
	_, _ = h.Write([]byte{5, 0})
	_, _ = h.Write([]byte(strconv.Itoa(m.actionRingIdx)))
	_, _ = h.Write([]byte{0})
	if m.actionRingFocus {
		_, _ = h.Write([]byte{5, 1})
	}
	if m.actionRingKeysActive() {
		_, _ = h.Write([]byte{5, 2})
	}
	if m.gate != nil {
		_, _ = h.Write([]byte{5, 3})
		_, _ = h.Write([]byte(m.gate.RunID))
		_, _ = h.Write([]byte(strings.Join(m.gate.Options, ",")))
		if m.gate.AwaitingCustom {
			_, _ = h.Write([]byte{5, 4})
		}
	}
	if m.question != nil && len(m.question.Selected) > 0 {
		_, _ = h.Write([]byte{5, 5})
		_, _ = h.Write([]byte(strings.Join(m.question.Selected, ",")))
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
		switch msg.Role {
		case "user":
			prefixStyle = styleUserLabel
			style = styleUser
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
		showCopy := (msg.Role == "user" || msg.Role == "assistant") && msg.FormatHint != "thinking" && strings.TrimSpace(msg.Content) != ""
		boxed := msg.Role == "user"
		if boxed {
			// CA-607: boxed user prompts clamp at maxUserPromptLines (4) with a
			// "...." tail on the last shown line; clicking the box expands the
			// full prompt (youBox attaches PromptExpandKey to every row). Wrap
			// at the box inner text width (pane - left border - right border -
			// leading space); youBox draws plain runes only so the right border
			// stays flush on every row (Ghostty SGR counting can no longer
			// shift the │ mid-row).
			innerW := width - 3
			if innerW < 8 {
				innerW = 8
			}
			lines := trimEmptyEdges(wrapText(msg.Content, innerW))
			truncatable := len(lines) > maxUserPromptLines
			if truncatable && !m.userPromptExpanded(msg.Content) {
				lines = clampPromptLines(lines, innerW)
			}
			if len(msg.Attachments) > 0 {
				chip := ""
				if len(msg.Attachments) == 1 {
					chip = "[1 image attached]"
				} else {
					chip = fmt.Sprintf("[%d images attached]", len(msg.Attachments))
				}
				lines = append(lines, chip)
			}
			msgRows := youBox(lines, width, m.asciiMode, showCopy, mi, truncatable, msg.Content)
			if mi > 0 && chatGapBefore(m.messages[mi-1], msg) {
				rows = append(rows, chatRow{})
			}
			rows = append(rows, msgRows...)
			continue
		}
		if msg.FormatHint == "question" {
			trimmed := strings.TrimSpace(msg.Content)
			if strings.HasPrefix(trimmed, "[QUESTION]") {
				msgRows := questionBox(msg.Content, width, m.asciiMode, mi)
				if mi > 0 && chatGapBefore(m.messages[mi-1], msg) {
					rows = append(rows, chatRow{})
				}
				rows = append(rows, msgRows...)
				continue
			} else if strings.HasPrefix(trimmed, "Answered:") {
				msgRows := answeredBox(msg.Content, width, m.asciiMode, mi)
				if mi > 0 && chatGapBefore(m.messages[mi-1], msg) {
					rows = append(rows, chatRow{})
				}
				rows = append(rows, msgRows...)
				continue
			}
		}
		contentWidth := width
		if prefix != "" {
			contentWidth = width - len([]rune(prefix))
			if contentWidth < 8 {
				contentWidth = width
				prefix = ""
			}
		}
		wrapped := wrapText(msg.Content, contentWidth)
		mdLines := textsToMD(trimEmptyEdges(wrapped))
		if msg.Role == "assistant" && msg.FormatHint == "" {
			mdLines = renderMarkdownRows(msg.Content, contentWidth, m.asciiMode)
		}
		var msgRows []chatRow
		fenceN := 0
		for i, ml := range mdLines {
			line := ml.Text
			lineStyle := style
			if msg.Role == "system" && strings.Contains(strings.ToLower(stripANSI(line)), "not installed") {
				lineStyle = styleError
			}
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
			// User request: remove [copy] at end of each prompt/answer.
			row := chatRow{Text: rendered, MsgIdx: mi, Copy: copyFence}
			if copyFence {
				row.CopyText = ml.CopyCode
				row.FenceIdx = fenceN
				fenceN++
			}
			msgRows = append(msgRows, row)
		}
		if mi > 0 && chatGapBefore(m.messages[mi-1], msg) {
			rows = append(rows, chatRow{})
		}
		rows = append(rows, msgRows...)
	}

	// Interactive action cards rendered directly on the chat timeline
	if m.approval != nil {
		head := "approval"
		if n := len(m.approvals); n > 1 {
			head = fmt.Sprintf("approval 1/%d", n)
		}
		chips := approvalDecisionChipsAt(m.approval, m.approvals, m.ringHighlightFor("approval"))
		rows = append(rows, chatRow{})
		rows = append(rows, chatRow{Text: styleGate.Render(head) + "  " + chips + "  " + styleSystem.Render("← → Enter · 1-9"), MsgIdx: -1})
	}
	if m.gate != nil && len(m.gate.Options) > 0 {
		var chips []string
		for i, opt := range m.gate.Options {
			chips = append(chips, renderActionRingChip(gateOptionChip(opt), m.actionRingHighlighted("gate", i)))
		}
		rows = append(rows, chatRow{})
		rows = append(rows, chatRow{Text: styleGate.Render("gate") + "  " + strings.Join(chips, "  ") + "  " + styleSystem.Render("← → Enter · 1-9"), MsgIdx: -1})
	}
	if m.question != nil {
		left := styleInputStroke.Render("┃")
		mid := styleInputStroke.Render("│")
		qbar := renderQuestionBar(left, mid, m.question, width, m.ringHighlightFor("question"))
		if n := len(m.questions); n > 1 {
			qbar = styleGate.Render(fmt.Sprintf("(%d/%d)", 1, n)) + " " + qbar
		}
		rows = append(rows, chatRow{})
		for _, ql := range strings.Split(qbar, "\n") {
			rows = append(rows, chatRow{Text: ql, MsgIdx: -1})
		}
	}
	if bar := m.renderAttentionBar(); bar != "" {
		rows = append(rows, chatRow{})
		for _, al := range strings.Split(bar, "\n") {
			rows = append(rows, chatRow{Text: al, MsgIdx: -1})
		}
	}
	if bar := m.renderBlockedBar(); bar != "" {
		rows = append(rows, chatRow{})
		for _, bl := range strings.Split(bar, "\n") {
			rows = append(rows, chatRow{Text: bl, MsgIdx: -1})
		}
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
	if msg.FormatHint == "question" {
		trimmed := strings.TrimSpace(msg.Content)
		return strings.HasPrefix(trimmed, "[QUESTION]") || strings.HasPrefix(trimmed, "Answered:")
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
	// CA-643: part-aware question card — head, body, options and the answer
	// confirmation each get their own color instead of one flat purple block.
	if strings.HasPrefix(stripped, "[QUESTION]") {
		return styleQuestionHead.Render("[QUESTION]") + styleQuestionBody.Render(strings.TrimPrefix(stripped, "[QUESTION]"))
	}
	if strings.HasPrefix(stripped, "Answered:") {
		return styleAnswer.Render("Answered:") + styleQuestionBody.Render(strings.TrimPrefix(stripped, "Answered:"))
	}
	if i := strings.Index(stripped, ") "); i > 0 && i <= 3 {
		num := stripped[:i+2]
		rest := stripped[i+2:]
		label, desc := rest, ""
		if j := strings.Index(rest, " — "); j >= 0 {
			label, desc = rest[:j], rest[j+len(" — "):]
		}
		out := styleQuestionHead.Render(num)
		out += styleQuestionOpt.Render(label)
		if desc != "" {
			out += styleQuestionDesc.Render(" — " + desc)
		}
		return out
	}
	if strings.Contains(stripped, "[multi-select]") {
		return styleQuestionDesc.Render(stripped)
	}
	// Wrapped prompt continuation or plain question hints ("Pick an option…",
	// "Selected …") — light body, distinct from the dim system default.
	return styleQuestionBody.Render(stripped)
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

func renderQuestionBar(left, mid string, q *QuestionState, width int, highlightIdx int) string {
	hint := "← → Enter · 1-9"
	if q.MultiSelect {
		hint = "Space toggle · Enter submit"
	}
	prefixStyled := left + " " + styleGate.Render("question") + " " + mid + " "
	prefixW := lipgloss.Width(prefixStyled)
	hintW := lipgloss.Width("  " + hint)
	// BUG-333 UX follow-up (operator: long question options lost the selected
	// chip's fill — "chỉ có vài padding được apply"): the overflow path used to
	// stripANSI the whole bar and re-render it flat, erasing every style.
	// Instead, SQUEEZE each option label (ANSI-safe, with an ellipsis) so the
	// STYLED line fits the width; chips, the selected fill and the hint all
	// survive. Highlight padding widens exactly one chip per surface, which the
	// budget accounts for.
	n := len(q.Options)
	hintStyled := styleSystem.Render("  " + hint)
	fit := func(label string, budget int) string {
		if budget < 3 {
			budget = 3
		}
		if lipgloss.Width(label) <= budget {
			return label
		}
		return truncateVisual(label, budget-1) + "…"
	}
	var chips []string
	if q.MultiSelect {
		// Conservative budget: count every chip as padded (highlight worst
		// case) so the assembled line always fits; non-highlighted chips just
		// leave a couple of spare columns.
		submitW := 2 + lipgloss.Width(renderActionRingChip("[Submit]", false))
		overhead := prefixW + hintW + submitW
		if n > 1 {
			overhead += (n - 1) * 2
		}
		overhead += n * (4 + 2) // "[x] " mark + chip padding
		budget := 3
		if n > 0 {
			budget = (width - overhead) / n
		}
		chipIdx := 0
		for i, o := range q.Options {
			if i > 0 {
				chips = append(chips, "  ")
			}
			hi := highlightIdx == chipIdx
			chipIdx++
			mark := "[ ]"
			sel := ""
			for _, v := range q.Selected {
				if v == questionOptionToken(o) {
					mark = "[x]"
					break
				}
			}
			sel = mark + " "
			chips = append(chips, renderActionRingChip(sel+fit(questionOptionLabel(o), budget), hi))
		}
		chips = append(chips, "  ")
		chips = append(chips, renderActionRingChip("[Submit]", highlightIdx == chipIdx))
	} else {
		// Single select: ONE chip per option — "1) Label" — so highlightIdx
		// matches actionRingItems (one qopt item per option, BUG-346). The old
		// split "1)" + "Label" chips made Tab drift: idx 3 painted option 2's
		// label ("2) An") while Enter chose option 4.
		overhead := prefixW + hintW
		if n > 1 {
			overhead += (n - 1) * 2
		}
		overhead += n * 2 // chip padding
		budget := 3
		if n > 0 {
			budget = (width - overhead) / n
		}
		for i, o := range q.Options {
			if i > 0 {
				chips = append(chips, "  ")
			}
			num := strconv.Itoa(i+1) + ") "
			label := num + fit(questionOptionLabel(o), budget-lipgloss.Width(num))
			chips = append(chips, renderActionRingChip(label, highlightIdx == i))
		}
	}
	line := prefixStyled + strings.Join(chips, "") + hintStyled
	if width > 1 && lipgloss.Width(line) > width {
		// Pathological ultra-narrow fallback: never emit a ragged row.
		return styleGate.Render(fitStatusWidth(stripANSI(line), width))
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
	// Task-308: keep textarea width in sync with chat pane for correct wrapping.
	if m.isTextareaReady() {
		m.textarea.SetWidth(w - 2)
	}
	if m.viewingChild() {
		// Desktop parity: a focused sub-agent transcript is read-only — chat may
		// only continue on the main run. Render a locked banner instead of an
		// editable composer (CA-519).
		msg := " Child transcript is read-only — chat continues on main (/agent main or Esc) "
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
		// While a raw (non-bracketed) paste flood is active, chars arrive
		// one-by-one and would render as char-by-char. Hide the partial flood
		// and show a single placeholder until it settles to [Pasted N chars].
		if m.pasteBurst.active && m.authPhase == AuthNone {
			body = "[Pasting…]"
		} else {
			body = m.inputValue
		}
	}
	caret := " "
	if m.cursorOn {
		if m.asciiMode {
			caret = styleCursor.Render("_")
		} else {
			caret = styleCursor.Render("▌")
		}
	}
	label := strings.TrimSpace(prefix)
	// User request: top-left of the input frame shows "chat" or the flow name
	// (in flow/step mode) and, beside it, the two status values "ready" and
	// "agent:main". The sidebar now only holds session+steps.
	if m.authPhase == AuthNone && !m.viewingChild() && !m.sessionLoading {
		if label == "chat" || label == "next" {
			label = m.chatFrameTitle()
		}
	}
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
	if bar := m.renderAttentionBar(); bar != "" {
		inner = append(inner, strings.Split(bar, "\n")...)
	}
	// Phase 2 (Task-308): when caret is sticky-end with draft, render via
	// textarea.View() instead of custom bodyLines. CA-560: no height clamp.
	if m.useTextareaView() {
		m.syncTextareaValue()
		m.textarea.SetWidth(innerW)
		h := m.textarea.LineCount()
		if h < 1 {
			h = 1
		}
		m.textarea.SetHeight(h)
		view := strings.TrimSuffix(m.textarea.View(), "\n")
		viewLines := strings.Split(view, "\n")
		inner = append(inner, viewLines...)
		footer := m.inputFrameFooter()
		return frameInput(inner, w, label, footer, m.asciiMode)
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
	footer := m.inputFrameFooter()
	return frameInput(inner, w, label, footer, m.asciiMode)
}

// Per user request: bottom-right shows Model · reasoning · YOLO (no provider,
// no quota/limits). The sidebar already holds session+steps only.
func (m *AppModel) inputFrameFooter() string {
	model := strings.TrimSpace(m.model)
	if model == "" {
		model = "—"
	}
	reasoning := strings.TrimSpace(m.reasoningEffort)
	if reasoning == "" {
		reasoning = "medium"
	}
	sep := " · "
	if m.asciiMode {
		sep = " | "
	}
	out := fmt.Sprintf("%s%sreasoning: %s%s%s", model, sep, reasoning, sep, m.yoloStatusLabel())
	if chip := m.workingModeChip(); chip != "" {
		out += sep + chip
	}
	return out
}

// chatFrameTitle is the top-left of the chat input frame.
// Per user request: top-left shows "Chat: <posture>" (scan/plan/code) in chat
// mode or "Flow: <flowName>" in flow/step mode, with the value highlighted.
// The two status values "ready" and "agent: main" are beside it, with the agent
// name highlighted. Only this header and the bottom-right footer exist.
// All segments carry the composer #1e1e1e bg so a lipgloss reset inside the
// title does not punch a black hole in the solid gray frame. The watchdog
// "input stalled ..." warning is intentionally excluded here — it goes to the
// small bottom line outside the frame (renderBottomNotice).
func (m *AppModel) chatFrameTitle() string {
	sepStyled := chatBarBg(styleStatus).Render(" · ")
	if m.asciiMode {
		sepStyled = chatBarBg(styleStatus).Render(" | ")
	}
	var baseStyled string
	if m.mode == ModeFlow || m.mode == ModeStep {
		label := strings.TrimSpace(m.launch.StatusLabel())
		if label == "" {
			label = "flow"
		}
		if len([]rune(label)) > 24 {
			label = string([]rune(label)[:21]) + "…"
		}
		// "Flow:" dim, value pink (styleStatusFlow) — on bar bg
		baseStyled = chatBarBg(styleStatus).Render("Flow:") + " " + chatBarBg(styleStatusFlow).Render(label)
		if m.vibeTaskTotal > 0 && m.vibeTaskIndex > 0 {
			task := fmt.Sprintf("task %d/%d", m.vibeTaskIndex, m.vibeTaskTotal)
			baseStyled += chatBarBg(styleStatus).Render(" · ") + chatBarBg(styleStatusFlow).Render(task)
		}
	} else {
		posture := m.activePosture()
		if posture == "" {
			posture = "non"
		}
		// CA-685: "non" is the no-mode default — the chrome shows just "Chat",
		// no "Chat: Non" suffix.
		if posture == "non" {
			baseStyled = chatBarBg(styleStatus).Render("Chat:")
		} else {
			displayPosture := posture
			if displayPosture != "" {
				displayPosture = strings.ToUpper(displayPosture[:1]) + displayPosture[1:]
			}
			baseStyled = chatBarBg(styleStatus).Render("Chat:") + " " + chatBarBg(postureStyle(posture)).Render(displayPosture)
		}
	}
	var parts []string
	parts = append(parts, baseStyled)
	if m.turnIsActive() {
		parts = append(parts, chatBarBg(styleError).Render("[stop]"))
	}
	if r := strings.TrimSpace(m.statusReadyLabel()); r != "" && !isStalledStatus(r) {
		// ready / thinking spinner – use status style on bar bg
		parts = append(parts, chatBarBg(styleStatus).Render(r))
	}
	if m.authNeedLogin {
		parts = append(parts, chatBarBg(styleStatusErr).Render("SIGN-IN"))
	}
	avPlain := strings.TrimSpace(m.formatAgentViewStatus())
	if avPlain != "" {
		// Re-render agent chip on bar bg so its bg is also #1e1e1e (original
		// formatAgentViewStatus has no bg and would punch a hole).
		name := ""
		switch {
		case m.viewingChild():
			name = m.agentNameForRun(m.focusRunID)
			if name == "" {
				name = shortID(m.focusRunID)
			}
		case m.mode == ModeFlow || m.mode == ModeStep || len(m.agentRuns) > 0:
			name = "main"
		}
		if name != "" {
			parts = append(parts, chatBarBg(styleStatus).Render("agent:")+chatBarBg(styleStatusAgent).Render(name))
		} else {
			// Fallback: wrap original but force bg via re-render (should not happen)
			parts = append(parts, avPlain)
		}
	} else {
		// Always show main agent beside ready per user request, highlighted
		parts = append(parts, chatBarBg(styleStatus).Render("agent:")+chatBarBg(styleStatusAgent).Render("main"))
	}
	return strings.Join(parts, sepStyled)
}

// renderBottomNotice is the single small line outside the composer frame at the
// bottom of the chat pane. It shows transient toasts (Copied) and the watchdog
// "input stalled ..." warning so the input chrome stays clean (user request).
func (m *AppModel) renderBottomNotice(w int) string {
	if w < 1 {
		w = 80
	}
	w = safeTermWidth(w)
	var parts []string
	if t := strings.TrimSpace(m.flashToast); t != "" {
		parts = append(parts, styleStatusOK.Render(truncateVisual(t, w)))
	}
	if s := strings.TrimSpace(m.statusMsg); s != "" && isStalledStatus(s) {
		// Stalled warning is long; truncate to width and keep error color.
		if len(parts) > 0 {
			// Join with dim separator so both show on one line when overlapping.
			w2 := w - lipgloss.Width(stripANSI(strings.Join(parts, " · "))) - lipgloss.Width(" · ") - 2
			if w2 < 10 {
				w2 = 10
			}
			parts = append(parts, styleStatus.Render(" · ")+styleStatusErr.Render(truncateVisual(s, w2)))
		} else {
			parts = append(parts, styleStatusErr.Render(truncateVisual(s, w)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	raw := strings.Join(parts, "")
	if lipgloss.Width(raw) > w {
		raw = truncateVisual(raw, w)
	}
	return raw
}

// ---- Helpers ----------------------------------------------------------------

func (m *AppModel) addMessage(role, content, hint string) {
	// CP-59 Task-315 (SD-26 D-7): the handoff seed envelope renders as a
	// one-line divider, never as a raw user bubble (live stream + replay).
	// The just-committed switch's stats format the carried count when present.
	if role == "user" && strings.HasPrefix(content, client.HandoffPromptPrefix) {
		role = "system"
		if m.lastSwitchStats != nil {
			content = m.switchDividerContent()
			m.lastSwitchStats = nil
		} else {
			content = "⇄ provider switched — prior conversation carried below"
		}
	}
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

// switchDividerContent formats the just-committed switch divider from
// lastSwitchStats/lastSwitchTarget (BUG-347: shared by the synchronous
// post-switch divider and the user-envelope conversion in addMessage).
func (m *AppModel) switchDividerContent() string {
	if m.lastSwitchStats != nil {
		st := m.lastSwitchStats
		carried := fmt.Sprintf("%d turns", st.IncludedTurnCount)
		if st.Truncated && st.OmittedTurnCount > 0 {
			carried = fmt.Sprintf("%d of %d turns", st.IncludedTurnCount, st.IncludedTurnCount+st.OmittedTurnCount)
		}
		return fmt.Sprintf("⇄ switched to %s — carried %s (%s)", m.lastSwitchTarget, carried, st.HandoffMode)
	}
	return "⇄ provider switched — prior conversation carried below"
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

// gateOptionChip renders a clickable chip label for a gate decision option
// (CA-650 desktop parity: the desktop modal shows radio choices — the TUI
// surfaces the same three as clickable chips).
func gateOptionChip(opt string) string {
	switch opt {
	case "keep-test-fix-code":
		return "[Fix code]"
	case "suggest-requirement-change":
		return "[Suggest req]"
	case "custom":
		return "[Custom]"
	case "ok":
		return "[OK]"
	case "cancel":
		return "[Cancel]"
	default:
		return "[" + opt + "]"
	}
}

// optionChips maps every gate option to its clickable chip label.
func optionChips(opts []string) []string {
	chips := make([]string, 0, len(opts))
	for _, o := range opts {
		chips = append(chips, gateOptionChip(o))
	}
	return chips
}

func buildGateMessage(status, errMsg string, opts []string, regressed []string) string {
	var sb strings.Builder
	lowStatus := strings.ToLower(strings.TrimSpace(status))
	hasOpts := len(opts) > 0
	hasRegressed := len(regressed) > 0
	// Only a real block with a decision card keeps the legacy detailed card.
	// Legacy tests emit GateOptions without Status — treat empty status + opts as block for view compat.
	if hasOpts && (lowStatus == "block" || lowStatus == "") {
		if len(opts) == 2 && ((opts[0] == "ok" && opts[1] == "cancel") || (opts[0] == "cancel" && opts[1] == "ok")) {
			sb.WriteString("[GATE] Run paused.\n")
			sb.WriteString("  Continue?\n")
			sb.WriteString("  " + strings.Join(optionChips(opts), "  "))
			return sb.String()
		}
		sb.WriteString("[GATE] Flow gate blocked.\n")
		if hasRegressed {
			sb.WriteString(fmt.Sprintf("  Regressed tests: %s\n", strings.Join(regressed, ", ")))
		}
		sb.WriteString("  Options: ")
		sb.WriteString(strings.Join(opts, ", "))
		sb.WriteString("\n  " + strings.Join(optionChips(opts), "  "))
		sb.WriteString("\n  (or type number / option name)")
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
		m.openLoginModal()
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
		var supabaseConfigured bool
		ctxSupabase, cancelSupabase := context.WithTimeout(context.Background(), 5*time.Second)
		if sc, scErr := cl.GetSupabaseConfig(ctxSupabase); scErr == nil && strings.TrimSpace(sc.APIURL) != "" && strings.TrimSpace(sc.AnonKey) != "" {
			supabaseConfigured = true
		}
		cancelSupabase()

		tuiLog("cmdLoadSessionDefaults() done dur=%v accounts=%d providers=%d projects=%d catalogErr=%q accErr=%v provErr=%v supabase=%v", time.Since(start), len(accounts), len(providers), len(projects), catalogErr, accErr, provErr, supabaseConfigured)
		return SessionDefaultsMsg{
			Provider:           provider,
			Model:              model,
			AccountLabel:       label,
			Providers:          providers,
			ProviderAccounts:   accounts,
			Projects:           projects,
			Project:            project,
			Account:            account,
			CatalogErr:         catalogErr,
			SupabaseConfigured: supabaseConfigured,
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

// cmdLoadProvidersCatalog retries GET /providers after the 8s session-unlock
// budget. First load must not hold chat locked (CA-535); an empty list after
// that timeout must still backfill (CA-657).
// opencodeVariantsMsg carries the real per-model effort options fetched from
// the runner (CA-689c) — the picker must be correct at model-selection time,
// not only after a chat turn happened to capture them.
type opencodeVariantsMsg struct {
	Model   string
	Efforts []string
	Default string
	Err     string
}

func (m *AppModel) cmdFetchOpencodeVariants(modelID string) tea.Cmd {
	runnerURL := m.runnerURL
	modelID = strings.TrimSpace(modelID)
	return func() tea.Msg {
		if modelID == "" {
			return nil
		}
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		efforts, current, err := cl.GetOpencodeModelVariants(ctx, modelID)
		if err != nil {
			return opencodeVariantsMsg{Model: modelID, Err: err.Error()}
		}
		return opencodeVariantsMsg{Model: modelID, Efforts: efforts, Default: current}
	}
}

// maybeFetchOpencodeVariants returns a fetch cmd for the current opencode
// model selection. Always fires for opencode — the catalog cannot distinguish
// its guessed effort list from captured truth, and the runner short-circuits
// already-captured models in milliseconds (CA-689c).
func (m *AppModel) maybeFetchOpencodeVariants() tea.Cmd {
	if !strings.EqualFold(m.provider, "opencode") || strings.TrimSpace(m.model) == "" {
		return nil
	}
	return m.cmdFetchOpencodeVariants(m.model)
}

func (m *AppModel) cmdLoadProvidersCatalog() tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		tuiLog("cmdLoadProvidersCatalog() start")
		start := time.Now()
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		ps, err := cl.ListProviders(ctx)
		if err != nil {
			tuiLog("cmdLoadProvidersCatalog() done dur=%v err=%v n=0", time.Since(start), err)
			return ProvidersCatalogMsg{Err: err.Error()}
		}
		tuiLog("cmdLoadProvidersCatalog() done dur=%v err=<nil> n=%d", time.Since(start), len(ps))
		return ProvidersCatalogMsg{Providers: ps}
	}
}

func (m *AppModel) scheduleOpencodeCatalogRetryCmd() tea.Cmd {
	if m.providersWarmRetries >= 3 {
		return nil
	}
	if !needsOpencodeCatalogWarmRetry(m.providers) {
		return nil
	}
	return tea.Tick(12*time.Second, func(time.Time) tea.Msg {
		return ProvidersWarmRetryMsg{}
	})
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

// flowPickerRefreshInterval bounds background /flow picker refreshes (BUG-351)
// while the picker stays open — fresh enough to converge after runner mirror
// sync, quiet enough to not spam Supabase per keystroke.
const flowPickerRefreshInterval = 10 * time.Second

func (m *AppModel) cmdMaybePrefetchFlows() tea.Cmd {
	line := m.slashSuggestLine()
	ok, _ := parseFlowArgPrefix(line)
	if !ok && !strings.EqualFold(strings.TrimSpace(line), "/flow") {
		return nil
	}
	if len(m.flowBuiltins) > 0 || len(m.flowWorkflows) > 0 {
		// BUG-351: the Tab picker renders from this cache, and the first
		// fetch of a session can race runner mirror-sync (a partial list
		// would then stick for the whole session). While the picker is open,
		// refresh silently in the background so the list converges; the
		// cache keeps showing meanwhile. In-flight dedup + interval bound
		// keep per-keypress calls cheap.
		if m.flowListInflight || time.Since(m.flowListFetchedAt) < flowPickerRefreshInterval {
			return nil
		}
		m.flowListInflight = true
		m.flowListFetchedAt = time.Now()
		return m.cmdFetchFlows(true)
	}
	if m.flowListInflight {
		return nil
	}
	m.flowListInflight = true
	m.flowListFetchedAt = time.Now()
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
	workingMode := m.workingMode
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
		input.WorkingMode = workingMode
		if launch.IsBuiltin() {
			input.FlowRef = launch.FlowRef
		}
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

// continueFeedbackFromArgs joins trailing /continue text as human feedback.
// Empty (bare /continue) means a plain approve and keeps the legacy path.
func continueFeedbackFromArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if s := strings.TrimSpace(a); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func (m *AppModel) cmdContinueFlow(runID string) tea.Cmd {
	return m.cmdContinueFlowWithFeedback(runID, "continue")
}

// cmdContinueFlowWithFeedback resumes a parked flow carrying human feedback
// text (Task-325: trailing text on /continue re-enters the writer; empty
// means a plain approve). The bare path keeps the legacy "continue" body.
func (m *AppModel) cmdContinueFlowWithFeedback(runID, feedback string) tea.Cmd {
	runnerURL := m.runnerURL
	parentID := runID
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		g, err := cl.ContinueFlowWithFeedback(ctx, parentID, feedback)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return AgentGraphHydratedMsg{ParentRunID: parentID, Graph: g}
	}
}

func (m *AppModel) cmdAmendFlow(runID string, paths []string) tea.Cmd {
	runnerURL := m.runnerURL
	parentID := runID
	amendPaths := append([]string(nil), paths...)
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		g, err := cl.AmendFlow(ctx, parentID, amendPaths)
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

// cmdSubmitGateDecisionCustom posts the custom gate decision with the
// operator's own remediation text (CA-650).
func (m *AppModel) cmdSubmitGateDecisionCustom(runID, customText string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.SubmitGateDecisionCustom(context.Background(), runID, customText); err != nil {
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

// tuiMsgFilter drops hover mouse motion before Bubble Tea processes it.
// WithMouseCellMotion delivers MouseMsg motion on every pixel move; on
// Windows this shares the 64-slot conhost ReadConsoleInput queue with keys.
// Filtering here (not in Update) avoids tuiLog + View (100-170ms when the
// right sidebar is open) for every hover event.
func tuiMsgFilter(model tea.Model, msg tea.Msg) tea.Msg {
	m, ok := model.(*AppModel)
	if ok {
		if km, isKey := msg.(tea.KeyMsg); isKey {
			// BUG-328: the only KeyMsg rewrite is the ConPTY ctrl+h -> Backspace
			// remap. No debounce/dedupe lives here anymore — every key Bubble Tea
			// parses from the single VT pipe reader must reach handleKey.
			msg = remapVTControlKeys(km)
		}
	}
	if !ok {
		return msg
	}
	mm, isMouse := msg.(tea.MouseMsg)
	if !isMouse {
		return msg
	}
	switch mm.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		return msg
	}
	if mm.Action == tea.MouseActionMotion && !m.mouseDrag.down && !mm.Shift {
		m.markMotionAlive()
		return nil
	}
	return msg
}

// remapVTControlKeys maps Windows VT bytes onto the keys handleKey already
// implements. ConPTY sends Backspace as 0x08 (KeyCtrlH), while KeyBackspace
// is 0x7F (tui.log pid 2692: dozens of ctrl+h, zero backspace).
func remapVTControlKeys(msg tea.KeyMsg) tea.KeyMsg {
	if msg.Type == tea.KeyCtrlH {
		msg.Type = tea.KeyBackspace
	}
	return msg
}

// tuiProgramOpts returns Bubble Tea program options. Wheel + drag-only mouse:
// AltScreen + Filter only (no CellMotion/1003). Wheel scroll is enabled via
// ?1000h and drag motion via ?1002h in Init/maybeRearm — 1002 delivers motion
// ONLY while a button is held (live bôi-đen highlight, BUG-345), hover never
// enters the 64-slot conhost queue (BUG-328).
func tuiProgramOpts() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithAltScreen(), tea.WithFilter(tuiMsgFilter)}
}

// tuiRunProgramOpts is the live Run() option set: shared AltScreen+Filter.
// Windows input is intentionally NOT wrapped: Bubble Tea's native coninput
// reader (readConInputs over the console record queue) is the only path that
// delivered F2/F4/Esc and IME text reliably (tui.log pid 19296/14012). The
// VT-pipe wrapper (CA-663/664) depended on ConPTY's record→VT translation,
// which mangles IME commits and drops ESC-prefixed sequences afterwards
// (tui.log pid 11948: "A,\u0091" mojibake, then zero f2/f4/esc for 49s).
func tuiRunProgramOpts() []tea.ProgramOption {
	return tuiProgramOpts()
}

// ---- Run (entrypoint) -------------------------------------------------------

// applyProductionInputGuards arms the Windows-only raw-paste rejection so a
// Ctrl+V flood in Windows Terminal (which steals the chord and delivers one
// rune per key event) is rejected at the composer boundary instead of being
// inserted char-by-char and collapsing into a paste token. CA-630 removed the
// Run() assignment, so production Windows never armed the guard while every
// test still set the field directly — the suite stayed green and live sessions
// regressed (log pid 16512: "burst collapse inputLen=16" instead of
// "(windows reject)", then 0 KeyMsg for minutes because the flood filled the
// 64-slot conhost queue while View ~357ms with the F2 sidebar). goos is passed
// in so the contract is testable on any host; Run() always passes runtime.GOOS.
func applyProductionInputGuards(m *AppModel, goos string) {
	if goos == "windows" {
		m.rejectWindowsRawPaste = true
	}
	tuiLog("input guards: rejectWindowsRawPaste=%v goos=%s", m.rejectWindowsRawPaste, goos)
}

// Run starts the Bubble Tea program. In headless/print mode it runs
// the model loop and prints the final response to stdout, then exits.
func Run(cfg config.ChatConfig, runnerURL string) error {
	initTUILog()
	tuiLog("Run() start print=%v runner=%s", cfg.Print, runnerURL)
	m := New(cfg, runnerURL)
	applyProductionInputGuards(m, runtime.GOOS)
	primeConsoleBeforeProgram()
	tuiLog("tuiProgramOpts: AltScreen+Filter mouseCellMotion=off nativeConinputReader=%v (BUG-328)",
		runtime.GOOS == "windows")

	if cfg.Print {
		err := runHeadless(m, cfg.Prompt)
		tuiLog("Run() headless done err=%v", err)
		tuiLogClose()
		return err
	}

	// CA-636: bound the renderer output through a queue-backed writer so a
	// wedged console (Windows text selection blocks WriteConsole until a
	// keypress) can never hold the renderer mutex and freeze the event loop.
	// Without this, a copy-paste into the chat box froze the TUI for minutes
	// with 0 KeyMsg (tui.log 13:06:31 → 13:54:51). queuedOutput keeps
	// term.File (Fd) semantics so resize/WindowSizeMsg still work.
	out := newQueuedOutput(os.Stdout)
	p := tea.NewProgram(m, append(tuiRunProgramOpts(), tea.WithOutput(out))...)
	_, err := p.Run()
	restoreWindowsStdin()
	_, _ = os.Stdout.WriteString(decawmOn)
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
		case "user_decision_card_requested":
			// CP-62 P-3 (Task-345): headless has no interactive card — print
			// the question and options so the operator can answer on the next
			// stdin turn; non-fatal (Q-1 prose fallback).
			if card := ev.DecisionCard; card != nil {
				fmt.Fprintf(os.Stdout, "Decision needed: %s\n", card.Question)
				for i, opt := range card.Options {
					fmt.Fprintf(os.Stdout, "  %d. %s — %s\n", i+1, opt.Label, opt.Consequence)
				}
			}
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
