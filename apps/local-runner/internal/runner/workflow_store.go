package runner

import (
	"context"
	"sync"

	"flowpilot-runner/internal/agentpack"
)

// Phase 5 (04-05): the persistence boundary for workflow orchestration. The runner
// becomes the single backend, reading/writing run/step/log state in Supabase. The
// orchestrator depends only on this interface; the live PostgREST implementation is
// SupabaseWorkflowStore (supabase_workflow_store.go), and tests use the in-memory
// fakeWorkflowStore.
//
// All writes are idempotent (ApplyTransition is keyed by step id and overwrites; a
// retried apply converges to the same row) so a partial failure mid-progress is
// safe to retry — the concurrency requirement in 04-05 (T-42).
type WorkflowStore interface {
	// LoadRunSteps returns a run's steps in execution order.
	LoadRunSteps(ctx context.Context, runID string) ([]RuntimeWorkflowStep, error)
	// ApplyStepTransition patches one step (idempotent, last-write-wins).
	ApplyStepTransition(ctx context.Context, runID string, t WorkflowStepTransition) error
	// SetRunStatus updates the run-level status + finished_at.
	SetRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt string) error
	// AppendLog records a step log line.
	AppendLog(ctx context.Context, stepID string, log WorkflowLog) error
}

// InteractiveStateStore is the Phase 8 A2 persistence boundary for interactive
// session/question/approval state. It is optional so the Phase 5 workflow store can
// evolve incrementally without forcing every caller to depend on the new tables at
// once.
type InteractiveStateStore interface {
	AppendEvent(ctx context.Context, event ProviderEvent) error
	UpsertProviderSession(ctx context.Context, session ProviderSessionState) error
	DeleteProviderSession(ctx context.Context, runID string) error
	UpsertApproval(ctx context.Context, approval ProviderApprovalState) error
	UpsertQuestion(ctx context.Context, question ProviderQuestionState) error
}

// SessionHistoryReader is an optional extension of InteractiveStateStore that
// lets projectRunHistory rehydrate run history after a runner/app-server restart
// (BUG-060 F-1). Stores that implement this allow history to survive the
// process-scoped in-memory map being empty on a new service instance.
type SessionHistoryReader interface {
	ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error)
	GetProviderSession(ctx context.Context, runID string) (ProviderSessionState, bool, error)
}

// FlowEventStore is an optional extension of InteractiveStateStore that
// persists CP-41 flow events (FlowContextPackage, ValidationResult, ValidationRetry,
// AuditDraft) to a per-run sidecar NDJSON so they survive process restarts.
type FlowEventStore interface {
	LoadFlowEvents(ctx context.Context, runID string) ([]ProviderEvent, error)
	DeleteFlowEvents(ctx context.Context, runID string) error
}

// QuestionHistoryReader is an optional extension of InteractiveStateStore that
// lets reconstructRun re-synthesize resolved/expired user_question_required
// events after a process restart (BUG-StaleQuestion-Restart). A restart rebuilds
// rs.events from the sidecar/provider transcript, which has no concept of
// FlowPilot's own ask_user gate — the raw asked event replays via FlowEventStore,
// but whether it was later answered or expired lives in the separately-upserted
// ProviderQuestionState. Stores that implement this let reconstructRun stamp the
// resolved Answer (read-only render) or drop an expired question, instead of the
// question either vanishing or replaying as a fresh interactive form.
type QuestionHistoryReader interface {
	ListQuestionsByRun(ctx context.Context, runID string) ([]ProviderQuestionState, error)
}

// ApprovalHistoryReader is the approval-side twin of QuestionHistoryReader
// (BUG-ApprovalReplay-Restart). A restart rebuilds rs.events from the
// sidecar/provider transcript, which has no concept of FlowPilot's own
// permission_required approval card — the raw asked event replays via
// FlowEventStore (EventPermissionRequired is now a sidecar type), but whether
// it was approved/denied/expired lives in the separately-upserted
// ProviderApprovalState. Stores that implement this let reconstructRun stamp
// the recorded Decision (read-only render) or drop an expired approval,
// instead of the card either vanishing or replaying as a fresh interactive
// prompt the run appears to be waiting on. Mirrors QuestionHistoryReader.
type ApprovalHistoryReader interface {
	ListApprovalsByRun(ctx context.Context, runID string) ([]ProviderApprovalState, error)
}

type SessionIndexReader interface {
	ListAllProviderSessions(ctx context.Context) ([]ProviderSessionState, error)
}

