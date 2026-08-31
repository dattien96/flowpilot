package flowgate

import (
	"strings"
	"testing"
)

// Task-260: r-additive-tests — hard enforce additive-tests-only (replaces synthetic r-tamper warn).

func TestRAdditiveTestsInDefaultRules(t *testing.T) {
	found := false
	for _, r := range DefaultRules() {
		if r.ID == "r-additive-tests" {
			found = true
			if r.Trigger != "pre_existing_test_edited" {
				t.Fatalf("r-additive-tests trigger = %q, want pre_existing_test_edited", r.Trigger)
			}
			if r.Action != "reprompt" {
				t.Fatalf("r-additive-tests action = %q, want reprompt", r.Action)
			}
			if !r.Enabled {
				t.Fatal("r-additive-tests must be enabled")
			}
			if r.RequiredOutput != "additive_tests_only_or_user_approved_legacy_edit" {
				t.Fatalf("r-additive-tests required_output = %q", r.RequiredOutput)
			}
			if r.Scope != "step" {
				t.Fatalf("r-additive-tests scope = %q, want step", r.Scope)
			}
		}
	}
	if !found {
		t.Fatal("DefaultRules must contain r-additive-tests (Task-260)")
	}
}

func TestMergeDefaultRulesPreservesRAdditiveTests(t *testing.T) {
	merged := MergeDefaultRules(nil)
	found := false
	for _, r := range merged {
		if r.ID == "r-additive-tests" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("MergeDefaultRules(nil) must contain r-additive-tests")
	}
	// Old stored file without r-additive-tests should get it appended.
	old := []Rule{{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}}
	merged2 := MergeDefaultRules(old)
	found = false
	for _, r := range merged2 {
		if r.ID == "r-additive-tests" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("MergeDefaultRules must append missing r-additive-tests")
	}
}

func TestRAdditiveTestsFiresOnTamperedTestPaths(t *testing.T) {
	tr := TurnResult{
		TamperedTestPaths: []string{"pkg/foo_test.go"},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			found = true
			if !strings.Contains(v.Detail, "foo_test.go") {
				t.Fatalf("detail %q must contain tampered path", v.Detail)
			}
		}
	}
	if !found {
		t.Fatal("r-additive-tests must fire when TamperedTestPaths non-empty")
	}
}

func TestRAdditiveTestsFiresOnModifiedTestFileViaTamperedPaths(t *testing.T) {
	// After Critical fix, GitDiff alone does NOT fire — TamperedTestPaths (oracle) is authoritative.
	// GitDiff M without Tampered must not fire (override case).
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "internal/bar_test.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire on GitDiff M alone (needs TamperedTestPaths), got %v", v.Detail)
		}
	}
	// Same path via TamperedTestPaths must fire.
	tr2 := TurnResult{
		TamperedTestPaths: []string{"internal/bar_test.go"},
		GitDiff:           []ChangedFile{{Path: "internal/bar_test.go", Status: "M"}},
	}
	violations2 := Evaluate(tr2, DefaultRules())
	found := false
	for _, v := range violations2 {
		if v.Rule.ID == "r-additive-tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("r-additive-tests must fire when TamperedTestPaths non-empty")
	}
}

func TestRAdditiveTestsPolyglotKotlinViaTamperedPaths(t *testing.T) {
	if !IsTestFile("src/test/java/com/example/AddTest.kt") {
		t.Fatal("IsTestFile must recognise *Test.kt")
	}
	// GitDiff alone must not fire
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "src/test/java/com/example/AddTest.kt", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("Kotlin GitDiff M alone must NOT fire, got %v", v.Detail)
		}
	}
	// Via TamperedTestPaths must fire
	tr2 := TurnResult{
		TamperedTestPaths: []string{"src/test/java/com/example/AddTest.kt"},
	}
	violations2 := Evaluate(tr2, DefaultRules())
	found := false
	for _, v := range violations2 {
		if v.Rule.ID == "r-additive-tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("r-additive-tests must fire for Kotlin test file via TamperedTestPaths")
	}
}

func TestRAdditiveTestsDoesNotFireOnAddedTestFile(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "pkg/new_feature_test.go", Status: "A"}},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire on pure A (new test file), got %v", v.Detail)
		}
	}
	// Also via TamperedTestPaths empty + A diff.
	tr2 := TurnResult{
		TamperedTestPaths: nil,
		GitDiff:           []ChangedFile{{Path: "pkg/new_feature_test.go", Status: "A"}},
	}
	violations2 := Evaluate(tr2, DefaultRules())
	for _, v := range violations2 {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire on pure A via TamperedTestPaths empty")
		}
	}
	// Polyglot A variants must also not fire
	for _, path := range []string{"src/foo.test.ts", "src/test/java/com/example/AddTest.kt", "pkg/app_test.go"} {
		tr3 := TurnResult{
			GitDiff: []ChangedFile{{Path: path, Status: "A"}},
		}
		violations3 := Evaluate(tr3, DefaultRules())
		for _, v := range violations3 {
			if v.Rule.ID == "r-additive-tests" {
				t.Fatalf("r-additive-tests must NOT fire on A %q", path)
			}
		}
	}
}

func TestRAdditiveTestsDoesNotFireWhenOnlyTamperedEmptyAndProdChanged(t *testing.T) {
	// Production code change without tampered test should not fire r-additive-tests
	// (r-newtest is separate). This ensures adding a new test satisfies r-newtest
	// without false r-additive-tests.
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "pkg/service.go", Status: "M"},
			{Path: "pkg/service_new_test.go", Status: "A"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire when only prod M + new test A, got %v", v.Detail)
		}
	}
}

