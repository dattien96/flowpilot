package runner

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/tournament"
)

// BUG-426 / CP-65 P-5: engine dispatch for the tournament-harness flow nodes.
// The flow YAML declares three engine-owned surfaces the pre-P-5 executor
// could not drive: parallel_rollout (a behavior-less inline marker that fans
// out to the candidate cohort), tournament_arbiter, and merge_and_audit
// (tournament.merge). Without these cases the escalation child spawned its
// entry scout and then stalled forever — no node could advance.

// tournamentRolloutPassthrough handles an inline marker node with no behavior
// (tournament-harness's parallel_rollout): the node itself does nothing — its
// declared forward "done" edges are the real fan-out. Returns the resolved
// target node list so the caller can spawn them through the normal delegate
// path. Returns ok=false when the node is not a passthrough marker.
func tournamentRolloutPassthrough(node agentpack.FlowNode, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode) ([]agentpack.FlowNode, bool) {
	if strings.TrimSpace(node.Behavior) != "" || !strings.EqualFold(strings.TrimSpace(node.Run), "inline") {
		return nil, false
	}
	targets := forwardDoneTargets(edges, node.ID)
	if len(targets) == 0 {
		return nil, false
	}
	out := make([]agentpack.FlowNode, 0, len(targets))
	for _, id := range targets {
		t, ok := findFlowNode(nodes, id)
		if !ok {
			return nil, false
		}
		canonical, cok := agentpack.NormalizeBehaviorID(t.Behavior)
		if !cok || canonical != "agent.delegate" {
			return nil, false
		}
		out = append(out, t)
	}
	return out, true
}

// tournamentCandidateWorktree binds a tournament candidate node (cohort:
// tournament) to its isolated worktree, creating it lazily when the rollout
// spawns the candidate for the first time. The base commit is the flow's
// captured start HEAD so every candidate diffs against the same baseline.
func (s *InteractiveService) tournamentCandidateWorktree(parentRunID string, node agentpack.FlowNode) string {
	if !strings.EqualFold(strings.TrimSpace(node.Cohort), "tournament") {
		return ""
	}
	cwd := s.workspaceCwdFor(parentRunID)
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	s.mu.Lock()
	base := ""
	if rs := s.runs[parentRunID]; rs != nil {
		base = strings.TrimSpace(rs.flowStartGitHead)
	}
	s.mu.Unlock()
	if base == "" {
		// A run resumed without a captured flow-start HEAD still needs a
		// resolvable base — fall back to the workspace's current HEAD.
		if out, err := exec.Command("git", "-C", cwd, "rev-parse", "HEAD").Output(); err == nil {
			base = strings.TrimSpace(string(out))
		}
	}
	var mgr tournament.WorktreeManager
	path, err := mgr.Create(cwd, base, node.ID)
	if err != nil {
		log.Printf("[tournament] worktree create for %q failed: %v — candidate runs in main workspace", node.ID, err)
		return ""
	}
	return path
}

// tournamentJoinSatisfied enforces the arbiter node's declared join:all —
// every forward "done" predecessor must have a completed child run before
// the arbiter dispatches. A completion that arrives while a sibling is still
// running returns false (claimed as handled; the sibling's own completion
// re-enters this path).
func (s *InteractiveService) tournamentJoinSatisfied(parentRunID string, edges []agentpack.FlowEdge, nodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range edges {
		if !strings.EqualFold(strings.TrimSpace(e.To), nodeID) ||
			!strings.EqualFold(strings.TrimSpace(e.When), "done") ||
			!strings.EqualFold(strings.TrimSpace(e.Kind), "forward") {
			continue
		}
		// join:all collects every terminal outcome — a failed/cancelled
		// candidate is still a finished member; the arbiter scores its
		// (empty) worktree rather than vetoing the whole cohort. Live
		// run-1890: candidate-a's provider failure left this gate false
		// forever, silently discarding candidate-b's patch.
		done := false
		for _, c := range s.runs {
			if c.parentRunID == parentRunID && c.label == e.From && worktreeTerminal(c.status) {
				done = true
				break
			}
		}
		if !done {
			log.Printf("[tournament] arbiter join waiting on sibling %q (run %q)", e.From, parentRunID)
			return false
		}
	}
	return true
}