type ProviderSessionState struct {
	RunID             string
	ProjectID         string
	WorkflowID        string
	ProviderSessionID string
	ProviderKey       ProviderKey
	ProviderAccountID string
	WorkingDirectory  string
	Status            RunStatus
	LastPrompt        string
	LastMessage       string
	StartedAt         string
	UpdatedAt         string
	RunKind           string
	SourceMachineID   string
	SourceRunID       string
	RestoredFrom      string
	SyncStatus        string
	SyncUpdatedAt     string
	// ParentRunID is set for child agent runs (CP-19 / Task-082); empty for root runs.
	ParentRunID string
	AgentName   string
	Label       string
	Role        string
	DependsOn   []string
	AgentStatus string
	ModelName   string
	ChangeType  string
	SourceDocID string
	TurnCount   int
	// PendingAgentContext carries notes about UI-spawned children not yet folded into this
	// run's provider conversation, persisted so the parent still learns about them after a
	// restart (BUG-122).
	PendingAgentContext []string
	// LoopState persists the flow-engine loop state for root (parent) runs so a
	// runner restart or Drive-synced cross-PC move can resume the correct round/cap
	// (Task-085). Zero-valued for child runs and plain chat runs.
	LoopState AgentLoopState
	// AutoOrchestrate persists the hub auto-reinvocation flag (Task-093 / CP-36 P-7).
	// False for child runs and plain chat runs.
	AutoOrchestrate bool
	// FlowCohortID persists the cohort membership ID for child runs (CP-36 / Task-095 BUG fix).
	// Empty for parent runs and plain chat runs.
	FlowCohortID string
	// ActiveFlowEdges/ActiveFlowNodes persist a resolved flow's tracked topology
	// for root (parent) runs (BUG-NOTE-CP42 #16), so edge-driven back-edge
	// routing (resolveContinueBackEdgeTarget) and forward auto-advance
	// (tryAdvanceFlowFromNode) keep working after a runner restart or a
	// Drive-synced cross-PC move — without this, a chat reopened mid-flow
	// after a restart silently reverts to legacy isCoderRun role matching.
	// Empty for a plain chat run never started via a resolved flowRef.
	ActiveFlowEdges []agentpack.FlowEdge
	ActiveFlowNodes []agentpack.FlowNode
	// ChatSubMode/ChatFlowRef persist the explicit Chat-Mode orchestration
	// picker selection (CP-42/Task-177) a run was started with, e.g.
	// subMode="bug", flowRef="flowpilot-core-flow-pack/review-loop"
	// (BUG-263). Empty for a plain chat run or a Flow-Mode workflow-picker
	// launch (which has its own WorkflowID/launchMode restore path already).
	ChatSubMode string
	ChatFlowRef string
	// PendingFlowGateSettle is durable gate-pending state (V10 P0): after
	// restart, reconstructRun re-queues post-turn gate instead of treating
	// the child/root as completed.
	PendingFlowGateSettle     bool
	PendingFlowGateFinalMsg   string
	PendingFlowGateOccurredAt string
	// PendingFlowGateTurnID is the provider turn id for the deferred terminal
	// event (V10R P1). Without it, resume materializes EventTurnCompleted with
	// empty ProviderTurnID.
	PendingFlowGateTurnID string
	// TurnStartGitHead / TurnStartWorktree / PendingGateChangedFiles are the
	// turn-scoped gate snapshot. Without these, resumePendingFlowGate would
	// re-observe against empty base and treat all historical dirt as this turn.
	TurnStartGitHead        string
	TurnStartWorktree       map[string]string
	PendingGateChangedFiles []string
	// StepID / LastTurnStepID persist the execution-step context needed to
	// restart a rehydrated approval/question turn after restart (V10R P1).
	StepID         string
	LastTurnStepID string
	// PendingGateReprompt* / PendingGateCodePaths / RepromptAttempts survive
	// restart so a gate-reprompt remediation is not lost if startTurn fails or
	// the process dies mid-window (V10R4 P1).
	PendingGateRepromptPrompt string
	PendingGateRepromptStepID string
	PendingGateCodePaths      []string
	RepromptAttempts          int
	// PendingResumePrompt/StepID is a durable continuation intent after
	// rehydrated approval/question resolve. Cleared only after startTurn
	// accepts the turn (V10R4 P1).
	PendingResumePrompt string
	PendingResumeStepID string
	// PendingResumeGen / PendingGateRepromptGen are compare-and-clear tokens so
	// a newer intent is not wiped by a late successful startTurn for an older
	// intent (V10R4 P1).
	PendingResumeGen       int64
	PendingGateRepromptGen int64
	// Pending*DeliveredGen + AcceptedTurn are set ONLY after startTurn accepts
	// a turn for that generation (V10R4 P0-02). Never write DeliveredGen before
	// the provider call — that created a permanent intent-loss window.
	PendingResumeDeliveredGen         int64
	PendingGateRepromptDeliveredGen   int64
	PendingResumeAcceptedTurn         string
	PendingGateRepromptAcceptedTurn   string
	// Durable fail budget keyed by generation (survives process restart).
	PendingResumeFailCount       int
	PendingResumeFailGen         int64
	PendingGateRepromptFailCount int
	PendingGateRepromptFailGen   int64
	// PendingResumeApprovalID / PendingResumeDecision reconcile card resolution
	// with continuation intent across a two-write crash (V10R4 P1).
	PendingResumeApprovalID string
	PendingResumeDecision   string
	// PendingResumeQuestionChoices preserves the original multi-select answer
	// slice for reconciliation (BUG-288 P2-01). PendingResumeDecision keeps a
	// joined display string (used for approvals and for display/back-compat),
	// but reconciling a crashed multi-select answer from the joined string
	// re-split on ", " would corrupt any choice that itself contains that
	// separator (e.g. ["a, b", "c"] -> ["a, b, c"]). This field carries the
	// real slice so rehydrate can restore it verbatim instead of re-deriving.
	PendingResumeQuestionChoices []string
	// FlowStartGitHead is the workspace HEAD captured at startResolvedFlow
	// (Task-242 tier-3). Persisted so audit aggregate diff survives restart
	// (Codex review Important #3).
	FlowStartGitHead string
	// StopGeneration / ParentStopGenSeen implement the parent stop-tombstone
	// authority (BUG-288 P1-04): StopGeneration is the root run's own durable,
	// monotonically increasing stop counter; ParentStopGenSeen is a child run's
	// snapshot of its parent's StopGeneration as of this child's last
	// checkpoint. Child reconstruction and resumePendingFlowGate reject a
	// deferred gate when the parent's current StopGeneration is newer than the
	// child's ParentStopGenSeen — the child was checkpointed before a Stop it
	// never itself observed, so its pending gate must not resurrect.
	StopGeneration    int64
	ParentStopGenSeen int64
	// IntentBlockedKind/Reason/At durably record a durable turn intent that
	// exhausted its permanent-failure retry budget (BUG-288 P1-09).
	IntentBlockedKind   string
	IntentBlockedReason string
	IntentBlockedAt     string
	// TransitionLogDegraded/At/Reason durably record a step-transition-log
	// append failure (BUG-288 P2-03) so restore/replay can detect that the
	// step timeline for this run is not fully trustworthy.
	TransitionLogDegraded       bool
	TransitionLogDegradedAt     string
	TransitionLogDegradedReason string
	// PendingRestartRunID/Prompt durably record a Stall-Retry restart intent
	// on the PARENT run (BUG-288 P1-18/P1-14). Previously this lived only in
	// RAM (interactiveRun.pendingRestartRunID/pendingRestartPrompt); a crash
	// between the member_action "retry" request (which cancels the child's
	// in-flight turn) and finishTurn actually re-starting the turn silently
	// dropped the retry with no trace. Persisting it here mirrors the
	// existing PendingResume*/PendingGateReprompt* durable-intent pattern.
	PendingRestartRunID string
	PendingRestartPrompt string
	// PendingRestartGen is the durable delivery generation for stall-retry
	// (BUG-288 R15-P0): claim + idempotency key "durable-{child}-restart-{gen}"
	// so crash between clear and startTurn cannot duplicate or drop the retry.
	PendingRestartGen int64
	// FlowContextInjected (BUG-288 R13-16) survives restart so a durable
	// PendingRestartPrompt that already embeds a flowpilot-fcp marker does not
	// get feature-history / FCP re-injected after process restart (MAC secret is
	// per-process; the durable flag is the double-injection guard).
	FlowContextInjected bool
}

