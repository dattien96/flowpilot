package agentpack

import "testing"

func TestPack_VibeIngestLoopsAtValidatorNotLock(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def FlowDefinition
	for _, f := range pack.Flows {
		if f.ID == "vibe-ingest" {
			def = f
			break
		}
	}
	if def.ID == "" {
		t.Fatal("missing vibe-ingest")
	}
	var validatorContinue, lockContinue, lockDone bool
	for _, e := range def.Edges {
		if e.From == "ss_validator" && e.To == "ss_converter" && e.When == "continue" {
			validatorContinue = true
		}
		if e.From == "ss_lock" && e.To == "ss_converter" && e.When == "continue" {
			lockContinue = true
		}
		if e.From == "ss_lock" && e.To == "cp_writer" && e.When == "done" {
			lockDone = true
		}
	}
	if !validatorContinue {
		t.Fatal("ss_validator --continue--> ss_converter missing (loop before lock)")
	}
	if lockContinue {
		t.Fatal("ss_lock must not --continue--> ss_converter (lock is once)")
	}
	if !lockDone {
		t.Fatal("ss_lock --done--> cp_writer missing")
	}
}
