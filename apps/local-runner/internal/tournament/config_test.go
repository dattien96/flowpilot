package tournament

import (
	"strings"
	"testing"
)

func TestParseTournamentConfigDefaults(t *testing.T) {
	cfg, err := ParseTournamentConfig(nil)
	if err != nil {
		t.Fatalf("nil config must yield R-1 defaults: %v", err)
	}
	if len(cfg.Candidates) != 2 || cfg.Candidates[0].Provider != "claude" || cfg.Candidates[1].Provider != "codex" {
		t.Fatalf("default candidates must be Claude+Codex, got %+v", cfg.Candidates)
	}
	if !cfg.AutoPick || cfg.MaxAttempts != 1 || cfg.Serial {
		t.Fatalf("defaults must be auto_pick=true max_attempts=1 serial=false, got %+v", cfg)
	}
}

func TestParseTournamentConfigAcceptsThree(t *testing.T) {
	cfg, err := ParseTournamentConfig(map[string]any{
		"candidates": []any{
			map[string]any{"candidate_id": "a", "provider": "claude", "model": "claude-sonnet"},
			map[string]any{"candidate_id": "b", "provider": "codex", "model": "gpt-5.4-mini"},
			map[string]any{"candidate_id": "c", "provider": "grok", "model": "grok-4.5"},
		},
		"auto_pick":    false,
		"max_attempts": 2,
		"serial":       true,
	})
	if err != nil {
		t.Fatalf("3-candidate config must parse: %v", err)
	}
	if len(cfg.Candidates) != 3 || cfg.Candidates[2].Provider != "grok" {
		t.Fatalf("third candidate must be grok, got %+v", cfg.Candidates)
	}
	if cfg.AutoPick || cfg.MaxAttempts != 2 || !cfg.Serial {
		t.Fatalf("flags must round-trip, got %+v", cfg)
	}
}

func TestParseTournamentConfigRejectsBadInput(t *testing.T) {
	bad := []map[string]any{
		{"candidates": []any{}},
		{"candidates": []any{
			map[string]any{"candidate_id": "a", "provider": "claude", "model": "m"},
			map[string]any{"candidate_id": "b", "provider": "codex", "model": "m"},
			map[string]any{"candidate_id": "c", "provider": "grok", "model": "m"},
			map[string]any{"candidate_id": "d", "provider": "grok", "model": "m"},
		}},
		{"candidates": []any{map[string]any{"candidate_id": "a", "provider": "gemini", "model": "m"}}},
		{"candidates": []any{
			map[string]any{"candidate_id": "a", "provider": "claude", "model": "m"},
			map[string]any{"candidate_id": "a", "provider": "codex", "model": "m"},
		}},
		{"max_attempts": 0},
		{"max_attempts": 4},
		{"auto_pick": "yes"},
	}
	for i, raw := range bad {
		if _, err := ParseTournamentConfig(raw); err == nil {
			t.Fatalf("bad config %d (%v) must fail", i, raw)
		}
	}
}

func TestDecideTournamentActionMatrix(t *testing.T) {
	winner := TournamentVerdict{WinnerCandidateID: "candidate-a"}
	tie := TournamentVerdict{NeedsHumanDecision: true, Reason: "tie"}
	cases := []struct {
		name         string
		verdict      TournamentVerdict
		autoPick     bool
		attempt, max int
		want         string
	}{
		{"winner merges", winner, true, 1, 2, TournamentActionMerge},
		{"tie retries when attempts left", tie, true, 1, 2, TournamentActionRetry},
		{"tie asks when out of attempts", tie, true, 2, 2, TournamentActionAsk},
		{"tie asks when auto_pick off", tie, false, 1, 2, TournamentActionAsk},
		{"default max_attempts=1 never retries", tie, true, 1, 1, TournamentActionAsk},
	}
	for _, tc := range cases {
		if got := DecideTournamentAction(tc.verdict, tc.autoPick, tc.attempt, tc.max); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDistillFailureBriefNamesCandidates(t *testing.T) {
	var arb TournamentArbiter
	verdict := arb.Decide([]CandidateResult{
		{CandidateID: "candidate-a", TotalTests: 10, PassedTests: 8},
		{CandidateID: "candidate-b", TotalTests: 10, PassedTests: 8},
	})
	brief := DistillFailureBrief(verdict, 1)
	if !strings.Contains(brief, "candidate-a") || !strings.Contains(brief, "candidate-b") {
		t.Fatalf("brief must name the tied candidates, got %q", brief)
	}
	if !strings.Contains(brief, "clean slate") {
		t.Fatalf("brief must steer away from the dead direction, got %q", brief)
	}
}
