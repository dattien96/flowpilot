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
			if ok {
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
	if session.ProviderKey == ProviderKeyCodex {
		if logger, ok := s.workflowStore.(TurnLogStore); ok {
			if entries, err := logger.ReadTurnLog(context.Background(), runID); err == nil {
				for _, entry := range entries {
					if entry.Kind == turnLogKindCodexSession {
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

// normalizeResumedStatus maps an in-flight status read back from disk to a
// terminal one. A run that was running / starting / waiting for approval or a
// question cannot still be in flight after the owning process exited (server
// restart): the turn goroutine and any pending approval/question records are
// gone. Reading it back verbatim would leave the UI showing a spinner or a
// resolved-but-unanswerable prompt forever, so we surface it as cancelled.
func normalizeResumedStatus(status RunStatus) RunStatus {
	switch status {
	case RunStatusRunning, RunStatusStarting, RunStatusWaitingApproval, RunStatusWaitingQuestion:
		return RunStatusCancelled
	default:
		return status
	}
}

func (s *InteractiveService) reconstructRun(st ProviderSessionState) (*interactiveRun, *apiErr) {
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
		providerKey:            st.ProviderKey,
		providerSessionID:      st.ProviderSessionID,
		realProviderSessionID:  st.ProviderSessionID,
		lastCodexTurnSessionID: st.ProviderSessionID,
		providerAccountID:      st.ProviderAccountID,
		workspaceCwd:           st.WorkingDirectory,
		runKind:                st.RunKind,
		status:                 normalizeResumedStatus(st.Status),
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
		idempotency:            map[string]string{},
		resumedFromDisk:        true,
		pendingAgentContext:    append([]string(nil), st.PendingAgentContext...),
		autoOrchestrate:        st.AutoOrchestrate,
		flowCohortId:           st.FlowCohortID,
	}
	// Restore CP-41 flow events from the sidecar so FindFlowContextPackage,
	// FindAuditDraft etc. work after a process restart. rs is not yet visible to
	// other goroutines here so no lock is needed for the initial population.
	if fes, ok := s.workflowStore.(FlowEventStore); ok {
		if evs, _ := fes.LoadFlowEvents(context.Background(), st.RunID); len(evs) > 0 {
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

	s.mu.Lock()
	s.runs[rs.id] = rs
	if seeder, ok := s.workflowStore.(workflowRunSeeder); ok {
		seeder.seed(rs.id, []RuntimeWorkflowStep{{
			ID:               "chat-" + rs.id,
			StepType:         "chat",
			Status:           StepStatusPending,
			RequiresApproval: false,
		}})
	}
	s.mu.Unlock()
	// Restore flow-engine loop state so a restarted or Drive-synced run resumes
	// at the correct round/cap/mode (Task-085 T-4).
	if st.LoopState.Mode != "" || st.LoopState.Cap > 0 || st.LoopState.Round > 0 {
		s.agentOrchestrator.setLoop(rs.id, st.LoopState)
	}
	return rs, nil
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
	if rs.providerKey != ProviderKeyCodex {
		return s.resumeSessionID(rs) != ""
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
	if rs.providerKey == ProviderKeyGemini {
		s.seedGeminiTranscriptFromState(rs)
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
	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok && (strings.TrimSpace(rs.providerAccountID) == "" || rs.providerAccountID == "default") {
		home, ok = defaultProviderSessionHome(rs.providerKey)
	}
	if !ok {
		return
	}

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
		path, found := LocateSessionFile(rs.providerKey, home, sessionID, rs.workspaceCwd)
		if !found {
			return
		}
		filePaths = []string{path}
	}

	// Load events from all files.
	var historical []ProviderEvent
	for _, fp := range filePaths {
		historical = append(historical, loader(fp)...)
	}
	if len(historical) == 0 {
		return
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

	stepID := "chat-" + rs.id
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
	if st.RunKind != "chat" {
		log.Printf("[chat-history-open] persisted run unsupported run_id=%q run_kind=%q", runID, st.RunKind)
		return nil, newAPIErr(http.StatusConflict, "resume_unsupported", "only chat runs can be resumed in this version")
	}
	log.Printf(
		"[chat-history-open] persisted run loaded run_id=%q provider=%q provider_session_id=%q provider_account_id=%q status=%q sync_status=%q cwd=%q",
		st.RunID, st.ProviderKey, st.ProviderSessionID, st.ProviderAccountID, st.Status, st.SyncStatus, st.WorkingDirectory,
	)
	return s.reconstructRun(st)
}