// tournamentNodeConfig merges the rollout node's candidates/serial config
// with the arbiter node's auto_pick/max_attempts so ParseTournamentConfig
// sees the flow's full declared shape regardless of which node carried what.
func tournamentNodeConfig(nodes []agentpack.FlowNode, arbiter agentpack.FlowNode) map[string]any {
	merged := map[string]any{}
	for k, v := range arbiter.Config {
		merged[k] = v
	}
	for _, n := range nodes {
		if n.ID == arbiter.ID || n.Config == nil {
			continue
		}
		if _, ok := n.Config["candidates"]; ok {
			if _, have := merged["candidates"]; !have {
				merged["candidates"] = n.Config["candidates"]
			}
			if _, have := merged["serial"]; !have {
				if s, ok2 := n.Config["serial"]; ok2 {
					merged["serial"] = s
				}
			}
		}
	}
	return merged
}

// runTournamentArbiterNode dispatches the tournament_arbiter inline node:
// collect candidate metrics, decide, then route the verdict — merge
// (forward done), retry (declared back-edge to the rollout), or ask
// (escalate with the ranking decision card).
func (s *InteractiveService) runTournamentArbiterNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	if s.flowRunTerminalLocked(parentRunID) {
		return true
	}
	if !s.loopIsAdvancing(parentRunID) {
		return false
	}
	// join:all — a single candidate finishing early must not run the arbiter
	// on a half-collected field.
	if !s.tournamentJoinSatisfied(parentRunID, edges, node.ID) {
		return true
	}
	escalate := func(reason string) bool {
		s.flowDiagLog(parentRunID, "tournament_arbiter_blocked", reason, "node_id", node.ID)
		if s.isFlowEngineDriven(parentRunID) {
			s.stampLastEscalatedInlineNode(parentRunID, node.ID)
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "tournament arbiter blocked: " + reason,
		}); err != nil {
			log.Printf("[tournament] arbiter escalate failed: %v", err)
		}
		return true
	}

	s.mu.Lock()
	attempt := 0
	base := ""
	if rs := s.runs[parentRunID]; rs != nil {
		attempt = rs.tournamentAttempt
		base = strings.TrimSpace(rs.flowStartGitHead)
	}
	s.mu.Unlock()
	out, err := behaviorTournamentArbiter(ctx, BehaviorInput{
		NodeID:       node.ID,
		WorkspaceCwd: s.workspaceCwdFor(parentRunID),
		RawArgs: map[string]any{
			"config":      tournamentNodeConfig(nodes, node),
			"attempt":     attempt,
			"base_commit": base,
		},
	})
	if err != nil {
		return escalate(err.Error())
	}
	s.flowDiagLog(parentRunID, "tournament_arbiter_decided", "tournament arbiter verdict routed",
		"node_id", node.ID, "status", out.Status,
		"winner", fmt.Sprint(out.Payload["winner"]))
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
	}

	// Stash the per-candidate patch snapshots for EVERY verdict, not just
	// escalate: BUG-459 (live run-20041) — an auto-picked winner whose diff is
	// empty reached merge with an absent patch key, took the live-worktree
	// path, and reported "merged" on a no-op instead of the BUG-453 explicit
	// empty-patch escalate. Snapshots are per-round; a retry's next arbiter
	// verdict overwrites them.
	if patches, ok := out.Payload["patches"].(map[string]string); ok {
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.tournamentPatches = patches
		}
		s.mu.Unlock()
	}
	switch strings.TrimSpace(out.Status) {
	case "done":
		// Winner picked — hand it to the merge node via the run record and
		// advance through the declared forward done edge.
		winner, _ := out.Payload["winner"].(string)
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.tournamentWinner = strings.TrimSpace(winner)
		}
		s.mu.Unlock()
		targetID, ok := edgeTargetFrom(edges, node.ID, "done", "forward")
		if !ok {
			return escalate("arbiter done but no forward done edge to merge")
		}
		next, ok := findFlowNode(nodes, targetID)
		if !ok {
			return escalate("merge target " + targetID + " missing from flow nodes")
		}
		return s.tryAdvanceFlowThroughInline(parentRunID, edges, nodes, next, resultMessage)
	case "retry":
		return s.retryTournamentRollout(parentRunID, edges, nodes, node, out)
	default:
		// escalate/ask — park with the ranking decision card (BUG-414: the
		// card's options are the candidate ids plus retry/ask so a captured
		// choice can route back into merge or a fresh rollout).
		card := tournamentDecisionCard(out)
		if s.isFlowEngineDriven(parentRunID) {
			s.stampLastEscalatedInlineNode(parentRunID, node.ID)
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: out.Summary,
			Payload: map[string]any{"decision_card": card},
		}); err != nil {
			log.Printf("[tournament] arbiter ask escalate failed: %v", err)
		}
		return true
	}
}

