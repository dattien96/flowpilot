package flowgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 9 {
		t.Fatalf("expected 9 rules, got %d", len(rules))
	}
	ids := []string{"r-ca", "r-fk", "r-bug", "r-task", "r-tests", "r-reg", "r-dep", "r-artifact-output", "r-artifact-output-structure"}
	for i, id := range ids {
		if rules[i].ID != id {
			t.Errorf("rules[%d].ID = %q, want %q", i, rules[i].ID, id)
		}
		if !rules[i].Enabled {
			t.Errorf("rules[%d] should be enabled", i)
		}
	}
}

func TestEvaluateRequiredArtifactOutputMissing(t *testing.T) {
	dir := t.TempDir()
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/coder-summary.md"},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-artifact-output" {
			found = true
			if !strings.Contains(v.Detail, "docs/coder-summary.md") {
				t.Fatalf("detail should name missing path, got %q", v.Detail)
			}
		}
	}
	if !found {
		t.Fatal("expected r-artifact-output when required path is missing")
	}
}

func TestEvaluateRequiredArtifactOutputPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "coder-summary.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/coder-summary.md"},
	}
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-artifact-output" {
			t.Fatalf("unexpected r-artifact-output when file exists: %+v", v)
		}
	}
}

func TestRepromptPromptNamesMissingArtifactOutputPaths(t *testing.T) {
	result := Enforce([]Violation{{
		Rule:   Rule{ID: "r-artifact-output", Trigger: "required_artifact_output_missing", Action: "reprompt", Enabled: true},
		Detail: "required file artifact output missing: docs/coder-summary.md",
	}}, "enforce")
	prompt := RepromptPrompt(result)
	if !strings.Contains(prompt, "docs/coder-summary.md") {
		t.Fatalf("reprompt should name missing path, got %q", prompt)
	}
	if !strings.Contains(prompt, GateRepromptPrefix) {
		t.Fatalf("reprompt should use gate prefix, got %q", prompt)
	}
}

func TestMatchMarkdownSectionsFlexibleLevelAndCase(t *testing.T) {
	content := "# What\n\nbody\n\n### why:\n\nmore\n\n## Baseline .\n"
	missing := MissingMarkdownSections(content, []string{"What", "Why", "Baseline"})
	if len(missing) != 0 {
		t.Fatalf("expected all sections present with flex match, missing %v", missing)
	}
}

func TestMatchMarkdownSectionsRejectsNonHeading(t *testing.T) {
	content := "What\nWhy\nBaseline\n"
	missing := MissingMarkdownSections(content, []string{"What", "Why", "Baseline"})
	if len(missing) != 3 {
		t.Fatalf("plain lines must not count as headings, missing %v", missing)
	}
	// Synonyms are not accepted in v1.
	content2 := "## Rationale\n## What\n## Baseline\n"
	missing2 := MissingMarkdownSections(content2, []string{"What", "Why", "Baseline"})
	if len(missing2) != 1 || missing2[0] != "Why" {
		t.Fatalf("want missing Why only, got %v", missing2)
	}
}

func TestEvaluateRequiredArtifactOutputStructurePresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "## What\n\nx\n\n## Why\n\ny\n\n## Baseline\n\nz\n"
	if err := os.WriteFile(filepath.Join(dir, "docs", "coder-summary.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/coder-summary.md"},
		RequiredStructuredFileArtifactOutputs: []StructuredFileArtifactOutput{{
			Path:     "docs/coder-summary.md",
			Sections: []string{"What", "Why", "Baseline"},
		}},
	}
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-artifact-output-structure" {
			t.Fatalf("unexpected structure violation: %+v", v)
		}
	}
}

func TestEvaluateRequiredArtifactOutputStructureMissingSection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Flex present for What/Baseline but missing Why.
	body := "### what\n\n## Baseline\n"
	if err := os.WriteFile(filepath.Join(dir, "docs", "coder-summary.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/coder-summary.md"},
		RequiredStructuredFileArtifactOutputs: []StructuredFileArtifactOutput{{
			Path:     "docs/coder-summary.md",
			Sections: []string{"What", "Why", "Baseline"},
		}},
	}
	found := false
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-artifact-output-structure" {
			found = true
			if !strings.Contains(v.Detail, "docs/coder-summary.md") || !strings.Contains(v.Detail, "Why") {
				t.Fatalf("detail should name path and Why, got %q", v.Detail)
			}
		}
	}
	if !found {
		t.Fatal("expected r-artifact-output-structure when Why missing")
	}
}

