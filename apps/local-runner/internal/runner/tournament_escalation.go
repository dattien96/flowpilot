package runner

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Tournament escalation fallback leg (CP-65 P-4, Task-371): when a review
// loop hits its round cap or a vibe owner debate stalls past its retries,
// the runner opens a tournament-harness child run with the stuck run's
// intent and contract instead of terminating failed/stopped. Everything here
// is flag-gated (FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION, default OFF): with
// the flag unset every hook below is a no-op and the old escalate-card /
// parked-cap paths run byte-identical.
//
// The tournament child is linked via parentRunID but deliberately NOT
// registered in the orchestrator children map: that map drives
// park/freeze sweeps (parkFlowForAwaitingUser wipes pending prompts of
// listed children), and the rescue intent must survive the parent's park.
// The trade-off is documented: UI completion plumbing that keys off the
// children map will not see the tournament child; s.runs linkage carries
// the parent/child relation for verdict return (T-3).

// TournamentEscalationEnv is the CP-65 §8 rollout/fallback flag. Unset or
// false keeps the pre-CP-65 behavior: caps park escalate cards, stalls park
// member_stalled, debate failures park caps — no tournament is ever opened.
const TournamentEscalationEnv = "FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION"

// tournamentHarnessFlowID is the pack flow id opened for a rescue.
const tournamentHarnessFlowID = "tournament-harness"

// errTournamentEscalationExists is returned by escalateToTournament when the
// in-lock authoritative dedup finds a tournament_escalation child already
// registered (BUG-446). Callers treat it as "rescue already in flight", not
// a failure — the legacy park path must NOT run over a live tournament.
var errTournamentEscalationExists = errors.New("tournament escalation child already exists")

// TournamentEscalationEnabled reports whether the escalation leg is on.
// Only the explicit truthy set enables it (same pattern as
// ReproduceGateEnabled); unset keeps legacy behavior.
func TournamentEscalationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(TournamentEscalationEnv))) {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true
	default:
		return false
	}
}

