package runner

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// deleteChatSession removes a run from memory, persistent storage, and all
// provider session files it can locate on disk. A single chat run belongs to
// exactly one provider (never both Claude and Codex), but a session file may
// have been copied into multiple account homes via the cross-account resume
// feature, so we scan every registered account of the matching provider type.
//
// File deletion failures are swallowed (best-effort): we always complete the
// memory + store cleanup so the history entry disappears for the user.
func (s *InteractiveService) deleteChatSession(runID string) *apiErr {
	session, found := s.sessionForDelete(runID)
	if !found {
		return newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}

	deleteOrder, sessionsByRun, treeErr := s.collectDeleteRunTree(runID, session)
	if treeErr != nil {
		// BUG-491: refuse the delete — a partial tree leaves orphaned
		// durable records (and possibly live bindings) behind.
		return newAPIErr(http.StatusBadGateway, "session_index_unavailable", treeErr.Error())
	}
	for _, id := range deleteOrder {
		session, ok := sessionsByRun[id]
		if !ok {
			continue
		}
		s.deleteProviderFilesForSession(id, session)
	}

	s.pruneDeletedRunState(deleteOrder)

	// --- remove from persistent store ---
	if deleter, ok := s.workflowStore.(interface {
		DeleteProviderSession(ctx context.Context, runID string) error
	}); ok {
		for _, id := range deleteOrder {
			if err := deleter.DeleteProviderSession(context.Background(), id); err != nil {
				return newAPIErr(http.StatusInternalServerError, "store_error", err.Error())
			}
		}
	}

	// --- remove per-run turn log (BUG-083) ---
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		for _, id := range deleteOrder {
			_ = logger.DeleteTurnLog(context.Background(), id)
		}
	}

	// --- remove per-run CP-41 flow-events sidecar ---
	if fes, ok := s.workflowStore.(FlowEventStore); ok {
		for _, id := range deleteOrder {
			_ = fes.DeleteFlowEvents(context.Background(), id)
		}
	}

	// --- remove per-run step-transition sidecar (Task-239 B5) ---
	if tlog, ok := s.workflowStore.(StepTransitionLogStore); ok {
		for _, id := range deleteOrder {
			_ = tlog.DeleteStepTransitions(context.Background(), id)
		}
	}

	return nil
}

func (s *InteractiveService) sessionForDelete(runID string) (ProviderSessionState, bool) {
	if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
		session, found, err := reader.GetProviderSession(context.Background(), runID)
		if err == nil && found {
			return session, true
		}
	}
	s.mu.Lock()
	rs, inMem := s.runs[runID]
	s.mu.Unlock()
	if !inMem {
		return ProviderSessionState{}, false
	}
	return sessionStateOf(rs), true
}

func (s *InteractiveService) collectDeleteRunTree(runID string, root ProviderSessionState) ([]string, map[string]ProviderSessionState, error) {
	sessionsByRun := map[string]ProviderSessionState{root.RunID: root}
	childrenByParent := make(map[string][]string)
	addChild := func(parentRunID, childRunID string) {
		parentRunID = strings.TrimSpace(parentRunID)
		childRunID = strings.TrimSpace(childRunID)
		if parentRunID == "" || childRunID == "" || parentRunID == childRunID {
			return
		}
		childrenByParent[parentRunID] = append(childrenByParent[parentRunID], childRunID)
	}

	if indexReader, ok := s.workflowStore.(SessionIndexReader); ok {
		// BUG-491: delete walks the whole tree — a partial enumeration would
		// orphan persisted children. Fail closed instead.
		sessions, err := indexReader.ListAllProviderSessions(context.Background())
		if err != nil {
			return nil, nil, fmt.Errorf("collectDeleteRunTree: session index unreadable: %w", err)
		}
		for _, session := range sessions {
			if _, exists := sessionsByRun[session.RunID]; !exists {
				sessionsByRun[session.RunID] = session
			}
			addChild(session.ParentRunID, session.RunID)
		}
	}

	s.mu.Lock()
	for _, rs := range s.runs {
		session := sessionStateOf(rs)
		if _, exists := sessionsByRun[session.RunID]; !exists {
			sessionsByRun[session.RunID] = session
		}
		addChild(session.ParentRunID, session.RunID)
	}
	s.mu.Unlock()

	seen := make(map[string]struct{})
	order := make([]string, 0, len(sessionsByRun))
	var walk func(string)
	walk = func(id string) {
		if _, visited := seen[id]; visited {
			return
		}
		seen[id] = struct{}{}
		childSeen := make(map[string]struct{})
		for _, childID := range childrenByParent[id] {
			if _, duplicate := childSeen[childID]; duplicate {
				continue
			}
			childSeen[childID] = struct{}{}
			walk(childID)
		}
		order = append(order, id)
	}
	walk(runID)
	return order, sessionsByRun, nil
}

func (s *InteractiveService) deleteProviderFilesForSession(runID string, session ProviderSessionState) {
	sessionIDs := s.deleteSessionIDsForRun(runID, session)
	if session.ProviderKey == "" || len(sessionIDs) == 0 {
		return
	}
	r := s.runner
	if r == nil {
		r = &Runner{}
	}
	accounts, err := r.ListProviderAccounts()
	if err != nil {
		return
	}
	for _, account := range accounts {
		if ProviderKey(account.ProviderKey) != session.ProviderKey {
			continue
		}
		if strings.TrimSpace(account.HomePath) == "" {
			continue
		}
		for _, sessionID := range sessionIDs {
			filePath, ok := LocateSessionFile(session.ProviderKey, account.HomePath, sessionID, session.WorkingDirectory)
			if !ok {
				continue
			}
			// Grok session artifacts are directories; Codex/Claude are single files.
			if session.ProviderKey == ProviderKeyGrok {
				_ = os.RemoveAll(filePath)
			} else {
				_ = os.Remove(filePath)
			}
		}
	}
}

func (s *InteractiveService) pruneDeletedRunState(runIDs []string) {
	if len(runIDs) == 0 {
		return
	}
	deleted := make(map[string]struct{}, len(runIDs))
	for _, id := range runIDs {
		deleted[id] = struct{}{}
	}

	s.mu.Lock()
	for _, id := range runIDs {
		// CP-84 (Task-429): tombstone the run on the mux plane so subscribed
		// clients drop its lane immediately — drain sees rs gone -> remove.
		s.markRunRealtimeDirtyLocked(id)
		delete(s.runs, id)
	}
	s.mu.Unlock()

	o := s.agentOrchestrator
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, id := range runIDs {
		delete(o.children, id)
		delete(o.waiters, id)
		delete(o.historical, id)
		delete(o.summaries, id)
		delete(o.edges, id)
		delete(o.bus, id)
		delete(o.loop, id)
		delete(o.queued, id)
	}
	// Purge cohort buffers keyed by deleted parent run IDs.
	for k := range o.cohort {
		for _, id := range runIDs {
			if strings.HasPrefix(k, id+"/") {
				delete(o.cohort, k)
				delete(o.cohortExpected, k)
			}
		}
	}

	for parentRunID, childIDs := range o.children {
		kept := childIDs[:0]
		for _, childID := range childIDs {
			if _, remove := deleted[childID]; remove {
				continue
			}
			kept = append(kept, childID)
		}
		if len(kept) == 0 {
			delete(o.children, parentRunID)
			continue
		}
		o.children[parentRunID] = kept
	}

	for parentRunID, summaries := range o.summaries {
		for id := range summaries {
			if _, remove := deleted[id]; remove {
				delete(summaries, id)
			}
		}
		if len(summaries) == 0 {
			delete(o.summaries, parentRunID)
		}
	}

	for parentRunID, historical := range o.historical {
		kept := historical[:0]
		for _, summary := range historical {
			if _, remove := deleted[summary.RunID]; remove {
				continue
			}
			kept = append(kept, summary)
		}
		if len(kept) == 0 {
			delete(o.historical, parentRunID)
			continue
		}
		o.historical[parentRunID] = kept
	}

	for parentRunID, edges := range o.edges {
		kept := edges[:0]
		for _, edge := range edges {
			if _, remove := deleted[edge.FromRunID]; remove {
				continue
			}
			if _, remove := deleted[edge.ToRunID]; remove {
				continue
			}
			kept = append(kept, edge)
		}
		if len(kept) == 0 {
			delete(o.edges, parentRunID)
			continue
		}
		o.edges[parentRunID] = kept
	}

	filterBus := func(src []AgentBusMessage) []AgentBusMessage {
		kept := src[:0]
		for _, msg := range src {
			if _, remove := deleted[msg.FromRunID]; remove {
				continue
			}
			if _, remove := deleted[msg.ToRunID]; remove {
				continue
			}
			kept = append(kept, msg)
		}
		return kept
	}

	for parentRunID, bus := range o.bus {
		kept := filterBus(bus)
		if len(kept) == 0 {
			delete(o.bus, parentRunID)
			continue
		}
		o.bus[parentRunID] = kept
	}

	for parentRunID, queued := range o.queued {
		kept := filterBus(queued)
		if len(kept) == 0 {
			delete(o.queued, parentRunID)
			continue
		}
		o.queued[parentRunID] = kept
	}
}

func (s *InteractiveService) deleteSessionIDsForRun(runID string, session ProviderSessionState) []string {
	seen := make(map[string]struct{})
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		seen[id] = struct{}{}
	}

	add(session.ProviderSessionID)
	if session.ProviderKey == ProviderKeyCodex || session.ProviderKey == ProviderKeyGrok {
		if logger, ok := s.workflowStore.(TurnLogStore); ok {
			if entries, err := logger.ReadTurnLog(context.Background(), runID); err == nil {
				for _, entry := range entries {
					if session.ProviderKey == ProviderKeyCodex && entry.Kind == turnLogKindCodexSession {
						add(entry.SessionID)
					}
					if session.ProviderKey == ProviderKeyGrok && entry.Kind == turnLogKindGrokSession {
						add(entry.SessionID)
					}
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func terminalFlowLoopStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "done", "blocked", "stopped":
		return true
	default:
		return false
	}
}

func (s *InteractiveService) shouldTreatCodexFlowHubSessionAsSynthetic(rs *interactiveRun) bool {
	if rs == nil || rs.providerKey != ProviderKeyCodex {
		return false
	}
	if !strings.HasPrefix(s.resumeSessionID(rs), "thread-") {
		return false
	}
	if rs.runKind != "workflow" || len(rs.activeFlowNodes) == 0 {
		return false
	}
	logger, ok := s.workflowStore.(TurnLogStore)
	if !ok {
		return true
	}
	entries, err := logger.ReadTurnLog(context.Background(), rs.id)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.Kind == turnLogKindCodexSession && strings.TrimSpace(entry.SessionID) != "" {
			return false
		}
	}
	return true
}

func promptOnlyTranscriptEvents(rawPrompts []string) []ProviderEvent {
	out := make([]ProviderEvent, 0, len(rawPrompts)*2)
	for i, prompt := range rawPrompts {
		prompt = strings.TrimSpace(prompt)
		if prompt == "" || isSystemPrompt(prompt) {
			continue
		}
		out = append(out, ProviderEvent{
			Type:           EventTurnStarted,
			Prompt:         prompt,
			ProviderTurnID: fmt.Sprintf("replay-prompt-%d", i+1),
		})
		out = append(out, ProviderEvent{
			Type:           EventTurnCompleted,
			ProviderTurnID: fmt.Sprintf("replay-prompt-%d", i+1),
		})
	}
	return out
}

func normalizeResumedFlowRun(s *InteractiveService, rs *interactiveRun, st ProviderSessionState) {
	if rs == nil || len(rs.activeFlowNodes) == 0 {
		return
	}
	// V10R P0: root mid-gate must stay Running so resumePendingFlowGate can
	// finish; do not race-cancel after reconstruct schedules the gate.
	if st.PendingFlowGateSettle || rs.pendingFlowGateSettle {
		return
	}
	// V10R3 P0: children with pending gate/approval/question already reconstructed
	// — keep parent non-terminal so the flow can continue.
	if s != nil && s.parentHasLivePendingChildren(rs.id) {
		if rs.status == RunStatusCancelled || rs.status == RunStatusCompleted {
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
		}
		return
	}
	if !resumedFlowRunIncomplete(st) {
		return
	}
	if st.Status == RunStatusFailed {
		rs.autoOrchestrate = false
		return
	}
	rs.status = RunStatusCancelled
	rs.agentStatus = string(RunStatusCancelled)
	rs.autoOrchestrate = false
}

// parentHasLivePendingChildren reports whether parentRunID has an in-memory
// child still waiting on gate settle or a pending approval/question card.
func (s *InteractiveService) parentHasLivePendingChildren(parentRunID string) bool {
	if s == nil || parentRunID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, child := range s.runs {
		if child == nil || child.parentRunID != parentRunID {
			continue
		}
		if child.pendingFlowGateSettle {
			return true
		}
		if child.status == RunStatusWaitingApproval || child.status == RunStatusWaitingQuestion {
			return true
		}
		if child.pendingApprovalID != "" || child.pendingQuestionID != "" {
			return true
		}
		// Durable continuation still to flush (V10R4).
		if strings.TrimSpace(child.pendingResumePrompt) != "" ||
			strings.TrimSpace(child.pendingGateRepromptPrompt) != "" {
			return true
		}
	}
	return false
}

// resumedFlowStepsComplete reports whether the FLOW ITSELF reached a genuine
// terminal completion, for the purpose of defaulting an evidence-less
// step-timeline row (an inline hub node, or -- BUG-320 -- a child whose only
// evidence is a tombstone) to DONE. This is deliberately NOT the same
// question normalizeResumedFlowStatus answers: that function intentionally
// reports "Completed" for a STOPPED loop that later got a plain-chat
// follow-up, so the run's own history badge reads correctly (BUG-308,
// run-33289 "vậy là done fix chưa"). But the flow's STEPS did not all
// genuinely finish just because the chat kept going after Stop -- a coder
// that ran and a reviewer stopped mid-turn is not "every node done" (the
// live repro run-55348/run-55467: hub turn completed normally, but the
// reviewer was interrupted and synthesis never started). Used by BOTH the
// evidence-walk default below AND the transition-log replay's hub-promotion
// check so a Drive-restored run (no local sidecar) and a same-machine
// restart (sidecar present) agree on whether a stopped flow's un-evidenced
// steps are DONE or PENDING.
func resumedFlowStepsComplete(st ProviderSessionState) bool {
	if strings.TrimSpace(st.LoopState.Status) == "stopped" {
		return false
	}
	return !resumedFlowRunIncomplete(st) && strings.TrimSpace(st.LoopState.Status) != "blocked"
}

func resumedFlowRunIncomplete(st ProviderSessionState) bool {
	if terminalFlowLoopStatus(st.LoopState.Status) {
		return false
	}
	if st.Status == RunStatusCompleted &&
		len(st.PendingAgentContext) == 0 &&
		strings.TrimSpace(st.LoopState.Status) != "running" &&
		strings.TrimSpace(st.LoopState.ActiveNode) == "" {
		return false
	}
	if st.Status == RunStatusCompleted &&
		!st.AutoOrchestrate &&
		len(st.PendingAgentContext) == 0 &&
		strings.TrimSpace(st.LoopState.Status) != "running" {
		return false
	}
	return true
}

func resumedChildRunStepStatus(status RunStatus) RuntimeWorkflowStepStatus {
	switch normalizeResumedStatus(status) {
	case RunStatusCompleted:
		return StepStatusDone
	case RunStatusFailed:
		return StepStatusFailed
	case RunStatusCancelled:
		return StepStatusCanceled
	default:
		return StepStatusPending
	}
}

func flowHubHadJoinedReviewNote(st ProviderSessionState) bool {
	if strings.Contains(st.LastPrompt, "[flow-engine joined result note]") {
		return true
	}
	for _, note := range st.PendingAgentContext {
		if strings.Contains(note, "[flow-engine joined result note]") {
			return true
		}
	}
	return false
}

// pendingGateStates reads the per-run approval and question sidecar stores.
// BUG-495: a read error is not an empty gate history — propagating keeps
// resume from promoting a durably waiting gate on an unreadable store.
// Stores without the reader interfaces legitimately have no gate history.
func (s *InteractiveService) pendingGateStates(runID string) ([]ProviderApprovalState, []ProviderQuestionState, error) {
	var approvals []ProviderApprovalState
	var questions []ProviderQuestionState
	if ahr, ok := s.workflowStore.(ApprovalHistoryReader); ok {
		states, err := ahr.ListApprovalsByRun(context.Background(), runID)
		if err != nil {
			return nil, nil, fmt.Errorf("pendingGateStates: approvals unreadable for run %s: %w", runID, err)
		}
		approvals = states
	}
	if qhr, ok := s.workflowStore.(QuestionHistoryReader); ok {
		states, err := qhr.ListQuestionsByRun(context.Background(), runID)
		if err != nil {
			return nil, nil, fmt.Errorf("pendingGateStates: questions unreadable for run %s: %w", runID, err)
		}
		questions = states
	}
	return approvals, questions, nil
}

// childPendingGateNodeIDs returns parent flow node ids whose matching child
// sessions still have a pending approval or question on disk (BUG-288 #22).
// Approvals/questions are keyed by the child's RunID, so parent-scoped list
// APIs never surface them — resume must walk child sessions explicitly.
func (s *InteractiveService) childPendingGateNodeIDs(rs *interactiveRun) ([]string, error) {
	if rs == nil || len(rs.activeFlowNodes) == 0 {
		return nil, nil
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return nil, nil
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		// BUG-491: an unreadable index must not masquerade as "no pending
		// child gates" — resume would promote nodes whose children still
		// wait on approval/question. Propagate so the caller fails closed.
		return nil, fmt.Errorf("childPendingGateNodeIDs: %w", err)
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	ahr, hasAHR := s.workflowStore.(ApprovalHistoryReader)
	qhr, hasQHR := s.workflowStore.(QuestionHistoryReader)
	if !hasAHR && !hasQHR {
		return nil, nil
	}
	out := make([]string, 0)
	seen := make(map[string]bool)
	legacyCohortNodeByRun := s.inferredFlowNodeByLegacyCohort(rs, sessions)
	for _, session := range sessions {
		if session.ParentRunID != rs.id || strings.TrimSpace(session.RunID) == "" {
			continue
		}
		nodeID := matchFlowNodeForSession(rs.activeFlowNodes, session)
		if nodeID == "" {
			nodeID = legacyCohortNodeByRun[session.RunID]
		}
		if nodeID == "" || seen[nodeID] {
			continue
		}
		pending := false
		if hasAHR {
			states, err := ahr.ListApprovalsByRun(context.Background(), session.RunID)
			if err != nil {
				// BUG-495: same contract as the session index — an
				// unreadable child gate store must not read as
				// "no pending gate".
				return nil, fmt.Errorf("childPendingGateNodeIDs: approvals unreadable for run %s: %w", session.RunID, err)
			}
			if hasPendingGate(states, nil) {
				pending = true
			}
		}
		if !pending && hasQHR {
			states, err := qhr.ListQuestionsByRun(context.Background(), session.RunID)
			if err != nil {
				return nil, fmt.Errorf("childPendingGateNodeIDs: questions unreadable for run %s: %w", session.RunID, err)
			}
			if hasPendingGate(nil, states) {
				pending = true
			}
		}
		if pending {
			seen[nodeID] = true
			out = append(out, nodeID)
		}
	}
	return out, nil
}

func matchFlowNodeForSession(nodes []agentpack.FlowNode, session ProviderSessionState) string {
	if label := strings.TrimSpace(session.Label); label != "" {
		for _, node := range nodes {
			if node.ID == label {
				return node.ID
			}
		}
	}
	agentName := strings.TrimSpace(session.AgentName)
	role := strings.TrimSpace(session.Role)
	match := ""
	for _, node := range nodes {
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); !ok || canonical != "agent.delegate" {
			continue
		}
		nodeAgent := flowNodeAgentName(node)
		if nodeAgent == "" {
			continue
		}
		if nodeAgent != agentName && nodeAgent != role {
			continue
		}
		if match != "" {
			return ""
		}
		match = node.ID
	}
	return match
}

func flowCohortSourceNodeID(cohortID string) string {
	const prefix = "flow-auto-"
	rest := strings.TrimPrefix(strings.TrimSpace(cohortID), prefix)
	if rest == cohortID {
		return ""
	}
	idx := strings.LastIndex(rest, "-round-")
	if idx <= 0 {
		return ""
	}
	return rest[:idx]
}

func flowCohortTargetNodeIDs(nodes []agentpack.FlowNode, edges []agentpack.FlowEdge, cohortID string) []string {
	sourceID := flowCohortSourceNodeID(cohortID)
	if sourceID == "" {
		return nil
	}
	delegate := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.delegate" {
			delegate[node.ID] = struct{}{}
		}
	}
	targets := make([]string, 0)
	for _, edge := range edges {
		if edge.From != sourceID || edge.Kind != "forward" {
			continue
		}
		if _, ok := delegate[edge.To]; !ok {
			continue
		}
		targets = append(targets, edge.To)
	}
	return targets
}

func (s *InteractiveService) inferredFlowNodeByLegacyCohort(rs *interactiveRun, sessions []ProviderSessionState) map[string]string {
	groups := make(map[string][]ProviderSessionState)
	for _, session := range sessions {
		if session.ParentRunID != rs.id || strings.TrimSpace(session.Label) != "" || strings.TrimSpace(session.FlowCohortID) == "" {
			continue
		}
		groups[session.FlowCohortID] = append(groups[session.FlowCohortID], session)
	}
	out := make(map[string]string)
	for cohortID, group := range groups {
		targets := flowCohortTargetNodeIDs(rs.activeFlowNodes, rs.activeFlowEdges, cohortID)
		if len(targets) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].StartedAt == group[j].StartedAt {
				return group[i].RunID < group[j].RunID
			}
			return group[i].StartedAt < group[j].StartedAt
		})
		for i, session := range group {
			if i >= len(targets) {
				break
			}
			out[session.RunID] = targets[i]
		}
	}
	return out
}

