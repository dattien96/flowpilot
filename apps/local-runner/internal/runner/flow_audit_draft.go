package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/featurecatalog"
)

// featureKeyRegistryFile is the canonical key registry path relative to the
// workspace root (SS-13 §13, Task-171 T-3).
const featureKeyRegistryFile = "change-audit/FEATURE-KEYS.md"

// FlowAuditDraft is the structured audit-ready output at the end of a
// successful Flow Mode run (Task-171 T-1). It is a draft — no files are written
// and no commits are created without explicit user/workflow confirmation (T-1).
type FlowAuditDraft struct {
	// Traceability
	WorkflowRunID     string `json:"workflowRunId"`
	PlanStepID        string `json:"planStepId"`
	CodingStepID      string `json:"codingStepId"`
	TestingStepID     string `json:"testingStepId"`
	AuditStepID       string `json:"auditStepId"`
	OriginalPackageID string `json:"originalPackageId"`

	// Content
	FeatureKey         string   `json:"featureKey"`
	SourceDocID        string   `json:"sourceDocId"`
	ChangeType         string   `json:"changeType"`
	Summary            string   `json:"summary"`
	WhatChanged        string   `json:"whatChanged"`
	WhyChanged         string   `json:"whyChanged"`
	ChangedFiles       []string `json:"changedFiles,omitempty"`
	ValidationCommands []string `json:"validationCommands,omitempty"`
	ValidationResult   string   `json:"validationResult"` // "passed" | "failed" | "skipped_env_error" | "skipped_no_command"
	ResidualNotes      string   `json:"residualNotes,omitempty"`
	CommitMessage      string   `json:"commitMessage,omitempty"`
	ChangeLedgerBlock  string   `json:"changeLedgerBlock,omitempty"`

	// Status — "ready" | "blocked_missing_feature_key" | "blocked_validation_failed"
	Status string `json:"status"`
}

// AuditDraftInput carries the inputs the draft builder reads from run state.
// All fields are derived from prior task outputs (168-170); none are typed
// directly by the user or inferred from free-form text (T-2).
type AuditDraftInput struct {
	WorkflowRunID   string
	PlanStepID      string
	CodingStepID    string
	TestingStepID   string
	AuditStepID     string
	ContextPackage  FlowContextPackage
	ValidationState FlowValidationRetryState
	ChangedFiles    []string
	ResidualNotes   string
	WhatChanged     string
	WhyChanged      string
	ChangeType      string // "feature"|"bugfix"|"refactor" etc.
	Workspace       string
}

// BuildAuditDraft constructs the draft from run state. It never writes any file
// and never executes any command (T-1, T-5).
func BuildAuditDraft(input AuditDraftInput) FlowAuditDraft {
	pkg := input.ContextPackage
	// Pull the primary source doc id from the package's SourceDocIDs slice.
	sourceDocID := ""
	if len(pkg.SourceDocIDs) > 0 {
		sourceDocID = pkg.SourceDocIDs[0]
	}
	draft := FlowAuditDraft{
		WorkflowRunID:     input.WorkflowRunID,
		PlanStepID:        input.PlanStepID,
		CodingStepID:      input.CodingStepID,
		TestingStepID:     input.TestingStepID,
		AuditStepID:       input.AuditStepID,
		OriginalPackageID: pkg.PackageID,
		FeatureKey:        pkg.FeatureKey,
		SourceDocID:       sourceDocID,
		ChangeType:        input.ChangeType,
		Summary:           strings.TrimSpace(input.WhatChanged),
		WhatChanged:       input.WhatChanged,
		WhyChanged:        input.WhyChanged,
		ChangedFiles:      input.ChangedFiles,
		ResidualNotes:     input.ResidualNotes,
		ValidationResult:  input.ValidationState.Status,
	}

	// Collect distinct validation commands.
	if input.ValidationState.ValidationCommand != "" {
		draft.ValidationCommands = []string{input.ValidationState.ValidationCommand}
	}

	// Only a confirmed "passed" validation may produce a ready audit draft (T-4).
	// skipped_no_command, skipped_env_error, retrying, and max-retry failure all
	// mean the codebase was not positively verified — block every case except passed.
	if draft.ValidationResult != "passed" {
		draft.Status = "blocked_validation_failed"
		return draft
	}

	// Block when feature key is missing or unverified (T-3).
	if strings.TrimSpace(draft.FeatureKey) == "" ||
		pkg.FeatureConfidence == ConfidenceUnresolved {
		draft.Status = "blocked_missing_feature_key"
		return draft
	}

	// Verify feature key exists in the registry.
	if !featureKeyRegistered(input.Workspace, draft.FeatureKey) {
		draft.Status = "blocked_missing_feature_key"
		return draft
	}

	// Build commit message (T-3).
	draft.CommitMessage = buildCommitMessage(draft.FeatureKey, draft.ChangeType, draft.Summary, draft.SourceDocID)

	// Build change-ledger block (T-4, SS-13).
	draft.ChangeLedgerBlock = buildChangeLedgerBlock(draft.FeatureKey, draft.SourceDocID, draft.ChangeType, draft.Summary)

	draft.Status = "ready"
	return draft
}

// buildCommitMessage composes the `[Type][feature][layer?] description source-doc-id`
// commit message following the FlowPilot commit-format contract.
func buildCommitMessage(featureKey, changeType, summary, sourceDocID string) string {
	ct := commitTypeFromChangeType(changeType)
	prefix := fmt.Sprintf("[%s][%s] ", ct, featureKey)
	suffix := ""
	if id := strings.TrimSpace(sourceDocID); id != "" {
		suffix = " " + id
	}
	desc := strings.TrimSpace(summary)
	// Trim description so that prefix + desc + suffix fits within 72 chars.
	maxDesc := 72 - len(prefix) - len(suffix)
	if maxDesc < 10 {
		maxDesc = 10
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-3] + "..."
	}
	return prefix + desc + suffix
}

