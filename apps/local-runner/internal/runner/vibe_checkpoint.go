package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

// vibeCheckpointLayers is highest-progress first. Demote walks downward
// when the recorded artifacts were deleted from the target project.
var vibeCheckpointLayers = []string{
	"tdd",
	vibeTaskSlicerNodeID,
	vibeCpWriterNodeID,
	vibeSSLockNodeID,
}

func vibeWorkspaceFileExists(cwd, rel string) bool {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(rel) == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(rel)))
	return err == nil && !st.IsDir() && st.Size() > 0
}

func vibeAllFilesExist(cwd string, rels []string) bool {
	if len(rels) == 0 {
		return false
	}
	for _, rel := range rels {
		if !vibeWorkspaceFileExists(cwd, rel) {
			return false
		}
	}
	return true
}

func collectVibeSSFiles(cwd string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(cwd, "requirements", "05-System-Specs", "SS-*.md"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, abs := range matches {
		base := filepath.Base(abs)
		if strings.HasPrefix(strings.ToUpper(base), "FORMAT-") {
			continue
		}
		rel, err := filepath.Rel(cwd, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func collectVibeArtifactsForNode(cwd, nodeID string, rs *interactiveRun) []string {
	switch strings.TrimSpace(nodeID) {
	case vibeSSLockNodeID, vibeSSValidatorNodeID:
		if rs != nil {
			if p := strings.TrimSpace(rs.vibeLockedSS); p != "" {
				return []string{p}
			}
			if p := strings.TrimSpace(rs.vibeLockPath); p != "" && rs.vibeLockNodeID == vibeSSLockNodeID {
				return []string{p}
			}
		}
		return collectVibeSSFiles(cwd)
	case vibeCpWriterNodeID, vibeCpLockNodeID:
		if p := collectLatestVibeCP(cwd); p != "" {
			return []string{p}
		}
		if rs != nil {
			if p := strings.TrimSpace(rs.vibeLockedCP); p != "" {
				return []string{p}
			}
		}
		return nil
	case vibeTaskSlicerNodeID, vibeSprintSlicerNodeID:
		return collectVibeTaskPlan(cwd)
	case "tdd", "audit":
		if hasVibeTddSignatures(cwd) {
			return []string{vibeTddSignaturesRel}
		}
		return collectVibeSignatureTestRels(cwd)
	default:
		return nil
	}
}

func normalizeVibeCheckpointLayer(nodeID string) string {
	switch strings.TrimSpace(nodeID) {
	case "audit", "tdd":
		return "tdd"
	case vibeSprintSlicerNodeID, vibeTaskSlicerNodeID:
		return vibeTaskSlicerNodeID
	case vibeCpLockNodeID, vibeCpWriterNodeID:
		return vibeCpWriterNodeID
	case vibeSSValidatorNodeID, vibeSSLockNodeID:
		return vibeSSLockNodeID
	default:
		return ""
	}
}

func demoteVibeCheckpoint(cwd, nodeID string, artifacts []string, rs *interactiveRun) (string, []string) {
	want := normalizeVibeCheckpointLayer(nodeID)
	if want == "" {
		want = strings.TrimSpace(nodeID)
	}
	if vibeAllFilesExist(cwd, artifacts) {
		if layer := normalizeVibeCheckpointLayer(want); layer != "" {
			want = layer
		}
		return want, append([]string(nil), artifacts...)
	}
	start := 0
	for i, layer := range vibeCheckpointLayers {
		if layer == want {
			start = i
			break
		}
	}
	for _, layer := range vibeCheckpointLayers[start:] {
		arts := collectVibeArtifactsForNode(cwd, layer, rs)
		if vibeAllFilesExist(cwd, arts) {
			return layer, arts
		}
	}
	return "", nil
}

func (s *InteractiveService) tryCommitVibeCheckpoint(parentRunID, completedNodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		return false
	}
	layer := normalizeVibeCheckpointLayer(completedNodeID)
	if layer == "" {
		return false
	}
	arts := collectVibeArtifactsForNode(rs.workspaceCwd, layer, rs)
	if !vibeAllFilesExist(rs.workspaceCwd, arts) {
		return false
	}
	rs.vibeCheckpointNode = layer
	rs.vibeCheckpointArtifacts = append([]string(nil), arts...)
	return true
}

func applyVibeCheckpointFromDisk(rs *interactiveRun) {
	if rs == nil || rs.workingMode != workingmode.Vibe {
		return
	}
	node, arts := demoteVibeCheckpoint(rs.workspaceCwd, rs.vibeCheckpointNode, rs.vibeCheckpointArtifacts, rs)
	rs.vibeCheckpointNode = node
	rs.vibeCheckpointArtifacts = arts

}

// vibeSSLockArtifactsPresent reports whether the workspace still has SS files
// an ss_lock park can lock. Empty cwd is treated as present/unknown so
// reconstruct fixtures without a sandbox keep CA-770 behavior (Task-327 T-1).
func vibeSSLockArtifactsPresent(cwd string, rs *interactiveRun) bool {
	if strings.TrimSpace(cwd) == "" {
		return true
	}
	arts := collectVibeArtifactsForNode(cwd, vibeSSLockNodeID, rs)
	return len(arts) > 0 && vibeAllFilesExist(cwd, arts)
}

// maybeRecoverMissingVibeSSLock closes CP-60 O-6 / R-SS-D residual (Task-327):
// parked on ss_lock but SS artifacts deleted → clear lock park and restart
// vibe-ingest from ingest_reader. No-op when SS still exists or cwd unknown.
func (s *InteractiveService) maybeRecoverMissingVibeSSLock(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe || !rs.vibeAwaitingLock {
		s.mu.Unlock()
		return false
	}
	nodeID := strings.TrimSpace(rs.vibeLockNodeID)
	if nodeID != "" && nodeID != vibeSSLockNodeID {
		s.mu.Unlock()
		return false
	}
	if vibeSSLockArtifactsPresent(rs.workspaceCwd, rs) {
		s.mu.Unlock()
		return false
	}
	prompt := strings.TrimSpace(rs.lastPrompt)
	if prompt == "" {
		prompt = strings.TrimSpace(rs.lastFullPrompt)
	}
	if prompt == "" {
		prompt = "[vibe] SS artifacts missing; restarting vibe-ingest from ingest_reader (Task-327)."
	}
	rs.vibeAwaitingLock = false
	rs.vibeLockNodeID = ""
	rs.vibeLockPath = ""
	rs.vibeLockedSS = ""
	rs.vibeSSSealed = false
	s.mu.Unlock()

	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		st.ActiveNode = ""
		return st
	})
	s.setFlowStepStatus(context.Background(), parentRunID, vibeSSLockNodeID, StepStatusCanceled)
	s.startResolvedFlowFromNode(context.Background(), parentRunID, workingmode.PackPrefix+vibeIngestFlowID, prompt, "ingest_reader")
	go s.persistParentSession(parentRunID)
	return true
}
