package flowgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules, got %d", len(rules))
	}
	ids := []string{"r-ca", "r-bug", "r-tests", "r-reg", "r-dep"}
	for i, id := range ids {
		if rules[i].ID != id {
			t.Errorf("rules[%d].ID = %q, want %q", i, rules[i].ID, id)
		}
		if !rules[i].Enabled {
			t.Errorf("rules[%d] should be enabled", i)
		}
	}
}

func TestLoadRulesMissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	rules, err := LoadRules(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defaults := DefaultRules()
	if len(rules) != len(defaults) {
		t.Fatalf("expected %d rules, got %d", len(defaults), len(rules))
	}
}

func TestLoadRulesAndSave(t *testing.T) {
	dir := t.TempDir()
	originals := DefaultRules()
	if err := SaveRules(dir, originals); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "flow-rules.json")); err != nil {
		t.Fatalf("flow-rules.json not created: %v", err)
	}
	loaded, err := LoadRules(dir)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(loaded) != len(originals) {
		t.Fatalf("expected %d rules, got %d", len(originals), len(loaded))
	}
	for i := range originals {
		if loaded[i].ID != originals[i].ID {
			t.Errorf("rule %d ID mismatch: %q != %q", i, loaded[i].ID, originals[i].ID)
		}
	}
}

func TestEvaluateCodeChangeWithoutAuditNote(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/foo.go", Status: "M"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-ca" {
			found = true
		}
	}
	if !found {
		t.Error("expected r-ca violation for code change without audit note")
	}
}

func TestEvaluateCodeChangeWithAuditNote(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/foo.go", Status: "M"},
			{Path: "change-audit/CA-001.md", Status: "A"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-ca" {
			t.Error("unexpected r-ca violation when audit note is present")
		}
	}
}

func TestEvaluateNoCodeChanges(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "requirements/some-doc.md", Status: "M"},
			{Path: "change-audit/CA-002.md", Status: "A"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-ca" {
			t.Errorf("unexpected r-ca violation with no code changes")
		}
	}
}

func TestEvaluateTestsFailed(t *testing.T) {
	tr := TurnResult{
		Tests: TestOutcome{
			Ran:    true,
			Failed: []string{"TestFoo", "TestBar"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-tests" {
			found = true
			if v.Detail == "" {
				t.Error("expected non-empty detail for tests_failed")
			}
		}
	}
	if !found {
		t.Error("expected r-tests violation when tests fail")
	}
}

func TestEvaluateTestsPassedNoViolation(t *testing.T) {
	tr := TurnResult{
		Tests: TestOutcome{
			Ran:    true,
			Passed: []string{"TestFoo"},
			Failed: nil,
		},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-tests" {
			t.Error("unexpected r-tests violation when tests pass")
		}
	}
}

func TestEvaluateRemovedGoFile(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/old.go", Status: "D"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-dep" {
			found = true
		}
	}
	if !found {
		t.Error("expected r-dep violation for deleted .go file")
	}
}

func TestEnforceNoViolationsIsPass(t *testing.T) {
	result := Enforce(nil, "strict")
	if result.Action != "pass" {
		t.Errorf("expected pass, got %q", result.Action)
	}
}

func TestEnforceRegressionAlwaysBlocksInWarnMode(t *testing.T) {
	regRule := Rule{
		ID:      "r-reg",
		Trigger: "regression_test_broke",
		Action:  "block",
		Enabled: true,
	}
	violations := []Violation{{Rule: regRule, Detail: "TestFoo regressed"}}
	result := Enforce(violations, "warn")
	if result.Action != "block" {
		t.Errorf("regression should always block even in warn mode, got %q", result.Action)
	}
}

func TestEnforceTestsFailedAlwaysBlocksInWarnMode(t *testing.T) {
	testRule := Rule{
		ID:      "r-tests",
		Trigger: "tests_failed",
		Action:  "block",
		Enabled: true,
	}
	violations := []Violation{{Rule: testRule, Detail: "TestBar failed"}}
	result := Enforce(violations, "warn")
	if result.Action != "block" {
		t.Errorf("tests_failed should always block even in warn mode, got %q", result.Action)
	}
}

func TestEnforceRCAInWarnModeDowngradedToWarn(t *testing.T) {
	caRule := Rule{
		ID:      "r-ca",
		Trigger: "code_changed",
		Action:  "reprompt",
		Enabled: true,
	}
	violations := []Violation{{Rule: caRule, Detail: "no audit note"}}
	result := Enforce(violations, "warn")
	if result.Action != "warn" {
		t.Errorf("r-ca in warn mode should be downgraded to warn, got %q", result.Action)
	}
}

func TestEnforceRCAInStrictMode(t *testing.T) {
	caRule := Rule{
		ID:      "r-ca",
		Trigger: "code_changed",
		Action:  "reprompt",
		Enabled: true,
	}
	violations := []Violation{{Rule: caRule, Detail: "no audit note"}}
	result := Enforce(violations, "strict")
	if result.Action != "reprompt" {
		t.Errorf("r-ca in strict mode should be reprompt, got %q", result.Action)
	}
}

func TestEnforceMessageContainsDetails(t *testing.T) {
	caRule := Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	violations := []Violation{{Rule: caRule, Detail: "missing audit note"}}
	result := Enforce(violations, "strict")
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
	if result.Message[:10] != "Flow gate:" {
		t.Errorf("message should start with 'Flow gate:', got %q", result.Message)
	}
}

