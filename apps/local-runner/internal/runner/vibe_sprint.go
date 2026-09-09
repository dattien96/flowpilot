package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

const vibeTddSignaturesRel = "requirements/.flowpilot/vibe/tdd-signatures.md"

func runHasFlowNode(rs *interactiveRun, id string) bool {
	if rs == nil || strings.TrimSpace(id) == "" {
		return false
	}
	for _, n := range rs.activeFlowNodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func vibeCoderBlocked(workingMode, fromNode, coderNodeID string, tddDone, hasTestArtifact bool) bool {
	if workingMode != workingmode.Vibe || coderNodeID != "coder" {
		return false
	}
	if !tddDone {
		return true
	}
	if fromNode == "tdd" && !hasTestArtifact {
		return true
	}
	return false
}

// vibeCoderSpawnBlocked is the fail-closed spawn gate: any vibe coder needs
// the sprint tdd-signatures artifact (no glob of pre-existing tests).
func vibeCoderSpawnBlocked(workingMode, coderNodeID string, hasSignatures bool) bool {
	return workingMode == workingmode.Vibe && coderNodeID == "coder" && !hasSignatures
}

func hasVibeTddSignatures(cwd string) bool {
	return vibeWorkspaceFileExists(cwd, vibeTddSignaturesRel)
}

// hasVibeTddOutput is the coder spawn gate. The pack writes
// tdd-signatures.md; harness-style empty TestX frames in *_test.go also
// count when git-new (untracked/added). Full-body tests (t.Fatal/assert)
// and tracked repo placeholders do not (CA-769).
func hasVibeTddOutput(cwd string) bool {
	if hasVibeTddSignatures(cwd) {
		return true
	}
	return len(collectVibeSignatureTestRels(cwd)) > 0
}

func vibeStripComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				b.WriteByte('\n')
			}
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func vibeFileLooksLikeSignatureOnly(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return false
	}
	s := vibeStripComments(string(raw))
	if !strings.Contains(s, "func Test") && !strings.Contains(s, "test(") {
		return false
	}
	lower := strings.ToLower(s)
	for _, needle := range []string{"t.error", "t.fatal", "t.fail", "assert.", "require.", "expect("} {
		if strings.Contains(lower, needle) {
			return false
		}
	}
	return true
}

func vibeRelIsGitNew(cwd, rel string) bool {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(rel) == "" {
		return false
	}
	inside, err := exec.Command("git", "-C", cwd, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return false
	}
	out, err := exec.Command("git", "-C", cwd, "status", "--porcelain", "--", filepath.ToSlash(rel)).Output()
	if err != nil {
		return false
	}
	line := strings.TrimSpace(string(out))
	if len(line) < 2 {
		return false
	}
	x, y := line[0], line[1]
	return (x == '?' && y == '?') || x == 'A' || y == 'A'
}

func collectVibeSignatureTestRels(cwd string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	var out []string
	_ = filepath.Walk(cwd, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() {
				name := info.Name()
				if name == "vendor" || name == "node_modules" || name == ".git" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") && !strings.HasSuffix(p, ".test.ts") && !strings.HasSuffix(p, ".test.tsx") {
			return nil
		}
		if !vibeFileLooksLikeSignatureOnly(p) {
			return nil
		}
		rel, err := filepath.Rel(cwd, p)
		if err != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		if !vibeRelIsGitNew(cwd, rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out
}

func (s *InteractiveService) countChildRunsWithLabelLocked(parentRunID, label string) int {
	n := 0
	for _, run := range s.runs {
		if run.parentRunID == parentRunID && run.label == label {
			n++
		}
	}
	return n
}

func (s *InteractiveService) parkVibeRequirement(parentRunID, gateReason string) {
	if s == nil || s.agentOrchestrator == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "requirement"
		st.GateReason = gateReason
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
}

func (s *InteractiveService) vibeTddStepDone(parentRunID string) bool {
	if s == nil || s.workflowStore == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		id := strings.TrimSpace(step.NodeID)
		if id == "" {
			id = strings.TrimSpace(step.ID)
		}
		if id == "tdd" {
			return step.Status == StepStatusDone
		}
	}
	return false
}

func (s *InteractiveService) persistedLiveCoderExists(parentRunID string) bool {
	if s == nil || s.workflowStore == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return false
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		return false
	}
	for _, session := range sessions {
		if session.ParentRunID != parentRunID || strings.TrimSpace(session.Label) != "coder" {
			continue
		}
		switch session.Status {
		case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingQuestion:
			return true
		}
	}
	return false
}

// persistedCompletedChildExists reports a durable Completed child with the
// given label. After a restart step rows reseed as PENDING (per-step progress
// is not persisted), so a DONE row alone cannot prove the node finished —
// the persisted child session is the fallback evidence (turn-2 RestartLost).
func (s *InteractiveService) persistedCompletedChildExists(parentRunID, label string) bool {
	if s == nil || s.workflowStore == nil || strings.TrimSpace(parentRunID) == "" || strings.TrimSpace(label) == "" {
		return false
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return false
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		return false
	}
	for _, session := range sessions {
		if session.ParentRunID != parentRunID || strings.TrimSpace(session.Label) != strings.TrimSpace(label) {
			continue
		}
		if session.Status == RunStatusCompleted {
			return true
		}
	}
	return false
}

const vibeResumePausedReason = "paused"

