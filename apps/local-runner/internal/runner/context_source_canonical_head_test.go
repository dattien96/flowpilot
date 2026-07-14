package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// TestFeatureHistorySourcePrependsCanonicalHead verifies Task-188's T-1: when
// a Canonical Head exists on disk for the resolved feature, its rendered
// block leads the feature.history section body, before any raw history.
func TestFeatureHistorySourcePrependsCanonicalHead(t *testing.T) {
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

	src := &featureHistorySource{priority: 1}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace:         dir,
		FeatureKey:        "calc-core",
		FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(section.Body, "Canonical state of") {
		t.Fatalf("expected the Canonical Head block to be prepended, got body: %q", section.Body)
	}
	if !strings.Contains(section.Body, "lookup table") {
		t.Fatalf("expected the rejected decision to appear in the block, got body: %q", section.Body)
	}
	if len(section.Warnings) != 0 {
		t.Fatalf("expected no warning when a Head block exists, got %v", section.Warnings)
	}
}

func TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior(t *testing.T) {
	dir := t.TempDir()
	src := &featureHistorySource{priority: 1}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace:         dir,
		FeatureKey:        "never-seen-before",
		FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if section.Body != "" {
		t.Fatalf("expected empty body with no Head and no history, got %q", section.Body)
	}
	if len(section.Warnings) != 1 {
		t.Fatalf("expected the original no-history warning to still fire, got %v", section.Warnings)
	}
}
