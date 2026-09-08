package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

const vibeLockBlockReason = "vibe_lock"

func isVibeLockNode(id string) bool {
	return id == vibeSSLockNodeID || id == vibeCpLockNodeID
}

func (s *InteractiveService) parkVibeLock(parentRunID, nodeID string) bool {
	cwd := s.workspaceCwdFor(parentRunID)
	s.mu.Lock()
	rs := s.runs[parentRunID]
	path := ""
	if rs != nil {
		path = strings.TrimSpace(rs.sourceDocID)
		if path == "" {
			path = strings.TrimSpace(rs.vibeLockedCP)
		}
		if path == "" {
			path = strings.TrimSpace(rs.vibeLockedSS)
		}
		rs.vibeLockNodeID = nodeID
		rs.vibeLockPath = path
		rs.vibeAwaitingLock = true
	}
	s.mu.Unlock()

	draft := ""
	if path != "" && cwd != "" {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, path)
		}
		if b, err := os.ReadFile(abs); err == nil {
			draft = string(b)
		}
	}
	kind := "SS Preview & Lock"
	if nodeID == vibeCpLockNodeID {
		kind = "CP Preview & Lock"
	}
	gate := kind
	if path != "" {
		gate += " — " + path
	}
	gate += "\nEmpty Continue locks the current draft. Paste edits to write-back and re-validate.\n"
	if draft != "" {
		if len(draft) > 24000 {
			draft = draft[:24000] + "\n…"
		}
		gate += "\n" + draft
	}

	s.setFlowStepStatus(context.Background(), parentRunID, nodeID, StepStatusWaitingUserApr)
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeLockBlockReason
		st.GateReason = gate
		st.ActiveNode = nodeID
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	return true
}

func (s *InteractiveService) resumeVibeLock(parentRunID, feedback string, snap AgentGraphSnapshot) (AgentGraphSnapshot, bool) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return snap, false
	}
	nodeID := rs.vibeLockNodeID
	path := rs.vibeLockPath
	cwd := rs.workspaceCwd
	if nodeID == "" {
		s.mu.Unlock()
		return snap, false
	}
	s.mu.Unlock()

	fb := strings.TrimSpace(feedback)
	if fb == "continue" || fb == "lock" {
		fb = ""
	}
	abs := path
	if abs != "" && cwd != "" && !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, path)
	}
	if fb != "" && abs != "" {
		if nodeID == vibeCpLockNodeID {
			if err := workingmode.RejectNonCP(path, fb); err != nil {
				s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
					st.Status = "blocked"
					st.BlockReason = vibeLockBlockReason
					st.GateReason = err.Error() + "\n" + fb
					return st
				})
				s.parkFlowForAwaitingUser(parentRunID)
				return s.agentGraphSnapshot(parentRunID), true
			}
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err == nil {
			_ = os.WriteFile(abs, []byte(fb), 0o644)
		}
	}

	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		r.vibeAwaitingLock = false
		r.sourceDocID = path
		if nodeID == vibeCpLockNodeID {
			r.vibeLockedCP = path
		} else {
			r.vibeLockedSS = path
		}
	}
	s.mu.Unlock()
	s.setFlowStepStatus(context.Background(), parentRunID, nodeID, StepStatusDone)
	cleared := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		return st
	})
	s.emitAgentGraph(parentRunID, cleared)
	go s.tryAdvanceFlowFromNode(parentRunID, nodeID, "locked")
	return cleared, true
}