func (s *InteractiveService) vibeNodeHasLiveWork(parentRunID, nodeID string) bool {
	if s == nil || strings.TrimSpace(parentRunID) == "" || strings.TrimSpace(nodeID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return false
	}
	if rs.turnInFlight && strings.TrimSpace(rs.activeHubNodeID) == nodeID {
		return true
	}
	for _, child := range s.runs {
		if child == nil || child.parentRunID != parentRunID || child.label != nodeID {
			continue
		}
		switch child.status {
		case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingQuestion:
			return true
		}
	}
	return false
}

func (s *InteractiveService) vibeSuccessorNeedsResume(parentRunID, nodeID string) bool {
	switch s.lookupFlowStepStatus(parentRunID, nodeID) {
	case StepStatusDone, StepStatusSkipped:
		return false
	case StepStatusRunning:
		// Ghost RUNNING after a dropped hub reinvoke (run-220036 synthesis).
		return !s.vibeNodeHasLiveWork(parentRunID, nodeID)
	default:
		// PENDING / WAITING / CANCELED / FAILED: reconstruct remaps in-flight
		// RUNNING to CANCELED (I-17). That is unfinished, not "do not resume".
		return true
	}
}

func (s *InteractiveService) pendingVibeResumeFromNode(parentRunID string) string {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return ""
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return ""
	}
	nodes := append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	edges := append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
	s.mu.Unlock()
	from := ""
	for _, n := range nodes {
		if s.lookupFlowStepStatus(parentRunID, n.ID) != StepStatusDone && !s.persistedCompletedChildExists(parentRunID, n.ID) {
			continue
		}
		for _, e := range edges {
			if e.From != n.ID || !strings.EqualFold(e.When, "done") || !strings.EqualFold(e.Kind, "forward") || e.To == "" {
				continue
			}
			if e.To == "done" || e.To == "ask_user" {
				continue
			}
			if s.vibeSuccessorNeedsResume(parentRunID, e.To) {
				from = n.ID
			}
		}
	}
	return from
}

func (s *InteractiveService) healVibeFailedForReopenPark(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	if s.pendingVibeResumeFromNode(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" {
		return
	}
	if rs.status != RunStatusFailed {
		return
	}
	if rs.workingMode != workingmode.Vibe && !(runHasFlowNode(rs, "tdd") && runHasFlowNode(rs, "coder")) {
		return
	}
	// Stop-fence / stall left Failed + hub_stalled on disk. /open must park
	// Resume from last DONE, not skip park (CA-806 genuine Failed stays Failed
	// when there is no unfinished successor).
	rs.status = RunStatusCancelled
	rs.agentStatus = string(RunStatusCancelled)
}

func (s *InteractiveService) maybeParkVibeResumeConfirm(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || rs.status == RunStatusFailed {
		s.mu.Unlock()
		return
	}
	hasTdd := runHasFlowNode(rs, "tdd")
	hasCoder := runHasFlowNode(rs, "coder")
	if rs.workingMode != workingmode.Vibe && !(hasTdd && hasCoder) {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// Stop seals auto-reinvoke, not the reopen gate. /open must still ask.
	from := s.pendingVibeResumeFromNode(parentRunID)
	if from == "" {
		return
	}
	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		r.vibeResumeConfirm = true
		r.vibeResumeFromNode = from
	}
	s.mu.Unlock()
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeResumePausedReason
		st.GateReason = fmt.Sprintf("Resume from %s?", from)
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
}

func (s *InteractiveService) maybeResumeVibeCoderAfterTdd(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return
	}
	if rs.vibeCoderResumeInFlight {
		s.mu.Unlock()
		return
	}
	rs.vibeCoderResumeInFlight = true
	cwd := rs.workspaceCwd
	checkpoint := rs.vibeCheckpointNode
	hasTdd := runHasFlowNode(rs, "tdd")
	hasCoder := runHasFlowNode(rs, "coder")
	coderKids := s.countChildRunsWithLabelLocked(parentRunID, "coder")
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeCoderResumeInFlight = false
		}
		s.mu.Unlock()
	}()
	if !s.loopIsAdvancing(parentRunID) {
		return
	}
	tddDone := checkpoint == "tdd" || s.vibeTddStepDone(parentRunID)
	if !hasVibeTddOutput(cwd) {
		if tddDone {
			s.parkVibeRequirement(parentRunID, "tdd artifact missing before coder (no bypass)")
		}
		return
	}
	if coderKids > 0 || s.persistedLiveCoderExists(parentRunID) {
		return
	}
	switch s.lookupFlowStepStatus(parentRunID, "coder") {
	case StepStatusRunning, StepStatusDone, StepStatusWaitingUserApr:
		return
	}
	if !tddDone {
		return
	}
	if hasTdd && hasCoder {
		s.tryAdvanceFlowFromNode(parentRunID, "tdd", "resume after tdd")
		return
	}
	s.parkVibeRequirement(parentRunID, "sprint graph missing; cannot resume coder")
}

func hasVibeTestArtifact(cwd string) bool {
	if hasVibeTddSignatures(cwd) {
		return true
	}
	if strings.TrimSpace(cwd) == "" {
		return false
	}
	found := false
	_ = filepath.Walk(cwd, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() {
				name := info.Name()
				if name == "vendor" || name == "node_modules" || name == ".git" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, ".test.ts") || strings.HasSuffix(p, ".test.tsx") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
