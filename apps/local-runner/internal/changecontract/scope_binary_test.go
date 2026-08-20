package changecontract

import (
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// run-208282: gatesandbox binary must not drive r-scope. Declared scope is
// source files; docs and build artifacts are excluded. Provider-agnostic.

func TestScopeDiffIgnoresBinaryArtifacts(t *testing.T) {
	c := Contract{
		Confidence:    ConfidenceDeclared,
		FeatureKey:    "calc-core",
		DeclaredPaths: []string{"calc.go", "calc_test.go"},
	}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "calc_test.go", Status: "M"},
		{Path: "gatesandbox", Status: "A"},
		{Path: "bin/app", Status: "A"},
		{Path: "change-audit/CA-914-calc-core-subtract-with-guard.md", Status: "A"},
		{Path: "requirements/08-Task/done/Task-910-calc-core-subtract-with-guard.md", Status: "A"},
	}
	out, _ := ScopeDiff(c, diff, nil)
	if len(out) != 0 {
		t.Fatalf("binary and docs must be ignored, got out=%v", out)
	}
}

func TestScopeDiffStillFlagsRealOutOfScope(t *testing.T) {
	c := Contract{
		Confidence:    ConfidenceDeclared,
		DeclaredPaths: []string{"calc.go"},
	}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "format.go", Status: "M"}, // real code outside declared
	}
	out, _ := ScopeDiff(c, diff, nil)
	if len(out) != 1 || out[0] != "format.go" {
		t.Fatalf("format.go must be out-of-scope, got %v", out)
	}
}

func TestScopeDiffIgnoresBinaryButNotCode(t *testing.T) {
	c := Contract{
		Confidence:    ConfidenceDeclared,
		DeclaredPaths: []string{"calc.go"},
	}
	diff := []flowgate.ChangedFile{
		{Path: "gatesandbox", Status: "A"},
		{Path: "format.go", Status: "M"},
	}
	out, _ := ScopeDiff(c, diff, nil)
	if len(out) != 1 || out[0] != "format.go" {
		t.Fatalf("only format.go must be flagged, got %v", out)
	}
}