func (s *InteractiveService) resumedFlowStepRows(rs *interactiveRun, st ProviderSessionState) ([]RuntimeWorkflowStep, error) {
	if len(rs.activeFlowNodes) == 0 {
		return nil, nil
	}
	// BUG-260: a flow can reach a genuinely terminal loop state ("done") even
	// though one of its cohort members individually FAILED — CA-251/BUG-254
	// deliberately let the hub proceed past a partial cohort failure (join only
	// requires completed members; the hub can still declare the round done off
	// the survivors' verdicts). The old code treated "flow reached terminal,
	// non-blocked state" as "every node genuinely succeeded" and short-circuited
	// straight to an unconditional all-DONE fast path, silently overwriting that
	// failed member's own real outcome. flowComplete now only controls the
	// DEFAULT for a node with no session evidence at all (e.g. an inline hub
	// node, or a legacy run with no persisted children) — per-child evidence,
	// including FAILED, set below always wins over that default.
	flowComplete := resumedFlowStepsComplete(st)
	rows := s.flowStepRowsFromNodes(context.Background(), rs.id, rs.activeFlowNodes, StepStatusPending, "")
	byID := make(map[string]*RuntimeWorkflowStep, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	if indexReader, ok := s.workflowStore.(SessionIndexReader); ok {
		// BUG-491: cohort reconstruction from a partial index rebuilds step
		// rows wrongly — fail closed so reconstructRun can retry/surface it.
		sessions, err := indexReader.ListAllProviderSessions(context.Background())
		if err != nil {
			return nil, fmt.Errorf("resumedFlowStepRows: session index unreadable: %w", err)
		}
		{
			legacyCohortNodeByRun := s.inferredFlowNodeByLegacyCohort(rs, sessions)
			for _, session := range sessions {
				if session.ParentRunID != rs.id {
					continue
				}
				nodeID := matchFlowNodeForSession(rs.activeFlowNodes, session)
				if nodeID == "" {
					nodeID = legacyCohortNodeByRun[session.RunID]
				}
				if nodeID == "" {
					continue
				}
				row := byID[nodeID]
				if row == nil {
					continue
				}
				switch resumedChildRunStepStatus(session.Status) {
				case StepStatusDone:
					row.Status = StepStatusDone
				case StepStatusFailed:
					if row.Status != StepStatusDone {
						row.Status = StepStatusFailed
					}
				case StepStatusCanceled:
					if row.Status == StepStatusPending {
						row.Status = StepStatusCanceled
					}
				}
			}
		}
	}
	if hubID := hubInlineNodeID(rs.activeFlowNodes); hubID != "" {
		if hub := byID[hubID]; hub != nil && hub.Status == StepStatusPending {
			switch {
			case flowComplete:
				hub.Status = StepStatusDone
			case strings.TrimSpace(st.LoopState.ActiveNode) == hubID || flowHubHadJoinedReviewNote(st):
				hub.Status = StepStatusCanceled
			}
		}
	}
	if flowComplete {
		// Fallback default for any other node no session evidence matched at all
		// (e.g. a legacy run whose children predate persisted labels) — a node
		// that genuinely failed or was canceled above is never still PENDING
		// here, so this can only promote real gaps, not overwrite real evidence.
		for i := range rows {
			if rows[i].Status == StepStatusPending {
				rows[i].Status = StepStatusDone
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range rows {
		switch rows[i].Status {
		case StepStatusDone:
			rows[i].StartedAt = now
			rows[i].FinishedAt = now
		case StepStatusFailed, StepStatusCanceled, StepStatusSkipped:
			rows[i].FinishedAt = now
		}
	}
	return rows, nil
}

// normalizeResumedStatus maps an in-flight status read back from disk to a
// terminal one. A run that was running / starting cannot still be in flight
// after the owning process exited (server restart): the turn goroutine is gone.
//
// V10R4 P0-01: WaitingApproval / WaitingQuestion are preserved — durable cards
// rehydrate after restart, so canceling them would drop the user decision surface
// and cascade parent cancellation.
//
// Task-239 / BUG-251 follow-up: agent-status values "spawned" and
// "waiting_dependency" are also in-flight (cast through RunStatus when used as
// AgentStatus) and must normalize to cancelled so listAgentRunSummaries disk
// fallback does not leave a permanent spinner.
func normalizeResumedStatus(status RunStatus) RunStatus {
	switch status {
	case RunStatusRunning, RunStatusStarting:
		return RunStatusCancelled
	case RunStatusWaitingApproval, RunStatusWaitingQuestion:
		// Keep — rehydratePendingGatesLocked restores the actionable card.
		return status
	case RunStatus("spawned"), RunStatus("waiting_dependency"):
		return RunStatusCancelled
	default:
		return status
	}
}

func normalizeResumedFlowStatus(st ProviderSessionState) RunStatus {
	// Loop status is authoritative for flow hubs. Do not require ActiveFlowNodes:
	// a completed run may still have empty/stale node lists while LoopState is
	// "done" (run-15827/18200/20332 history showed Running spinner for every
	// non-selected completed chat because Status stayed "running").
	switch strings.TrimSpace(st.LoopState.Status) {
	case "done":
		return RunStatusCompleted
	case "stopped":
		// BUG-308 residual: Stop seals the flow loop only. A hub that later
		// completed a plain-chat follow-up persists status=completed while
		// LoopState stays stopped — history/reopen must show Completed, not
		// Cancelled (run-33289 screenshot after "vậy là done fix chưa").
		switch st.Status {
		case RunStatusCompleted, RunStatusFailed, RunStatusRunning,
			RunStatusWaitingApproval, RunStatusWaitingQuestion:
			return st.Status
		default:
			return RunStatusCancelled
		}
	case "blocked":
		// Awaiting user (escalate/cap) — not a live turn spinner.
		if st.Status == RunStatusFailed || st.Status == RunStatusCancelled {
			return st.Status
		}
		return RunStatus("blocked")
	}
	// V10 P0: gate-pending child must stay Running so resume re-evaluates gate
	// instead of normalizing to cancelled.
	if st.PendingFlowGateSettle {
		return RunStatusRunning
	}
	// Durable continuation intents keep the run non-terminal across restart.
	if strings.TrimSpace(st.PendingResumePrompt) != "" ||
		strings.TrimSpace(st.PendingGateRepromptPrompt) != "" {
		return RunStatusRunning
	}
	return normalizeResumedStatus(st.Status)
}

func (s *InteractiveService) reconstructRun(st ProviderSessionState) (*interactiveRun, *apiErr) {
	return s.reconstructRunInternal(st, false)
}

// reconstructRunDeferred loads a child without auto-scheduling pending gates /
// durable intents so the parent can finish cohortExpected + sibling buffers
// first (V10R4 P0 atomic cohort recovery).
func (s *InteractiveService) reconstructRunDeferred(st ProviderSessionState) (*interactiveRun, *apiErr) {
	return s.reconstructRunInternal(st, true)
}

func (s *InteractiveService) reconstructRunInternal(st ProviderSessionState, deferGate bool) (*interactiveRun, *apiErr) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Preserve the persisted updatedAt — merely opening/viewing a chat must not bump its
	// timestamp. The in-memory rs.updatedAt is what the history list reports, so seeding it
	// with `now` made every opened chat jump to the top with the current date. Only a real
	// turn (emitLocked) should advance updatedAt. Fall back to now if none was stored. (BUG-118)
	updatedAt := strings.TrimSpace(st.UpdatedAt)
	if updatedAt == "" {
		updatedAt = now
	}
	// BUG-289 A7/F-10: normalize agentStatus with the same rules as status so a
	// reconstructed child does not surface a permanent stale "running"/"spawned"
	// badge after resume (BUG-251 class on the live-reconstruct path).
	normalizedStatus := normalizeResumedFlowStatus(st)
	agentStatus := strings.TrimSpace(st.AgentStatus)
	if agentStatus == "" || agentStatus == string(RunStatusRunning) || agentStatus == "spawned" ||
		agentStatus == string(RunStatusWaitingApproval) || agentStatus == string(RunStatusWaitingQuestion) {
		// Align non-terminal / in-flight labels with normalized run status.
		agentStatus = string(normalizedStatus)
	}
	// BUG-455: heal runs persisted before the create-time default — a durable
	// record with no working directory gets the same fallback the provider
	// session path already applies (Runner.workspace), so Go-inline consumers
	// (validate baseline load, captureGitHead, contract inject) do not see ""
	// after a restart and dead-park skipped_no_command.
	restoredWorkspaceCwd := strings.TrimSpace(st.WorkingDirectory)
	if restoredWorkspaceCwd == "" && s.runner != nil {
		restoredWorkspaceCwd = strings.TrimSpace(s.runner.workspace)
	}
	rs := &interactiveRun{
		id:                     st.RunID,
		projectID:              st.ProjectID,
		workflowID:             st.WorkflowID,
		parentRunID:            st.ParentRunID,
		agentName:              st.AgentName,
		label:                  st.Label,
		role:                   st.Role,
		dependsOn:              append([]string(nil), st.DependsOn...),
		agentStatus:            agentStatus,
		modelName:              st.ModelName,
		providerKey:            st.ProviderKey,
		providerSessionID:      st.ProviderSessionID,
		realProviderSessionID:  st.ProviderSessionID,
		lastCodexTurnSessionID: st.ProviderSessionID,
		lastGrokTurnSessionID:  st.ProviderSessionID,
		// BUG-329: mirror Grok — a restored opencode run resumes its real ses_*
		// through the lastOpencodeTurnSessionID fallback in
		// turnResumeProviderSessionID.
		lastOpencodeTurnSessionID: st.ProviderSessionID,
		providerAccountID:         st.ProviderAccountID,
		accountPinned:             st.AccountPinned,
		quotaClaimID:              st.QuotaClaimID,
		workspaceCwd:              restoredWorkspaceCwd,
		worktree:                  worktreeBindingFromSession(st),
		runKind:                   st.RunKind,
		// BUG-330 residual: the handle echo reads rs.chatID/legSeq, but the
		// disk-reconstruction path never restored them — a restarted chat run
		// lost ChatID and routeProviderSwitch never fired again (same symptom
		// as the original bug, restart-only).
		chatID: st.ChatID,
		legSeq: st.LegSeq,
		// BUG-405: same class — legState/legClosedReason/switchFromRunID were
		// persisted but never restored, so every reconstructed leg surfaced
		// legState:"" (timeline lies, switch-provider 409s chat_no_active_leg)
		// and the next persistSessionSnapshot rewrote the durable row with the
		// three fields omitempty-dropped — last-wins data loss.
		legState:        st.LegState,
		legClosedReason: st.LegClosedReason,
		switchFromRunID: st.SwitchFromRunID,
		status:          normalizedStatus,
		createdAt:       st.StartedAt,
		updatedAt:       updatedAt,
		lastPrompt:      st.LastPrompt,
		lastFullPrompt:  st.LastFullPrompt,
		lastMessage:     st.LastMessage,
		sourceMachineID: st.SourceMachineID,
		sourceRunID:     st.SourceRunID,
		restoredFrom:    st.RestoredFrom,
		syncStatus:      st.SyncStatus,
		syncUpdatedAt:   st.SyncUpdatedAt,
		changeType:      st.ChangeType,
		sourceDocID:     st.SourceDocID,
		turnCount:       st.TurnCount,
		// Task-446 T-3: restore the frozen routing-policy snapshot — the run
		// keeps resolving under the policy it was created with.
		quotaRouting: st.QuotaRouting,
		subs:         map[int64]chan ProviderEvent{},
		// BUG-288 R16-P0: restore durable idempotency keys (not empty map).
		idempotency:                     copyStringMap(st.IdempotencyKeys),
		resumedFromDisk:                 true,
		pendingAgentContext:             append([]string(nil), st.PendingAgentContext...),
		pendingFlowGateSettle:           st.PendingFlowGateSettle,
		pendingFlowGateFinalMsg:         st.PendingFlowGateFinalMsg,
		pendingFlowGateOccurredAt:       st.PendingFlowGateOccurredAt,
		pendingFlowGateTurnID:           st.PendingFlowGateTurnID,
		turnStartGitHead:                st.TurnStartGitHead,
		turnStartWorktree:               copyStringMap(st.TurnStartWorktree),
		pendingGateChangedFiles:         append([]string(nil), st.PendingGateChangedFiles...),
		stepID:                          st.StepID,
		lastTurnStepID:                  st.LastTurnStepID,
		lastTurnID:                      st.PendingFlowGateTurnID, // seed for gate materialize
		pendingGateRepromptPrompt:       st.PendingGateRepromptPrompt,
		pendingGateRepromptStepID:       st.PendingGateRepromptStepID,
		hubContinueDelegatedTurnID:      st.HubContinueDelegatedTurnID,
		pendingGateCodePaths:            append([]string(nil), st.PendingGateCodePaths...),
		repromptAttempts:                st.RepromptAttempts,
		pendingResumePrompt:             st.PendingResumePrompt,
		pendingResumeStepID:             st.PendingResumeStepID,
		pendingResumeGen:                st.PendingResumeGen,
		pendingGateRepromptGen:          st.PendingGateRepromptGen,
		pendingResumeDeliveredGen:       st.PendingResumeDeliveredGen,
		pendingGateRepromptDeliveredGen: st.PendingGateRepromptDeliveredGen,
		pendingResumeAcceptedTurn:       st.PendingResumeAcceptedTurn,
		pendingGateRepromptAcceptedTurn: st.PendingGateRepromptAcceptedTurn,
		pendingResumeFailCount:          st.PendingResumeFailCount,
		pendingResumeFailGen:            st.PendingResumeFailGen,
		pendingGateRepromptFailCount:    st.PendingGateRepromptFailCount,
		pendingGateRepromptFailGen:      st.PendingGateRepromptFailGen,
		pendingResumeApprovalID:         st.PendingResumeApprovalID,
		pendingResumeDecision:           st.PendingResumeDecision,
		pendingResumeQuestionChoices:    append([]string(nil), st.PendingResumeQuestionChoices...),
		stopGeneration:                  st.StopGeneration,
		parentStopGenSeen:               st.ParentStopGenSeen,
		intentBlockedKind:               st.IntentBlockedKind,
		intentBlockedReason:             st.IntentBlockedReason,
		intentBlockedAt:                 st.IntentBlockedAt,
		transitionLogDegraded:           st.TransitionLogDegraded,
		transitionLogDegradedAt:         st.TransitionLogDegradedAt,
		transitionLogDegradedReason:     st.TransitionLogDegradedReason,
		suppressAutoGateResume:          deferGate,
		autoOrchestrate:                 st.AutoOrchestrate,
		flowCohortId:                    st.FlowCohortID,
		activeFlowEdges:                 append([]agentpack.FlowEdge(nil), st.ActiveFlowEdges...),
		activeFlowNodes:                 append([]agentpack.FlowNode(nil), st.ActiveFlowNodes...),
		chatSubMode:                     st.ChatSubMode,
		chatFlowRef:                     inferPackFlowRefFromNodes(st.ActiveFlowNodes, st.ChatFlowRef),
		workingMode:                     st.WorkingMode,
		vibeAwaitingLock:                st.VibeAwaitingLock,
		vibeTaskPlan:                    append([]string(nil), st.VibeTaskPlan...),
		vibeCpDocID:                     st.VibeCpDocID,
		vibeRequirementFromNode:         st.VibeRequirementFromNode,
		vibeSprintIndex:                 st.VibeSprintIndex,
		vibeSprintBudget:                st.VibeSprintBudget,
		vibeSprintBoundaryDeclined:      st.VibeSprintBoundaryDeclined,
		vibeLockedCP:                    st.VibeLockedCP,
		vibeLockedSS:                    st.VibeLockedSS,
		vibeLockNodeID:                  st.VibeLockNodeID,
		vibeLockPath:                    st.VibeLockPath,
		vibeCheckpointNode:              st.VibeCheckpointNode,
		vibeCheckpointArtifacts:         append([]string(nil), st.VibeCheckpointArtifacts...),
		vibeTaskIndex:                   st.VibeTaskIndex,
		vibeTaskTotal:                   st.VibeTaskTotal,
		vibeTaskName:                    st.VibeTaskName,
		// BUG-404: restore the debate-parked sprint topology + buffered coder
		// batches — previously RAM-only, so a restart mid-negotiation lost them
		// and the debate_synthesis done verdict settled the whole flow.
		vibeParkedNodes:      append([]agentpack.FlowNode(nil), st.VibeParkedNodes...),
		vibeParkedEdges:      append([]agentpack.FlowEdge(nil), st.VibeParkedEdges...),
		vibeParkedAcceptance: append([]string(nil), st.VibeParkedAcceptance...),
		vibeParkedFlowRef:    st.VibeParkedFlowRef,
		// BUG-478: parked merge card + patch snapshots were RAM-only — a
		// restart dropped every actionable alternate.
		tournamentWinner:            st.TournamentWinner,
		tournamentPatches:           copyStringMap(st.TournamentPatches),
		tournamentAttempt:           st.TournamentAttempt,
		decisionCard:                st.DecisionCard,
		decisionCardChosen:          st.DecisionCardChosen,
		pendingBatchSignatureByStep: copyBatchSignatureMap(st.PendingBatchSignatureByStep),
		flowStartGitHead:            st.FlowStartGitHead,
		pendingRestartRunID:         st.PendingRestartRunID,
		pendingRestartPrompt:        st.PendingRestartPrompt,
		pendingRestartGen:           st.PendingRestartGen,
		flowContextInjected:         st.FlowContextInjected,
		// CP-51 Task-252 (Codex-suggested fix, 2026-07-17): these three durable
		// provenance fields now round-trip through both session-store backends
		// (encoder fix), but reconstruction never restored them onto the new run
		// — so allowedFCPMarkerIDs() saw only rs.id after every restart, silently
		// losing the recorded handoff binding. Restore them here.
		markerProvenanceRunIDs:             append([]string(nil), st.MarkerProvenanceRunIDs...),
		pendingRestartProvenanceRunID:      st.PendingRestartProvenanceRunID,
		pendingGateRepromptProvenanceRunID: st.PendingGateRepromptProvenanceRunID,
		// BUG-299 residual: restore durable YOLO when present; flow force applied below.
		yolo: st.Yolo,
		// BUG-360: restore the cached scout draft so post-restart freeze can
		// parse it after the transient scout child is gone.
		preflightDraftResult:      st.PreflightDraftResult,
		lastFailedDelegateNodeID:  st.LastFailedDelegateNodeID,
		lastEscalatedInlineNodeID: st.LastEscalatedInlineNodeID,
	}
	if rs.idempotency == nil {
		rs.idempotency = map[string]string{}
	}
	// V10R P1: re-engage flow executor when topology was restored (needed for
	// child approval resume to stamp parent step RUNNING after restart).
	if len(rs.activeFlowNodes) > 0 {
		rs.flowEngineDriven = true
	}
	applyVibeCheckpointFromDisk(rs)
	// BUG-299 residual (run-35329): sessionStateOf historically omitted yolo, so
	// rehydrate always left rs.yolo=false. Force Flow/Workflow/flow-engine runs
	// back to true independent of the stored zero value; chat keeps st.Yolo.
	rs.yolo = resolveEffectiveYolo(rs.yolo, rs.runKind, rs.workflowID, rs.flowEngineDriven)
	// V10R4 P1: migrate legacy durable intents that have prompt/step but gen=0
	// (pre-generation sessions). Without this claim rejects forever.
	migratedIntent := false
	if strings.TrimSpace(rs.pendingResumePrompt) != "" &&
		strings.TrimSpace(rs.pendingResumeStepID) != "" &&
		rs.pendingResumeGen == 0 {
		rs.pendingResumeGen = 1
		migratedIntent = true
	}
	if strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" &&
		strings.TrimSpace(rs.pendingGateRepromptStepID) != "" &&
		rs.pendingGateRepromptGen == 0 {
		rs.pendingGateRepromptGen = 1
		migratedIntent = true
	}
	// Restore CP-41 flow events from the sidecar so FindFlowContextPackage,
	// FindAuditDraft etc. work after a process restart. rs is not yet visible to
	// other goroutines here so no lock is needed for the initial population.
	if fes, ok := s.workflowStore.(FlowEventStore); ok {
		evs, loadEvErr := fes.LoadFlowEvents(context.Background(), st.RunID)
		if loadEvErr != nil {
			log.Printf("reconstructRun: LoadFlowEvents runID=%s: %v (resuming with partial CP-41 state)", st.RunID, loadEvErr)
		}
		if len(evs) > 0 {
			for i := range evs {
				rs.seq++
				evs[i].Seq = rs.seq
				rs.events = append(rs.events, evs[i])
			}
			// Restore rs.planContextPackage from the most recent EventFlowContextPackage
			// so injectFlowContextIfCoding skips the rebuild path after restart.
			for i := len(rs.events) - 1; i >= 0; i-- {
				if rs.events[i].Type == EventFlowContextPackage && rs.events[i].FlowContextPackage != nil {
					pkg := *rs.events[i].FlowContextPackage
					rs.planContextPackage = &pkg
					break
				}
			}
			// BUG-289 A4/F-8: restore flowValidationRetryState so max-retry cap
			// survives restart (previously always nil → RetryAttempt reset to 0).
			for i := len(rs.events) - 1; i >= 0; i-- {
				if rs.events[i].Type == EventFlowValidationRetry && rs.events[i].FlowValidationRetryState != nil {
					st := *rs.events[i].FlowValidationRetryState
					rs.flowValidationRetryState = &st
					break
				}
			}
		}
	}
	// Merge persisted question resolution state onto any restored
	// user_question_required events (BUG-StaleQuestion-Restart): the sidecar
	// reload above only has the raw "asked" event — whether it was later
	// answered or expired lives separately in ProviderQuestionState. Stamp the
	// resolved Answer (read-only render, mirrors subscribe()'s reconnect-while-
	// alive behavior) and drop expired questions (no answer to show, and
	// replaying them interactive would let the user submit into an already-
	// expired question and hit a 409).
	if qhr, ok := s.workflowStore.(QuestionHistoryReader); ok {
		states, qErr := qhr.ListQuestionsByRun(context.Background(), st.RunID)
		if qErr != nil {
			log.Printf("reconstructRun: ListQuestionsByRun runID=%s: %v (resuming without question state)", st.RunID, qErr)
		}
		if len(states) > 0 {
			byID := make(map[string]ProviderQuestionState, len(states))
			for _, q := range states {
				byID[q.QuestionID] = q
			}
			filtered := make([]ProviderEvent, 0, len(rs.events))
			for _, ev := range rs.events {
				if ev.Type == EventUserQuestionRequired && ev.QuestionID != "" {
					if q, found := byID[ev.QuestionID]; found {
						switch q.Status {
						case "resolved":
							ev.Answer = append([]string(nil), q.Choice...)
						case "expired":
							continue
						}
					}
				}
				filtered = append(filtered, ev)
			}
			rs.events = filtered
		}
	}
	// Merge persisted approval resolution onto any restored permission_required
	// events (BUG-ApprovalReplay-Restart) — the approval-side twin of the
	// question merge above. The sidecar reload has only the raw "asked" event;
	// whether it was approved/denied/expired lives separately in
	// ProviderApprovalState. Stamp the recorded Decision (read-only render) and
	// drop expired approvals (nothing to show, and replaying them interactive
	// would let the user submit into an already-expired approval and hit a 409).
	//
	// Runs whenever the store can read approval history at all (not only when
	// some state exists), because the else-branch below MUST fire even for a run
	// with an empty approvals.ndjson: reconstructRun is exclusively the
	// after-restart path, so no replayed permission_required can still be
	// live-actionable (its turn goroutine is gone). A record-less event is
	// therefore either a pre-persist-fix resolution (BUG-272 first pass, or a
	// chat created before that landed) or a process that died mid-approval —
	// both must render read-only, never as a fresh interactive prompt that would
	// flip a settled run back to waiting_approval. Stamping a generic "resolved"
	// backfills those old chats (we cannot recover the exact approve/deny, since
	// it was never persisted — new resolutions carry the concrete decision).
	if ahr, ok := s.workflowStore.(ApprovalHistoryReader); ok {
		states, aErr := ahr.ListApprovalsByRun(context.Background(), st.RunID)
		if aErr != nil {
			log.Printf("reconstructRun: ListApprovalsByRun runID=%s: %v (resuming without approval state)", st.RunID, aErr)
		}
		byID := make(map[string]ProviderApprovalState, len(states))
		for _, a := range states {
			byID[a.ApprovalID] = a
		}
		filtered := make([]ProviderEvent, 0, len(rs.events))
		for _, ev := range rs.events {
			if ev.Type == EventPermissionRequired && ev.ApprovalID != "" {
				if a, found := byID[ev.ApprovalID]; found {
					switch a.Status {
					case "resolved":
						// Prefer the concrete approve/deny; fall back to a
						// generic "resolved" so the card still renders
						// read-only when only Status was recorded.
						if strings.TrimSpace(a.Decision) != "" {
							ev.Decision = a.Decision
						} else {
							ev.Decision = "resolved"
						}
					case "expired":
						continue
					}
				} else {
					// No persisted resolution — backfill read-only (see above).
					ev.Decision = "resolved"
				}
			}
			filtered = append(filtered, ev)
		}
		rs.events = filtered
	}
	// Record how many sidecar-origin events are sitting at the front of
	// rs.events so seedTranscriptFromDisk can move them after the real
	// transcript once it loads one (see reorderSidecarPrefixToEnd) — without
	// this, any restored user_question_required (or other CP-41 event)
	// permanently renders above the entire prior conversation, regardless of
	// when it actually happened, because reconstructRun necessarily assigns
	// these events the lowest Seq numbers before seedTranscriptFromDisk ever
	// runs (BUG-StaleQuestion-Restart-Ordering).
	rs.sidecarPrefixCount = int64(len(rs.events))

	s.mu.Lock()
	s.runs[rs.id] = rs
	// V10 P1: rehydrate pending approval/question records so submit is not 404.
	s.rehydratePendingGatesLocked(rs.id)
	s.mu.Unlock()
	// Persist migrated generation tokens before flush so crash mid-resume still
	// has reclaimable gen>0 intents.
	if migratedIntent {
		snap := sessionStateOf(rs)
		if rs.parentRunID == "" {
			snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
		}
		_ = s.persistProviderSession(snap)
	}
	// BUG-232: reseedFlowStepRuntimeForResume (via flowStepRowsFromNodes ->
	// resolveFlowNodeProviderModel) acquires s.mu itself to read the run's
	// baseline posture — it must run AFTER s.mu.Unlock() above, never while
	// still holding the lock, or every resumed flow-engine run deadlocks on
	// InteractiveService's non-reentrant mutex (the whole test suite then
	// hangs until go test's default 10-minute timeout kills it).
	//
	// BUG-170: only re-seed the synthetic chat step for chat runs, mirroring createRun's
	// own runKind branch (interactive_handlers.go). Before this run reached reconstructRun,
	// resume was rejected outright for any non-chat run, so this call was unconditionally
	// chat-shaped and never actually seeded a workflow run's real steps. Now that workflow
	// runs can reach here, seeding this fake single step would blow away the seeder's real
	// per-step list for that run (fakeWorkflowStore.seed replaces wholesale) — workflow runs
	// keep whatever step-runtime state their store already has instead.
	// Vibe (and other chat-mode flows) keep RunKind=chat but carry
	// activeFlowNodes. Seeding the synthetic chat-* row first used to wipe
	// the real timeline (live run-220036: /open showed vibe-cp-ingest and
	// "(no steps)"). Prefer persisted nodes whenever they exist.
	if len(rs.activeFlowNodes) > 0 {
		// BUG-178: the local runner's step-runtime store is in-memory, so a
		// flow run's step list is empty after a server restart and its history
		// timeline showed "No step-runtime data for this run yet". Rebuild it
		// from the persisted flow nodes plus any child/session evidence we still
		// have on disk, so completed nodes remain DONE and in-flight nodes settle
		// to CANCELED after a restart rather than reverting to a misleading all-
		// PENDING/all-DONE display.
		//
		// V10R P1: flowEngineDriven is restored above when ActiveFlowNodes exist
		// so approval/question resume can stamp parent steps RUNNING.
		//
		// Task-239 / T-10 / I-17: when a step-transition log exists, merge it
		// on top of the evidence-walk (last-wins per node that has log lines;
		// nodes never logged keep evidence-walk — legacy no-label merge rule).
		// LoadStepTransitions is I/O and must run outside s.mu (already unlocked).
		rows, rowsErr := s.resumedFlowStepRows(rs, st)
		if rowsErr != nil {
			return nil, newAPIErr(http.StatusBadGateway, "session_index_unavailable", rowsErr.Error())
		}
		// BUG-495: pending approval/question reads feed keepWaitingNodeIDsForResume —
		// a store fault converted to an empty list would promote a durably waiting
		// gate to not-waiting on resume. Fail closed like the session index above.
		pendingApprovals, pendingQuestions, gateErr := s.pendingGateStates(st.RunID)
		if gateErr != nil {
			return nil, newAPIErr(http.StatusBadGateway, "gate_state_unavailable", gateErr.Error())
		}
		// BUG-288 #22 / V9-08: child gates under child RunID.
		childWaiting, childErr := s.childPendingGateNodeIDs(rs)
		if childErr != nil {
			return nil, newAPIErr(http.StatusBadGateway, "session_index_unavailable", childErr.Error())
		}
		keepWaiting := keepWaitingNodeIDsForResume(st, rs.activeFlowNodes, pendingApprovals, pendingQuestions, childWaiting...)
		if tlog, ok := s.workflowStore.(StepTransitionLogStore); ok {
			if lines, loadErr := tlog.LoadStepTransitions(context.Background(), rs.id); loadErr != nil {
				log.Printf("reconstructRun: LoadStepTransitions runID=%s: %v (falling back to evidence-walk)", rs.id, loadErr)
			} else if len(lines) > 0 {
				rows = applyStepTransitionReplay(rows, lines, keepWaiting)
				// Hub precedence when flow genuinely completed (mirrors evidence-walk):
				// hub PENDING → DONE. I-3: never promote FAILED/CANCELED. BUG-320: use
				// the SAME stopped-aware predicate as the evidence-walk default above
				// (resumedFlowStepsComplete), not normalizeResumedFlowStatus -- that
				// function intentionally reports "Completed" for a stopped loop with a
				// later plain-chat follow-up (BUG-308's own history-badge contract),
				// which is a different question from "did the flow's own steps really
				// finish". Without this, a same-machine restart (transition log
				// present) promoted a stopped flow's pending hub to DONE while a
				// Drive-restored copy of the exact same run (no local sidecar) left it
				// PENDING via the evidence-walk default -- two displays disagreeing
				// about the same underlying fact.
				if resumedFlowStepsComplete(st) {
					if hubID := hubInlineNodeID(rs.activeFlowNodes); hubID != "" {
						for i := range rows {
							if rows[i].ID == hubID && rows[i].Status == StepStatusPending {
								rows[i].Status = StepStatusDone
								nowTS := time.Now().UTC().Format(time.RFC3339Nano)
								rows[i].StartedAt = nowTS
								rows[i].FinishedAt = nowTS
							}
						}
					}
				}
			}
		}
		// V9-08: even without transition log, overlay child pending WAITING labels.
		for _, id := range childWaiting {
			for i := range rows {
				if rows[i].ID == id {
					rows[i].Status = StepStatusWaitingUserApr
				}
			}
		}
		s.seedFlowStepRuntimeRows(rs.id, rows)
	} else if st.RunKind == "chat" {
		if seeder, ok := s.workflowStore.(workflowRunSeeder); ok {
			seeder.seed(rs.id, []RuntimeWorkflowStep{{
				ID:               "chat-" + rs.id,
				StepType:         "chat",
				Status:           StepStatusPending,
				RequiresApproval: false,
			}})
		}
	}
	// Restore flow-engine loop state so a restarted or Drive-synced run resumes
	// at the correct round/cap/mode (Task-085 T-4).
	if st.LoopState.Mode != "" || st.LoopState.Cap > 0 || st.LoopState.Round > 0 || strings.TrimSpace(st.LoopState.Status) != "" {
		// BUG-410: a snapshot taken mid-crash can carry a stale blockReason (the
		// hub_stalled watchdog wrote it while the loop status was still
		// "running"). Restoring it verbatim leaves surfaces reading a phantom
		// block on a live loop. BlockReason is block-only; GateReason also
		// carries legitimate pause/stop reasons, so keep it there.
		switch strings.TrimSpace(st.LoopState.Status) {
		case "blocked":
			// A blocked loop owns both reasons.
		case "paused", "stopped":
			st.LoopState.BlockReason = ""
		default:
			st.LoopState.BlockReason = ""
			st.LoopState.GateReason = ""
		}
		s.agentOrchestrator.setLoop(rs.id, st.LoopState)
	}
	// run-63960: if durable loop is already blocked awaiting user, drop stale
	// post-turn gate settle / gate-reprompt so boot does not re-run gate →
	// startTurn → flow_awaiting_user 409 (desktop Failed, no Continue/Stop).
	// Persist the clear so a later restart does not re-arm from disk.
	// Keep pendingGateRepromptGen high-water (run-23820 / clearIntentFieldsLocked).
	if strings.TrimSpace(st.LoopState.Status) == "blocked" {
		s.mu.Lock()
		hadStaleAutoIntent := clearStaleFlowGateIntentsLocked(rs)
		var snap ProviderSessionState
		if hadStaleAutoIntent {
			snap = sessionStateOf(rs)
			if rs.parentRunID == "" {
				snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
			}
		}
		s.mu.Unlock()
		if hadStaleAutoIntent {
			_ = s.persistProviderSession(snap)
		}
	}
	// V10R3 P0: reconstruct pending/cohort children BEFORE normalize so parent
	// is not Cancelled while children still need approval/gate/barrier.
	if rs.parentRunID == "" && len(rs.activeFlowNodes) > 0 {
		if err := s.reconstructPendingChildSessions(rs.id); err != nil {
			return nil, newAPIErr(http.StatusBadGateway, "session_index_unavailable", err.Error())
		}
	}
	// V10R P0: normalize AFTER child reconstruct; skip cancel when children pending.
	normalizeResumedFlowRun(s, rs, st)
	// V10 P0 / V10R4: schedule post-turn gate only after normalize, and only when
	// not suppressed for atomic cohort restore (parent path flushes later).
	// Skipped when loop is blocked (settle already cleared above).
	if rs.pendingFlowGateSettle && !rs.suppressAutoGateResume {
		go s.resumePendingFlowGate(rs.id)
	}
	// V10R4 P1: durable gate-reprompt / approval-resume intents after restart.
	if !rs.suppressAutoGateResume {
		s.flushDurableTurnIntents(rs.id)
	}
	if rs.parentRunID == "" && !rs.suppressAutoGateResume {
		go s.maybeSettleVibeOwnerDebate(rs.id)
		s.healVibeFailedForReopenPark(rs.id)
		// Boundary first: it owns the next decision when a sprint just
		// finished (resume-confirm no-ops while boundary is pending, but
		// the reverse is not true — parking resume first would steal the
		// card and strand the boundary offer).
		// Sprint boundary is memory-only: re-derive it from audit DONE +
		// remaining plan so reopening a boundary-parked run shows the
		// Continue form again (stopped stays stopped; silent-done re-offers
		// unless declined; finished stays done).
		s.maybeReparkVibeSprintBoundary(rs.id)
		// Clamp stale sprint cursor when earlier Task files are still draft
		// (BUG-372: reopen kept chip task 2/3 while Task-904 was draft).
		s.mu.Lock()
		clamped := reconcileVibeSprintCursor(rs)
		s.mu.Unlock()
		if clamped {
			s.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
				s.mu.Lock()
				r := s.runs[rs.id]
				s.mu.Unlock()
				return attachVibeTaskProgressLocked(r, st)
			})
			go s.persistParentSession(rs.id)
		}

		// Task-327/328/329 O-6 order: SS-missing → CP-missing → Task-missing
		// → generic node resume → CP→task_slicer join (cp_writer→done has no
		// forward successor for pendingVibeResumeFromNode).
		// R-TK-D3: call restartVibeIngest directly (not awaiting-gated) so a
		// vibe-sprint reopen after delete SS+CP+Task does not park Resume→tdd.
		switch {
		case s.restartVibeIngestForMissingSS(rs.id):
		case s.restartVibeCpWriterForMissingCP(rs.id):
		case s.restartVibeTaskSlicerForMissingTasks(rs.id):
		default:
			s.maybeParkVibeResumeConfirm(rs.id)
			s.mu.Lock()
			parked := s.runs[rs.id] != nil && s.runs[rs.id].vibeResumeConfirm
			s.mu.Unlock()
			if !parked {
				s.maybeParkVibeCpJoinResume(rs.id)
			}
		}
	}
	return rs, nil
}

// RequeueBlockedIntent is the recovery hook for BUG-288 P1-09: a durable
// resume/reprompt intent that exhausted durableIntentMaxPermanentFails sits
// in an inert, durably-recorded "blocked" state (intentBlockedKind/Reason/At)
// rather than silently vanishing. This clears that state and the per-kind
// fail budget (WITHOUT discarding the underlying prompt/stepID/gen — the
// intent itself is preserved so the retry targets the same continuation) and
// re-invokes flushDurableTurnIntents so a fixed provider/account/config is
// actually retried instead of the run remaining inert forever.
func (s *InteractiveService) RequeueBlockedIntent(runID string) *apiErr {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if rs.intentBlockedKind == "" {
		s.mu.Unlock()
		return newAPIErr(http.StatusConflict, "not_blocked", "no blocked durable intent to requeue")
	}
	kind := rs.intentBlockedKind
	rs.intentBlockedKind = ""
	rs.intentBlockedReason = ""
	rs.intentBlockedAt = ""
	switch kind {
	case "reprompt":
		rs.pendingGateRepromptFailCount = 0
		rs.pendingGateRepromptFailGen = 0
	case "resume":
		rs.pendingResumeFailCount = 0
		rs.pendingResumeFailGen = 0
	}
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		return newAPIErr(http.StatusInternalServerError, "persist_failed", "failed to persist intent requeue: "+err.Error())
	}
	go s.flushDurableTurnIntents(runID)
	return nil
}

