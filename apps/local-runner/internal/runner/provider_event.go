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
	// ProviderKeyGrok is Grok Build over ACP (CP-46). Appended last — existing
	// cases/order are unchanged (CP-46 P-0 base-regression guard).
	ProviderKeyGrok ProviderKey = "grok"
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
	EventTokenUsageUpdated    ProviderEventType = "token_usage_updated"
	EventToolStarted          ProviderEventType = "tool_started"
	EventToolCompleted        ProviderEventType = "tool_completed"
	EventFileChanged          ProviderEventType = "file_changed"
	EventPermissionRequired   ProviderEventType = "permission_required"
	EventUserQuestionRequired ProviderEventType = "user_question_required"
	EventTurnFailed           ProviderEventType = "turn_failed"
	EventTurnCompleted        ProviderEventType = "turn_completed"
	EventAgentGraphUpdated    ProviderEventType = "agent_graph_updated"
	EventAgentBusMessage      ProviderEventType = "agent_bus_message"
	// Emitted on the parent run when a user triggers a spawn from the UI (BUG-121).
	// Persisted to the parent event log so the annotation survives server restarts.
	EventAgentSpawnedByUser  ProviderEventType = "agent_spawned_by_user"
	EventAgentResultInjected ProviderEventType = "agent_result_injected"
	// Emitted after a turn completes when the post-turn flow gate detects a violation
	// (CP-35 P-4/P-5). The desktop surfaces it as an inline warning card.
	EventFlowGateViolation  ProviderEventType = "flow_gate_violation"
	EventFlowContextPackage ProviderEventType = "flow_context_package"
	// Emitted by the Testing step when a validation command completes (Task-170).
	EventFlowValidationResult ProviderEventType = "flow_validation_result"
	// Emitted when a Coding retry is scheduled after a failed validation (Task-170).
	EventFlowValidationRetry ProviderEventType = "flow_validation_retry"
	// Emitted when the Audit step prepares its draft (Task-171).
	EventFlowAuditDraft ProviderEventType = "flow_audit_draft"
)

// ApprovalDecisionOption is one decision the runtime offers for an approval.
type ApprovalDecisionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ApprovalDetails describes what the runtime wants to do (permission_required).
type ApprovalDetails struct {
	Command string `json:"command,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Kind classifies the approval so the runner and desktop can treat shell
	// commands differently from file writes / MCP prompts. Only "exec" approvals
	// are eligible for the per-project "don't ask again" allowlist (BUG-246).
	// One of: "exec", "file", "mcp", "other" (empty ~= "other").
	Kind      string                   `json:"kind,omitempty"`
	Decisions []ApprovalDecisionOption `json:"decisions"`
}

// QuestionOption is one choice for a user_question_required card.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
}

type TokenUsageBreakdown struct {
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	InputTokens           int64 `json:"inputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
	TotalTokens           int64 `json:"totalTokens"`
}

type TokenUsageSnapshot struct {
	Last               *TokenUsageBreakdown `json:"last,omitempty"`
	Total              *TokenUsageBreakdown `json:"total,omitempty"`
	ModelContextWindow *int64               `json:"modelContextWindow,omitempty"`
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
	// token_usage_updated
	TokenUsage *TokenUsageSnapshot `json:"tokenUsage,omitempty"`
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
	// Decision is populated only when replaying an already-resolved approval on
	// a full server restart (BUG-ApprovalReplay-Restart) — it carries the
	// recorded approve/deny (or "resolved") so the client renders the approval
	// card read-only instead of re-showing an interactive prompt the run
	// appears to be waiting on. The approval-side twin of Answer above.
	Decision string `json:"decision,omitempty"`
	// user_question_required
	QuestionID  string           `json:"questionId,omitempty"`
	Prompt      string           `json:"prompt,omitempty"`
	Options     []QuestionOption `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
	// Answer is populated only when replaying an already-resolved question on
	// reconnect (BUG-StaleQuestion) — it carries the recorded choice so the
	// client renders the QuestionCard read-only instead of re-showing an
	// interactive form for a question that was already answered.
	Answer []string `json:"answer,omitempty"`
	// turn_failed
	Error       string `json:"error,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
	// flow_gate_violation (r-reg decision card — Task-155)
	GateOptions        []string `json:"gateOptions,omitempty"`
	GateRegressedTests []string `json:"gateRegressedTests,omitempty"`
	// agent_graph_updated / agent_bus_message
	AgentGraphSnapshot *AgentGraphSnapshot `json:"agentGraphSnapshot,omitempty"`
	AgentBusMessage    *AgentBusMessage    `json:"agentBusMessage,omitempty"`
	// agent_spawned_by_user / agent_result_injected (BUG-121)
	AgentName  string `json:"agentName,omitempty"`
	ChildRunID string `json:"childRunId,omitempty"`
	// flow_context_package (Task-168)
	FlowContextPackage *FlowContextPackage `json:"flowContextPackage,omitempty"`
	// flow_validation_result (Task-170): bounded metadata for a Testing step command run.
	// Raw stdout/stderr are not persisted here; only metadata and exit code are kept.
	FlowValidationResult *ValidationResultMeta `json:"flowValidationResult,omitempty"`
	// flow_validation_retry (Task-170): snapshot of the retry state transition.
	FlowValidationRetryState *FlowValidationRetryState `json:"flowValidationRetryState,omitempty"`
	// flow_audit_draft (Task-171): audit draft prepared after successful validation.
	FlowAuditDraft *FlowAuditDraft `json:"flowAuditDraft,omitempty"`
}