type ProviderApprovalState struct {
	ApprovalID      string
	RunID           string
	ProviderKey     ProviderKey
	ProviderTurnID  string
	Command         string
	Cwd             string
	Reason          string
	Status          string
	Decision        string
	Policy          string
	ExpiresAt       string
	ResolvedChoices []string
}

type ProviderQuestionState struct {
	QuestionID      string
	RunID           string
	ProviderTurnID  string
	Prompt          string
	Options         []QuestionOption
	MultiSelect     bool
	Status          string
	Choice          []string
	ExpiresAt       string
	ResolvedChoices []string
}

// ---- in-memory fake (tests) ------------------------------------------------

type fakeWorkflowStore struct {
	mu        sync.Mutex
	steps     map[string][]RuntimeWorkflowStep // runID -> steps
	runStatus map[string]WorkflowRunStatus
	finished  map[string]string
	logs      map[string][]WorkflowLog // stepID -> logs
	applies   int                      // total ApplyStepTransition calls (idempotency probe)
	events    map[string][]ProviderEvent
	sessions  map[string]ProviderSessionState
	approvals map[string]ProviderApprovalState
	questions map[string]ProviderQuestionState
}

func newFakeWorkflowStore() *fakeWorkflowStore {
	return &fakeWorkflowStore{
		steps:     map[string][]RuntimeWorkflowStep{},
		runStatus: map[string]WorkflowRunStatus{},
		finished:  map[string]string{},
		logs:      map[string][]WorkflowLog{},
		events:    map[string][]ProviderEvent{},
		sessions:  map[string]ProviderSessionState{},
		approvals: map[string]ProviderApprovalState{},
		questions: map[string]ProviderQuestionState{},
	}
}

