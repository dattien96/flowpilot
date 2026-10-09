package agentpack

import "testing"

// TestVibeFlowsCloneable pins the Settings clone contract for the internal
// vibe-* family: every vibe flow must be cloneable so a user can copy it
// into an editable flow (e.g. to point it at a different model).
// Selectability and editability are intentionally untouched — these flows
// stay non-selectable engine internals (selectableIn stays empty,
// editable stays false).
func TestVibeFlowsCloneable(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	byID := make(map[string]FlowDefinition, len(pack.Flows))
	for _, def := range pack.Flows {
		byID[def.ID] = def
	}
	for _, id := range []string{
		"vibe-sprint",
		"vibe-tasks",
		"vibe-cp-ingest",
		"vibe-ingest",
		"vibe-adopt",
		"vibe-adopt-sprint",
		"vibe-owner-debate",
	} {
		def, ok := byID[id]
		if !ok {
			t.Fatalf("%s flow missing from builtin pack", id)
		}
		if !def.Builtin.Cloneable {
			t.Fatalf("%s must be cloneable:true (Settings Clone contract)", id)
		}
		// Pin the manifest side explicitly too: the runtime mirror path
		// only reads the flow file's own builtin.* copy, so a manifest-only
		// revert would silently have no effect (BUG-NOTE-CP42 #22).
		manifestFound := false
		manifestCloneable := false
		for _, mf := range pack.Manifest.Flows {
			if mf.Path == "flows/"+id+".yaml" {
				manifestFound = true
				manifestCloneable = mf.Cloneable
			}
		}
		if !manifestFound {
			t.Fatalf("%s missing from pack manifest", id)
		}
		if !manifestCloneable {
			t.Fatalf("manifest entry for %s must be cloneable:true", id)
		}
	}
}
