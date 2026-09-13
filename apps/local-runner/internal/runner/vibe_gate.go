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
	// Warn-mode gate (Task-352 re-review): Enforce downgrades block/reprompt to
	// "warn" under a warn-configured gate — the pre-unification live behavior
	// passed those through, so only a VIOLATION-routed debate downgrades back
	// to passthrough. Drift-routed debates (DriftRouted) are independent of
	// the gate action and still escalate.
	if res.Route == flowgate.PrecedenceRouteOwnerDebate && !res.DriftRouted && result.Action == "warn" {
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
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("vibe drift score %d (>= %d): owner debate to choose remediation",
				driftScore, vibeDriftDebateThreshold)
		}
		log.Printf("[vibe-gate] start vibe-owner-debate hub=%s child=%s drift=%d", hub, runID, driftScore)
		s.startVibeOwnerDebate(hub, message)
		return true
	default:
		return false
	}
}

// startVibeOwnerDebate stashes the parked flow and starts the debate flow
// (CP-62 P-1). The stash also drops any pending drift-ladder context
// reduction so the debate turn assembles with the full violation context (T-3).
func (s *InteractiveService) startVibeOwnerDebate(hub, message string) {
	s.stashVibeFlowForDebate(hub)
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
		return false
	}
	hub := runID
	if parentID != "" {
		hub = parentID
	}
	log.Printf("[vibe-gate] drift-only escalation run=%s score=%d -> owner debate", hub, driftScore)
	s.startVibeOwnerDebate(hub, fmt.Sprintf(
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
	if ssPath == "" && rs.parentRunID != "" {
		s.mu.Lock()
		if p := s.runs[rs.parentRunID]; p != nil {
			ssPath = strings.TrimSpace(p.vibeLockedSS)
			if ssPath == "" {
				ssPath = strings.TrimSpace(p.sourceDocID)
			}
			if cwd == "" {
				cwd = p.workspaceCwd
			}
		}
		s.mu.Unlock()
	}
	if ssPath == "" {
		ssPath = strings.TrimSpace(rs.sourceDocID)
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
