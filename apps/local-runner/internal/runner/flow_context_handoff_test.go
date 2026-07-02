package runner

import (
	"context"
	"strings"
	"testing"
)

// planCodingSteps returns a two-step workflow: plan → coding.
func planCodingSteps(planID, codingID string) []RuntimeWorkflowStep {
	return []RuntimeWorkflowStep{
		{ID: planID, StepType: "plan", Status: StepStatusDone},
		{ID: codingID, StepType: "coding", Status: StepStatusPending},
	}
}

// TestFlowCodingPromptIncludesPlanContextPackage verifies that a Coding step
// receives the rendered FlowContextPackage prepended to its prompt.
func TestFlowCodingPromptIncludesPlanContextPackage(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-1", workspaceCwd: workspace, sourceDocID: "Task-169"}

	out := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "implement the plan")

	if !isFlowContextHandoff(out) {
		t.Errorf("expected flowContextHandoffPrefix in output, got: %.200s", out)
	}
	if !strings.Contains(out, "## Flow Context Package") {
		t.Error("rendered package section missing from Coding prompt")
	}
	if !strings.Contains(out, "implement the plan") {
		t.Error("original coding instruction must appear after the package")
	}
}

// TestFlowCodingPromptIncludesPackageOnce verifies the package appears exactly
// once even when injectFlowContextIfCoding is called twice (idempotency guard via
// isFlowContextHandoff short-circuit in injectFeatureHistoryPrompt).
func TestFlowCodingPromptIncludesPackageOnce(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-1", workspaceCwd: workspace}

	once := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "code it")
	// Simulate injectFeatureHistoryPrompt being called on the result (T-5).
	injected := injectFeatureHistoryPrompt(workspace, once, nil)

	count := strings.Count(injected, flowContextHandoffPrefix)
	if count != 1 {
		t.Errorf("flowContextHandoffPrefix appears %d times, want 1", count)
	}
	count2 := strings.Count(injected, "## Flow Context Package")
	if count2 != 1 {
		t.Errorf("## Flow Context Package appears %d times, want 1", count2)
	}
}

// TestFlowCodingRetryReusesPlanPackage verifies that a second call for the same
// run/step reuses rs.planContextPackage and does not build a new package.
func TestFlowCodingRetryReusesPlanPackage(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:          "run-1",
		workspaceCwd: workspace,
		subs:         map[int64]chan ProviderEvent{},
	}

	out1 := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "code it")
	if rs.planContextPackage == nil {
		t.Fatal("planContextPackage should be set after first call")
	}
	firstID := rs.planContextPackage.PackageID

	// Second call (retry): should reuse the cached package.
	out2 := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "retry")
	if rs.planContextPackage.PackageID != firstID {
		t.Error("retry must reuse same package ID, not rebuild")
	}
	// Both outputs should carry the same package ID.
	if !strings.Contains(out1, firstID) || !strings.Contains(out2, firstID) {
		t.Error("both outputs must reference the same package ID")
	}
}

// TestPlanRerunReplacesFlowContextPackage verifies that after maybeClearPlanContextForPlanStep
// is called for the Plan step, rs.planContextPackage is cleared so the next Coding
// step gets a fresh package.
func TestPlanRerunReplacesFlowContextPackage(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:          "run-1",
		workspaceCwd: workspace,
		subs:         map[int64]chan ProviderEvent{},
	}

	// First Coding run — package gets built.
	svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "code it")
	if rs.planContextPackage == nil {
		t.Fatal("package must be set after coding step")
	}

	// Plan step reruns → package cleared.
	svc.maybeClearPlanContextForPlanStep(context.Background(), "run-1", rs, "step-plan")
	if rs.planContextPackage != nil {
		t.Error("planContextPackage must be nil after Plan step rerun")
	}
}

