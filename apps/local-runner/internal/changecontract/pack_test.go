package changecontract

import (
	"strings"
	"testing"
)

func TestRenderHeadBlockEmptyForZeroValueHead(t *testing.T) {
	if got := RenderHeadBlock(CanonicalHead{}); got != "" {
		t.Fatalf("expected empty block for a zero-value Head, got %q", got)
	}
}

func TestRenderHeadBlockIncludesBehaviorAndSignatureAndStatus(t *testing.T) {
	h := CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "Divide returns an error on zero divisor",
		Status:            HeadStatusCurrent,
		IntentSignature:   "abcdef0123456789",
	}
	block := RenderHeadBlock(h)
	if !strings.Contains(block, "calc-core") {
		t.Errorf("expected block to name the feature, got %q", block)
	}
	if !strings.Contains(block, "Divide returns an error on zero divisor") {
		t.Errorf("expected block to include the behavior statement, got %q", block)
	}
	if !strings.Contains(block, "abcdef012345") {
		t.Errorf("expected block to include a truncated signature, got %q", block)
	}
	if strings.Contains(block, "Rejected:") {
		t.Errorf("expected no rejected-decisions section when there are no Decisions, got %q", block)
	}
}

func TestRenderHeadBlockAnnotatesSpecLess(t *testing.T) {
	h := CanonicalHead{FeatureKey: "calc-core", Status: HeadStatusSpecLess, SpecConfidence: SpecConfidenceSpecLess}
	block := RenderHeadBlock(h)
	if !strings.Contains(block, "spec-less") {
		t.Fatalf("expected a spec-less low-confidence annotation, got %q", block)
	}
}

func TestRenderHeadBlockListsRejectedAndRevertedDecisions(t *testing.T) {
	h := CanonicalHead{
		FeatureKey: "calc-core",
		Decisions: []Decision{
			{Tried: "lookup table", Outcome: DecisionRejected, Reason: "no negative divisor support"},
			{Tried: "recursive approach", Outcome: DecisionReverted, Reason: "stack overflow on large inputs"},
			{Tried: "direct arithmetic guard", Outcome: DecisionAdopted},
		},
	}
	block := RenderHeadBlock(h)
	if !strings.Contains(block, "Rejected:") {
		t.Fatalf("expected a rejected-decisions section, got %q", block)
	}
	if !strings.Contains(block, "lookup table") || !strings.Contains(block, "recursive approach") {
		t.Fatalf("expected both rejected and reverted approaches listed, got %q", block)
	}
	if strings.Contains(block, "direct arithmetic guard") {
		t.Fatalf("expected the adopted decision to be excluded from the do-not-re-attempt list, got %q", block)
	}
}