// flushDurableTurnIntents schedules startTurn for durable reprompt/resume
// intents after reconstruct (or after cohort restore / turn settle). Only one
// in-flight delivery owns a generation via claimDurableIntentLocked (V10R4).
//
// V10R4 P0-02: DeliveredGen is set ONLY after startTurn accepts a turn (not
// before the call). Pre-call markers caused permanent intent loss on crash
// between persist and startTurn. Duplicate prevention uses:
//   - in-process claim lease
//   - deterministic idempotency key "durable-{kind}-{gen}" on startTurn
//   - after accept: mark DeliveredGen + clear intent in one persist
func (s *InteractiveService) flushDurableTurnIntents(runID string) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// run-1675: while the flow is blocked for a human form, do not flush
	// gate-reprompt / resume intents (startTurn would also reject; skip early).
	loopID := runID
	if rs.parentRunID != "" {
		loopID = rs.parentRunID
	}
	if st := s.agentOrchestrator.loopStateFor(loopID).Status; st == "blocked" || st == "stopped" || st == "done" {
		s.mu.Unlock()
		return
	}
	// Never flush continuation while a durable card is still pending unless we
	// already recorded a decision for that card (reconcile two-write crash).
	if (rs.pendingApprovalID != "" || rs.pendingQuestionID != "") &&
		strings.TrimSpace(rs.pendingResumeDecision) == "" {
		s.mu.Unlock()
		return
	}
	// CP-51 A1: suppress only a same-turn hub gate reprompt (turn identity from
	// the deferred gate turn / last turn — never use the marker as the turn
	// hint, or every later reprompt would be wrongly suppressed).
	if rs.parentRunID == "" && rs.hubContinueDelegatedTurnID != "" &&
		strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" {
		gateTurn := rs.pendingFlowGateTurnID
		if gateTurn == "" {
			gateTurn = rs.lastTurnID
		}
		if gateTurn != "" && clearHubGateRepromptIfContinueDelegatedLocked(rs, gateTurn) {
			snap := sessionStateOf(rs)
			snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
			s.mu.Unlock()
			_ = s.persistProviderSession(snap)
			s.flowDiagLog(runID, "hub_gate_reprompt_suppressed_after_continue",
				"idle flush dropped durable hub gate reprompt after continue-delegate",
				"turn_id", gateTurn,
			)
			return
		}
	}
	// Prefer gate reprompt over generic resume (more specific remediation).
	prompt := strings.TrimSpace(rs.pendingGateRepromptPrompt)
	stepID := strings.TrimSpace(rs.pendingGateRepromptStepID)
	gen := rs.pendingGateRepromptGen
	delivered := rs.pendingGateRepromptDeliveredGen
	acceptedTurn := rs.pendingGateRepromptAcceptedTurn
	failCount, failGen := rs.pendingGateRepromptFailCount, rs.pendingGateRepromptFailGen
	kind := "reprompt"
	if prompt == "" {
		prompt = strings.TrimSpace(rs.pendingResumePrompt)
		stepID = strings.TrimSpace(rs.pendingResumeStepID)
		gen = rs.pendingResumeGen
		delivered = rs.pendingResumeDeliveredGen
		acceptedTurn = rs.pendingResumeAcceptedTurn
		failCount, failGen = rs.pendingResumeFailCount, rs.pendingResumeFailGen
		kind = "resume"
	}
	// CP-51 A1: hub park — do not start a hub gate reprompt while children run.
	// Drop s.mu before shouldParkHubWriteTurn (it re-locks for the child scan).
	if kind == "reprompt" && rs.parentRunID == "" {
		parkRunID := rs.id
		s.mu.Unlock()
		if s.shouldParkHubWriteTurn(parkRunID) {
			return
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil {
			s.mu.Unlock()
			return
		}
		// Re-sample after unlock window (intent may have been cleared).
		prompt = strings.TrimSpace(rs.pendingGateRepromptPrompt)
		stepID = strings.TrimSpace(rs.pendingGateRepromptStepID)
		gen = rs.pendingGateRepromptGen
		delivered = rs.pendingGateRepromptDeliveredGen
		acceptedTurn = rs.pendingGateRepromptAcceptedTurn
		failCount, failGen = rs.pendingGateRepromptFailCount, rs.pendingGateRepromptFailGen
		if prompt == "" || stepID == "" {
			s.mu.Unlock()
			return
		}
	}
	// BUG-288 R15-P0: also flush durable stall-retry on this run (parent).
	if rs.pendingRestartRunID != "" && rs.pendingRestartPrompt != "" {
		parentID := rs.id
		s.mu.Unlock()
		go s.deliverPendingRestart(parentID)
		return
	}
	if prompt == "" || stepID == "" {
		s.mu.Unlock()
		return
	}
	// BUG-288 R13-25: "Consumed" branch below is retained for forward-compat if
	// DeliveredGen/AcceptedTurn are ever written on accept; production today
	// clears intents via clearIntentFieldsLocked after startTurn and relies on
	// claimDurableIntentLocked + gen idempotency instead of marking delivered.
	if delivered == gen && gen != 0 && strings.TrimSpace(acceptedTurn) != "" {
		clearIntentFieldsLocked(rs, kind)
		snap := sessionStateOf(rs)
		s.mu.Unlock()
		_ = s.persistProviderSession(snap)
		return
	}
	// Permanent failure budget for THIS generation only — park with next-attempt
	// is still a gap (P1-09); at least surface blocked state via fail count.
	if failGen == gen && failCount >= durableIntentMaxPermanentFails {
		s.mu.Unlock()
		return
	}
	if !s.claimDurableIntentLocked(rs, kind, prompt, stepID, gen) {
		// BUG-377: the lease may belong to a wedged delivery; arm a bounded
		// re-check so the intent is reclaimed at/after lease expiry instead of
		// stranded until restart.
		claimUntil := rs.intentClaimUntil
		s.mu.Unlock()
		s.rearmDurableIntentIdleCheck(runID, claimUntil)
		return
	}
	s.mu.Unlock()
	// Do NOT persist DeliveredGen before startTurn (P0-02).
	go s.startTurnClearingIntent(runID, stepID, prompt, kind, gen)
}

// clearStaleFlowGateIntentsLocked drops post-turn gate settle + gate-reprompt
// payload when the loop is terminal/blocked and must not auto-reprompt.
// Caller holds s.mu. Preserves pendingGateRepromptGen high-water (run-23820).
// Returns true when any field was non-empty (caller should persist).
func clearStaleFlowGateIntentsLocked(rs *interactiveRun) bool {
	if rs == nil {
		return false
	}
	had := rs.pendingFlowGateSettle ||
		strings.TrimSpace(rs.pendingFlowGateFinalMsg) != "" ||
		strings.TrimSpace(rs.pendingFlowGateTurnID) != "" ||
		strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" ||
		strings.TrimSpace(rs.pendingGateRepromptStepID) != "" ||
		len(rs.pendingGateChangedFiles) > 0
	rs.pendingFlowGateSettle = false
	rs.pendingFlowGateFinalMsg = ""
	rs.pendingFlowGateOccurredAt = ""
	rs.pendingFlowGateTurnID = ""
	rs.pendingGateChangedFiles = nil
	clearIntentFieldsLocked(rs, "reprompt")
	return had
}

// clearIntentFieldsLocked clears one kind of durable intent (caller holds s.mu).
//
// run-23820 / durable multi-reprompt: do NOT zero pending*Gen. Gen is a
// per-run high-water mark for durable idempotency keys
// (`durable-<run>-reprompt-<gen>`). Resetting it to 0 made the next queue
// reuse gen=1, so startTurn short-circuited as a replay of the completed first
// reprompt turn, cleared the new intent, and stranded the child (hub never
// advanced). Prompt/step empty means "no intent"; gen stays so the next
// gate_hook `pendingGateRepromptGen++` yields a fresh key.
func clearIntentFieldsLocked(rs *interactiveRun, kind string) {
	if rs == nil {
		return
	}
	switch kind {
	case "reprompt":
		rs.pendingGateRepromptPrompt = ""
		rs.pendingGateRepromptStepID = ""
		// Keep pendingGateRepromptGen as high-water (do not set 0).
		rs.pendingGateRepromptDeliveredGen = 0
		rs.pendingGateRepromptAcceptedTurn = ""
		rs.pendingGateRepromptFailCount = 0
		rs.pendingGateRepromptFailGen = 0
	case "resume":
		rs.pendingResumePrompt = ""
		rs.pendingResumeStepID = ""
		// Keep pendingResumeGen as high-water (do not set 0).
		rs.pendingResumeDeliveredGen = 0
		rs.pendingResumeAcceptedTurn = ""
		rs.pendingResumeFailCount = 0
		rs.pendingResumeFailGen = 0
		rs.pendingResumeApprovalID = ""
		rs.pendingResumeDecision = ""
	}
}

// claimDurableIntentLocked atomically leases a durable intent generation before
// startTurn so duplicate flush/retry cannot create two turns. Stale leases
// (older than durableIntentLease) are reclaimable after crash-equivalent hang.
// Caller holds s.mu.
func (s *InteractiveService) claimDurableIntentLocked(rs *interactiveRun, kind, prompt, stepID string, gen int64) bool {
	if rs == nil {
		return false
	}
	// gen==0 only after migration failure; reject empty intents.
	if gen == 0 {
		return false
	}
	now := time.Now()
	// Verify intent still matches.
	switch kind {
	case "reprompt":
		if rs.pendingGateRepromptGen != gen ||
			rs.pendingGateRepromptPrompt != prompt ||
			rs.pendingGateRepromptStepID != stepID {
			return false
		}
	case "resume":
		if rs.pendingResumeGen != gen ||
			rs.pendingResumePrompt != prompt ||
			rs.pendingResumeStepID != stepID {
			return false
		}
	case "restart":
		// stepID carries the child run id for stall-retry (BUG-288 R15-P0).
		if rs.pendingRestartGen != gen ||
			rs.pendingRestartPrompt != prompt ||
			rs.pendingRestartRunID != stepID {
			return false
		}
	default:
		return false
	}
	// Another delivery already owns this gen?
	if rs.intentClaimGen == gen && rs.intentClaimKind == kind {
		if now.Before(rs.intentClaimUntil) {
			return false
		}
		// Stale lease — reclaim.
	}
	// Different gen claim in flight for this kind: wait for settle-driven flush.
	if rs.intentClaimGen != 0 && rs.intentClaimKind == kind && rs.intentClaimGen != gen {
		if now.Before(rs.intentClaimUntil) {
			return false
		}
	}
	rs.intentClaimKind = kind
	rs.intentClaimGen = gen
	rs.intentClaimUntil = now.Add(durableIntentLease)
	return true
}

const durableIntentLease = 30 * time.Minute

// durableIntentRearmProbe bounds how long a queued durable reprompt/resume
// intent may sit undelivered while another delivery holds the claim lease.
// BUG-377 (live cp37 run-1): the lease alone only prevents double-dispatch —
// when the owning startTurn goroutine wedges silently (no error, no turn), no
// path re-checks the intent and it strands until the next user turn or a
// restart. A failed claim now arms a one-shot idle re-check: at most this far
// out, or at lease expiry, whichever comes first. Each fired check re-samples
// state, so the chain self-terminates once the intent clears or delivers.
// var (not const) so tests can shrink the probe.
var durableIntentRearmProbe = 15 * time.Second

// rearmDurableIntentIdleCheck schedules a bounded one-shot notifyTurnIdle so a
// queued durable intent whose claim is held by another (possibly wedged)
// delivery is retried instead of stranded. Caller must NOT hold s.mu.
func (s *InteractiveService) rearmDurableIntentIdleCheck(runID string, claimUntil time.Time) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	delay := time.Until(claimUntil)
	if delay <= 0 || delay > durableIntentRearmProbe {
		delay = durableIntentRearmProbe
	}
	time.AfterFunc(delay, func() { s.notifyTurnIdle(runID) })
}

// durableIntentMaxPermanentFails stops auto-retry spam for permanent startTurn
// errors (account/provider/config). Transient conflicts do not count.
const durableIntentMaxPermanentFails = 5

// isPermanentStartTurnError classifies API errors that should not busy-retry
// every idle notify (V10R4 P1).
func isPermanentStartTurnError(e *apiErr) bool {
	if e == nil {
		return false
	}
	switch e.code {
	case "turn_in_progress", "gate_in_progress", "awaiting_user",
		// CP-51 A1: hub park while children write is temporary — flush again when idle.
		"hub_parked", "flow_awaiting_user":
		return false
	case "flow_stopped":
		// Stop is permanent until user resumes the loop.
		return true
	case "provider_account_changed", "provider_unavailable", "account_not_signed_in",
		"account_unavailable", "run_not_found", "invalid_request", "session_unavailable":
		return true
	default:
		// Unknown codes: treat as permanent after classification caution —
		// prefer not to spin forever.
		return true
	}
}

// releaseDurableIntentClaimLocked drops an in-process lease without clearing
// the durable intent (failed start / conflict). Caller holds s.mu.
func releaseDurableIntentClaimLocked(rs *interactiveRun, kind string, gen int64) {
	if rs == nil {
		return
	}
	if rs.intentClaimKind == kind && rs.intentClaimGen == gen {
		rs.intentClaimKind = ""
		rs.intentClaimGen = 0
		rs.intentClaimUntil = time.Time{}
	}
}

// startTurnClearingIntent claims the generation (if not already leased) then
// startTurn with a deterministic idempotency key. On success records
// DeliveredGen + AcceptedTurn and clears the intent. Never marks delivered
// before the provider call (V10R4 P0-02).
func (s *InteractiveService) startTurnClearingIntent(runID, stepID, prompt, kind string, gen int64) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// Direct callers (approval/question) may not have claimed yet.
	owned := rs.intentClaimKind == kind && rs.intentClaimGen == gen && time.Now().Before(rs.intentClaimUntil)
	if !owned {
		if !s.claimDurableIntentLocked(rs, kind, prompt, stepID, gen) {
			// BUG-377: another delivery owns this gen and may be wedged;
			// re-check at/after its lease expiry so the intent cannot strand.
			claimUntil := rs.intentClaimUntil
			s.mu.Unlock()
			s.rearmDurableIntentIdleCheck(runID, claimUntil)
			return
		}
	}
	s.mu.Unlock()

	// Deterministic key so a crash after accept + restart cannot open a second
	// provider turn for the same intent generation.
	// BUG-288 R18-1: zero-pad gen for stable ordering in durable snapshots.
	idem := fmt.Sprintf("durable-%s-%s-%020d", runID, kind, gen)
	scenario := ""
	if kind == "reprompt" {
		// BUG-391: mark the delivery so startTurn does not zero the reprompt
		// counter — the cap bounds the reprompt LOOP across turns.
		scenario = scenarioGateReprompt
	}
	turnID, apiErr := s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, scenario, idem)
	if apiErr != nil {
		log.Printf("[resume-intent] startTurn failed run=%s kind=%s gen=%d code=%s: %s",
			runID, kind, gen, apiErr.code, apiErr.msg)
		s.mu.Lock()
		if r := s.runs[runID]; r != nil {
			releaseDurableIntentClaimLocked(r, kind, gen)
			if isPermanentStartTurnError(apiErr) {
				// Durable per-generation fail budget.
				if kind == "reprompt" {
					if r.pendingGateRepromptFailGen != gen {
						r.pendingGateRepromptFailGen = gen
						r.pendingGateRepromptFailCount = 0
					}
					r.pendingGateRepromptFailCount++
					if r.pendingGateRepromptFailCount >= durableIntentMaxPermanentFails {
						log.Printf("[resume-intent] permanent fail budget exhausted run=%s kind=%s gen=%d",
							runID, kind, gen)
						// BUG-288 P1-09: durably record the exhausted/blocked state
						// instead of just returning — RequeueBlockedIntent is the
						// recovery hook once the underlying condition is fixed.
						r.intentBlockedKind = kind
						r.intentBlockedReason = apiErr.msg
						r.intentBlockedAt = time.Now().UTC().Format(time.RFC3339Nano)
						failSnap := sessionStateOf(r)
						s.mu.Unlock()
						_ = s.persistProviderSession(failSnap)
						return
					}
					failN := r.pendingGateRepromptFailCount
					failSnap := sessionStateOf(r)
					s.mu.Unlock()
					_ = s.persistProviderSession(failSnap)
					delay := time.Duration(failN*failN) * time.Second
					if delay > 30*time.Second {
						delay = 30 * time.Second
					}
					go func() {
						time.Sleep(delay)
						s.notifyTurnIdle(runID)
					}()
					return
				}
				if r.pendingResumeFailGen != gen {
					r.pendingResumeFailGen = gen
					r.pendingResumeFailCount = 0
				}
				r.pendingResumeFailCount++
				if r.pendingResumeFailCount >= durableIntentMaxPermanentFails {
					log.Printf("[resume-intent] permanent fail budget exhausted run=%s kind=%s gen=%d",
						runID, kind, gen)
					// BUG-288 P1-09: see the mirrored "reprompt" comment above.
					r.intentBlockedKind = kind
					r.intentBlockedReason = apiErr.msg
					r.intentBlockedAt = time.Now().UTC().Format(time.RFC3339Nano)
					failSnap := sessionStateOf(r)
					s.mu.Unlock()
					_ = s.persistProviderSession(failSnap)
					return
				}
				failN := r.pendingResumeFailCount
				failSnap := sessionStateOf(r)
				s.mu.Unlock()
				_ = s.persistProviderSession(failSnap)
				delay := time.Duration(failN*failN) * time.Second
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				go func() {
					time.Sleep(delay)
					s.notifyTurnIdle(runID)
				}()
				return
			}
			// Transient: release claim; re-arm on idle.
			transSnap := sessionStateOf(r)
			s.mu.Unlock()
			_ = s.persistProviderSession(transSnap)
			return
		}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	rs = s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// BUG-288 R20-1: clear outer intent only when recovery can observe a live
	// or terminal turn for turnID. startTurn may return a reused turnID for an
	// incomplete durable key only after re-launch; ghost launch-ack alone must
	// not drop pending resume/reprompt.
	cleared := false
	if durableIntentClearOK(rs, turnID) {
		if kind == "reprompt" &&
			rs.pendingGateRepromptGen == gen &&
			rs.pendingGateRepromptPrompt == prompt &&
			rs.pendingGateRepromptStepID == stepID {
			clearIntentFieldsLocked(rs, "reprompt")
			cleared = true
		}
		if kind == "resume" &&
			rs.pendingResumeGen == gen &&
			rs.pendingResumePrompt == prompt &&
			rs.pendingResumeStepID == stepID {
			clearIntentFieldsLocked(rs, "resume")
			cleared = true
		}
	} else {
		log.Printf("[resume-intent] startTurn returned turn=%s but not clear-safe; keeping durable intent run=%s kind=%s gen=%d",
			turnID, runID, kind, gen)
	}
	releaseDurableIntentClaimLocked(rs, kind, gen)
	snap := sessionStateOf(rs)
	if rs.parentRunID == "" {
		snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
	}
	s.mu.Unlock()
	if cleared {
		if err := s.persistProviderSession(snap); err != nil {
			log.Printf("[resume-intent] persist after accept failed run=%s: %v", runID, err)
		}
	}
}

