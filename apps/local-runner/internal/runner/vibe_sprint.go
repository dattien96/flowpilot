package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
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

func (s *InteractiveService) isVibeWorkingMode(parentRunID string) bool {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return false
	}
	if rs.workingMode == workingmode.Vibe {
		return true
	}
	return runHasFlowNode(rs, "tdd") && runHasFlowNode(rs, "coder")
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

// vibeTddEvidencePresent is the fail-closed TDD-evidence check (BUG-462): the
// filesystem artifact OR a frozen contract that already carries scaffold-
// pinned LockedSignatures for the coder step. The signature lock only lands
// after the scaffold gate passed for THIS run's contract, so it is strictly
// stronger evidence than a path glob and cannot be satisfied by stale
// full-body test files (CA-769 stays intact). A pre-tdd freeze (v1, no
// signatures) still fails the check.
func (s *InteractiveService) vibeTddEvidencePresent(parentRunID, coderStepID, cwd string) bool {
	if hasVibeTddOutput(cwd) {
		return true
	}
	if strings.TrimSpace(parentRunID) == "" || strings.TrimSpace(cwd) == "" {
		return false
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return false
	}
	rec, ok, _ := store.GetFrozenForStep(parentRunID, coderStepID)
	if !ok {
		return false
	}
	// LockedSignatures pin proves the scaffold gate passed; ReadOnlyPaths on the
	// coder record likewise only land via a post-freeze test lock (scaffold or
	// reproduce gate) — either is the runner's own durable attestation that TDD
	// produced an artifact. A bare v1 freeze carries neither and still fails
	// closed. (BUG-463 could leave sigs empty while read-only paths did lock.)
	return len(rec.LockedSignatures) > 0 || len(rec.ReadOnlyPaths) > 0
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

// vibeRequirementResumeCoder marks requirement parks reached from
// maybeResumeVibeCoderAfterTdd — Continue re-runs that resume function
// instead of tryAdvanceFlowFromNode (BUG-471).
const vibeRequirementResumeCoder = "resume:coder"

func (s *InteractiveService) parkVibeRequirement(parentRunID, gateReason string) {
	s.parkVibeRequirementFrom(parentRunID, gateReason, "")
}

func (s *InteractiveService) parkVibeRequirementFrom(parentRunID, gateReason, retryFromNode string) {
	if s == nil || s.agentOrchestrator == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		// BUG-471: record the advance context that parked so Continue can
		// re-invoke it — the generic hub reinvoke no-ops on post-lock vibe
		// runs (vibeHubSealed), leaving a running loop with no work.
		rs.vibeRequirementFromNode = retryFromNode
	}
	s.mu.Unlock()
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "requirement"
		st.GateReason = gateReason
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
	// BUG-365: the park must reach the client — without this graph event the
	// TUI kept "Thinking" with no [Retry] card after the slicer parked (live
	// run-646702). Mirrors parkVibeLock.
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
}

// resumeVibeRequirement re-invokes the advance that parked on
// blocked/requirement (BUG-471). The generic hub reinvoke cannot drive a
// post-lock vibe run — vibeHubSealed skips it — so a bare Continue would
// leave the loop running with no work until the stall watchdog fires.
// Re-running the same advance re-evaluates the parked condition: tasks /
// contract / tdd artifacts fixed while parked proceed normally; still
// missing re-parks with the same reason. Returns false for requirement
// parks with no recorded node (gate-classified parks keep the generic path).
func (s *InteractiveService) resumeVibeRequirement(parentRunID, feedback string) bool {
	s.mu.Lock()
	fromNode := ""
	if rs := s.runs[parentRunID]; rs != nil {
		fromNode = strings.TrimSpace(rs.vibeRequirementFromNode)
		rs.vibeRequirementFromNode = ""
	}
	s.mu.Unlock()
	if fromNode == "" {
		return false
	}
	if fromNode == vibeRequirementResumeCoder {
		s.maybeResumeVibeCoderAfterTdd(parentRunID)
		return true
	}
	msg := "resume after requirement park"
	if trimmed := strings.TrimSpace(feedback); trimmed != "" {
		msg = trimmed
	}
	if !s.tryAdvanceFlowFromNode(parentRunID, fromNode, msg) {
		s.flowDiagLog(parentRunID, "vibe_requirement_resume_no_advance", "requirement resume produced no dispatch — loop may rely on stall watchdog",
			"from_node", fromNode,
		)
	}
	return true
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
	if rs.vibeSprintBoundaryPending {
		// The boundary Continue gate owns this run's next decision;
		// resume-confirm must not stack a second gate on top of it.
		s.mu.Unlock()
		return
	}
	if rs.vibeSprintBoundaryDeclined {
		// Explicit boundary decline suppresses every reopen offer,
		// including node-resume: the operator said no more sprints.
		s.mu.Unlock()
		return
	}
	hasTdd := runHasFlowNode(rs, "tdd")
	hasCoder := runHasFlowNode(rs, "coder")
	if rs.workingMode != workingmode.Vibe && !(hasTdd && hasCoder) {
		s.mu.Unlock()
		return
	}
	// A run with live child work is actually advancing: never park a resume
	// card over it. Multi-sprint runs keep the previous sprint's completed
	// children around while the next sprint's children are live, so
	// pendingVibeResumeFromNode can otherwise resurrect a stale "Resume from
	// tdd?" card and cancel the running sprint on reopen.
	for _, child := range s.runs {
		if child == nil || child.parentRunID != parentRunID {
			continue
		}
		switch child.status {
		case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingQuestion:
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()
	// Stop seals auto-reinvoke, not the reopen gate. /open must still ask.
	from := s.pendingVibeResumeFromNode(parentRunID)
	if from == "" {
		return
	}
	s.mu.Lock()
	// Re-check and stamp in the same critical section as the loop mutate: a
	// boundary park (or a boundary-driven sprint start) landing while
	// pendingVibeResumeFromNode computed `from` owns this run's next
	// decision, and a decline suppresses every reopen offer. Never stack a
	// second card, never park a just-started sprint.
	r := s.runs[parentRunID]
	if r == nil || r.vibeSprintBoundaryPending || r.vibeSprintBoundaryDeclined || r.vibeSprintStartInFlight {
		s.mu.Unlock()
		return
	}
	r.vibeResumeConfirm = true
	r.vibeResumeFromNode = from
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeResumePausedReason
		st.GateReason = fmt.Sprintf("Resume from %s?", from)
		return st
	})
	s.mu.Unlock()
	s.parkFlowForAwaitingUser(parentRunID)
}

