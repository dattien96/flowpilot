package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"flowpilot-runner/internal/lsp"
	"flowpilot-runner/internal/structure"
	"flowpilot-runner/internal/tournament"
)

// Tournament execution glue (CP-65 P-3, Task-370 T-4): the two inline
// behaviors of tournament-harness.yaml. They collect metrics inside
// candidate worktrees (real suite runs; LSP/dependents via replaceable
// probes), score through tournament.TournamentArbiter, and merge/clean up
// through tournament.WorktreeManager. No provider calls here (Case-1
// agnostic); candidate provider/model selection lives in the flow config,
// provider execution in the delegate candidate nodes (P-5 E2E drives them
// with mock turns).

// SuiteResult is the parsed outcome of one `go test -json` run.
type SuiteResult struct {
	TotalTests    int
	PassedTests   int
	Failed        []string
	PassedNames   []string
	AllNames      []string
	CompileBroken bool
	Output        string
}

type goTestEvent struct {
	Action  string
	Test    string
	Package string
}

// defaultSuiteTimeout bounds a candidate suite run when the caller context
// carries no deadline.
const defaultSuiteTimeout = 120 * time.Second

// TournamentSuiteRunner runs suiteCmd in dir and parses `go test -json`
// output. Separated as a var so behavior tests can stub the suite without
// shelling out (the real path is covered by RunGoTestJSON's own test and
// the P-5 E2E, which runs suites for real per Task-372 T-1).
var TournamentSuiteRunner = RunGoTestJSON

// RunGoTestJSON executes suiteCmd (default "go test -json ./...", whitespace
// split — no shell quoting) in dir and counts per-test terminal actions.
// A nonzero exit with zero test events means the suite did not compile
// (CompileBroken); a nonzero exit WITH events is a normal red suite.
func RunGoTestJSON(ctx context.Context, dir, suiteCmd string) (SuiteResult, error) {
	var res SuiteResult
	if strings.TrimSpace(suiteCmd) == "" {
		suiteCmd = "go test -json ./..."
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSuiteTimeout)
		defer cancel()
	}
	parts := strings.Fields(suiteCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = dir
	out, exitErr := cmd.CombinedOutput()
	res.Output = string(out)
	terminal := map[string]string{} // test name -> last terminal action
	for _, line := range strings.Split(res.Output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			terminal[ev.Test] = ev.Action
		}
	}
	seen := map[string]bool{}
	for name, action := range terminal {
		if seen[name] {
			continue
		}
		seen[name] = true
		res.AllNames = append(res.AllNames, name)
		switch action {
		case "pass":
			res.PassedTests++
			res.PassedNames = append(res.PassedNames, name)
		case "fail":
			res.Failed = append(res.Failed, name)
		}
	}
	res.TotalTests = res.PassedTests + len(res.Failed)
	if exitErr != nil && len(res.AllNames) == 0 {
		res.CompileBroken = true
	}
	return res, nil
}

// TournamentLSPProbe counts compiler errors for changed paths in a
// worktree. The default consults the CP-63 server set (fail-soft: no
// servers/unsupported platform yields "") and counts formatted diagnostic
// lines — an estimate, documented as such; behavior tests and the P-5 E2E
// inject exact counts (Task-372 T-1 stub path).
var TournamentLSPProbe = func(ctx context.Context, worktree string, changed []string) int {
	out := lsp.DefaultSet().CheckFiles(ctx, worktree, changed)
	if strings.TrimSpace(out) == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(out, "\n"), "\n"))
}

// TournamentDependentsProbe counts dependents for changed paths via the
// offline structure fallback provider (git-history based, no npx needed).
// Unresolvable paths are skipped (best-effort, same posture as the
// context.produce degraded-input precedent).
var TournamentDependentsProbe = func(ctx context.Context, repoDir string, changed []string) int {
	prov := structure.New(repoDir, false)
	max := 0
	for _, p := range changed {
		sum, err := prov.Dependents(ctx, p)
		if err != nil {
			continue
		}
		if sum.Count > max {
			max = sum.Count
		}
	}
	return max
}

