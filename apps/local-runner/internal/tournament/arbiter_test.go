package tournament

import (
	"testing"
)

func TestArbiterPrefersCandidateWithHigherTestPassRate(t *testing.T) {
	var arb TournamentArbiter
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: 10, PassedTests: 6},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: 10, PassedTests: 10},
	})
	if verdict.NeedsHumanDecision {
		t.Fatalf("expected a winner, got human-decision: %s", verdict.Reason)
	}
	if verdict.WinnerCandidateID != "candidate-b" {
		t.Fatalf("expected candidate-b to win on higher pass rate, got %q", verdict.WinnerCandidateID)
	}
	var sa, sb CandidateScore
	for _, s := range verdict.Ranking {
		switch s.CandidateID {
		case "candidate-a":
			sa = s
		case "candidate-b":
			sb = s
		}
	}
	if sa.STests != 0.6 || sb.STests != 1.0 {
		t.Fatalf("STests must be exact pass ratios, got a=%.4f b=%.4f", sa.STests, sb.STests)
	}
	if sb.Total != 1.0*weightTests+1.0*weightLSP+1.0*weightBlast {
		t.Fatalf("perfect candidate must total the full weight sum, got %.4f", sb.Total)
	}
}

func TestArbiterPenalizesLSPCompilerErrors(t *testing.T) {
	var arb TournamentArbiter
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: 10, PassedTests: 10, LSPErrors: 5},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: 10, PassedTests: 10, LSPErrors: 0},
	})
	if verdict.WinnerCandidateID != "candidate-b" {
		t.Fatalf("expected clean-LSP candidate-b to win, got %q", verdict.WinnerCandidateID)
	}
	var sa, sb CandidateScore
	for _, s := range verdict.Ranking {
		switch s.CandidateID {
		case "candidate-a":
			sa = s
		case "candidate-b":
			sb = s
		}
	}
	if sb.SLsp != 1.0 {
		t.Fatalf("0 errors must score SLsp 1.0, got %.4f", sb.SLsp)
	}
	if sa.SLsp != 0.5 {
		t.Fatalf("5 errors must score SLsp 0.5, got %.4f", sa.SLsp)
	}
	if sa.SLsp >= sb.SLsp {
		t.Fatalf("more errors must score strictly lower: a=%.4f b=%.4f", sa.SLsp, sb.SLsp)
	}
}

func TestArbiterPrefersLowerBlastRadiusOnTie(t *testing.T) {
	var arb TournamentArbiter
	// Identical tests + LSP, different dependents: totals tie before the
	// SBlast component separates them.
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: 10, PassedTests: 8, DependentsCount: 7},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: 10, PassedTests: 8, DependentsCount: 0},
	})
	if verdict.NeedsHumanDecision {
		t.Fatalf("SBlast must break the tie, got human-decision: %s", verdict.Reason)
	}
	if verdict.WinnerCandidateID != "candidate-b" {
		t.Fatalf("expected low-blast candidate-b to win the tie, got %q", verdict.WinnerCandidateID)
	}
}

func TestArbiterDisqualifiesCandidateBreakingExistingTests(t *testing.T) {
	var arb TournamentArbiter
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: 10, PassedTests: 10, BrokeExistingTests: true},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: 10, PassedTests: 4},
	})
	if verdict.WinnerCandidateID != "candidate-b" {
		t.Fatalf("regressing candidate must never win, got %q", verdict.WinnerCandidateID)
	}
	for _, s := range verdict.Ranking {
		if s.CandidateID == "candidate-a" {
			if !s.Disqualified {
				t.Fatal("regressing candidate must be flagged Disqualified")
			}
			if s.Total != 0 {
				t.Fatalf("disqualified candidate must total 0, got %.4f", s.Total)
			}
		}
	}
}

func TestArbiterAllDisqualifiedNeedsHumanDecision(t *testing.T) {
	var arb TournamentArbiter
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: 10, PassedTests: 10, BrokeExistingTests: true},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: 10, PassedTests: 9, BrokeExistingTests: true},
	})
	if !verdict.NeedsHumanDecision {
		t.Fatalf("all-disqualified field must escalate, got winner %q", verdict.WinnerCandidateID)
	}
	if verdict.WinnerCandidateID != "" {
		t.Fatalf("human-decision verdict must name no winner, got %q", verdict.WinnerCandidateID)
	}
	if len(verdict.Ranking) != 2 {
		t.Fatalf("ranking must stay visible as the decision card, got %d entries", len(verdict.Ranking))
	}
}
