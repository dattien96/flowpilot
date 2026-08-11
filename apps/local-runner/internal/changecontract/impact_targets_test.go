package changecontract

import (
	"context"
	"reflect"
	"testing"

	"flowpilot-runner/internal/structure"
)

func TestParseDeclarationSymbolsLine(t *testing.T) {
	text := `[Change Contract]
feature: calc-core
intent: add helper
files: calc.go
symbols: DivideChecked, ParseDeclaration
`
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := []string{"DivideChecked", "ParseDeclaration"}
	if !reflect.DeepEqual(c.DeclaredSymbols, want) {
		t.Errorf("DeclaredSymbols = %v, want %v", c.DeclaredSymbols, want)
	}
}

func TestGitNexusImpactTargetsSymbolsBeforePaths(t *testing.T) {
	c := Contract{
		DeclaredSymbols: []string{"Bar", "ScopeDiff"},
		DeclaredPaths: []string{
			"apps/local-runner/internal/runner/foo.go",
			"apps", // dir bucket
			"internal/**",
			"requirements/x.md",
			"-rf",
		},
	}
	got := GitNexusImpactTargets(c)
	want := []string{"Bar", "Foo", "ScopeDiff"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GitNexusImpactTargets = %v, want %v", got, want)
	}
}

func TestGitNexusQueryTargetsForPathSymbolFirst(t *testing.T) {
	got := GitNexusQueryTargetsForPath("user.go")
	want := []string{"User", "user.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GitNexusQueryTargetsForPath = %v, want %v", got, want)
	}
}

func TestPathBasenameToGitNexusSymbol(t *testing.T) {
	cases := map[string]string{
		"gate_hook.go":                              "GateHook",
		"apps/local-runner/internal/runner/x.go":    "X",
		"scope.go":                                  "Scope",
		"http-local-runner-gateway.ts":              "HttpLocalRunnerGateway",
	}
	for path, want := range cases {
		if got := pathBasenameToGitNexusSymbol(path); got != want {
			t.Errorf("pathBasenameToGitNexusSymbol(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHighSeverityUsesSymbolThenPathFallback(t *testing.T) {
	sp := &fakeProvider{
		available: true,
		deps: map[string]structure.DependentsSummary{
			"User":    {Count: 0},
			"user.go": {Count: 2},
		},
	}
	if !HighSeverity(context.Background(), sp, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=true via path fallback after symbol miss")
	}
	if len(sp.deps) == 0 {
		t.Fatal("fake deps map missing")
	}
}