// r-tests and r-reg are coupled in v1: both fire on the same failing-test condition
// with an identical Detail. Enforce must dedupe so the message is not doubled
// ("Tests failed: X; Tests failed: X"). (CP-35)
func TestEnforceDedupesIdenticalDetails(t *testing.T) {
	rTests := Rule{ID: "r-tests", Trigger: "tests_failed", Action: "block", Enabled: true}
	rReg := Rule{ID: "r-reg", Trigger: "regression_test_broke", Action: "block", Enabled: true}
	violations := []Violation{
		{Rule: rTests, Detail: "Tests failed: TestAdd"},
		{Rule: rReg, Detail: "Tests failed: TestAdd"},
	}
	result := Enforce(violations, "enforce")
	if result.Action != "block" {
		t.Errorf("Action = %q, want block", result.Action)
	}
	if result.Message != "Flow gate: Tests failed: TestAdd" {
		t.Errorf("Message = %q, want single (deduped) detail", result.Message)
	}
}

// RepromptPrompt must give the AI actionable, file-level remediation — not the terse
// symptom message — so it knows to CREATE a BugFix doc rather than edit something else.
// (BUG-140)
func TestRepromptPromptIsActionable(t *testing.T) {
	rBug := Rule{ID: "r-bug", Trigger: "bug_fixed", Action: "reprompt", Enabled: true}
	result := Enforce([]Violation{{Rule: rBug, Detail: "bug fix detected but no bugfix doc found"}}, "enforce")
	prompt := RepromptPrompt(result)
	for _, want := range []string{"requirements/09-BugFix/done/BUG-", "FORMAT-REFERENCE-BUGFIX.md", "Do NOT edit the change-audit note"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("reprompt prompt missing %q\ngot: %s", want, prompt)
		}
	}
}

func TestRepromptPromptCoversBothMissingDocs(t *testing.T) {
	rCA := Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	rBug := Rule{ID: "r-bug", Trigger: "bug_fixed", Action: "reprompt", Enabled: true}
	result := Enforce([]Violation{
		{Rule: rCA, Detail: "code changed but no change-audit note found"},
		{Rule: rBug, Detail: "bug fix detected but no bugfix doc found"},
	}, "enforce")
	prompt := RepromptPrompt(result)
	if !strings.Contains(prompt, "change-audit/CA-") || !strings.Contains(prompt, "requirements/09-BugFix/done/BUG-") {
		t.Errorf("reprompt prompt should name both files\ngot: %s", prompt)
	}
}

// When no reprompt-action rule is present, RepromptPrompt falls back to the terse message.
func TestRepromptPromptFallsBackToMessage(t *testing.T) {
	rTests := Rule{ID: "r-tests", Trigger: "tests_failed", Action: "block", Enabled: true}
	result := Enforce([]Violation{{Rule: rTests, Detail: "Tests failed: TestAdd"}}, "enforce")
	if RepromptPrompt(result) != result.Message {
		t.Errorf("expected fallback to message %q, got %q", result.Message, RepromptPrompt(result))
	}
}

func TestIsDocOrAuditFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"requirements/foo.md", true},
		{"change-audit/CA-001.md", true},
		{"internal/foo/bar.go", false},
		{"README.md", true},
		{"cmd/main.go", false},
	}
	for _, c := range cases {
		if got := IsDocOrAuditFile(c.path); got != c.want {
			t.Errorf("IsDocOrAuditFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestHasChangeAuditNote(t *testing.T) {
	diff := []ChangedFile{
		{Path: "change-audit/CA-042.md", Status: "A"},
	}
	if !HasChangeAuditNote(diff) {
		t.Error("expected HasChangeAuditNote to return true")
	}
	diff2 := []ChangedFile{
		{Path: "change-audit/CA-042.md", Status: "D"},
	}
	if HasChangeAuditNote(diff2) {
		t.Error("deleted CA file should not count as audit note")
	}
}

func TestHasBugFixDoc(t *testing.T) {
	diff := []ChangedFile{
		{Path: "requirements/09-BugFix/BUG-001.md", Status: "A"},
	}
	if !HasBugFixDoc(diff) {
		t.Error("expected HasBugFixDoc to return true")
	}
}

// The reqscaffold creates requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md in bound
// projects. Without the guard, HasBugFixDoc would match on "requirements/09-BugFix" and
// return true, silently suppressing the r-bug violation even when no real BUG doc was
// written. (BUG-141)
func TestHasBugFixDocIgnoresFormatReferenceFile(t *testing.T) {
	diff := []ChangedFile{
		{Path: "requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md", Status: "A"},
		{Path: "calc.go", Status: "M"},
	}
	if HasBugFixDoc(diff) {
		t.Error("FORMAT-REFERENCE-BUGFIX.md must not count as a bugfix doc")
	}
}

func TestHasBugFixDocRealDocAlongsideFormatReference(t *testing.T) {
	diff := []ChangedFile{
		{Path: "requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md", Status: "A"},
		{Path: "requirements/09-BugFix/done/BUG-141-fix-something.md", Status: "A"},
	}
	if !HasBugFixDoc(diff) {
		t.Error("a real BUG doc alongside FORMAT-REFERENCE should still return true")
	}
}
