package changecontract

import (
	"reflect"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

func TestInferFromDiffBucketsByTopLevelDir(t *testing.T) {
	diff := []flowgate.ChangedFile{
		{Path: "apps/local-runner/internal/changecontract/contract.go", Status: "A"},
		{Path: "apps/local-runner/internal/changecontract/parse.go", Status: "A"},
		{Path: "apps/desktop-flowpilot/src/App.tsx", Status: "M"},
	}
	c := InferFromDiff("calc-core", diff)

	if c.FeatureKey != "calc-core" {
		t.Errorf("FeatureKey = %q, want calc-core", c.FeatureKey)
	}
	if c.Confidence != ConfidenceInferred {
		t.Errorf("Confidence = %q, want %q", c.Confidence, ConfidenceInferred)
	}
	want := []string{"apps"} // both top-level dirs collapse to "apps"
	if !reflect.DeepEqual(c.DeclaredPaths, want) {
		t.Errorf("DeclaredPaths = %v, want %v", c.DeclaredPaths, want)
	}
}

func TestInferFromDiffExcludesDocAndAuditFiles(t *testing.T) {
	diff := []flowgate.ChangedFile{
		{Path: "requirements/09-BugFix/done/BUG-1.md", Status: "A"},
		{Path: "change-audit/CA-1.md", Status: "A"},
		{Path: "README.md", Status: "M"},
		{Path: "calc.go", Status: "M"},
	}
	c := InferFromDiff("calc-core", diff)

	want := []string{"calc.go"} // root-level non-doc file buckets to itself
	if !reflect.DeepEqual(c.DeclaredPaths, want) {
		t.Errorf("DeclaredPaths = %v, want %v (docs/audit excluded)", c.DeclaredPaths, want)
	}
}

func TestInferFromDiffEmptyDiffYieldsNilPaths(t *testing.T) {
	c := InferFromDiff("calc-core", nil)
	if c.DeclaredPaths != nil {
		t.Errorf("DeclaredPaths = %v, want nil for an empty diff", c.DeclaredPaths)
	}
}

func TestInferFromDiffAllDocsYieldsNilPaths(t *testing.T) {
	diff := []flowgate.ChangedFile{
		{Path: "requirements/07-Coding-Plan/todo/CP-1.md", Status: "M"},
	}
	c := InferFromDiff("calc-core", diff)
	if c.DeclaredPaths != nil {
		t.Errorf("DeclaredPaths = %v, want nil when every changed file is doc/audit", c.DeclaredPaths)
	}
}

func TestTopLevelDirRootFileBucketsToItself(t *testing.T) {
	if got := topLevelDir("go.mod"); got != "go.mod" {
		t.Errorf("topLevelDir(go.mod) = %q, want go.mod", got)
	}
	if got := topLevelDir("apps/local-runner/main.go"); got != "apps" {
		t.Errorf("topLevelDir(...) = %q, want apps", got)
	}
}
