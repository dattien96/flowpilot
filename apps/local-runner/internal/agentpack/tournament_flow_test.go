package agentpack

import (
	"testing"

	"flowpilot-runner/internal/tournament"
)

// CP-65 P-3 (Task-370): tournament-harness flow definition. New file — no
// pre-existing test is modified.

func findTournamentFlow(t *testing.T) FlowDefinition {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load builtin pack: %v", err)
	}
	for _, flow := range pack.Flows {
		if flow.ID == "tournament-harness" {
			return flow
		}
	}
	t.Fatal("flow tournament-harness missing from builtin pack")
	return FlowDefinition{}
}

func TestTournamentHarnessTopologyValid(t *testing.T) {
	flow := findTournamentFlow(t)
	if flow.Mode != "flow" {
		t.Fatalf("mode = %q, want flow", flow.Mode)
	}
	// AC-5: selectable from the picker, never a default.
	if len(flow.Builtin.SelectableIn) != 1 || flow.Builtin.SelectableIn[0] != "flow" {
		t.Fatalf("selectableIn = %v, want [flow]", flow.Builtin.SelectableIn)
	}
	behaviors := map[string]string{}
	lifecycles := map[string]string{}
	configs := map[string]map[string]any{}
	for _, n := range flow.Nodes {
		behaviors[n.ID] = n.Behavior
		lifecycles[n.ID] = n.Lifecycle
		if n.Config != nil {
			configs[n.ID] = n.Config
		}
	}
	wantBehaviors := map[string]string{
		"problem_scout":      "agent.delegate",
		"candidate-a":        "agent.delegate",
		"candidate-b":        "agent.delegate",
		"tournament_arbiter": "tournament.arbiter",
		"merge_and_audit":    "tournament.merge",
	}
	for id, want := range wantBehaviors {
		canonical, ok := NormalizeBehaviorID(behaviors[id])
		if !ok || canonical != want {
			t.Fatalf("node %s behavior = %q, want %q", id, behaviors[id], want)
		}
	}
	// Fresh sub-agent spawns every rollout pass (clean context per attempt).
	if lifecycles["candidate-a"] != "spawn" || lifecycles["candidate-b"] != "spawn" {
		t.Fatalf("candidate lifecycles must be spawn/spawn, got %q/%q",
			lifecycles["candidate-a"], lifecycles["candidate-b"])
	}
	hasEdge := func(from, to, when, kind string) bool {
		for _, e := range flow.Edges {
			if e.From == from && e.To == to && e.When == when && e.Kind == kind {
				return true
			}
		}
		return false
	}
	for _, e := range [][4]string{
		{"problem_scout", "parallel_rollout", "done", "forward"},
		{"parallel_rollout", "candidate-a", "done", "forward"},
		{"parallel_rollout", "candidate-b", "done", "forward"},
		{"candidate-a", "tournament_arbiter", "done", "forward"},
		{"candidate-b", "tournament_arbiter", "done", "forward"},
		{"tournament_arbiter", "merge_and_audit", "done", "forward"},
		{"tournament_arbiter", "parallel_rollout", "retry", "back"},
		{"tournament_arbiter", "ask_user", "escalate", "forward"},
		{"merge_and_audit", "done", "done", "forward"},
		{"merge_and_audit", "ask_user", "escalate", "forward"},
	} {
		if !hasEdge(e[0], e[1], e[2], e[3]) {
			t.Fatalf("missing edge %s -> %s (%s/%s)", e[0], e[1], e[2], e[3])
		}
	}
	// Exactly one back-edge (the retry leg).
	backEdges := 0
	for _, e := range flow.Edges {
		if e.Kind == "back" {
			backEdges++
		}
	}
	if backEdges != 1 {
		t.Fatalf("back-edge count = %d, want 1 (arbiter retry only)", backEdges)
	}
	// No agent.code writer: candidates are sandboxed delegates, the only
	// main-workspace write is the arbiter-approved patch (merge behavior).
	for _, n := range flow.Nodes {
		if n.Behavior == "agent.code" {
			t.Fatalf("node %s must not be agent.code (tournament writes stay in worktrees)", n.ID)
		}
	}
	if err := ValidateFlowSafetyTopology(flow); err != nil {
		t.Fatalf("safety topology: %v", err)
	}
	// Arbiter node carries the Q-1 auto_pick + max_attempts config.
	arbiterCfg, ok := configs["tournament_arbiter"]
	if !ok {
		t.Fatal("tournament_arbiter node must declare config")
	}
	parsed, err := tournament.ParseTournamentConfig(arbiterCfg)
	if err != nil {
		t.Fatalf("arbiter config must parse: %v", err)
	}
	if !parsed.AutoPick || parsed.MaxAttempts != 2 {
		t.Fatalf("arbiter config = auto_pick:%v max_attempts:%d, want true/2", parsed.AutoPick, parsed.MaxAttempts)
	}
}

