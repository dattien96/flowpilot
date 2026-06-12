package runner

import "testing"

const smNow = "2026-06-12T10:00:00Z"

func step(id, stepType string, status RuntimeWorkflowStepStatus, requiresApproval bool) RuntimeWorkflowStep {
	return RuntimeWorkflowStep{ID: id, StepType: stepType, Status: status, RequiresApproval: requiresApproval}
}

func TestPlanWorkflowProgress_CompletesNonApprovalSteps(t *testing.T) {
	plan := PlanWorkflowProgress([]RuntimeWorkflowStep{
		step("s1", "plan", StepStatusPending, false),
		step("s2", "build", StepStatusPending, false),
	}, false, smNow)

	if plan.RunStatus != RunStatusEngineDone || plan.RunFinishedAt != smNow || plan.PausedStepID != "" {
		t.Fatalf("plan = %+v, want DONE finished now no pause", plan)
	}
	if len(plan.StepTransitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(plan.StepTransitions))
	}
	for _, tr := range plan.StepTransitions {
		if tr.Patch.Status != StepStatusDone || tr.Patch.FinishedAt == nil || *tr.Patch.FinishedAt != smNow {
			t.Fatalf("transition %+v not DONE@now", tr)
		}
	}
}

func TestPlanWorkflowProgress_PausesOnApprovalGate(t *testing.T) {
	plan := PlanWorkflowProgress([]RuntimeWorkflowStep{
		step("s1", "plan", StepStatusDone, false),
		step("s2", "build", StepStatusPending, true),
		step("s3", "ship", StepStatusPending, false),
	}, false, smNow)

	if plan.RunStatus != RunStatusEngineRunning || plan.PausedStepID != "s2" {
		t.Fatalf("plan = %+v, want RUNNING paused on s2", plan)
	}
	// s1 (DONE) skipped, s2 → WAITING, return before s3
	if len(plan.StepTransitions) != 1 {
		t.Fatalf("transitions = %d, want 1 (s2 waiting)", len(plan.StepTransitions))
	}
	tr := plan.StepTransitions[0]
	if tr.StepID != "s2" || tr.Patch.Status != StepStatusWaitingUserApr {
		t.Fatalf("transition %+v, want s2 WAITING_USER_APPROVAL", tr)
	}
	if tr.Patch.FinishedAt == nil || *tr.Patch.FinishedAt != "" {
		t.Fatalf("waiting step finishedAt should be null (ptr to \"\"), got %v", tr.Patch.FinishedAt)
	}
}

func TestPlanWorkflowProgress_YoloAutoApprovesApprovalGate(t *testing.T) {
	plan := PlanWorkflowProgress([]RuntimeWorkflowStep{
		step("s1", "build", StepStatusPending, true),
	}, true, smNow)

	if plan.RunStatus != RunStatusEngineDone || plan.PausedStepID != "" {
		t.Fatalf("plan = %+v, want DONE no pause under YOLO", plan)
	}
	tr := plan.StepTransitions[0]
	if tr.Patch.Status != StepStatusDone {
		t.Fatalf("step should be DONE under YOLO, got %s", tr.Patch.Status)
	}
	if len(tr.Logs) != 1 || tr.Logs[0].Message != "YOLO mode auto-approved step build." {
		t.Fatalf("log = %+v, want YOLO auto-approve message", tr.Logs)
	}
}

func TestPlanWorkflowProgress_WaitingStepPausesWithoutYolo(t *testing.T) {
	plan := PlanWorkflowProgress([]RuntimeWorkflowStep{
		step("s1", "review", StepStatusWaitingUserApr, true),
	}, false, smNow)

	if plan.RunStatus != RunStatusEngineRunning || plan.PausedStepID != "s1" {
		t.Fatalf("plan = %+v, want RUNNING paused on s1", plan)
	}
	if len(plan.StepTransitions) != 0 {
		t.Fatalf("a waiting step without YOLO emits no transition, got %d", len(plan.StepTransitions))
	}
}

func TestPlanWorkflowProgress_WaitingStepAutoApprovedWithYolo(t *testing.T) {
	plan := PlanWorkflowProgress([]RuntimeWorkflowStep{
		step("s1", "review", StepStatusWaitingUserApr, true),
		step("s2", "ship", StepStatusPending, false),
	}, true, smNow)

	if plan.RunStatus != RunStatusEngineDone {
		t.Fatalf("plan = %+v, want DONE", plan)
	}
	if len(plan.StepTransitions) != 2 {
		t.Fatalf("transitions = %d, want 2 (waiting auto-approved + pending done)", len(plan.StepTransitions))
	}
}

func TestPlanRejectedStepRetry(t *testing.T) {
	tr := PlanRejectedStepRetry("s1", 1, smNow, "needs work")
	if tr.Patch.Status != StepStatusPending {
		t.Fatalf("status = %s, want PENDING", tr.Patch.Status)
	}
	if tr.Patch.RetryCount == nil || *tr.Patch.RetryCount != 2 {
		t.Fatalf("retryCount = %v, want 2", tr.Patch.RetryCount)
	}
	if tr.Patch.RejectionNote == nil || *tr.Patch.RejectionNote != "needs work" {
		t.Fatalf("rejectionNote = %v", tr.Patch.RejectionNote)
	}
	if len(tr.Logs) != 1 || tr.Logs[0].LogLevel != LogWarn {
		t.Fatalf("log = %+v, want one warn log", tr.Logs)
	}
}
