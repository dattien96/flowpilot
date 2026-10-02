package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// vibeDriftDebateThreshold mirrors flowgate.VibeDriftDebateThreshold (CP-62
// P-1): a vibe run whose drift score reaches it escalates through the
// owner-debate resolver instead of the dev ladder's deferred pause path.
const vibeDriftDebateThreshold = flowgate.VibeDriftDebateThreshold

type vibeGateKind int

const (
	vibeGatePassthrough vibeGateKind = iota
	vibeGateRequirement
	vibeGateOwnerDebate
)

func classifyVibeGate(mode string, result flowgate.EnforceResult) vibeGateKind {
	// Pre-CP-62 signature kept byte-stable for the legacy pins
	// (vibe_gate_test.go): drift score 0 reproduces the old behavior.
	return classifyVibeGateWithDrift(mode, result, 0)
}

// classifyVibeGateWithDrift is the CP-62 P-1 drift-aware classifier.
// mode != vibe is passthrough (the Task-335 ladder owns dev). In vibe:
// requirement-class wins first (user-only, SS-18 BR-4); drift >= 80 escalates
// to the owner debate even on a clean gate; other block/reprompt violations
// keep today's owner-debate semantics; warn-gated results pass through
// (Enforce's warn downgrade is honored — Task-352 re-review).
// Task-351: the live classifier CONSUMES flowgate.ResolvePrecedence (SD-20 §7)
// instead of duplicating the precedence — one semantics, one place to change.
func classifyVibeGateWithDrift(mode string, result flowgate.EnforceResult, driftScore int) vibeGateKind {
	if mode != workingmode.Vibe {
		return vibeGatePassthrough
	}
	// isOwnerDebateTurn stays false: the debate child carries its own fresh
	// per-run drift state (structural no-re-route), so the drift-routed
	// debate is the only escalation surface.
	res := flowgate.ResolvePrecedence(result.Violations, mode, driftScore, false)
	// Warn-mode gate (Task-352 re-review, extended CA-1086): Enforce downgrades
	// block/reprompt to "warn" under a warn-configured gate — the pre-unification
	// live behavior passed those through, so every violation-routed escalation
	// downgrades back to passthrough, requirement-class included (a warn event
	// still surfaces to the user — BR-4 routing preserved; it just cannot park
	// a chat-surface run into flow_awaiting_user). Drift-routed debates
	// (DriftRouted) are independent of the gate action and still escalate.
	if !res.DriftRouted && result.Action == "warn" {
		return vibeGatePassthrough
	}
	switch res.Route {
	case flowgate.PrecedenceRouteUser:
		return vibeGateRequirement
	case flowgate.PrecedenceRouteOwnerDebate:
		return vibeGateOwnerDebate
	default:
		return vibeGatePassthrough
	}
}

func requirementDetail(result flowgate.EnforceResult) string {
	for _, v := range result.Violations {
		if v.Rule.ID == flowgate.RequirementRuleID {
			if v.Detail != "" {
				return v.Detail
			}
		}
	}
	return "requirement signatures drifted from the locked SS"
}

func (s *InteractiveService) applyVibeGateResolver(runID, parentID, turnID string, rs *interactiveRun, result flowgate.EnforceResult) bool {
	if rs == nil {
		return false
	}
	// CP-62 P-1 (Task-337): the classifier is drift-aware — a vibe run whose
	// latest turn reached the debate threshold escalates even when the gate
	// itself is clean (DriftRouted), never into the dev card / deferred pause.
	driftScore := s.latestVibeDriftScore(rs)
	switch classifyVibeGateWithDrift(rs.workingMode, result, driftScore) {
	case vibeGateRequirement:
		detail := requirementDetail(result)
		if s.agentOrchestrator != nil {
			s.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Status = "blocked"
				st.BlockReason = "requirement"
				st.GateReason = detail
				return st
			})
		}
		s.mu.Lock()
		if r := s.runs[runID]; r != nil {
			r.status = RunStatusWaitingUserApr
			r.agentStatus = string(RunStatusWaitingUserApr)
			// BUG-471: this gate park records no advance context — clear any
			// stale one so Continue takes the generic path, never a replay of
			// an earlier advance-path park's node.
			r.vibeRequirementFromNode = ""
			s.emitLocked(r, ProviderEvent{
				Type:           EventFlowGateViolation,
				ProviderTurnID: turnID,
				Error:          detail,
				Status:         "block",
			})
		}
		s.mu.Unlock()
		return true
	case vibeGateOwnerDebate:
		hub := runID
		if parentID != "" {
			hub = parentID
		}
		// BUG-570 (live run-100368): a reprompt-class violation routed to the
		// owner debate IS the remediation reprompt — the debate's own verdict
		// orders the gated child to redo the work. The dev/child reprompt
		// branches bound this loop with repromptAttempts/maxFlowGateReprompts,
		// but this route returned early and never counted, so a stubborn
		// violation mounted an unbounded debate → reprompt → debate cycle.
		// Consume the same budget here; on exhaustion escalate like the
		// sibling reprompt paths instead of mounting another debate.
		if result.Action == "reprompt" {
			s.mu.Lock()
			gated := s.runs[runID]
			attempts := 0
			if gated != nil {
				attempts = gated.repromptAttempts
				gated.repromptAttempts++
			}
			s.mu.Unlock()
			if attempts >= maxFlowGateReprompts {
				log.Printf("[vibe-gate] debate-routed reprompt budget exhausted run=%s attempts=%d — escalating instead of mounting another debate", runID, attempts)
				if parentID != "" {
					s.mu.Lock()
					s.stampEscalatedChildNodeLocked(parentID, childEscalatedNodeID(rs))
					s.mu.Unlock()
					_, _ = s.applyFlowControl(parentID, FlowControlInput{
						Status:  "escalate",
						Summary: "owner-debate remediation reprompts exhausted: " + result.Message,
					})
				} else {
					_, _ = s.applyFlowControl(runID, FlowControlInput{
						Status:  "escalate",
						Summary: "owner-debate remediation reprompts exhausted: " + result.Message,
					})
				}
				return true
			}
		}
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("vibe drift score %d (>= %d): owner debate to choose remediation",
				driftScore, vibeDriftDebateThreshold)
		}
		log.Printf("[vibe-gate] start vibe-owner-debate hub=%s child=%s drift=%d", hub, runID, driftScore)
		s.startVibeOwnerDebate(hub, runID, message)
		return true
	default:
		return false
	}
}

