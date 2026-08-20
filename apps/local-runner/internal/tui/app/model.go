// Package app implements the Bubble Tea TUI for `flowpilot chat`.
// It has no dependency on flowpilot-runner/internal/runner (CP-56 boundary).
//
// Skill: cli-tui (CP-56)
package app

import (
	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// Mode represents the current display mode of the TUI.
type Mode int

const (
	ModeChat Mode = iota // normal chat
	ModeFlow             // flow/step orchestration mode
	ModeStep             // step-level focus
)

func (m Mode) String() string {
	switch m {
	case ModeFlow:
		return "flow"
	case ModeStep:
		return "step"
	default:
		return "chat"
	}
}

// ChatMessage is a single rendered message in the conversation history.
type ChatMessage struct {
	Role    string // "user" | "assistant" | "system" | "tool"
	Content string
	// FormatHint is one of: "" (plain), "tool", "approval", "question", "gate", "error"
	FormatHint string
}

// GateState holds the active gate decision UI state.
type GateState struct {
	Options        []string
	RegressedTests []string
	RunID          string
}

// ApprovalState holds the active approval UI state.
type ApprovalState struct {
	ID      string
	Details map[string]any
	RunID   string
	// Command/Cwd/Reason/Kind/Decisions are the typed BUG-246 approval details
	// surfaced by the runner; empty when the event carries no details.
	Command   string
	Cwd       string
	Reason    string
	Kind      string
	Decisions []client.ApprovalDecisionOption
}

// QuestionState holds the active question UI state.
type QuestionState struct {
	ID          string
	Prompt      string
	Options     []map[string]string
	MultiSelect bool
	// Selected holds the option values toggled so far for a multiSelect
	// question; submitted as a string array only on explicit submit.
	Selected []string
	RunID    string
}

// AuthPhase is the interactive Supabase login wizard state (Desktop LoginScreen parity).
type AuthPhase int

const (
	AuthNone AuthPhase = iota
	AuthEmail
	AuthPassword
)

// ConnStatus represents the connection status shown in the status line.
type ConnStatus int

const (
	ConnIdle ConnStatus = iota
	ConnConnecting
	ConnRunning
	ConnWaiting
	ConnError
	ConnDone
)

func (s ConnStatus) String() string {
	switch s {
	case ConnConnecting:
		return "connecting"
	case ConnRunning:
		return "running"
	case ConnWaiting:
		return "waiting"
	case ConnError:
		return "error"
	case ConnDone:
		return "done"
	default:
		return "idle"
	}
}

// Exported tea.Msg types so tests can simulate events without a live server.

// ErrMsg carries an error to the Update loop.
type ErrMsg struct{ Err error }

// chatPostureMsg carries the runner's chat-posture document after a GET/PUT so
// Update can apply the active posture's profile (provider/model/reasoning/yolo)
// to the session.
type chatPostureMsg struct {
	Cfg client.ChatPostureConfig
	Err error
}

// grokSyncFailedMsg is returned when the Grok YOLO posture sync HTTP call
// fails. The Update handler re-enables the flag so the next turn retries.
type grokSyncFailedMsg struct {
	Yolo bool
}

// RunStartedMsg carries a newly created RunHandle.
type RunStartedMsg struct{ Handle client.RunHandle }

// RunResumedMsg carries a resumed RunHandle.
type RunResumedMsg struct{ Handle client.RunHandle }

// EventMsg carries a ProviderEvent.
type EventMsg struct{ Ev client.ProviderEvent }

// TurnDoneMsg signals turn completion.
type TurnDoneMsg struct{ FinalMsg string }

// TurnFailedMsg signals a turn failure (used for headless non-zero exit).
type TurnFailedMsg struct{ Reason string }

// ConnectedMsg signals runner connection ready.
type ConnectedMsg struct{ RunnerURL string }

// sessionKeysUnlockMsg is deprecated since CA-514 fourth pass: the FlowPilot
// banner stays up until SessionDefaultsMsg decides the catalog (typing is never
// hard-locked; only send is blocked). Kept as a defensive no-op in Update.
type sessionKeysUnlockMsg struct{}

// sessionLoadTimeoutMsg fires only if SessionDefaultsMsg never arrived.
type sessionLoadTimeoutMsg struct{}

// ProjectsCatalogMsg is a late/retry project list after the fast session path.
type ProjectsCatalogMsg struct {
	Projects []client.Project
	Project  *client.Project
	Err      string
}

// SessionDefaultsMsg carries active provider/model discovered after connect.
type SessionDefaultsMsg struct {
	Provider         string
	Model            string
	AccountLabel     string
	Providers        []client.Provider
	ProviderAccounts []client.ProviderAccountSummary
	Projects         []client.Project
	Project          *client.Project
	Account          *client.ProviderAccountSummary
	CatalogErr       string
}

// FlowListMsg carries /flow list results.
type FlowListMsg struct {
	Builtins   []client.BuiltinFlowOption
	Workflows  []client.Workflow
	CatalogErr string // soft failure for Supabase workflow catalog
	Silent     bool   // cache only — no chat dump (prefetch while typing /flow)
}

// cursorTickMsg drives the blinking input caret.
type cursorTickMsg struct{}

// thinkingTickMsg advances the animated "Thinking" placeholder (spinner /
// shimmer / elapsed). It self-cancels when the thinking row disappears.
type thinkingTickMsg struct{}

// LoginResultMsg carries a completed Supabase password login.
type LoginResultMsg struct {
	Email        string
	UserID       string
	PersistedTo  string // Desktop session file path (optional)
	PersistError string // soft failure while writing Desktop session
}

// QuitMsg requests app exit.
type QuitMsg struct{}

// ApprovalResolvedMsg is a successful POST /client/approvals/{id}/decision.
type ApprovalResolvedMsg struct {
	ID       string
	Decision string
}

// QuestionResolvedMsg is a successful POST /client/questions/{id}/answer.
type QuestionResolvedMsg struct {
	ID     string
	Choice string
}

// StoppedMsg is a successful POST .../interrupt.
type StoppedMsg struct{}

// CopiedMsg reports a clipboard write from [copy] / /copy.
type CopiedMsg struct {
	Kind string
	Err  string
}

// toastClearMsg dismisses a transient flash toast when its generation still matches.
type toastClearMsg struct {
	ID int64
}

// TokenUsageMsg carries updated token usage for the statusline.
type TokenUsageMsg struct{ Usage *client.TokenUsageSnapshot }

// AgentGraphMsg carries an updated agent graph snapshot.
type AgentGraphMsg struct{ Graph *client.AgentGraphSnapshot }

// AttentionLoadedMsg carries the result of a dispatch-attention refresh.
type AttentionLoadedMsg struct {
	RunID string
	Items []client.DispatchAttentionItem
	Err   error
}

// AttentionInspectedMsg carries a single turn inspect result.
type AttentionInspectedMsg struct {
	RunID  string
	TurnID string
	Result *client.DispatchInspectResult
	Err    error
}

// AttentionResolvedMsg reports a successful operator resolution so the TUI can
// drop the resolved item (and refresh) rather than require a separate read.
type AttentionResolvedMsg struct {
	RunID   string
	TurnID  string
	Kind    string // uncertain | repair_required
	Outcome string
	Err     error
}

// AppModel is the Bubble Tea model for the chat TUI.
type AppModel struct {
	cfg        config.ChatConfig
	runnerURL  string
	client     *client.Client
	mode       Mode
	connStatus ConnStatus
	statusMsg  string
	// flashToast is a short-lived status overlay (e.g. "Copied answer.") — not chat.
	flashToast   string
	flashToastID int64
	messages     []ChatMessage
	// visiblePromptCount is how many newest user-prompt groups to render (Task-290).
	visiblePromptCount        int
	historyLoadedAfterSeq     int64 // events with seq <= this are not in memory; 0 = loaded from start
	mainHistoryLoadedAfterSeq int64 // preserved while viewing child transcript
	historyChunkInFlight      bool

	inputValue  string
	inputCursor int // rune index; <0 means caret sticks to the end
	// pasteSegments holds the full text behind collapsed "[Pasted N lines]" tokens.
	// pasteBurst guards a raw (non-bracketed) paste arriving as a flood of key
	// events: while a rune burst is active, Enter inserts a newline instead of
	// submitting, and the settled region collapses to a paste token.
	pasteSegments []pasteSegment
	pasteBurst    pasteBurst
	// promptHistory is the sent-prompts ring for Up/Down recall (bash-style).
	// promptHistIdx points into it while browsing; -1 means "show live draft".
	promptHistory []string
	promptHistIdx int
	promptDraft   string
	viewport      viewportState
	mouseSel      mouseSelect
	mouseDrag     mouseDrag
	rowCache      []chatRow
	rowCacheSig   uint64
	runHandle     *client.RunHandle
	// expandedToolGroups tracks which multi-tool-call runs (CA-525) are expanded.
	// Keyed by the joined tool names of the run (content-derived, stable across
	// thinking-placeholder reordering that shifts message indices).
	expandedToolGroups map[string]bool
	// expandedUserPrompts tracks which user prompt bubbles (4-line clamp) are
	// expanded. Keyed by message content (content-derived like expandedToolGroups).
	expandedUserPrompts map[string]bool

	// Per-turn settings
	yolo                bool
	agentsFocus         bool
	// chatPosture is the active Scan/Plan/Code posture ("" = code). Scan/Plan are
	// read-only: the runner auto-approves reads and auto-denies writes without
	// asking. Set via /mode or the Tab cycle; resend on every chat turn.
	chatPosture         string
	chatPostureCfg      client.ChatPostureConfig // cached runner document (/mode-setup reads it)
	chatPostureDirty    bool                     // local profile edit pending a PUT
	// chatPostureSaving tracks a /mode-setup edit that is awaiting PUT completion
	// so the "saving…" banner can be replaced with "saved" instead of hanging.
	chatPostureSaving        bool
	chatPostureSavingPosture string
	// modeSetupDraft holds staged wizard edits for /mode-setup (multiple fields
	// and postures) before a single Enter save. Nil when wizard not open.
	modeSetupDraft      *client.ChatPostureConfig
	modeSetupDraftDirty bool
	// modeSetupModal is the 3-tab overlay that replaces the wizard's tmp steps.
	modeSetupModalOpen      bool
	modeSetupModalTab       string
	modeSetupModalDraft     *client.ChatPostureConfig
	modeSetupModalFocus     int
	modeSetupModalPickerOpen bool
	modeSetupModalPickerKind string
	modeSetupModalPickerIdx  int
	// chatPosturePending remembers what to do after the runner config loads:
	// "" = nothing; "apply:<posture>" = apply that posture's profile; "show" =
	// just display the config; "setup:<posture>:<field>:<value>" = apply a
	// profile edit and save; "modal:<tab>" = open modal.
	chatPosturePending string
	// postureGrokSync / postureGrokSyncSet mirror a Grok YOLO posture sync
	// requested by a scan/plan profile pin (applied via cmdGrokYoloPosture).
	postureGrokSync    bool
	postureGrokSyncSet bool
	selectedSkills     []client.SkillSelection
	skillsCatalog       []client.ProviderSkill // Desktop ChatInput skills list
	workspaceFiles      []string               // last @file picker fetch (nil = not loaded)
	workspaceFilesQuery string                 // query that produced workspaceFiles
	pendingAttach       []client.PromptAttachment
	pendingLocalPaths   map[string]string // attachment ID → materialized temp path
	attachPanelOpen     bool              // modal list of pending images (Desktop chips)
	launch              LaunchArm
	firstTurnPending    bool // consume builtin FirstTurnExtras once
	// pendingFlowRestore holds mode/flow from disk until project catalog binds.
	// Restoring ModeFlow on cold start (before project_id) left the TUI unusable.
	pendingFlowRestore *prefs.Session
	reasoningEffort    string
	flowBuiltins       []client.BuiltinFlowOption
	flowWorkflows      []client.Workflow
	chatList           []client.RunHistoryItem // last /history result for picker + /open <n>
	// remoteChatList caches the project's Drive-backed chat index (G3 /restore).
	// G2 /sync reconciles against it to skip runs already present on Drive.
	remoteChatList []client.RemoteChatSessionSummary
	// driveSync tracks an in-flight /sync batch; nil when idle.
	driveSync *driveSyncState
	// restoreBatch tracks an in-flight /restore batch; nil when idle.
	restoreBatch    *restoreState
	sessionPanel    sessionInfoPanel // collapsible top-right session/status overlay
	flowSteps       []client.WorkflowStepRuntime
	flowStepsActive string // node name currently RUNNING
	// flowLoopStatus mirrors the orchestrator LoopState.Status from the latest
	// agent_graph_updated (done | running | blocked | stopped | …). Flow hubs keep
	// the raw handle status "running" past loop "done" until the last SSE settles,
	// so the TUI settles chrome on loop+step+agent state, not the stale handle.
	flowLoopStatus string
	// flowBlockReason is LoopState.BlockReason when flowLoopStatus=="blocked"
	// (BUG-231): "cap" | "escalate" | "member_stalled". Surfaced in the banner so
	// the user knows the flow is parked awaiting their decision, not live-running.
	flowBlockReason string
	// Dispatch operator attention (CP-51 Task-256): uncertain turns / open
	// repairs that need an operator decision. Desktop DispatchAttentionCard
	// parity — surfaced as clickable chips above the composer. Automated
	// dispatch stays blocked until every item is resolved.
	attention             []client.DispatchAttentionItem
	attentionInspect      map[string]*client.DispatchInspectResult // key runID/turnID
	attentionInFlight     bool
	attentionErr          string
	attentionRetryConfirm map[string]bool // retry-as-new cancel-bias double-confirm
	turnStream            *turnStreamState
	orchStream            *orchStreamState // Desktop orchestration SSE after turn
	focusStream           *orchStreamState // child transcript while /agent focused
	turnSendPending       bool             // user turn POSTed, stream not opened yet (run-107774)
	focusRunID            string           // empty = main run viewport
	mainTranscript        []ChatMessage    // cached while viewing a child
	lastEventSeq          int64
	stepsPollTicks        int // cursor ticks while flow is live
	// In-flight + failure guards so dead runner cannot pile up HTTP cmds / lock UX.
	stepsPollInFlight     bool
	agentsHydrateInFlight bool
	// agentHydrateRetries counts consecutive hydrate attempts that returned no
	// child run while steps still need an [open] chip (CA-528). Capped so a slow
	// or dead runner is not flooded.
	agentHydrateRetries  int
	runnerPollFailStreak int    // consecutive steps/agent poll dial/timeout failures
	lastTurnError        string // last turn_failed error (fallback FAIL reason in chat)

	// Pending gate/approval/question state. A turn can fan out several approval
	// or question cards in parallel (BUG-157/158); the TUI used to keep a single
	// pointer and silently dropped every card but the last, leaving the dropped
	// card unresolved and the run hung. `approval`/`question` are the HEAD (first
	// unresolved) cards — kept as pointers so every legacy check works unchanged —
	// while `approvals`/`questions` carry the full queue. All mutations go
	// through the helpers in chat_pending_queue.go.
	gate      *GateState
	approval  *ApprovalState
	question  *QuestionState
	approvals []ApprovalState
	questions []QuestionState

	// Navigation
	project          *client.Project
	projects         []client.Project
	projectPath      string // resolved target path for statusline
	projectBranch    string // git branch at projectPath
	provider         string
	model            string
	accountLabel     string // from provider-accounts display_label
	account          *client.ProviderAccountSummary
	providers        []client.Provider
	providerAccounts []client.ProviderAccountSummary // for ChatInput-parity readiness
	modelContextWin  int64
	lastTokens       *client.TokenUsageSnapshot
	agentRuns        []client.AgentRunSummary
	focusedAgentIdx  int
	stepID           string // synthetic chat step from StartRun / Resume
	pendingPrompt    string // first prompt waiting for StartRun to finish

	// Supabase auth (Desktop LoginScreen parity via POST /supabase-auth/login)
	authPhase     AuthPhase
	authEmail     string
	authNeedLogin bool
	signedInEmail string

	// Input focus / slash suggestion selection
	cursorOn               bool
	suggIdx                int
	statusSkillsExpanded   bool // F3: expand attached skill names under the status chip
	statusDetailsCollapsed bool // F4 / click line 0: hide mode–project rows; status row stays

	// sessionLoading locks chat while provider/project catalogs load after connect.
	sessionLoading bool
	// sessionDefaultsLoaded is set after the first SessionDefaultsMsg (real chat gate).
	sessionDefaultsLoaded bool
	loadingFrame          int

	// thinkingFrame drives the animated "Thinking" placeholder (spinner /
	// shimmer / elapsed). Advanced by thinkingTickMsg while a thinking row is
	// live; the row-cache signature hashes it so the animation re-renders.
	thinkingFrame int
	// thinkingTickerActive tracks whether the 90ms thinking tick is scheduled,
	// so the always-on cursor tick only (re)starts it once per thinking phase.
	thinkingTickerActive bool

	// driveSyncFrame drives the animated Drive sync/restore spinner shown in the
	// right sidebar / session panel while a batch is in flight (CA-551).
	driveSyncFrame int
	// driveSyncTickerActive tracks whether the 90ms drive tick is scheduled.
	driveSyncTickerActive bool

	// Terminal dimensions
	width  int
	height int

	// fullWidth is the real terminal width, preserved even while View() temporarily
	// narrows width to the chat column (right sidebar, CA-524). Sidebar geometry
	// reads this so it is stable regardless of the render-time width mutation.
	fullWidth int

	// ASCII mode for legacy Windows consoles
	asciiMode bool

	// quitting indicates the TUI is exiting
	quitting bool
	// headless collects the final message when --print is set
	headlessOutput string
	err            error
}

// suggestItem is one row in the live slash / flow / history / model picker.
type suggestItem struct {
	value  string // command name, flowRef/workflow id, chat run id, model, or effort
	detail string
	kind   string // "cmd" | "flow" | "history" | "model" | "reasoning" | "provider" | "provider-connect" | "skill"
	slash  string // for history: "/history" | "/open" | "/resume"
}

// viewportState tracks scrolling state.
type viewportState struct {
	offset int // lines scrolled from bottom
}

// mouseSelect is drag transcript highlight (Shift optional), cell-accurate
// from (x0,y0) to (x1,y1). A click with no motion still clears it so
// Approve/copy chips stay distinct from select.
type mouseSelect struct {
	armed  bool
	x0, y0 int
	x1, y1 int
}

// mouseDrag tracks a button-down gesture so motion can start a selection
// without treating the initial press as an armed highlight (legacy click tests).
type mouseDrag struct {
	down   bool
	moved  bool
	x0, y0 int
}

func (s mouseSelect) empty() bool {
	return !s.armed
}

func (s mouseSelect) contains(x, y int) bool {
	if s.empty() {
		return false
	}
	y0, y1 := s.y0, s.y1
	x0, x1 := s.x0, s.x1
	if y0 > y1 {
		y0, y1 = y1, y0
		x0, x1 = x1, x0
	}
	if y < y0 || y > y1 {
		return false
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 == y1 {
		return x >= x0 && x <= x1
	}
	if y == y0 {
		return x >= x0
	}
	if y == y1 {
		return x <= x1
	}
	return true
}

// slashCommand represents a slash command the user can invoke.
type slashCommand struct {
	name        string
	description string
}

var knownSlashCommands = []slashCommand{
	{"/help", "Show available commands"},
	{"/clear", "Clear conversation history"},
	{"/exit", "Exit the TUI"},
	{"/quit", "Exit the TUI"},
	{"/scan", "Switch to scan posture — read-only (like /mode scan)"},
	{"/yolo", "Toggle YOLO in chat mode (flow mode is auto-on)"},
	{"/mode", "Switch chat posture — /mode scan|plan|code"},
	{"/mode-setup", "Configure posture profiles (provider/model/reasoning/yolo)"},
	{"/agents", "List/cycle sub-agents (Tab while focused)"},
	{"/agent", "View a sub-agent transcript — /agent main|<name>"},
	{"/stop", "Stop the in-flight turn (flow: main + all children)"},
	{"/continue", "Unblock a parked (blocked) flow loop — /continue"},
	{"/flow", "Start or list flows"},
	{"/chat", "Switch to chat mode"},
	{"/skill", "Skills — Tab multi-pick [name]+chip · Enter closes picker · F3"},
	{"/image", "Images — Tab open/paste; pick index · [N img] [open]/[x]"},
	{"/provider", "Switch / connect / install — /provider  then ↑↓ Tab Enter"},
	{"/model", "Switch model — type /model  then ↑↓ Tab Enter"},
	{"/reasoning", "Set effort — type /reasoning  then ↑↓ Tab Enter"},
	{"/new", "Start a new conversation"},
	{"/step", "Select workflow step"},
	{"/resume", "Open chat — type /resume  then ↑↓ Tab Enter"},
	{"/history", "List/open chats — type /history  then ↑↓ Tab Enter"},
	{"/open", "Open chat — type /open  then ↑↓ Tab Enter"},
	{"/approve", "Approve a pending approval"},
	{"/deny", "Deny a pending approval"},
	{"/headless", "Print next response to stdout only"},
	{"/status", "Show current connection status"},
	{"/info", "Toggle session info panel (top-right; also F2)"},
	{"/login", "Sign in to Supabase (email/password) — Desktop session parity"},
	{"/settings", "Open Desktop app for Settings (start if not running)"},
	{"/sync", "Push session to Drive — /sync · /sync all"},
	{"/restore", "Pull a Drive-backed chat — /restore · /restore all"},
	{"/init", "Init — /init skill (flow-pack) · /init all (full engine)"},
}
