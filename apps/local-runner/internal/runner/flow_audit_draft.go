package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const flowAuditArtifactSourceKind = "flow_audit_draft"

type FlowAuditDraft struct {
	WorkflowRunID      string   `json:"workflowRunId"`
	PlanStepRunID      string   `json:"planStepRunId"`
	CodingStepRunID    string   `json:"codingStepRunId"`
	TestingStepRunID   string   `json:"testingStepRunId"`
	AuditStepRunID     string   `json:"auditStepRunId"`
	FeatureKey         string   `json:"featureKey"`
	SourceDocID        string   `json:"sourceDocId"`
	ChangeType         string   `json:"changeType"`
	Summary            string   `json:"summary"`
	WhatChanged        string   `json:"whatChanged"`
	WhyChanged         string   `json:"whyChanged"`
	ChangedFiles       []string `json:"changedFiles,omitempty"`
	ValidationCommands []string `json:"validationCommands,omitempty"`
	ValidationResult   string   `json:"validationResult"`
	ResidualNotes      []string `json:"residualNotes,omitempty"`
	CommitMessage      string   `json:"commitMessage"`
	ChangeLedgerBlock  string   `json:"changeLedgerBlock"`
	Status             string   `json:"status"`
	DraftID            string   `json:"draftId"`
}

func BuildFlowAuditDraft(repoDir string, pkg FlowContextPackage, codingStepID, testingStepID, auditStepID string, changedFiles, validationCommands, residualNotes []string, validationResult, changeType, summary string) FlowAuditDraft {
	sourceDocID := ""
	if len(pkg.SourceDocIDs) > 0 {
		sourceDocID = pkg.SourceDocIDs[0]
	}
	draft := FlowAuditDraft{
		WorkflowRunID:      pkg.WorkflowRunID,
		PlanStepRunID:      pkg.PlanStepRunID,
		CodingStepRunID:    codingStepID,
		TestingStepRunID:   testingStepID,
		AuditStepRunID:     auditStepID,
		FeatureKey:         pkg.FeatureKey,
		SourceDocID:        sourceDocID,
		ChangeType:         changeType,
		Summary:            summary,
		WhatChanged:        strings.TrimSpace(strings.Join(append([]string(nil), changedFiles...), "\n")),
		WhyChanged:         strings.TrimSpace(pkg.DiscussionBlock),
		ChangedFiles:       uniqueSorted(changedFiles),
		ValidationCommands: uniqueSorted(validationCommands),
		ValidationResult:   validationResult,
		ResidualNotes:      uniqueSorted(residualNotes),
		Status:             "draft",
	}

	if strings.TrimSpace(repoDir) != "" && draft.FeatureKey != "" {
		if verifyFeatureKey(repoDir, draft.FeatureKey) {
			draft.Status = "ready"
		} else {
			draft.Status = "blocked_missing_feature_key"
		}
	}

	draft.CommitMessage = buildFlowAuditCommitMessage(draft.FeatureKey, draft.ChangeType, draft.Summary, draft.SourceDocID)
	draft.ChangeLedgerBlock = renderFlowAuditLedgerBlock(draft.FeatureKey, draft.SourceDocID, draft.ChangeType, draft.Summary)
	draft.DraftID = flowAuditDraftID(draft)
	if draft.WhatChanged == "" {
		draft.WhatChanged = renderFlowAuditBulletList(draft.ChangedFiles)
	}
	if draft.WhyChanged == "" {
		draft.WhyChanged = "No prior discussion block was available."
	}
	return draft
}

func buildFlowAuditCommitMessage(featureKey, changeType, summary, sourceDocID string) string {
	if strings.TrimSpace(featureKey) == "" {
		return ""
	}
	layer := "infra"
	if changeType == "docs" {
		layer = "docs"
	} else if changeType == "feature" {
		layer = "domain"
	}
	return fmt.Sprintf("[Feature][%s][%s] %s %s", featureKey, layer, strings.TrimSpace(summary), strings.TrimSpace(sourceDocID))
}

func renderFlowAuditLedgerBlock(featureKey, sourceDocID, changeType, summary string) string {
	return strings.TrimSpace(strings.Join([]string{
		"# ---8<--- flowpilot:change-ledger",
		"feature_key: " + strings.TrimSpace(featureKey),
		"source_doc_id: " + strings.TrimSpace(sourceDocID),
		"change_type: " + strings.TrimSpace(changeType),
		"summary: " + strings.TrimSpace(summary),
		"# --->8---",
	}, "\n"))
}

func RenderFlowAuditDraft(draft FlowAuditDraft) string {
	var sb strings.Builder
	sb.WriteString("## Flow Audit Draft\n")
	sb.WriteString(fmt.Sprintf("- Draft ID: %s\n", draft.DraftID))
	sb.WriteString(fmt.Sprintf("- Feature key: %s\n", draft.FeatureKey))
	sb.WriteString(fmt.Sprintf("- Source doc ID: %s\n", draft.SourceDocID))
	sb.WriteString(fmt.Sprintf("- Commit message: %s\n", draft.CommitMessage))
	sb.WriteString(fmt.Sprintf("- Status: %s\n", draft.Status))
	sb.WriteString("\n### What changed\n")
	sb.WriteString(strings.TrimSpace(draft.WhatChanged))
	sb.WriteString("\n\n### Why changed\n")
	sb.WriteString(strings.TrimSpace(draft.WhyChanged))
	sb.WriteString("\n\n### Validation\n")
	if len(draft.ValidationCommands) == 0 {
		sb.WriteString("None\n")
	} else {
		sb.WriteString(renderFlowAuditBulletList(draft.ValidationCommands))
		sb.WriteString("\n")
	}
	sb.WriteString(strings.TrimSpace(draft.ValidationResult))
	sb.WriteString("\n\n### Residual notes\n")
	if len(draft.ResidualNotes) == 0 {
		sb.WriteString("None\n")
	} else {
		sb.WriteString(renderFlowAuditBulletList(draft.ResidualNotes))
		sb.WriteString("\n")
	}
	sb.WriteString("\n### Change ledger\n")
	sb.WriteString(draft.ChangeLedgerBlock)
	return strings.TrimSpace(sb.String())
}

func flowAuditDraftID(draft FlowAuditDraft) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		draft.WorkflowRunID,
		draft.PlanStepRunID,
		draft.CodingStepRunID,
		draft.TestingStepRunID,
		draft.AuditStepRunID,
		draft.FeatureKey,
		draft.Summary,
		draft.ValidationResult,
	}, "\n")))
	return "flowaudit_" + hex.EncodeToString(sum[:8])
}

func verifyFeatureKey(repoDir, featureKey string) bool {
	raw, err := os.ReadFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"))
	if err != nil {
		return false
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		key, _, ok := strings.Cut(line, " — ")
		if !ok {
			key, _, ok = strings.Cut(line, " - ")
		}
		if ok && strings.TrimSpace(key) == strings.TrimSpace(featureKey) {
			return true
		}
	}
	return false
}

func renderFlowAuditBulletList(values []string) string {
	if len(values) == 0 {
		return "None"
	}
	lines := make([]string, len(values))
	for i, value := range values {
		lines[i] = "- " + strings.TrimSpace(value)
	}
	return strings.Join(lines, "\n")
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
