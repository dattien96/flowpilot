package workingmode

import "testing"

func TestLooksLikeVibeFlow_IDsAndCatalogNames(t *testing.T) {
	yes := []string{
		"vibe-ingest",
		"vibe-cp-ingest",
		"vibe-sprint",
		"vibe-owner-debate",
		"flowpilot-core-flow-pack/vibe-cp-ingest",
		"Vibe Cp Ingest",
		"Vibe Owner Debate",
		"Vibe Sprint",
	}
	for _, s := range yes {
		if !LooksLikeVibeFlow(s) {
			t.Fatalf("%q must look like vibe", s)
		}
	}
	no := []string{"task-harness", "Task Harness", "bug-plan-harness", "cp-harness-smoke", ""}
	for _, s := range no {
		if LooksLikeVibeFlow(s) {
			t.Fatalf("%q must not look like vibe", s)
		}
	}
}
