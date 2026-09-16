package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/promptpacker"
	"flowpilot-runner/internal/structure"
)

// fakeLister scripts process/model data (Task-373 T-5 doubles).
type fakeLister struct {
	flows     []structure.FlowSummary
	models    []structure.ModelInfo
	flowsErr  error
	modelsErr error
}

func (f *fakeLister) ListProcesses(context.Context) ([]structure.FlowSummary, error) {
	if f.flowsErr != nil {
		return nil, f.flowsErr
	}
	return f.flows, nil
}

func (f *fakeLister) ListModelCandidates(context.Context) ([]structure.ModelInfo, error) {
	if f.modelsErr != nil {
		return nil, f.modelsErr
	}
	return f.models, nil
}

func shopLister() *fakeLister {
	return &fakeLister{
		flows: []structure.FlowSummary{
			{
				ID: "Checkout_Pipeline", Label: "Cart to order through payment",
				ProcessType: "cross_community", StepCount: 8,
				Symbols: []structure.FlowSymbol{
					{ID: "Method:shop/cart/service.go:Checkout", Kind: "Method", Path: "shop/cart/service.go", Name: "Checkout"},
					{ID: "Function:shop/pay/charge.go:Charge", Kind: "Function", Path: "shop/pay/charge.go", Name: "Charge"},
				},
			},
			{
				ID: "Login_Flow", Label: "Credential check and session issue",
				ProcessType: "local", StepCount: 4,
				Symbols: []structure.FlowSymbol{
					{ID: "Function:shop/auth/login.go:Login", Kind: "Function", Path: "shop/auth/login.go", Name: "Login"},
				},
			},
		},
		models: []structure.ModelInfo{
			{ID: "Struct:shop/cart/model.go:Order", Kind: "Struct", Path: "shop/cart/model.go", Name: "Order", FlowCount: 5},
		},
	}
}

