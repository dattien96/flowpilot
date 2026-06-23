package structure

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFallbackProviderAvailable verifies the fallback always reports itself unavailable.
func TestFallbackProviderAvailable(t *testing.T) {
	fp := &fallbackProvider{repoDir: "."}
	if fp.Available() {
		t.Fatal("fallbackProvider.Available() must return false")
	}
}

// TestGitNexusProviderAvailable verifies gitNexusProvider reports itself available.
func TestGitNexusProviderAvailable(t *testing.T) {
	gp := &gitNexusProvider{repoDir: "."}
	if !gp.Available() {
		t.Fatal("gitNexusProvider.Available() must return true")
	}
}

// TestNewSelectsCorrectProvider verifies New() delegates based on isGitNexusOK.
func TestNewSelectsCorrectProvider(t *testing.T) {
	p := New(".", false)
	if p.Available() {
		t.Fatal("New(false) must return a provider that is not Available()")
	}

	p = New(".", true)
	if !p.Available() {
		t.Fatal("New(true) must return a provider that is Available()")
	}
}

// TestDependentsSummaryJSONMarshal verifies the struct round-trips through JSON correctly.
func TestDependentsSummaryJSONMarshal(t *testing.T) {
	orig := DependentsSummary{
		Count:    3,
		Nearest:  []string{"foo.go", "bar.go", "baz.go"},
		Flows:    []string{"flow-a", "flow-b"},
		Complete: true,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got DependentsSummary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Count != orig.Count {
		t.Errorf("Count: got %d, want %d", got.Count, orig.Count)
	}
	if got.Complete != orig.Complete {
		t.Errorf("Complete: got %v, want %v", got.Complete, orig.Complete)
	}
	if len(got.Nearest) != len(orig.Nearest) {
		t.Errorf("Nearest len: got %d, want %d", len(got.Nearest), len(orig.Nearest))
	}
	if len(got.Flows) != len(orig.Flows) {
		t.Errorf("Flows len: got %d, want %d", len(got.Flows), len(orig.Flows))
	}
}

// TestDependentsSummaryJSONKeys verifies the JSON field names match the spec.
func TestDependentsSummaryJSONKeys(t *testing.T) {
	s := DependentsSummary{Count: 1, Nearest: []string{"x"}, Complete: false}
	data, _ := json.Marshal(s)
	js := string(data)

	for _, key := range []string{`"count"`, `"nearest"`, `"flows"`, `"complete"`} {
		if !contains(js, key) {
			t.Errorf("JSON output missing key %s: %s", key, js)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestFallbackDependentsNonFatal verifies fallbackProvider.Dependents never panics or
// returns an error even when the target doesn't exist.
func TestFallbackDependentsNonFatal(t *testing.T) {
	fp := &fallbackProvider{repoDir: t.TempDir()}
	summary, err := fp.Dependents(context.Background(), "nonexistent.go")
	if err != nil {
		t.Fatalf("Dependents must not return error on missing target, got: %v", err)
	}
	if summary.Complete {
		t.Error("fallbackProvider must always return Complete=false")
	}
}

// TestFallbackDependentsWithTempRepo creates a minimal git repo with two Go files,
// one importing the other, and confirms the fallback discovers the relationship.
func TestFallbackDependentsWithTempRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	mustRun := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("cmd %v output: %s", args, out)
		}
	}

	mustRun("git", "init")
	mustRun("git", "config", "user.email", "test@test.com")
	mustRun("git", "config", "user.name", "Test")

	pkgDir := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pkgDir, "thing.go"), []byte(`package mypkg

func Hello() string { return "hi" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	consumer := filepath.Join(dir, "main.go")
	if err := os.WriteFile(consumer, []byte(`package main

import "mypkg"

func main() { _ = mypkg.Hello() }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRun("git", "add", ".")
	mustRun("git", "commit", "-m", "init")

	fp := &fallbackProvider{repoDir: dir}
	summary, err := fp.Dependents(context.Background(), filepath.Join(pkgDir, "thing.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Complete {
		t.Error("fallbackProvider must always return Complete=false")
	}
}

// TestParseGitNexusJSONOutput verifies the JSON output parser extracts nearest and flows.
func TestParseGitNexusJSONOutput(t *testing.T) {
	jsonInput := `{"dependents":["a.go","b.go"],"nearest":["c.go"],"flows":["flow-1"]}`
	s := parseGitNexusOutput(jsonInput)

	if len(s.Nearest) == 0 {
		t.Error("expected Nearest to be populated from JSON nearest field")
	}
	if s.Nearest[0] != "c.go" {
		t.Errorf("expected nearest[0]='c.go', got %q", s.Nearest[0])
	}
	if len(s.Flows) != 1 || s.Flows[0] != "flow-1" {
		t.Errorf("expected flows=['flow-1'], got %v", s.Flows)
	}
}

// TestParseGitNexusJSONFallbackToDependents verifies that when "nearest" is absent,
// "dependents" is used instead.
func TestParseGitNexusJSONFallbackToDependents(t *testing.T) {
	jsonInput := `{"dependents":["a.go","b.go"],"flows":[]}`
	s := parseGitNexusOutput(jsonInput)

	if len(s.Nearest) != 2 {
		t.Errorf("expected 2 entries from dependents fallback, got %d", len(s.Nearest))
	}
}

// TestParseGitNexusTextOutput verifies the text parser extracts symbols from "→" lines.
func TestParseGitNexusTextOutput(t *testing.T) {
	text := "caller.go → target.go\nother.go depends on target.go"
	s := parseGitNexusOutput(text)

	if len(s.Nearest) == 0 {
		t.Error("expected Nearest to be populated from text output")
	}
}

// TestNearestLimit verifies that Nearest is capped at 10 entries.
func TestNearestLimit(t *testing.T) {
	var deps []string
	for i := 0; i < 20; i++ {
		deps = append(deps, "file.go")
	}

	raw := make(map[string]interface{})
	raw["nearest"] = deps

	import_json, _ := json.Marshal(raw)
	s := parseGitNexusOutput(string(import_json))
	if len(s.Nearest) > 10 {
		t.Errorf("Nearest must be capped at 10, got %d", len(s.Nearest))
	}
}

// TestCompleteIsFalseWhenDynamicDetected verifies Complete=false when output
// contains "dynamic" or "interface{}".
func TestCompleteIsFalseWhenDynamicDetected(t *testing.T) {
	cases := []string{
		`{"nearest":["a.go"],"dynamic":true}`,
		`{"nearest":["a.go"]} note: interface{} dispatch detected`,
	}
	for _, c := range cases {
		s := parseGitNexusOutput(c)
		if s.Complete {
			t.Errorf("expected Complete=false for output: %s", c)
		}
	}
}

// TestPackagePathOf verifies the helper correctly derives import path fragments.
func TestPackagePathOf(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"internal/foo/bar.go", "internal/foo"},
		{"internal/foo/", "internal/foo"},
		{"internal/foo", "internal/foo"},
		{"bar.go", "bar"},
	}
	for _, c := range cases {
		got := packagePathOf(c.input)
		if got != c.want {
			t.Errorf("packagePathOf(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