// runIsTournamentFlow reports whether runID already runs tournament-harness,
// data-driven (never flow-id string matching at call sites): a tournament
// behavior in the live topology, or the tournament flow ref on the run.
// Callers must NOT hold s.mu (activeFlowNodesFor locks internally).
func (s *InteractiveService) runIsTournamentFlow(runID string) bool {
	if s == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	for _, n := range s.activeFlowNodesFor(runID) {
		if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok &&
			(canonical == string(BehaviorTournamentArbiter) || canonical == string(BehaviorTournamentMerge)) {
			return true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return false
	}
	for _, ref := range []string{rs.workflowID, rs.chatFlowRef} {
		if workingmode.BareFlowID(ref) == tournamentHarnessFlowID {
			return true
		}
	}
	return false
}

// shouldEscalateToTournament is the single choke point for every rescue
// trigger: flag on, parent known, and not already a tournament run
// (anti-recursion — a tournament of a tournament is forbidden).
func (s *InteractiveService) shouldEscalateToTournament(parentRunID string) bool {
	if !TournamentEscalationEnabled() || s == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	s.mu.Lock()
	exists := s.runs[parentRunID] != nil
	s.mu.Unlock()
	return exists && !s.runIsTournamentFlow(parentRunID)
}

// escalateToTournament dispatches a tournament-harness child run inheriting
// the stuck run's workspace, provider/model and contract (the frozen
// contract lives in the shared workspace store, so sharing the workspace
// IS the contract handoff — no FrozenRecord parameter needed), records the
// distilled intent on the child, and flips the parent loop to
// tournament_escalation (never failed/stopped). The child's flow topology is
// attached record-level; its entry delegates dispatch on its first turn
// through the normal engine path (no provider calls happen here, so this is
// unit-testable). Returns the child run id.
func (s *InteractiveService) escalateToTournament(parentRunID, reason string) (string, error) {
	if !s.shouldEscalateToTournament(parentRunID) {
		return "", fmt.Errorf("tournament: escalation refused for run %q (flag off, unknown run, or already a tournament)", parentRunID)
	}
	def, err := tournamentFlowDefinition()
	if err != nil {
		return "", err
	}
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("tournament: parent run %q vanished", parentRunID)
	}
	// BUG-446: the authoritative dedup lives INSIDE the insertion lock — the
	// outer scan in maybeEscalateCapToTournament is a cheap pre-check only.
	// Two triggers that both pass the pre-check must not allocate
	// -tournament and -tournament-2 and dispatch competing rescues.
	for _, other := range s.runs {
		if other.parentRunID == parentRunID && other.label == "tournament_escalation" {
			s.mu.Unlock()
			return "", fmt.Errorf("%w: run %q already has child %q", errTournamentEscalationExists, parentRunID, other.id)
		}
	}
	intent := fmt.Sprintf("Tournament escalation of run %s (%s). Prior loop: %s (round %d/%d). The prior direction failed — solve from a clean slate, do not repeat it.",
		parentRunID, strings.TrimSpace(reason), strings.TrimSpace(loop.GateReason), loop.Round, effectiveCap(loop))
	base := parentRunID + "-tournament"
	childID := base
	for n := 2; s.runs[childID] != nil; n++ {
		childID = fmt.Sprintf("%s-%d", base, n)
	}
	// BUG-412: the child must carry the same durable provider identity a
	// createRun child gets — a synthetic thread-* session id plus the active
	// provider account — or its first/resume turn rejects 409
	// provider_account_changed / session_unavailable. The account read can
	// hit disk, so resolve it before taking s.mu.
	accountID := strings.TrimSpace(parent.providerAccountID)
	if accountID == "" {
		accountID = s.activeAccountForProvider(parent.providerKey)
	}
	child := &interactiveRun{
		id:                childID,
		parentRunID:       parentRunID,
		projectID:         parent.projectID,
		workspaceCwd:      parent.workspaceCwd,
		providerKey:       parent.providerKey,
		modelName:         parent.modelName,
		label:             "tournament_escalation",
		workflowID:        workingmode.PackPrefix + tournamentHarnessFlowID,
		chatFlowRef:       workingmode.PackPrefix + tournamentHarnessFlowID,
		pendingTurnPrompt: intent,
		flowEngineDriven:  true,
		runKind:           parent.runKind,
		status:            RunStatusIdle,
		stepID:            tournamentHarnessFlowID,
		providerSessionID: s.nextID("thread"),
		providerAccountID: accountID,
		activeFlowNodes:   append([]agentpack.FlowNode(nil), def.Nodes...),
		activeFlowEdges:   append([]agentpack.FlowEdge(nil), def.Edges...),
	}
	s.runs[childID] = child
	s.mu.Unlock()

	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = LoopStatusTournamentEscalation
		st.GateReason = fmt.Sprintf("tournament escalation: %s dispatched (%s)", childID, strings.TrimSpace(reason))
		return st
	})
	s.flowDiagLog(parentRunID, "tournament_escalated", "review/debate deadlock rescued into a tournament",
		"child_run_id", childID, "reason", reason,
	)
	s.flowDiagLog(childID, "tournament_child_dispatched", "tournament-harness child ready for its first turn",
		"parent_run_id", parentRunID, "flow_ref", workingmode.PackPrefix+tournamentHarnessFlowID,
	)
	// BUG-412: kick the child's first turn so the engine actually starts the
	// harness (problem_scout entry) — previously the child sat idle with only
	// pendingTurnPrompt recorded and never ran. Async: startTurn takes s.mu and
	// the caller may still hold loop/dispatch state.
	go func() {
		if _, aerr := s.startTurn(childID, TurnInput{
			StepID:  tournamentHarnessFlowID,
			FlowRef: workingmode.PackPrefix + tournamentHarnessFlowID,
			Prompt:  intent,
		}, "tournament", ""); aerr != nil {
			log.Printf("[tournament] child %q first turn rejected: %s", childID, aerr.msg)
			s.flowDiagLog(parentRunID, "tournament_child_start_failed", "tournament child first turn rejected",
				"child_run_id", childID, "error", aerr.msg,
			)
		}
	}()
	return childID, nil
}

