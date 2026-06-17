package runner

import (
	"context"
	"sync"
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
	UpsertApproval(ctx context.Context, approval ProviderApprovalState) error
	UpsertQuestion(ctx context.Context, question ProviderQuestionState) error
}

// SessionHistoryReader is an optional extension of InteractiveStateStore that
// lets projectRunHistory rehydrate run history after a runner/app-server restart
// (BUG-060 F-1). Stores that implement this allow history to survive the
// process-scoped in-memory map being empty on a new service instance.
type SessionHistoryReader interface {
	ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error)
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
		steps[i].Status = t.Patch.Status
		if t.Patch.StartedAt != nil {
			steps[i].StartedAt = *t.Patch.StartedAt
		}
		if t.Patch.RejectionNote != nil {
			steps[i].RejectionNote = *t.Patch.RejectionNote
		}
		if t.Patch.RetryCount != nil {
			steps[i].RetryCount = *t.Patch.RetryCount
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
