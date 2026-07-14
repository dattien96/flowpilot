package changecontract

import "testing"

func TestRetireHeadMergedCopiesDecisionsToTargetAndSetsSupersededBy(t *testing.T) {
	h := CanonicalHead{
		FeatureKey: "calc-core-old",
		Decisions:  []Decision{{Tried: "lookup table", Outcome: DecisionRejected, Reason: "no negative divisor support"}},
	}
	target := CanonicalHead{FeatureKey: "calc-core", Decisions: []Decision{{Tried: "prior dead end", Outcome: DecisionRejected}}}

	retired, targets := RetireHead(h, RetireActionMerged, []CanonicalHead{target})

	if retired.Status != RetireActionMerged {
		t.Fatalf("expected status merged, got %q", retired.Status)
	}
	if retired.RetiredAt == nil {
		t.Fatal("expected retired_at to be set")
	}
	if len(retired.SupersededBy) != 1 || retired.SupersededBy[0] != "calc-core" {
		t.Fatalf("expected superseded_by [calc-core], got %v", retired.SupersededBy)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 updated target, got %d", len(targets))
	}
	if len(targets[0].Decisions) != 2 {
		t.Fatalf("expected target decisions to include both the original and the folded-in one, got %+v", targets[0].Decisions)
	}
}

func TestRetireHeadRenamedCopiesDecisions(t *testing.T) {
	h := CanonicalHead{FeatureKey: "old-key", Decisions: []Decision{{Tried: "x", Outcome: DecisionRejected}}}
	target := CanonicalHead{FeatureKey: "new-key"}

	retired, targets := RetireHead(h, RetireActionRenamed, []CanonicalHead{target})

	if retired.Status != RetireActionRenamed {
		t.Fatalf("expected status renamed, got %q", retired.Status)
	}
	if len(targets[0].Decisions) != 1 {
		t.Fatalf("expected renamed target to receive the decision, got %+v", targets[0].Decisions)
	}
}

func TestRetireHeadDeprecatedDoesNotTouchTargets(t *testing.T) {
	h := CanonicalHead{FeatureKey: "old-key", Decisions: []Decision{{Tried: "x", Outcome: DecisionRejected}}}

	retired, targets := RetireHead(h, RetireActionDeprecated, nil)

	if retired.Status != RetireActionDeprecated {
		t.Fatalf("expected status deprecated, got %q", retired.Status)
	}
	if retired.RetiredAt == nil {
		t.Fatal("expected retired_at to be set for a deprecated Head")
	}
	if len(targets) != 0 {
		t.Fatalf("expected no targets for a deprecated Head, got %+v", targets)
	}
}

func TestRetireHeadNeverDeletesDecisionsFromRetiredHead(t *testing.T) {
	h := CanonicalHead{FeatureKey: "old-key", Decisions: []Decision{{Tried: "x", Outcome: DecisionRejected}}}
	retired, _ := RetireHead(h, RetireActionDeprecated, nil)
	if len(retired.Decisions) != 1 {
		t.Fatalf("expected the retired Head's own Decisions to remain intact for provenance, got %+v", retired.Decisions)
	}
}
