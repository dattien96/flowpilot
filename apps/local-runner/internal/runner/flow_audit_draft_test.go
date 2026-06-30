package runner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// auditFixture extends fcpFixture by also seeding change-audit/FEATURE-KEYS.md
// inside workspace (which is what featureKeyRegistered checks).
func auditFixture(t *testing.T) (workspace, repoDir string) {
	t.Helper()
	workspace, repoDir = fcpFixture(t)
	caInWorkspace := filepath.Join(workspace, "change-audit")
	if err := os.MkdirAll(caInWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caInWorkspace, "FEATURE-KEYS.md"),
		[]byte("- agent-flow-engine — generic flow engine and review loop (CP-36)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, repoDir
}

// auditDraftInput returns a pre-filled AuditDraftInput for a successful run
// using the workspace seeded by fcpFixture.
func auditDraftInput(workspace string, pkg FlowContextPackage, validationStatus string) AuditDraftInput {
	state := NewFlowValidationRetryState(pkg.PackageID, "go test ./...")
	state.Status = validationStatus
	return AuditDraftInput{
		WorkflowRunID:   "run-171",
		PlanStepID:      "step-plan",
		CodingStepID:    "step-coding",
		TestingStepID:   "step-testing",
		AuditStepID:     "step-audit",
		ContextPackage:  pkg,
		ValidationState: state,
		ChangedFiles:    []string{"runner/foo.go", "runner/bar.go"},
		WhatChanged:     "Added FlowAuditDraft model and builder",
		WhyChanged:      "Closes out CP-41 audit step per Task-171",
		ChangeType:      "feature",
		ResidualNotes:   "Watch for registry-miss edge case",
		Workspace:       workspace,
	}
}

// TestFlowAuditDraftFromSuccessfulRun verifies that a passed validation state
// produces a "ready" draft with all required fields.
func TestFlowAuditDraftFromSuccessfulRun(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine", SourceDocID: "Task-171"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))

	if draft.Status != "ready" {
		t.Fatalf("status = %q, want ready", draft.Status)
	}
	if draft.FeatureKey == "" {
		t.Error("FeatureKey must be set")
	}
	if draft.WhatChanged == "" {
		t.Error("WhatChanged must be set")
	}
	if draft.WhyChanged == "" {
		t.Error("WhyChanged must be set")
	}
	if draft.WorkflowRunID != "run-171" {
		t.Errorf("WorkflowRunID = %q", draft.WorkflowRunID)
	}
	if draft.AuditStepID != "step-audit" {
		t.Errorf("AuditStepID = %q", draft.AuditStepID)
	}
}

// TestFlowAuditDraftIncludesChangeLedgerBlock verifies that the change-ledger
// block follows SS-13 format and contains the feature key.
func TestFlowAuditDraftIncludesChangeLedgerBlock(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine", SourceDocID: "Task-171"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))

	if draft.Status != "ready" {
		t.Fatalf("status = %q, want ready", draft.Status)
	}
	if !strings.Contains(draft.ChangeLedgerBlock, "```flowpilot:change-ledger") {
		t.Error("ChangeLedgerBlock must contain ```flowpilot:change-ledger fence")
	}
	if !strings.Contains(draft.ChangeLedgerBlock, "feature_key:") {
		t.Error("ChangeLedgerBlock must contain feature_key field")
	}
	if !strings.Contains(draft.ChangeLedgerBlock, "source_doc_id: Task-171") {
		t.Errorf("ChangeLedgerBlock must reference Task-171, got:\n%s", draft.ChangeLedgerBlock)
	}
}

// TestFlowAuditDraftCommitMessageUsesKnownFeatureKey verifies that the commit
// message contains the resolved feature key and source doc id.
func TestFlowAuditDraftCommitMessageUsesKnownFeatureKey(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine", SourceDocID: "Task-171"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))

	if draft.Status != "ready" {
		t.Fatalf("status = %q, want ready", draft.Status)
	}
	if !strings.Contains(draft.CommitMessage, "[") {
		t.Error("CommitMessage must follow [Type][feature] bracket format")
	}
	if !strings.Contains(draft.CommitMessage, draft.FeatureKey) {
		t.Errorf("CommitMessage must contain feature key %q, got: %s", draft.FeatureKey, draft.CommitMessage)
	}
	if !strings.Contains(draft.CommitMessage, "Task-171") {
		t.Errorf("CommitMessage must reference source doc Task-171, got: %s", draft.CommitMessage)
	}
}

// TestFlowAuditDraftBlocksMissingFeatureKey verifies that an unresolved
// feature key produces blocked_missing_feature_key status.
func TestFlowAuditDraftBlocksMissingFeatureKey(t *testing.T) {
	workspace, _ := auditFixture(t)
	// Build a package with ConfidenceUnresolved by using a workspace that has
	// no catalog.
	emptyWorkspace := t.TempDir()
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "unknown-feature-xyz"}
	pkg, _ := BuildFlowContextPackage(emptyWorkspace, hints)

	input := auditDraftInput(workspace, pkg, "passed")
	input.Workspace = workspace
	draft := BuildAuditDraft(input)

	if draft.Status != "blocked_missing_feature_key" {
		t.Errorf("status = %q, want blocked_missing_feature_key", draft.Status)
	}
	if draft.CommitMessage != "" {
		t.Error("CommitMessage must be empty when blocked")
	}
	if draft.ChangeLedgerBlock != "" {
		t.Error("ChangeLedgerBlock must be empty when blocked")
	}
}

