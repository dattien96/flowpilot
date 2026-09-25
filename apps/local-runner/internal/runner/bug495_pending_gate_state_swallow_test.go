package runner

// BUG-495: reconstructRunInternal read the pending approval/question
// sidecar stores with `err == nil` gating — an unreadable store produced
// empty pending lists, so keepWaitingNodeIDsForResume promoted a durably
// waiting gate to not-waiting on resume. Contract: an unreadable gate
// store must abort resume (502), never silently advance past an
// unanswered approval/question.

import (
	"context"
	"errors"
	"testing"
)

type bug495ApprovalErrStore struct {
	*fakeWorkflowStore
}

func (s *bug495ApprovalErrStore) ListApprovalsByRun(context.Context, string) ([]ProviderApprovalState, error) {
	return nil, errors.New("approvals sidecar unreadable")
}

type bug495QuestionErrStore struct {
	*fakeWorkflowStore
}

func (s *bug495QuestionErrStore) ListQuestionsByRun(context.Context, string) ([]ProviderQuestionState, error) {
	return nil, errors.New("questions sidecar unreadable")
}

func TestBUG495_PendingGateStatesApprovalErrorPropagates(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug495ApprovalErrStore{newFakeWorkflowStore()}}
	if _, _, err := svc.pendingGateStates("run-a"); err == nil {
		t.Fatal("unreadable approvals store must surface an error, not an empty pending list")
	}
}

func TestBUG495_PendingGateStatesQuestionErrorPropagates(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug495QuestionErrStore{newFakeWorkflowStore()}}
	if _, _, err := svc.pendingGateStates("run-a"); err == nil {
		t.Fatal("unreadable questions store must surface an error, not an empty pending list")
	}
}

func TestBUG495_PendingGateStatesHealthyUnchanged(t *testing.T) {
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	if err := fws.UpsertApproval(ctx, ProviderApprovalState{RunID: "run-a", ApprovalID: "ap-1"}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	if err := fws.UpsertQuestion(ctx, ProviderQuestionState{RunID: "run-a", QuestionID: "q-1"}); err != nil {
		t.Fatalf("seed question: %v", err)
	}
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}
	approvals, questions, err := svc.pendingGateStates("run-a")
	if err != nil {
		t.Fatalf("healthy store returned error: %v", err)
	}
	if len(approvals) != 1 || len(questions) != 1 {
		t.Fatalf("pending states lost on healthy path: approvals=%d questions=%d", len(approvals), len(questions))
	}
}

