package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/structure"
)

// bugIProduceFixture builds the minimal produce→consumer topology shared by
// the BUG-418/419/421 tests: an inline context.produce node forwarding to an
// agent.delegate consumer (task-harness's context → plan_writer shape).
func bugIProduceFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	nodes := []agentpack.FlowNode{
		{ID: "context", Run: "inline", Behavior: "context.produce"},
		{ID: "plan_writer", Run: "delegate", Behavior: "agent.delegate",
			Agent: "agents/doc-writer.md", ContextProfile: "plan_writer"},
	}
	edges := []agentpack.FlowEdge{
		{From: "context", To: "plan_writer", When: "done", Kind: "forward"},
	}
	return edges, nodes
}

// writeBugICatalog seeds .flowpilot/catalog/features.ndjson with the given
// features — the live bed shape where a mega-glob noise feature coexists with
// the registered one.
func writeBugICatalog(t *testing.T, ws string, feats []featurecatalog.Feature) {
	t.Helper()
	dir := filepath.Join(ws, ".flowpilot", "catalog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fh, err := os.Create(filepath.Join(dir, "features.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	for _, f := range feats {
		b, _ := json.Marshal(f)
		if _, err := fh.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

// saveBugIContract persists a legacy (declared, not frozen) contract row for
// runID — the contract-declared-but-not-frozen window BUG-418/419 live in.
func saveBugIContract(t *testing.T, ws, runID, featureKey string, declaredPaths []string) {
	t.Helper()
	store, err := changecontract.NewStore(ws)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(changecontract.Contract{
		RunID:         runID,
		FeatureKey:    featureKey,
		Intent:        "fix the divide guard",
		DeclaredPaths: declaredPaths,
		Confidence:    changecontract.ConfidenceDeclared,
	}); err != nil {
		t.Fatalf("Save contract: %v", err)
	}
}

func planContextPackageFor(t *testing.T, svc *InteractiveService, runID string) *FlowContextPackage {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil || rs.planContextPackage == nil {
		t.Fatal("runContextProduceNode never stored a plan context package")
	}
	return rs.planContextPackage
}

// TestBug418_ProduceUsesDeclaredFeatureKey pins the mid-flow defect (3× live:
// runs 1560/2274/3131): context.produce re-resolved the feature from planner
// prose — provider names/"sandbox" tokens out-scored the contract's real
// feature_key. The declared contract row is the authority.
func TestBug418_ProduceUsesDeclaredFeatureKey(t *testing.T) {
	ws := t.TempDir()
	writeBugICatalog(t, ws, []featurecatalog.Feature{
		{Key: "calc-core", Keywords: []string{"calc", "divide"}},
		{Key: "sandbox-meta", Keywords: []string{"sandbox", "meta", "claude", "codex", "providers"}},
	})
	svc := newFreezeTestService(t)
	edges, nodes := bugIProduceFixture()
	runID := newFreezeTestRun(t, svc, ws, edges, nodes, "")
	saveBugIContract(t, ws, runID, "calc-core", []string{"calc.go"})

	ctxNode, _ := findFlowNode(nodes, "context")
	svc.runContextProduceNode(context.Background(), runID, edges, nodes, ctxNode,
		"plan the sandbox meta integration with claude and codex providers")

	pkg := planContextPackageFor(t, svc, runID)
	if pkg.FeatureKey != "calc-core" {
		t.Fatalf("produce re-resolved feature from prose: FeatureKey = %q, want contract's calc-core", pkg.FeatureKey)
	}
}

// TestBug419_ProduceSeedsBareDeclaredPaths pins the asymmetric excerpt gap:
// the frozen contract declares root-level files (calc.go — no slash, so the
// prose tokenizer can never recover it); the freeze-chain seeds
// rec.DeclaredPaths directly while mid-flow produce seeded only
// prose-extracted paths → source.excerpt stayed empty for the bug site file.
func TestBug419_ProduceSeedsBareDeclaredPaths(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "calc.go"),
		[]byte("package calc\n\nfunc Div(a, b int) int { return a / b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFreezeTestService(t)
	edges, nodes := bugIProduceFixture()
	runID := newFreezeTestRun(t, svc, ws, edges, nodes, "")
	saveBugIContract(t, ws, runID, "calc-core", []string{"calc.go"})

	ctxNode, _ := findFlowNode(nodes, "context")
	svc.runContextProduceNode(context.Background(), runID, edges, nodes, ctxNode,
		"investigate the division panic")

	pkg := planContextPackageFor(t, svc, runID)
	var excerptSec *FlowContextSection
	for i := range pkg.Sections {
		if pkg.Sections[i].SourceType == string(ContextSourceSourceExcerpt) {
			excerptSec = &pkg.Sections[i]
		}
	}
	if excerptSec == nil {
		t.Fatal("no source.excerpt section produced")
	}
	for _, ex := range excerptSec.Excerpts {
		if ex.Path == "calc.go" {
			return
		}
	}
	t.Fatalf("declared bare filename calc.go never excerpted: excerpts=%v omitted=%v",
		excerptSec.Excerpts, excerptSec.Omitted)
}

// TestBug421_ProduceResolvesConsumerProfile pins the dead wiring: packages
// were built from the producing node's (empty) ContextSources, so
// contextProfiles.<consumer>.candidateSources never reached the package —
// knowledge.flow could not appear in any builtin-flow prompt (CP-66
// BUG-LIVE-66-1). The consuming node's profile governs its prompt's package.
func TestBug421_ProduceResolvesConsumerProfile(t *testing.T) {
	ws := t.TempDir()
	seedKnowledgeFixture(t, ws)
	svc := newFreezeTestService(t)
	edges, nodes := bugIProduceFixture()
	runID := newFreezeTestRun(t, svc, ws, edges, nodes, "")
	// The run is a task-harness flow — plan_writer's profile opts into
	// knowledge.flow (candidateSources), the produce node itself has none.
	svc.mu.Lock()
	svc.runs[runID].chatFlowRef = "task-harness"
	svc.mu.Unlock()

	ctxNode, _ := findFlowNode(nodes, "context")
	svc.runContextProduceNode(context.Background(), runID, edges, nodes, ctxNode,
		"plan the checkout pipeline work")

	pkg := planContextPackageFor(t, svc, runID)
	for _, sec := range pkg.Sections {
		if sec.SourceType == "knowledge.flow" {
			return
		}
	}
	types := make([]string, 0, len(pkg.Sections))
	for _, sec := range pkg.Sections {
		types = append(types, sec.SourceType)
	}
	t.Fatalf("knowledge.flow absent from plan_writer's package — consumer candidateSources still dead: %v", types)
}

// TestBug417_SuggestFeatureKeysOnlyRegistered pins the r-fk reprompt defect:
// the suggested-key list contained auto-derived ledger keys (calc, claude)
// that are not registered FEATURE-KEYS.md entries — the reprompt exists to
// steer the agent onto a registered key, so unregistered catalog entries must
// never be suggested when a registry exists.
func TestBug417_SuggestFeatureKeysOnlyRegistered(t *testing.T) {
	ws := t.TempDir()
	writeBugICatalog(t, ws, []featurecatalog.Feature{
		{Key: "calc-core", Keywords: []string{"calc"}, FileGlobs: []string{"calc*.go"}},
		{Key: "claude", FileGlobs: []string{"calc.go", "requirements/**", ".claude/**", "stringutil/**"}},
	})
	if err := os.MkdirAll(filepath.Join(ws, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "change-audit", "FEATURE-KEYS.md"),
		[]byte("# Feature Keys\n\n- calc-core — calculator core\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := suggestFeatureKeys(filepath.Join(ws, ".flowpilot"), []string{"calc.go"}, "calc work")
	for _, k := range got {
		if k != "calc-core" {
			t.Fatalf("r-fk suggestions must only contain registered FEATURE-KEYS.md entries, got %v", got)
		}
	}
	if len(got) == 0 {
		t.Fatal("expected calc-core to be suggested")
	}
}

// TestBug420_DependenceWarnsWhenAllLookupsFail pins the silent-elision defect:
// every per-target `gitnexus impact` erroring (repo-name mismatch on a cloned
// bed) produced an empty body with no warning — the section vanished with
// zero signal. Failures must surface as section warnings.
func TestBug420_DependenceWarnsWhenAllLookupsFail(t *testing.T) {
	ws := t.TempDir()
	saveBugIContract(t, ws, "run-dep", "calc-core", []string{"calc.go"})

	src := &dependenceSource{priority: 3, newProvider: func(string, bool) structure.Provider {
		return stubDependenceProvider{err: errDependenceProbe}
	}}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		WorkflowRunID: "run-dep",
		Workspace:     ws,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(section.Warnings) == 0 {
		t.Fatal("all-targets-failed dependence lookups must surface a warning, got none")
	}
	if !strings.Contains(section.Warnings[0], "gitnexus") {
		t.Fatalf("warning should name the failing tool, got %q", section.Warnings[0])
	}
}

type stubDependenceProvider struct{ err error }

func (stubDependenceProvider) Available() bool { return true }
func (p stubDependenceProvider) Dependents(context.Context, string) (structure.DependentsSummary, error) {
	return structure.DependentsSummary{}, p.err
}

type errString string

func (e errString) Error() string { return string(e) }

var errDependenceProbe = errString(`gitnexus impact: Repository "lt-cp44" not found`)
