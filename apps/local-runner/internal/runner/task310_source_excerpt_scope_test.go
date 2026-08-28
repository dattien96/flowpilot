package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-310: coding prompts omit source.excerpt. change.contract already lists
// declared_paths; writers Read files themselves. Package Fetch still collects
// excerpts so pre-existing package tests stay green.

func TestComposeFlowCodingPromptOmitsSourceExcerptBodies(t *testing.T) {
	pkg := FlowContextPackage{
		WorkflowRunID: "run-task310",
		FeatureKey:    "calc-core",
		SourceExcerpts: []FlowContextExcerpt{
			{Path: "calc.go", Excerpt: "package main\nfunc Calc() {}\n"},
			{Path: "billing.go", Excerpt: "package main\nfunc SplitBill() {}\n"},
		},
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceChangeContract), Priority: 3, Body: "Scope:\n- calc.go\n"},
			{SourceType: string(ContextSourceDependence), Priority: 3, Body: "## Dependence\n\n- `Calc` → 1:\n  - SplitBill\n"},
			{SourceType: string(ContextSourceSourceExcerpt), Priority: 4, Excerpts: []FlowContextExcerpt{
				{Path: "calc.go", Excerpt: "package main\nfunc Calc() {}\n"},
			}},
		},
	}
	prompt := ComposeFlowCodingPrompt(pkg, "Add DivideChecked3")
	if !strings.Contains(prompt, "[FlowPilot flow context package]") {
		t.Fatal("expected flow context package envelope")
	}
	if !strings.Contains(prompt, "### change.contract") {
		t.Fatalf("contract must remain:\n%s", prompt)
	}
	if !strings.Contains(prompt, "### source.dependence") {
		t.Fatalf("dependence must remain:\n%s", prompt)
	}
	if strings.Contains(prompt, "### Source:") {
		t.Fatalf("coding prompt must not dump source.excerpt fences:\n%s", prompt)
	}
	if strings.Contains(prompt, "func Calc()") || strings.Contains(prompt, "func SplitBill()") {
		t.Fatalf("coding prompt must not include excerpt file bodies:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Add DivideChecked3") {
		t.Fatal("coding instruction missing")
	}
}

func TestComposeFlowCodingPromptWithSecretOmitsSourceExcerpt(t *testing.T) {
	pkg := FlowContextPackage{
		WorkflowRunID:  "run-task310-secret",
		SourceExcerpts: []FlowContextExcerpt{{Path: "dirty.go", Excerpt: "package dirty"}},
	}
	prompt := ComposeFlowCodingPromptWithSecret(pkg, "implement", []byte("test-secret"))
	if strings.Contains(prompt, "### Source:") || strings.Contains(prompt, "package dirty") {
		t.Fatalf("secret prompt still has excerpt:\n%s", prompt)
	}
}

func TestOmitSourceExcerptLeavesStoredPackageIntact(t *testing.T) {
	pkg := FlowContextPackage{
		SourceExcerpts: []FlowContextExcerpt{{Path: "calc.go", Excerpt: "package main"}},
		Sections: []FlowContextSection{
			{SourceType: string(ContextSourceSourceExcerpt), Priority: 4, Excerpts: []FlowContextExcerpt{
				{Path: "calc.go", Excerpt: "package main"},
			}},
		},
	}
	_ = ComposeFlowCodingPrompt(pkg, "x")
	if len(pkg.SourceExcerpts) != 1 {
		t.Fatalf("ComposeFlowCodingPrompt mutated stored package excerpts: %#v", pkg.SourceExcerpts)
	}
}

func TestRenderFlowContextPackageStillDumpsExcerptForPackageTests(t *testing.T) {
	pkg := FlowContextPackage{
		SourceExcerpts: []FlowContextExcerpt{{Path: "small.go", Excerpt: "package main\n"}},
	}
	out := RenderFlowContextPackage(pkg)
	if !strings.Contains(out, "### Source: small.go") {
		t.Fatalf("RenderFlowContextPackage must keep excerpt for package-level tests:\n%s", out)
	}
}

func TestBuildFlowContextPackageStillCollectsExcerpt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildFlowContextPackage(dir, FlowContextHints{
		WorkflowRunID:       "run-task310-collect",
		ExplicitSourcePaths: []string{"small.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.SourceExcerpts) != 1 || pkg.SourceExcerpts[0].Path != "small.go" {
		t.Fatalf("Fetch must still collect excerpts on the package, got %#v", pkg.SourceExcerpts)
	}
	prompt := ComposeFlowCodingPrompt(pkg, "do it")
	if strings.Contains(prompt, "### Source: small.go") {
		t.Fatalf("collected excerpt must not appear in coding prompt:\n%s", prompt)
	}
}