func (f *fakeWorkflowStore) seed(runID string, steps []RuntimeWorkflowStep) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps[runID] = steps
}

func (f *fakeWorkflowStore) LoadRunSteps(_ context.Context, runID string) ([]RuntimeWorkflowStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RuntimeWorkflowStep, len(f.steps[runID]))
	copy(out, f.steps[runID])
	return out, nil
}

func (f *fakeWorkflowStore) ApplyStepTransition(_ context.Context, runID string, t WorkflowStepTransition) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies++
	steps := f.steps[runID]
	for i := range steps {
		if steps[i].ID != t.StepID {
			continue
		}
		// BUG-228: an empty Status means "leave unchanged" (e.g. setFlowStepPosture
		// only patches Provider/Model) — a real transition always sets a non-empty
		// RuntimeWorkflowStepStatus, so this is unambiguous.
		if t.Patch.Status != "" {
			steps[i].Status = t.Patch.Status
		}
		if t.Patch.StartedAt != nil {
			steps[i].StartedAt = *t.Patch.StartedAt
		}
		if t.Patch.FinishedAt != nil {
			steps[i].FinishedAt = *t.Patch.FinishedAt
		}
		if t.Patch.RejectionNote != nil {
			steps[i].RejectionNote = *t.Patch.RejectionNote
		}
		if t.Patch.RetryCount != nil {
			steps[i].RetryCount = *t.Patch.RetryCount
		}
		if t.Patch.Provider != nil {
			steps[i].Provider = *t.Patch.Provider
		}
		if t.Patch.Model != nil {
			steps[i].Model = *t.Patch.Model
		}
		break
	}
	return nil
}

func (f *fakeWorkflowStore) SetRunStatus(_ context.Context, runID string, status WorkflowRunStatus, finishedAt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runStatus[runID] = status
	f.finished[runID] = finishedAt
	return nil
}

func (f *fakeWorkflowStore) AppendLog(_ context.Context, stepID string, log WorkflowLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs[stepID] = append(f.logs[stepID], log)
	return nil
}

func (f *fakeWorkflowStore) AppendEvent(_ context.Context, event ProviderEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[event.WorkflowRunID] = append(f.events[event.WorkflowRunID], event)
	return nil
}

func (f *fakeWorkflowStore) UpsertProviderSession(_ context.Context, session ProviderSessionState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[session.RunID] = session
	return nil
}

func (f *fakeWorkflowStore) DeleteProviderSession(_ context.Context, runID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, runID)
	return nil
}

func (f *fakeWorkflowStore) UpsertApproval(_ context.Context, approval ProviderApprovalState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approvals[approval.ApprovalID] = approval
	return nil
}

func (f *fakeWorkflowStore) UpsertQuestion(_ context.Context, question ProviderQuestionState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.questions[question.QuestionID] = question
	return nil
}

func (f *fakeWorkflowStore) ListQuestionsByRun(_ context.Context, runID string) ([]ProviderQuestionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ProviderQuestionState
	for _, q := range f.questions {
		if q.RunID == runID {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) ListApprovalsByRun(_ context.Context, runID string) ([]ProviderApprovalState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ProviderApprovalState
	for _, a := range f.approvals {
		if a.RunID == runID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) ListProviderSessionsByProject(_ context.Context, projectID string) ([]ProviderSessionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ProviderSessionState
	for _, s := range f.sessions {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) GetProviderSession(_ context.Context, runID string) (ProviderSessionState, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.sessions[runID]
	return st, ok, nil
}

func (f *fakeWorkflowStore) ListAllProviderSessions(_ context.Context) ([]ProviderSessionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProviderSessionState, 0, len(f.sessions))
	for _, session := range f.sessions {
		out = append(out, session)
	}
	return out, nil
}