func TestEvaluateRequiredArtifactOutputStructureSkipsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/coder-summary.md"},
		RequiredStructuredFileArtifactOutputs: []StructuredFileArtifactOutput{{
			Path:     "docs/coder-summary.md",
			Sections: []string{"What", "Why", "Baseline"},
		}},
	}
	var exist, structure bool
	for _, v := range Evaluate(tr, DefaultRules()) {
		switch v.Rule.ID {
		case "r-artifact-output":
			exist = true
		case "r-artifact-output-structure":
			structure = true
		}
	}
	if !exist {
		t.Fatal("expected existence violation when file missing")
	}
	if structure {
		t.Fatal("structure gate must not fire when file is missing")
	}
}

func TestEvaluateRequiredArtifactOutputStructureSkipsWhenNoStructure(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "notes.md"), []byte("no headings"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := TurnResult{
		WorkspaceCwd:                dir,
		RequiredFileArtifactOutputs: []string{"docs/notes.md"},
		// No RequiredStructuredFileArtifactOutputs
	}
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-artifact-output-structure" {
			t.Fatalf("paths-only must not structure-gate: %+v", v)
		}
	}
}

func TestRepromptPromptNamesMissingArtifactOutputStructure(t *testing.T) {
	result := Enforce([]Violation{{
		Rule:   Rule{ID: "r-artifact-output-structure", Trigger: "required_artifact_output_structure_missing", Action: "reprompt", Enabled: true},
		Detail: "required file artifact structure missing: docs/coder-summary.md (Why)",
	}}, "enforce")
	prompt := RepromptPrompt(result)
	if !strings.Contains(prompt, "docs/coder-summary.md") || !strings.Contains(prompt, "Why") {
		t.Fatalf("reprompt should name path and section, got %q", prompt)
	}
	if !strings.Contains(prompt, "structure incomplete") {
		t.Fatalf("reprompt should mention structure, got %q", prompt)
	}
}

func TestMissingRequiredFileArtifactOutputsRejectsOutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	missing := MissingRequiredFileArtifactOutputs(dir, []string{"../outside.md", "/tmp/abs.md"})
	if len(missing) != 2 {
		t.Fatalf("expected both paths missing/rejected, got %v", missing)
	}
}

func TestMergeDefaultRulesAddsMissingArtifactOutput(t *testing.T) {
	old := []Rule{
		{ID: "r-ca", Scope: "step", Trigger: "code_changed", Action: "reprompt", Enabled: true},
	}
	merged := MergeDefaultRules(old)
	found := false
	for _, r := range merged {
		if r.ID == "r-artifact-output" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected MergeDefaultRules to append r-artifact-output")
	}
	if merged[0].ID != "r-ca" {
		t.Fatalf("expected loaded rules first, got %q", merged[0].ID)
	}
}

func TestMissingRequiredFileArtifactOutputsRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "escape.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	missing := MissingRequiredFileArtifactOutputs(dir, []string{"escape.md"})
	if len(missing) != 1 || missing[0] != "escape.md" {
		t.Fatalf("expected symlink escape treated as missing, got %v", missing)
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
		WrittenPaths: []string{"internal/foo/foo.go"}, // AI wrote this file
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
		WrittenPaths: []string{"internal/foo/foo.go", "change-audit/CA-001.md"}, // AI wrote both
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-ca" {
			t.Error("unexpected r-ca violation when audit note is present")
		}
	}
}

// r-ca must not fire when the AI wrote no files in this turn, even if the git
// working tree has pre-existing dirty code files or runner-internal state changes
// (e.g. test_baseline.json). This was the false positive seen in E2E case 1 of
// Task-155: "Tell me what Add does. Do not edit files." triggered the gate because
// ObserveGitDiff picked up stale sandbox files.
func TestEvaluateReadOnlyTurnNeverTriggersRCA(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/foo.go", Status: "M"},
			{Path: ".flowpilot/guard/test_baseline.json", Status: "M"},
		},
		WrittenPaths: nil, // AI wrote nothing this turn
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-ca" {
			t.Error("r-ca must not fire on a read-only turn even with dirty git tree")
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

func TestEvaluateCommitFeatureKeyMissingSkipsDocsOnlyTurns(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "requirements/08-Task/done/Task-001.md", Status: "A"},
		},
		CommitSubjects:   []string{"[Task][missing-key][docs] add docs"},
		KnownFeatureKeys: []string{"chat-ui"},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-fk" {
			t.Fatal("unexpected r-fk violation for docs-only turn")
		}
	}
}

func TestEvaluateCommitFeatureKeyMissingRejectsUnknownKey(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/chat/input.go", Status: "M"},
		},
		WrittenPaths:     []string{"internal/chat/input.go"},
		CommitSubjects:   []string{"[Task][unknown-key][ui] update chat"},
		KnownFeatureKeys: []string{"chat-ui"},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-fk" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected r-fk violation for unknown key")
	}
}

