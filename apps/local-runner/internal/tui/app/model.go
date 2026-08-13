// Package app implements the Bubble Tea TUI for `flowpilot chat`.
// It has no dependency on flowpilot-runner/internal/runner (CP-56 boundary).
//
// Skill: cli-tui (CP-56)
package app

import (
	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
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
}

// QuestionState holds the active question UI state.
type QuestionState struct {
	ID      string
	Prompt  string
	Options []map[string]string
	RunID   string
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

// LoginResultMsg carries a completed Supabase password login.
type LoginResultMsg struct {
	Email        string
	UserID       string
	PersistedTo  string // Desktop session file path (optional)
	PersistError string // soft failure while writing Desktop session
}

// QuitMsg requests app exit.
type QuitMsg struct{}

// TokenUsageMsg carries updated token usage for the statusline.
type TokenUsageMsg struct{ Usage *client.TokenUsageSnapshot }

// AgentGraphMsg carries an updated agent graph snapshot.
type AgentGraphMsg struct{ Graph *client.AgentGraphSnapshot }

// AppModel is the Bubble Tea model for the chat TUI.
type AppModel struct {
	cfg        config.ChatConfig
	runnerURL  string
	client     *client.Client
	mode       Mode
	connStatus ConnStatus
	statusMsg  string
	messages   []ChatMessage

	inputValue string
	viewport   viewportState
	runHandle  *client.RunHandle

	// Per-turn settings
	yolo            bool
	agentsFocus     bool
	selectedSkills  []client.SkillSelection
	skillsCatalog   []client.ProviderSkill // Desktop ChatInput skills list
	pendingAttach   []client.PromptAttachment
	launch          LaunchArm
	firstTurnPending bool // consume builtin FirstTurnExtras once
	reasoningEffort string
	flowBuiltins    []client.BuiltinFlowOption
	flowWorkflows   []client.Workflow
	chatList        []client.RunHistoryItem // last /history result for picker + /open <n>
	sessionPanel    sessionInfoPanel        // collapsible top-right session/status overlay
	flowSteps       []client.WorkflowStepRuntime
	flowStepsActive string // node name currently RUNNING
	turnStream      *turnStreamState
	orchStream      *orchStreamState // Desktop orchestration SSE after turn
	lastEventSeq    int64
	stepsPollTicks  int    // cursor ticks while flow is live
	lastTurnError   string // last turn_failed error (fallback FAIL reason in chat)

	// Pending gate/approval/question state
	gate     *GateState
	approval *ApprovalState
	question *QuestionState

	// Navigation
	project         *client.Project
	projects        []client.Project
	projectPath     string // resolved target path for statusline
	projectBranch   string // git branch at projectPath
	provider        string
	model           string
	accountLabel     string // from provider-accounts display_label
	account          *client.ProviderAccountSummary
	providers        []client.Provider
	providerAccounts []client.ProviderAccountSummary // for ChatInput-parity readiness
	modelContextWin  int64
	lastTokens      *client.TokenUsageSnapshot
	agentRuns       []client.AgentRunSummary
	focusedAgentIdx int
	stepID          string // synthetic chat step from StartRun / Resume
	pendingPrompt   string // first prompt waiting for StartRun to finish

	// Supabase auth (Desktop LoginScreen parity via POST /supabase-auth/login)
	authPhase     AuthPhase
	authEmail     string
	authNeedLogin bool
	signedInEmail string

	// Input focus / slash suggestion selection
	cursorOn bool
	suggIdx  int
	statusSkillsExpanded bool // F3: expand attached skill names under the status chip

	// sessionLoading locks chat while provider/project catalogs load after connect.
	sessionLoading bool
	// sessionDefaultsLoaded is set after the first SessionDefaultsMsg (real chat gate).
	sessionDefaultsLoaded bool
	loadingFrame          int

	// Terminal dimensions
	width  int
	height int

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
	{"/yolo", "Toggle YOLO in chat mode (flow mode is auto-on)"},
	{"/agents", "Focus on the agent graph"},
	{"/agent", "Focus a specific agent by name"},
	{"/flow", "Start or list flows"},
	{"/chat", "Switch to chat mode"},
	{"/skill", "Skills picker — /skill  then ↑↓ Tab tick · Enter apply · F3 status"},
	{"/image", "Attach/list/open images — Alt+V or /image paste, /image <path>, /image open <n>"},
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
}