// notifyTurnIdle re-flushes durable intents after a turn or gate becomes idle
// so conflicted deliveries are not stranded for process restart (V10R4 P1).
//
// BUG-289 H5/F-5: also drains pendingHubReinvoke after the post-turn gate
// releases turnInFlight. runTurn's pre-gate drain can re-defer during the gate
// window; without this second drain the notify reinvoke is stranded while the
// loop stays "running".
func (s *InteractiveService) notifyTurnIdle(runID string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// Only when no turn/gate is active.
	// BUG-354 P2-R2 (sub-agent review F2, run-540927): the same ownership rule
	// as the hub watchdog (CA-742 F1) — V9-03 holds turnInFlight true for the
	// ENTIRE post-turn gate window, so an armed gate owns the busy signal and a
	// stale (>postTurnGateBusyBound) gate must not keep the pending hub
	// reinvoke stranded forever. turnInFlight without an armed gate stays busy
	// (a live provider turn is real activity); stale settle without a live gate
	// is not busy (hub-level H-C contract, run-1618).
	gateLive := gateCancelLive(rs.postTurnGateStartedAt, rs.postTurnGateCancel)
	busy := (rs.turnInFlight && rs.postTurnGateCancel == nil) || gateLive
	hasIntent := strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" ||
		strings.TrimSpace(rs.pendingResumePrompt) != ""
	// Hub reinvoke drain (mirror runTurn :5280-5301) when idle.
	pendingHubReinvoke := !busy && rs.parentRunID == "" && rs.pendingHubReinvoke
	pendingHubReinvokePrompt := ""
	if pendingHubReinvoke {
		rs.pendingHubReinvoke = false
		pendingHubReinvokePrompt = rs.pendingHubReinvokePrompt
		rs.pendingHubReinvokePrompt = ""
		touchHubProgressLocked(rs)
	}
	s.mu.Unlock()

	if pendingHubReinvoke {
		if pendingHubReinvokePrompt != "" {
			go s.maybeAutoReinvokeHubWithPrompt(runID, pendingHubReinvokePrompt)
		} else {
			go s.maybeAutoReinvokeHub(runID)
		}
	}
	if busy || !hasIntent {
		if !busy {
			s.maybeScheduleHubStallCheck(runID)
		}
		// CP-51 A1: when a child becomes idle, try the parent hub's parked
		// gate-reprompt / resume intents (they were deferred while children ran).
		s.mu.Lock()
		parentID := ""
		if r := s.runs[runID]; r != nil {
			parentID = r.parentRunID
		}
		s.mu.Unlock()
		if parentID != "" && !s.hasActiveFlowChild(parentID) {
			go s.flushDurableTurnIntents(parentID)
		}
		return
	}
	go s.flushDurableTurnIntents(runID)
	s.maybeScheduleHubStallCheck(runID)
}

// reconstructPendingChildSessions loads child sessions under parentRunID that
// still need live reconstruction after parent resume: pending post-turn gate,
// pending approval/question cards, and full reviewer cohorts (V10R / V10R3 P0).
// Idempotent — skips children already in s.runs.
func (s *InteractiveService) reconstructPendingChildSessions(parentRunID string) error {
	if strings.TrimSpace(parentRunID) == "" {
		return nil
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return nil
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		// BUG-491: unreadable index must not masquerade as "no children" —
		// the parent would normalize past pending gates/cohorts with the
		// persisted children silently orphaned. Propagate; caller aborts.
		return fmt.Errorf("reconstructPendingChildSessions: %w", err)
	}
	if len(sessions) == 0 {
		return nil
	}
	ahr, hasAHR := s.workflowStore.(ApprovalHistoryReader)
	qhr, hasQHR := s.workflowStore.(QuestionHistoryReader)

	children := make([]ProviderSessionState, 0)
	for _, session := range sessions {
		if session.ParentRunID != parentRunID || strings.TrimSpace(session.RunID) == "" {
			continue
		}
		children = append(children, session)
	}
	if len(children) == 0 {
		return nil
	}

	// Cohort recovery: decide liveness from PERSISTED pre-normalization state.
	// normalizeResumedFlowStatus maps Running→Cancelled; if we consult only the
	// normalized status, a crash mid-review leaves incomplete cohorts forever
	// (completed siblings never loaded, expected too small) — V10R4 P0.
	cohortNeed := map[string]bool{}
	for _, session := range children {
		cid := strings.TrimSpace(session.FlowCohortID)
		if cid == "" {
			continue
		}
		if sessionIsCohortLivePersisted(session) {
			cohortNeed[cid] = true
			continue
		}
		if hasAHR {
			if states, aerr := ahr.ListApprovalsByRun(context.Background(), session.RunID); aerr == nil {
				for _, a := range states {
					if strings.EqualFold(strings.TrimSpace(a.Status), "pending") {
						cohortNeed[cid] = true
						break
					}
				}
			}
		}
		if hasQHR {
			if states, qerr := qhr.ListQuestionsByRun(context.Background(), session.RunID); qerr == nil {
				for _, q := range states {
					if strings.EqualFold(strings.TrimSpace(q.Status), "pending") {
						cohortNeed[cid] = true
						break
					}
				}
			}
		}
	}

	needSession := func(session ProviderSessionState) bool {
		if session.PendingFlowGateSettle {
			return true
		}
		if cid := strings.TrimSpace(session.FlowCohortID); cid != "" && cohortNeed[cid] {
			return true
		}
		if hasAHR {
			if states, aerr := ahr.ListApprovalsByRun(context.Background(), session.RunID); aerr == nil {
				for _, a := range states {
					if strings.EqualFold(strings.TrimSpace(a.Status), "pending") {
						return true
					}
				}
			}
		}
		if hasQHR {
			if states, qerr := qhr.ListQuestionsByRun(context.Background(), session.RunID); qerr == nil {
				for _, q := range states {
					if strings.EqualFold(strings.TrimSpace(q.Status), "pending") {
						return true
					}
				}
			}
		}
		return false
	}

	// Also recover durable continuation intents (approval resume / gate reprompt).
	needSessionWithIntent := func(session ProviderSessionState) bool {
		if needSession(session) {
			return true
		}
		return strings.TrimSpace(session.PendingResumePrompt) != "" ||
			strings.TrimSpace(session.PendingGateRepromptPrompt) != ""
	}

	// Group cohorts for expected-count restore after all members are loaded.
	// V10R4 P0: suppress gate resume on children until preRegister+buffer complete.
	cohortMembers := map[string][]ProviderSessionState{}
	loadedIDs := make([]string, 0)
	for _, session := range children {
		if !needSessionWithIntent(session) {
			continue
		}
		s.mu.Lock()
		already := s.runs[session.RunID] != nil
		s.mu.Unlock()
		if !already {
			// Mark suppress before reconstruct schedules anything — set via
			// reconstructing with a flag after insert by patching immediately.
			// reconstructRun will schedule gate unless suppressAutoGateResume is set
			// on the run before the schedule line. We inject it by pre-registering
			// a placeholder? Cleaner: pass through a package-level? No.
			// Use reconstructRun then cancel any premature schedule by setting
			// suppress before the go resumes — racey.
			// Better: set suppress on session via temporary field during reconstruct.
			// Implement: reconstructDeferredGate(session).
			if _, apiErr := s.reconstructRunDeferred(session); apiErr != nil {
				log.Printf("reconstructPendingChildSessions: child %s: %v", session.RunID, apiErr)
				continue
			}
		} else {
			s.mu.Lock()
			if child := s.runs[session.RunID]; child != nil {
				child.suppressAutoGateResume = true
			}
			s.mu.Unlock()
		}
		loadedIDs = append(loadedIDs, session.RunID)
		// Re-register parent→child edge for graph/release paths.
		s.agentOrchestrator.registerChild(parentRunID, session.RunID)
		if cid := strings.TrimSpace(session.FlowCohortID); cid != "" {
			cohortMembers[cid] = append(cohortMembers[cid], session)
		}
	}

	// Rebuild cohortExpected + buffer completed/cancelled sibling results so the
	// barrier can complete when remaining members finish their pending gate.
	for cid, members := range cohortMembers {
		if len(members) == 0 {
			continue
		}
		s.agentOrchestrator.preRegisterCohort(parentRunID, cid, len(members))
		for _, session := range members {
			s.mu.Lock()
			child := s.runs[session.RunID]
			s.mu.Unlock()
			if child == nil {
				continue
			}
			// Still mid-gate, waiting user, or durable continuation — settle
			// will append the only authoritative result later (V10R4 P0:
			// never buffer resumable children as failed).
			if child.pendingFlowGateSettle ||
				child.status == RunStatusWaitingApproval ||
				child.status == RunStatusWaitingQuestion ||
				strings.TrimSpace(child.pendingResumePrompt) != "" ||
				strings.TrimSpace(child.pendingGateRepromptPrompt) != "" ||
				strings.TrimSpace(session.PendingResumePrompt) != "" ||
				strings.TrimSpace(session.PendingGateRepromptPrompt) != "" {
				continue
			}
			status := "completed"
			finalMsg := session.LastMessage
			switch child.status {
			case RunStatusFailed:
				status = "failed"
			case RunStatusCancelled:
				// Distinguish stop/restart cancel from failure (Task-241 matrix).
				status = "cancelled"
			case RunStatusCompleted:
				status = "completed"
			default:
				// In-flight at kill with no durable intent → failed so barrier
				// is not stuck forever.
				status = "failed"
			}
			s.agentOrchestrator.appendCohortResult(parentRunID, cid, cohortEntry{
				Label:        firstNonEmptyResumeValue(child.label, session.Label, session.AgentName),
				Provider:     string(child.providerKey),
				FinalMessage: truncateDisplayField(finalMsg, 1500),
				Status:       status,
			})
		}
		// V10R4 P0: if recovery itself completed the cohort (all members
		// terminal, no pending gate/intent), drain + reinvoke hub now — there
		// will be no further child event to trigger synthesis.
		if s.agentOrchestrator.cohortComplete(parentRunID, cid) {
			s.joinRecoveredCohort(parentRunID, cid)
		}
	}

	// Atomic flush: only now allow pending gates / durable intents to run.
	for _, id := range loadedIDs {
		s.mu.Lock()
		if child := s.runs[id]; child != nil {
			child.suppressAutoGateResume = false
			pendingGate := child.pendingFlowGateSettle
			s.mu.Unlock()
			if pendingGate {
				go s.resumePendingFlowGate(id)
			}
			s.flushDurableTurnIntents(id)
		} else {
			s.mu.Unlock()
		}
	}
	return nil
}

// sessionIsCohortLivePersisted reports whether a disk session indicates the
// member was still live at crash (before Running→Cancelled normalization).
func sessionIsCohortLivePersisted(session ProviderSessionState) bool {
	if session.PendingFlowGateSettle {
		return true
	}
	if strings.TrimSpace(session.PendingResumePrompt) != "" ||
		strings.TrimSpace(session.PendingGateRepromptPrompt) != "" {
		return true
	}
	switch session.Status {
	case RunStatusRunning, RunStatusStarting, RunStatusWaitingApproval, RunStatusWaitingQuestion:
		return true
	case RunStatus("spawned"), RunStatus("waiting_dependency"):
		return true
	default:
		return false
	}
}

// joinRecoveredCohort drains a cohort that became complete during parent resume
// reconstruction and reinvokes the hub with the joined note (V10R4 P0).
func (s *InteractiveService) joinRecoveredCohort(parentRunID, cohortID string) {
	entries := s.agentOrchestrator.drainCohort(parentRunID, cohortID)
	if len(entries) == 0 {
		return
	}
	round := s.agentOrchestrator.loopStateFor(parentRunID).Round
	note := buildCohortNote(parentRunID, cohortID, entries, round)
	s.appendPendingAgentContext(parentRunID, note)
	s.mu.Lock()
	if parent := s.runs[parentRunID]; parent != nil {
		parent.lastCohortNote = note
	}
	s.mu.Unlock()
	if s.isFlowEngineDriven(parentRunID) {
		for _, e := range entries {
			if e.Label == "" {
				continue
			}
			switch e.Status {
			case "completed":
				s.setFlowStepStatus(context.Background(), parentRunID, e.Label, StepStatusDone)
			case "failed":
				s.setFlowStepStatus(context.Background(), parentRunID, e.Label, StepStatusFailed)
			case "cancelled":
				s.setFlowStepStatus(context.Background(), parentRunID, e.Label, StepStatusCanceled)
			}
		}
		if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" && s.loopIsAdvancing(parentRunID) {
			s.setFlowStepStatus(context.Background(), parentRunID, hubID, StepStatusRunning)
		}
	}
	go s.maybeAutoReinvokeHubWithNote(parentRunID, note)
}

func (s *InteractiveService) resumedParentAgentAnnotations(parentRunID string) []ProviderEvent {
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok || strings.TrimSpace(parentRunID) == "" {
		return nil
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		// BUG-491: annotation restore is best-effort enrichment, but the
		// blind read must be visible in the log.
		log.Printf("resumedParentAgentAnnotations: session index unreadable for parent %s: %v", parentRunID, err)
		return nil
	}
	type childSession struct {
		runID       string
		agentName   string
		lastMessage string
		startedAt   string
		updatedAt   string
		completed   bool
		activations int // lifecycle:reinvoke may run several turns on one run id
	}
	children := make([]childSession, 0)
	seen := make(map[string]struct{})
	for _, session := range sessions {
		if session.ParentRunID != parentRunID || strings.TrimSpace(session.RunID) == "" {
			continue
		}
		if _, exists := seen[session.RunID]; exists {
			continue
		}
		seen[session.RunID] = struct{}{}
		children = append(children, childSession{
			runID: session.RunID,
			// Prefer flow node label (grok-coder / my-reviewer) so main-chat cards
			// match live (AgentTimelineCard uses label). AgentName alone is the
			// role slug "coder" / "reviewer" — wrong after restart (run-12613).
			agentName:   firstNonEmptyResumeValue(session.Label, session.AgentName, session.Role, "agent"),
			lastMessage: strings.TrimSpace(session.LastMessage),
			startedAt:   session.StartedAt,
			updatedAt:   session.UpdatedAt,
			// BUG-294: only a genuinely completed child gets the result annotation
			// (which renders the "— completed" suffix on the parent's agent card).
			// This mirrors the LIVE emit condition exactly (interactive_service.go:
			// EventAgentResultInjected fires only for completion.status ==
			// RunStatusCompleted). A child killed mid-turn persists a non-empty
			// LastMessage but status "running" (normalized to cancelled on resume);
			// annotating it produced a card reading "cancelled … — completed".
			completed:   session.Status == RunStatusCompleted,
			activations: resumeChildActivationCount(s.workflowStore, session),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].startedAt == children[j].startedAt {
			return children[i].runID < children[j].runID
		}
		return children[i].startedAt < children[j].startedAt
	})
	// Peer start times (one per child run) — used to park reinvoke activations in
	// the wall-clock gap between Review Loop rounds (after R0 reviewers, before R1).
	peerStarts := make([]string, 0, len(children))
	for _, child := range children {
		if strings.TrimSpace(child.startedAt) != "" {
			peerStarts = append(peerStarts, child.startedAt)
		}
	}
	sort.Strings(peerStarts)

	// How many distinct child runs claim each label. A reinvoke-lifecycle node
	// (Claude review-loop's my-coder / my-reviewer-claude) keeps ONE run id
	// across every round, so its label is claimed by exactly one entry here —
	// safe to expand every log activation onto that child. A spawn-lifecycle
	// node (Codex reviewer_correctness, fresh run id each round) has several
	// claimants; each child is paired with one log activation in startedAt order.
	labelCounts := make(map[string]int, len(children))
	for _, child := range children {
		labelCounts[child.agentName]++
	}

	// Durable authority (Terra / CA-412 redesign): parent step-transition log
	// append order + synthesis RUNNING boundaries. Wall-clock is not used to
	// assign rounds when this log is present.
	var stepActsByNode map[string][]stepNodeActivation
	var spawnActCursor map[string]int
	if store, ok := s.workflowStore.(StepTransitionLogStore); ok {
		if lines, err := store.LoadStepTransitions(context.Background(), parentRunID); err == nil && len(lines) > 0 {
			allActs := stepActivationsFromOrderedLog(lines)
			stepActsByNode = make(map[string][]stepNodeActivation, len(allActs))
			for _, act := range allActs {
				stepActsByNode[act.nodeID] = append(stepActsByNode[act.nodeID], act)
			}
			spawnActCursor = make(map[string]int, len(stepActsByNode))
		}
	}

	out := make([]ProviderEvent, 0, len(children)*4)
	for _, child := range children {
		var acts []stepNodeActivation
		if stepActsByNode != nil {
			nodeActs := stepActsByNode[child.agentName]
			if labelCounts[child.agentName] == 1 {
				// Reinvoke: one run owns every activation of this label.
				acts = nodeActs
			} else if len(nodeActs) > 0 {
				// Spawn-lifecycle: one child run ↔ one log activation (by start order).
				idx := spawnActCursor[child.agentName]
				if idx < len(nodeActs) {
					acts = []stepNodeActivation{nodeActs[idx]}
					spawnActCursor[child.agentName] = idx + 1
				}
			}
		}

		if len(acts) > 0 {
			for i, act := range acts {
				spawnAt := firstNonEmptyResumeValue(act.runningAt, child.startedAt)
				resultAt := firstNonEmptyResumeValue(act.doneAt, child.updatedAt, spawnAt)
				spawnID := fmt.Sprintf("resume-spawn-%s-%d", child.runID, i)
				out = append(out, ProviderEvent{
					ID:            spawnID,
					Type:          EventAgentSpawnedByUser,
					AgentName:     child.agentName,
					ChildRunID:    child.runID,
					OccurredAt:    spawnAt,
					ResumeDurable: true,
					ResumeCohort:  act.cohort,
					ResumeLogOrd:  act.ord,
				})
				if !child.completed {
					continue
				}
				finalMsg := child.lastMessage
				if i < len(acts)-1 {
					finalMsg = firstNonEmptyResumeValue(finalMsg, "completed")
					if child.lastMessage != "" {
						finalMsg = "completed"
					}
				}
				if finalMsg == "" {
					continue
				}
				out = append(out, ProviderEvent{
					ID:            fmt.Sprintf("resume-result-%s-%d", child.runID, i),
					Type:          EventAgentResultInjected,
					AgentName:     child.agentName,
					ChildRunID:    child.runID,
					FinalMessage:  finalMsg,
					OccurredAt:    resultAt,
					ResumeDurable: true,
					ResumeCohort:  act.cohort,
					ResumeLogOrd:  act.ord,
				})
			}
			continue
		}

		// Legacy fallback: no step-transition sidecar (older fixtures / unit tests).
		// Keep pre-CA-412 wave parking so no-sidecar resume-order tests stay green.
		activations := child.activations
		if activations < 1 {
			activations = 1
		}
		if activations > 1 {
			if waves := peerStartWaveTimes(peerStarts, child.startedAt, 45*time.Second); len(waves) > 0 && activations > len(waves) {
				activations = len(waves)
			}
		}
		times := resumeActivationTimestamps(child.startedAt, child.updatedAt, activations, peerStarts, child.startedAt)
		for i := 0; i < activations; i++ {
			spawnID := fmt.Sprintf("resume-spawn-%s-%d", child.runID, i)
			spawnAt, resultAt := times[i][0], times[i][1]
			out = append(out, ProviderEvent{
				ID:         spawnID,
				Type:       EventAgentSpawnedByUser,
				AgentName:  child.agentName,
				ChildRunID: child.runID,
				OccurredAt: spawnAt,
			})
			if !child.completed {
				continue
			}
			finalMsg := child.lastMessage
			if i < activations-1 {
				finalMsg = firstNonEmptyResumeValue(finalMsg, "completed")
				if child.lastMessage != "" {
					finalMsg = "completed"
				}
			}
			if finalMsg == "" {
				continue
			}
			out = append(out, ProviderEvent{
				ID:           fmt.Sprintf("resume-result-%s-%d", child.runID, i),
				Type:         EventAgentResultInjected,
				AgentName:    child.agentName,
				ChildRunID:   child.runID,
				FinalMessage: finalMsg,
				OccurredAt:   resultAt,
			})
		}
	}
	return out
}

// resumeChildActivationCount is how many main-chat agent cards a restored child
// should produce. lifecycle:reinvoke (Review Loop coder) keeps one run id across
// rounds — TurnCount / turn-log prompts > 1 means round-2+ must get another card.
func resumeChildActivationCount(store WorkflowStore, session ProviderSessionState) int {
	n := session.TurnCount
	if logger, ok := store.(TurnLogStore); ok {
		if entries, err := logger.ReadTurnLog(context.Background(), session.RunID); err == nil {
			prompts := 0
			for _, e := range entries {
				if e.Kind == turnLogKindPrompt && strings.TrimSpace(e.Prompt) != "" {
					prompts++
				}
			}
			if prompts > n {
				n = prompts
			}
		}
	}
	if n < 1 {
		return 1
	}
	return n
}

// stepNodeActivation is one RUNNING→terminal window for a flow agent node,
// read from the durable step-transition sidecar (Task-239).
type stepNodeActivation struct {
	nodeID    string
	runningAt string
	doneAt    string
	// cohort is the count of synthesis-node RUNNING transitions that precede
	// this activation in the parent step-transition log (append order). It is
	// the durable round boundary — not wall-clock spacing (Terra REWORK_DESIGN).
	cohort int
	// ord is the global append order among agent activations in the same log.
	ord int
}

// isSynthesisStepNode reports whether nodeID is a hub synthesis step (boundary
// between Review Loop rounds). Matches "synthesis", "grok-synthesis", etc.
func isSynthesisStepNode(nodeID string) bool {
	n := strings.ToLower(strings.TrimSpace(nodeID))
	if n == "" {
		return false
	}
	return n == "synthesis" || strings.Contains(n, "synthesis")
}

// stepActivationsFromOrderedLog walks the full parent step-transition sidecar
// in append order and returns every agent-node RUNNING→terminal activation with
// durable cohort/ord. Synthesis RUNNING lines advance the cohort counter; they
// are not themselves agent cards. PENDING/provider posture lines are ignored.
func stepActivationsFromOrderedLog(lines []stepTransitionLine) []stepNodeActivation {
	out := make([]stepNodeActivation, 0, len(lines)/2)
	openIdx := make(map[string]int) // nodeID → index in out of open RUNNING
	synthSeen := 0
	ord := 0
	for _, line := range lines {
		nodeID := strings.TrimSpace(line.NodeID)
		if nodeID == "" {
			continue
		}
		st := RuntimeWorkflowStepStatus(strings.TrimSpace(line.Status))
		if isSynthesisStepNode(nodeID) {
			if st == StepStatusRunning {
				synthSeen++
			}
			continue
		}
		switch st {
		case StepStatusRunning:
			out = append(out, stepNodeActivation{
				nodeID:    nodeID,
				runningAt: line.TS,
				cohort:    synthSeen,
				ord:       ord,
			})
			openIdx[nodeID] = len(out) - 1
			ord++
		case StepStatusDone, StepStatusFailed, StepStatusCanceled:
			if idx, ok := openIdx[nodeID]; ok {
				if out[idx].doneAt == "" {
					out[idx].doneAt = line.TS
				}
				delete(openIdx, nodeID)
			}
		}
	}
	return out
}