// ChangedPathsInWorktree lists tracked-modified plus untracked paths in a
// worktree (porcelain v1; renames resolve to the new path).
func ChangedPathsInWorktree(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tournament: git status: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		if i := strings.Index(p, " -> "); i >= 0 {
			p = strings.TrimSpace(p[i+4:])
		}
		p = strings.Trim(p, `"`)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// CollectCandidateMetrics runs the suite ONCE for real in the candidate
// worktree and assembles its CandidateResult plus the raw suite outcome
// (callers reuse suite.Failed for regression attribution instead of
// re-running). lspErrors/dependentsCount arrive pre-aggregated by the
// caller (production: the probes above; tests/E2E: injected values).
// BrokeExistingTests is left false here — the arbiter behavior sets it by
// comparing against the baseline run.
func CollectCandidateMetrics(ctx context.Context, worktreePath string, cand tournament.TournamentCandidateConfig, changed []string, suiteCmd string, lspErrors, dependents int) (tournament.CandidateResult, SuiteResult, error) {
	suite, err := TournamentSuiteRunner(ctx, worktreePath, suiteCmd)
	if err != nil {
		return tournament.CandidateResult{}, SuiteResult{}, fmt.Errorf("tournament: suite for %s: %w", cand.CandidateID, err)
	}
	return AssembleCandidateResult(cand, changed, suite, lspErrors, dependents), suite, nil
}

// AssembleCandidateResult is the pure half of CollectCandidateMetrics: pack
// one suite outcome plus aggregated probe values into a CandidateResult.
func AssembleCandidateResult(cand tournament.TournamentCandidateConfig, changed []string, suite SuiteResult, lspErrors, dependents int) tournament.CandidateResult {
	return tournament.CandidateResult{
		CandidateID:     cand.CandidateID,
		Label:           cand.CandidateID + " (" + cand.Provider + ")",
		ProviderKey:     cand.Provider,
		TotalTests:      suite.TotalTests,
		PassedTests:     suite.PassedTests,
		LSPErrors:       lspErrors,
		ChangedPaths:    changed,
		DependentsCount: dependents,
	}
}

func tournamentStringArg(v any, def string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return def
}

func tournamentAttempt(v any) int {
	if n, ok := toIntArg(v); ok && n >= 1 {
		return n
	}
	return 1
}

// parseInjectedCandidateResults accepts pre-collected results (the P-5 mock
// seam: E2E/test callers inject CandidateResults without any git activity).
// Typed values pass through; generic maps use snake_case keys.
func parseInjectedCandidateResults(v any) ([]tournament.CandidateResult, bool) {
	if out, ok := v.([]tournament.CandidateResult); ok && len(out) > 0 {
		return out, true
	}
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	var out []tournament.CandidateResult
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		id, _ := m["candidate_id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, false
		}
		total, _ := toIntArg(m["total_tests"])
		passed, _ := toIntArg(m["passed_tests"])
		lspErrors, _ := toIntArg(m["lsp_errors"])
		dependents, _ := toIntArg(m["dependents_count"])
		broke, _ := m["broke_existing_tests"].(bool)
		provider, _ := m["provider"].(string)
		var changed []string
		switch c := m["changed_paths"].(type) {
		case []string:
			changed = c
		case []any:
			for _, p := range c {
				if s, ok := p.(string); ok {
					changed = append(changed, s)
				}
			}
		}
		out = append(out, tournament.CandidateResult{
			CandidateID: id, ProviderKey: provider,
			TotalTests: total, PassedTests: passed,
			BrokeExistingTests: broke, LSPErrors: lspErrors,
			ChangedPaths: changed, DependentsCount: dependents,
		})
	}
	return out, true
}