// retryTournamentRollout consumes the arbiter's retry verdict: bump the
// attempt counter, re-open the rollout node, and spawn a fresh candidate
// cohort carrying the distilled failure brief.
func (s *InteractiveService) retryTournamentRollout(parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, arbiter agentpack.FlowNode, out BehaviorOutput) bool {
	targetID, ok := edgeTargetFrom(edges, arbiter.ID, "retry", "back")
	if !ok {
		return false
	}
	rollout, ok := findFlowNode(nodes, targetID)
	if !ok {
		return false
	}
	candidates, ok := tournamentRolloutPassthrough(rollout, edges, nodes)
	if !ok {
		return false
	}
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.tournamentAttempt++
	}
	s.mu.Unlock()
	brief, _ := out.Payload["brief"].(string)
	s.spawnTournamentCandidates(parentRunID, rollout, candidates, brief)
	return true
}

// spawnTournamentCandidates fans the rollout node's candidate nodes out as
// delegate children in one cohort; each cohort:tournament member is bound to
// its isolated worktree.
func (s *InteractiveService) spawnTournamentCandidates(parentRunID string, rollout agentpack.FlowNode, candidates []agentpack.FlowNode, brief string) {
	cwd := s.workspaceCwdFor(parentRunID)
	round := s.agentOrchestrator.loopStateFor(parentRunID).Round
	cohortID := fmt.Sprintf("flow-auto-%s-round-%d", rollout.ID, round)
	for i, node := range candidates {
		if !s.loopIsAdvancing(parentRunID) {
			break
		}
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			continue
		}
		prompt := buildFlowReviewHandoffPrompt(rollout.ID, brief, node)
		prompt = composeFlowNodeAgentPrompt(cwd, prompt, node)
		prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           prompt,
			Wait:             false,
			Label:            node.ID,
			FlowCohortID:     cohortID,
			CohortSize:       len(candidates),
			AutoOrchestrate:  i == 0,
			AgentDefOverride: agentDef,
			Model:            s.delegateSpawnModel(context.Background(), parentRunID, node),
			WorkspaceCwd:     s.tournamentCandidateWorktree(parentRunID, node),
		}); err != nil {
			log.Printf("[tournament] candidate spawn %q failed: %v", node.ID, err)
			s.flowDiagLog(parentRunID, "tournament_candidate_spawn_failed",
				"candidate spawn failed", "node_id", node.ID, "error", err.Error())
			continue
		}
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(context.Background(), parentRunID, node.ID, StepStatusRunning)
			s.stampFlowNodePosture(context.Background(), parentRunID, node)
		}
	}
}

// runTournamentMergeNode dispatches merge_and_audit: lands the winner patch
// in the main workspace, then advances the declared forward edge — "done"
// terminalizes the flow and (for an escalation child) resumes the parent.
func (s *InteractiveService) runTournamentMergeNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	if s.flowRunTerminalLocked(parentRunID) {
		return true
	}
	s.mu.Lock()
	winner := ""
	winnerPatch, patchRecorded := "", false
	if rs := s.runs[parentRunID]; rs != nil {
		winner = strings.TrimSpace(rs.tournamentWinner)
		// BUG-453: convey snapshot presence — an absent key keeps merge on
		// the live-worktree path, a present-but-empty value means the picked
		// candidate recorded no mergeable diff.
		winnerPatch, patchRecorded = rs.tournamentPatches[winner]
	}
	s.mu.Unlock()
	rawArgs := map[string]any{"winner": winner}
	if patchRecorded {
		rawArgs["patch"] = winnerPatch
	}
	out, err := behaviorTournamentMerge(ctx, BehaviorInput{
		NodeID:       node.ID,
		WorkspaceCwd: s.workspaceCwdFor(parentRunID),
		RawArgs:      rawArgs,
	})
	if err != nil {
		s.flowDiagLog(parentRunID, "tournament_merge_failed", "merge behavior errored",
			"node_id", node.ID, "error", err.Error())
		if s.isFlowEngineDriven(parentRunID) {
			s.stampLastEscalatedInlineNode(parentRunID, node.ID)
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		}
		if _, aerr := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "tournament merge failed: " + err.Error(),
		}); aerr != nil {
			log.Printf("[tournament] merge escalate failed: %v", aerr)
		}
		return true
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
	}
	if strings.TrimSpace(out.Status) == "done" {
		// Sweep loser worktrees. The arbiter-merge path already cleaned them
		// at verdict time (idempotent no-op here); the human-pick path
		// (escalate keeps all candidates mergeable) needs it here.
		var mgr tournament.WorktreeManager
		var losers []string
		for _, n := range nodes {
			if strings.EqualFold(strings.TrimSpace(n.Cohort), "tournament") && n.ID != winner {
				losers = append(losers, n.ID)
			}
		}
		_ = mgr.Cleanup(s.workspaceCwdFor(parentRunID), losers)
		s.finishTournamentRun(parentRunID, winner, out.Summary)
		return true
	}
	// Conflict/escalate — park with the patch evidence.
	if s.isFlowEngineDriven(parentRunID) {
		s.stampLastEscalatedInlineNode(parentRunID, node.ID)
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
	}
	if _, aerr := s.applyFlowControl(parentRunID, FlowControlInput{
		Status:  "escalate",
		Summary: out.Summary,
		Payload: map[string]any{"tournament_merge": out.Payload},
	}); aerr != nil {
		log.Printf("[tournament] merge-conflict escalate failed: %v", aerr)
	}
	return true
}