// tournamentFlowDefinition loads the tournament-harness definition from the
// builtin pack (pure data — no dispatch, no spawn).
func tournamentFlowDefinition() (agentpack.FlowDefinition, error) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return agentpack.FlowDefinition{}, fmt.Errorf("tournament: load builtin pack: %w", err)
	}
	for _, flow := range pack.Flows {
		if flow.ID == tournamentHarnessFlowID {
			return flow, nil
		}
	}
	return agentpack.FlowDefinition{}, fmt.Errorf("tournament: flow %q missing from builtin pack", tournamentHarnessFlowID)
}

// maybeEscalateCapToTournament is the shared hook body for the three rescue
// triggers (review round-cap, cohort stall sweep, debate owner-fail cap).
// It only rescues stuck loops (blocked) and never re-rescues: a parent
// already in tournament_escalation keeps its single child. Returns true
// when a tournament was dispatched (callers skip their legacy park path).
func (s *InteractiveService) maybeEscalateCapToTournament(parentRunID, reason string) bool {
	if !s.shouldEscalateToTournament(parentRunID) {
		return false
	}
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	if loop.Status == LoopStatusTournamentEscalation {
		return false
	}
	if loop.Status != "blocked" {
		return false
	}
	// BUG-413: belt-and-suspenders — any caller that already mutated the loop
	// status (e.g. a continue that ran Round++ before this check) would see
	// "blocked" and re-dispatch. Refuse while a tournament child exists,
	// regardless of the parent's current loop status.
	s.mu.Lock()
	for _, rs := range s.runs {
		if rs.parentRunID == parentRunID && rs.label == "tournament_escalation" {
			s.mu.Unlock()
			s.flowDiagLog(parentRunID, "tournament_escalation_deduped",
				"tournament escalation refused: a tournament child already exists")
			return false
		}
	}
	s.mu.Unlock()
	childID, err := s.escalateToTournament(parentRunID, reason)
	if err != nil {
		// BUG-446: a concurrent trigger won the in-lock claim — the rescue is
		// already dispatched, so report handled; the caller must not run its
		// legacy park path over an in-flight tournament.
		if errors.Is(err, errTournamentEscalationExists) {
			s.flowDiagLog(parentRunID, "tournament_escalation_deduped",
				"tournament escalation refused: a tournament child already exists")
			return true
		}
		log.Printf("[tournament] escalation failed for run %q: %v", parentRunID, err)
		return false
	}
	_ = childID
	return true
}

// resumeParentAfterTournament is the T-4 return path at state level: the
// tournament child finished and its verdict is in.
//   - merged=true: the winner patch landed — reopen the parent loop so the
//     next Continue re-enters validation through the normal engine edges
//     (full live re-entry needs provider turns, P-5 scope; this flips the
//     state the engine resumes from).
//   - merged=false: tie + human refused — back to the classic escalate park
//     (fail-closed exactly as before the rescue).
func (s *InteractiveService) resumeParentAfterTournament(parentRunID, winner string, merged bool) error {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return fmt.Errorf("tournament: missing parent run id")
	}
	s.mu.Lock()
	if s.runs[parentRunID] == nil {
		s.mu.Unlock()
		return fmt.Errorf("tournament: unknown parent run %q", parentRunID)
	}
	s.mu.Unlock()
	if merged {
		s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = fmt.Sprintf("tournament winner %s merged — resuming validation", strings.TrimSpace(winner))
			return st
		})
		s.flowDiagLog(parentRunID, "tournament_resumed", "parent loop reopened after tournament merge",
			"winner", winner,
		)
		return nil
	}
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "cap"
		st.GateReason = "tournament deadlocked and human refused — back to manual escalation"
		return st
	})
	s.flowDiagLog(parentRunID, "tournament_returned_to_escalate", "tournament failed closed; manual escalation restored")
	return nil
}
