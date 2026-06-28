package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

func TestValidationRetryPromptIncludesOriginalContextPackage(t *testing.T) {
	pkg := FlowContextPackage{
		PackageID:         "pkg-1",
		WorkflowRunID:     "run-1",
		PlanStepRunID:     "plan-1",
		FeatureKey:        "context-regression-engine",
		FeatureConfidence: FlowContextConfidenceVerified,
		Sections: []FlowContextSection{
			{Title: "History", Body: "## Prior work\n- entry"},
		},
	}
	result := FlowValidationResult{
		ValidationCommand: "go test ./...",
		StdoutSummary:     "ok",
		StderrSummary:     "compiler error: cannot build",
		ExitCode:          1,
	}
	prompt := buildValidationRetryPrompt(pkg, result)
	if strings.Count(prompt, "## Flow Context Package") != 1 {
		t.Fatalf("retry prompt should include original package once: %q", prompt)
	}
	if !strings.Contains(prompt, "Validation Feedback") || !strings.Contains(prompt, "compiler error") {
		t.Fatalf("retry prompt missing validation feedback: %q", prompt)
	}
	if !strings.Contains(prompt, "Keep the original Flow Context Package unchanged.") {
		t.Fatalf("retry prompt missing no-mutation instruction: %q", prompt)
	}
}

func TestSummarizeFlowValidationOutputIsBounded(t *testing.T) {
	text := strings.Repeat("line\n", 40)
	summary := summarizeFlowValidationOutput("go test ./...", text, text, 1)
	if strings.Count(summary, "line") > 25 {
		t.Fatalf("summary not bounded: %q", summary)
	}
}

func TestLoadFlowModeConfigDefaultsWhenMissing(t *testing.T) {
	dotFP := filepath.Join(t.TempDir(), ".flowpilot")
	cfg, err := loadFlowModeConfig(dotFP)
	if err != nil {
		t.Fatalf("loadFlowModeConfig: %v", err)
	}
	if cfg.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.MaxContextPackageBytes != flowContextDefaultMaxBytes {
		t.Fatalf("MaxContextPackageBytes = %d, want %d", cfg.MaxContextPackageBytes, flowContextDefaultMaxBytes)
	}
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", flowModeConfigFileName), []byte(`{"validationCommand":"go test ./...","maxRetries":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadFlowModeConfig(dotFP)
	if err != nil {
		t.Fatalf("loadFlowModeConfig after write: %v", err)
	}
	if cfg.ValidationCommand != "go test ./..." || cfg.MaxRetries != 3 {
		t.Fatalf("unexpected cfg after write: %+v", cfg)
	}
}

