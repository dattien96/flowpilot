package runner

// Phase 2 (04-02): provider-runtime contract types for the interactive APIs.
// These are the Go realization of the shared contract in
// requirements/10-Refactor/New-System/04-Detailed-Coding-Plan.md and the DTOs the
// desktop client (04-01) consumes. Codex/Claude/Gemini specifics live in their own
// adapters (Phase 3+); this file is provider-neutral.

// ProviderKey identifies a provider runtime.
type ProviderKey string

const (
	ProviderKeyCodex  ProviderKey = "codex"
	ProviderKeyClaude ProviderKey = "claude"
	ProviderKeyGemini ProviderKey = "gemini"
)

// RunStatus mirrors the client-facing RunStatus set (04-01) — the user-facing
// projection of the provider session/turn states (03).
type RunStatus string

const (
	RunStatusIdle            RunStatus = "idle"
	RunStatusStarting        RunStatus = "starting"
	RunStatusRunning         RunStatus = "running"
	RunStatusWaitingApproval RunStatus = "waiting_approval"
	RunStatusWaitingQuestion RunStatus = "waiting_question"
	RunStatusCompleted       RunStatus = "completed"
	RunStatusFailed          RunStatus = "failed"
	RunStatusCancelled       RunStatus = "cancelled"
)

// ProviderEventType is the discriminator for the normalized event union.
type ProviderEventType string

const (
	EventTurnStarted          ProviderEventType = "turn_started"
	EventMessageDelta         ProviderEventType = "message_delta"
	EventMessageCompleted     ProviderEventType = "message_completed"
	EventToolStarted          ProviderEventType = "tool_started"
	EventToolCompleted        ProviderEventType = "tool_completed"
	EventFileChanged          ProviderEventType = "file_changed"
	EventPermissionRequired   ProviderEventType = "permission_required"
	EventUserQuestionRequired ProviderEventType = "user_question_required"
	EventTurnFailed           ProviderEventType = "turn_failed"
	EventTurnCompleted        ProviderEventType = "turn_completed"
)

// ApprovalDecisionOption is one decision the runtime offers for an approval.
type ApprovalDecisionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ApprovalDetails describes what the runtime wants to do (permission_required).
type ApprovalDetails struct {
	Command   string                   `json:"command,omitempty"`
	Cwd       string                   `json:"cwd,omitempty"`
	Reason    string                   `json:"reason,omitempty"`
	Decisions []ApprovalDecisionOption `json:"decisions"`
}

// QuestionOption is one choice for a user_question_required card.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
}

// ProviderEvent is the normalized, serialized event — a single Go struct keyed by
// Type (the Go-friendly form of the 04 discriminated union). Every event carries a
// monotonic per-run Seq (the reconnect cursor, 04-02). Type-specific fields are
// omitempty so the JSON matches the per-variant shape the client expects.
type ProviderEvent struct {
	ID                string            `json:"id"`
	Seq               int64             `json:"seq"`
	Type              ProviderEventType `json:"type"`
	WorkflowRunID     string            `json:"workflowRunId"`
	WorkflowStepRunID string            `json:"workflowStepRunId,omitempty"`
	ProviderSessionID string            `json:"providerSessionId"`
	ProviderKey       ProviderKey       `json:"providerKey"`
	ProviderTurnID    string            `json:"providerTurnId,omitempty"`
	OccurredAt        string            `json:"occurredAt"`

	// message_delta / message_completed
	Text string `json:"text,omitempty"`
	// turn_completed
	FinalMessage string `json:"finalMessage,omitempty"`
	// tool_started / tool_completed
	ToolName string `json:"toolName,omitempty"`
	Input    any    `json:"input,omitempty"`
	Output   any    `json:"output,omitempty"`
	Status   string `json:"status,omitempty"`
	// file_changed
	Path       string `json:"path,omitempty"`
	ChangeType string `json:"changeType,omitempty"`
	// permission_required
	ApprovalID string           `json:"approvalId,omitempty"`
	Provider   ProviderKey      `json:"provider,omitempty"`
	Details    *ApprovalDetails `json:"details,omitempty"`
	// user_question_required
	QuestionID  string           `json:"questionId,omitempty"`
	Prompt      string           `json:"prompt,omitempty"`
	Options     []QuestionOption `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
	// turn_failed
	Error       string `json:"error,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
}

// ProviderCapabilities advertises what a provider supports (03/04-07).
type ProviderCapabilities struct {
	Streaming      bool `json:"streaming"`
	Resume         bool `json:"resume"`
	ApprovalEvents bool `json:"approvalEvents"`
	FileEvents     bool `json:"fileEvents"`
	SkillSelection bool `json:"skillSelection"`
	Mcp            bool `json:"mcp"`
	Interrupt      bool `json:"interrupt"`
}

// ---- Run lifecycle DTOs (mirror 04-01 contract.ts) -------------------------

type RunHandle struct {
	RunID             string      `json:"runId"`
	ProviderSessionID string      `json:"providerSessionId"`
	ProviderKey       ProviderKey `json:"providerKey"`
	Status            RunStatus   `json:"status"`
	StepID            string      `json:"stepId,omitempty"`
}

type StartRunInput struct {
	ProjectID  string `json:"projectId"`
	WorkflowID string `json:"workflowId,omitempty"`
	StepID     string `json:"stepId"`
	// ProviderKey selects the provider runtime explicitly (direct-chat UI selector).
	// Empty → the runner auto-selects: from Model if given (workflow/step mode), else the
	// first available provider. A disabled/placeholder provider is rejected runner-side
	// (04-07 capability enforcement).
	ProviderKey ProviderKey `json:"providerKey,omitempty"`
	// Model is the workflow/step's configured model; when ProviderKey is empty the runner
	// derives the provider from it (providerKeyFromModel). Ignored when ProviderKey is set.
	Model    string `json:"model,omitempty"`
	YoloMode bool   `json:"yoloMode,omitempty"`
	// Cwd is the active workspace directory for this run (04-06 multi-workspace).
	// Per-run/per-thread cwd is authoritative; Runner.workspace is only a default.
	Cwd string `json:"cwd,omitempty"`
}

type SkillSelection struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source"`
}

type TurnInput struct {
	StepID         string           `json:"stepId"`
	Prompt         string           `json:"prompt"`
	SelectedSkills []SkillSelection `json:"selectedSkills,omitempty"`
}

// ---- Catalog DTOs (navigator; fake catalog in P2) --------------------------

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Workflow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Step struct {
	ID           string `json:"id"`
	WorkflowID   string `json:"workflowId,omitempty"`
	Name         string `json:"name"`
	Order        int    `json:"order"`
	DefaultSkill string `json:"defaultSkill,omitempty"`
}

type ProviderSkill struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
}

type Artifact struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Preview   string `json:"preview,omitempty"`
	CreatedAt string `json:"createdAt"`
}