// stepNodeActivationsFromLog filters stepActivationsFromOrderedLog to one node.
// Kept for call sites / tests that still reason per-node.
func stepNodeActivationsFromLog(lines []stepTransitionLine, nodeID string) []stepNodeActivation {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	all := stepActivationsFromOrderedLog(lines)
	out := make([]stepNodeActivation, 0, len(all))
	for _, act := range all {
		if act.nodeID == nodeID {
			out = append(out, act)
		}
	}
	return out
}

// stepNodeActivationTimes converts logged activations into [spawnAt, resultAt]
// pairs. fallbackEnd covers a RUNNING without a matching terminal line.
func stepNodeActivationTimes(lines []stepTransitionLine, nodeID, fallbackEnd string) [][2]string {
	acts := stepNodeActivationsFromLog(lines, nodeID)
	if len(acts) == 0 {
		return nil
	}
	out := make([][2]string, len(acts))
	for i, act := range acts {
		done := act.doneAt
		if done == "" {
			done = firstNonEmptyResumeValue(fallbackEnd, act.runningAt)
		}
		out[i] = [2]string{act.runningAt, done}
	}
	return out
}

// resumeActivationTimestamps is the LEGACY no-sidecar fallback only. Prefer
// stepActivationsFromOrderedLog cohorts when the step-transition log exists.
// Synthetic wave parking must not be the durable order authority (Terra).
func resumeActivationTimestamps(startedAt, updatedAt string, activations int, peerStarts []string, selfStarted string) [][2]string {
	if activations < 1 {
		activations = 1
	}
	end := firstNonEmptyResumeValue(updatedAt, startedAt)
	start := firstNonEmptyResumeValue(startedAt, end)
	out := make([][2]string, activations)
	if activations == 1 {
		out[0] = [2]string{start, end}
		return out
	}
	waves := peerStartWaveTimes(peerStarts, selfStarted, 45*time.Second)
	if len(waves) >= 1 {
		// act0: original start → first peer wave (end of round 0 work).
		// act i>0: just before peer wave i (this round's reviewers).
		tEnd, endOK := parseResumeTime(end)
		for i := 0; i < activations; i++ {
			if i == 0 {
				resultAt := end
				if len(waves) > 0 {
					resultAt = waves[0].UTC().Format(time.RFC3339Nano)
				}
				out[i] = [2]string{start, resultAt}
				continue
			}
			if i < len(waves) {
				prev := waves[i-1]
				cur := waves[i]
				reinvoke := cur.Add(-time.Millisecond)
				if !reinvoke.After(prev) {
					// Degenerate / overlapping waves — keep a mid-gap fallback.
					reinvoke = prev.Add(cur.Sub(prev) * 9 / 10)
				}
				resultAt := cur.UTC().Format(time.RFC3339Nano)
				if i == activations-1 && endOK && tEnd.After(cur) {
					resultAt = end
				}
				out[i] = [2]string{reinvoke.UTC().Format(time.RFC3339Nano), resultAt}
				continue
			}
			// More activations than waves — park remaining at end.
			out[i] = [2]string{end, end}
		}
		return out
	}
	// Fallback: even split across [startedAt, updatedAt] when peers give no waves.
	t0, ok0 := parseResumeTime(start)
	t1, ok1 := parseResumeTime(end)
	if !ok0 || !ok1 || !t1.After(t0) {
		for i := 0; i < activations; i++ {
			out[i] = [2]string{start, end}
		}
		return out
	}
	span := t1.Sub(t0)
	for i := 0; i < activations; i++ {
		spawnT := t0.Add(time.Duration(int64(span) * int64(i) / int64(activations)))
		resultT := t0.Add(time.Duration(int64(span) * int64(i+1) / int64(activations)))
		out[i] = [2]string{spawnT.UTC().Format(time.RFC3339Nano), resultT.UTC().Format(time.RFC3339Nano)}
	}
	return out
}

// peerStartWaveTimes groups other children's startedAt into Review Loop waves
// (starts within maxGap of each other share a wave). Self is excluded.
func peerStartWaveTimes(peerStarts []string, selfStarted string, maxGap time.Duration) []time.Time {
	filtered := make([]time.Time, 0, len(peerStarts))
	for _, raw := range peerStarts {
		if strings.TrimSpace(raw) == "" || raw == selfStarted {
			continue
		}
		if t, ok := parseResumeTime(raw); ok {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Before(filtered[j]) })
	waves := []time.Time{filtered[0]}
	lastWaveStart := filtered[0]
	for i := 1; i < len(filtered); i++ {
		if filtered[i].Sub(lastWaveStart) > maxGap {
			waves = append(waves, filtered[i])
			lastWaveStart = filtered[i]
		}
	}
	return waves
}

// largestPeerStartGap finds the largest wall-clock gap between consecutive peer
// start times (excluding selfStarted). Used to locate the Review Loop continue
// boundary for reinvoke card placement.
func largestPeerStartGap(peerStarts []string, selfStarted string, minGap time.Duration) (gapStart, gapEnd string, ok bool) {
	filtered := make([]time.Time, 0, len(peerStarts))
	for _, raw := range peerStarts {
		if strings.TrimSpace(raw) == "" || raw == selfStarted {
			continue
		}
		if t, parsed := parseResumeTime(raw); parsed {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) < 2 {
		return "", "", false
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Before(filtered[j]) })
	// unique
	uniq := filtered[:1]
	for i := 1; i < len(filtered); i++ {
		if !filtered[i].Equal(uniq[len(uniq)-1]) {
			uniq = append(uniq, filtered[i])
		}
	}
	if len(uniq) < 2 {
		return "", "", false
	}
	best := time.Duration(0)
	var bestI int
	for i := 0; i < len(uniq)-1; i++ {
		d := uniq[i+1].Sub(uniq[i])
		if d > best {
			best = d
			bestI = i
		}
	}
	if best < minGap {
		return "", "", false
	}
	return uniq[bestI].UTC().Format(time.RFC3339Nano),
		uniq[bestI+1].UTC().Format(time.RFC3339Nano),
		true
}

func firstNonEmptyResumeValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *InteractiveService) resolveAccountHome(providerKey ProviderKey, accountID string) (string, bool) {
	r := s.runner
	if r == nil {
		r = &Runner{}
	}
	accounts, err := r.ListProviderAccounts()
	if err == nil {
		for _, account := range accounts {
			if account.ProviderKey == string(providerKey) && account.ID == accountID && strings.TrimSpace(account.HomePath) != "" {
				return account.HomePath, true
			}
		}
	}
	if accountID == "" || accountID == "default" {
		return DetectDefaultAccountHomePath(string(providerKey))
	}
	return "", false
}

// activeAccountForProvider returns the account id that is active for the given
// provider. Active accounts are tracked per provider — a Claude, Codex and Gemini
// account can each be active at the same time (see ActivateProviderAccount, which
// only deactivates same-provider accounts) — so resume, turn-gating and new-run
// stamping must resolve "the active account" scoped to the run's provider rather
// than the single legacy activeAccountID, which only ever holds one provider's
// selection (Task-067 issue 1).
//
// The per-provider active account is the durable truth in the provider-accounts
// store. We only consult it when a runner is attached (production) or an explicit
// test config path is set; otherwise we fall back to activeAccountID. This keeps
// the many unit tests that build a bare service (no runner, no config) hermetic —
// they never read or mutate the user's real provider-accounts.json.
func (s *InteractiveService) activeAccountForProvider(providerKey ProviderKey) string {
	if s.runner != nil || strings.TrimSpace(os.Getenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH")) != "" {
		r := s.runner
		if r == nil {
			r = &Runner{}
		}
		if acct, err := r.ResolveProviderAccount(string(providerKey), ""); err == nil && strings.TrimSpace(acct.ID) != "" {
			return acct.ID
		}
	}
	return s.activeAccountID
}

func (s *InteractiveService) locateSessionAcrossProviderAccounts(providerKey ProviderKey, activeAccountID, sessionID, cwd string) (ProviderAccount, string, bool) {
	r := s.runner
	if r == nil {
		r = &Runner{}
	}
	accounts, err := r.ListProviderAccounts()
	if err != nil {
		return ProviderAccount{}, "", false
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		return accounts[i].ID == activeAccountID && accounts[j].ID != activeAccountID
	})
	for _, account := range accounts {
		if ProviderKey(account.ProviderKey) != providerKey || strings.TrimSpace(account.HomePath) == "" {
			continue
		}
		if path, found := LocateSessionFile(providerKey, account.HomePath, sessionID, cwd); found {
			return account, path, true
		}
	}
	return ProviderAccount{}, "", false
}

func (s *InteractiveService) ensureResumeReady(rs *interactiveRun) *apiErr {
	// "The active account" must be scoped to this run's provider, not the single
	// global activeAccountID: a Codex chat is resumed against the active Codex
	// account regardless of which Claude/Gemini account is active (Task-067 issue 1).
	// Task-447: for a claim-pinned leg the expected account is the pin itself —
	// the session lives under the pinned account's home and must never rebind
	// to the global active account.
	expectedAccountID := s.activeAccountForProvider(rs.providerKey)
	if rs.accountPinned && strings.TrimSpace(rs.providerAccountID) != "" {
		expectedAccountID = rs.providerAccountID
	}
	activeAccountID := expectedAccountID
	log.Printf(
		"[chat-history-open] resume check run_id=%q provider=%q stored_account_id=%q expected_account_id=%q account_pinned=%t provider_session_id=%q cwd=%q resumed_from_disk=%t",
		rs.id, rs.providerKey, rs.providerAccountID, expectedAccountID, rs.accountPinned, s.resumeSessionID(rs), rs.workspaceCwd, rs.resumedFromDisk,
	)
	if rs.providerKey == ProviderKeyOpencode {
		// CA-688: opencode keeps sessions in the shared
		// ~/.local/share/opencode/opencode.db — there are no per-session files
		// and ACP session/load works from any process on this machine (BUG-329
		// live probe). A real ses_* id is enough to resume, so the whole
		// file-based availability flow below (account-home resolution +
		// LocateSessionFile) does not apply. Synthetic thread-* ids still
		// cannot resume; the auth check runs only when the account home resolves.
		sessionID := s.resumeSessionID(rs)
		if !isOpencodeRealSessionID(sessionID) {
			log.Printf("[chat-history-open] opencode session id not resumable run_id=%q session_id=%q", rs.id, sessionID)
			return newAPIErr(http.StatusConflict, "session_unavailable", "opencode session has no real session id to resume")
		}
		if srcHome, homeOK := s.resolveAccountHome(rs.providerKey, rs.providerAccountID); homeOK {
			if rs.providerAccountID == activeAccountID && !HasLocalAuthAtPath(string(rs.providerKey), srcHome) {
				log.Printf("[chat-history-open] auth missing run_id=%q provider=%q account_id=%q home=%q", rs.id, rs.providerKey, activeAccountID, srcHome)
				return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
			}
		}
		log.Printf("[chat-history-open] opencode db-backed session run_id=%q session_id=%q (file check not applicable)", rs.id, sessionID)
		return nil
	}
	if rs.providerKey == ProviderKeyDevin {
		// Appended last (CP-70/Task-401): same CA-688 shape as opencode —
		// Devin keeps sessions in the shared ~/.local/share/devin/cli/sessions.db
		// SQLite store; ACP session/load works from any process on this machine
		// (live-verified F-23). A real slug id is enough; the file-based flow
		// below does not apply.
		sessionID := s.resumeSessionID(rs)
		if !isDevinRealSessionID(sessionID) {
			log.Printf("[chat-history-open] devin session id not resumable run_id=%q session_id=%q", rs.id, sessionID)
			return newAPIErr(http.StatusConflict, "session_unavailable", "devin session has no real session id to resume")
		}
		if srcHome, homeOK := s.resolveAccountHome(rs.providerKey, rs.providerAccountID); homeOK {
			if rs.providerAccountID == activeAccountID && !HasLocalAuthAtPath(string(rs.providerKey), srcHome) {
				log.Printf("[chat-history-open] auth missing run_id=%q provider=%q account_id=%q home=%q", rs.id, rs.providerKey, activeAccountID, srcHome)
				return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
			}
		}
		log.Printf("[chat-history-open] devin db-backed session run_id=%q session_id=%q (file check not applicable)", rs.id, sessionID)
		return nil
	}
	srcHome, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok && (strings.TrimSpace(rs.providerAccountID) == "" || rs.providerAccountID == "default") {
		srcHome, ok = defaultProviderSessionHome(rs.providerKey)
	}
	recoveredAccount := false
	if !ok {
		account, recoveredPath, found := s.locateSessionAcrossProviderAccounts(
			rs.providerKey,
			activeAccountID,
			s.resumeSessionID(rs),
			rs.workspaceCwd,
		)
		if found {
			log.Printf(
				"[chat-history-open] stale account recovered run_id=%q stale_account_id=%q recovered_account_id=%q recovered_home=%q session_path=%q",
				rs.id, rs.providerAccountID, account.ID, account.HomePath, recoveredPath,
			)
			rs.providerAccountID = account.ID
			srcHome = account.HomePath
			ok = true
			recoveredAccount = true
		}
		if !ok {
			if rs.providerAccountID == activeAccountID {
				log.Printf("[chat-history-open] source home unresolved run_id=%q account_id=%q result=account_not_signed_in", rs.id, rs.providerAccountID)
				return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
			}
			log.Printf("[chat-history-open] source home unresolved run_id=%q stored_account_id=%q active_account_id=%q result=session_unavailable", rs.id, rs.providerAccountID, activeAccountID)
			return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
		}
	}
	log.Printf("[chat-history-open] source home resolved run_id=%q account_id=%q home=%q", rs.id, rs.providerAccountID, srcHome)
	if !s.ensureProviderResumeHandle(rs, srcHome) {
		log.Printf("[chat-history-open] provider resume handle unavailable run_id=%q provider=%q source_home=%q", rs.id, rs.providerKey, srcHome)
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	if rs.providerKey == ProviderKeyGemini {
		return s.ensureGeminiResumeReady(rs, srcHome, activeAccountID, recoveredAccount)
	}
	sessionID := s.resumeSessionID(rs)
	srcPath, found := LocateSessionFile(rs.providerKey, srcHome, sessionID, rs.workspaceCwd)
	if !found {
		log.Printf("[chat-history-open] session file not found run_id=%q provider=%q session_id=%q source_home=%q cwd=%q", rs.id, rs.providerKey, sessionID, srcHome, rs.workspaceCwd)
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	log.Printf("[chat-history-open] session file found run_id=%q provider=%q session_id=%q path=%q", rs.id, rs.providerKey, sessionID, srcPath)

	if rs.providerAccountID == activeAccountID {
		if !HasLocalAuthAtPath(string(rs.providerKey), srcHome) {
			log.Printf("[chat-history-open] auth missing run_id=%q provider=%q account_id=%q home=%q", rs.id, rs.providerKey, activeAccountID, srcHome)
			return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
		}
		if recoveredAccount {
			if err := s.persistProviderSession(sessionStateOf(rs)); err != nil {
				log.Printf("[chat-history-open] recovered account persistence failed run_id=%q active_account_id=%q error=%q", rs.id, activeAccountID, err)
				return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
			}
		}
		log.Printf("[chat-history-open] resume ready run_id=%q mode=same_account account_id=%q", rs.id, activeAccountID)
		return nil
	}
	log.Printf("[chat-history-open] cross-account preparation start run_id=%q stored_account_id=%q active_account_id=%q", rs.id, rs.providerAccountID, activeAccountID)
	return s.prepareCrossAccountResume(rs, srcPath, activeAccountID)
}

func (s *InteractiveService) ensureGeminiResumeReady(rs *interactiveRun, sourceHome, activeAccountID string, recoveredAccount bool) *apiErr {
	projectID := s.resumeSessionID(rs)
	sourceConfigPath := geminiProjectConfigPathForHome(sourceHome, projectID)
	if !fileExists(sourceConfigPath) {
		log.Printf("[chat-history-open] gemini project config missing run_id=%q project_id=%q source_home=%q path=%q", rs.id, projectID, sourceHome, sourceConfigPath)
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	log.Printf("[chat-history-open] gemini project config found run_id=%q project_id=%q path=%q", rs.id, projectID, sourceConfigPath)

	if rs.providerAccountID == activeAccountID {
		if !HasLocalAuthAtPath(string(rs.providerKey), sourceHome) {
			log.Printf("[chat-history-open] auth missing run_id=%q provider=%q account_id=%q home=%q", rs.id, rs.providerKey, activeAccountID, sourceHome)
			return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
		}
		if recoveredAccount {
			if err := s.persistProviderSession(sessionStateOf(rs)); err != nil {
				log.Printf("[chat-history-open] recovered account persistence failed run_id=%q active_account_id=%q error=%q", rs.id, activeAccountID, err)
				return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
			}
		}
		log.Printf("[chat-history-open] resume ready run_id=%q mode=same_account_gemini account_id=%q", rs.id, activeAccountID)
		return nil
	}

	targetHome, ok := s.resolveAccountHome(rs.providerKey, activeAccountID)
	if !ok {
		log.Printf("[chat-history-open] target home unresolved run_id=%q provider=%q active_account_id=%q", rs.id, rs.providerKey, activeAccountID)
		return newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
	}
	if !HasLocalAuthAtPath(string(rs.providerKey), targetHome) {
		log.Printf("[chat-history-open] target auth missing run_id=%q provider=%q active_account_id=%q target_home=%q", rs.id, rs.providerKey, activeAccountID, targetHome)
		return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
	}
	if _, err := relocateGeminiProjectConfig(sourceConfigPath, targetHome, projectID); err != nil {
		log.Printf("[chat-history-open] gemini project relocation failed run_id=%q project_id=%q active_account_id=%q error=%q", rs.id, projectID, activeAccountID, err)
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	rs.providerAccountID = activeAccountID
	if snapErr := s.persistProviderSession(sessionStateOf(rs)); snapErr != nil {
		log.Printf("[chat-history-open] account rebind persistence failed run_id=%q active_account_id=%q error=%q", rs.id, activeAccountID, snapErr)
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", snapErr.Error())
	}
	log.Printf("[chat-history-open] resume ready run_id=%q mode=cross_account_gemini active_account_id=%q target_home=%q", rs.id, activeAccountID, targetHome)
	return nil
}

func (s *InteractiveService) prepareCrossAccountResume(rs *interactiveRun, srcPath, activeAccountID string) *apiErr {
	sourceHome, _ := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	targetHome, ok := s.resolveAccountHome(rs.providerKey, activeAccountID)
	if !ok {
		log.Printf("[chat-history-open] target home unresolved run_id=%q provider=%q active_account_id=%q", rs.id, rs.providerKey, activeAccountID)
		return newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
	}
	if !HasLocalAuthAtPath(string(rs.providerKey), targetHome) {
		log.Printf("[chat-history-open] target auth missing run_id=%q provider=%q active_account_id=%q target_home=%q", rs.id, rs.providerKey, activeAccountID, targetHome)
		return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
	}
	if _, err := RelocateSessionFile(rs.providerKey, srcPath, targetHome, s.resumeSessionID(rs), rs.workspaceCwd); err != nil {
		log.Printf("[chat-history-open] session relocation failed run_id=%q provider=%q active_account_id=%q error=%q", rs.id, rs.providerKey, activeAccountID, err)
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	if err := s.relocateCodexTurnLogSessions(rs, sourceHome, targetHome); err != nil {
		log.Printf("[chat-history-open] turn-log relocation failed run_id=%q provider=%q active_account_id=%q error=%q", rs.id, rs.providerKey, activeAccountID, err)
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	if err := s.relocateGrokTurnLogSessions(rs, sourceHome, targetHome); err != nil {
		log.Printf("[chat-history-open] grok turn-log relocation failed run_id=%q provider=%q active_account_id=%q error=%q", rs.id, rs.providerKey, activeAccountID, err)
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	rs.providerAccountID = activeAccountID
	if snapErr := s.persistProviderSession(sessionStateOf(rs)); snapErr != nil {
		log.Printf("[chat-history-open] account rebind persistence failed run_id=%q active_account_id=%q error=%q", rs.id, activeAccountID, snapErr)
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", snapErr.Error())
	}
	log.Printf("[chat-history-open] resume ready run_id=%q mode=cross_account active_account_id=%q target_home=%q", rs.id, activeAccountID, targetHome)
	return nil
}

func (s *InteractiveService) relocateCodexTurnLogSessions(rs *interactiveRun, sourceHome, targetHome string) error {
	if rs.providerKey != ProviderKeyCodex || sourceHome == "" || targetHome == "" {
		return nil
	}
	logger, ok := s.workflowStore.(TurnLogStore)
	if !ok {
		return nil
	}
	entries, err := logger.ReadTurnLog(context.Background(), rs.id)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	if stableID := s.resumeSessionID(rs); stableID != "" {
		seen[stableID] = true
	}
	for _, entry := range entries {
		if entry.Kind != turnLogKindCodexSession || entry.SessionID == "" || seen[entry.SessionID] {
			continue
		}
		seen[entry.SessionID] = true
		srcPath, found := LocateSessionFile(rs.providerKey, sourceHome, entry.SessionID, rs.workspaceCwd)
		if !found {
			continue
		}
		if _, err := RelocateSessionFile(rs.providerKey, srcPath, targetHome, entry.SessionID, rs.workspaceCwd); err != nil {
			return err
		}
	}
	return nil
}

// relocateGrokTurnLogSessions copies every real Grok session directory referenced
// by the run's turn log into the target account home (Task-210 Option B). The
// durable resume handle is already relocated by prepareCrossAccountResume; this
// mirrors relocateCodexTurnLogSessions so transcript replay stays complete after
// rebind. Missing extra turn-log dirs hard-fail so rebind does not silently drop
// history under the new account.
func (s *InteractiveService) relocateGrokTurnLogSessions(rs *interactiveRun, sourceHome, targetHome string) error {
	if rs.providerKey != ProviderKeyGrok || sourceHome == "" || targetHome == "" {
		return nil
	}
	logger, ok := s.workflowStore.(TurnLogStore)
	if !ok {
		return nil
	}
	entries, err := logger.ReadTurnLog(context.Background(), rs.id)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	if stableID := s.resumeSessionID(rs); isGrokRealSessionID(stableID) {
		seen[stableID] = true
	}
	for _, entry := range entries {
		if entry.Kind != turnLogKindGrokSession || !isGrokRealSessionID(entry.SessionID) || seen[entry.SessionID] {
			continue
		}
		seen[entry.SessionID] = true
		srcPath, found := LocateSessionFile(ProviderKeyGrok, sourceHome, entry.SessionID, rs.workspaceCwd)
		if !found {
			return fmt.Errorf("grok session dir missing for turn-log id %q under %s", entry.SessionID, sourceHome)
		}
		if _, err := RelocateSessionFile(ProviderKeyGrok, srcPath, targetHome, entry.SessionID, rs.workspaceCwd); err != nil {
			return err
		}
	}
	return nil
}

func (s *InteractiveService) syncCodexStableSessionToKnownAccounts(rs *interactiveRun) error {
	if rs.providerKey != ProviderKeyCodex {
		return nil
	}
	sessionID := s.resumeSessionID(rs)
	if sessionID == "" || strings.HasPrefix(sessionID, "thread-") {
		return nil
	}
	sourceHome, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		return nil
	}
	srcPath, found := LocateSessionFile(rs.providerKey, sourceHome, sessionID, rs.workspaceCwd)
	if !found {
		return nil
	}
	r := s.runner
	if r == nil {
		r = &Runner{}
	}
	accounts, err := r.ListProviderAccounts()
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if ProviderKey(account.ProviderKey) != rs.providerKey || account.ID == rs.providerAccountID || strings.TrimSpace(account.HomePath) == "" {
			continue
		}
		if _, found := LocateSessionFile(rs.providerKey, account.HomePath, sessionID, rs.workspaceCwd); !found {
			continue
		}
		if _, err := RelocateSessionFile(rs.providerKey, srcPath, account.HomePath, sessionID, rs.workspaceCwd); err != nil {
			return err
		}
	}
	return nil
}

func (s *InteractiveService) ensureProviderResumeHandle(rs *interactiveRun, accountHome string) bool {
	if rs.providerKey == ProviderKeyGrok {
		return s.ensureGrokProviderResumeHandle(rs, accountHome)
	}
	if rs.providerKey == ProviderKeyDevin {
		// Appended last (CP-70/Task-401): Devin mirrors the Grok turn-log
		// promotion — durable devin_session ids only, never workspace discovery.
		return s.ensureDevinProviderResumeHandle(rs, accountHome)
	}
	if rs.providerKey != ProviderKeyCodex {
		return s.resumeSessionID(rs) != ""
	}
	if s.shouldTreatCodexFlowHubSessionAsSynthetic(rs) {
		return true
	}
	if id := s.resumeSessionID(rs); id != "" && !strings.HasPrefix(id, "thread-") {
		return true
	}
	// Prefer a run-owned codex_session from the durable turn log before any
	// workspace-wide discovery (run-75035: discovery can return the hub's newest
	// rollout when children share cwd).
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		if entries, err := logger.ReadTurnLog(context.Background(), rs.id); err == nil {
			var latestOwn string
			for _, entry := range entries {
				if entry.Kind != turnLogKindCodexSession || !isCodexRealSessionID(entry.SessionID) {
					continue
				}
				if s.isForeignProviderSessionID(rs, entry.SessionID) {
					continue
				}
				latestOwn = entry.SessionID
			}
			if latestOwn != "" {
				rs.realProviderSessionID = latestOwn
				if rs.resumedFromDisk {
					rs.providerSessionID = latestOwn
				}
				return true
			}
		}
	}
	// Children never promote workspace-newest discovery — that is how hub
	// freeform transcripts were rebound onto coder after restart.
	if strings.TrimSpace(rs.parentRunID) != "" {
		return false
	}
	id, ok := DiscoverCodexRolloutSessionID(accountHome, rs.workspaceCwd)
	if !ok || s.isForeignProviderSessionID(rs, id) {
		return false
	}
	rs.realProviderSessionID = id
	if rs.resumedFromDisk {
		rs.providerSessionID = id
	}
	return true
}

// ensureGrokProviderResumeHandle promotes a real ACP session id into the run's
// durable resume handle so LocateSessionFile can resolve the session directory.
//
// Allowed sources only (Task-210 Option B + run-536/584 safety):
//  1. already-real resumeSessionID (persisted realProviderSessionID / provider_session_id)
//  2. latest run-owned turn-log grok_session id (legacy thread-* rows like run-370)
//
// NEVER promote from discoverGrokSessionDirs: GROK_HOME+cwd is shared across
// chats, so "exactly one dir" still means "someone else's chat" for a synthetic
// run with no turn-log entry (Codex review + run-536 class).
func (s *InteractiveService) ensureGrokProviderResumeHandle(rs *interactiveRun, accountHome string) bool {
	if id := s.resumeSessionID(rs); isGrokRealSessionID(id) {
		return true
	}
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		if entries, err := logger.ReadTurnLog(context.Background(), rs.id); err == nil {
			var latest string
			for _, entry := range entries {
				if entry.Kind == turnLogKindGrokSession && isGrokRealSessionID(entry.SessionID) {
					latest = entry.SessionID
				}
			}
			if latest != "" {
				rs.realProviderSessionID = latest
				if rs.resumedFromDisk {
					rs.providerSessionID = latest
				}
				return true
			}
		}
	}
	return false
}

// ensureDevinProviderResumeHandle promotes a real Devin ACP session slug into
// the run's durable resume handle (CP-70/Task-401). Mirrors
// ensureGrokProviderResumeHandle: allowed sources are an already-real
// resumeSessionID or a run-owned turn-log devin_session entry — never
// workspace-wide discovery (sessions.db is shared across chats).
func (s *InteractiveService) ensureDevinProviderResumeHandle(rs *interactiveRun, accountHome string) bool {
	if id := s.resumeSessionID(rs); isDevinRealSessionID(id) {
		return true
	}
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		if entries, err := logger.ReadTurnLog(context.Background(), rs.id); err == nil {
			var latest string
			for _, entry := range entries {
				if entry.Kind == turnLogKindDevinSession && isDevinRealSessionID(entry.SessionID) {
					latest = entry.SessionID
				}
			}
			if latest != "" {
				rs.realProviderSessionID = latest
				if rs.resumedFromDisk {
					rs.providerSessionID = latest
				}
				return true
			}
		}
	}
	return false
}

func (s *InteractiveService) resumeSessionID(rs *interactiveRun) string {
	if rs.realProviderSessionID != "" {
		return rs.realProviderSessionID
	}
	return rs.providerSessionID
}

// seedTranscriptFromDisk loads provider session file(s) and populates rs.events
// so the SSE snapshot path replays the prior conversation to the desktop.
// Best-effort: any error is silently ignored to not block resume.
//
// BUG-083 fixes applied here:
//
//	F-1 — raw user prompts from the turn log replace the composed prompt text
//	       stored in the provider file (which includes the ask_user reinforcement
//	       and any skill/MCP preamble the live path injected).
//	F-3 — for Codex, all per-turn rollout files are loaded in order (Codex writes
//	       one file per turn with a distinct session id; only the stored id's file
//	       was previously loaded, dropping every turn after the first).
func (s *InteractiveService) seedTranscriptFromDisk(rs *interactiveRun) {
	if !rs.resumedFromDisk {
		return
	}
	// Whatever branch below appends (or doesn't, e.g. Grok/default today — see
	// Task-212 DOD-5/DOD-9), move any sidecar-origin events reconstructRun had
	// to seed first back after the real transcript (reorderSidecarPrefixToEnd).
	defer s.reorderSidecarPrefixToEnd(rs)
	if rs.providerKey == ProviderKeyGemini {
		s.seedGeminiTranscriptFromState(rs)
		s.appendResumedParentAnnotations(rs)
		return
	}
	if rs.providerKey == ProviderKeyGrok {
		s.seedGrokTranscriptFromDisk(rs)
		s.appendResumedParentAnnotations(rs)
		return
	}
	if rs.providerKey == ProviderKeyOpencode {
		s.seedOpencodeTranscriptFromDisk(rs)
		s.appendResumedParentAnnotations(rs)
		return
	}
	if rs.providerKey == ProviderKeyDevin {
		// Appended last (CP-70/Task-401): sessions.db has no per-session file —
		// rebuild from the durable turn log exactly like opencode.
		s.seedDevinTranscriptFromDisk(rs)
		s.appendResumedParentAnnotations(rs)
		return
	}
	var loader func(string) ([]ProviderEvent, error)
	switch rs.providerKey {
	case ProviderKeyClaude:
		loader = loadClaudeTranscriptEvents
	case ProviderKeyCodex:
		loader = loadCodexTranscriptEvents
	default:
		return
	}
	defer s.appendResumedParentAnnotations(rs)

	// Load turn log: raw prompts (F-1) and Codex per-turn session ids (F-3).
	// Keep each durable turn id with its raw text so normal-chat sidecars can be
	// restored beside the replayed prompt after a server restart.
	var rawPrompts []turnLogLine
	var codexSessionIDs []string
	var allEntries []turnLogLine
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			allEntries = entries
			for _, e := range entries {
				switch e.Kind {
				case turnLogKindPrompt:
					if e.Prompt != "" && !isSystemPrompt(e.Prompt) {
						rawPrompts = append(rawPrompts, e)
					}
				case turnLogKindCodexSession:
					if e.SessionID != "" {
						codexSessionIDs = append(codexSessionIDs, e.SessionID)
					}
				}
			}
		}
	}
	// run-24377 class: shared provider session with children can pollute hub
	// reopen. Prefer durable turn-log prose when the hub has assistant frames.
	// Cross-provider Case 1 — same gate as seedGrokTranscriptFromDisk.
	if s.preferFlowHubTurnLogTranscript(rs, allEntries) {
		s.seedFlowHubTranscriptFromTurnLog(rs, allEntries)
		return
	}

	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok && (strings.TrimSpace(rs.providerAccountID) == "" || rs.providerAccountID == "default") {
		home, ok = defaultProviderSessionHome(rs.providerKey)
	}
	if !ok {
		historical := mergeTurnLogAssistantsIntoTranscript(promptOnlyTurnLogEvents(rawPrompts), allEntries)
		if len(historical) == 0 {
			return
		}
		s.appendTranscriptReplayEvents(rs, historical)
		return
	}

	// Collect the session file path(s) to load.
	sessionID := s.resumeSessionID(rs) // used for event correlation below
	// run-75035 defense in depth: never load parent/sibling provider sessions
	// that leaked into this run's turn log (or into resumeSessionID).
	if isCodexRealSessionID(sessionID) && s.isForeignProviderSessionID(rs, sessionID) {
		sessionID = ""
	}
	codexSessionIDs = s.filterOwnedProviderSessionIDs(rs, codexSessionIDs)
	var filePaths []string
	if rs.providerKey == ProviderKeyCodex {
		// Load every per-turn rollout file in recorded order (F-3, BUG-083).
		seen := map[string]bool{}
		if sessionID != "" && isCodexRealSessionID(sessionID) {
			if path, found := LocateSessionFile(rs.providerKey, home, sessionID, rs.workspaceCwd); found {
				filePaths = append(filePaths, path)
			}
			seen[sessionID] = true
		}
		for _, sid := range codexSessionIDs {
			if seen[sid] {
				continue
			}
			seen[sid] = true
			if path, found := LocateSessionFile(rs.providerKey, home, sid, rs.workspaceCwd); found {
				filePaths = append(filePaths, path)
			}
		}
	}
	// Fall back to the single stored session file: old Codex runs without a turn
	// log, and all Claude runs (one JSONL holds the full conversation).
	if len(filePaths) == 0 {
		if path, found := LocateSessionFile(rs.providerKey, home, sessionID, rs.workspaceCwd); found {
			filePaths = []string{path}
		}
	}

	// Load events from all files.
	var historical []ProviderEvent
	for _, fp := range filePaths {
		ev, loadErr := loader(fp)
		if loadErr != nil {
			// BUG-487: replay the parsed prefix but never silently — a
			// truncated transcript tail is surfaced so operators know the
			// restored history is incomplete.
			fmt.Printf("[transcript-load] partial read run=%s file=%s: %v\n", rs.id, fp, loadErr)
		}
		historical = append(historical, ev...)
	}
	if len(historical) == 0 {
		historical = promptOnlyTurnLogEvents(rawPrompts)
		if len(historical) == 0 {
			return
		}
	}

	// Same as Grok: filter shared-session pollution before overlay remaps slots.
	if shouldFilterFlowHubProviderHistory(rs, allEntries) {
		historical = filterFlowHubUnassistedProviderHistory(historical, turnLogPromptTexts(rawPrompts))
	}

	// Replay raw user intent and its durable turn id for every provider. This
	// lets the shared sidecar reorderer insert restored approvals/questions
	// directly after their originating prompt instead of at the bottom.
	historical = overlayRawTurnPrompts(historical, rawPrompts)
	// A flow-hub's turn 1 can spawn its entry child directly without the hub
	// itself ever making a real provider call (its first genuine turn in the
	// session file is the post-join synthesis turn, which overlayRawTurnPrompts
	// correctly leaves system-flagged and unconsumed). That leaves rawPrompts[0]
	// with no historical turn_started slot to overlay onto, so it must be
	// restored as its own synthetic prompt-only turn instead of silently
	// dropping the run's original user message on restart. Mirrors the same
	// safety net already used by seedGrokTranscriptFromDisk.
	historical = prependMissingPromptOnlyEvents(turnLogPromptTexts(rawPrompts), historical)
	// Provider files remain the primary transcript source. The durable turn log
	// only fills an omitted assistant frame (for example a rotated segment), so
	// history reopen has the same user-visible content as the live transcript.
	historical = mergeTurnLogAssistantsIntoTranscript(historical, allEntries)

	historical = userFacingTranscriptEvents(historical)
	s.appendTranscriptReplayEvents(rs, historical)
}