// TestFlowAuditDraftDoesNotClaimSuccessWhenValidationFailed verifies T-4:
// failed validation cannot produce a "ready" draft.
func TestFlowAuditDraftDoesNotClaimSuccessWhenValidationFailed(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "failed_validation_max_retries"))

	if draft.Status == "ready" {
		t.Error("status must not be ready when validation failed")
	}
	if draft.Status != "blocked_validation_failed" {
		t.Errorf("status = %q, want blocked_validation_failed", draft.Status)
	}
}

// TestFlowAuditDraftIncludesResidualNotes verifies that the caller-supplied
// residual notes appear in the draft.
func TestFlowAuditDraftIncludesResidualNotes(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	input := auditDraftInput(workspace, pkg, "passed")
	input.ResidualNotes = "Watch the registry miss edge case carefully"
	draft := BuildAuditDraft(input)

	if draft.Status != "ready" {
		t.Fatalf("status = %q", draft.Status)
	}
	if draft.ResidualNotes != input.ResidualNotes {
		t.Errorf("ResidualNotes = %q, want %q", draft.ResidualNotes, input.ResidualNotes)
	}
}

// TestFlowAuditDraftDoesNotWriteWithoutApproval verifies that BuildAuditDraft
// does not write any files (T-1). It passes when no write occurs; the test
// monitors the workspace for new/changed files.
func TestFlowAuditDraftDoesNotWriteWithoutApproval(t *testing.T) {
	workspace, _ := auditFixture(t)

	// Count files before.
	before := countFilesInDir(t, workspace)

	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)
	BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))

	after := countFilesInDir(t, workspace)
	if after != before {
		t.Errorf("BuildAuditDraft wrote %d file(s) — must not write without approval", after-before)
	}
}

// TestFlowAuditDraftPersistsAgainstAuditStep verifies that PersistAuditDraft
// emits an EventFlowAuditDraft event keyed to the Audit step ID.
func TestFlowAuditDraftPersistsAgainstAuditStep(t *testing.T) {
	workspace, _ := auditFixture(t)
	store := newFakeWorkflowStore()
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))
	err := PersistAuditDraft(context.Background(), store, "run-171", "step-audit", draft)
	if err != nil {
		t.Fatalf("PersistAuditDraft: %v", err)
	}

	found, ok := FindAuditDraft(store.events["run-171"], "step-audit")
	if !ok {
		t.Fatal("EventFlowAuditDraft not found in store")
	}
	if found.Status != "ready" {
		t.Errorf("found draft status = %q, want ready", found.Status)
	}
}

