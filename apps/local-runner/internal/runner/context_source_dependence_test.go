package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/structure"
)

type fakeDependenceProvider struct {
	available bool
	byTarget  map[string]structure.DependentsSummary
	calls     []string
}

func (f *fakeDependenceProvider) Available() bool { return f.available }

func (f *fakeDependenceProvider) Dependents(_ context.Context, target string) (structure.DependentsSummary, error) {
	f.calls = append(f.calls, target)
	return f.byTarget[target], nil
}

func newDependenceSourceWithFake(f *fakeDependenceProvider) *dependenceSource {
	return &dependenceSource{
		priority: 3,
		newProvider: func(string, bool) structure.Provider {
			return f
		},
	}
}

func saveDependenceTestContract(t *testing.T, dir, runID string, c changecontract.Contract) {
	t.Helper()
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.RunID = runID
	if c.DeclaredAt.IsZero() {
		c.DeclaredAt = time.Now().UTC()
	}
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
}

func TestDependenceSourceFromContractTargets(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:      "feat",
		DeclaredPaths:   []string{"apps/local-runner/internal/runner/foo.go"},
		DeclaredSymbols: []string{"Bar"},
		Confidence:      changecontract.ConfidenceDeclared,
	})
	fake := &fakeDependenceProvider{
		available: true,
		byTarget: map[string]structure.DependentsSummary{
			"Bar": {Count: 1, Nearest: []string{"Baz"}},
			"Foo": {Count: 2, Nearest: []string{"Qux"}},
		},
	}
	src := newDependenceSourceWithFake(fake)
	sec, err := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sec.Body, "Bar") || !strings.Contains(sec.Body, "Foo") {
		t.Fatalf("body missing targets: %q", sec.Body)
	}
	if !strings.Contains(sec.SourceRef, "contracts.ndjson") {
		t.Fatalf("SourceRef = %q", sec.SourceRef)
	}
	wantCalls := []string{"Bar", "Foo"}
	if len(fake.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", fake.calls, wantCalls)
	}
	for i, w := range wantCalls {
		if fake.calls[i] != w {
			t.Fatalf("calls[%d] = %q, want %q (all=%v)", i, fake.calls[i], w, fake.calls)
		}
	}
}

func TestDependenceSourceNoContractDegrades(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeDependenceProvider{available: true}
	src := newDependenceSourceWithFake(fake)
	sec, err := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if sec.Body != "" || sec.Warnings != nil || sec.Omitted != nil {
		t.Fatalf("expected zero-value section, got %+v", sec)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("unexpected calls: %v", fake.calls)
	}
}

func TestDependenceSourceInferredDirBucketYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:    "feat",
		DeclaredPaths: []string{"apps", "internal"},
		Confidence:    changecontract.ConfidenceInferred,
	})
	fake := &fakeDependenceProvider{available: true}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if sec.Body != "" {
		t.Fatalf("expected empty body for dir-bucket contract, got %q", sec.Body)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v, want none", fake.calls)
	}
}

func TestDependenceSourceDropsGlobDocAndFlagTargets(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey: "feat",
		DeclaredPaths: []string{
			"internal/**",
			"requirements/x.md",
			"internal/runner/x.go",
		},
		DeclaredSymbols: []string{"-rf", "ValidSym"},
		Confidence:      changecontract.ConfidenceDeclared,
	})
	fake := &fakeDependenceProvider{
		available: true,
		byTarget: map[string]structure.DependentsSummary{
			"ValidSym": {Count: 1, Nearest: []string{"A"}},
			"X":        {Count: 1, Nearest: []string{"B"}},
		},
	}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if sec.Body == "" {
		t.Fatal("expected body")
	}
	wantCalls := []string{"ValidSym", "X"}
	if len(fake.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", fake.calls, wantCalls)
	}
}

func TestDependenceSourceGitNexusUnavailableRendersNote(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:      "feat",
		DeclaredPaths:   []string{"internal/runner/x.go"},
		DeclaredSymbols: []string{"Sym"},
		Confidence:      changecontract.ConfidenceDeclared,
	})
	fake := &fakeDependenceProvider{available: false}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if !strings.Contains(strings.ToLower(sec.Body), "unavailable") {
		t.Fatalf("body = %q", sec.Body)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v, want none when unavailable", fake.calls)
	}
}

