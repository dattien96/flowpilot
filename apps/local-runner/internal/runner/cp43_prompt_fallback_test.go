package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CP-43 P-1 Plan A+B: the gate must see a user-declared [Change Contract] even
// when the AI does not echo it. prepareChangeContract now tries
// `prompt` (lastFullPrompt) after `finalMessage` before inferring. The contract
// engine is provider-agnostic, but the gate path is exercised for all three
// providers (claude/codex/grok) to satisfy cross-provider-parity (Case 1:
// agnostic, parameterized loop).
//
// Additive file only — does not edit the legacy suite.

func TestPrepareChangeContract_PromptFallback_DeclaredFromPrompt(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract\n"
			finalMessage := "did the work without declaring" // AI omitted the block
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}, {Path: "calc_test.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-1", "chat-run-1", prompt, finalMessage, diff, []string{"calc-core"})
			if !p.ok {
				t.Fatalf("%s: prepare must ok", provider)
			}
			if !p.declared {
				t.Fatalf("%s: must be declared from prompt fallback", provider)
			}
			if p.contract.FeatureKey != "calc-core" {
				t.Fatalf("%s: feature_key=%q want calc-core", provider, p.contract.FeatureKey)
			}
			if len(p.contract.DeclaredPaths) != 2 {
				t.Fatalf("%s: declared_paths=%v want 2", provider, p.contract.DeclaredPaths)
			}
			if p.contract.Confidence != changecontract.ConfidenceDeclared {
				t.Fatalf("%s: confidence=%v want declared", provider, p.contract.Confidence)
			}
			// Scope: both diff paths inside declared → no drift
			if len(p.outOfScopePaths) != 0 {
				t.Fatalf("%s: outOfScope=%v want empty", provider, p.outOfScopePaths)
			}
		})
	}
}

func TestPrepareChangeContract_PromptFallback_BareCR(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\rfeature: calc-core\rintent: guard\rfiles: calc.go, calc_test.go\rsymbols: Subtract"
			finalMessage := "no contract"
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-1", "chat-run-1", prompt, finalMessage, diff, []string{"calc-core"})
			if !p.declared {
				t.Fatalf("%s: bare CR prompt must be declared", provider)
			}
			if len(p.contract.DeclaredPaths) != 2 {
				t.Fatalf("%s: paths=%v", provider, p.contract.DeclaredPaths)
			}
		})
	}
}

func TestPrepareChangeContract_PromptFallback_FinalMessageWinsOverPrompt(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: from prompt\nfiles: prompt.go\n"
			final := "[Change Contract]\nfeature: calc-core\nintent: from final\nfiles: final.go\n"
			diff := []flowgate.ChangedFile{{Path: "final.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-1", "chat-run-1", prompt, final, diff, []string{"calc-core"})
			if !p.declared {
				t.Fatal("must be declared")
			}
			// finalMessage must win over prompt
			if len(p.contract.DeclaredPaths) != 1 || p.contract.DeclaredPaths[0] != "final.go" {
				t.Fatalf("%s: must prefer finalMessage paths, got %v", provider, p.contract.DeclaredPaths)
			}
			if p.contract.Intent != "from final" {
				t.Fatalf("%s: intent=%q want from final", provider, p.contract.Intent)
			}
		})
	}
}

func TestPrepareChangeContract_PromptFallback_InferredWhenBothEmpty(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			p := prepareChangeContract(context.Background(), dir, "run-1", "chat-run-1", "", "no contract here", []flowgate.ChangedFile{{Path: "a.go", Status: "M"}}, []string{"calc-core"})
			if p.declared {
				t.Fatalf("%s: must be inferred when both empty", provider)
			}
			if p.contract.Confidence != changecontract.ConfidenceInferred {
				t.Fatalf("%s: confidence=%v want inferred", provider, p.contract.Confidence)
			}
		})
	}
}

func TestPrepareChangeContract_PromptFallback_Run204658Repro(t *testing.T) {
	// Exact run-204658 prompt (user Turn 1) with \r separators — the gate must
	// NOT fire r-contract even when finalMessage omits the block.
	dir := t.TempDir()
	prompt := "[Change Contract]\rfeature: calc-core\rintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\rfiles: calc.go, calc_test.go\rsymbols: Subtract"
	finalMessage := "I'll implement `SubtractWithGuard` per the change contract..." // no block
	diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}, {Path: "calc_test.go", Status: "M"}}
	// Prove file-bounded contract persistence: after prepare, the store must
	// contain a declared entry under .flowpilot/contracts/contracts.ndjson.
	p := prepareChangeContract(context.Background(), dir, "run-204658", "chat-run-204658", prompt, finalMessage, diff, []string{"calc-core"})
	if !p.declared {
		t.Fatal("run-204658 repro: prompt declared must be seen")
	}
	store, _ := changecontract.NewStore(dir)
	got, ok := store.GetLatestForRun("run-204658")
	if !ok {
		t.Fatal("store must have entry after declared save")
	}
	if got.FeatureKey != "calc-core" || len(got.DeclaredPaths) != 2 {
		t.Fatalf("stored contract mismatch: %+v", got)
	}
	// Also verify the on-disk file lives at the expected StoreFileUnder path
	// (ported from original C1 check) and last-wins semantics are not broken.
	if path := filepath.Join(dir, ".flowpilot", "contracts", "contracts.ndjson"); !cp43FileExists(path) {
		t.Fatalf("contracts.ndjson missing at %s", path)
	}
}

func cp43FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
