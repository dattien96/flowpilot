package runner

import (
	"strings"
	"testing"
)

func TestApplyFlowContextPackBudgetDropsHistoryKeepsHead(t *testing.T) {
	hugeHistory := strings.Repeat("x", flowContextRenderBudgetBytes)
	headBody := "## Canonical \"calc-core\" [current] sig=abc\n\nBehavior: guard zero divisor\n"
	pkg := FlowContextPackage{
		PackageID:     "pkg-1",
		WorkflowRunID: "run-1",
		FeatureKey:    "calc-core",
		HistoryBlock:  hugeHistory,
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceCanonicalHead), Priority: 1, Body: headBody},
			{SourceType: string(ContextSourceFeatureHistory), Priority: 2, Body: hugeHistory},
		},
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "guard zero divisor") {
		t.Fatalf("Canonical Head must survive budget pressure, got:\n%s", rendered)
	}
	if strings.Contains(rendered, strings.Repeat("x", 1024)) {
		t.Fatal("feature.history body should be dropped under budget")
	}
	if !strings.Contains(rendered, "omitted feature.history under budget") {
		t.Fatalf("expected budget warning in render, got:\n%s", rendered)
	}
}

func TestApplyFlowContextPackBudgetNoOpWhenSmall(t *testing.T) {
	pkg := FlowContextPackage{
		PackageID:    "pkg-2",
		FeatureKey:   "f",
		HistoryBlock: "short history",
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceFeatureHistory), Priority: 2, Body: "short history"},
		},
	}
	before := pkg.HistoryBlock
	applyFlowContextPackBudget(&pkg)
	if pkg.HistoryBlock != before {
		t.Fatalf("small package must not drop history, got %q", pkg.HistoryBlock)
	}
}

func TestRenderFlowContextPackageHeadBeforeHistoryAfterBudget(t *testing.T) {
	pkg := FlowContextPackage{
		PackageID:    "pkg-3",
		FeatureKey:   "f",
		HistoryBlock: strings.Repeat("h", flowContextRenderBudgetBytes),
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceCanonicalHead), Priority: 1, Body: "HEAD-MARKER\n"},
		},
	}
	out := RenderFlowContextPackage(pkg)
	if !strings.Contains(out, "HEAD-MARKER") {
		t.Fatal("missing head after budget")
	}
	headIdx := strings.Index(out, "HEAD-MARKER")
	histIdx := strings.Index(out, "## History")
	if histIdx >= 0 && headIdx > histIdx {
		t.Fatal("head must render before history when history present")
	}
}
