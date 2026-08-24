package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// run-127174: package resolved "grok" (catalog noise from .grok/skills), but
// coder declared "calc-format" via Contract. Audit must prefer the declared
// contract when it is registered, so the flow finishes DONE instead of parking.

func writeFeatureKeys(t *testing.T, workspace string, keys []string) {
	t.Helper()
	content := "# Feature Keys\n\n"
	for _, k := range keys {
		content += "- " + k + " — test\n"
	}
	if err := os.WriteFile(filepath.Join(workspace, "change-audit", "FEATURE-KEYS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write FEATURE-KEYS: %v", err)
	}
	// Also write at workspace root FEATURE-KEYS alternative checked by featureKeyRegistered
	// featureKeyRegistered checks workspace/featureKeyRegistryFile = "change-audit/FEATURE-KEYS.md"
}

func preferContractKey(workspace, runID string, pkg FlowContextPackage) FlowContextPackage {
	// Mirror production override in runAuditNode.
	if workspace != "" {
		if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
			if c, ok := store.GetLatestForRun(runID); ok && strings.TrimSpace(c.FeatureKey) != "" && c.Confidence == changecontract.ConfidenceDeclared {
				if featureKeyRegistered(workspace, c.FeatureKey) {
					pkg.FeatureKey = c.FeatureKey
					pkg.FeatureConfidence = ConfidenceVerified
				}
			}
		}
	}
	return pkg
}

func TestRun127174_AuditPrefersDeclaredContractOverGrokPackage(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFeatureKeys(t, ws, []string{"calc-format", "calc-core"})
	// Contract declares calc-format
	store, err := changecontract.NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{RunID: "run-127174", StepID: "implement", FeatureKey: "calc-format", Confidence: changecontract.ConfidenceDeclared}); err != nil {
		t.Fatal(err)
	}

	pkg := FlowContextPackage{FeatureKey: "grok", FeatureConfidence: ConfidenceVerified, PackageID: "pkg-1"}
	overridden := preferContractKey(ws, "run-127174", pkg)
	if overridden.FeatureKey != "calc-format" {
		t.Fatalf("FeatureKey = %q, want calc-format (declared contract should override grok)", overridden.FeatureKey)
	}
	if overridden.FeatureConfidence != ConfidenceVerified {
		t.Fatalf("FeatureConfidence = %q, want verified", overridden.FeatureConfidence)
	}

	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   "run-127174",
		ContextPackage:  overridden,
		ValidationState: FlowValidationRetryState{Status: "passed"},
		ChangedFiles:    []string{"format.go"},
		WhatChanged:     "ClampChecked",
		Workspace:       ws,
	})
	if draft.Status != "ready" {
		t.Fatalf("draft.Status = %q, want ready (registered calc-format + passed)", draft.Status)
	}
	if draft.FeatureKey != "calc-format" {
		t.Fatalf("draft.FeatureKey = %q, want calc-format", draft.FeatureKey)
	}
}

func TestRun127174_AuditNoContractStillBlockedOnGrok(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFeatureKeys(t, ws, []string{"calc-format"})
	// No contract store entry for this run
	pkg := FlowContextPackage{FeatureKey: "grok", FeatureConfidence: ConfidenceVerified, PackageID: "pkg-1"}
	overridden := preferContractKey(ws, "run-127174-no-contract", pkg)
	if overridden.FeatureKey != "grok" {
		t.Fatalf("FeatureKey = %q, want grok (no contract to override)", overridden.FeatureKey)
	}
	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   "run-127174-no-contract",
		ContextPackage:  overridden,
		ValidationState: FlowValidationRetryState{Status: "passed"},
		ChangedFiles:    []string{"format.go"},
		Workspace:       ws,
	})
	if draft.Status != "blocked_missing_feature_key" {
		t.Fatalf("draft.Status = %q, want blocked_missing_feature_key (grok not in FEATURE-KEYS)", draft.Status)
	}
}

func TestRun127174_AuditUnregisteredContractKeyStillBlocked(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFeatureKeys(t, ws, []string{"calc-format"})
	store, err := changecontract.NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{RunID: "run-x", StepID: "implement", FeatureKey: "not-registered", Confidence: changecontract.ConfidenceDeclared}); err != nil {
		t.Fatal(err)
	}
	pkg := FlowContextPackage{FeatureKey: "grok", FeatureConfidence: ConfidenceVerified, PackageID: "pkg-1"}
	overridden := preferContractKey(ws, "run-x", pkg)
	if overridden.FeatureKey != "grok" {
		t.Fatalf("FeatureKey = %q, want grok (unregistered contract should not override)", overridden.FeatureKey)
	}
	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   "run-x",
		ContextPackage:  overridden,
		ValidationState: FlowValidationRetryState{Status: "passed"},
		Workspace:       ws,
	})
	if draft.Status != "blocked_missing_feature_key" {
		t.Fatalf("draft.Status = %q, want blocked_missing_feature_key", draft.Status)
	}
}

func TestRun127174_AuditPassedValidationRequiredEvenWithContract(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFeatureKeys(t, ws, []string{"calc-format"})
	store, err := changecontract.NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{RunID: "run-y", StepID: "implement", FeatureKey: "calc-format", Confidence: changecontract.ConfidenceDeclared}); err != nil {
		t.Fatal(err)
	}
	pkg := FlowContextPackage{FeatureKey: "grok", FeatureConfidence: ConfidenceVerified, PackageID: "pkg-1"}
	overridden := preferContractKey(ws, "run-y", pkg)
	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   "run-y",
		ContextPackage:  overridden,
		ValidationState: FlowValidationRetryState{Status: "failed"},
		ChangedFiles:    []string{"format.go"},
		Workspace:       ws,
	})
	if draft.Status != "blocked_validation_failed" {
		t.Fatalf("draft.Status = %q, want blocked_validation_failed (contract cannot bypass failed validation)", draft.Status)
	}
}