// finishTournamentRun settles the tournament run's own loop done and — when
// this run is an escalation child — resumes its parent's parked loop through
// resumeParentAfterTournament.
func (s *InteractiveService) finishTournamentRun(runID, winner, summary string) {
	if _, err := s.applyFlowControl(runID, FlowControlInput{
		Status:  "done",
		Summary: summary,
	}); err != nil {
		log.Printf("[tournament] done settle failed for %q: %v", runID, err)
	}
	s.mu.Lock()
	rs := s.runs[runID]
	parentID, isEscalationChild := "", false
	if rs != nil {
		parentID = rs.parentRunID
		isEscalationChild = rs.parentRunID != "" && rs.label == "tournament_escalation"
	}
	s.mu.Unlock()
	if isEscalationChild {
		if err := s.resumeParentAfterTournament(parentID, winner, true); err != nil {
			log.Printf("[tournament] resume parent %q failed: %v", parentID, err)
		}
	}
}

// tournamentDecisionCard renders the arbiter's escalate payload as a
// request_user_decision card map (consumed by applyFlowControl's
// decision_card branch). Options: every ranked candidate plus retry and ask.
func tournamentDecisionCard(out BehaviorOutput) map[string]any {
	// options must be []any: parseUserDecisionCard asserts that exact type
	// ([]map[string]any fails the assertion → decision_card_invalid and the
	// prose fallback parks instead — live run-2500).
	options := []any{}
	if verdict, ok := out.Payload["verdict"].(tournament.TournamentVerdict); ok {
		for _, r := range verdict.Ranking {
			options = append(options, map[string]any{
				"id":          r.CandidateID,
				"label":       fmt.Sprintf("%s (score %.1f)", r.CandidateID, r.Total),
				"consequence": "merge this candidate's patch into the workspace",
			})
		}
	}
	options = append(options,
		map[string]any{"id": "retry", "label": "Retry the tournament", "consequence": "spawn a fresh candidate round with the distilled failure brief"},
		map[string]any{"id": "ask", "label": "Keep asking me", "consequence": "stay parked for free-form guidance"},
	)
	return map[string]any{
		"kind":     DecisionCardKindTournament,
		"question": "Tournament needs a decision — pick a winner, retry, or keep asking",
		"detail":   out.Summary,
		"options":  options,
	}
}

