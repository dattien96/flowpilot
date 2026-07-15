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
	// FlowStartGitHead is the workspace HEAD captured at startResolvedFlow
	// (Task-242 tier-3). Persisted so audit aggregate diff survives restart
	// (Codex review Important #3).
	FlowStartGitHead string
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
