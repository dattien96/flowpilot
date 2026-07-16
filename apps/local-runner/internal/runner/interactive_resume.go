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

	deleteOrder, sessionsByRun := s.collectDeleteRunTree(runID, session)
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

func (s *InteractiveService) collectDeleteRunTree(runID string, root ProviderSessionState) ([]string, map[string]ProviderSessionState) {
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
		if sessions, err := indexReader.ListAllProviderSessions(context.Background()); err == nil {
			for _, session := range sessions {
				if _, exists := sessionsByRun[session.RunID]; !exists {
					sessionsByRun[session.RunID] = session
				}
				addChild(session.ParentRunID, session.RunID)
			}
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
	return order, sessionsByRun
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
		if prompt == "" {
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

// childPendingGateNodeIDs returns parent flow node ids whose matching child
// sessions still have a pending approval or question on disk (BUG-288 #22).
// Approvals/questions are keyed by the child's RunID, so parent-scoped list
// APIs never surface them — resume must walk child sessions explicitly.
func (s *InteractiveService) childPendingGateNodeIDs(rs *interactiveRun) []string {
	if rs == nil || len(rs.activeFlowNodes) == 0 {
		return nil
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return nil
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil || len(sessions) == 0 {
		return nil
	}
	ahr, hasAHR := s.workflowStore.(ApprovalHistoryReader)
	qhr, hasQHR := s.workflowStore.(QuestionHistoryReader)
	if !hasAHR && !hasQHR {
		return nil
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
			if states, err := ahr.ListApprovalsByRun(context.Background(), session.RunID); err == nil && hasPendingGate(states, nil) {
				pending = true
			}
		}
		if !pending && hasQHR {
			if states, err := qhr.ListQuestionsByRun(context.Background(), session.RunID); err == nil && hasPendingGate(nil, states) {
				pending = true
			}
		}
		if pending {
			seen[nodeID] = true
			out = append(out, nodeID)
		}
	}
	return out
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

func (s *InteractiveService) resumedFlowStepRows(rs *interactiveRun, st ProviderSessionState) []RuntimeWorkflowStep {
	if len(rs.activeFlowNodes) == 0 {
		return nil
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
	flowComplete := !resumedFlowRunIncomplete(st) && strings.TrimSpace(st.LoopState.Status) != "blocked"
	rows := s.flowStepRowsFromNodes(context.Background(), rs.id, rs.activeFlowNodes, StepStatusPending, "")
	byID := make(map[string]*RuntimeWorkflowStep, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	if indexReader, ok := s.workflowStore.(SessionIndexReader); ok {
		if sessions, err := indexReader.ListAllProviderSessions(context.Background()); err == nil {
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
	return rows
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
	if len(st.ActiveFlowNodes) > 0 && strings.TrimSpace(st.LoopState.Status) == "done" {
		return RunStatusCompleted
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
	rs := &interactiveRun{
		id:                     st.RunID,
		projectID:              st.ProjectID,
		workflowID:             st.WorkflowID,
		parentRunID:            st.ParentRunID,
		agentName:              st.AgentName,
		label:                  st.Label,
		role:                   st.Role,
		dependsOn:              append([]string(nil), st.DependsOn...),
		agentStatus:            st.AgentStatus,
		modelName:              st.ModelName,
		providerKey:            st.ProviderKey,
		providerSessionID:      st.ProviderSessionID,
		realProviderSessionID:  st.ProviderSessionID,
		lastCodexTurnSessionID: st.ProviderSessionID,
		providerAccountID:      st.ProviderAccountID,
		workspaceCwd:           st.WorkingDirectory,
		runKind:                st.RunKind,
		status:                 normalizeResumedFlowStatus(st),
		createdAt:              st.StartedAt,
		updatedAt:              updatedAt,
		lastPrompt:             st.LastPrompt,
		lastMessage:            st.LastMessage,
		sourceMachineID:        st.SourceMachineID,
		sourceRunID:            st.SourceRunID,
		restoredFrom:           st.RestoredFrom,
		syncStatus:             st.SyncStatus,
		syncUpdatedAt:          st.SyncUpdatedAt,
		changeType:             st.ChangeType,
		sourceDocID:            st.SourceDocID,
		turnCount:              st.TurnCount,
		subs:                   map[int64]chan ProviderEvent{},
		// BUG-288 R16-P0: restore durable idempotency keys (not empty map).
		idempotency:            copyStringMap(st.IdempotencyKeys),
		resumedFromDisk:           true,
		pendingAgentContext:       append([]string(nil), st.PendingAgentContext...),
		pendingFlowGateSettle:     st.PendingFlowGateSettle,
		pendingFlowGateFinalMsg:   st.PendingFlowGateFinalMsg,
		pendingFlowGateOccurredAt: st.PendingFlowGateOccurredAt,
		pendingFlowGateTurnID:     st.PendingFlowGateTurnID,
		turnStartGitHead:          st.TurnStartGitHead,
		turnStartWorktree:         copyStringMap(st.TurnStartWorktree),
		pendingGateChangedFiles:   append([]string(nil), st.PendingGateChangedFiles...),
		stepID:                    st.StepID,
		lastTurnStepID:            st.LastTurnStepID,
		lastTurnID:                st.PendingFlowGateTurnID, // seed for gate materialize
		pendingGateRepromptPrompt: st.PendingGateRepromptPrompt,
		pendingGateRepromptStepID: st.PendingGateRepromptStepID,
		pendingGateCodePaths:      append([]string(nil), st.PendingGateCodePaths...),
		repromptAttempts:          st.RepromptAttempts,
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
		stopGeneration:                 st.StopGeneration,
		parentStopGenSeen:              st.ParentStopGenSeen,
		intentBlockedKind:              st.IntentBlockedKind,
		intentBlockedReason:            st.IntentBlockedReason,
		intentBlockedAt:                st.IntentBlockedAt,
		transitionLogDegraded:          st.TransitionLogDegraded,
		transitionLogDegradedAt:        st.TransitionLogDegradedAt,
		transitionLogDegradedReason:    st.TransitionLogDegradedReason,
		suppressAutoGateResume:          deferGate,
		autoOrchestrate:           st.AutoOrchestrate,
		flowCohortId:              st.FlowCohortID,
		activeFlowEdges:           append([]agentpack.FlowEdge(nil), st.ActiveFlowEdges...),
		activeFlowNodes:           append([]agentpack.FlowNode(nil), st.ActiveFlowNodes...),
		chatSubMode:               st.ChatSubMode,
		chatFlowRef:               st.ChatFlowRef,
		flowStartGitHead:          st.FlowStartGitHead,
		pendingRestartRunID:       st.PendingRestartRunID,
		pendingRestartPrompt:      st.PendingRestartPrompt,
		pendingRestartGen:         st.PendingRestartGen,
		flowContextInjected:       st.FlowContextInjected,
	}
	if rs.idempotency == nil {
		rs.idempotency = map[string]string{}
	}
	// V10R P1: re-engage flow executor when topology was restored (needed for
	// child approval resume to stamp parent step RUNNING after restart).
	if len(rs.activeFlowNodes) > 0 {
		rs.flowEngineDriven = true
	}
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
	if st.RunKind == "chat" {
		if seeder, ok := s.workflowStore.(workflowRunSeeder); ok {
			seeder.seed(rs.id, []RuntimeWorkflowStep{{
				ID:               "chat-" + rs.id,
				StepType:         "chat",
				Status:           StepStatusPending,
				RequiresApproval: false,
			}})
		}
	} else if len(rs.activeFlowNodes) > 0 {
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
		rows := s.resumedFlowStepRows(rs, st)
		var pendingApprovals []ProviderApprovalState
		var pendingQuestions []ProviderQuestionState
		if ahr, ok := s.workflowStore.(ApprovalHistoryReader); ok {
			if states, err := ahr.ListApprovalsByRun(context.Background(), st.RunID); err == nil {
				pendingApprovals = states
			}
		}
		if qhr, ok := s.workflowStore.(QuestionHistoryReader); ok {
			if states, err := qhr.ListQuestionsByRun(context.Background(), st.RunID); err == nil {
				pendingQuestions = states
			}
		}
		// BUG-288 #22 / V9-08: child gates under child RunID.
		childWaiting := s.childPendingGateNodeIDs(rs)
		keepWaiting := keepWaitingNodeIDsForResume(st, rs.activeFlowNodes, pendingApprovals, pendingQuestions, childWaiting...)
		if tlog, ok := s.workflowStore.(StepTransitionLogStore); ok {
			if lines, loadErr := tlog.LoadStepTransitions(context.Background(), rs.id); loadErr != nil {
				log.Printf("reconstructRun: LoadStepTransitions runID=%s: %v (falling back to evidence-walk)", rs.id, loadErr)
			} else if len(lines) > 0 {
				rows = applyStepTransitionReplay(rows, lines, keepWaiting)
				// Hub precedence when flow genuinely completed (mirrors evidence-walk):
				// hub PENDING → DONE. I-3: never promote FAILED/CANCELED.
				if normalizeResumedFlowStatus(st) == RunStatusCompleted {
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
	}
	// Restore flow-engine loop state so a restarted or Drive-synced run resumes
	// at the correct round/cap/mode (Task-085 T-4).
	if st.LoopState.Mode != "" || st.LoopState.Cap > 0 || st.LoopState.Round > 0 {
		s.agentOrchestrator.setLoop(rs.id, st.LoopState)
	}
	// V10R3 P0: reconstruct pending/cohort children BEFORE normalize so parent
	// is not Cancelled while children still need approval/gate/barrier.
	if rs.parentRunID == "" && len(rs.activeFlowNodes) > 0 {
		s.reconstructPendingChildSessions(rs.id)
	}
	// V10R P0: normalize AFTER child reconstruct; skip cancel when children pending.
	normalizeResumedFlowRun(s, rs, st)
	// V10 P0 / V10R4: schedule post-turn gate only after normalize, and only when
	// not suppressed for atomic cohort restore (parent path flushes later).
	if rs.pendingFlowGateSettle && !rs.suppressAutoGateResume {
		go s.resumePendingFlowGate(rs.id)
	}
	// V10R4 P1: durable gate-reprompt / approval-resume intents after restart.
	if !rs.suppressAutoGateResume {
		s.flushDurableTurnIntents(rs.id)
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
	// Never flush continuation while a durable card is still pending unless we
	// already recorded a decision for that card (reconcile two-write crash).
	if (rs.pendingApprovalID != "" || rs.pendingQuestionID != "") &&
		strings.TrimSpace(rs.pendingResumeDecision) == "" {
		s.mu.Unlock()
		return
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
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// Do NOT persist DeliveredGen before startTurn (P0-02).
	go s.startTurnClearingIntent(runID, stepID, prompt, kind, gen)
}

// clearIntentFieldsLocked clears one kind of durable intent (caller holds s.mu).
func clearIntentFieldsLocked(rs *interactiveRun, kind string) {
	if rs == nil {
		return
	}
	switch kind {
	case "reprompt":
		rs.pendingGateRepromptPrompt = ""
		rs.pendingGateRepromptStepID = ""
		rs.pendingGateRepromptGen = 0
		rs.pendingGateRepromptDeliveredGen = 0
		rs.pendingGateRepromptAcceptedTurn = ""
		rs.pendingGateRepromptFailCount = 0
		rs.pendingGateRepromptFailGen = 0
	case "resume":
		rs.pendingResumePrompt = ""
		rs.pendingResumeStepID = ""
		rs.pendingResumeGen = 0
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
	case "turn_in_progress", "gate_in_progress", "awaiting_user":
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
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()

	// Deterministic key so a crash after accept + restart cannot open a second
	// provider turn for the same intent generation.
	idem := fmt.Sprintf("durable-%s-%s-%d", runID, kind, gen)
	turnID, apiErr := s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", idem)
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
	// Accepted by startTurn: clear durable intent. Idempotency key prevents a
	// second provider turn if we crash before this persist lands.
	cleared := false
	_ = turnID
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
	busy := rs.turnInFlight || rs.pendingFlowGateSettle || rs.postTurnGateCancel != nil
	hasIntent := strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" ||
		strings.TrimSpace(rs.pendingResumePrompt) != ""
	s.mu.Unlock()
	if busy || !hasIntent {
		return
	}
	go s.flushDurableTurnIntents(runID)
}

// reconstructPendingChildSessions loads child sessions under parentRunID that
// still need live reconstruction after parent resume: pending post-turn gate,
// pending approval/question cards, and full reviewer cohorts (V10R / V10R3 P0).
// Idempotent — skips children already in s.runs.
func (s *InteractiveService) reconstructPendingChildSessions(parentRunID string) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil || len(sessions) == 0 {
		return
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
		return
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
		return nil
	}
	type childSession struct {
		runID       string
		agentName   string
		lastMessage string
		startedAt   string
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
			runID:       session.RunID,
			agentName:   firstNonEmptyResumeValue(session.AgentName, session.Role, "agent"),
			lastMessage: strings.TrimSpace(session.LastMessage),
			startedAt:   session.StartedAt,
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].startedAt == children[j].startedAt {
			return children[i].runID < children[j].runID
		}
		return children[i].startedAt < children[j].startedAt
	})
	out := make([]ProviderEvent, 0, len(children)*2)
	for _, child := range children {
		out = append(out, ProviderEvent{
			Type:       EventAgentSpawnedByUser,
			AgentName:  child.agentName,
			ChildRunID: child.runID,
		})
		if child.lastMessage != "" {
			out = append(out, ProviderEvent{
				Type:         EventAgentResultInjected,
				AgentName:    child.agentName,
				ChildRunID:   child.runID,
				FinalMessage: child.lastMessage,
			})
		}
	}
	return out
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
	activeAccountID := s.activeAccountForProvider(rs.providerKey)
	log.Printf(
		"[chat-history-open] resume check run_id=%q provider=%q stored_account_id=%q active_account_id=%q provider_session_id=%q cwd=%q resumed_from_disk=%t",
		rs.id, rs.providerKey, rs.providerAccountID, activeAccountID, s.resumeSessionID(rs), rs.workspaceCwd, rs.resumedFromDisk,
	)
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
	if rs.providerKey != ProviderKeyCodex {
		return s.resumeSessionID(rs) != ""
	}
	if s.shouldTreatCodexFlowHubSessionAsSynthetic(rs) {
		return true
	}
	if id := s.resumeSessionID(rs); id != "" && !strings.HasPrefix(id, "thread-") {
		return true
	}
	id, ok := DiscoverCodexRolloutSessionID(accountHome, rs.workspaceCwd)
	if !ok {
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
	var loader func(string) []ProviderEvent
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
	var rawPrompts []string
	var codexSessionIDs []string
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			for _, e := range entries {
				switch e.Kind {
				case turnLogKindPrompt:
					if e.Prompt != "" {
						rawPrompts = append(rawPrompts, e.Prompt)
					}
				case turnLogKindCodexSession:
					if e.SessionID != "" {
						codexSessionIDs = append(codexSessionIDs, e.SessionID)
					}
				}
			}
		}
	}

	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok && (strings.TrimSpace(rs.providerAccountID) == "" || rs.providerAccountID == "default") {
		home, ok = defaultProviderSessionHome(rs.providerKey)
	}
	if !ok {
		historical := promptOnlyTranscriptEvents(rawPrompts)
		if len(historical) == 0 {
			return
		}
		s.appendTranscriptReplayEvents(rs, historical)
		return
	}

	// Collect the session file path(s) to load.
	sessionID := s.resumeSessionID(rs) // used for event correlation below
	var filePaths []string
	if rs.providerKey == ProviderKeyCodex {
		// Load every per-turn rollout file in recorded order (F-3, BUG-083).
		seen := map[string]bool{}
		if sessionID != "" {
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
		historical = append(historical, loader(fp)...)
	}
	if len(historical) == 0 {
		historical = promptOnlyTranscriptEvents(rawPrompts)
		if len(historical) == 0 {
			return
		}
	}

	// Override each replayed prompt with the stored raw input (F-1, BUG-083).
	// rawPrompts[i] maps to the (i+1)th turn_started event that carries a prompt,
	// which is exactly the order startTurn appended them to the turn log.
	if len(rawPrompts) > 0 {
		promptIdx := 0
		for i := range historical {
			if historical[i].Type == EventTurnStarted && historical[i].Prompt != "" && promptIdx < len(rawPrompts) {
				historical[i].Prompt = rawPrompts[promptIdx]
				promptIdx++
			}
		}
	}

	s.appendTranscriptReplayEvents(rs, historical)
}

func (s *InteractiveService) appendTranscriptReplayEvents(rs *interactiveRun, historical []ProviderEvent) {
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
		historical[i].OccurredAt = rs.createdAt
		rs.events = append(rs.events, historical[i])
	}
	// Append a synthetic turn_completed to close any trailing "Thinking..." row.
	// timelineReducer.finalize injects a thinking row after every non-terminal event
	// (message_completed / tool_completed etc.), so without this the resumed idle chat
	// would render a perpetual spinner.
	if last := rs.events[len(rs.events)-1]; last.Type != EventTurnCompleted && last.Type != EventTurnFailed {
		rs.seq++
		rs.events = append(rs.events, ProviderEvent{
			Seq:               rs.seq,
			Type:              EventTurnCompleted,
			ID:                s.nextID("transcript"),
			WorkflowRunID:     rs.id,
			WorkflowStepRunID: stepID,
			ProviderSessionID: sessionID,
			ProviderKey:       rs.providerKey,
			OccurredAt:        rs.createdAt,
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
	for i := range annotations {
		rs.seq++
		annotations[i].Seq = rs.seq
		annotations[i].ID = s.nextID("transcript")
		annotations[i].WorkflowRunID = rs.id
		annotations[i].WorkflowStepRunID = stepID
		annotations[i].ProviderSessionID = sessionID
		annotations[i].ProviderKey = rs.providerKey
		annotations[i].OccurredAt = rs.createdAt
		rs.events = append(rs.events, annotations[i])
	}
}

// reorderSidecarPrefixToEnd moves the CP-41/question sidecar events that
// reconstructRun had to seed into rs.events before any real transcript was
// loaded (rs.sidecarPrefixCount, see reconstructRun) to the end of the
// timeline, after everything seedTranscriptFromDisk just appended.
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

// seedGrokTranscriptFromDisk replays a resumed Grok chat's history from its
// per-session chat_history.jsonl files (BUG-GrokReplay-Restart / Task-212 T-6).
// Grok writes one session dir per FlowPilot turn (see grok_transcript_loader.go),
// so this concatenates them in order — precise per-turn ids from the run's turn
// log when present, else a best-effort mtime-ordered discovery of every Grok
// session for the workspace (covers runs created before per-turn capture).
func (s *InteractiveService) seedGrokTranscriptFromDisk(rs *interactiveRun) {
	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		home, ok = defaultProviderSessionHome(rs.providerKey)
	}
	if !ok {
		return
	}

	var sessionIDs []string
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		if entries, _ := logger.ReadTurnLog(context.Background(), rs.id); len(entries) > 0 {
			seen := map[string]bool{}
			for _, e := range entries {
				if e.Kind == turnLogKindGrokSession && e.SessionID != "" && !seen[e.SessionID] {
					seen[e.SessionID] = true
					sessionIDs = append(sessionIDs, e.SessionID)
				}
			}
		}
	}
	// Fallback for runs with no per-turn ids logged (created before the capture
	// landed): replay every Grok session dir for this workspace, oldest-first.
	if len(sessionIDs) == 0 {
		sessionIDs = discoverGrokSessionDirs(home, rs.workspaceCwd)
	}
	if len(sessionIDs) == 0 {
		return
	}

	var historical []ProviderEvent
	for _, sid := range sessionIDs {
		historical = append(historical, loadGrokTranscriptEvents(grokChatHistoryPath(home, rs.workspaceCwd, sid))...)
	}
	if len(historical) == 0 {
		return
	}
	// Distinct replay turn ids so the desktop derives unique prompt bubble ids
	// ("prompt-${providerTurnId}") across concatenated per-turn session files.
	var promptN int
	for i := range historical {
		if historical[i].Type == EventTurnStarted && historical[i].Prompt != "" {
			promptN++
			historical[i].ProviderTurnID = fmt.Sprintf("replay-prompt-%d", promptN)
		}
	}
	s.appendTranscriptReplayEvents(rs, historical)
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
				if turnID != "" && prompt != "" {
					promptByTurnID[turnID] = prompt
				}
			}
			for _, entry := range entries {
				switch entry.Kind {
				case turnLogKindTranscriptTurn:
					turnID := strings.TrimSpace(entry.TurnID)
					prompt := strings.TrimSpace(entry.Prompt)
					assistant := strings.TrimSpace(entry.Assistant)
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
					if prompt == "" {
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