// startVibeOwnerDebate stashes the parked flow and starts the debate flow
// (CP-62 P-1). The stash also drops any pending drift-ladder context
// reduction so the debate turn assembles with the full violation context (T-3).
// gatedRunID is the run whose post-turn gate was diverted into the debate
// (empty when the resolver had no gated child — e.g. drift-only on the hub).
func (s *InteractiveService) startVibeOwnerDebate(hub, gatedRunID, message string) {
	// CA-1095: stash is the atomic mount decision — it reports false when a
	// debate already owns this hub (violation-routed escalation mid-debate,
	// or a concurrent drift mount). The gated child was still recorded for
	// its post-debate reprompt; launching a second debate flow here is the
	// nested-mount wedge from live run-3362.
	if !s.stashVibeFlowForDebate(hub, gatedRunID) {
		log.Printf("[vibe-gate] owner debate mount suppressed hub=%s gated=%s: debate already active", hub, gatedRunID)
		return
	}
	go s.startResolvedFlow(context.Background(), hub, workingmode.PackPrefix+vibeOwnerDebateFlowID, message)
}

// latestVibeDriftScore reads the drift score of the most recent evaluated
// turn for the run. 0 when the drift detector flag is OFF (Task-335 default:
// behavior-neutral), so the CP-62 drift routing stays inert by default.
func (s *InteractiveService) latestVibeDriftScore(rs *interactiveRun) int {
	if s == nil || rs == nil || !driftDetectorEnabled() {
		return 0
	}
	st := driftStateFor(s, rs.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.lastScore
}

// applyVibeDriftOnlyResolver routes a clean-gate vibe turn whose drift score
// reached the debate threshold (CP-62 P-1 rule 2: never the deferred pause
// path, never a raw dev card). Called from the gate's no-violations branch
// before the passthrough return; dev mode is untouched.
func (s *InteractiveService) applyVibeDriftOnlyResolver(runID, parentID string, rs *interactiveRun) bool {
	if s == nil || rs == nil || rs.workingMode != workingmode.Vibe || !driftDetectorEnabled() {
		return false
	}
	driftScore := s.latestVibeDriftScore(rs)
	if driftScore < vibeDriftDebateThreshold {
		// The score decayed below the debate threshold — real correction
		// happened (a clean turn with a file delta halves the carried
		// score). Re-arm the drift-only escalation so a genuinely NEW
		// drift episode can mount a fresh debate.
		st := driftStateFor(s, rs.id)
		st.mu.Lock()
		st.debateDischarged = false
		st.mu.Unlock()
		return false
	}
	hub := runID
	if parentID != "" {
		hub = parentID
	}
	// CA-1063: never re-mount the owner debate while the hub is already inside
	// it (sprint topology parked for the debate, or the debate flow still the
	// active chat flow). Debate turns are deliberation — zero file deltas is
	// their normal outcome — so a zero_delta_progress-driven drift score
	// re-mounts the debate on every debate_synthesis turn, loops remediation
	// forever, and the parked sprint chain never resumes (live run-15525:
	// scores 80→100 mounted a fresh debate on every synthesis turn until the
	// operator interrupted).
	s.mu.Lock()
	hubRs := rs
	if hub != runID {
		hubRs = s.runs[hub]
	}
	inDebate := hubRs != nil && (len(hubRs.vibeParkedNodes) > 0 ||
		workingmode.BareFlowID(hubRs.chatFlowRef) == vibeOwnerDebateFlowID)
	s.mu.Unlock()
	if inDebate {
		log.Printf("[vibe-gate] drift-only escalation suppressed run=%s score=%d: owner debate already active", hub, driftScore)
		return false
	}
	// Post-debate discharge (CA-1073): a drift-only debate already mounted for
	// this accumulated debt. While the score stays pinned at/above the
	// threshold — which orchestration turns guarantee, since they produce
	// zero file delta by design — re-mounting would re-park the sprint on
	// every clean-gate turn and starve the reprompted worker the debate just
	// ordered. Suppress until the score decays below the threshold and
	// re-climbs (fresh drift evidence). A violation-routed debate
	// (applyVibeGateResolver) is unaffected: real gate failures still
	// escalate per turn.
	st := driftStateFor(s, rs.id)
	st.mu.Lock()
	if st.debateDischarged {
		st.mu.Unlock()
		log.Printf("[vibe-gate] drift-only escalation suppressed run=%s score=%d: drift debt already discharged by a prior debate; re-arms when the score decays below %d and climbs again", hub, driftScore, vibeDriftDebateThreshold)
		return false
	}
	st.debateDischarged = true
	st.mu.Unlock()
	log.Printf("[vibe-gate] drift-only escalation run=%s score=%d -> owner debate", hub, driftScore)
	s.startVibeOwnerDebate(hub, runID, fmt.Sprintf(
		"vibe drift score %d (>= %d) on a clean gate: owner debate to choose remediation",
		driftScore, vibeDriftDebateThreshold))
	return true
}

func prepareWorkingModeRules(mode string, rules []flowgate.Rule, tr *flowgate.TurnResult) []flowgate.Rule {
	if mode == workingmode.Vibe {
		flowgate.CoerceVibeRequirementDrift(tr)
	}
	return flowgate.EnabledRulesFor(mode, rules)
}

func (s *InteractiveService) injectVibeSSDrift(rs *interactiveRun, tr *flowgate.TurnResult) {
	if s == nil || rs == nil || tr == nil || rs.workingMode != workingmode.Vibe {
		return
	}
	if !tr.Tests.Ran || len(tr.Tests.Failed) > 0 || len(tr.Tests.Regressed) > 0 {
		return
	}
	if tr.RequirementDrift {
		return
	}
	ssPath := strings.TrimSpace(rs.vibeLockedSS)
	cwd := strings.TrimSpace(rs.workspaceCwd)
	// CA-1086: the sourceDocID fallback only counts when the pinned doc is an
	// actual SS — CP-sourced vibe runs pin CP-*.md and task/bugfix chats pin
	// Task-*/BUG-* docs; treating those as the locked SS makes every AC-* line
	// read "uncovered" the moment a green-baseline turn writes no tests (live
	// run-2280 user-blocked on a plain chat turn).
	isSpecDoc := func(p string) bool {
		return strings.HasPrefix(filepath.Base(p), "SS-")
	}
	if ssPath == "" && rs.parentRunID != "" {
		s.mu.Lock()
		if p := s.runs[rs.parentRunID]; p != nil {
			ssPath = strings.TrimSpace(p.vibeLockedSS)
			if ssPath == "" {
				if d := strings.TrimSpace(p.sourceDocID); isSpecDoc(d) {
					ssPath = d
				}
			}
			if cwd == "" {
				cwd = p.workspaceCwd
			}
		}
		s.mu.Unlock()
	}
	if ssPath == "" {
		if d := strings.TrimSpace(rs.sourceDocID); isSpecDoc(d) {
			ssPath = d
		}
	}
	ssBody := ""
	if ssPath != "" && cwd != "" {
		abs := ssPath
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, ssPath)
		}
		if b, err := os.ReadFile(abs); err == nil {
			ssBody = string(b)
		}
	}
	if ssBody == "" {
		return
	}
	var b strings.Builder
	for _, pth := range tr.WrittenPaths {
		if !isVibeTestSourcePath(pth) {
			continue
		}
		abs := pth
		if cwd != "" && !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, pth)
		}
		if raw, err := os.ReadFile(abs); err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	// CA-1086: a turn that wrote no test sources has nothing to compare —
	// without this every locked-SS AC reads "uncovered" on any quiet turn.
	if b.Len() == 0 {
		return
	}
	drift, detail := flowgate.ComputeRequirementDrift(ssBody, b.String())
	if drift {
		tr.RequirementDrift = true
		tr.RequirementDriftDetail = detail
	}
}

func isVibeTestSourcePath(p string) bool {
	p = strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	return strings.HasSuffix(p, "_test.go") ||
		strings.Contains(p, "_test.") ||
		strings.HasSuffix(p, ".test.ts") ||
		strings.HasSuffix(p, ".test.tsx") ||
		strings.HasSuffix(p, ".test.js")
}