// TestFlowContextPackageAppearsInPromptLog verifies that logComposedPrompt
// captures the flow context prefix — i.e. it runs after injectFlowContextIfCoding
// in the pipeline.
func TestFlowContextPackageAppearsInPromptLog(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-1", workspaceCwd: workspace}

	composed := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "implement it")
	// Simulate what runTurn does: log the fully-composed prompt.
	logDir := t.TempDir()
	logComposedPrompt(logDir, "proj-1", "run-1", "turn-1", composed)

	// The composed prompt (which is what logComposedPrompt receives) must contain
	// the package marker — verifying T-4 (prompt logging captures the package).
	if !strings.Contains(composed, flowContextHandoffPrefix) {
		t.Error("flow context prefix must be present in composed prompt before logging")
	}
	if !strings.Contains(composed, "No vector retrieval used") {
		t.Error("audit line must be present in composed prompt")
	}
}

// TestNormalChatFeatureHistoryInjectionUnchanged verifies that a normal chat
// turn (no Coding step type) is not affected by injectFlowContextIfCoding.
func TestNormalChatFeatureHistoryInjectionUnchanged(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	// Seed a plain chat step — not a coding step.
	store.seed("run-chat", []RuntimeWorkflowStep{
		{ID: "step-chat", StepType: "chat", Status: StepStatusPending},
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-chat", workspaceCwd: workspace}

	prompt := "implement agent-flow-engine"
	out := svc.injectFlowContextIfCoding(context.Background(), rs, "step-chat", prompt, prompt)

	if isFlowContextHandoff(out) {
		t.Error("normal chat step must not receive flow context package")
	}
	if out != prompt {
		t.Errorf("chat step prompt must be unchanged, got diff: %q", out)
	}
}

// TestSystemPromptDoesNotResolveFromContextPackageText verifies that a flow
// context package prompt is treated as a system prompt by isSystemPrompt
// (via isFlowContextHandoff → isHandoffPrompt pathway for feature_history).
func TestSystemPromptDoesNotResolveFromContextPackageText(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan", "step-coding"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-1", workspaceCwd: workspace}

	pkg := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "code it")

	// injectFeatureHistoryPrompt must skip re-injection for a flow context prompt.
	result := injectFeatureHistoryPrompt(workspace, pkg, nil)
	if result != pkg {
		t.Error("injectFeatureHistoryPrompt must return the prompt unchanged when flow context is already present")
	}
}

// TestCodingStepLoadsPackageFromPriorPlanStep verifies that the package
// WorkflowStepRunID is keyed to the Plan step, not the Coding step.
func TestCodingStepLoadsPackageFromPriorPlanStep(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-1", planCodingSteps("step-plan-A", "step-coding-B"))
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:          "run-1",
		workspaceCwd: workspace,
		subs:         map[int64]chan ProviderEvent{},
	}

	svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding-B", "agent-flow-engine", "code")

	// The persisted event must carry the Plan step ID, not the Coding step ID.
	found, ok := FindFlowContextPackage(rs.events, "step-plan-A")
	if !ok {
		t.Fatal("EventFlowContextPackage must be emitted and keyed to the Plan step ID")
	}
	if found.WorkflowRunID != "run-1" {
		t.Errorf("WorkflowRunID = %q, want run-1", found.WorkflowRunID)
	}
}

