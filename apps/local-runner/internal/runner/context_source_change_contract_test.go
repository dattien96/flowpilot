package runner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
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
	if !strings.Contains(sec.Body, "a.go") || !strings.Contains(sec.Body, "Declared scope") {
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
	if !strings.Contains(got, "Canonical state") {
		t.Fatalf("want head-only: %q", got)
	}
}
