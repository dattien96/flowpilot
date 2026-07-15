package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// Task-244: feature.history no longer prepends Canonical Head.
func TestFeatureHistorySourceNoLongerPrependsHead(t *testing.T) {
	dir := t.TempDir()
	h := changecontract.CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "Divide returns an error on zero divisor",
		Status:            changecontract.HeadStatusCurrent,
		Decisions:         []changecontract.Decision{{Tried: "lookup table", Outcome: changecontract.DecisionRejected, Reason: "broke on negative divisors"}},
	}
	h.IntentSignature = changecontract.ComputeSignature(h)
	if err := changecontract.SaveHead(dir, h); err != nil {
		t.Fatalf("SaveHead: %v", err)
	}
	src := &featureHistorySource{priority: 2}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: dir, FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if strings.Contains(section.Body, "Canonical state") {
		t.Fatalf("feature.history must not prepend Head, got %q", section.Body)
	}
	if len(section.Warnings) != 1 || !strings.Contains(section.Warnings[0], "no change history found") {
		t.Fatalf("G-4: expected no-history warning when ledger empty, got %v", section.Warnings)
	}
}

func TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior(t *testing.T) {
	dir := t.TempDir()
	src := &featureHistorySource{priority: 2}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: dir, FeatureKey: "never-seen-before", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if section.Body != "" {
		t.Fatalf("expected empty body, got %q", section.Body)
	}
	if len(section.Warnings) != 1 {
		t.Fatalf("expected no-history warning, got %v", section.Warnings)
	}
}

func TestCanonicalHeadSourceFetchesHeadBlock(t *testing.T) {
	dir := t.TempDir()
	h := changecontract.CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "Divide returns an error on zero divisor",
		Status:            changecontract.HeadStatusCurrent,
		Decisions:         []changecontract.Decision{{Tried: "lookup table", Outcome: changecontract.DecisionRejected, Reason: "x"}},
	}
	h.IntentSignature = changecontract.ComputeSignature(h)
	if err := changecontract.SaveHead(dir, h); err != nil {
		t.Fatalf("SaveHead: %v", err)
	}
	src := &canonicalHeadSource{priority: 1}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: dir, FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(section.Body, "Canonical state of") {
		t.Fatalf("body: %q", section.Body)
	}
	if !strings.Contains(section.Body, "lookup table") {
		t.Fatalf("missing rejected decision: %q", section.Body)
	}
	if section.SourceRef == "" {
		t.Fatal("SourceRef empty")
	}
}

func TestCanonicalHeadSourceNoHeadDegrades(t *testing.T) {
	src := &canonicalHeadSource{priority: 1}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: t.TempDir(), FeatureKey: "x", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatal(err)
	}
	if section.Body != "" || section.Warnings != nil {
		t.Fatalf("want empty degrade, got %+v", section)
	}
}

func TestCanonicalHeadSourceUnverifiedFeatureEmpty(t *testing.T) {
	src := &canonicalHeadSource{priority: 1}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: t.TempDir(), FeatureKey: "x", FeatureConfidence: ConfidenceLow,
	})
	if err != nil || section.Body != "" {
		t.Fatalf("unverified must be empty: err=%v body=%q", err, section.Body)
	}
}

func TestCanonicalHeadInDefaultSetAndRegistered(t *testing.T) {
	found := false
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceCanonicalHead) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("canonical.head missing from defaultContextSourceIDs")
	}
	if _, err := NewDefaultContextSourceRegistry().Resolve(string(ContextSourceCanonicalHead)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}