// behaviorTournamentArbiter (CP-65 P-3) is the deterministic judge: ensure
// candidate worktrees, collect metrics (baseline worktree first, so
// regressions are attributed precisely), decide via TournamentArbiter, and
// return merge/retry/ask. Worktree hygiene per the no-orphan policy: winner
// verdict deletes losers now (winner goes to the merge behavior); every
// other verdict deletes everything (evidence travels in the payload).
func behaviorTournamentArbiter(ctx context.Context, in BehaviorInput) (BehaviorOutput, error) {
	const id = "tournament.arbiter"
	rawCfg, _ := in.RawArgs["config"].(map[string]any)
	cfg, err := tournament.ParseTournamentConfig(rawCfg)
	if err != nil {
		return BehaviorOutput{}, fmt.Errorf("%s: %w", id, err)
	}
	if strings.TrimSpace(in.WorkspaceCwd) == "" {
		return BehaviorOutput{}, fmt.Errorf("%s: missing workspace for node %q", id, in.NodeID)
	}
	attempt := tournamentAttempt(in.RawArgs["attempt"])

	// Fast path: pre-collected results (P-5 mock seam). No git touched.
	if pre, ok := parseInjectedCandidateResults(in.Payload["candidateResults"]); ok {
		return finishTournamentArbiter(pre, cfg, attempt, nil), nil
	}

	suiteCmd := tournamentStringArg(in.RawArgs["suite_cmd"], "go test -json ./...")
	base := tournamentStringArg(in.RawArgs["base_commit"], "")
	if base == "" {
		cmd := exec.Command("git", "-C", in.WorkspaceCwd, "rev-parse", "HEAD")
		raw, err := cmd.Output()
		if err != nil {
			return BehaviorOutput{}, fmt.Errorf("%s: resolve base commit: %w", id, err)
		}
		base = strings.TrimSpace(string(raw))
	}
	var mgr tournament.WorktreeManager
	// Setup reuses caller-provided worktrees (candidate turns already ran
	// inside them) and creates missing ones — it never wipes existing
	// candidate output. Freshness across attempts comes from the retry
	// verdict, which cleans everything before the next rollout.
	ids := make([]string, 0, len(cfg.Candidates)+1)
	for _, c := range cfg.Candidates {
		ids = append(ids, c.CandidateID)
	}
	ensure := func(id string) (string, error) {
		path := tournament.WorktreePath(in.WorkspaceCwd, id)
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			return path, nil
		}
		return mgr.Create(in.WorkspaceCwd, base, id)
	}

	// Baseline suite once: tests failing here are pre-existing red, never
	// regressions (attribution precision for BrokeExistingTests).
	baselinePath, err := ensure("tournament-baseline")
	if err != nil {
		return BehaviorOutput{}, fmt.Errorf("%s: baseline worktree: %w", id, err)
	}
	baselineSuite, err := TournamentSuiteRunner(ctx, baselinePath, suiteCmd)
	_ = mgr.Cleanup(in.WorkspaceCwd, []string{"tournament-baseline"})
	if err != nil {
		return BehaviorOutput{}, fmt.Errorf("%s: baseline suite: %w", id, err)
	}
	baselinePassed := map[string]bool{}
	for _, name := range baselineSuite.PassedNames {
		baselinePassed[name] = true
	}

	results := make([]tournament.CandidateResult, 0, len(cfg.Candidates))
	for _, cand := range cfg.Candidates {
		worktree, err := ensure(cand.CandidateID)
		if err != nil {
			_ = mgr.Cleanup(in.WorkspaceCwd, ids)
			return BehaviorOutput{}, fmt.Errorf("%s: worktree for %s: %w", id, cand.CandidateID, err)
		}
		changed, err := ChangedPathsInWorktree(worktree)
		if err != nil {
			_ = mgr.Cleanup(in.WorkspaceCwd, ids)
			return BehaviorOutput{}, fmt.Errorf("%s: changed paths for %s: %w", id, cand.CandidateID, err)
		}
		res, suite, err := CollectCandidateMetrics(ctx, worktree, cand, changed, suiteCmd,
			TournamentLSPProbe(ctx, worktree, changed),
			TournamentDependentsProbe(ctx, worktree, changed))
		if err != nil {
			_ = mgr.Cleanup(in.WorkspaceCwd, ids)
			return BehaviorOutput{}, err
		}
		// Regression = failed now a test that passed at baseline (reuses the
		// single suite run above — no second execution per candidate).
		for _, name := range suite.Failed {
			if baselinePassed[name] {
				res.BrokeExistingTests = true
				break
			}
		}
		results = append(results, res)
	}
	return finishTournamentArbiter(results, cfg, attempt, func(losers []string) {
		_ = mgr.Cleanup(in.WorkspaceCwd, losers)
	}), nil
}

