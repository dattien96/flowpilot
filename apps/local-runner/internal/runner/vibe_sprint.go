package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		case RunStatusRunning, RunStatusCompleted, RunStatusWaitingApproval, RunStatusWaitingQuestion:
			return true
		}
	}
	return false
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
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	if loop.Status == "blocked" || loop.Status == "stopped" || loop.Status == "done" {
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
