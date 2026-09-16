package agentpack

import (
	"testing"
)

// Scenario: builtin pack count after Task-321, CP-64 and CP-65 P-3.
// Input: LoadBuiltinPack()
// Expect: len(Flows)==13 (CP-65 P-3 adds tournament-harness, authorized
// inventory change); len(Agents)==9 (CP-64 added agents/reproducer.md;
// CP-65 reuses agents/coder.md, no new agent).
func TestPack_InventoryUnchanged(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	if len(pack.Flows) != 13 {
		t.Fatalf("flows=%d, want 13", len(pack.Flows))
	}
	if len(pack.Agents) != 9 {
		t.Fatalf("agents=%d, want 9 (%v)", len(pack.Agents), SortedAgentNames(pack.Agents))
	}
}

// Scenario: vibe flows stay hidden from the Dev selectableIn catalog.
func TestPack_VibeSelectableInEmpty(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	want := map[string]struct{}{
		"vibe-ingest": {}, "vibe-sprint": {}, "vibe-owner-debate": {}, "vibe-cp-ingest": {},
	}
	found := 0
	for _, def := range pack.Flows {
		if _, ok := want[def.ID]; !ok {
			continue
		}
		found++
		if len(def.Builtin.SelectableIn) != 0 {
			t.Fatalf("%s selectableIn=%v, want empty", def.ID, def.Builtin.SelectableIn)
		}
	}
	if found != 4 {
		t.Fatalf("found %d vibe flows, want 4", found)
	}
}
