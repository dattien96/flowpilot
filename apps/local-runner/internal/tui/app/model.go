// Package app implements the Bubble Tea TUI for `flowpilot chat`.
// It has no dependency on flowpilot-runner/internal/runner (CP-56 boundary).
//
// Skill: cli-tui (CP-56)
package app

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

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
	// Attachments holds the original file names of images attached to a user turn.
	// Only set on Role=="user" messages that were sent with images. The chip is
	// rendered outside the 4-line prompt clamp so it is always visible (BUG-337).
	Attachments []string
}

// GateState holds the active gate decision UI state.
type GateState struct {
	Options        []string
	RegressedTests []string
	RunID          string
	ResumeFrom     string
	// AwaitingCustom is set after the operator clicked the [Custom] chip
	// (CA-650): the next Enter submits the typed input as the custom gate
	// decision. The runner rejects option=custom without customText.
	AwaitingCustom bool
}

// DecisionCardState holds the armed CP-62 P-3 (Task-345) structured
// escalation card (request_user_decision). Matching input (number, option id,
// or label) submits that option id as parked-run feedback; any other text is
// sent verbatim as the Q-1 prose fallback.
type DecisionCardState struct {
	RunID       string
	Question    string
	Options     []client.DecisionCardOption
	Recommended string
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

// chatLoadTimeoutMsg fires when the cold-start chat list never settles
// within budget: init-loading passes degraded (loud) instead of holding
// the banner forever.
type chatLoadTimeoutMsg struct{}

// ProjectsCatalogMsg is a late/retry project list after the fast session path.
type ProjectsCatalogMsg struct {
	Projects []client.Project
	Project  *client.Project
	Err      string
}

// ProvidersCatalogMsg is a late/retry provider list after the 8s session
// unlock (CA-535). Session load must not wait on a slow CLI probe, but an
// empty catalog after timeout is sticky unless we backfill (CA-657).
type ProvidersCatalogMsg struct {
	Providers []client.Provider
	Err       string
}

// ProvidersWarmRetryMsg triggers a background GET /providers after OpenCode
// model warm has had time to finish (undersized catalog retry).
type ProvidersWarmRetryMsg struct{}

// SessionDefaultsMsg carries active provider/model discovered after connect.
type SessionDefaultsMsg struct {
	Provider           string
	Model              string
	AccountLabel       string
	Providers          []client.Provider
	ProviderAccounts   []client.ProviderAccountSummary
	Projects           []client.Project
	Project            *client.Project
	Account            *client.ProviderAccountSummary
	CatalogErr         string
	SupabaseConfigured bool
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

// ProjectCreatedMsg is returned after successfully creating and binding a project.
type ProjectCreatedMsg struct {
	Project client.Project
}

// SupabaseConfigSavedMsg is returned after saving Supabase workspace config.
type SupabaseConfigSavedMsg struct{}

// LSPStatusMsg carries the runner's LSP server presence for the bound
// workspace (nil = unknown; fetch failed or not attempted yet). Path echoes
// the requested workspace so stale responses for a previous project are
// dropped instead of overwriting fresh state.
type LSPStatusMsg struct {
	Path   string
	Status *client.LSPStatus
}

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
	// CP-81 (Task-417): runner lifecycle participation — this TUI's lease,
	// the last-seen phase/inventory for the status chip, reconnect tracking
	// for planned restarts, and the three-choice close dialog.
	lease            tuiLease
	lifecyclePhase   string
	lifecycleClients int
	lifecycleWork    int
	updatePending    bool
	reconnect        *reconnectState
	closeDialog      *closeDialogState
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
	// textarea is the bubbles/textarea composer (Task-308). inputValue/inputCursor
	// are kept as live composer (old tests assign them). textarea is a
	// value-only mirror for future View() migration — no caret poke.
	textarea      textarea.Model
	textareaReady bool
	// pasteSegments holds the full text behind collapsed "[Pasted N lines]" tokens.
	// pasteBurst guards a raw (non-bracketed) paste arriving as a flood of key
	// events: while a rune burst is active, Enter inserts a newline instead of
	// submitting, and the settled region collapses to a paste token.
	pasteSegments []pasteSegment
	pasteBurst    pasteBurst
	// pasteCtrlVHintShown ensures the Windows Ctrl+V hint toast fires once per
	// session after the first raw paste flood (WT steals Ctrl+V).
	pasteCtrlVHintShown bool
	// rejectWindowsRawPaste blocks raw (non-bracketed) paste floods on Windows
	// (WT steals Ctrl+V). Set only in Run() so tests keep the burst green.
	rejectWindowsRawPaste bool
	// pasteHijacked marks that the current raw flood has been hijacked to
	// clipboard (1 msg). While true, subsequent flood runes must be swallowed
	// without reverting the just-inserted [Pasted] token (log 18700).
	pasteHijacked bool
	// promptHistory is the sent-prompts ring for Up/Down recall (bash-style).
	// promptHistIdx points into it while browsing; -1 means "show live draft".
	promptHistory      []string
	promptHistIdx      int
	promptDraft        string
	lastHistoryKeyAt   time.Time
	lastHistoryKeyType tea.KeyType
	lastHistoryWasNav  bool
	prevHistoryKeyAt   time.Time
	// lastInputAt is the timestamp of the last KeyMsg/MouseMsg that reached
	// Update; inputStallLogged marks a fired input-watchdog stall banner
	// (input_watchdog.go, CA-645) so it logs once per stall episode.
	lastInputAt      time.Time
	inputStallLogged bool
	inputHadRealKey  bool // true after the first real KeyMsg/MouseMsg (BUG-328 watchdog)
	// actionRingIdx highlights one chip in the must-answer action ring (BUG-328).
	actionRingIdx     int
	actionRingFocus   bool
	actionRingCardSig string
	f2StepPickIdx     int
	attachPanelSel    int // 0-based row while attach panel is open
	// lastMotionAt is stamped by hover-motion events at the tuiMsgFilter level
	// (they never reach Update). It proves the console input pipe is still
	// delivering events during an input stall, separating "keys dropped
	// upstream" from "console fully dead" in the watchdog fingerprint.
	lastMotionAt time.Time
	// lastConsoleRearmAt throttles Windows QuickEdit/mouse-off re-arm (BUG-328).
	lastConsoleRearmAt time.Time
	// inputExpectedSince is set when session defaults finish loading; the
	// watchdog uses it to detect startup wedges with zero KeyMsg (BUG-328).
	inputExpectedSince time.Time
	// ss3 holds a bare 'O' rune while Windows ConPTY delivers an SS3 function
	// key as 'O'+suffix rune records (BUG-328, tui.log pid 18400).
	ss3      ss3FKeyState
	viewport viewportState
	// wheel coalesce: burst wheel events share one View() to avoid BUG-328 hang
	// after ~20s of continuous scroll (pid 20632: wheel-only 1000h still wedged
	// when every notch painted the full markdown transcript).
	lastWheelAt       time.Time
	pendingWheelDelta int
	lastWheelDelta    int
	mouseSel          mouseSelect
	mouseDrag         mouseDrag
	rowCache          []chatRow
	rowCacheSig       uint64
	runHandle         *client.RunHandle
	// expandedToolGroups tracks which multi-tool-call runs (CA-525) are expanded.
	// Keyed by the joined tool names of the run (content-derived, stable across
	// thinking-placeholder reordering that shifts message indices).
	expandedToolGroups map[string]bool
	// expandedUserPrompts tracks which user prompt bubbles (4-line clamp) are
	// expanded (CA-559/CA-607). Content-keyed so expansion survives message
	// index shifts.
	expandedUserPrompts map[string]bool

	// Per-turn settings
	yolo        bool
	workingMode string // Task-326: ""=unfiltered cache, "dev"|"vibe" after /vibe
	// CP-71: arm the next run for worktree isolation; liveWorktree mirrors the
	// active run's binding state for the status badge.
	worktree     bool
	liveWorktree string // "" | active | merge_pending | lost | merged | ...
	worktreeSlug string
	agentsFocus  bool
	// chatPosture is the active Scan/Plan/Code posture ("" = code). Scan/Plan are
	// read-only: the runner auto-approves reads and auto-denies writes without
	// asking. Set via /mode or the Tab cycle; resend on every chat turn.
	chatPosture      string
	chatPostureCfg   client.ChatPostureConfig // cached runner document (/mode-setup reads it)
	chatPostureDirty bool                     // local profile edit pending a PUT
	// chatPostureSaving tracks a /mode-setup edit that is awaiting PUT completion
	// so the "saving…" banner can be replaced with "saved" instead of hanging.
	chatPostureSaving        bool
	chatPostureSavingPosture string
	// modeSetupDraft holds staged wizard edits for /mode-setup (multiple fields
	// and postures) before a single Enter save. Nil when wizard not open.
	modeSetupDraft      *client.ChatPostureConfig
	modeSetupDraftDirty bool
	// modeSetupModal is the 3-tab overlay that replaces the wizard's tmp steps.
	modeSetupModalOpen       bool
	modeSetupModalTab        string
	modeSetupModalDraft      *client.ChatPostureConfig
	modeSetupModalFocus      int
	modeSetupModalPickerOpen bool
	modeSetupModalPickerKind string
	modeSetupModalPickerIdx  int
	// modeSetupModalPickerFilter is the type-to-filter query for the modal's
	// model picker (same UX as the /model input picker): typing narrows the
	// list, Backspace deletes, Esc closes. Empty = unfiltered.
	modeSetupModalPickerFilter string
	// Project onboarding wizard (standalone TUI project creation & binding).
	projectWizardOpen     bool
	projectWizardField    int // 0: name, 1: platform, 2: model, 3: submit
	projectWizardDir      string
	projectWizardName     string
	projectWizardPlatform string
	projectWizardModel    string
	projectWizardBusy     bool
	projectWizardErr      string
	// In-TUI interactive login modal.
	loginModalOpen     bool
	loginModalField    int // 0: email, 1: password, 2: submit
	loginModalEmail    string
	loginModalPassword string
	loginModalBusy     bool
	loginModalErr      string
	// In-TUI interactive Supabase setup wizard.
	supabaseSetupModalOpen       bool
	supabaseSetupModalField      int // 0: url, 1: anonKey, 2: serviceRoleKey, 3: submit
	supabaseSetupModalURL        string
	supabaseSetupModalAnonKey    string
	supabaseSetupModalServiceKey string
	supabaseSetupModalBusy       bool
	supabaseSetupModalErr        string
	// supabaseJustConfigured arms the onboarding chain for the next
	// SessionDefaultsMsg after a first-run Supabase setup save (F-2): the
	// follow-up session load is not firstLoad, but the user still needs login.
	supabaseJustConfigured bool
	// lspStatus is the runner's LSP server presence for the bound workspace
	// (nil = unknown). Rendered as a compact sidebar warning when the
	// project platform's server binary is missing.
	lspStatus *client.LSPStatus
	// lspStatusPath is the workspace path lspStatus was fetched for; refetch
	// when the bound project changes.
	lspStatusPath string
	// chatPosturePending remembers what to do after the runner config loads:
	// "" = nothing; "apply:<posture>" = apply that posture's profile; "show" =
	// just display the config; "setup:<posture>:<field>:<value>" = apply a
	// profile edit and save; "modal:<tab>" = open modal.
	chatPosturePending string
	// postureGrokSync / postureGrokSyncSet mirror a Grok YOLO posture sync
	// requested by a scan/plan profile pin (applied via cmdGrokYoloPosture).
	postureGrokSync     bool
	postureGrokSyncSet  bool
	selectedSkills      []client.SkillSelection
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
	// flowListInflight dedups background /flow picker refreshes (BUG-351);
	// flowListFetchedAt bounds them while the picker stays open.
	flowListInflight  bool
	flowListFetchedAt time.Time
	chatList          []client.RunHistoryItem // last /history result for picker + /open <n>
	// chatListInflight dedups background history-picker refreshes (BUG-355 F1);
	// chatListFetchedAt bounds them while the picker stays open.
	chatListInflight  bool
	chatListFetchedAt time.Time
	// deleteSelected tracks ticked rows in the /delete picker (skill-like multi-select).
	deleteSelected map[string]bool
	// deletePending* arms a two-step delete confirm for /delete (Task-318).
	// Single and batch deletes share the same y/n gate — pending may be 1..N ids.
	deletePendingRunID string
	deletePendingIDs   []string
	deletePendingLabel string
	// deleteBatch* tracks sequential batch deletes after y confirm (Enter on picker).
	deleteBatchQueue []string
	deleteBatchTotal int
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
	// flowGateReason is LoopState.GateReason — the human explanation behind a
	// parked Continue/Stop decision (escalate/cap/delegate_failed). Surfaced on
	// the blocked bar so a chip is never shown without its reason (run-142155).
	flowGateReason string
	// flowLoopRound/flowLoopCap mirror LoopState.Round + Cap (fallback RoundCap)
	// from the latest agent-graph snapshot (Task-322): rendered as the
	// "round R/C" chip on the steps header. Zero cap = unknown → chip hidden.
	flowLoopRound int
	flowLoopCap   int
	// vibeTaskIndex/Total/Name mirror LoopState vibe progress (BUG-367).
	vibeTaskIndex int
	vibeTaskTotal int
	vibeTaskName  string
	// flowStepsProvider/flowStepsModel carry the run-level provider/model posture
	// from the latest steps-runtime snapshot (Task-322): per-step fallback when
	// a step row has no own provider/model (built-in nodes inherit run posture).
	flowStepsProvider string
	flowStepsModel    string
	// CA-633: composeCellBuf cache. composeCellBuf(chat,side,...) is a pure
	// function of its inputs, so when the chat/side pane strings and geometry
	// are byte-identical to the last call the ~300ms cellbuf merge (126×50,
	// Windows conhost) is skipped. Together with the idle caret pinning this
	// keeps idle frames off the heavy path — rebuilding the merged buffer
	// every 530ms cursor tick filled the 64-slot input queue and keys never
	// reached Update (logs 16512/24144: 0 KeyMsg after session ready).
	lastComposeChat  string
	lastComposeSide  string
	lastComposeFullW int
	lastComposeChatW int
	lastComposeSideW int
	lastComposeH     int
	composeOut       string
	composeBuilds    int // instrumentation: how many times composeCellBuf ran
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
	gate     *GateState
	approval *ApprovalState
	question *QuestionState
	// decisionCard is the CP-62 P-3 (Task-345) structured escalation card
	// (request_user_decision): answered by sending the chosen option id as
	// parked-run feedback; any other text is the Q-1 prose fallback.
	decisionCard *DecisionCardState
	approvals    []ApprovalState
	questions    []QuestionState

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

	// Chat switch surface (CP-59 Task-315): one in-flight switch at a time;
	// a posture picked mid-switch queues for the new leg.
	chatSwitchInFlight      bool
	chatSwitchQueuedPosture string
	// chatDetached marks a restored chat with no locally-active leg (SD26 §10):
	// the next prompt reattaches via startRun carrying the chat identity.
	chatDetached bool
	// chatBackfillDone guards /open restore-by-chat rendering (idempotent per
	// opened chat).
	chatBackfillDone bool
	// lastSwitchStats/lastSwitchTarget carry the just-committed switch's handoff
	// stats so the seed-envelope collapse renders the carried-count divider.
	lastSwitchStats  *client.ChatSwitchHandoffStats
	lastSwitchTarget string
	// seedTurnActive (BUG-347): while the post-switch seed turn is streaming,
	// its envelope reply is noise (an orphan assistant bubble with no You-box)
	// — the divider renders synchronously on switch commit and all seed
	// assistant output is dropped until the seed turn completes.
	seedTurnActive bool
	// turnLive tracks a user turn from local send until turn_completed/failed,
	// surviving stream switches (turnStream → orchStream). It prevents premature
	// "done" and C2 Tab races where TUI thought idle but runner was still
	// busy (BUG-341/B-4: 417944 first turn blank, Tab 409 + in-place leak).
	turnLive bool

	// Supabase auth (Desktop LoginScreen parity via POST /supabase-auth/login)
	authPhase     AuthPhase
	authEmail     string
	authNeedLogin bool
	signedInEmail string

	// Input focus / slash suggestion selection
	cursorOn bool
	suggIdx  int

	// sessionLoading locks chat while provider/project catalogs load after connect.
	sessionLoading bool
	// sessionDefaultsLoaded is set after the first SessionDefaultsMsg (real chat gate).
	sessionDefaultsLoaded bool
	// chatWaitPending holds init-loading until the first chat list settles
	// (cold start with a bound project): the history picker renders from
	// m.chatList, so passing ready earlier leaves /open stuck on
	// "loading chats…" when the silent first fetch fails.
	chatWaitPending bool
	// providersWarmRetries counts background /providers refetches while OpenCode
	// model cache is still warming (undersized catalog).
	providersWarmRetries int
	loadingFrame         int
	scaffoldBusy         bool
	// CA-916: live scaffold progress feed — seq cursor for /scaffold/progress
	// polling, in-flight fetch guard, plus the latest phase label shown next to
	// the busy spinner.
	scaffoldProgressSeq      int64
	scaffoldProgressInFlight bool
	scaffoldPhase            string

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

	// CA-621: throttle View slow log so chat history open does not spam tui.log
	lastViewSlowLog time.Time

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
	// expandKey arms a press that started on an already-EXPANDED user prompt
	// box (BUG-359): the toggle must not fire on press-down (that would
	// collapse the box before a drag-select even starts). The toggle fires
	// on release only when released on the same box (= real click); a
	// press→release over different cells is a drag-select that copies and
	// never toggles. Empty for ordinary presses.
	expandKey string
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
	{"/vibe", "Switch working mode — /vibe [on|off] or /vibe <requirement>"},
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
	{"/delete", "Delete chats — type /delete  then Tab tick · Enter del · all"},
	{"/approve", "Approve a pending approval"},
	{"/deny", "Deny a pending approval"},
	{"/headless", "Print next response to stdout only"},
	{"/status", "Show current connection status"},
	{"/dumpview", "Dump live View() layout to /tmp/flowpilot-you-view.txt (debug)"},
	{"/info", "Print session/status details (also F2)"},
	{"/login", "Sign in to Supabase (email/password) — Desktop session parity"},
	{"/project", "Project management — /project add to onboard current folder"},
	{"/setup", "Configure Supabase workspace credentials"},
	{"/supabase", "Configure Supabase workspace credentials (alias for /setup)"},
	{"/settings", "Open Desktop app for Settings (start if not running)"},
	{"/sync", "Push session to Drive — /sync · /sync all"},
	{"/restore", "Pull a Drive-backed chat — /restore · /restore all"},
	{"/init", "Init — /init skill (flow-pack) · /init all (full engine)"},
}

// hasModalOpen returns true if any full-screen or popup modal is open.
func (m *AppModel) hasModalOpen() bool {
	return m.projectWizardOpen || m.loginModalOpen || m.supabaseSetupModalOpen || m.modeSetupModalOpen || m.closeDialog != nil
}

// handleOnboardingAfterSession runs the Task-353 D-2 first-run chain after a
// SessionDefaultsMsg: Supabase setup → login → project wizard. Called on
// firstLoad and once more after a first-run Supabase save (supabaseJustConfigured).
// Note: m.sessionDefaultsLoaded is already true when the handler calls this, so
// firstLoad must be passed explicitly.
func (m *AppModel) handleOnboardingAfterSession(msg SessionDefaultsMsg, firstLoad bool) {
	if !firstLoad && !m.supabaseJustConfigured {
		return
	}
	if firstLoad {
		m.addMessage("system", "Ready — type / for commands.", "")
	}
	m.supabaseJustConfigured = false
	m.maybeAutoOnboard(msg.SupabaseConfigured)
}

// maybeAutoOnboard opens the first missing onboarding step for the standalone
// TUI. Idempotent per modal — a modal that is already open is never re-opened,
// so repeated SessionDefaultsMsg refreshes cannot reset in-progress input.
func (m *AppModel) maybeAutoOnboard(supabaseConfigured bool) {
	if !supabaseConfigured {
		// F-1: an unconfigured workspace means the catalog is the offline
		// fake, so setup takes precedence over login — but never interrupt an
		// established session that already has a bound project (CA-633 typing
		// contract, chat_posture_test).
		if m.project == nil && !m.supabaseSetupModalOpen {
			m.openSupabaseSetupModal()
		}
		return
	}
	if m.authNeedLogin {
		if !m.loginModalOpen {
			m.openLoginModal()
		}
		return
	}
	if m.project == nil && m.cfg.ProjectPath != "" && !m.projectWizardOpen {
		m.openProjectWizard(m.cfg.ProjectPath)
	}
}
