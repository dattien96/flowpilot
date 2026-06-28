package runner

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFlowAuditDraftIncludesChangeLedgerBlockAndCommitMessage(t *testing.T) {
	workspace := t.TempDir()
	repoDir := workspace
	mustWrite(t, filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), "- context-regression-engine — Context Regression Engine\n")

	pkg := FlowContextPackage{
		WorkflowRunID:   "run-1",
		PlanStepRunID:   "plan-1",
		FeatureKey:      "context-regression-engine",
		SourceDocIDs:    []string{"Task-168"},
		DiscussionBlock: "## Prior discussion\n- keep the plan deterministic",
	}
	draft := BuildFlowAuditDraft(repoDir, pkg, "coding-1", "testing-1", "audit-1", []string{"internal/runner/sample.go"}, []string{"go test ./..."}, []string{"no residuals"}, "Validation passed.", "feature", "implement flow harness")
	if draft.Status != "ready" {
		t.Fatalf("status = %q, want ready", draft.Status)
	}
	if !strings.Contains(draft.ChangeLedgerBlock, "flowpilot:change-ledger") {
		t.Fatalf("ledger block missing: %q", draft.ChangeLedgerBlock)
	}
	if !strings.Contains(draft.CommitMessage, "[Feature][context-regression-engine]") {
		t.Fatalf("commit message missing feature key: %q", draft.CommitMessage)
	}
	if !strings.Contains(draft.CommitMessage, "Task-168") {
		t.Fatalf("commit message missing source doc id: %q", draft.CommitMessage)
	}
	rendered := RenderFlowAuditDraft(draft)
	if !strings.Contains(rendered, "## Flow Audit Draft") || !strings.Contains(rendered, "Validation passed.") {
		t.Fatalf("rendered audit draft missing sections: %q", rendered)
	}
}

func TestFlowAuditDraftBlocksMissingFeatureKey(t *testing.T) {
	workspace := t.TempDir()
	pkg := FlowContextPackage{WorkflowRunID: "run-1", PlanStepRunID: "plan-1", FeatureKey: "missing-feature", SourceDocIDs: []string{"Task-168"}}
	draft := BuildFlowAuditDraft(workspace, pkg, "coding-1", "testing-1", "audit-1", nil, nil, nil, "Validation passed.", "feature", "implement flow harness")
	if draft.Status != "blocked_missing_feature_key" {
		t.Fatalf("status = %q, want blocked_missing_feature_key", draft.Status)
	}
}
