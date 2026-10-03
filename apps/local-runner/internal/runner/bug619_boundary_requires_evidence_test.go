package runner

import (
	"context"
	"path/filepath"
	"testing"
)

// BUG-619 (live run-150388, second Task-032 skip): a resumed run re-evaluated
// the audit node while sprint legs were still PENDING; the general settle
// path called maybeAutoAdvanceVibeSprintBoundary which armed the boundary
// unconditionally — audit DONE, task stamped, next sprint spawned. The
// CA-1096 evidence gate only covered the blocked_missing_feature_key draft
// branch. Now every advance requires vibeSprintEvidenceComplete plus no
// running sprint step.
func TestVibeSprintBoundaryRefusesUnfinishedLegs(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	now := "2026-10-03T12:00:00Z"
	if _, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-619",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-619",
		RunKind:         "workflow",
		Status:          RunStatusRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: bug616VibeSprintNodes(),
		AutoOrchestrate: true,
		VibeSprintIndex: 2,
		VibeTaskPlan:    []string{"Task-031", "Task-032", "Task-033"},
		LoopState:       AgentLoopState{Status: "running", Round: 15, Cap: 20},
	}); apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	// Sprint-2 coder never ran → its step row is PENDING. The boundary must
	// refuse to advance — pre-fix it stamped audit DONE and armed the
	// next-sprint start.
	if svc.maybeAutoAdvanceVibeSprintBoundary(context.Background(), "run-hub-619", "audit") {
		t.Fatal("boundary advanced with coder PENDING — BUG-619 repro")
	}
	if got := svc.lookupFlowStepStatus("run-hub-619", "audit"); got == StepStatusDone {
		t.Fatalf("audit = %q — boundary stamped DONE on unproven sprint", got)
	}
}
