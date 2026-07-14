package changecontract

import "testing"

func TestUpdateHeadRefreshesBehaviorAndStaysCurrent(t *testing.T) {
	h := CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "old statement",
		Status:            HeadStatusCurrent,
	}
	before := ComputeSignature(h)

	c := Contract{Intent: "new refined statement"}
	updated := UpdateHead(h, c)

	if updated.BehaviorStatement != "new refined statement" {
		t.Fatalf("expected behavior_statement refreshed from contract intent, got %q", updated.BehaviorStatement)
	}
	if updated.Status != HeadStatusCurrent {
		t.Fatalf("expected status current after UpdateHead, got %q", updated.Status)
	}
	if updated.IntentSignature == before {
		t.Fatal("expected the signature to change when the behavior statement changes")
	}
	if updated.IntentSignature != ComputeSignature(updated) {
		t.Fatal("expected IntentSignature to be recomputed consistently")
	}
}

func TestUpdateHeadKeepsExistingStatementWhenContractIntentEmpty(t *testing.T) {
	h := CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "kept statement"}
	updated := UpdateHead(h, Contract{})
	if updated.BehaviorStatement != "kept statement" {
		t.Fatalf("expected behavior_statement to be preserved when contract intent is empty, got %q", updated.BehaviorStatement)
	}
}

func TestRebaselineWithSpecTransitionsSpecLessToCurrent(t *testing.T) {
	dir := t.TempDir()
	writeGoverningDoc(t, dir, "SS-14-Code-Context-And-Regression-Safety", "spec content")

	h := CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "add zero-divisor guard",
		Status:            HeadStatusSpecLess,
		SpecConfidence:    SpecConfidenceSpecLess,
	}

	updated := RebaselineWithSpec(dir, h, []string{"SS-14-Code-Context-And-Regression-Safety"})

	if updated.Status != HeadStatusCurrent {
		t.Fatalf("expected status current after RebaselineWithSpec, got %q", updated.Status)
	}
	if updated.SpecConfidence != SpecConfidenceSpecBacked {
		t.Fatalf("expected spec_confidence spec_backed after RebaselineWithSpec, got %q", updated.SpecConfidence)
	}
	if len(updated.GoverningDocIDs) != 1 || updated.GoverningDocIDs[0] != "SS-14-Code-Context-And-Regression-Safety" {
		t.Fatalf("expected governing_doc_ids to be set, got %v", updated.GoverningDocIDs)
	}
	if updated.GoverningDocHashes["SS-14-Code-Context-And-Regression-Safety"] == "" {
		t.Fatal("expected a non-empty hash recorded for the newly attached governing doc")
	}
}
