package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
)

func TestChangeContractSourceFetchAndDefault(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := changecontract.Contract{
		RunID: "run-1", StepID: "coder", FeatureKey: "feat", Intent: "do x",
		DeclaredPaths: []string{"a.go"}, Confidence: changecontract.ConfidenceDeclared,
		DeclaredAt: time.Now().UTC(),
	}
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	src := &changeContractSource{priority: 3}
	sec, err := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sec.Body, "a.go") || !strings.Contains(sec.Body, "Scope (do not edit outside)") {
		t.Fatalf("body: %q", sec.Body)
	}
	if sec.SourceRef == "" {
		t.Fatal("SourceRef empty")
	}
	// empty run
	sec2, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir})
	if sec2.Body != "" {
		t.Fatal("empty run id must yield empty")
	}
	// default set
	found := false
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceChangeContract) {
			found = true
		}
	}
	if !found {
		t.Fatal("change.contract not in default set")
	}
	if _, err := NewDefaultContextSourceRegistry().Resolve(string(ContextSourceChangeContract)); err != nil {
		t.Fatal(err)
	}
}

func TestAppendChangeContractIfAnyNoDouble(t *testing.T) {
	dir := t.TempDir()
	store, _ := changecontract.NewStore(dir)
	_ = store.Save(changecontract.Contract{
		RunID: "r", StepID: "s", FeatureKey: "f", DeclaredPaths: []string{"x.go"},
		Confidence: changecontract.ConfidenceDeclared, DeclaredAt: time.Now().UTC(),
	})
	p1 := appendChangeContractIfAny(dir, "r", "base prompt")
	if !strings.Contains(p1, "Change Contract") {
		t.Fatalf("expected inject: %q", p1)
	}
	p2 := appendChangeContractIfAny(dir, "r", p1)
	if strings.Count(p2, changeContractPromptMarker) != 1 {
		t.Fatalf("double inject: %q", p2)
	}
}

// Parent-keyed contract (child gate capture) must inject into re-entry compose paths.
func TestAppendChangeContractIfAnyParentKeyedFromChildCapture(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Simulates child gate saving under parent flow run id with child node step.
	if err := store.Save(changecontract.Contract{
		RunID: "parent-flow", StepID: "coder", FeatureKey: "feat",
		DeclaredPaths: []string{"apps/foo.go"}, Confidence: changecontract.ConfidenceDeclared,
		DeclaredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	// Child run id must NOT be required for inject.
	if p := appendChangeContractIfAny(dir, "child-coder-run", "retry base"); strings.Contains(p, "Change Contract") {
		t.Fatalf("child run id alone must not find parent contract: %q", p)
	}
	got := appendChangeContractIfAny(dir, "parent-flow", "retry base")
	if !strings.Contains(got, "apps/foo.go") || !strings.Contains(got, changeContractPromptMarker) {
		t.Fatalf("parent-keyed inject failed: %q", got)
	}
}

func TestComposeFeatureBlocksHeadOnly(t *testing.T) {
	// Task-245: head injects even without history ledger entries.
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	h := changecontract.CanonicalHead{
		FeatureKey: "feat-a", BehaviorStatement: "behaves", Status: changecontract.HeadStatusCurrent,
	}
	h.IntentSignature = changecontract.ComputeSignature(h)
	if err := changecontract.SaveHead(dir, h); err != nil {
		t.Fatal(err)
	}
	got := composeFeatureBlocks(dot, "feat-a")
	if !strings.Contains(got, "## Canonical") {
		t.Fatalf("want head-only: %q", got)
	}
}

func TestComposeFeatureBlocksNoHeadNoHistory(t *testing.T) {
	// Task-245: empty feature with no head and no ledger → empty inject.
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(dot, 0o755); err != nil {
		t.Fatal(err)
	}
	got := composeFeatureBlocks(dot, "missing-feature")
	if strings.TrimSpace(got) != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestComposeFeatureBlocksHeadBeforeHistory(t *testing.T) {
	// Task-245: head leads, history follows (ordering regression).
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(dot, 0o755); err != nil {
		t.Fatal(err)
	}
	h := changecontract.CanonicalHead{
		FeatureKey: "feat-b", BehaviorStatement: "behaves-b", Status: changecontract.HeadStatusCurrent,
	}
	h.IntentSignature = changecontract.ComputeSignature(h)
	if err := changecontract.SaveHead(dir, h); err != nil {
		t.Fatal(err)
	}
	ledger, err := changeledger.New(dot)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Upsert([]changeledger.Entry{{
		CommitHash:  "aabbccdd0011",
		FeatureKey:  "feat-b",
		Summary:     "prior work entry for ordering",
		CommittedAt: time.Now().UTC().Format(time.RFC3339),
		Confidence:  changeledger.ConfidenceHigh,
	}}); err != nil {
		t.Fatal(err)
	}
	got := composeFeatureBlocks(dot, "feat-b")
	headIdx := strings.Index(got, "## Canonical")
	if headIdx < 0 {
		t.Fatalf("want head in compose: %q", got)
	}
	histIdx := strings.Index(got, "prior work entry for ordering")
	if histIdx < 0 {
		t.Fatalf("want history in compose: %q", got)
	}
	if histIdx < headIdx {
		t.Fatalf("history must not precede head: %q", got)
	}
}

func TestRenderFlowContextPackageHeadFirst(t *testing.T) {
	// Task-244: Canonical Head section renders before Change History.
	pkg := FlowContextPackage{
		PackageID:         "p1",
		FeatureKey:        "f",
		FeatureConfidence: "high",
		HistoryBlock:      "history-body-here",
		Sections: []FlowContextSection{
			{
				SourceType: string(ContextSourceCanonicalHead),
				Body:       "## Canonical \"f\" [current] sig=abc\n\nhead-body-here\n",
			},
		},
	}
	got := RenderFlowContextPackage(pkg)
	headIdx := strings.Index(got, "head-body-here")
	histIdx := strings.Index(got, "history-body-here")
	if headIdx < 0 || histIdx < 0 {
		t.Fatalf("missing head or history in render: %q", got)
	}
	if headIdx > histIdx {
		t.Fatalf("head must render before history: %q", got)
	}
}