type AgentDependencyEdge struct {
	FromRunID string `json:"fromRunId"`
	ToRunID   string `json:"toRunId"`
	Kind      string `json:"kind"`
}

type AgentBusMessage struct {
	ID          string `json:"id"`
	ParentRunID string `json:"parentRunId"`
	FromRunID   string `json:"fromRunId,omitempty"`
	ToRunID     string `json:"toRunId,omitempty"`
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Queued      bool   `json:"queued"`
	OccurredAt  string `json:"occurredAt"`
}

type AgentLoopState struct {
	Status     string `json:"status"`
	Round      int    `json:"round"`
	RoundCap   int    `json:"roundCap"`      // legacy; use Cap for flow-engine paths
	Cap        int    `json:"cap,omitempty"` // flow-engine cap (Task-090); mirrors RoundCap when 0
	GateReason string `json:"gateReason,omitempty"`
	// New fields added by Task-090 (flow engine)
	OpenIssues  int    `json:"openIssues,omitempty"`
	Mode        string `json:"mode,omitempty"` // "keyword" | "explicit"
	ActiveNode  string `json:"activeNode,omitempty"`
	ExtendCount int    `json:"extendCount,omitempty"`
	// ExtendBy is how much extendCap/resumeFlowWithFeedback raise Cap by on a
	// cap-hit, seeded from the flow's own Definition.Policy.ExtendBy at
	// startResolvedFlow (falls back to 2 when unset/zero, matching the
	// pre-existing hardcoded default). Per-flow, not global, so a custom flow
	// with a different policy_extend_by value actually takes effect.
	ExtendBy int `json:"extendBy,omitempty"`
	// BlockReason distinguishes WHY Status=="blocked" (BUG-231): "cap" (the
	// round cap was reached mid-loop, via "continue") vs. "escalate" (the
	// flow's control tool explicitly escalated, e.g. submit_review_outcome
	// status=blocked). Both are non-terminal "awaiting user" pauses, but the
	// desktop's recovery affordance differs in how it resumes (see
	// resumeFlowWithFeedback): a "cap" block auto-raises the cap, an
	// "escalate" block does not need to. Cleared ("") whenever Status leaves
	// "blocked".
	BlockReason string `json:"blockReason,omitempty"`
}

type AgentGraphSnapshot struct {
	ParentRunID string                `json:"parentRunId"`
	Runs        []AgentRunSummary     `json:"runs"`
	Edges       []AgentDependencyEdge `json:"edges"`
	BusMessages []AgentBusMessage     `json:"busMessages"`
	LoopState   AgentLoopState        `json:"loopState"`
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
	// Vision advertises that the adapter can accept image attachments on a turn
	// (Task-052). The desktop gates the attach control on this; false for the
	// placeholder/non-vision adapters.
	Vision bool `json:"vision"`
}

