package flowgate

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveOverrideWithReasonWritesLedger(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	if err := SaveOverrideWithReason(dot, Override{TestName: "TestFoo"}, "flaky env", "run-1", "operator", 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	open, err := ListOpenWaivers(dot)
	if err != nil || len(open) != 1 || open[0].Reason != "flaky env" {
		t.Fatalf("ledger: open=%#v err=%v", open, err)
	}
	if !IsOverridden(map[string]Override{"TestFoo": {TestName: "TestFoo", ExpiresAt: open[0].ExpiresAt}}, "TestFoo") {
		t.Fatal("override should be active")
	}
}

func TestExpiredOverrideReArmsOnLoad(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if err := writeOverrides(dot, map[string]Override{
		"TestBar": {TestName: "TestBar", HumanConfirm: true, ExpiresAt: past},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOverrides(dot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expired override should be pruned, got %#v", got)
	}
}

func TestSaveOverrideWithReasonRejectsEmptyReason(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	if err := SaveOverrideWithReason(dot, Override{TestName: "TestX"}, "  ", "", "", DefaultWaiverTTL); err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestCP53RNewtestRepromptOnProdOnly(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "internal/foo/bar.go", Status: "M"}},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-newtest" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected r-newtest violation for prod-only diff")
	}
}

func TestCP53RNewtestSilentWhenNewTestAdded(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/bar.go", Status: "M"},
			{Path: "internal/foo/bar_test.go", Status: "A"},
		},
	}
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-newtest" {
			t.Fatal("r-newtest must not fire when a new test file was added")
		}
	}
}

func TestCP53RNewtestSilentOnDocsOnly(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{{Path: "requirements/08-Task/todo/Task-1.md", Status: "M"}},
	}
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-newtest" {
			t.Fatal("r-newtest must not fire on docs-only diff")
		}
	}
}

func TestCP53RNewtestOldTestEditDoesNotSatisfy(t *testing.T) {
	tr := TurnResult{
		GitDiff: []ChangedFile{
			{Path: "internal/foo/bar.go", Status: "M"},
			{Path: "internal/foo/bar_test.go", Status: "M"},
		},
	}
	found := false
	for _, v := range Evaluate(tr, DefaultRules()) {
		if v.Rule.ID == "r-newtest" {
			found = true
		}
	}
	if !found {
		t.Fatal("editing an old test file must not satisfy r-newtest")
	}
}
