package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CP-43 passed-case gaps (C1/C2/C3): the runner's real gate entry point
// prepareChangeContract + commitChangeContract had no direct test for the
// declared→store persistence (C1), the undeclared→inferred path (C2), or the
// docs-excluded out-of-scope listing (C3) — those were only covered at the
// changecontract/flowgate unit level. Provider-agnostic (Case 1): the loop
// runs claude/codex/grok. Additive — legacy suites untouched.

func TestPrepareChangeContractDeclaredPersistsToStore(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract\n"
			finalMessage := "no contract echoed"
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}, {Path: "calc_test.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-1", "chat-run-1", prompt, finalMessage, diff, []string{"calc-core"})
			if !p.ok || !p.declared {
				t.Fatalf("%s: prepare must be ok+declared, got ok=%v declared=%v", provider, p.ok, p.declared)
			}
			if len(p.outOfScopePaths) != 0 {
				t.Fatalf("%s: in-scope diff must yield no drift, got %v", provider, p.outOfScopePaths)
			}
			if err := commitChangeContract(dir, p, canonicalPendingRoute{}, nil); err != nil {
				t.Fatalf("%s: commitChangeContract: %v", provider, err)
			}
			store, err := changecontract.NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := store.GetLatestForRun("run-1")
			if !ok {
				t.Fatalf("%s: committed contract must be readable from contracts.ndjson", provider)
			}
			if got.Confidence != changecontract.ConfidenceDeclared {
				t.Fatalf("%s: persisted confidence = %v, want declared", provider, got.Confidence)
			}
			if got.FeatureKey != "calc-core" {
				t.Fatalf("%s: persisted feature_key = %q, want calc-core", provider, got.FeatureKey)
			}
			if len(got.DeclaredPaths) != 2 {
				t.Fatalf("%s: persisted declared_paths = %v, want 2", provider, got.DeclaredPaths)
			}
		})
	}
}

func TestPrepareChangeContractUndeclaredInfersFromDiff(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "Fix giup toi: Percentage(part, whole int) tra ve 0.0 khi whole=0 nhung khong canh bao. Them ham PercentageSafe tra error khi whole=0."
			finalMessage := "done"
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-2", "chat-run-2", prompt, finalMessage, diff, []string{"calc-core"})
			if !p.ok {
				t.Fatalf("%s: prepare must ok", provider)
			}
			if p.declared {
				t.Fatalf("%s: must NOT be declared (no contract in prompt/final)", provider)
			}
			if p.contract.Confidence != changecontract.ConfidenceInferred {
				t.Fatalf("%s: confidence = %v, want inferred", provider, p.contract.Confidence)
			}
			if len(p.contract.DeclaredPaths) != 1 || p.contract.DeclaredPaths[0] != "calc.go" {
				t.Fatalf("%s: inferred declared_paths = %v, want [calc.go]", provider, p.contract.DeclaredPaths)
			}
			if len(p.outOfScopePaths) != 0 {
				t.Fatalf("%s: inferred scope must cover the diff by construction, got %v", provider, p.outOfScopePaths)
			}
		})
	}
}

func TestPrepareChangeContractOutOfScopeExcludesDocs(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham ClampLoHi vao format.go\nfiles: format.go\nsymbols: Clamp\n"
			finalMessage := "done"
			diff := []flowgate.ChangedFile{
				{Path: "format.go", Status: "M"},
				{Path: "user_test.go", Status: "A"},
				{Path: "requirements/05-System-Specs/CP-43.md", Status: "M"},
				{Path: "change-audit/CA-1.md", Status: "A"},
			}
			p := prepareChangeContract(context.Background(), dir, "run-3", "chat-run-3", prompt, finalMessage, diff, []string{"calc-core"})
			if !p.ok || !p.declared {
				t.Fatalf("%s: prepare must be ok+declared", provider)
			}
			if len(p.outOfScopePaths) != 1 || p.outOfScopePaths[0] != "user_test.go" {
				t.Fatalf("%s: out-of-scope = %v, want exactly [user_test.go] (docs must be excluded)", provider, p.outOfScopePaths)
			}
			if !strings.Contains(p.contract.DeclaredPaths[0], "format.go") {
				t.Fatalf("%s: declared paths = %v, want format.go", provider, p.contract.DeclaredPaths)
			}
		})
	}
}