func TestValidationRetryReEntersTestingStep(t *testing.T) {
	workspace := t.TempDir()
	dotFP := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", flowModeConfigFileName), []byte(`{"validationCommand":"exit 1","maxRetries":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newFakeWorkflowStore()
	store.seed("run-1", []RuntimeWorkflowStep{
		step("coding-1", "coding", StepStatusDone, false),
		step("testing-1", "testing", StepStatusDone, false),
		step("audit-1", "audit", StepStatusPending, false),
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	rs := &interactiveRun{id: "run-1", stepID: "testing-1", workspaceCwd: workspace, projectID: "proj-1", lastTurnID: "coding-turn-1"}
	blocked, handled := svc.maybeRunFlowValidation(context.Background(), rs, finalizeInput{RunID: "run-1", TurnID: "testing-turn-1", FinalMessage: "done"}, dotFP)
	if !blocked || !handled {
		t.Fatalf("expected validation failure to route into retry, got blocked=%v handled=%v", blocked, handled)
	}
	steps, err := store.LoadRunSteps(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if steps[1].Status != StepStatusPending || steps[1].RetryCount != 1 {
		t.Fatalf("testing step was not reset for retry: %+v", steps[1])
	}
	if steps[0].Status != StepStatusDone {
		t.Fatalf("coding step should stay done: %+v", steps[0])
	}
}

func TestValidationRetryStopsAtMaxRetries(t *testing.T) {
	workspace := t.TempDir()
	dotFP := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", flowModeConfigFileName), []byte(`{"validationCommand":"exit 1","maxRetries":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newFakeWorkflowStore()
	store.seed("run-1", []RuntimeWorkflowStep{
		step("coding-1", "coding", StepStatusDone, false),
		step("testing-1", "testing", StepStatusDone, false),
		step("audit-1", "audit", StepStatusPending, false),
	})
	steps, err := store.LoadRunSteps(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	steps[1].RetryCount = 1
	if err := store.ApplyStepTransition(context.Background(), "run-1", WorkflowStepTransition{
		StepID: "testing-1",
		Patch:  WorkflowStepPatch{Status: StepStatusDone, RetryCount: intptr(1)},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	rs := &interactiveRun{id: "run-1", stepID: "testing-1", workspaceCwd: workspace, projectID: "proj-1", lastTurnID: "coding-turn-1"}
	blocked, handled := svc.maybeRunFlowValidation(context.Background(), rs, finalizeInput{RunID: "run-1", TurnID: "testing-turn-1", FinalMessage: "done"}, dotFP)
	if !handled || blocked {
		t.Fatalf("expected validation failure without retry, got blocked=%v handled=%v", blocked, handled)
	}
	steps, err = store.LoadRunSteps(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if steps[1].RetryCount != 1 || steps[1].Status != StepStatusDone {
		t.Fatalf("testing step should remain at max retries: %+v", steps[1])
	}
}

func TestValidationAuditDraftUsesCodingTurnArtifact(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", flowModeConfigFileName), []byte(`{"validationCommand":"true","maxRetries":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), "- context-regression-engine — Context Regression Engine\n")
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "context-regression-engine", Summary: "seed", CommittedAt: "2026-06-28T00:00:00Z", Confidence: changeledger.ConfidenceHigh}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, ledger, dotFP); err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildFlowContextPackage(workspace, "context-regression-engine", nil, FlowContextHints{
		WorkflowRunID: "run-1",
		PlanStepRunID: "plan-1",
		Prompt:        "context-regression-engine",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := New(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.SaveFlowContextPackageArtifact("proj-1", pkg); err != nil {
		t.Fatal(err)
	}
	store := newFakeWorkflowStore()
	store.seed("run-1", []RuntimeWorkflowStep{
		step("coding-1", "coding", StepStatusDone, false),
		step("testing-1", "testing", StepStatusDone, false),
		step("audit-1", "audit", StepStatusPending, false),
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	svc.runner = runner
	svc.finalizer = newFinalizer()
	if err := svc.finalizer.Finalize(finalizeInput{RunID: "run-1", TurnID: "coding-turn-1", FinalMessage: "coding done", ChangedFiles: []string{"src/coding.go", "src/shared.go"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.finalizer.Finalize(finalizeInput{RunID: "run-1", TurnID: "testing-turn-1", FinalMessage: "testing done", ChangedFiles: []string{"tests/testing_only.go"}}); err != nil {
		t.Fatal(err)
	}
	rs := &interactiveRun{id: "run-1", stepID: "testing-1", workspaceCwd: workspace, projectID: "proj-1", lastTurnID: "coding-turn-1"}
	blocked, handled := svc.maybeRunFlowValidation(context.Background(), rs, finalizeInput{RunID: "run-1", TurnID: "testing-turn-1", FinalMessage: "done"}, dotFP)
	if blocked || !handled {
		t.Fatalf("expected validation pass to be handled without blocking, got blocked=%v handled=%v", blocked, handled)
	}
	artifacts, err := runner.ListArtifacts()
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	var artifact ArtifactDetail
	found := false
	for _, a := range artifacts {
		if a.SourceKind == artifactSourceFlowAuditDraft && a.WorkflowRunID == "run-1" {
			artifact = a
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("did not find flow audit draft artifact: %+v", artifacts)
	}
	if !strings.Contains(artifact.ContentMarkdown, "src/coding.go") || !strings.Contains(artifact.ContentMarkdown, "src/shared.go") {
		t.Fatalf("audit draft missing coding changed files: %q", artifact.ContentMarkdown)
	}
	if strings.Contains(artifact.ContentMarkdown, "tests/testing_only.go") {
		t.Fatalf("audit draft picked testing changed files: %q", artifact.ContentMarkdown)
	}
}