func TestRAdditiveTestsNotFiringWhenFilteredViaOverrides(t *testing.T) {
	// Simulate oracle already filtered overrides → TamperedTestPaths empty.
	// GitDiff still has M test file but oracle filtered it, so gate must not fire.
	// After Critical fix, GitDiff fallback is removed, so empty Tampered must not fire.
	for _, paths := range [][]string{nil, {}} {
		tr := TurnResult{
			TamperedTestPaths: paths, // oracle filtered (nil or empty)
			GitDiff:           []ChangedFile{{Path: "pkg/foo_test.go", Status: "M"}},
		}
		violations := Evaluate(tr, DefaultRules())
		for _, v := range violations {
			if v.Rule.ID == "r-additive-tests" {
				t.Fatalf("r-additive-tests must NOT fire when overridden (Tampered empty, GitDiff M), got %v", v.Detail)
			}
		}
	}
	// Also empty Tampered + empty GitDiff must not fire
	tr2 := TurnResult{
		TamperedTestPaths: nil,
		GitDiff:           nil,
	}
	violations := Evaluate(tr2, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatal("r-additive-tests must not fire when TamperedTestPaths empty and no GitDiff M")
		}
	}
	// Non-empty Tampered must fire even though GitDiff same M
	tr3 := TurnResult{
		TamperedTestPaths: []string{"pkg/foo_test.go"},
		GitDiff:           []ChangedFile{{Path: "pkg/foo_test.go", Status: "M"}},
	}
	violations3 := Evaluate(tr3, DefaultRules())
	found := false
	for _, v := range violations3 {
		if v.Rule.ID == "r-additive-tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("r-additive-tests must fire when TamperedTestPaths non-empty (override not present)")
	}
}

func TestRAdditiveTestsFiresOnDRCStatuses(t *testing.T) {
	for _, status := range []string{"D", "R", "C"} {
		// D/R/C via TamperedTestPaths (oracle maps R/C -> M and filters IsTestFile)
		path := "pkg/old_test.go"
		// IsTestFile must hold
		if !IsTestFile(path) {
			t.Fatalf("IsTestFile(%q) must be true", path)
		}
		tr := TurnResult{
			TamperedTestPaths: []string{path + ":" + status},
			GitDiff:           []ChangedFile{{Path: path, Status: status}},
		}
		violations := Evaluate(tr, DefaultRules())
		found := false
		for _, v := range violations {
			if v.Rule.ID == "r-additive-tests" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("r-additive-tests must fire on tampered status %q", status)
		}
		// GitDiff alone without Tampered must NOT fire (override-safe)
		tr2 := TurnResult{
			GitDiff: []ChangedFile{{Path: "pkg/old_test.go", Status: status}},
		}
		violations2 := Evaluate(tr2, DefaultRules())
		for _, v := range violations2 {
			if v.Rule.ID == "r-additive-tests" {
				t.Fatalf("GitDiff %q alone must NOT fire without TamperedTestPaths", status)
			}
		}
	}
}

func TestRAdditiveTestsRemediationContainsPathAndContract(t *testing.T) {
	tr := TurnResult{
		TamperedTestPaths: []string{"pkg/foo_test.go", "pkg/bar_test.go"},
	}
	violations := Evaluate(tr, DefaultRules())
	var target *Violation
	for i := range violations {
		if violations[i].Rule.ID == "r-additive-tests" {
			target = &violations[i]
			break
		}
	}
	if target == nil {
		t.Fatal("expected r-additive-tests violation")
	}
	result := Enforce([]Violation{*target}, "enforce")
	prompt := RepromptPrompt(result)
	if !strings.Contains(prompt, "foo_test.go") {
		t.Fatalf("remediation must contain tampered path, got %q", prompt)
	}
	if !strings.Contains(prompt, "additive-tests-only") && !strings.Contains(strings.ToLower(prompt), "safe-fix-contract") {
		t.Fatalf("remediation must cite additive-tests-only / safe-fix-contract, got %q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "do not") && !strings.Contains(prompt, "do NOT") {
		t.Fatalf("remediation must tell to not edit pre-existing tests, got %q", prompt)
	}
}

func TestRAdditiveTestsNotInDocScopeOrArtifact(t *testing.T) {
	for _, id := range DocScopeRuleIDs() {
		if id == "r-additive-tests" {
			t.Fatal("r-additive-tests must NOT be in DocScopeRuleIDs (Task-260: CP-58 owns Flow child wiring)")
		}
	}
	for _, id := range ArtifactRuleIDs() {
		if id == "r-additive-tests" {
			t.Fatal("r-additive-tests must NOT be in ArtifactRuleIDs")
		}
	}
}

func TestRAdditiveTestsDoesNotFireOnNonTestFileM(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "pkg/service.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire on non-test file M, got %v", v.Detail)
		}
	}
	// Also TamperedTestPaths should never contain non-test files; if it did via bug, it would fire — but production oracle filters IsTestFile, so non-test M must not fire at all.
	tr2 := TurnResult{
		TamperedTestPaths: []string{},
		GitDiff:           []ChangedFile{{Path: "pkg/service.go", Status: "M"}},
	}
	violations2 := Evaluate(tr2, DefaultRules())
	for _, v := range violations2 {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("r-additive-tests must NOT fire on non-test file M (empty tampered), got %v", v.Detail)
		}
	}
}
