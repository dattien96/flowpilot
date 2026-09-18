package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/knowledge"
	"flowpilot-runner/internal/promptpacker"
	"flowpilot-runner/internal/structure"
)

// seedKnowledgeFixture distills the shared stub flows into ws.
func seedKnowledgeFixture(t *testing.T, ws string) {
	t.Helper()
	kb, err := knowledge.Distill(context.Background(), ws, stubKnowledgeLister{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledge.WriteFull(ws, kb); err != nil {
		t.Fatal(err)
	}
}

func fetchKnowledge(t *testing.T, ws string, hints FlowContextHints) FlowContextSection {
	t.Helper()
	hints.Workspace = ws
	src, err := DefaultContextSourceRegistry().Resolve("knowledge.flow")
	if err != nil {
		t.Fatalf("resolve knowledge.flow: %v", err)
	}
	section, err := src.Fetch(context.Background(), hints)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	return section
}

func TestKnowledgeFlowSourceKeyMatches(t *testing.T) {
	src, err := DefaultContextSourceRegistry().Resolve(string(ContextSourceKnowledgeFlow))
	if err != nil {
		t.Fatalf("registry must resolve knowledge.flow: %v", err)
	}
	if src.ID() != "knowledge.flow" {
		t.Errorf("ID = %q, want knowledge.flow", src.ID())
	}
	if !src.Deterministic() {
		t.Errorf("knowledge.flow must be deterministic (SD-22 D-2)")
	}
	// Duplicate registration is rejected per registry contract.
	r := NewContextSourceRegistry()
	if err := r.Register(&knowledgeFlowSource{}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(&knowledgeFlowSource{}); err == nil {
		t.Errorf("duplicate knowledge.flow register must fail")
	}
}

func TestKnowledgeFlowSourceResolvesFlowFromLocus(t *testing.T) {
	ws := t.TempDir()
	seedKnowledgeFixture(t, ws)
	section := fetchKnowledge(t, ws, FlowContextHints{ChangedPaths: []string{"shop/cart/service.go"}})
	if !strings.Contains(section.Body, "## Flow: Checkout_Pipeline") {
		t.Errorf("body missing checkout section:\n%s", section.Body)
	}
	if strings.Contains(section.Body, "## Flow: Login_Flow") {
		t.Errorf("body leaks unrelated flow:\n%s", section.Body)
	}
	if len(section.Warnings) != 0 {
		t.Errorf("clean match should warn nothing, got %v", section.Warnings)
	}
	// Symbol-shaped locus resolves too (qualified mention meets simple key).
	section = fetchKnowledge(t, ws, FlowContextHints{ExplicitSourcePaths: []string{"shop/auth/login.go"}})
	if !strings.Contains(section.Body, "## Flow: Login_Flow") {
		t.Errorf("explicit path did not resolve login flow:\n%s", section.Body)
	}
}

func TestKnowledgeFlowSourceReturnsEmptyGracefullyWhenMissing(t *testing.T) {
	section := fetchKnowledge(t, t.TempDir(), FlowContextHints{ChangedPaths: []string{"shop/cart/service.go"}})
	if section.Body != "" {
		t.Errorf("missing base must yield empty body, got %q", section.Body)
	}
	if len(section.Omitted) != 1 || section.Omitted[0] != knowledgeFlowOmittedMissing {
		t.Errorf("Omitted = %v, want [%s]", section.Omitted, knowledgeFlowOmittedMissing)
	}
	// Empty workspace + empty locus: also quiet, never an error.
	section = fetchKnowledge(t, t.TempDir(), FlowContextHints{})
	if section.Body != "" {
		t.Errorf("empty hints must yield empty body, got %q", section.Body)
	}
}

func TestKnowledgeFlowSourceRespectsTokenLimit(t *testing.T) {
	ws := t.TempDir()
	lister := &bulkFlowLister{}
	kb, err := knowledge.Distill(context.Background(), ws, lister, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledge.WriteFull(ws, kb); err != nil {
		t.Fatal(err)
	}
	section := fetchKnowledge(t, ws, FlowContextHints{ChangedPaths: []string{"shop/a.go", "shop/b.go", "shop/c.go"}})
	tokens := promptpacker.EstimateTokens(section.Body)
	if tokens > knowledgeFlowSectionCap {
		t.Errorf("packed body = %d tokens, want <= %d", tokens, knowledgeFlowSectionCap)
	}
	if !strings.Contains(section.Body, "## Flow: Flow_A") || !strings.Contains(section.Body, "## Flow: Flow_B") {
		t.Errorf("greedy pack must keep the first fitting sections:\n%s", section.Body)
	}
	if strings.Contains(section.Body, "## Flow: Flow_C") {
		t.Errorf("third section must be cut whole (never mid-section):\n%s", section.Body)
	}
}

// bulkFlowLister builds 3 ~230-token flows so the 500-token cap keeps two.
type bulkFlowLister struct{}

func (bulkFlowLister) ListProcesses(context.Context) ([]structure.FlowSummary, error) {
	mk := func(id, path string) structure.FlowSummary {
		var syms []structure.FlowSymbol
		for i := 0; i < 14; i++ {
			name := "Symbol" + itoaTest(i) + id
			syms = append(syms, structure.FlowSymbol{ID: "Function:" + path + ":" + name, Kind: "Function", Path: path, Name: name})
		}
		return structure.FlowSummary{ID: id, Label: "bulk flow " + id + " with a reasonably long purpose line for token weight", ProcessType: "cross_community", StepCount: 10, Symbols: syms}
	}
	return []structure.FlowSummary{mk("Flow_A", "shop/a.go"), mk("Flow_B", "shop/b.go"), mk("Flow_C", "shop/c.go")}, nil
}

func (bulkFlowLister) ListModelCandidates(context.Context) ([]structure.ModelInfo, error) {
	return nil, nil
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

func TestKnowledgeSymbolMatches(t *testing.T) {
	cases := []struct {
		index, locus string
		want         bool
	}{
		{"Checkout", "Checkout", true},
		{"Checkout", "cart.Checkout", true},
		{"cart.Checkout", "Checkout", true},
		{"Method:shop/cart/service.go:Checkout", "Checkout", true},
		{"Checkout", "Method:shop/cart/service.go:Checkout", true},
		{"Checkout", "Checkin", false},
		{"CheckoutFlow", "Checkout", false}, // suffix must break on a segment separator
		{"", "Checkout", false},
	}
	for _, c := range cases {
		if got := knowledgeSymbolMatches(c.index, c.locus); got != c.want {
			t.Errorf("matches(%q, %q) = %v, want %v", c.index, c.locus, got, c.want)
		}
	}
}

func TestSelectFlowSectionsSkipsOverBudgetSingle(t *testing.T) {
	big := matchedFlowSection{id: "Huge", tokens: knowledge.MaxSectionTokens + 1, body: "BIG"}
	small := matchedFlowSection{id: "Small", tokens: 100, body: "SMALL"}
	picked, skipped := selectFlowSections([]matchedFlowSection{big, small}, knowledgeFlowSectionCap)
	if len(picked) != 1 || picked[0] != "SMALL" {
		t.Errorf("picked = %v, want [SMALL]", picked)
	}
	if len(skipped) != 1 || skipped[0] != "Huge" {
		t.Errorf("skipped = %v, want [Huge]", skipped)
	}
}