// PromptAttachment is an image attached to a chat turn (Task-052), mirroring the
// desktop `PromptAttachment` in contract.ts. Data is the base64 of the normalized
// bytes (no `data:` prefix), carried inline in the turn payload (V1 — D-2).
type PromptAttachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"` // "image"
	OriginalName string `json:"originalName"`
	MimeType     string `json:"mimeType"`
	Data         string `json:"data"` // base64, no data: prefix
	SizeBytes    int64  `json:"sizeBytes"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

// ---- Run lifecycle DTOs (mirror 04-01 contract.ts) -------------------------

type RunHandle struct {
	RunID             string      `json:"runId"`
	ProviderSessionID string      `json:"providerSessionId"`
	ProviderKey       ProviderKey `json:"providerKey"`
	Status            RunStatus   `json:"status"`
	StepID            string      `json:"stepId,omitempty"`
	// LastEventSeq is the seq of the last persisted event at resume time. The desktop
	// replays the run from seq 0 and uses this as the stop cursor so a multi-turn run
	// is replayed in full (not truncated at the first turn_completed). 0 when unknown.
	LastEventSeq int64 `json:"lastEventSeq,omitempty"`
}

type StartRunInput struct {
	ProjectID  string `json:"projectId"`
	WorkflowID string `json:"workflowId,omitempty"`
	StepID     string `json:"stepId,omitempty"`
	// ProviderKey selects the provider runtime explicitly (direct-chat UI selector).
	// Empty → the runner auto-selects: from Model if given (workflow/step mode), else the
	// first available provider. A disabled/placeholder provider is rejected runner-side
	// (04-07 capability enforcement).
	ProviderKey ProviderKey `json:"providerKey,omitempty"`
	// Model is the workflow/step's configured model; when ProviderKey is empty the runner
	// derives the provider from it (providerKeyFromModel). Ignored when ProviderKey is set.
	Model           string `json:"model,omitempty"`
	YoloMode        bool   `json:"yoloMode,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// ChatMode == "normal_chat" signals that the desktop is in provider-chat mode (no
	// workflow/step selection). The runner mints a synthetic chat step and tags the run
	// with RunKind="chat" so it is excluded from workflow catalogs (T-7).
	ChatMode string `json:"chatMode,omitempty"`
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
	ChangeType     string           `json:"changeType,omitempty"`
	SourceDocID    string           `json:"sourceDocId,omitempty"`
	SelectedSkills []SkillSelection `json:"selectedSkills,omitempty"`
	// ReasoningEffort/Model/YoloMode are per-turn chat overrides (BUG-063): the desktop
	// resends the current control values on every chat turn so model, reasoning, and YOLO
	// can be changed between prompts (the providers re-apply them per turn). nil pointers
	// mean "not supplied" (workflow/step mode, or an older client) and fall back to the
	// run-level value captured at startRun. An empty Model string is a deliberate "Default".
	ReasoningEffort string  `json:"reasoningEffort,omitempty"`
	Model           *string `json:"model,omitempty"`
	YoloMode        *bool   `json:"yoloMode,omitempty"`
	// Attachments carries image attachments for chat-mode turns (Task-052), inline as
	// base64. Empty in workflow/step mode and when no images are attached.
	Attachments []PromptAttachment `json:"attachments,omitempty"`
	// SubMode/FlowRef select an optional built-in Chat Mode orchestration
	// template (CP-42/Task-177). Validated by handleStartTurn against
	// BuiltinOrchestrationOptions before reaching startTurn, so by the time
	// startTurn sees a non-empty FlowRef it is already a known-valid option
	// for SubMode.
	SubMode string `json:"subMode,omitempty"`
	FlowRef string `json:"flowRef,omitempty"`
}

// ---- Catalog DTOs (navigator; fake catalog in P2) --------------------------

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Model is projects.default_model — the "Project" tier of the Step > Flow >
	// Project > default resolution order (SS-05/SD-06, BUG-165).
	Model string `json:"model,omitempty"`
}

type Workflow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Model is workflows.model_override — the "Flow" tier of the Step > Flow >
	// Project > default resolution order (SS-05/SD-06, BUG-165).
	Model    string `json:"model,omitempty"`
	YoloMode bool   `json:"yoloMode,omitempty"`
}

type Step struct {
	ID           string `json:"id"`
	WorkflowID   string `json:"workflowId,omitempty"`
	Name         string `json:"name"`
	Order        int    `json:"order"`
	DefaultSkill string `json:"defaultSkill,omitempty"`
	NodeID       string `json:"nodeId,omitempty"`
	BehaviorID   string `json:"behaviorId,omitempty"`
	AgentRef     string `json:"agentRef,omitempty"`
	// Model is the step's step_definitions.model — the "Step" tier of the
	// Step > Flow > Project > default resolution order (SS-05/SD-06, BUG-165).
	Model string `json:"model,omitempty"`
	// YoloMode is the step's step_definitions.yolo_mode default. Workflow/Flow
	// starts lift an enabled entry-step default to the run-level YOLO posture.
	YoloMode bool `json:"yoloMode,omitempty"`
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
