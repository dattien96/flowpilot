package flowgate

import "testing"

// Task-260: oracle IsOverridden(filepath.Base) must clear tamper — proves Critical fix.
func TestTask260OracleOverrideClearsTamper(t *testing.T) {
	repoDir := t.TempDir()
	bl := &Baseline{
		CapturedAt: "2024-01-01T00:00:00Z",
		GreenTests: []string{},
		TestCmd:    "go test ./...",
	}
	diff := []ChangedFile{
		{Path: "internal/foo/foo_test.go", Status: "M"},
	}
	overrides := map[string]Override{
		"foo_test.go": {TestName: "foo_test.go"},
	}
	result := RunOracle(repoDir, bl, diff, overrides)
	if result.HasTampering {
		t.Fatalf("overridden foo_test.go must not be tampered, got %v", result.Tampered)
	}
	if len(result.Tampered) != 0 {
		t.Fatalf("Tampered = %v, want []", result.Tampered)
	}
	// Without override, same diff must still tamper
	result2 := RunOracle(repoDir, bl, diff, nil)
	if !result2.HasTampering {
		t.Fatal("without override, foo_test.go M must be tampered")
	}
}

func TestTask260OracleOverrideWithTamperedPathsIntegration(t *testing.T) {
	// End-to-end: oracle filtered Tampered == nil/empty → Evaluate r-additive-tests must NOT fire
	repoDir := t.TempDir()
	bl := &Baseline{
		CapturedAt: "2024-01-01T00:00:00Z",
		GreenTests: []string{},
		TestCmd:    "go test ./...",
	}
	diff := []ChangedFile{
		{Path: "pkg/bar_test.go", Status: "M"},
		{Path: "pkg/service.go", Status: "M"},
	}
	overrides := map[string]Override{
		"bar_test.go": {TestName: "bar_test.go"},
	}
	oracle := RunOracle(repoDir, bl, diff, overrides)
	if oracle.HasTampering || len(oracle.Tampered) != 0 {
		t.Fatalf("overridden bar_test.go must not tamper, got %v", oracle.Tampered)
	}
	tr := TurnResult{
		GitDiff:           diff,
		TamperedTestPaths: append([]string(nil), oracle.Tampered...),
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire when oracle filtered override, got %v", v.Detail)
		}
	}
}