func TestDependenceSourceBoundsTargetsAndDependents(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = fmt.Sprintf("internal/pkg/f%d.go", i)
	}
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:    "feat",
		DeclaredPaths: paths,
		Confidence:    changecontract.ConfidenceDeclared,
	})
	nearest := make([]string, 40)
	for i := range nearest {
		nearest[i] = "dep" + string(rune('a'+i%26))
	}
	fake := &fakeDependenceProvider{
		available: true,
		byTarget:  map[string]structure.DependentsSummary{"F": {Count: 40, Nearest: nearest}},
	}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if len(fake.calls) > dependenceMaxTargets {
		t.Fatalf("calls = %d, want <= %d", len(fake.calls), dependenceMaxTargets)
	}
	if sec.Body != "" {
		lines := strings.Count(sec.Body, "\n  - dep")
		if lines > dependenceMaxDependents {
			t.Fatalf("rendered %d dependent lines, want <= %d", lines, dependenceMaxDependents)
		}
	}
}

func TestDependenceSourceDeterministicOutput(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:      "feat",
		DeclaredSymbols: []string{"Alpha"},
		Confidence:      changecontract.ConfidenceDeclared,
	})
	fake := &fakeDependenceProvider{
		available: true,
		byTarget: map[string]structure.DependentsSummary{
			"Alpha": {Count: 2, Nearest: []string{"z", "a"}},
		},
	}
	src := newDependenceSourceWithFake(fake)
	sec1, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	fake.calls = nil
	sec2, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if sec1.Body != sec2.Body {
		t.Fatalf("non-deterministic body:\n1=%q\n2=%q", sec1.Body, sec2.Body)
	}
	if !strings.Contains(sec1.Body, "  - a\n") || !strings.Contains(sec1.Body, "  - z\n") {
		t.Fatalf("expected sorted nearest: %q", sec1.Body)
	}
}

func TestDependenceSourceIncompleteNote(t *testing.T) {
	dir := t.TempDir()
	saveDependenceTestContract(t, dir, "run-1", changecontract.Contract{
		FeatureKey:      "feat",
		DeclaredSymbols: []string{"Sym"},
		Confidence:      changecontract.ConfidenceDeclared,
	})
	fake := &fakeDependenceProvider{
		available: true,
		byTarget: map[string]structure.DependentsSummary{
			"Sym": {Count: 1, Nearest: []string{"A"}, Complete: false},
		},
	}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-1"})
	if !strings.Contains(sec.Body, "Complete=false") {
		t.Fatalf("body = %q", sec.Body)
	}
}

func TestDependenceInDefaultSetAndRegistered(t *testing.T) {
	found := false
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceDependence) {
			found = true
		}
	}
	if !found {
		t.Fatal("source.dependence not in default set")
	}
	src, err := DefaultContextSourceRegistry().Resolve(string(ContextSourceDependence))
	if err != nil {
		t.Fatal(err)
	}
	if src.Priority() != 3 {
		t.Fatalf("priority = %d, want 3", src.Priority())
	}
	def := parseTestFlow(t, minimalContextFlowYAML("      - source.dependence\n"))
	if err := ValidateFlowContextSources(def); err != nil {
		t.Fatal(err)
	}
}

func TestRenderFlowContextPackageDependenceAfterContract(t *testing.T) {
	pkg := FlowContextPackage{
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceChangeContract), Priority: 3, Body: "contract body"},
			{SourceType: string(ContextSourceDependence), Priority: 3, Body: "dependence body"},
			{SourceType: string(ContextSourceSourceExcerpt), Priority: 4, Excerpts: []FlowContextExcerpt{{Path: "a.go", Excerpt: "x"}}},
		},
	}
	var sb strings.Builder
	for _, s := range sectionsForRender(pkg) {
		renderFlowContextSection(&sb, s, pkg)
	}
	out := sb.String()
	idxContract := strings.Index(out, "### change.contract")
	idxDependence := strings.Index(out, "### source.dependence")
	idxExcerpt := strings.Index(out, "### Source:")
	if idxContract < 0 || idxDependence < 0 || idxExcerpt < 0 {
		t.Fatalf("missing headings in:\n%s", out)
	}
	if !(idxContract < idxDependence && idxDependence < idxExcerpt) {
		t.Fatalf("order wrong: contract=%d dependence=%d excerpt=%d", idxContract, idxDependence, idxExcerpt)
	}
}