func userFacingTranscriptEvents(historical []ProviderEvent) []ProviderEvent {
	if len(historical) == 0 {
		return nil
	}
	out := historical[:0]
	for _, e := range historical {
		if isInternalTranscriptEvent(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// liveTurnStartedDisplayPrompt returns the prompt to render on a LIVE
// turn_started bubble. Internal flow-engine orchestration prompts — the joined
// result note, gate reprompts, and cross-provider handoff envelopes — are hidden
// from the human transcript on REPLAY (userFacingTranscriptEvents ->
// isInternalTranscriptEvent below, keyed on isSystemPrompt). Historically the
// live emit streamed the full prompt regardless, so such a bubble appeared live
// yet vanished after a restart/reopen ("lost message", BUG-293 / CP-51 A1). This
// keeps the two paths symmetric by returning "" for those prompts so the desktop
// renders no user bubble (timelineReducer only pushes a prompt when e.prompt is
// truthy). The provider still receives the full prompt: callers pass in.Prompt to
// runTurn and the turn log unchanged — only this DISPLAY copy is redacted.
//
// The predicate is deliberately isSystemPrompt (not a joined-note special case)
// so live and replay classify every internal prompt identically and can never
// drift apart again.
func liveTurnStartedDisplayPrompt(prompt string) string {
	if isSystemPrompt(prompt) {
		return ""
	}
	return prompt
}

func isInternalTranscriptEvent(event ProviderEvent) bool {
	if event.Type == EventTurnStarted {
		return isSystemPrompt(event.Prompt)
	}
	if event.Type == EventToolCompleted {
		return strings.EqualFold(strings.TrimSpace(event.ToolName), "spawn_agent")
	}
	if event.Type != EventMessageCompleted {
		return false
	}
	text := strings.TrimSpace(event.Text)
	return strings.HasPrefix(text, "Spawned agent **") ||
		(strings.HasPrefix(text, "**[") && strings.Contains(text, "]**"))
}

func (s *InteractiveService) appendTranscriptReplayEvents(rs *interactiveRun, historical []ProviderEvent) {
	s.appendTranscriptReplayEventsOpt(rs, historical, true)
}

// appendTranscriptReplayEventsOpt appends reconstructed transcript frames.
// defaultTime=true stamps empty OccurredAt with rs.createdAt (legacy provider
// history seeds). defaultTime=false leaves empty times so flow-hub turn-log
// seeds rely on insertUnanchoredFlowAgentLifecycle clustering instead of
// reorderResumedTimelineLocked dumping every child after a shared createdAt
// (run-24377 bottom agent dump).
func (s *InteractiveService) appendTranscriptReplayEventsOpt(rs *interactiveRun, historical []ProviderEvent, defaultTime bool) {
	if rs == nil || len(historical) == 0 {
		return
	}
	stepID := "chat-" + rs.id
	sessionID := s.resumeSessionID(rs)
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs.transcriptSeeded {
		return // concurrent call guard
	}
	rs.transcriptSeeded = true
	for i := range historical {
		rs.seq++
		historical[i].Seq = rs.seq
		historical[i].ID = s.nextID("transcript")
		historical[i].WorkflowRunID = rs.id
		historical[i].WorkflowStepRunID = stepID
		historical[i].ProviderSessionID = sessionID
		historical[i].ProviderKey = rs.providerKey
		if defaultTime && strings.TrimSpace(historical[i].OccurredAt) == "" {
			historical[i].OccurredAt = rs.createdAt
		}
		rs.events = append(rs.events, historical[i])
	}
	// Append a synthetic terminal to close any trailing "Thinking..." row.
	// timelineReducer.finalize injects a thinking row after every non-terminal event
	// (message_completed / tool_completed etc.), so without this the resumed idle chat
	// would render a perpetual spinner. BUG-384: a run whose last turn FAILED must
	// close with turn_failed — replaying it as turn_completed masks the failure.
	if last := rs.events[len(rs.events)-1]; last.Type != EventTurnCompleted && last.Type != EventTurnFailed {
		tailType := EventTurnCompleted
		if rs.status == RunStatusFailed || rs.status == RunStatusCancelled {
			tailType = EventTurnFailed
		}
		rs.seq++
		rs.events = append(rs.events, ProviderEvent{
			Seq:               rs.seq,
			Type:              tailType,
			ID:                s.nextID("transcript"),
			WorkflowRunID:     rs.id,
			WorkflowStepRunID: stepID,
			ProviderSessionID: sessionID,
			ProviderKey:       rs.providerKey,
			OccurredAt:        firstNonEmptyResumeValue(rs.updatedAt, rs.createdAt),
		})
	}
}

func (s *InteractiveService) appendResumedParentAnnotations(rs *interactiveRun) {
	if rs == nil || strings.TrimSpace(rs.parentRunID) != "" {
		return
	}
	annotations := s.resumedParentAgentAnnotations(rs.id)
	if len(annotations) == 0 {
		return
	}
	sessionID := s.resumeSessionID(rs)
	stepID := "chat-" + rs.id
	s.mu.Lock()
	defer s.mu.Unlock()
	// Dedupe by event id when present so lifecycle:reinvoke can restore multiple
	// cards for the same childRunId (run-9034 second coder card). Fall back to
	// type:childRunId for legacy single-activation rows without ids.
	seen := make(map[string]struct{})
	existingSpawnsByChild := make(map[string]int)
	existingResultsByChild := make(map[string]int)
	for _, event := range rs.events {
		switch event.Type {
		case EventAgentSpawnedByUser:
			seen[resumeAnnotationDedupeKey(event)] = struct{}{}
			existingSpawnsByChild[event.ChildRunID]++
		case EventAgentResultInjected:
			seen[resumeAnnotationDedupeKey(event)] = struct{}{}
			existingResultsByChild[event.ChildRunID]++
		}
	}
	spawns := make([]ProviderEvent, 0, len(annotations))
	results := make([]ProviderEvent, 0, len(annotations))
	spawnSlotByChild := make(map[string]int)
	resultSlotByChild := make(map[string]int)
	for _, annotation := range annotations {
		switch annotation.Type {
		case EventAgentSpawnedByUser:
			slot := spawnSlotByChild[annotation.ChildRunID]
			spawnSlotByChild[annotation.ChildRunID] = slot + 1
			// Skip activations already present from a live stream / prior seed.
			if slot < existingSpawnsByChild[annotation.ChildRunID] {
				continue
			}
			if _, exists := seen[resumeAnnotationDedupeKey(annotation)]; exists {
				continue
			}
			spawns = append(spawns, annotation)
		case EventAgentResultInjected:
			slot := resultSlotByChild[annotation.ChildRunID]
			resultSlotByChild[annotation.ChildRunID] = slot + 1
			if slot < existingResultsByChild[annotation.ChildRunID] {
				continue
			}
			if _, exists := seen[resumeAnnotationDedupeKey(annotation)]; exists {
				continue
			}
			results = append(results, annotation)
		}
	}
	annotationIndex := 0
	for i := range rs.events {
		if rs.events[i].Type != EventToolStarted || !strings.EqualFold(strings.TrimSpace(rs.events[i].ToolName), "spawn_agent") {
			continue
		}
		for annotationIndex < len(spawns) {
			annotation := spawns[annotationIndex]
			annotationIndex++
			key := resumeAnnotationDedupeKey(annotation)
			if _, exists := seen[key]; exists {
				continue
			}
			// The provider transcript keeps this tool call in its original
			// position. Reuse that position for the durable child card instead
			// of appending every restored child after the final synthesis turn.
			annotation.ID = rs.events[i].ID
			annotation.Seq = rs.events[i].Seq
			annotation.WorkflowRunID = rs.events[i].WorkflowRunID
			annotation.WorkflowStepRunID = rs.events[i].WorkflowStepRunID
			annotation.ProviderSessionID = rs.events[i].ProviderSessionID
			annotation.ProviderKey = rs.events[i].ProviderKey
			annotation.OccurredAt = rs.events[i].OccurredAt
			rs.events[i] = annotation
			seen[key] = struct{}{}
			seen[resumeAnnotationDedupeKey(annotation)] = struct{}{}
			break
		}
	}
	// Flow-engine nodes are spawned by the runner (no spawn_agent tool anchor).
	// Multi-round Review Loop must NOT dump every child before the first hub
	// synthesis message (run-9034: coder+review×4 then one response). Place each
	// startedAt cluster before the matching hub message_completed so restore
	// reads: coder → reviewers R0 → response → reviewers R1 → response.
	// Non-flow runs still need orphan results bound after already-anchored spawns.
	s.insertUnanchoredFlowAgentLifecycleLocked(rs, spawns[annotationIndex:], results, sessionID, stepID, seen)
	if rs.flowEngineDriven {
		// Prefer durable timestamps when every frame has one so multi-round
		// children interleave past hub synthesis turns; otherwise keep the
		// cluster-before-message insertion order and only renumber Seq.
		if allEventsHaveOccurredAt(rs.events) {
			s.reorderResumedTimelineLocked(rs)
		} else {
			// Mixed timestamps: hub turn-log prose often has empty OccurredAt
			// (seedFlowHubTranscriptFromTurnLog) while agent cards carry child
			// wall-clock starts. The desktop orderHistoryReplayEvents sorts
			// timed events BEFORE untimed ones, which dumps every agent card
			// above the original "fix bug …" prompt (run-24377 live). Strip
			// partial stamps so Seq order from cluster placement is preserved.
			for i := range rs.events {
				rs.events[i].OccurredAt = ""
			}
			s.resequenceResumedTimelineLocked(rs)
		}
		// Annotation events keep Thinking... on the desktop (shouldKeepThinking).
		// After a terminal flow resume the last frames are agent cards — append a
		// synthetic turn_completed so the client clears the spinner (run-20332).
		s.ensureTerminalTurnCompletedAfterResumeLocked(rs)
		return
	}
	s.reorderResumedTimelineLocked(rs)
	s.ensureTerminalTurnCompletedAfterResumeLocked(rs)
}

// ensureTerminalTurnCompletedAfterResumeLocked appends turn_completed when the
// resumed run is terminal and the event stream does not already end with a
// terminal turn frame (so desktop does not leave a perpetual "Thinking...").
func (s *InteractiveService) ensureTerminalTurnCompletedAfterResumeLocked(rs *interactiveRun) {
	if rs == nil || len(rs.events) == 0 {
		return
	}
	terminal := rs.status == RunStatusCompleted || rs.status == RunStatusFailed || rs.status == RunStatusCancelled
	if !terminal && rs.flowEngineDriven {
		// Loop may be done while status is still running (same class as history list).
		if s.agentOrchestrator.loopStateFor(rs.id).Status == "done" {
			terminal = true
			rs.status = RunStatusCompleted
			rs.agentStatus = string(RunStatusCompleted)
		}
	}
	if !terminal {
		return
	}
	last := rs.events[len(rs.events)-1]
	if last.Type == EventTurnCompleted || last.Type == EventTurnFailed {
		return
	}
	rs.seq++
	rs.events = append(rs.events, ProviderEvent{
		Seq:               rs.seq,
		Type:              EventTurnCompleted,
		ID:                s.nextID("transcript"),
		WorkflowRunID:     rs.id,
		WorkflowStepRunID: "chat-" + rs.id,
		ProviderSessionID: s.resumeSessionID(rs),
		ProviderKey:       rs.providerKey,
		OccurredAt:        firstNonEmptyResumeValue(rs.updatedAt, rs.createdAt),
	})
}

// flowAgentLifecyclePair is one restored child card: spawn plus optional result.
type flowAgentLifecyclePair struct {
	spawn   ProviderEvent
	result  ProviderEvent
	hasRes  bool
	at      string // legacy sort key (spawn OccurredAt) when !durable
	durable bool
	cohort  int
	ord     int
}

func resumeAnnotationDedupeKey(ev ProviderEvent) string {
	if id := strings.TrimSpace(ev.ID); id != "" {
		return string(ev.Type) + ":id:" + id
	}
	return string(ev.Type) + ":" + ev.ChildRunID
}

// insertUnanchoredFlowAgentLifecycleLocked restores flow-engine child cards that
// have no spawn_agent tool row. Prefer durable step-log cohorts (ResumeDurable):
// each synthesis-bound cohort is inserted immediately before the matching hub
// message_completed. Without durable metadata (no step-transition sidecar),
// falls back to startedAt gap clustering for legacy fixtures.
//
// Always binds orphan results (spawn already present) after their spawn — including
// non-flow-engine parents. Caller holds s.mu.
func (s *InteractiveService) insertUnanchoredFlowAgentLifecycleLocked(rs *interactiveRun, spawns, results []ProviderEvent, sessionID, stepID string, seen map[string]struct{}) {
	if rs == nil {
		return
	}
	// Results keyed in order per child so reinvoke activation N binds to spawn N.
	resultsByChild := make(map[string][]ProviderEvent)
	for _, annotation := range results {
		key := resumeAnnotationDedupeKey(annotation)
		if _, exists := seen[key]; exists {
			continue
		}
		resultsByChild[annotation.ChildRunID] = append(resultsByChild[annotation.ChildRunID], annotation)
	}
	pairs := make([]flowAgentLifecyclePair, 0, len(spawns))
	if rs.flowEngineDriven {
		for _, annotation := range spawns {
			key := resumeAnnotationDedupeKey(annotation)
			if _, exists := seen[key]; exists {
				continue
			}
			if strings.TrimSpace(annotation.ID) == "" {
				annotation.ID = s.nextID("transcript")
			}
			annotation.WorkflowRunID = rs.id
			annotation.WorkflowStepRunID = stepID
			annotation.ProviderSessionID = sessionID
			annotation.ProviderKey = rs.providerKey
			if strings.TrimSpace(annotation.OccurredAt) == "" {
				annotation.OccurredAt = rs.createdAt
			}
			pair := flowAgentLifecyclePair{
				spawn:   annotation,
				at:      annotation.OccurredAt,
				durable: annotation.ResumeDurable,
				cohort:  annotation.ResumeCohort,
				ord:     annotation.ResumeLogOrd,
			}
			if queue := resultsByChild[annotation.ChildRunID]; len(queue) > 0 {
				result := queue[0]
				resultsByChild[annotation.ChildRunID] = queue[1:]
				if strings.TrimSpace(result.ID) == "" {
					result.ID = s.nextID("transcript")
				}
				result.WorkflowRunID = rs.id
				result.WorkflowStepRunID = stepID
				result.ProviderSessionID = sessionID
				result.ProviderKey = rs.providerKey
				if strings.TrimSpace(result.OccurredAt) == "" {
					result.OccurredAt = annotation.OccurredAt
				}
				// Keep durable markers on the result so resequence paths that
				// inspect results stay consistent with the spawn.
				result.ResumeDurable = annotation.ResumeDurable
				result.ResumeCohort = annotation.ResumeCohort
				result.ResumeLogOrd = annotation.ResumeLogOrd
				pair.result = result
				pair.hasRes = true
				seen[resumeAnnotationDedupeKey(result)] = struct{}{}
			}
			pairs = append(pairs, pair)
			seen[key] = struct{}{}
		}
	}
	// Orphan results (spawn already anchored / non-flow) still bind after their spawn.
	for childID, queue := range resultsByChild {
		for _, result := range queue {
			key := resumeAnnotationDedupeKey(result)
			if _, exists := seen[key]; exists {
				continue
			}
			if strings.TrimSpace(result.ID) == "" {
				result.ID = s.nextID("transcript")
			}
			result.WorkflowRunID = rs.id
			result.WorkflowStepRunID = stepID
			result.ProviderSessionID = sessionID
			result.ProviderKey = rs.providerKey
			if strings.TrimSpace(result.OccurredAt) == "" {
				result.OccurredAt = rs.createdAt
			}
			insertAt := -1
			for i, event := range rs.events {
				if event.Type == EventAgentSpawnedByUser && event.ChildRunID == childID {
					insertAt = i + 1
				}
			}
			if insertAt < 0 {
				rs.events = append(rs.events, result)
			} else {
				reordered := make([]ProviderEvent, 0, len(rs.events)+1)
				reordered = append(reordered, rs.events[:insertAt]...)
				reordered = append(reordered, result)
				reordered = append(reordered, rs.events[insertAt:]...)
				rs.events = reordered
			}
			seen[key] = struct{}{}
		}
	}
	if !rs.flowEngineDriven || len(pairs) == 0 {
		return
	}
	// run-24377: identify first user prompt and the *first post-flow follow-up*
	// (second user-facing turn_started). Synthesis anchors are only hub
	// message_completed frames before that follow-up — never the follow-up
	// answer ("ok") or later chat. Using lastUserPromptIdx was wrong when the
	// user sent a second follow-up ("turn cũ…"): "ok" became a fake anchor and
	// R2 reviewers landed between "done rồi hả" and "ok" (Image 1).
	firstUserPromptIdx, firstPostFlowFollowUpIdx := -1, -1
	userPromptCount := 0
	for i, event := range rs.events {
		if event.Type == EventTurnStarted && strings.TrimSpace(event.Prompt) != "" && !isSystemPrompt(event.Prompt) {
			userPromptCount++
			if firstUserPromptIdx < 0 {
				firstUserPromptIdx = i
			} else if firstPostFlowFollowUpIdx < 0 {
				firstPostFlowFollowUpIdx = i
			}
		}
	}
	msgIdxs := make([]int, 0, 4)
	for i, event := range rs.events {
		if event.Type != EventMessageCompleted || strings.TrimSpace(event.Text) == "" {
			continue
		}
		// Only pre-follow-up hub messages are synthesis anchors for agent rounds.
		if firstPostFlowFollowUpIdx >= 0 && i >= firstPostFlowFollowUpIdx {
			continue
		}
		msgIdxs = append(msgIdxs, i)
	}

	// Prefer durable step-log cohorts even when only a subset of pairs have
	// ResumeDurable (partial sidecar / unmatched spawn child). Never drop
	// durable cohorts into the 45s wall-clock path just because one pair is
	// legacy — that reintroduces synthetic-time round assignment.
	durablePairs := make([]flowAgentLifecyclePair, 0, len(pairs))
	legacyPairs := make([]flowAgentLifecyclePair, 0, len(pairs))
	for _, p := range pairs {
		if p.durable {
			durablePairs = append(durablePairs, p)
		} else {
			legacyPairs = append(legacyPairs, p)
		}
	}
	var clusters [][]flowAgentLifecyclePair
	if len(durablePairs) > 0 {
		clusters = append(clusters, clusterFlowAgentPairsByDurableCohort(durablePairs)...)
	}
	if len(legacyPairs) > 0 {
		sort.SliceStable(legacyPairs, func(i, j int) bool {
			if legacyPairs[i].at == legacyPairs[j].at {
				return legacyPairs[i].spawn.ChildRunID < legacyPairs[j].spawn.ChildRunID
			}
			return legacyPairs[i].at < legacyPairs[j].at
		})
		// Legacy-only or leftover pairs: gap-cluster (documented degraded path).
		clusters = append(clusters, clusterFlowAgentPairsByStartGap(legacyPairs, 45*time.Second)...)
	}
	// Do NOT even-partition when under-segmented (run-24377 Image 2).

	// Clamp inserts: never above the original prompt; never at/after the first
	// post-flow follow-up (agents must not sit between "done rồi hả" and "ok").
	clampPlacement := func(beforeIdx int) int {
		if firstUserPromptIdx >= 0 && beforeIdx <= firstUserPromptIdx {
			beforeIdx = firstUserPromptIdx + 1
		}
		if firstPostFlowFollowUpIdx >= 0 && beforeIdx >= firstPostFlowFollowUpIdx {
			beforeIdx = firstPostFlowFollowUpIdx
		}
		return beforeIdx
	}
	// Map cluster → insertion index in current rs.events (before that index).
	// cluster i goes before message i when available; leftover clusters before
	// the last synthesis (run-9034: R0 before synth0, R1 before synth1 when both
	// restored; never after follow-up).
	type placement struct {
		beforeIdx int // insert before this index; len(events) means append
		cluster   []flowAgentLifecyclePair
		order     int // original cluster index (R0, R1, …)
	}
	placements := make([]placement, 0, len(clusters))
	switch {
	case len(msgIdxs) == 0:
		// No hub prose restored — append agent clusters at the end (still after
		// the first user prompt and before any follow-up when present).
		for i, cluster := range clusters {
			placements = append(placements, placement{
				beforeIdx: clampPlacement(len(rs.events)),
				cluster:   cluster,
				order:     i,
			})
		}
	case len(msgIdxs) == 1:
		// Only one synthesis frame (common when Grok resume loses intermediate
		// hub turns). Put *all* agent clusters before it so the final response
		// still appears after code-review round 2, not stranded mid-timeline
		// between R0 and R1 (run-9034 screenshot).
		for i, cluster := range clusters {
			placements = append(placements, placement{
				beforeIdx: clampPlacement(msgIdxs[0]),
				cluster:   cluster,
				order:     i,
			})
		}
	default:
		// Multi-message: cluster i before synthesis message i; leftover clusters
		// before the *last* synthesis so trailing reviews still precede it.
		lastMsg := msgIdxs[len(msgIdxs)-1]
		for i, cluster := range clusters {
			beforeIdx := lastMsg
			if i < len(msgIdxs) {
				beforeIdx = msgIdxs[i]
			}
			placements = append(placements, placement{
				beforeIdx: clampPlacement(beforeIdx),
				cluster:   cluster,
				order:     i,
			})
		}
	}
	// Apply from the end so earlier indices stay valid. When several clusters
	// share the same beforeIdx (single synthesis / no hub prose), insert later
	// rounds first so earlier rounds remain first after successive inserts
	// (run-52518 single-synth: R0 then R1 before final message, not reversed).
	sort.SliceStable(placements, func(i, j int) bool {
		if placements[i].beforeIdx != placements[j].beforeIdx {
			return placements[i].beforeIdx > placements[j].beforeIdx
		}
		return placements[i].order > placements[j].order
	})
	for _, p := range placements {
		block := flattenFlowAgentCluster(p.cluster)
		if len(block) == 0 {
			continue
		}
		at := p.beforeIdx
		if at < 0 {
			at = 0
		}
		if at > len(rs.events) {
			at = len(rs.events)
		}
		reordered := make([]ProviderEvent, 0, len(rs.events)+len(block))
		reordered = append(reordered, rs.events[:at]...)
		reordered = append(reordered, block...)
		reordered = append(reordered, rs.events[at:]...)
		rs.events = reordered
	}
}

// clusterFlowAgentPairsByDurableCohort groups pairs by ResumeCohort (synthesis
// RUNNING count preceding the activation in the step-transition log). Within a
// cohort, order by ResumeLogOrd then ChildRunID — append order, not wall-clock.
func clusterFlowAgentPairsByDurableCohort(pairs []flowAgentLifecyclePair) [][]flowAgentLifecyclePair {
	if len(pairs) == 0 {
		return nil
	}
	sorted := append([]flowAgentLifecyclePair(nil), pairs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].cohort != sorted[j].cohort {
			return sorted[i].cohort < sorted[j].cohort
		}
		if sorted[i].ord != sorted[j].ord {
			return sorted[i].ord < sorted[j].ord
		}
		return sorted[i].spawn.ChildRunID < sorted[j].spawn.ChildRunID
	})
	clusters := [][]flowAgentLifecyclePair{{sorted[0]}}
	for i := 1; i < len(sorted); i++ {
		if sorted[i].cohort != sorted[i-1].cohort {
			clusters = append(clusters, []flowAgentLifecyclePair{sorted[i]})
			continue
		}
		clusters[len(clusters)-1] = append(clusters[len(clusters)-1], sorted[i])
	}
	return clusters
}

// clusterFlowAgentPairsByStartGap is the LEGACY no-sidecar clusterer. Prefer
// clusterFlowAgentPairsByDurableCohort when step-transition cohorts exist.
//
// Groups restored children into Review Loop rounds: a gap larger than maxGap
// between consecutive startedAt values starts a new cluster (hub synthesis +
// continue sits in that gap).
//
// The gap is measured from the previous pair's *latest* known time (result
// OccurredAt when present, else spawn) so a long-running coder that starts a
// round does not form its own cluster separate from the reviewers it unblocks
// a minute later.
func clusterFlowAgentPairsByStartGap(pairs []flowAgentLifecyclePair, maxGap time.Duration) [][]flowAgentLifecyclePair {
	if len(pairs) == 0 {
		return nil
	}
	clusters := [][]flowAgentLifecyclePair{{pairs[0]}}
	prevEdge, prevOK := pairTimelineEdge(pairs[0])
	for i := 1; i < len(pairs); i++ {
		at, ok := parseResumeTime(pairs[i].at)
		newCluster := false
		if prevOK && ok && at.Sub(prevEdge) > maxGap {
			newCluster = true
		}
		if newCluster {
			clusters = append(clusters, []flowAgentLifecyclePair{pairs[i]})
		} else {
			clusters[len(clusters)-1] = append(clusters[len(clusters)-1], pairs[i])
		}
		if edge, edgeOK := pairTimelineEdge(pairs[i]); edgeOK {
			prevEdge, prevOK = edge, true
		}
	}
	return clusters
}

func pairTimelineEdge(p flowAgentLifecyclePair) (time.Time, bool) {
	if p.hasRes {
		if t, ok := parseResumeTime(p.result.OccurredAt); ok {
			return t, true
		}
	}
	return parseResumeTime(p.at)
}

func parseResumeTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// flattenFlowAgentCluster expands a startedAt cluster into timeline events.
// Concurrent reviewers (near-identical start) emit spawn×N then result×N to
// match live wall-clock; serial children keep spawn→result pairs.
func flattenFlowAgentCluster(cluster []flowAgentLifecyclePair) []ProviderEvent {
	if len(cluster) == 0 {
		return nil
	}
	concurrent := len(cluster) > 1
	if concurrent {
		first, okFirst := parseResumeTime(cluster[0].at)
		for i := 1; i < len(cluster) && concurrent; i++ {
			at, ok := parseResumeTime(cluster[i].at)
			if !okFirst || !ok || at.Sub(first) > 5*time.Second {
				concurrent = false
			}
		}
	}
	out := make([]ProviderEvent, 0, len(cluster)*2)
	if concurrent {
		for _, p := range cluster {
			out = append(out, p.spawn)
		}
		for _, p := range cluster {
			if p.hasRes {
				out = append(out, p.result)
			}
		}
		return out
	}
	for _, p := range cluster {
		out = append(out, p.spawn)
		if p.hasRes {
			out = append(out, p.result)
		}
	}
	return out
}

func (s *InteractiveService) reorderResumedTimelineLocked(rs *interactiveRun) {
	if len(rs.events) < 2 || !allEventsHaveOccurredAt(rs.events) {
		return
	}
	sort.SliceStable(rs.events, func(i, j int) bool {
		return rs.events[i].OccurredAt < rs.events[j].OccurredAt
	})
	s.resequenceResumedTimelineLocked(rs)
}

func (s *InteractiveService) resequenceResumedTimelineLocked(rs *interactiveRun) {
	for i := range rs.events {
		rs.events[i].Seq = int64(i + 1)
	}
	rs.seq = int64(len(rs.events))
}

func allEventsHaveOccurredAt(events []ProviderEvent) bool {
	for _, event := range events {
		if strings.TrimSpace(event.OccurredAt) == "" {
			return false
		}
	}
	return true
}

// reorderSidecarPrefixToEnd moves the CP-41/question sidecar events that
// reconstructRun had to seed into rs.events before any real transcript was
// loaded (rs.sidecarPrefixCount, see reconstructRun) to the end of the
// timeline, after everything seedTranscriptFromDisk just appended. When every
// restored event has a durable timestamp, it instead restores exact chronology.
//
// reconstructRun runs before seedTranscriptFromDisk (they are two separate
// calls the caller makes back to back — resumeRun, loadHandoffSourceRun), so
// it has no choice but to assign the sidecar/question events the lowest Seq
// numbers in rs.events. Left there, a restored resolved user_question_required
// (BUG-271/Task-183) permanently renders pinned above the entire prior
// conversation on the desktop after a full server restart, regardless of when
// it actually happened, because the client (timelineReducer) simply appends
// events in the Seq order the server streams them — it does not reorder.
//
// This is not a perfect chronological fix: the transcript loader does not
// preserve real per-turn timestamps to interleave sidecar events against (all
// replayed transcript events share one OccurredAt, rs.createdAt), so exact
// interleaving isn't recoverable here. But moving the sidecar prefix after the
// replayed transcript is uniformly closer to correct than always-first, and
// matches what these events actually are — durable *current* state (the
// latest flow-context package, validation result, audit draft, or question),
// not history.
//
// A no-op when nothing was appended after reconstructRun (e.g. Grok/default
// today, Task-212 DOD-5/DOD-9 — there is no transcript loader for Grok yet, so
// there is nothing to move the prefix after).
func (s *InteractiveService) reorderSidecarPrefixToEnd(rs *interactiveRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Provider transcript frames do not always carry their original timestamp
	// (Grok is one such provider). The durable gate event's ProviderTurnID is a
	// stronger ordering key than a fallback timestamp: put it back directly after
	// the prompt for that same turn. This keeps resolved approval/question cards
	// in their original conversation turn on restart.
	//
	// BUG residual run-35329: this used to gate on runKind=="chat" only. Workflow
	// / flow-engine hubs (post-done follow-up on a Review Loop, etc.) also restore
	// permission_required from the flow-events sidecar and then seed transcript —
	// without turn-anchoring those cards were always moved to the absolute end of
	// the timeline after every later follow-up, even when ProviderTurnID matched.
	// Anchor for every run kind; fall through when no turn ids match.
	if s.anchorSidecarPrefixByTurnLocked(rs) {
		return
	}
	if allEventsHaveOccurredAt(rs.events) {
		s.reorderResumedTimelineLocked(rs)
		return
	}
	n := int(rs.sidecarPrefixCount)
	rs.sidecarPrefixCount = 0
	if n <= 0 || n >= len(rs.events) {
		return
	}
	reordered := make([]ProviderEvent, 0, len(rs.events))
	reordered = append(reordered, rs.events[n:]...)
	reordered = append(reordered, rs.events[:n]...)
	for i := range reordered {
		reordered[i].Seq = int64(i + 1)
	}
	rs.events = reordered
	rs.seq = int64(len(reordered))
}

// anchorSidecarPrefixByTurnLocked restores durable approval/question events
// that reconstructRun loaded before the provider transcript. A transcript is
// replayed in provider order, but the sidecar carries the authoritative turn
// id. Keep unmatched sidecar events at the end as a conservative fallback.
//
// Formerly chat-only (anchorChatSidecarPrefixByTurnLocked); renamed when the
// workflow/flow path was included (run-35329 residual).
func (s *InteractiveService) anchorSidecarPrefixByTurnLocked(rs *interactiveRun) bool {
	n := int(rs.sidecarPrefixCount)
	if n <= 0 || n >= len(rs.events) {
		return false
	}

	sidecars := rs.events[:n]
	transcript := rs.events[n:]
	byTurn := make(map[string][]ProviderEvent)
	var unmatched []ProviderEvent
	for _, event := range sidecars {
		turnID := strings.TrimSpace(event.ProviderTurnID)
		if turnID == "" {
			unmatched = append(unmatched, event)
			continue
		}
		byTurn[turnID] = append(byTurn[turnID], event)
	}
	if len(byTurn) == 0 {
		return false
	}

	reordered := make([]ProviderEvent, 0, len(rs.events))
	anchored := 0
	usedTurnIDs := make(map[string]bool)
	for _, event := range transcript {
		reordered = append(reordered, event)
		if event.Type != EventTurnStarted {
			continue
		}
		turnID := strings.TrimSpace(event.ProviderTurnID)
		if turnID == "" || len(byTurn[turnID]) == 0 {
			continue
		}
		reordered = append(reordered, byTurn[turnID]...)
		anchored += len(byTurn[turnID])
		usedTurnIDs[turnID] = true
		delete(byTurn, turnID)
	}
	for _, event := range sidecars {
		turnID := strings.TrimSpace(event.ProviderTurnID)
		if turnID != "" && !usedTurnIDs[turnID] {
			unmatched = append(unmatched, event)
		}
	}
	reordered = append(reordered, unmatched...)
	if anchored == 0 {
		return false
	}
	rs.events = reordered
	rs.sidecarPrefixCount = 0
	s.resequenceResumedTimelineLocked(rs)
	return true
}

// seedGrokTranscriptFromDisk replays a resumed Grok chat's history from its
// per-session chat_history.jsonl files (BUG-GrokReplay-Restart / Task-212 T-6).
//
// Flow hubs (parent, flowEngineDriven) with durable transcript_turn assistants
// rebuild prose from the per-run turn log instead of the provider session file.
// Grok can bind the hub and a flow child to the SAME ACP session id (run-24377:
// hub run-24377 and coder run-24382 both used 019f8526-…); loading that file
// for the hub replays child turns into main chat and scrambles order after
// restart. When the turn log has no hub assistants yet, fall through to the
// provider-history path (run-12613 / run-20332) but filterFlowHubUnassistedProviderHistory
// still drops sibling/foreign frames from a shared session (run-98153).
func (s *InteractiveService) seedGrokTranscriptFromDisk(rs *interactiveRun) {
	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		home, ok = defaultProviderSessionHome(rs.providerKey)
	}
	if !ok {
		return
	}

	var sessionIDs []string
	var rawPrompts []turnLogLine
	var allEntries []turnLogLine
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			allEntries = entries
			seen := map[string]bool{}
			for _, e := range entries {
				if e.Kind == turnLogKindPrompt && strings.TrimSpace(e.Prompt) != "" && !isSystemPrompt(e.Prompt) {
					rawPrompts = append(rawPrompts, e)
				}
				if e.Kind == turnLogKindGrokSession && e.SessionID != "" && !seen[e.SessionID] {
					seen[e.SessionID] = true
					sessionIDs = append(sessionIDs, e.SessionID)
				}
			}
		}
	}
	if s.preferFlowHubTurnLogTranscript(rs, allEntries) {
		s.seedFlowHubTranscriptFromTurnLog(rs, allEntries)
		return
	}
	// run-75035 parity: drop parent/sibling session ids that leaked into the
	// child turn log (write path already hardened for Grok; filter still
	// protects historical pollution).
	sessionIDs = s.filterOwnedProviderSessionIDs(rs, sessionIDs)
	if len(sessionIDs) == 0 {
		// Never fall back to workspace-wide discovery for child runs — that is
		// the shared-cwd theft class (run-536 / run-75035).
		if strings.TrimSpace(rs.parentRunID) == "" {
			sessionIDs = discoverGrokSessionDirs(home, rs.workspaceCwd)
			sessionIDs = s.filterOwnedProviderSessionIDs(rs, sessionIDs)
		}
	}
	if len(sessionIDs) == 0 {
		historical := mergeTurnLogAssistantsIntoTranscript(promptOnlyTurnLogEvents(rawPrompts), allEntries)
		historical = userFacingTranscriptEvents(historical)
		if len(historical) > 0 {
			s.appendTranscriptReplayEvents(rs, stampReplayPromptIDs(historical))
		}
		return
	}

	var historical []ProviderEvent
	for _, sid := range sessionIDs {
		gp := grokChatHistoryPath(home, rs.workspaceCwd, sid)
		ev, loadErr := loadGrokTranscriptEvents(gp)
		if loadErr != nil {
			fmt.Printf("[transcript-load] partial read run=%s file=%s: %v\n", rs.id, gp, loadErr)
		}
		historical = append(historical, ev...)
	}
	// BUG-384: every SendTurn retry re-issues session/prompt and Grok persists
	// each attempt as a <user_query> frame. Collapse adjacent identical user
	// turns before overlay so surplus frames can't surface as phantom turns
	// holding the full composed prompt.
	historical = dedupeRetryDuplicatedTurnStarts(historical)
	// Strip sibling/foreign frames BEFORE overlayRawTurnPrompts: overlay maps
	// durable prompts onto the first overlayable user slot and can relabel a
	// foreign chat as the hub prompt, then keep the following foreign assistant
	// (run-98153). Assistant-only dumps (run-12613) stay intact.
	if shouldFilterFlowHubProviderHistory(rs, allEntries) {
		historical = filterFlowHubUnassistedProviderHistory(historical, turnLogPromptTexts(rawPrompts))
	}
	historical = overlayRawGrokTurnPrompts(historical, rawPrompts)
	historical = prependMissingPromptOnlyEvents(turnLogPromptTexts(rawPrompts), historical)
	if len(allEntries) > 0 {
		historical = mergeTurnLogAssistantsIntoTranscript(historical, allEntries)
	}
	historical = userFacingTranscriptEvents(historical)
	if len(historical) == 0 {
		return
	}
	s.appendTranscriptReplayEvents(rs, stampReplayPromptIDs(historical))
}