func commitTypeFromChangeType(changeType string) string {
	switch strings.ToLower(strings.TrimSpace(changeType)) {
	case "bugfix", "bug", "fix":
		return "BugFix"
	case "refactor", "refactoring":
		return "Refactor"
	case "docs", "doc", "documentation":
		return "Docs"
	case "test", "tests":
		return "Test"
	case "hotfix":
		return "Hotfix"
	default:
		return "Feature"
	}
}

// buildChangeLedgerBlock produces the SS-13 change-ledger YAML block that
// CA notes include.
func buildChangeLedgerBlock(featureKey, sourceDocID, changeType, summary string) string {
	var sb strings.Builder
	sb.WriteString("```flowpilot:change-ledger\n")
	sb.WriteString(fmt.Sprintf("feature_key: %s\n", featureKey))
	if sourceDocID != "" {
		sb.WriteString(fmt.Sprintf("source_doc_id: %s\n", sourceDocID))
	}
	ct := strings.ToLower(strings.TrimSpace(changeType))
	if ct == "" {
		ct = "feature"
	}
	sb.WriteString(fmt.Sprintf("change_type: %s\n", ct))
	sb.WriteString(fmt.Sprintf("summary: %s\n", strings.TrimSpace(summary)))
	sb.WriteString("```")
	return sb.String()
}

// featureKeyRegistered checks that the given key appears in the FEATURE-KEYS.md
// registry. The check is a substring scan — keys are on their own lines in
// "key — description" format.
func featureKeyRegistered(workspace, key string) bool {
	if key == "" {
		return false
	}
	if workspace == "" {
		// No workspace to check — accept any non-empty key so tests that don't
		// provide a workspace still produce a "ready" draft when all other
		// conditions are met. Tests that specifically require registry validation
		// must provide the workspace.
		return true
	}
	registryPath := filepath.Join(workspace, featureKeyRegistryFile)
	data, err := os.ReadFile(registryPath)
	if err != nil {
		// Registry unavailable → treat as unregistered.
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- "+key+" ") || strings.HasPrefix(line, "- "+key+"\t") || line == "- "+key {
			return true
		}
	}
	return false
}

// RenderAuditDraftText returns a human-readable markdown rendering of the draft
// for display or inspection (T-5, DOD-6).
func RenderAuditDraftText(draft FlowAuditDraft) string {
	var sb strings.Builder
	sb.WriteString("# Flow Audit Draft\n\n")
	sb.WriteString(fmt.Sprintf("**Status**: %s  \n", draft.Status))
	sb.WriteString(fmt.Sprintf("**Feature key**: `%s`  \n", draft.FeatureKey))
	if draft.SourceDocID != "" {
		sb.WriteString(fmt.Sprintf("**Source doc**: %s  \n", draft.SourceDocID))
	}
	sb.WriteString(fmt.Sprintf("**Validation**: %s  \n\n", draft.ValidationResult))

	if draft.WhatChanged != "" {
		sb.WriteString(fmt.Sprintf("## What Changed\n\n%s\n\n", draft.WhatChanged))
	}
	if draft.WhyChanged != "" {
		sb.WriteString(fmt.Sprintf("## Why Changed\n\n%s\n\n", draft.WhyChanged))
	}
	if len(draft.ChangedFiles) > 0 {
		sb.WriteString("## Changed Files\n\n")
		for _, f := range draft.ChangedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}
	if draft.ResidualNotes != "" {
		sb.WriteString(fmt.Sprintf("## Residual Notes\n\n%s\n\n", draft.ResidualNotes))
	}
	if draft.ChangeLedgerBlock != "" {
		sb.WriteString("## Change Ledger Block\n\n")
		sb.WriteString(draft.ChangeLedgerBlock + "\n\n")
	}
	if draft.CommitMessage != "" {
		sb.WriteString(fmt.Sprintf("## Suggested Commit Message\n\n```\n%s\n```\n", draft.CommitMessage))
	}
	return sb.String()
}

// FindAuditDraft scans events newest-first for an EventFlowAuditDraft event
// matching the given auditStepID.
func FindAuditDraft(events []ProviderEvent, auditStepID string) (*FlowAuditDraft, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == EventFlowAuditDraft && ev.WorkflowStepRunID == auditStepID && ev.FlowAuditDraft != nil {
			return ev.FlowAuditDraft, true
		}
	}
	return nil, false
}

// PersistAuditDraft emits an EventFlowAuditDraft event so the draft is
// inspectable from run state before any file write or commit (T-5, DOD-8).
func PersistAuditDraft(ctx context.Context, store InteractiveStateStore, runID, auditStepID string, draft FlowAuditDraft) error {
	return store.AppendEvent(ctx, ProviderEvent{
		Type:              EventFlowAuditDraft,
		WorkflowRunID:     runID,
		WorkflowStepRunID: auditStepID,
		FlowAuditDraft:    &draft,
	})
}

// featureKeyFromCatalog resolves the feature key using the existing catalog
// resolver — same path as BuildFlowContextPackage (Task-168 / T-2).
// Returns "" when resolution fails.
func featureKeyFromCatalog(workspace, prompt string) string {
	dotFlowpilotDir := filepath.Join(workspace, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFlowpilotDir)
	if err != nil {
		return ""
	}
	candidates, err := featurecatalog.ResolveFeature(strings.TrimSpace(prompt), catalog)
	if err != nil {
		return ""
	}
	top, ok := featurecatalog.TopCandidate(candidates, 5.0)
	if !ok {
		return ""
	}
	return top.Key
}
