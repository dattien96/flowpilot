package tournament

import (
	"fmt"
	"sort"
)

// Scoring weights (CP-65 §3.2, Task-368 T-2..T-4).
const (
	weightTests = 0.5
	weightLSP   = 0.3
	weightBlast = 0.2
)

// lspErrorPenalty is the linear deduction per LSP compiler error (Task-368
// T-3 calibration: 0 errors -> 1.0, 10+ errors -> 0.0 floor).
const lspErrorPenalty = 0.1

// TournamentArbiter scores candidates deterministically: same input always
// yields the same verdict (unit tests assert exact scores). Stateless.
type TournamentArbiter struct{}

// BlastBucket maps a dependents count to the S_blast bucket (Task-368 T-3,
// thresholds locked from CP-65 §3.2): 0 -> Low 1.0; 1-3 -> Medium 0.7;
// 4-10 -> High 0.3; >10 -> Critical 0.0. Negative counts clamp to 0.
func BlastBucket(count int) float64 {
	if count < 0 {
		count = 0
	}
	switch {
	case count == 0:
		return 1.0
	case count <= 3:
		return 0.7
	case count <= 10:
		return 0.3
	default:
		return 0.0
	}
}

// lspScore maps an error count to S_lsp: 1.0 at zero errors, linear decay,
// floored at 0 (Task-368 T-3).
func lspScore(errors int) float64 {
	if errors < 0 {
		errors = 0
	}
	s := 1.0 - lspErrorPenalty*float64(errors)
	if s < 0 {
		return 0
	}
	return s
}

// testScore maps passed/total to S_tests. Zero total means no evidence ran,
// which scores 0 (never a free pass). Inputs clamp into [0, total].
func testScore(passed, total int) float64 {
	if total <= 0 {
		return 0
	}
	if passed < 0 {
		passed = 0
	}
	if passed > total {
		passed = total
	}
	return float64(passed) / float64(total)
}

// Score breaks one candidate down into weighted components. A candidate
// with BrokeExistingTests is disqualified: Total forced to 0 so it can
// never win, while components stay visible for the decision card.
func (TournamentArbiter) Score(r CandidateResult) CandidateScore {
	s := CandidateScore{
		CandidateID:  r.CandidateID,
		STests:       testScore(r.PassedTests, r.TotalTests),
		SLsp:         lspScore(r.LSPErrors),
		SBlast:       BlastBucket(r.DependentsCount),
		Disqualified: r.BrokeExistingTests,
	}
	if s.Disqualified {
		return s
	}
	s.Total = s.STests*weightTests + s.SLsp*weightLSP + s.SBlast*weightBlast
	return s
}

// Decide ranks every candidate and picks the winner. Ordering is fully
// deterministic: Total desc, then SBlast desc, then CandidateID asc.
// A candidate is eligible only when not disqualified. An exact tie on both
// Total and SBlast between the top two eligible candidates — or zero
// eligible candidates at all — yields NeedsHumanDecision instead of a
// random pick (Task-368 T-5, CP-65 §7).
func (TournamentArbiter) Decide(results []CandidateResult) TournamentVerdict {
	var arb TournamentArbiter
	ranking := make([]CandidateScore, 0, len(results))
	for _, r := range results {
		ranking = append(ranking, arb.Score(r))
	}
	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].Total != ranking[j].Total {
			return ranking[i].Total > ranking[j].Total
		}
		if ranking[i].SBlast != ranking[j].SBlast {
			return ranking[i].SBlast > ranking[j].SBlast
		}
		return ranking[i].CandidateID < ranking[j].CandidateID
	})

	eligible := ranking[:0:0]
	for _, s := range ranking {
		if !s.Disqualified {
			eligible = append(eligible, s)
		}
	}
	human := func(reason string) TournamentVerdict {
		return TournamentVerdict{Ranking: ranking, NeedsHumanDecision: true, Reason: reason}
	}
	switch len(eligible) {
	case 0:
		if len(ranking) == 0 {
			return human("no candidates submitted")
		}
		return human("all candidates disqualified (broke existing tests)")
	case 1:
		return TournamentVerdict{
			WinnerCandidateID: eligible[0].CandidateID,
			Ranking:           ranking,
			Reason:            fmt.Sprintf("sole eligible candidate %s", eligible[0].CandidateID),
		}
	default:
		top, next := eligible[0], eligible[1]
		if top.Total == next.Total && top.SBlast == next.SBlast {
			return human(fmt.Sprintf("tie between %s and %s at total %.4f — human decision required",
				top.CandidateID, next.CandidateID, top.Total))
		}
		return TournamentVerdict{
			WinnerCandidateID: top.CandidateID,
			Ranking:           ranking,
			Reason:            fmt.Sprintf("winner %s at total %.4f", top.CandidateID, top.Total),
		}
	}
}
