package runner

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

type vibeGateKind int

const (
	vibeGatePassthrough vibeGateKind = iota
	vibeGateRequirement
	vibeGateOwnerDebate
)

func classifyVibeGate(mode string, result flowgate.EnforceResult) vibeGateKind {
	if mode != workingmode.Vibe {
		return vibeGatePassthrough
	}
	for _, v := range result.Violations {
		if v.Rule.ID == flowgate.RequirementRuleID {
			return vibeGateRequirement
		}
	}
	if result.Action == "block" || result.Action == "reprompt" {
		return vibeGateOwnerDebate
	}
	return vibeGatePassthrough
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
	switch classifyVibeGate(rs.workingMode, result) {
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
		log.Printf("[vibe-gate] start vibe-owner-debate hub=%s child=%s", hub, runID)
		s.stashVibeFlowForDebate(hub)
		go s.startResolvedFlow(context.Background(), hub, workingmode.PackPrefix+vibeOwnerDebateFlowID, result.Message)
		return true
	default:
		return false
	}
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
