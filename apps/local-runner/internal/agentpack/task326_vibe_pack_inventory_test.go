package agentpack

import (
	"testing"
)

// Scenario: builtin pack count after Task-321, CP-64, CP-65 P-3 and CP-67.
// Input: LoadBuiltinPack()
// Expect: len(Flows)==14; len(Agents)==11 (CP-67 P-3 added
// agents/scaffold-architect.md; CA-1151 added agents/spec-aligner.md —
// both authorized inventory changes).
func TestPack_InventoryUnchanged(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	// CP-90: +vibe-tasks (sprint pre-broken tasks under a locked CP).
	if len(pack.Flows) != 14 {
		t.Fatalf("flows=%d, want 14", len(pack.Flows))
	}
	if len(pack.Agents) != 11 {
		t.Fatalf("agents=%d, want 11 (%v)", len(pack.Agents), SortedAgentNames(pack.Agents))
	}
}

// Scenario: vibe flows stay hidden from the Dev selectableIn catalog.
func TestPack_VibeSelectableInEmpty(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	want := map[string]struct{}{
		"vibe-ingest": {}, "vibe-sprint": {}, "vibe-owner-debate": {}, "vibe-cp-ingest": {}, "vibe-tasks": {},
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
	if found != 5 {
		t.Fatalf("found %d vibe flows, want 5", found)
	}
}

// Scenario: CP-90 vibe-tasks topology — cp_reader/validator/lock head is
// byte-identical to vibe-cp-ingest, then task_plan_reader (read-only) instead
// of task_slicer; no node may write Task files.
func TestPack_VibeTasksTopologySkipsSlicer(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def *FlowDefinition
	for i := range pack.Flows {
		if pack.Flows[i].ID == "vibe-tasks" {
			def = &pack.Flows[i]
			break
		}
	}
	if def == nil {
		t.Fatal("vibe-tasks missing from builtin pack")
	}
	ids := make([]string, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		ids = append(ids, n.ID)
		if n.ID == "task_slicer" {
			t.Fatal("vibe-tasks must not contain task_slicer")
		}
	}
	want := []string{"cp_reader", "cp_validator", "cp_lock", "task_plan_reader"}
	if len(ids) != len(want) {
		t.Fatalf("vibe-tasks nodes=%v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("vibe-tasks node[%d]=%q, want %q (nodes=%v)", i, ids[i], want[i], ids)
		}
	}
}
