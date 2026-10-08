package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// BUG-651 (live run-523131, root defect of the Task-113 wedge): a sprint leg
// binding its change contract fell through to InferFromDiff on a dirty
// worktree — `.flowpilot/` bookkeeping + prior-task output minted a bogus
// `app-bootstrap`/empty-scope contract under gate_mode:enforce, so all 33
// declared sprint paths read out-of-scope and every write blocked. The
// step's FROZEN contract must govern; inference is for unfrozen work only.
func TestBug651_FrozenContractBeatsInference(t *testing.T) {
	dir := t.TempDir()
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- feature-vault — vault feature\n")
	frozen := &changecontract.FrozenContractRecord{
		ContractID:    "c-9c2fb307",
		Version:       1,
		RunID:         "run-651",
		CoderStepID:   "coder",
		FeatureKey:    "feature-vault",
		Intent:        "CP-11 vault scope",
		DeclaredPaths: []string{"feature/vault/data/src/Repo.kt", "feature/vault/presentation/src/VM.kt"},
		DeclaredAt:    time.Now(),
	}
	// The live wedge shape: dirty diff of bookkeeping + a real declared path.
	diff := []flowgate.ChangedFile{
		{Path: ".flowpilot/ledger/feature_history.ndjson", Status: "M"},
		{Path: "feature/vault/data/src/Repo.kt", Status: "M"},
	}
	p := prepareChangeContract(context.Background(), dir, "run-651", "coder", "", "prose only — no declaration", diff, []string{"app-bootstrap"}, frozen)
	if !p.declared || p.contract.Confidence != changecontract.ConfidenceDeclared {
		t.Fatalf("frozen-governed run must bind a declared contract: declared=%v confidence=%q", p.declared, p.contract.Confidence)
	}
	if p.contract.FeatureKey != "feature-vault" {
		t.Fatalf("contract feature_key=%q, want frozen feature-vault (inference suggested app-bootstrap)", p.contract.FeatureKey)
	}
	if len(p.contract.DeclaredPaths) != len(frozen.DeclaredPaths) {
		t.Fatalf("declared_paths=%v, want the frozen scope %v", p.contract.DeclaredPaths, frozen.DeclaredPaths)
	}
	// The in-scope write passes; only genuinely out-of-scope dirt flags.
	for _, pth := range p.outOfScopePaths {
		if pth == "feature/vault/data/src/Repo.kt" {
			t.Fatalf("declared sprint path flagged out-of-scope — the wedge shape is back: %v", p.outOfScopePaths)
		}
	}
	// And the frozen scope lands durably as a declared row so later turns and
	// GetLatestForRun stay bound to it (the live operator repair shape).
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	latest, ok := store.GetLatestForRun("run-651")
	if !ok || latest.Confidence != changecontract.ConfidenceDeclared || latest.FeatureKey != "feature-vault" {
		t.Fatalf("frozen scope not persisted as declared row: ok=%v latest=%+v", ok, latest)
	}
}

// Near-miss guard: with no frozen contract the legacy declared→inferred
// resolution is untouched — inference still runs for unfrozen work.
func TestBug651_InferenceOnlyWhenUnfrozen(t *testing.T) {
	dir := t.TempDir()
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- calc-core — calculator core\n")
	diff := []flowgate.ChangedFile{{Path: "src/calc.go", Status: "M"}}
	p := prepareChangeContract(context.Background(), dir, "run-651b", "chat", "", "done", diff, []string{"calc-core"})
	if p.declared || p.contract.Confidence != changecontract.ConfidenceInferred {
		t.Fatalf("unfrozen run must still infer: declared=%v confidence=%q", p.declared, p.contract.Confidence)
	}
	if len(p.contract.DeclaredPaths) == 0 {
		t.Fatal("unfrozen inference produced empty scope — regression")
	}
}