// TestFlowAuditDraftCarriesAllStepIDs verifies that the draft carries all four
// step IDs (plan, coding, testing, audit) for full traceability (T-2).
func TestFlowAuditDraftCarriesAllStepIDs(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan-A", UserPrompt: "agent-flow-engine", SourceDocID: "Task-171"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	input := auditDraftInput(workspace, pkg, "passed")
	input.PlanStepID = "step-plan-A"
	input.CodingStepID = "step-coding-B"
	input.TestingStepID = "step-testing-C"
	input.AuditStepID = "step-audit-D"
	draft := BuildAuditDraft(input)

	if draft.PlanStepID != "step-plan-A" {
		t.Errorf("PlanStepID = %q", draft.PlanStepID)
	}
	if draft.CodingStepID != "step-coding-B" {
		t.Errorf("CodingStepID = %q", draft.CodingStepID)
	}
	if draft.TestingStepID != "step-testing-C" {
		t.Errorf("TestingStepID = %q", draft.TestingStepID)
	}
	if draft.AuditStepID != "step-audit-D" {
		t.Errorf("AuditStepID = %q", draft.AuditStepID)
	}
	if draft.OriginalPackageID != pkg.PackageID {
		t.Errorf("OriginalPackageID = %q, want %q", draft.OriginalPackageID, pkg.PackageID)
	}
}

// TestFlowAuditDraftScopedToWorkflowRun verifies that FindAuditDraft only
// returns the draft for the matching audit step ID and does not bleed across
// runs or steps.
func TestFlowAuditDraftScopedToWorkflowRun(t *testing.T) {
	workspace, _ := auditFixture(t)

	hints := FlowContextHints{WorkflowRunID: "run-A", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	inputA := auditDraftInput(workspace, pkg, "passed")
	inputA.AuditStepID = "step-audit-A"
	draftA := BuildAuditDraft(inputA)

	inputB := auditDraftInput(workspace, pkg, "passed")
	inputB.AuditStepID = "step-audit-B"
	draftB := BuildAuditDraft(inputB)

	events := []ProviderEvent{
		{Type: EventFlowAuditDraft, WorkflowRunID: "run-A", WorkflowStepRunID: "step-audit-A", FlowAuditDraft: &draftA},
		{Type: EventFlowAuditDraft, WorkflowRunID: "run-A", WorkflowStepRunID: "step-audit-B", FlowAuditDraft: &draftB},
	}

	found, ok := FindAuditDraft(events, "step-audit-A")
	if !ok {
		t.Fatal("expected to find draft for step-audit-A")
	}
	if found.AuditStepID != "step-audit-A" {
		t.Errorf("found wrong draft: AuditStepID = %q", found.AuditStepID)
	}

	// Unknown step must not match.
	_, ok2 := FindAuditDraft(events, "step-audit-Z")
	if ok2 {
		t.Error("lookup for unknown audit step must return false")
	}
}

// TestFlowAuditDraftRenderIsInspectable verifies that the text rendering
// contains the required sections for user inspection before commit (DOD-6).
func TestFlowAuditDraftRenderIsInspectable(t *testing.T) {
	workspace, _ := auditFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-171", PlanStepRunID: "step-plan", UserPrompt: "agent-flow-engine", SourceDocID: "Task-171"}
	pkg, _ := BuildFlowContextPackage(workspace, hints)

	draft := BuildAuditDraft(auditDraftInput(workspace, pkg, "passed"))
	rendered := RenderAuditDraftText(draft)

	for _, want := range []string{
		"# Flow Audit Draft",
		"**Status**:",
		"**Feature key**:",
		"## Suggested Commit Message",
		"## Change Ledger Block",
		"```flowpilot:change-ledger",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

// countFilesInDir walks dir and returns the total regular-file count.
// Used by TestFlowAuditDraftDoesNotWriteWithoutApproval to detect unwanted writes.
func countFilesInDir(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // ignore walk errors
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Logf("countFilesInDir walk error (ignored): %v", err)
	}
	return count
}
