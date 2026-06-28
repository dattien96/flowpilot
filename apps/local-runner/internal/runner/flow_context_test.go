package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

func TestBuildFlowContextPackageIncludesHistoryDiscussionAndExcerpt(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
	mustWrite(t, filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), "- context-regression-engine — Context Regression Engine\n")
	mustWrite(t, filepath.Join(repoDir, "internal", "runner", "sample.go"), "package runner\n\nconst sample = 1\n")

	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Upsert([]changeledger.Entry{{
		CommitHash:  "c1",
		FeatureKey:  "context-regression-engine",
		SourceDocID: "Task-168",
		ChangeType:  "feature",
		Summary:     "seed history",
		CommittedAt: "2026-06-28T00:00:00Z",
		Confidence:  changeledger.ConfidenceHigh,
	}}); err != nil {
		t.Fatal(err)
	}
	summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := summaryLedger.Append([]changeledger.ChatSummaryEntry{{
		RunID:      "run-1",
		TurnID:     "turn-1",
		FeatureKey: "context-regression-engine",
		Summary:    "discussion summary",
		CreatedAt:  "2026-06-28T00:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, ledger, dotFP); err != nil {
		t.Fatal(err)
	}

	pkg, err := BuildFlowContextPackage(workspace, "context-regression-engine", []transcriptTurn{{User: "context-regression-engine"}}, FlowContextHints{
		WorkflowRunID:       "run-1",
		PlanStepRunID:       "plan-1",
		Prompt:              "context-regression-engine",
		SourceDocID:         "Task-168",
		ExplicitSourcePaths: []string{"internal/runner/sample.go"},
		MaxBytes:            8 * 1024,
	})
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if pkg.FeatureKey != "context-regression-engine" {
		t.Fatalf("feature key = %q", pkg.FeatureKey)
	}
	if pkg.FeatureConfidence != FlowContextConfidenceVerified {
		t.Fatalf("feature confidence = %q", pkg.FeatureConfidence)
	}
	if pkg.HistoryBlock == "" || pkg.DiscussionBlock == "" {
		t.Fatalf("expected history + discussion blocks, got %+v", pkg)
	}
	if len(pkg.SourceExcerpts) != 1 {
		t.Fatalf("source excerpts = %+v", pkg.SourceExcerpts)
	}
	if !strings.Contains(pkg.SourceExcerpts[0].IncludedText, "const sample = 1") {
		t.Fatalf("excerpt missing file content: %+v", pkg.SourceExcerpts[0])
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "## Flow Context Package") || !strings.Contains(rendered, "No vector retrieval used.") {
		t.Fatalf("rendered package missing required sections: %s", rendered)
	}
}

func TestBuildFlowContextPackageRejectsOutsideWorkspacePath(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
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
		WorkflowRunID:       "run-1",
		PlanStepRunID:       "plan-1",
		Prompt:              "context-regression-engine",
		ExplicitSourcePaths: []string{"/etc/passwd"},
	})
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if len(pkg.Omitted) == 0 || !strings.Contains(strings.Join(pkg.Omitted, " "), "outside_workspace") {
		t.Fatalf("expected outside_workspace omission, got %+v", pkg.Omitted)
	}
}

func TestBuildFlowContextPackageDoesNotInheritOnAmbiguousPrompt(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
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

	pkg, err := BuildFlowContextPackage(workspace, "write a haiku about the sea", []transcriptTurn{{User: "context-regression-engine"}}, FlowContextHints{
		WorkflowRunID: "run-1",
		PlanStepRunID: "plan-1",
		Prompt:        "write a haiku about the sea",
	})
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if pkg.FeatureKey != "" {
		t.Fatalf("ambiguous prompt should not inherit prior history, got feature %q", pkg.FeatureKey)
	}
}

func TestRenderFlowContextPackageIsBoundedByMaxBytes(t *testing.T) {
	pkg := FlowContextPackage{
		PackageID:         "pkg-1",
		WorkflowRunID:     "run-1",
		PlanStepRunID:     "plan-1",
		FeatureKey:        "context-regression-engine",
		FeatureConfidence: FlowContextConfidenceVerified,
		MaxBytes:          160,
		SourceRefs:        []FlowContextSourceRef{{Kind: "feature", FeatureKey: "context-regression-engine"}},
		HistoryBlock:      strings.Repeat("history\n", 12),
		DiscussionBlock:   strings.Repeat("discussion\n", 12),
		SourceExcerpts:    []FlowContextExcerpt{{Path: "internal/runner/sample.go", IncludedText: strings.Repeat("x", 120)}},
		Constraints:       []string{strings.Repeat("constraint ", 12)},
		Warnings:          []string{strings.Repeat("warning ", 12)},
		Omitted:           []string{strings.Repeat("omitted ", 12)},
	}
	rendered := RenderFlowContextPackage(pkg)
	if len(rendered) > pkg.MaxBytes {
		t.Fatalf("rendered package exceeded max bytes: got %d want <= %d", len(rendered), pkg.MaxBytes)
	}
	if !strings.HasSuffix(rendered, "...[truncated]") {
		t.Fatalf("rendered package should be truncated, got %q", rendered)
	}
}

func TestFlowContextPackagePersistsAndReloads(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
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
	svc := NewInteractiveService()
	svc.runner = runner
	loaded, ok := svc.loadLatestFlowContextPackage("run-1")
	if !ok {
		t.Fatal("expected package to reload from artifact")
	}
	if loaded.PackageID != pkg.PackageID || loaded.FeatureKey != pkg.FeatureKey {
		t.Fatalf("reloaded package = %+v, want %+v", loaded, pkg)
	}
}

func TestCodingPromptIncludesFlowContextOnce(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
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
		step("plan-1", "plan", StepStatusDone, false),
		step("coding-1", "coding", StepStatusPending, false),
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	svc.runner = runner
	rs := &interactiveRun{id: "run-1", workflowID: "wf-1", workspaceCwd: workspace}
	got := svc.injectFlowContextPrompt(rs, TurnInput{StepID: "coding-1", Prompt: "implement the plan"}, "implement the plan")
	if strings.Count(got, "## Flow Context Package") != 1 {
		t.Fatalf("coding prompt should include one flow context block, got %q", got)
	}
	if !strings.Contains(got, "Use the Flow Context Package below as the source of truth") {
		t.Fatalf("coding prompt missing handoff instruction: %q", got)
	}
}

func TestCodingPromptUsesFlowContextPackageWhenMissingLegacyHistory(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	dotFP := filepath.Join(workspace, ".flowpilot")
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
		step("plan-1", "plan", StepStatusDone, false),
		step("coding-1", "coding", StepStatusPending, false),
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	svc.runner = runner
	rs := &interactiveRun{id: "run-1", workflowID: "wf-1", workspaceCwd: workspace}
	got := svc.injectFlowContextPrompt(rs, TurnInput{StepID: "coding-1", Prompt: "implement the plan"}, "implement the plan")
	if strings.Count(got, "## Flow Context Package") != 1 {
		t.Fatalf("coding prompt should include one flow context block, got %q", got)
	}
	if strings.Contains(got, "legacy feature history") {
		t.Fatalf("coding prompt unexpectedly included legacy history: %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