func TestDistillerGeneratesSystemOverview(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/shop\n\ngo 1.24\n\nrequire github.com/a/b v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"shop", "pkg"} {
		if err := os.MkdirAll(filepath.Join(ws, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	kb, err := Distill(context.Background(), ws, shopLister(), nil)
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	for _, want := range []string{"## Tech Stack", "## Layer Boundaries", "## Core Libraries", "example.com/shop", "github.com/a/b", "`shop`", "Execution flows indexed: 2"} {
		if !strings.Contains(kb.Overview, want) {
			t.Errorf("overview missing %q\n%s", want, kb.Overview)
		}
	}
}

func TestDistillerGeneratesExecutionFlows(t *testing.T) {
	kb, err := Distill(context.Background(), t.TempDir(), shopLister(), nil)
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if len(kb.Flows) != 2 {
		t.Fatalf("want 2 flows, got %d", len(kb.Flows))
	}
	byID := make(map[string]RenderedFlow)
	for _, r := range kb.Flows {
		byID[r.ID] = r
		if r.Heading != "## Flow: "+r.ID {
			t.Errorf("bad heading %q", r.Heading)
		}
		if r.File != "execution-flows.md" {
			t.Errorf("unsharded flow must target execution-flows.md, got %q", r.File)
		}
		if got := promptpacker.EstimateTokens(r.Body); got > MaxSectionTokens {
			t.Errorf("flow %s section %d tokens, want <= %d", r.ID, got, MaxSectionTokens)
		}
	}
	co := byID["Checkout_Pipeline"]
	for _, want := range []string{"Cart to order", "`Checkout`", "`Charge`", "shop/cart/service.go", "shop/pay/charge.go"} {
		if !strings.Contains(co.Body, want) {
			t.Errorf("checkout section missing %q\n%s", want, co.Body)
		}
	}
	if strings.Contains(co.Body, "Login") {
		t.Errorf("checkout section leaks the other flow\n%s", co.Body)
	}
	// Index contract: path/symbol -> section, file+heading placement.
	entry, ok := kb.Index.Flows["Checkout_Pipeline"]
	if !ok || entry.File != "execution-flows.md" || entry.Heading != "## Flow: Checkout_Pipeline" || entry.Tokens <= 0 {
		t.Errorf("bad index entry: %+v ok=%v", entry, ok)
	}
	if !contains(kb.Index.Paths["shop/cart/service.go"], "Checkout_Pipeline") {
		t.Errorf("paths reverse map missing: %v", kb.Index.Paths)
	}
	if !contains(kb.Index.Symbols["Charge"], "Checkout_Pipeline") {
		t.Errorf("symbols reverse map missing: %v", kb.Index.Symbols)
	}
	if kb.Index.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", kb.Index.SchemaVersion, SchemaVersion)
	}
}

func TestDistillerIncrementalUpdateOnChangedFiles(t *testing.T) {
	ws := t.TempDir()
	kb1, err := Distill(context.Background(), ws, shopLister(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFull(ws, kb1); err != nil {
		t.Fatalf("WriteFull: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(KnowledgeDir(ws), "execution-flows.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Fresh base: only the checkout purpose changed.
	lister2 := shopLister()
	lister2.flows[0].Label = "Cart to order through payment WITH refunds"
	kb2, err := Distill(context.Background(), ws, lister2, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = IncrementalUpdate(ws, []string{"shop/pay/charge.go"}, func(context.Context) (*KnowledgeBase, error) {
		calls++
		return kb2, nil
	})
	if err != nil {
		t.Fatalf("IncrementalUpdate: %v", err)
	}
	if calls != 1 {
		t.Errorf("redistill calls = %d, want 1", calls)
	}
	after, err := os.ReadFile(filepath.Join(KnowledgeDir(ws), "execution-flows.md"))
	if err != nil {
		t.Fatal(err)
	}
	secBefore := sectionOf(t, string(before), "## Flow: Checkout_Pipeline")
	secAfter := sectionOf(t, string(after), "## Flow: Checkout_Pipeline")
	if secBefore == secAfter {
		t.Errorf("affected section was not rewritten")
	}
	if !strings.Contains(secAfter, "WITH refunds") {
		t.Errorf("affected section missing fresh content:\n%s", secAfter)
	}
	otherBefore := sectionOf(t, string(before), "## Flow: Login_Flow")
	otherAfter := sectionOf(t, string(after), "## Flow: Login_Flow")
	if otherBefore != otherAfter {
		t.Errorf("untouched section changed:\n--- before ---\n%s\n--- after ---\n%s", otherBefore, otherAfter)
	}
	// Index stays in sync with the fresh base.
	idx, err := LoadIndex(ws)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Flows["Checkout_Pipeline"].Tokens != kb2.Index.Flows["Checkout_Pipeline"].Tokens {
		t.Errorf("index tokens not synced: %+v", idx.Flows["Checkout_Pipeline"])
	}
}

func TestDistillerIncrementalUpdateSkipsUnrelatedPaths(t *testing.T) {
	ws := t.TempDir()
	kb1, err := Distill(context.Background(), ws, shopLister(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFull(ws, kb1); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(KnowledgeDir(ws), "execution-flows.md"))
	calls := 0
	if err := IncrementalUpdate(ws, []string{"docs/notes.md"}, func(context.Context) (*KnowledgeBase, error) {
		calls++
		return kb1, nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("unrelated path triggered redistill (%d calls)", calls)
	}
	after, _ := os.ReadFile(filepath.Join(KnowledgeDir(ws), "execution-flows.md"))
	if string(before) != string(after) {
		t.Errorf("unrelated path rewrote the flows file")
	}
}

func TestDistillerDegradesWithoutModelsOrSymbols(t *testing.T) {
	// AC-4: no LSP lister, no model candidates — still a complete base.
	kb, err := Distill(context.Background(), t.TempDir(), &fakeLister{flows: shopLister().flows}, nil)
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if len(kb.Flows) != 2 || kb.Overview == "" || kb.Models == "" {
		t.Errorf("degraded distill incomplete: flows=%d overview=%d models=%d", len(kb.Flows), len(kb.Overview), len(kb.Models))
	}
}

func TestDistillerEmptyIndexStillWritesBase(t *testing.T) {
	ws := t.TempDir()
	kb, err := Distill(context.Background(), ws, &fakeLister{}, nil)
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if len(kb.Flows) != 0 {
		t.Fatalf("want 0 flows, got %d", len(kb.Flows))
	}
	if err := WriteFull(ws, kb); err != nil {
		t.Fatalf("WriteFull: %v", err)
	}
	if Missing(ws) {
		t.Errorf("base should exist after WriteFull")
	}
}

func TestDistillerShrinksOversizedSection(t *testing.T) {
	var syms []structure.FlowSymbol
	for i := 0; i < 200; i++ {
		syms = append(syms, structure.FlowSymbol{
			ID:   "Function:shop/big/f" + itoa(i) + ".go:Fn" + itoa(i),
			Kind: "Function", Path: "shop/big/f" + itoa(i) + ".go", Name: "Fn" + itoa(i),
		})
	}
	lister := &fakeLister{flows: []structure.FlowSummary{
		{ID: "Mega_Flow", Label: "a very large flow", ProcessType: "cross_community", StepCount: 200, Symbols: syms},
	}}
	kb, err := Distill(context.Background(), t.TempDir(), lister, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(kb.Flows) != 1 {
		t.Fatalf("want 1 flow, got %d", len(kb.Flows))
	}
	if got := promptpacker.EstimateTokens(kb.Flows[0].Body); got > MaxSectionTokens {
		t.Errorf("oversized section = %d tokens, want <= %d", got, MaxSectionTokens)
	}
	if !strings.Contains(kb.Flows[0].Body, "## Flow: Mega_Flow") {
		t.Errorf("shrunk section lost its heading")
	}
}

func TestWriterShardsPast500Flows(t *testing.T) {
	ws := t.TempDir()
	var flows []structure.FlowSummary
	for i := 0; i < 505; i++ {
		flows = append(flows, structure.FlowSummary{
			ID: "Flow_" + itoa(i), Label: "flow number " + itoa(i),
			ProcessType: "cross_community", StepCount: i,
			Symbols: []structure.FlowSymbol{
				{ID: "Function:shop/m" + itoa(i) + ".go:Fn", Kind: "Function", Path: "shop/m" + itoa(i) + ".go", Name: "Fn"},
			},
		})
	}
	kb, err := Distill(context.Background(), ws, &fakeLister{flows: flows}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFull(ws, kb); err != nil {
		t.Fatalf("WriteFull: %v", err)
	}
	dir := KnowledgeDir(ws)
	if _, err := os.Stat(filepath.Join(dir, "flows")); err != nil {
		t.Fatalf("shard dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "flows", "manifest.md")); err != nil {
		t.Errorf("shard manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "execution-flows.md")); !os.IsNotExist(err) {
		t.Errorf("single flows file must be removed when sharded")
	}
	idx, err := LoadIndex(ws)
	if err != nil {
		t.Fatal(err)
	}
	entry := idx.Flows["Flow_7"]
	if !strings.HasPrefix(entry.File, "flows/") {
		t.Errorf("sharded index entry points at %q", entry.File)
	}
	if _, err := os.Stat(filepath.Join(dir, entry.File)); err != nil {
		t.Errorf("indexed shard file missing: %v", err)
	}
}

func TestIncrementalUpdateRebuildsOnCorruptIndex(t *testing.T) {
	ws := t.TempDir()
	kb1, err := Distill(context.Background(), ws, shopLister(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFull(ws, kb1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(KnowledgeDir(ws), "index.json"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := IncrementalUpdate(ws, []string{"shop/cart/service.go"}, func(context.Context) (*KnowledgeBase, error) {
		calls++
		return kb1, nil
	}); err != nil {
		t.Fatalf("corrupt index should fall back to rebuild, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("rebuild redistill calls = %d, want 1", calls)
	}
	idx, err := LoadIndex(ws)
	if err != nil {
		t.Fatalf("index not readable after rebuild: %v", err)
	}
	if idx.SchemaVersion != SchemaVersion {
		t.Errorf("rebuilt schema = %d", idx.SchemaVersion)
	}
}

func sectionOf(t *testing.T, content, heading string) string {
	t.Helper()
	start := strings.Index(content, heading)
	if start < 0 {
		t.Fatalf("heading %q not found", heading)
	}
	rest := content[start:]
	next := strings.Index(rest[len(heading):], "\n## Flow: ")
	if next < 0 {
		return rest
	}
	return rest[:len(heading)+next]
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
