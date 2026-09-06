package agentpack

import (
	"testing"
)

// Dual-loop cap (plan review + code review share one Round budget, reset on
// plan approve): the two dual-loop harnesses get cap 5 per phase so a
// contested plan cannot starve the code review loop. Single-loop flows keep
// cap 3. New file — no pre-existing test is modified.
func TestDualLoopHarnessCapIsFive(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, id := range []string{"task-harness", "bug-plan-harness"} {
		def, ok := findFlowByID(pack.Flows, id)
		if !ok {
			t.Fatalf("%s flow missing from builtin pack", id)
		}
		if def.Policy.Cap != 5 {
			t.Fatalf("%s policy.cap = %d, want 5 (dual-loop per-phase budget)", id, def.Policy.Cap)
		}
	}
}

func TestSingleLoopHarnessCapStaysThree(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, id := range []string{"bug-harness", "rag-harness", "cp-harness", "review-loop"} {
		def, ok := findFlowByID(pack.Flows, id)
		if !ok {
			t.Fatalf("%s flow missing from builtin pack", id)
		}
		if def.Policy.Cap != 3 {
			t.Fatalf("%s policy.cap = %d, want 3 (single-loop flows keep the old envelope)", id, def.Policy.Cap)
		}
	}
}

// TestReviewLoopHiddenFromPickers locks the rag-harness hiding pattern:
// review-loop stays in the pack (cloneable reference, startResolvedFlow
// still resolves it) but offers no picker face in /flow or chat.
func TestReviewLoopHiddenFromPickers(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "review-loop")
	if !ok {
		t.Fatal("review-loop flow missing from builtin pack (hidden, not deleted)")
	}
	if len(def.Builtin.SelectableIn) != 0 {
		t.Fatalf("review-loop selectableIn = %v, want [] (hidden from all pickers)", def.Builtin.SelectableIn)
	}
	if len(def.Builtin.ChatSubModes) != 0 {
		t.Fatalf("review-loop chatSubModes = %v, want [] (no stale Bug face)", def.Builtin.ChatSubModes)
	}
	if !def.Builtin.Cloneable {
		t.Fatal("review-loop must stay cloneable:true (reference template)")
	}
}
