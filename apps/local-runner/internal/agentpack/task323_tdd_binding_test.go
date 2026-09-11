package agentpack

import "testing"

func TestPack_VibeSprintTddSignaturesBinding(t *testing.T) {
	def := loadVibeSprint(t)
	tdd := nodeByID(def, "tdd")
	found := false
	for _, b := range tdd.ArtifactBindings {
		if b.SlotName == "tdd_signatures" && b.Direction == "output" && b.Required {
			found = true
			got, _ := b.ConfigJSON["pathTemplate"].(string)
			if got != "requirements/.flowpilot/vibe/tdd-signatures.md" {
				t.Fatalf("pathTemplate=%q", got)
			}
		}
	}
	if !found {
		t.Fatal("tdd missing required tdd_signatures output")
	}
}
