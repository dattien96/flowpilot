// Package tournament implements the CP-65 multi-candidate tournament
// (Parallel-Distill-Refine) harness: N independent candidates solve the same
// hard problem in isolated git worktrees, and a deterministic Go arbiter
// (never an LLM) scores them 50% test pass rate + 30% LSP diagnostics +
// 20% GitNexus blast radius, then picks the winner to merge.
//
// The package is intentionally dependency-free: it scores pre-aggregated
// CandidateResult numbers only and never imports flowgate, lsp, structure or
// changecontract (provider parity Case-1 agnostic; metric collection in real
// worktrees is P-3 glue in the runner). It also never imports tui/ and
// creates no dependency cycle (same contract as internal/lsp).
package tournament

// CandidateResult is the aggregated outcome of one tournament candidate.
// All metrics arrive pre-collected by the P-3 execution glue (suite run,
// lsp.ServerSet.CheckFiles, structure.Provider.Dependents via
// changecontract.GitNexusQueryTargetsForPath); the arbiter only scores.
type CandidateResult struct {
	// CandidateID is the stable id declared in the tournament flow config
	// (e.g. "candidate-a"). Used for ranking order and the verdict.
	CandidateID string
	// Label is the human display name (e.g. "Claude strategy A").
	Label string
	// ProviderKey is a label only (e.g. "claude", "codex", "grok").
	// Scoring logic MUST NOT branch on it.
	ProviderKey string
	// TotalTests / PassedTests come from the suite run inside the
	// candidate's worktree.
	TotalTests  int
	PassedTests int
	// BrokeExistingTests marks a candidate that regressed a pre-existing
	// test. Such a candidate is disqualified outright (CP-65 §3.2: zero
	// points for breaking old tests — same regression-safety spirit as
	// the CP-64 r-reg oracle).
	BrokeExistingTests bool
	// LSPErrors is the error count from CP-63 diagnostics collection.
	LSPErrors int
	// ChangedPaths are the repo-relative files the candidate touched.
	ChangedPaths []string
	// DependentsCount is the DependentsSummary.Count across ChangedPaths
	// (max over paths, computed by the P-3 glue).
	DependentsCount int
}

// CandidateScore is the arbiter's deterministic breakdown for one candidate.
// Disqualified candidates keep Total 0 and stay visible in the ranking
// (decision-card transparency) but can never become the winner.
type CandidateScore struct {
	CandidateID  string
	STests       float64
	SLsp         float64
	SBlast       float64
	Total        float64
	Disqualified bool
}

// TournamentVerdict is the arbiter's final call. When NeedsHumanDecision is
// true, WinnerCandidateID is empty and the caller must escalate to ask_user
// with the Ranking as the decision card (CP-65 §7 failure case; Q-1
// auto_pick wiring lands in P-3).
type TournamentVerdict struct {
	WinnerCandidateID  string
	Ranking            []CandidateScore
	NeedsHumanDecision bool
	Reason             string
}
