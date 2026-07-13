package changecontract

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/structure"
)

func TestScopeDiffInScopeYieldsEmpty(t *testing.T) {
	c := Contract{DeclaredPaths: []string{"calc.go", "calc_test.go"}}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "calc_test.go", Status: "M"},
	}
	outPaths, outSymbols := ScopeDiff(c, diff, nil)
	if len(outPaths) != 0 {
		t.Errorf("outPaths = %v, want empty for an entirely in-scope diff", outPaths)
	}
	if len(outSymbols) != 0 {
		t.Errorf("outSymbols = %v, want empty", outSymbols)
	}
}

func TestScopeDiffFlagsOutOfScopePaths(t *testing.T) {
	c := Contract{DeclaredPaths: []string{"calc.go"}}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "user.go", Status: "M"},
	}
	outPaths, _ := ScopeDiff(c, diff, nil)
	want := []string{"user.go"}
	if !reflect.DeepEqual(outPaths, want) {
		t.Errorf("outPaths = %v, want %v", outPaths, want)
	}
}

func TestScopeDiffMatchesDirectoryPrefix(t *testing.T) {
	c := Contract{DeclaredPaths: []string{"apps"}} // e.g. from InferFromDiff's top-level bucketing
	diff := []flowgate.ChangedFile{
		{Path: "apps/local-runner/foo.go", Status: "M"},
		{Path: "docs/readme_helper.go", Status: "M"},
	}
	outPaths, _ := ScopeDiff(c, diff, nil)
	want := []string{"docs/readme_helper.go"}
	if !reflect.DeepEqual(outPaths, want) {
		t.Errorf("outPaths = %v, want %v", outPaths, want)
	}
}

func TestScopeDiffMatchesGlob(t *testing.T) {
	c := Contract{DeclaredPaths: []string{"*.go"}}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "change-audit/CA-1.md", Status: "A"}, // excluded regardless (doc file)
	}
	outPaths, _ := ScopeDiff(c, diff, nil)
	if len(outPaths) != 0 {
		t.Errorf("outPaths = %v, want empty (glob match + doc exclusion)", outPaths)
	}
}

func TestScopeDiffExcludesDocAndAuditFiles(t *testing.T) {
	c := Contract{DeclaredPaths: []string{"calc.go"}}
	diff := []flowgate.ChangedFile{
		{Path: "calc.go", Status: "M"},
		{Path: "requirements/09-BugFix/done/BUG-1.md", Status: "A"},
		{Path: "change-audit/CA-1.md", Status: "A"},
	}
	outPaths, _ := ScopeDiff(c, diff, nil)
	if len(outPaths) != 0 {
		t.Errorf("outPaths = %v, want empty — doc/audit files must never count as drift", outPaths)
	}
}

func TestScopeDiffNoDeclaredPathsYieldsNoDrift(t *testing.T) {
	c := Contract{DeclaredPaths: nil}
	diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}}
	outPaths, _ := ScopeDiff(c, diff, nil)
	if outPaths != nil {
		t.Errorf("outPaths = %v, want nil when there is nothing declared to compare against", outPaths)
	}
}

// fakeProvider is a minimal structure.Provider test double.
type fakeProvider struct {
	available bool
	deps      map[string]structure.DependentsSummary
	err       error
}

func (f *fakeProvider) Available() bool { return f.available }
func (f *fakeProvider) Dependents(_ context.Context, target string) (structure.DependentsSummary, error) {
	if f.err != nil {
		return structure.DependentsSummary{}, f.err
	}
	return f.deps[target], nil
}

func TestHighSeverityFalseWhenProviderUnavailable(t *testing.T) {
	sp := &fakeProvider{available: false, deps: map[string]structure.DependentsSummary{"user.go": {Count: 5}}}
	if HighSeverity(context.Background(), sp, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=false when structure.Available() is false, regardless of dependents")
	}
}

func TestHighSeverityFalseWhenNoDependents(t *testing.T) {
	sp := &fakeProvider{available: true, deps: map[string]structure.DependentsSummary{"user.go": {Count: 0}}}
	if HighSeverity(context.Background(), sp, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=false when no out-of-scope path has dependents")
	}
}

func TestHighSeverityTrueWhenDependentsExist(t *testing.T) {
	sp := &fakeProvider{available: true, deps: map[string]structure.DependentsSummary{"user.go": {Count: 2}}}
	if !HighSeverity(context.Background(), sp, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=true when an out-of-scope path has dependents")
	}
}

func TestHighSeverityIgnoresDependentsError(t *testing.T) {
	sp := &fakeProvider{available: true, err: errors.New("boom")}
	if HighSeverity(context.Background(), sp, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=false (non-fatal) when Dependents errors")
	}
}

func TestHighSeverityFalseForNilProvider(t *testing.T) {
	if HighSeverity(context.Background(), nil, []string{"user.go"}) {
		t.Fatal("expected HighSeverity=false for a nil provider")
	}
}