func (s *InteractiveService) seedOpencodeTranscriptFromDisk(rs *interactiveRun) {
	// Opencode: no provider-owned transcript file loader yet (unlike Grok's chat_history.jsonl).
	// Rebuild from durable turn-log prompts/assistants so reopen UI is not empty.
	// ACP session/load still continues the provider thread via LastOpencodeSessionID.
	var rawPrompts []turnLogLine
	var allEntries []turnLogLine
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			allEntries = entries
			for _, e := range entries {
				if e.Kind == turnLogKindPrompt && strings.TrimSpace(e.Prompt) != "" && !isSystemPrompt(e.Prompt) {
					rawPrompts = append(rawPrompts, e)
				}
			}
		}
	}
	if s.preferFlowHubTurnLogTranscript(rs, allEntries) {
		s.seedFlowHubTranscriptFromTurnLog(rs, allEntries)
		return
	}
	historical := mergeTurnLogAssistantsIntoTranscript(promptOnlyTurnLogEvents(rawPrompts), allEntries)
	historical = userFacingTranscriptEvents(historical)
	if len(historical) == 0 {
		return
	}
	s.appendTranscriptReplayEvents(rs, stampReplayPromptIDs(historical))
}

// seedDevinTranscriptFromDisk rebuilds a Devin run's transcript from the
// durable turn log (CP-70/Task-401). Devin keeps sessions in the shared
// cli/sessions.db SQLite store — there is no provider-owned transcript file,
// so replay mirrors seedOpencodeTranscriptFromDisk exactly. ACP session/load
// still continues the provider thread via LastDevinSessionID.
func (s *InteractiveService) seedDevinTranscriptFromDisk(rs *interactiveRun) {
	var rawPrompts []turnLogLine
	var allEntries []turnLogLine
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			allEntries = entries
			for _, e := range entries {
				if e.Kind == turnLogKindPrompt && strings.TrimSpace(e.Prompt) != "" && !isSystemPrompt(e.Prompt) {
					rawPrompts = append(rawPrompts, e)
				}
			}
		}
	}
	if s.preferFlowHubTurnLogTranscript(rs, allEntries) {
		s.seedFlowHubTranscriptFromTurnLog(rs, allEntries)
		return
	}
	historical := mergeTurnLogAssistantsIntoTranscript(promptOnlyTurnLogEvents(rawPrompts), allEntries)
	historical = userFacingTranscriptEvents(historical)
	if len(historical) == 0 {
		return
	}
	s.appendTranscriptReplayEvents(rs, stampReplayPromptIDs(historical))
}

// preferFlowHubTurnLogTranscript is true when this is a flow hub with durable
// per-run assistant frames — safe to rebuild without the (possibly shared)
// provider session file. Provider-agnostic: no providerKey branch.
func (s *InteractiveService) preferFlowHubTurnLogTranscript(rs *interactiveRun, entries []turnLogLine) bool {
	if rs == nil || strings.TrimSpace(rs.parentRunID) != "" || !rs.flowEngineDriven {
		return false
	}
	return turnLogHasAssistantFrames(entries)
}

// shouldFilterFlowHubProviderHistory is true when a flow hub falls through to
// provider history because the turn log has no assistant frames yet (run-12613
// class). Those hubs still need provider dumps for hub prose, but must not keep
// interleaved sibling/foreign turns from a shared session file (run-98153).
func shouldFilterFlowHubProviderHistory(rs *interactiveRun, entries []turnLogLine) bool {
	if rs == nil || strings.TrimSpace(rs.parentRunID) != "" || !rs.flowEngineDriven {
		return false
	}
	return !turnLogHasAssistantFrames(entries)
}

