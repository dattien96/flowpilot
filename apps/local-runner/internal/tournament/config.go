package tournament

import (
	"fmt"
	"sort"
	"strings"
)

// Tournament candidate orchestration config (CP-65 P-3). The pack parser
// carries the raw `config:` map of the parallel_rollout / tournament_arbiter
// nodes (see agentpack.FlowNode.Config); this file validates it into a typed
// config and derives the arbiter node's next action. Pure Go, no git, no
// providers — provider names are validated labels only, never dispatch keys.

// Known tournament providers (Task-370 Case-3 per-provider surface: the
// config may name Claude/Codex/Grok; default runs 2 per R-1).
var tournamentProviders = map[string]bool{"claude": true, "codex": true, "grok": true}

// Default candidates (CP-65 §9 R-1): Claude + Codex. A third (Grok) is
// accepted by config; live 3-way rollout expansion is a P-5 follow-up.
var defaultCandidates = []TournamentCandidateConfig{
	{CandidateID: "candidate-a", Provider: "claude", Model: "claude-sonnet"},
	{CandidateID: "candidate-b", Provider: "codex", Model: "gpt-5.4-mini"},
}

// Tournament action constants: the tournament_arbiter node's next edge.
const (
	TournamentActionMerge = "merge" // winner picked -> merge_and_audit (done)
	TournamentActionRetry = "retry" // tie/all-fail + attempts left -> parallel_rollout (back)
	TournamentActionAsk   = "ask"   // out of attempts / auto_pick off -> ask_user (escalate)
)

// TournamentCandidateConfig is one entry of the parallel_rollout `candidates`
// list: which provider/model solves as this candidate in its own worktree.
type TournamentCandidateConfig struct {
	CandidateID string
	Provider    string
	Model       string
}

// TournamentNodeConfig is the validated tournament flow config.
type TournamentNodeConfig struct {
	Candidates  []TournamentCandidateConfig
	AutoPick    bool
	MaxAttempts int
	Serial      bool
}

// ParseTournamentConfig validates a raw node `config:` map. Absent config
// yields the R-1 defaults (2 candidates, auto_pick on, 1 attempt = the
// pre-retry plan behavior, parallel rollout). Explicit values must satisfy:
// 1-3 candidates with unique ids and known providers, max_attempts in
// [1,3] (token guard — retry is opt-in, default off).
func ParseTournamentConfig(raw map[string]any) (TournamentNodeConfig, error) {
	cfg := TournamentNodeConfig{AutoPick: true, MaxAttempts: 1}
	if len(raw) == 0 {
		cfg.Candidates = append([]TournamentCandidateConfig(nil), defaultCandidates...)
		return cfg, nil
	}
	if v, ok := raw["auto_pick"]; ok {
		b, ok := v.(bool)
		if !ok {
			return cfg, fmt.Errorf("tournament: auto_pick must be bool, got %T", v)
		}
		cfg.AutoPick = b
	}
	if v, ok := raw["max_attempts"]; ok {
		n, ok := intField(v)
		if !ok || n < 1 || n > 3 {
			return cfg, fmt.Errorf("tournament: max_attempts must be an int in [1,3], got %v", v)
		}
		cfg.MaxAttempts = n
	}
	if v, ok := raw["serial"]; ok {
		b, ok := v.(bool)
		if !ok {
			return cfg, fmt.Errorf("tournament: serial must be bool, got %T", v)
		}
		cfg.Serial = b
	}
	rawCands, ok := raw["candidates"]
	if !ok {
		cfg.Candidates = append([]TournamentCandidateConfig(nil), defaultCandidates...)
		return cfg, nil
	}
	list, ok := rawCands.([]any)
	if !ok || len(list) == 0 {
		return cfg, fmt.Errorf("tournament: candidates must be a non-empty list")
	}
	if len(list) > 3 {
		return cfg, fmt.Errorf("tournament: at most 3 candidates, got %d", len(list))
	}
	seen := map[string]bool{}
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return cfg, fmt.Errorf("tournament: candidates[%d] must be a map", i)
		}
		c := TournamentCandidateConfig{
			CandidateID: strings.TrimSpace(stringValue(m["candidate_id"])),
			Provider:    strings.ToLower(strings.TrimSpace(stringValue(m["provider"]))),
			Model:       strings.TrimSpace(stringValue(m["model"])),
		}
		if c.CandidateID == "" || c.Provider == "" || c.Model == "" {
			return cfg, fmt.Errorf("tournament: candidates[%d] needs candidate_id, provider and model", i)
		}
		if !tournamentProviders[c.Provider] {
			return cfg, fmt.Errorf("tournament: candidates[%d] unknown provider %q (want claude/codex/grok)", i, c.Provider)
		}
		if seen[c.CandidateID] {
			return cfg, fmt.Errorf("tournament: duplicate candidate_id %q", c.CandidateID)
		}
		seen[c.CandidateID] = true
		cfg.Candidates = append(cfg.Candidates, c)
	}
	return cfg, nil
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func intField(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// DecideTournamentAction maps an arbiter verdict onto the tournament_arbiter
// node's next edge (CP-65 retry design 2026-09-16): a winner merges; a human
// verdict retries only when auto_pick is on and attempts remain (same models,
// fresh sub-agent spawns + distilled brief — context cleared, no model
// rotation); otherwise it escalates to ask_user. attempt is 1-based.
func DecideTournamentAction(verdict TournamentVerdict, autoPick bool, attempt, maxAttempts int) string {
	if !verdict.NeedsHumanDecision && verdict.WinnerCandidateID != "" {
		return TournamentActionMerge
	}
	if !autoPick {
		return TournamentActionAsk
	}
	if attempt < maxAttempts {
		return TournamentActionRetry
	}
	return TournamentActionAsk
}

// DistillFailureBrief renders the previous attempt's verdict into the compact
// brief the next rollout's fresh sub-agents receive (the Distill in PDR):
// what tied/failed and their scores — enough to steer away from the dead
// direction without carrying the contaminated transcript.
func DistillFailureBrief(verdict TournamentVerdict, attempt int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tournament attempt %d failed: %s.", attempt+1, verdict.Reason)
	ranking := append([]CandidateScore(nil), verdict.Ranking...)
	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].Total != ranking[j].Total {
			return ranking[i].Total > ranking[j].Total
		}
		return ranking[i].CandidateID < ranking[j].CandidateID
	})
	shown := 3
	if len(ranking) < shown {
		shown = len(ranking)
	}
	for i := 0; i < shown; i++ {
		s := ranking[i]
		tag := ""
		if s.Disqualified {
			tag = " [disqualified: broke existing tests]"
		}
		fmt.Fprintf(&b, " %s=%.3f%s;", s.CandidateID, s.Total, tag)
	}
	b.WriteString(" Do not repeat the failed direction above; solve from a clean slate.")
	return b.String()
}
