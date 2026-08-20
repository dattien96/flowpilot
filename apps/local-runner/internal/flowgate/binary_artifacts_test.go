package flowgate

import "testing"

// CP-43 P-2 + run-208282: build artifacts must be ignored for scope/code checks.
// Provider-agnostic (pure path classification).

func TestIsBinaryOrBuildArtifact(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"gatesandbox", true},
		{"gatesandbox.exe", true},
		{"app", true}, // bare root binary
		{"myapp", true},
		{"bin/app", true},
		{"dist/bundle.js", true},
		{"build/out.bin", true},
		{"out/app", true},
		{"target/debug/app", true},
		{"a.so", true},
		{"a.dll", true},
		{"a.dylib", true},
		{"a.o", true},
		{"a.a", true},
		{"calc.go", false},
		{"internal/foo/bar.go", false},
		{"requirements/foo.md", false}, // doc, but not binary
		{"Makefile", false},
		{"Dockerfile", false},
		{"README.md", false},
		{"go.mod", false},
		{"calc_test.go", false},
	}
	for _, c := range cases {
		if got := IsBinaryOrBuildArtifact(c.path); got != c.want {
			t.Errorf("IsBinaryOrBuildArtifact(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsDocOrAuditFileExcludesBinary(t *testing.T) {
	// IsDocOrAuditFile now also returns true for binaries via IsBinaryOrBuildArtifact
	if !IsDocOrAuditFile("gatesandbox") {
		t.Error("gatesandbox must be treated as ignored (binary)")
	}
	if !IsDocOrAuditFile("bin/app") {
		t.Error("bin/app must be ignored")
	}
	if IsDocOrAuditFile("calc.go") {
		t.Error("calc.go must not be ignored")
	}
}

func TestIsIgnoredForScope(t *testing.T) {
	if !IsIgnoredForScope("change-audit/CA-914.md") {
		t.Error("CA doc must be ignored")
	}
	if !IsIgnoredForScope("requirements/08-Task/done/Task-910.md") {
		t.Error("Task doc must be ignored")
	}
	if !IsIgnoredForScope("gatesandbox") {
		t.Error("binary must be ignored for scope")
	}
	if IsIgnoredForScope("calc.go") {
		t.Error("calc.go must not be ignored")
	}
}