func TestEvaluateCommitFeatureKeyMissingAcceptsVerifiedKey(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/chat/input.go", Status: "M"},
		},
		WrittenPaths:     []string{"internal/chat/input.go"},
		CommitSubjects:   []string{"[Task][chat-ui][ui] update chat"},
		KnownFeatureKeys: []string{"chat-ui"},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-fk" {
			t.Fatal("unexpected r-fk violation for verified key")
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

// E2E-13: when r-ca AND r-bug both fire, RepromptPrompt must include instructions for
// BOTH missing files in a single prompt so the AI can fix both in one turn.
func TestRepromptPromptBothRCAAndRBugCombined(t *testing.T) {
	rCA := Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	rBug := Rule{ID: "r-bug", Trigger: "bug_fixed", Action: "reprompt", Enabled: true}
	result := Enforce([]Violation{
		{Rule: rCA, Detail: "code changed but no change-audit note found"},
		{Rule: rBug, Detail: "bug fix detected but no bugfix doc found"},
	}, "enforce")
	if result.Action != "reprompt" {
		t.Errorf("Action = %q, want reprompt", result.Action)
	}
	prompt := RepromptPrompt(result)
	for _, want := range []string{"change-audit/CA-", "requirements/09-BugFix/done/BUG-"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("combined reprompt missing %q\ngot: %s", want, prompt)
		}
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

// r-task: final message references Task-NNN but no Task doc in diff → reprompt.
func TestEvaluateRTaskFiresWhenTaskRefMissingDoc(t *testing.T) {
	tr := TurnResult{
		FinalMessage: "Completed Task-113 implementation.",
		GitDiff:      []ChangedFile{{Path: "internal/flowgate/evaluate.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-task" {
			found = true
		}
	}
	if !found {
		t.Error("expected r-task violation when Task-NNN referenced but no task doc in diff")
	}
}

// r-task must NOT fire when the Task doc is present in the diff.
func TestEvaluateRTaskNoViolationWhenDocPresent(t *testing.T) {
	tr := TurnResult{
		FinalMessage: "Completed Task-113 implementation.",
		GitDiff: []ChangedFile{
			{Path: "internal/flowgate/evaluate.go", Status: "M"},
			{Path: "requirements/08-Task/done/Task-113-r-task-rule.md", Status: "A"},
		},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-task" {
			t.Error("unexpected r-task violation when Task doc is present")
		}
	}
}

func TestEvaluateRTaskFiresWhenChangeTypeIsTask(t *testing.T) {
	tr := TurnResult{
		ChangeType:  "task",
		SourceDocID: "Task-114",
		GitDiff:     []ChangedFile{{Path: "internal/flowgate/evaluate.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-task" {
			found = true
			if !v.Declared {
				t.Error("expected explicit task mode violation to be marked declared")
			}
			if v.SourceDocID != "Task-114" {
				t.Fatalf("sourceDocID = %q, want Task-114", v.SourceDocID)
			}
		}
	}
	if !found {
		t.Error("expected r-task violation when changeType=task and no task doc exists")
	}
}

func TestEvaluateRBugFiresWhenChangeTypeIsBugfix(t *testing.T) {
	tr := TurnResult{
		ChangeType:  "bugfix",
		SourceDocID: "BUG-141",
		GitDiff:     []ChangedFile{{Path: "internal/flowgate/evaluate.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-bug" {
			found = true
			if !v.Declared {
				t.Error("expected explicit bug mode violation to be marked declared")
			}
			if v.SourceDocID != "BUG-141" {
				t.Fatalf("sourceDocID = %q, want BUG-141", v.SourceDocID)
			}
		}
	}
	if !found {
		t.Error("expected r-bug violation when changeType=bugfix and no bug doc exists")
	}
}

// FORMAT-REFERENCE-TASK.md must not satisfy HasTaskDoc (same guard as r-bug / BUG-141).
func TestHasTaskDocIgnoresFormatReferenceFile(t *testing.T) {
	diff := []ChangedFile{
		{Path: "requirements/08-Task/FORMAT-REFERENCE-TASK.md", Status: "A"},
		{Path: "main.go", Status: "M"},
	}
	if HasTaskDoc(diff) {
		t.Error("FORMAT-REFERENCE-TASK.md must not count as a task doc")
	}
}

// RepromptPrompt for r-task must name the exact file and format reference.
func TestRepromptPromptRTaskIsActionable(t *testing.T) {
	rTask := Rule{ID: "r-task", Trigger: "task_referenced", Action: "reprompt", Enabled: true}
	result := Enforce([]Violation{{Rule: rTask, Detail: "task reference detected but no task document found"}}, "enforce")
	prompt := RepromptPrompt(result)
	for _, want := range []string{"requirements/08-Task/done/Task-", "FORMAT-REFERENCE-TASK.md", "Do NOT edit the change-audit note"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("r-task reprompt missing %q\ngot: %s", want, prompt)
		}
	}
}

// ---------------------------------------------------------------------------
// Task-159: Polyglot ecosystem coverage — IsTestFile and DetectTestRunner
// ---------------------------------------------------------------------------

func TestIsTestFileDart(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"lib/add_test.dart", true},
		{"test/widget_test.dart", true},
		{"lib/add.dart", false},
		{"lib/test_helper.dart", false},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsTestFileKotlin(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/test/java/com/example/AddTest.kt", true},
		{"src/test/java/com/example/CalculatorTest.kt", true},
		{"src/main/java/com/example/Add.kt", false},
		{"src/main/java/com/example/TestHelper.kt", false},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsTestFileJava(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/test/java/com/example/AddTest.java", true},
		{"src/test/java/com/example/AddTests.java", true},
		{"src/main/java/com/example/Add.java", false},
		{"src/main/java/com/example/TestHelper.java", false},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsTestFileSwift(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"MyAppTests/AddTests.swift", true},
		{"MyAppUITests/LoginTests.swift", true},
		{"Sources/MyApp/Add.swift", false},
		{"Sources/MyApp/TestHelper.swift", false},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestDetectGradleRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if runner.Cmd != "./gradlew test" {
		t.Errorf("got %q, want \"./gradlew test\"", runner.Cmd)
	}
	if runner.Dir != "" {
		t.Errorf("Dir = %q, want empty (root)", runner.Dir)
	}
}

func TestDetectGradleRunnerBatFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gradlew.bat"), []byte("@echo off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if runner.Cmd != "gradlew.bat test" {
		t.Errorf("got %q, want \"gradlew.bat test\"", runner.Cmd)
	}
}

func TestDetectMavenRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if runner.Cmd != "mvn test -q" {
		t.Errorf("got %q, want \"mvn test -q\"", runner.Cmd)
	}
}

func TestAngularNoWatchMode(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"scripts":{"test":"ng test"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "angular.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if !strings.Contains(runner.Cmd, "--watch=false") {
		t.Errorf("Angular runner must include --watch=false, got %q", runner.Cmd)
	}
	if !strings.Contains(runner.Cmd, "--no-progress") {
		t.Errorf("Angular runner must include --no-progress, got %q", runner.Cmd)
	}
}

func TestNonAngularNpmTestUnchanged(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"scripts":{"test":"jest"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if runner.Cmd != "npm test" {
		t.Errorf("non-Angular npm project: got %q, want \"npm test\"", runner.Cmd)
	}
}

func TestDetectFlutterSkipsWhenNotOnPath(t *testing.T) {
	if flutterOnPath() {
		t.Skip("flutter is on PATH; skipping not-on-path test")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte("name: myapp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := DetectTestRunner(dir)
	if strings.HasPrefix(runner.Cmd, "flutter") {
		t.Errorf("flutter should not be detected when SDK is not on PATH, got %q", runner.Cmd)
	}
}

// r-tamper must fire for Kotlin test files modified during a turn. This verifies
// the IsTestFile expansion (Task-159) feeds through to the tamper check in RunOracle.
func TestTamperDetectionKotlinTestFile(t *testing.T) {
	if !IsTestFile("src/test/java/com/example/AddTest.kt") {
		t.Fatal("IsTestFile must recognise *Test.kt for tamper detection to work")
	}
	diff := []ChangedFile{
		{Path: "src/test/java/com/example/AddTest.kt", Status: "M"},
	}
	var tampered []string
	for _, f := range diff {
		if IsTestFile(f.Path) && f.Status == "M" {
			tampered = append(tampered, f.Path)
		}
	}
	if len(tampered) == 0 {
		t.Error("expected AddTest.kt to be flagged as tampered")
	}
}

func TestRepromptPromptDeclaredTaskUsesResolvedID(t *testing.T) {
	rTask := Rule{ID: "r-task", Trigger: "task_referenced", Action: "reprompt", Enabled: true}
	result := Enforce([]Violation{{
		Rule:        rTask,
		Detail:      "declared task mode but no task document found",
		SourceDocID: "Task-114",
		Declared:    true,
	}}, "enforce")
	prompt := RepromptPrompt(result)
	for _, want := range []string{"started in Task mode", "Task-114-<short-title>.md"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("declared task reprompt missing %q\ngot: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Your final message references a Task-NNN") {
		t.Errorf("declared task reprompt should not mention regex path\n%s", prompt)
	}
}