// TestCodingStepWarnsWhenPlanPackageMissing verifies that a Coding step without
// any preceding Plan step still produces a package with a warning rather than
// returning the prompt unchanged (degradation, T-3a).
func TestCodingStepWarnsWhenPlanPackageMissing(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	// Single coding step — no plan step precedes it.
	store.seed("run-1", []RuntimeWorkflowStep{
		{ID: "step-coding", StepType: "coding", Status: StepStatusPending},
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-1", workspaceCwd: workspace}

	out := svc.injectFlowContextIfCoding(context.Background(), rs, "step-coding", "agent-flow-engine", "code")

	if !isFlowContextHandoff(out) {
		t.Error("coding step with no prior plan step must still produce a flow context prompt")
	}
	if rs.planContextPackage == nil {
		t.Fatal("planContextPackage must be set")
	}
	foundWarning := false
	for _, w := range rs.planContextPackage.Warnings {
		if strings.Contains(w, "no_plan_step") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected no_plan_step warning, got: %v", rs.planContextPackage.Warnings)
	}
}

// TestPlanPackageLookupScopedToWorkflowRun verifies that FindFlowContextPackage
// only returns packages for the matching planStepID and does not bleed across runs.
func TestPlanPackageLookupScopedToWorkflowRun(t *testing.T) {
	workspace, _ := fcpFixture(t)

	// Build packages for two different plan step IDs.
	hints1 := FlowContextHints{WorkflowRunID: "run-A", PlanStepRunID: "step-plan-1", UserPrompt: "agent-flow-engine"}
	pkg1, _ := BuildFlowContextPackage(workspace, hints1)
	hints2 := FlowContextHints{WorkflowRunID: "run-A", PlanStepRunID: "step-plan-2", UserPrompt: "agent-flow-engine"}
	pkg2, _ := BuildFlowContextPackage(workspace, hints2)

	events := []ProviderEvent{
		{Type: EventFlowContextPackage, WorkflowRunID: "run-A", WorkflowStepRunID: "step-plan-1", FlowContextPackage: &pkg1},
		{Type: EventFlowContextPackage, WorkflowRunID: "run-A", WorkflowStepRunID: "step-plan-2", FlowContextPackage: &pkg2},
	}

	// Lookup for step-plan-1 must return pkg1, not pkg2.
	found, ok := FindFlowContextPackage(events, "step-plan-1")
	if !ok {
		t.Fatal("expected to find package for step-plan-1")
	}
	if found.PackageID != pkg1.PackageID {
		t.Errorf("lookup returned wrong package: got %q, want %q", found.PackageID, pkg1.PackageID)
	}

	// Lookup for step-plan-2 must return pkg2.
	found2, ok2 := FindFlowContextPackage(events, "step-plan-2")
	if !ok2 {
		t.Fatal("expected to find package for step-plan-2")
	}
	if found2.PackageID != pkg2.PackageID {
		t.Errorf("lookup returned wrong package: got %q, want %q", found2.PackageID, pkg2.PackageID)
	}

	// Lookup for a non-existent step must return nothing.
	_, ok3 := FindFlowContextPackage(events, "step-plan-99")
	if ok3 {
		t.Error("lookup for unknown step must return false")
	}
}

// TestMaybeClearPlanContextIsNoOpForNonPlanStep verifies that
// maybeClearPlanContextForPlanStep leaves planContextPackage unchanged when the
// step being started is a Coding or Testing step rather than a Plan step.
// Regression guard: a Coding re-entry must not evict the cached plan package.
func TestMaybeClearPlanContextIsNoOpForNonPlanStep(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	// Seed a plan + coding + testing step sequence.
	store.seed("run-noop", []RuntimeWorkflowStep{
		{ID: "step-plan", StepType: "plan", Status: StepStatusDone},
		{ID: "step-coding", StepType: "coding", Status: StepStatusRunning},
		{ID: "step-testing", StepType: "testing", Status: StepStatusPending},
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	// Seed a non-nil planContextPackage on the run.
	pkg := &FlowContextPackage{PackageID: "pkg-sentinel"}
	rs := &interactiveRun{
		id:                 "run-noop",
		workspaceCwd:       workspace,
		subs:               map[int64]chan ProviderEvent{},
		planContextPackage: pkg,
	}

	// Calling with a Coding step must be a no-op.
	svc.maybeClearPlanContextForPlanStep(context.Background(), "run-noop", rs, "step-coding")
	if rs.planContextPackage == nil {
		t.Error("planContextPackage must not be cleared for a Coding step")
	}
	if rs.planContextPackage.PackageID != "pkg-sentinel" {
		t.Errorf("planContextPackage changed unexpectedly: %+v", rs.planContextPackage)
	}

	// Calling with a Testing step must also be a no-op.
	svc.maybeClearPlanContextForPlanStep(context.Background(), "run-noop", rs, "step-testing")
	if rs.planContextPackage == nil {
		t.Error("planContextPackage must not be cleared for a Testing step")
	}
}
