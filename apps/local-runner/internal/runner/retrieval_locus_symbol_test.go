package runner

import (
	"reflect"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

func TestBuildRetrievalLocusPopulatesSymbolsFromDeclaredPaths(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{"src/gate_hook.go"})

	locus := buildRetrievalLocus(ws, "run-1", "", nil)

	want := []string{"GateHook"}
	if !reflect.DeepEqual(locus.Symbols, want) {
		t.Fatalf("Symbols = %v, want %v", locus.Symbols, want)
	}
}

func TestBuildRetrievalLocusMergesDeclaredSymbolsAndPathDerived(t *testing.T) {
	ws := newLocusRepo(t)
	store, err := changecontract.NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{
		RunID:           "run-1",
		StepID:          "step-1",
		FeatureKey:      "calc-core",
		Intent:          "test",
		DeclaredPaths:   []string{"src/calc.go"},
		DeclaredSymbols: []string{"ScopeDiff"},
		Confidence:      changecontract.ConfidenceDeclared,
	}); err != nil {
		t.Fatal(err)
	}

	locus := buildRetrievalLocus(ws, "run-1", "", nil)

	want := []string{"Calc", "ScopeDiff"}
	if !reflect.DeepEqual(locus.Symbols, want) {
		t.Fatalf("Symbols = %v, want %v", locus.Symbols, want)
	}
}

func TestBuildRetrievalLocusDerivesSymbolsFromExplicitPaths(t *testing.T) {
	ws := newLocusRepo(t)
	locus := buildRetrievalLocus(ws, "", "", []string{"internal/flow_context_pack_budget.go"})

	want := []string{"FlowContextPackBudget"}
	if !reflect.DeepEqual(locus.Symbols, want) {
		t.Fatalf("Symbols = %v, want %v", locus.Symbols, want)
	}
}