func TestTournamentHarnessParsesCandidateConfigs(t *testing.T) {
	flow := findTournamentFlow(t)
	var rollout *FlowNode
	for i := range flow.Nodes {
		if flow.Nodes[i].ID == "parallel_rollout" {
			rollout = &flow.Nodes[i]
		}
	}
	if rollout == nil || rollout.Config == nil {
		t.Fatal("parallel_rollout node must declare config")
	}
	// AC-2: default rollout is the R-1 pair (Claude + Codex).
	cfg, err := tournament.ParseTournamentConfig(rollout.Config)
	if err != nil {
		t.Fatalf("rollout config must parse: %v", err)
	}
	if len(cfg.Candidates) != 2 || cfg.Candidates[0].Provider != "claude" || cfg.Candidates[1].Provider != "codex" {
		t.Fatalf("default rollout must be Claude+Codex, got %+v", cfg.Candidates)
	}
	if cfg.Serial {
		t.Fatal("default rollout must be parallel (serial=false)")
	}
	// AC-2: a 3-candidate declaration parses (Grok opt-in).
	three, err := tournament.ParseTournamentConfig(map[string]any{
		"candidates": []any{
			map[string]any{"candidate_id": "a", "provider": "claude", "model": "claude-sonnet"},
			map[string]any{"candidate_id": "b", "provider": "codex", "model": "gpt-5.4-mini"},
			map[string]any{"candidate_id": "c", "provider": "grok", "model": "grok-4.5"},
		},
		"serial": true,
	})
	if err != nil {
		t.Fatalf("3-candidate config must parse: %v", err)
	}
	if len(three.Candidates) != 3 || !three.Serial {
		t.Fatalf("3-candidate config must round-trip, got %+v", three)
	}
}

func TestTournamentSecondAttemptUsesDistilledFailure(t *testing.T) {
	// AC-7: tie on attempt 1 of 2 with auto_pick on retries with a brief.
	var arb tournament.TournamentArbiter
	verdict := arb.Decide([]tournament.CandidateResult{
		{CandidateID: "candidate-a", TotalTests: 10, PassedTests: 8},
		{CandidateID: "candidate-b", TotalTests: 10, PassedTests: 8},
	})
	if !verdict.NeedsHumanDecision {
		t.Fatal("equal candidates must tie")
	}
	if got := tournament.DecideTournamentAction(verdict, true, 1, 2); got != tournament.TournamentActionRetry {
		t.Fatalf("action = %q, want retry", got)
	}
	brief := tournament.DistillFailureBrief(verdict, 1)
	if brief == "" {
		t.Fatal("retry must carry a distilled failure brief for the fresh spawns")
	}
	// The retry back-edge exists in topology (pinned by TopologyValid).
	flow := findTournamentFlow(t)
	found := false
	for _, e := range flow.Edges {
		if e.From == "tournament_arbiter" && e.To == "parallel_rollout" && e.When == "retry" && e.Kind == "back" {
			found = true
		}
	}
	if !found {
		t.Fatal("retry back-edge missing from topology")
	}
}

func TestTournamentNoThirdAttempt(t *testing.T) {
	// AC-7: attempt 2 of 2 (or auto_pick off) escalates — never a 3rd round.
	var arb tournament.TournamentArbiter
	verdict := arb.Decide([]tournament.CandidateResult{
		{CandidateID: "candidate-a", TotalTests: 10, PassedTests: 8},
		{CandidateID: "candidate-b", TotalTests: 10, PassedTests: 8},
	})
	if got := tournament.DecideTournamentAction(verdict, true, 2, 2); got != tournament.TournamentActionAsk {
		t.Fatalf("exhausted attempts must ask, got %q", got)
	}
	if got := tournament.DecideTournamentAction(verdict, false, 1, 2); got != tournament.TournamentActionAsk {
		t.Fatalf("auto_pick=false must ask immediately, got %q", got)
	}
	// A merge conflict is not a verdict at all: MergeConflictError carries
	// no retryable signal, so the merge behavior escalates directly.
	if tournament.TournamentActionMerge == tournament.TournamentActionRetry {
		t.Fatal("merge/retry actions must stay distinct (conflict never retries)")
	}
}