// forceStartVibeSprintAtTdd starts (or restarts) vibe-sprint at tdd.
// Live R-TK-K hang (run-225468): Resume from tdd → maybeResumeVibeCoderAfterTdd
// no-op'd when tdd was PENDING / no tdd-signatures yet → UI idle.
func (s *InteractiveService) forceStartVibeSprintAtTdd(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return false
	}
	cwd := rs.workspaceCwd
	cpID := rs.vibeCpDocID
	prompt := ""
	// vibeSprintIndex is started-count (chip task N/M), NOT a 0-based plan
	// index. Using it as plan[idx] started Task-905 while chip said 2/3 and
	// Task-904 was still draft (BUG-372 follow-up).
	if len(rs.vibeTaskPlan) > 0 {
		prompt = strings.TrimSpace(rs.vibeTaskPlan[vibeSprintCurrentPlanIndex(rs.vibeSprintIndex, len(rs.vibeTaskPlan))])
	}
	rs.vibeResumeConfirm = false
	rs.vibeResumeFromNode = ""
	rs.chatFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	rs.autoOrchestrate = true
	rs.flowEngineDriven = true
	if rs.status == RunStatusCancelled || rs.status == RunStatusFailed || rs.status == RunStatusCompleted {
		rs.status = RunStatusRunning
		rs.agentStatus = string(RunStatusRunning)
	}
	s.mu.Unlock()
	if prompt == "" {
		if tasks := collectVibeTaskPlanForCP(cwd, cpID); len(tasks) > 0 {
			prompt = tasks[0]
		}
	}
	if prompt == "" {
		prompt = "[vibe] resume vibe-sprint at tdd (Task-330)."
	}
	s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		st.ActiveNode = ""
		return st
	})
	abandonActiveFrozenContractsForRun(cwd, parentRunID, "vibe-sprint resume at tdd")
	stampVibeTaskInProgress(cwd, prompt)
	s.setFlowStepStatus(context.Background(), parentRunID, "tdd", StepStatusPending)
	s.startResolvedFlowFromNode(context.Background(), parentRunID, workingmode.PackPrefix+vibeSprintFlowID, prompt, "tdd")
	go s.persistParentSession(parentRunID)
	return true
}

// resumeVibeAfterTddGate handles Resume OK when from=tdd (R-TK-K).
// Prefers coder advance when tdd artifacts exist; otherwise starts tdd.
func (s *InteractiveService) resumeVibeAfterTddGate(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	cwd := ""
	checkpoint := ""
	if rs != nil {
		cwd = rs.workspaceCwd
		checkpoint = rs.vibeCheckpointNode
	}
	s.mu.Unlock()
	tddDone := checkpoint == "tdd" || s.vibeTddStepDone(parentRunID)
	if s.vibeTddEvidencePresent(parentRunID, "coder", cwd) && tddDone {
		s.maybeResumeVibeCoderAfterTdd(parentRunID)
		return
	}
	s.forceStartVibeSprintAtTdd(parentRunID)
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
	if !s.vibeTddEvidencePresent(parentRunID, "coder", cwd) {
		if tddDone {
			s.parkVibeRequirementFrom(parentRunID, "tdd artifact missing before coder (no bypass)", vibeRequirementResumeCoder)
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
	s.parkVibeRequirementFrom(parentRunID, "sprint graph missing; cannot resume coder", vibeRequirementResumeCoder)
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