// filterFlowHubUnassistedProviderHistory keeps hub-owned frames when a flow hub
// loads provider history without durable assistants:
//   - assistant-only dumps (run-12613): keep message_completed
//   - interleaved foreign/child turns (run-98153): keep only turn_started that
//     match durable user prompts and the frames that follow them until the next
//     user turn
//   - remappable hub history (run-20332): provider user text differs from the
//     durable turn-log prompts but the non-system user-turn count equals the
//     durable count and *no* text match exists — keep in order so overlay can
//     remap prompts (do not apply when extras exist; that is pollution).
func filterFlowHubUnassistedProviderHistory(historical []ProviderEvent, allowedPrompts []string) []ProviderEvent {
	if len(historical) == 0 {
		return nil
	}
	hasUser := false
	for _, e := range historical {
		if e.Type == EventTurnStarted && strings.TrimSpace(e.Prompt) != "" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		out := make([]ProviderEvent, 0, len(historical))
		for _, e := range historical {
			if e.Type == EventMessageCompleted && strings.TrimSpace(e.Text) != "" {
				out = append(out, e)
			}
		}
		return out
	}
	allowed := make([]string, 0, len(allowedPrompts))
	for _, p := range allowedPrompts {
		if p = strings.TrimSpace(p); p != "" {
			allowed = append(allowed, p)
		}
	}
	// Count non-system user turns and whether any already match durable prompts.
	userCount := 0
	anyTextMatch := false
	for _, e := range historical {
		if e.Type != EventTurnStarted || strings.TrimSpace(e.Prompt) == "" {
			continue
		}
		if isSystemPrompt(e.Prompt) {
			continue
		}
		userCount++
		if promptMatchesAllowedHub(e.Prompt, allowed) {
			anyTextMatch = true
		}
	}
	// Positional remappable only when counts align and nothing text-matches
	// (overlay will rewrite user text). Extra foreign users → text mode only.
	positional := !anyTextMatch && len(allowed) > 0 && userCount == len(allowed)

	out := make([]ProviderEvent, 0, len(historical))
	keep := false
	userIdx := 0
	for _, e := range historical {
		switch e.Type {
		case EventTurnStarted:
			if isSystemPrompt(e.Prompt) {
				keep = false
				continue
			}
			if strings.TrimSpace(e.Prompt) == "" {
				keep = false
				continue
			}
			if positional {
				if userIdx < len(allowed) {
					keep = true
					out = append(out, e)
				} else {
					keep = false
				}
				userIdx++
				continue
			}
			if promptMatchesAllowedHub(e.Prompt, allowed) {
				keep = true
				out = append(out, e)
			} else {
				keep = false
			}
		default:
			if keep {
				out = append(out, e)
			}
		}
	}
	return out
}

func promptMatchesAllowedHub(prompt string, allowed []string) bool {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return false
	}
	if rest, wrapped := stripAgentContextBlock(p); wrapped {
		p = strings.TrimSpace(rest)
	}
	if p == "" {
		return false
	}
	if len(allowed) == 0 {
		return false
	}
	for _, a := range allowed {
		if p == a || strings.Contains(p, a) {
			return true
		}
	}
	return false
}

func turnLogHasAssistantFrames(entries []turnLogLine) bool {
	for _, e := range entries {
		switch e.Kind {
		case turnLogKindAssistant, turnLogKindTranscriptTurn:
			if strings.TrimSpace(e.Assistant) != "" {
				return true
			}
		}
	}
	return false
}

// seedFlowHubTranscriptFromTurnLog rebuilds hub main-chat prose from the durable
// turn log only (authoritative, per-run). Join/orchestration prompts are skipped
// (isSystemPrompt); their transcript_turn assistants still surface as hub
// synthesis bubbles. Agent cards are added later by appendResumedParentAnnotations.
// Shared by Grok and Claude resume paths (cross-provider-parity Case 1).
func (s *InteractiveService) seedFlowHubTranscriptFromTurnLog(rs *interactiveRun, entries []turnLogLine) {
	if rs == nil || len(entries) == 0 {
		return
	}
	historical := buildFlowHubTranscriptEventsFromTurnLog(entries)
	historical = userFacingTranscriptEvents(historical)
	if len(historical) == 0 {
		return
	}
	// Leave OccurredAt empty: agent cards use cluster-before-message placement
	// (run-9034/run-24377). Stamping everything with createdAt makes time-sort
	// reorder dump all children after hub prose.
	s.appendTranscriptReplayEventsOpt(rs, stampReplayPromptIDs(historical), false)
}

// buildFlowHubTranscriptEventsFromTurnLog walks the hub turn log in order and
// emits user-facing turn_started for non-system prompts plus message_completed
// for every durable assistant frame (including synthesis after a system join
// prompt that itself never becomes a bubble).
func buildFlowHubTranscriptEventsFromTurnLog(entries []turnLogLine) []ProviderEvent {
	out := make([]ProviderEvent, 0, len(entries)*2)
	for _, e := range entries {
		turnID := strings.TrimSpace(e.TurnID)
		switch e.Kind {
		case turnLogKindPrompt:
			prompt := strings.TrimSpace(e.Prompt)
			if prompt == "" || isSystemPrompt(prompt) {
				continue
			}
			out = append(out, ProviderEvent{
				Type:           EventTurnStarted,
				Prompt:         prompt,
				ProviderTurnID: turnID,
			})
		case turnLogKindAssistant, turnLogKindTranscriptTurn:
			text := strings.TrimSpace(e.Assistant)
			if text == "" {
				continue
			}
			out = append(out, ProviderEvent{
				Type:           EventMessageCompleted,
				Text:           text,
				ProviderTurnID: turnID,
			})
		}
	}
	return out
}

func stampReplayPromptIDs(historical []ProviderEvent) []ProviderEvent {
	var promptN int
	for i := range historical {
		if historical[i].Type == EventTurnStarted && historical[i].Prompt != "" {
			promptN++
			if strings.TrimSpace(historical[i].ProviderTurnID) == "" {
				historical[i].ProviderTurnID = fmt.Sprintf("replay-prompt-%d", promptN)
			}
		}
	}
	return historical
}

// mergeTurnLogAssistantsIntoTranscript fills provider-history gaps from the
// durable turn log. A transcript_turn is keyed by TurnID, not response text:
// equal responses may occur in separate turns, and a missing early response
// must stay before the next turn instead of being appended at the transcript tail.
func mergeTurnLogAssistantsIntoTranscript(historical []ProviderEvent, entries []turnLogLine) []ProviderEvent {
	out := append([]ProviderEvent(nil), historical...)
	for entryIndex, e := range entries {
		var text string
		switch e.Kind {
		case turnLogKindAssistant, turnLogKindTranscriptTurn:
			text = strings.TrimSpace(e.Assistant)
		default:
			continue
		}
		if text == "" {
			continue
		}
		turnID := strings.TrimSpace(e.TurnID)
		if turnID != "" && transcriptHasAssistantForTurn(out, turnID) {
			continue
		}
		// BUG-303: the ID-based check above only matches a historical event that
		// itself carries a ProviderTurnID. A historical event loaded from the raw
		// provider transcript can lack one entirely (older format, or a synthesis
		// turn that was never stamped) even when THIS turn-log entry has a durable
		// TurnID — in that mixed case the ID check above always misses, and this
		// entry's real duplicate went undetected because the old text fallback
		// below only ran when e.TurnID itself was empty, not when the candidate
		// historical event was untagged. Match by text against an UNTAGGED
		// historical event specifically (never one already carrying a DIFFERENT
		// turn id — that is a distinct, real event whose text coincidentally
		// matches, not this entry's duplicate; see
		// TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider,
		// which two separate turns both say "Acknowledged.").
		if transcriptHasUntaggedAssistantText(out, text) {
			continue
		}
		fallback := ProviderEvent{Type: EventMessageCompleted, Text: text, ProviderTurnID: turnID}
		if insertAt := transcriptAssistantInsertIndex(out, entries, entryIndex, turnID); insertAt >= 0 {
			out = append(out, ProviderEvent{})
			copy(out[insertAt+1:], out[insertAt:])
			out[insertAt] = fallback
			continue
		}
		out = append(out, fallback)
	}
	return out
}

func transcriptHasAssistantForTurn(events []ProviderEvent, turnID string) bool {
	for _, event := range events {
		if event.Type == EventMessageCompleted && strings.TrimSpace(event.ProviderTurnID) == turnID {
			return true
		}
	}
	return false
}

// transcriptHasUntaggedAssistantText reports whether an UNTAGGED (no
// ProviderTurnID of its own) EventMessageCompleted with this exact text
// already exists. Deliberately excludes an event already carrying a
// DIFFERENT turn id: that is a distinct, real event whose text happens to
// coincide, not this entry's duplicate (BUG-303).
func transcriptHasUntaggedAssistantText(events []ProviderEvent, text string) bool {
	for _, event := range events {
		if event.Type == EventMessageCompleted && strings.TrimSpace(event.ProviderTurnID) == "" && strings.TrimSpace(event.Text) == text {
			return true
		}
	}
	return false
}

// transcriptAssistantInsertIndex returns the causal slot for a missing durable
// response: after its own turn's current frames, or after the nearest earlier
// durable turn already represented in the provider history. A -1 means no
// durable position is known and the caller uses a conservative tail fallback.
func transcriptAssistantInsertIndex(events []ProviderEvent, entries []turnLogLine, entryIndex int, turnID string) int {
	if turnID != "" {
		lastOwn := -1
		firstTerminal := -1
		for i, event := range events {
			if strings.TrimSpace(event.ProviderTurnID) == turnID {
				lastOwn = i
				// BUG-435: a message_completed recovered from the turn log must
				// land BEFORE the turn's terminal event — inserting after it
				// produced turn_completed → message_completed and the tail
				// close then appended a duplicate turn_completed.
				if firstTerminal < 0 && (event.Type == EventTurnCompleted || event.Type == EventTurnFailed) {
					firstTerminal = i
				}
			}
		}
		if firstTerminal >= 0 {
			return firstTerminal
		}
		if lastOwn >= 0 {
			return lastOwn + 1
		}
	}
	for i := entryIndex - 1; i >= 0; i-- {
		previousTurnID := strings.TrimSpace(entries[i].TurnID)
		if previousTurnID == "" {
			continue
		}
		lastPrevious := -1
		for eventIndex, event := range events {
			if strings.TrimSpace(event.ProviderTurnID) == previousTurnID {
				lastPrevious = eventIndex
			}
		}
		if lastPrevious >= 0 {
			return lastPrevious + 1
		}
	}
	return -1
}

func overlayRawGrokTurnPrompts(historical []ProviderEvent, rawPrompts []turnLogLine) []ProviderEvent {
	return overlayRawTurnPrompts(historical, rawPrompts)
}

// dedupeRetryDuplicatedTurnStarts collapses consecutive turn_started events
// whose prompts are byte-identical after trimming (BUG-384). Provider retry
// attempts re-send the same composed prompt; a transport-failed attempt leaves
// no assistant/tool events between frames, so identical adjacent turn_starts
// are retry artifacts, not distinct user turns. Keeping the FIRST frame
// preserves overlay alignment (overlayRawTurnPrompts replaces the prompt text
// anyway) and the durable turn count stays authoritative: a genuine re-typed
// identical prompt with no intervening reply collapses here but is restored by
// prependMissingPromptOnlyEvents as a prompt-only event.
func dedupeRetryDuplicatedTurnStarts(events []ProviderEvent) []ProviderEvent {
	if len(events) < 2 {
		return events
	}
	out := events[:0]
	for _, e := range events {
		if e.Type == EventTurnStarted && len(out) > 0 {
			prev := out[len(out)-1]
			if prev.Type == EventTurnStarted &&
				strings.TrimSpace(prev.Prompt) != "" &&
				strings.TrimSpace(prev.Prompt) == strings.TrimSpace(e.Prompt) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// stripAgentContextBlock removes composeAgentContextBlock's "[FlowPilot system
// note — sub-agents …]" prefix (BUG-122) when present and returns the trailing
// genuine prompt plus whether the prefix was found. The prefix is context folded
// onto a REAL user turn (its user text follows the note), so on reconstruction it
// must not be mistaken for a pure orchestration prompt (BUG-306). Uses the shared
// agentContextBlock* constants so it cannot drift from the wrapper text.
func stripAgentContextBlock(prompt string) (remainder string, wrapped bool) {
	p := strings.TrimSpace(prompt)
	// isFlowEnginePrompt accepts both the em-dash and hyphen spellings; mirror that.
	if !strings.HasPrefix(p, agentContextBlockOpen) &&
		!strings.HasPrefix(p, "[FlowPilot system note - sub-agents") {
		return prompt, false
	}
	idx := strings.Index(p, agentContextBlockClose)
	if idx < 0 {
		return prompt, false
	}
	return strings.TrimSpace(p[idx+len(agentContextBlockClose):]), true
}

// isOverlayableUserTurn reports whether a historical turn_started slot is a genuine
// user turn that overlayRawTurnPrompts should replace with the raw turn-log prompt.
// A bare non-system prompt qualifies. A prompt carrying the agent-context note
// prefix also qualifies ONLY when the text after the note is itself a genuine user
// prompt — never when it wraps a join note / gate reprompt (BUG-300, run1264 keep
// those internal). isSystemPrompt itself is deliberately left unchanged so every
// transcript-hiding invariant elsewhere is preserved.
func isOverlayableUserTurn(prompt string) bool {
	if !isSystemPrompt(prompt) {
		return true
	}
	if rest, wrapped := stripAgentContextBlock(prompt); wrapped {
		return rest != "" && !isSystemPrompt(rest)
	}
	return false
}

// overlayRawTurnPrompts maps replayed provider prompts back to the durable raw
// user input recorded at startTurn. Provider transcript formats differ, but all
// providers share the same prompt order and must retain the same turn identity
// for restart-sidecar placement.
func overlayRawTurnPrompts(historical []ProviderEvent, rawPrompts []turnLogLine) []ProviderEvent {
	// BUG-306: rawPrompts is the complete ordered list of user prompts, but the
	// historical user-turn slots are only a SUFFIX of it — a flow hub's turn-1 is
	// suppressed (it spawns children without ever calling the provider, BUG-300), so
	// the leading prompt(s) have no slot. Aligning front-to-back therefore lands the
	// first raw prompt on the wrong slot. Count the overlay-able slots and start at
	// the matching offset so the last N raw prompts map to the N slots; the leading
	// suppressed prompt(s) fall through to prependMissingPromptOnlyEvents. When every
	// prompt has a slot (all normal chats, non-suppressed flows) the offset is 0 and
	// this is identical to the previous positional behavior.
	overlayable := 0
	for i := range historical {
		e := &historical[i]
		if e.Type == EventTurnStarted && strings.TrimSpace(e.Prompt) != "" && isOverlayableUserTurn(e.Prompt) {
			overlayable++
		}
	}
	promptIndex := 0
	if len(rawPrompts) > overlayable {
		promptIndex = len(rawPrompts) - overlayable
	}
	activeTurnID := ""
	for i := range historical {
		event := &historical[i]
		if event.Type == EventTurnStarted && strings.TrimSpace(event.Prompt) != "" {
			if !isOverlayableUserTurn(event.Prompt) {
				activeTurnID = ""
			} else if promptIndex < len(rawPrompts) {
				raw := rawPrompts[promptIndex]
				promptIndex++
				event.Prompt = raw.Prompt
				if turnID := strings.TrimSpace(raw.TurnID); turnID != "" {
					event.ProviderTurnID = turnID
				}
				activeTurnID = event.ProviderTurnID
			}
		}
		if activeTurnID != "" && strings.TrimSpace(event.ProviderTurnID) == "" {
			event.ProviderTurnID = activeTurnID
		}
	}
	return historical
}

// promptOnlyTurnLogEvents is the transcript-free resume fallback. Preserve a
// durable turn id when present; older runs without one retain the historical
// replay-prompt-N identity.
func promptOnlyTurnLogEvents(rawPrompts []turnLogLine) []ProviderEvent {
	out := make([]ProviderEvent, 0, len(rawPrompts)*2)
	for i, raw := range rawPrompts {
		prompt := strings.TrimSpace(raw.Prompt)
		if prompt == "" || isSystemPrompt(prompt) {
			continue
		}
		turnID := strings.TrimSpace(raw.TurnID)
		if turnID == "" {
			turnID = fmt.Sprintf("replay-prompt-%d", i+1)
		}
		out = append(out,
			ProviderEvent{Type: EventTurnStarted, Prompt: prompt, ProviderTurnID: turnID},
			ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID},
		)
	}
	return out
}

func turnLogPromptTexts(entries []turnLogLine) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if prompt := strings.TrimSpace(entry.Prompt); prompt != "" {
			out = append(out, prompt)
		}
	}
	return out
}

func prependMissingPromptOnlyEvents(rawPrompts []string, historical []ProviderEvent) []ProviderEvent {
	if len(rawPrompts) == 0 {
		return historical
	}
	seen := map[string]bool{}
	for _, e := range historical {
		if e.Type == EventTurnStarted {
			if p := strings.TrimSpace(e.Prompt); p != "" {
				seen[p] = true
			}
		}
	}
	missing := make([]string, 0, len(rawPrompts))
	for _, p := range rawPrompts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		missing = append(missing, p)
	}
	prefix := promptOnlyTranscriptEvents(missing)
	if len(prefix) == 0 {
		return historical
	}
	out := make([]ProviderEvent, 0, len(prefix)+len(historical))
	out = append(out, prefix...)
	out = append(out, historical...)
	return out
}

func (s *InteractiveService) seedGeminiTranscriptFromState(rs *interactiveRun) {
	turns := s.geminiTranscriptTurns(rs)
	if len(turns) == 0 {
		return
	}

	stepID := "chat-" + rs.id
	historical := make([]ProviderEvent, 0, len(turns)*3)
	for i, turn := range turns {
		turnID := fmt.Sprintf("gemini-replay-%d", i+1)
		if turn.User != "" {
			historical = append(historical, ProviderEvent{Type: EventTurnStarted, ProviderTurnID: turnID, Prompt: turn.User})
		}
		if turn.Assistant != "" {
			historical = append(historical, ProviderEvent{Type: EventMessageCompleted, ProviderTurnID: turnID, Text: turn.Assistant})
		}
		historical = append(historical, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: turn.Assistant})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if rs.transcriptSeeded {
		return
	}
	rs.transcriptSeeded = true
	sessionID := s.resumeSessionID(rs)
	for i := range historical {
		rs.seq++
		historical[i].Seq = rs.seq
		historical[i].ID = s.nextID("transcript")
		historical[i].WorkflowRunID = rs.id
		historical[i].WorkflowStepRunID = stepID
		historical[i].ProviderSessionID = sessionID
		historical[i].ProviderKey = rs.providerKey
		historical[i].OccurredAt = rs.updatedAt
		rs.events = append(rs.events, historical[i])
	}
}

func (s *InteractiveService) geminiTranscriptTurns(rs *interactiveRun) []transcriptTurn {
	type replayTurn struct {
		turnID string
		turn   transcriptTurn
	}
	var replay []replayTurn
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		if entries, err := logger.ReadTurnLog(context.Background(), rs.id); err == nil {
			promptByTurnID := make(map[string]string)
			for _, entry := range entries {
				if entry.Kind != turnLogKindPrompt {
					continue
				}
				turnID := strings.TrimSpace(entry.TurnID)
				prompt := strings.TrimSpace(entry.Prompt)
				if turnID != "" && prompt != "" && !isSystemPrompt(prompt) {
					promptByTurnID[turnID] = prompt
				}
			}
			for _, entry := range entries {
				switch entry.Kind {
				case turnLogKindTranscriptTurn:
					turnID := strings.TrimSpace(entry.TurnID)
					prompt := strings.TrimSpace(entry.Prompt)
					assistant := strings.TrimSpace(entry.Assistant)
					if isSystemPrompt(prompt) {
						prompt = ""
					}
					if prompt == "" && turnID != "" {
						prompt = promptByTurnID[turnID]
					}
					if prompt == "" && assistant == "" {
						continue
					}
					if len(replay) > 0 {
						if turnID != "" {
							for i := len(replay) - 1; i >= 0; i-- {
								if replay[i].turnID == turnID && strings.TrimSpace(replay[i].turn.Assistant) == "" {
									if replay[i].turn.User == "" {
										replay[i].turn.User = prompt
									}
									replay[i].turn.Assistant = assistant
									goto nextEntry
								}
							}
						}
						if prompt != "" {
							last := &replay[len(replay)-1]
							if strings.TrimSpace(last.turn.User) == prompt && strings.TrimSpace(last.turn.Assistant) == "" {
								last.turn.Assistant = assistant
								if last.turnID == "" {
									last.turnID = turnID
								}
								continue
							}
						}
						if prompt == "" && assistant != "" {
							for i := len(replay) - 1; i >= 0; i-- {
								if strings.TrimSpace(replay[i].turn.Assistant) == "" {
									replay[i].turn.Assistant = assistant
									if replay[i].turnID == "" {
										replay[i].turnID = turnID
									}
									goto nextEntry
								}
							}
						}
					}
					replay = append(replay, replayTurn{turnID: turnID, turn: transcriptTurn{User: prompt, Assistant: assistant}})
				case turnLogKindPrompt:
					turnID := strings.TrimSpace(entry.TurnID)
					prompt := strings.TrimSpace(entry.Prompt)
					if prompt == "" || isSystemPrompt(prompt) {
						continue
					}
					if turnID != "" {
						alreadyRecorded := false
						for i := range replay {
							if replay[i].turnID == turnID {
								if strings.TrimSpace(replay[i].turn.User) == "" {
									replay[i].turn.User = prompt
								}
								alreadyRecorded = true
								break
							}
						}
						if alreadyRecorded {
							continue
						}
					}
					replay = append(replay, replayTurn{turnID: turnID, turn: transcriptTurn{User: prompt}})
				case turnLogKindAssistant:
					assistant := strings.TrimSpace(entry.Assistant)
					if assistant == "" {
						continue
					}
					if len(replay) == 0 {
						replay = append(replay, replayTurn{turn: transcriptTurn{Assistant: assistant}})
						continue
					}
					paired := false
					for i := len(replay) - 1; i >= 0; i-- {
						if strings.TrimSpace(replay[i].turn.Assistant) == "" {
							replay[i].turn.Assistant = assistant
							paired = true
							break
						}
					}
					if !paired {
						replay = append(replay, replayTurn{turn: transcriptTurn{Assistant: assistant}})
					}
				}
			nextEntry:
			}
		}
	}
	turns := make([]transcriptTurn, 0, len(replay))
	for _, item := range replay {
		turns = append(turns, item.turn)
	}
	if len(turns) > 0 {
		last := &turns[len(turns)-1]
		if strings.TrimSpace(last.Assistant) == "" {
			last.Assistant = strings.TrimSpace(rs.lastMessage)
		}
		return turns
	}

	prompt := strings.TrimSpace(rs.lastPrompt)
	finalMessage := strings.TrimSpace(rs.lastMessage)
	if prompt == "" && finalMessage == "" {
		return nil
	}
	return []transcriptTurn{{User: prompt, Assistant: finalMessage}}
}

func geminiProjectConfigPathForHome(home, projectID string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return geminiProjectConfigPath(projectID, map[string]string{"HOME": home, "GEMINI_HOME": home})
}

func relocateGeminiProjectConfig(srcPath, targetHome, projectID string) (string, error) {
	dstPath := geminiProjectConfigPathForHome(targetHome, projectID)
	if strings.TrimSpace(dstPath) == "" {
		return "", os.ErrNotExist
	}
	if filepath.Clean(srcPath) == filepath.Clean(dstPath) {
		return srcPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dstPath); err == nil {
		same, cmpErr := sameFileContents(srcPath, dstPath)
		if cmpErr != nil {
			return "", cmpErr
		}
		if same {
			return dstPath, nil
		}
		return "", os.ErrExist
	} else if !os.IsNotExist(err) {
		return "", err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return dstPath, nil
}

func (s *InteractiveService) loadPersistedRun(runID string) (*interactiveRun, *apiErr) {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		log.Printf("[chat-history-open] persisted store unavailable run_id=%q", runID)
		return nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	st, found, err := reader.GetProviderSession(context.Background(), runID)
	if err != nil {
		log.Printf("[chat-history-open] persisted run read failed run_id=%q error=%q", runID, err)
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if !found {
		log.Printf("[chat-history-open] persisted run not found run_id=%q", runID)
		return nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	// BUG-170: workflow/flow-mode runs used to be rejected here with a blanket
	// resume_unsupported, so any history item other than a normal_chat run showed as
	// permanently unavailable once the runner restarted (in-memory state gone). The
	// reconstruction path below (reconstructRun / ensureResumeReady) is generic over
	// runKind already — it restores workflowID, flow loop state, and active flow
	// edges/nodes regardless — so there is no technical reason to gate resume on
	// runKind. The MVP-era restriction is lifted; see reconstructRun for the one
	// runKind-conditional step it still needs (the synthetic chat step seed).
	log.Printf(
		"[chat-history-open] persisted run loaded run_id=%q provider=%q provider_session_id=%q provider_account_id=%q status=%q sync_status=%q cwd=%q",
		st.RunID, st.ProviderKey, st.ProviderSessionID, st.ProviderAccountID, st.Status, st.SyncStatus, st.WorkingDirectory,
	)
	return s.reconstructRun(st)
}