// flowSharedInlineJoinTarget resolves the single inline node that every
// completed cohort member's forward "done" edge feeds into — the declared
// join:all target (tournament candidates -> tournament_arbiter). Returns
// ok=false when members disagree on targets or the target is not inline-
// dispatchable, so the caller keeps the hub-reinvoke fallback.
//
// Member outcome does not matter: join:all is a terminal barrier (a failed
// or cancelled member still counts), and the joined note already carries the
// failure. Routing around the declared target on a partial cohort would
// silently skip it — the live run-2 shape, where candidate-a's provider
// failure sent the cohort to a hub verdict that marked tournament_arbiter
// and merge_and_audit SKIPPED and dropped candidate-b's winning patch
// unmerged. The inline node (arbiter) reads candidate worktrees itself and
// scores an empty/absent one deterministically.
func flowSharedInlineJoinTarget(nodes []agentpack.FlowNode, edges []agentpack.FlowEdge, entries []cohortEntry) (agentpack.FlowNode, bool) {
	shared := ""
	for _, e := range entries {
		if strings.TrimSpace(e.Label) == "" {
			return agentpack.FlowNode{}, false
		}
		targets := forwardDoneTargets(edges, e.Label)
		if len(targets) != 1 {
			return agentpack.FlowNode{}, false
		}
		if shared == "" {
			shared = targets[0]
		} else if shared != targets[0] {
			return agentpack.FlowNode{}, false
		}
	}
	if shared == "" {
		return agentpack.FlowNode{}, false
	}
	node, ok := findFlowNode(nodes, shared)
	if !ok || !flowNodeInlineDispatchable(node) {
		return agentpack.FlowNode{}, false
	}
	// hub.inline/hub.notify targets keep the pre-existing hub reinvoke path —
	// dispatchHubNotifyNode does not drain the joined note into a synthesis
	// turn, so rerouting review cohorts through it would silently change the
	// review-loop contract.
	if c, cok := agentpack.NormalizeBehaviorID(node.Behavior); cok && (c == "hub.inline" || c == "hub.notify") {
		return agentpack.FlowNode{}, false
	}
	return node, true
}

// resumeTournamentChoice consumes a captured tournament decision-card choice
// (BUG-414). candidate ids route into the merge node, "retry" re-rolls the
// cohort, "ask" stays parked for prose follow-up, anything else is rejected
// so the answer is never accepted-then-dropped.
func (s *InteractiveService) resumeTournamentChoice(runID, chosen, feedback string) (AgentGraphSnapshot, error) {
	chosen = strings.TrimSpace(chosen)
	if chosen == "" {
		return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: decision card has no captured choice")
	}
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: run %q not found", runID)
	}
	nodes := rs.activeFlowNodes
	edges := rs.activeFlowEdges
	rs.decisionCard = nil
	rs.decisionCardChosen = ""
	s.mu.Unlock()

	isCandidate := false
	if rs != nil {
		for _, n := range nodes {
			if n.ID == chosen && strings.EqualFold(strings.TrimSpace(n.Cohort), "tournament") {
				isCandidate = true
				break
			}
		}
	}

	switch {
	case isCandidate:
		// Winner chosen — dispatch merge directly with the picked candidate.
		s.mu.Lock()
		if r := s.runs[runID]; r != nil {
			r.tournamentWinner = chosen
		}
		s.mu.Unlock()
		s.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = "tournament winner chosen: " + chosen
			return st
		})
		mergeNode, ok := findFlowNode(nodes, "merge_and_audit")
		if !ok {
			for _, n := range nodes {
				if c, cok := agentpack.NormalizeBehaviorID(n.Behavior); cok && c == "tournament.merge" {
					mergeNode, ok = n, true
					break
				}
			}
		}
		if !ok {
			return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: no merge node in flow topology")
		}
		s.runTournamentMergeNode(context.Background(), runID, edges, nodes, mergeNode, feedback)
		return s.agentGraphSnapshot(runID), nil
	case chosen == "retry":
		s.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = "tournament retry requested"
			return st
		})
		var rollout agentpack.FlowNode
		found := false
		for _, n := range nodes {
			if strings.TrimSpace(n.Behavior) == "" && strings.EqualFold(strings.TrimSpace(n.Run), "inline") {
				rollout, found = n, true
				break
			}
		}
		if !found {
			return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: no rollout node for retry")
		}
		candidates, ok := tournamentRolloutPassthrough(rollout, edges, nodes)
		if !ok {
			return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: rollout node has no candidate targets")
		}
		s.mu.Lock()
		if r := s.runs[runID]; r != nil {
			r.tournamentAttempt++
		}
		s.mu.Unlock()
		// The escalate park keeps candidate worktrees alive for a human pick;
		// a retry must clean that stale ground first or Create fails
		// "already exists" and the fresh round runs in the main workspace.
		var mgr tournament.WorktreeManager
		ids := make([]string, 0, len(candidates))
		for _, c := range candidates {
			ids = append(ids, c.ID)
		}
		_ = mgr.Cleanup(s.workspaceCwdFor(runID), ids)
		go s.spawnTournamentCandidates(runID, rollout, candidates, feedback)
		return s.agentGraphSnapshot(runID), nil
	case chosen == "ask":
		// Stay parked — the human wants to talk, not to resume the loop.
		return s.agentGraphSnapshot(runID), nil
	default:
		return s.agentGraphSnapshot(runID), fmt.Errorf("tournament: unhandled decision choice %q", chosen)
	}
}