// finishTournamentArbiter scores, picks the action, does the verdict-time
// cleanup via clean (nil = nothing to clean, mock path), and shapes the
// BehaviorOutput the flow edges route on (done/retry/escalate).
func finishTournamentArbiter(results []tournament.CandidateResult, cfg tournament.TournamentNodeConfig, attempt int, clean func([]string)) BehaviorOutput {
	var arb tournament.TournamentArbiter
	verdict := arb.Decide(results)
	action := tournament.DecideTournamentAction(verdict, cfg.AutoPick, attempt, cfg.MaxAttempts)
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.CandidateID)
	}
	losers := func() []string {
		var out []string
		for _, id := range ids {
			if id != verdict.WinnerCandidateID {
				out = append(out, id)
			}
		}
		return out
	}
	payload := map[string]any{
		"action": action, "verdict": verdict,
		"attempt": attempt, "maxAttempts": cfg.MaxAttempts,
	}
	switch action {
	case tournament.TournamentActionMerge:
		if clean != nil {
			clean(losers()) // losers now; winner goes to tournament.merge
		}
		payload["winner"] = verdict.WinnerCandidateID
		return BehaviorOutput{Status: "done", Summary: "tournament winner " + verdict.WinnerCandidateID, Payload: payload}
	case tournament.TournamentActionRetry:
		if clean != nil {
			clean(ids) // fresh ground for the fresh spawns
		}
		payload["brief"] = tournament.DistillFailureBrief(verdict, attempt)
		payload["nextAttempt"] = attempt + 1
		return BehaviorOutput{Status: "retry", Summary: "tournament tie — retrying with fresh candidates", Payload: payload}
	default:
		if clean != nil {
			clean(ids) // no-orphan: tie asks with evidence in payload, not dirs
		}
		return BehaviorOutput{Status: "escalate", Summary: "tournament needs a human: " + verdict.Reason, Payload: payload}
	}
}

// behaviorTournamentMerge (CP-65 P-3) lands the winner patch via
// WorktreeManager.MergeWinner and deletes the winner worktree at flow done.
// A conflict escalates with patch + paths in the payload (the manager
// already cleaned up — evidence in card, never a kept dir). The payload
// carries an audit-draft-shaped summary for the downstream audit consumer.
func behaviorTournamentMerge(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	const id = "tournament.merge"
	winner := tournamentStringArg(in.RawArgs["winner"], "")
	// Winner may also arrive from the arbiter payload on the same run.
	if winner == "" {
		if p, ok := in.Payload["winner"].(string); ok {
			winner = strings.TrimSpace(p)
		}
	}
	if winner == "" {
		return BehaviorOutput{}, fmt.Errorf("%s: node %q missing winner (RawArgs.winner)", id, in.NodeID)
	}
	if strings.TrimSpace(in.WorkspaceCwd) == "" {
		return BehaviorOutput{}, fmt.Errorf("%s: missing workspace for node %q", id, in.NodeID)
	}
	var mgr tournament.WorktreeManager
	if err := mgr.MergeWinner(in.WorkspaceCwd, winner); err != nil {
		var conflict *tournament.MergeConflictError
		if errors.As(err, &conflict) {
			return BehaviorOutput{
				Status:  "escalate",
				Summary: "tournament winner patch conflicts — human merge required",
				Payload: map[string]any{
					"winner": winner, "reason": conflict.Reason,
					"patch": string(conflict.Patch), "conflictPaths": conflict.ConflictPaths,
				},
			}, nil
		}
		return BehaviorOutput{}, fmt.Errorf("%s: %w", id, err)
	}
	_ = mgr.Cleanup(in.WorkspaceCwd, []string{winner}) // winner done: code is in main
	return BehaviorOutput{
		Status:  "done",
		Summary: "tournament winner " + winner + " merged",
		Payload: map[string]any{
			"winner": winner,
			"draftSummary": "tournament-harness: winner " + winner +
				" merged to the main workspace; worktrees cleaned",
		},
	}, nil
}
