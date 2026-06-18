package runner

import (
	"context"
	"net/http"
	"strings"
	"time"
)

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
	rs := &interactiveRun{
		id:                    st.RunID,
		projectID:             st.ProjectID,
		workflowID:            st.WorkflowID,
		providerKey:           st.ProviderKey,
		providerSessionID:     st.ProviderSessionID,
		realProviderSessionID: st.ProviderSessionID,
		providerAccountID:     st.ProviderAccountID,
		workspaceCwd:          st.WorkingDirectory,
		runKind:               st.RunKind,
		status:                normalizeResumedStatus(st.Status),
		createdAt:             st.StartedAt,
		updatedAt:             now,
		lastPrompt:            st.LastPrompt,
		lastMessage:           st.LastMessage,
		sourceMachineID:       st.SourceMachineID,
		sourceRunID:           st.SourceRunID,
		restoredFrom:          st.RestoredFrom,
		syncStatus:            st.SyncStatus,
		syncUpdatedAt:         st.SyncUpdatedAt,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
		resumedFromDisk:       true,
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

func (s *InteractiveService) ensureResumeReady(rs *interactiveRun) *apiErr {
	srcHome, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		if rs.providerAccountID == s.activeAccountID {
			return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
		}
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	if !s.ensureProviderResumeHandle(rs, srcHome) {
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	sessionID := s.resumeSessionID(rs)
	srcPath, found := LocateSessionFile(rs.providerKey, srcHome, sessionID, rs.workspaceCwd)
	if !found {
		return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}

	if rs.providerAccountID == s.activeAccountID {
		if !HasLocalAuthAtPath(string(rs.providerKey), srcHome) {
			return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
		}
		return nil
	}
	return s.prepareCrossAccountResume(rs, srcHome, srcPath)
}

func (s *InteractiveService) prepareCrossAccountResume(rs *interactiveRun, srcHome, srcPath string) *apiErr {
	targetHome, ok := s.resolveAccountHome(rs.providerKey, s.activeAccountID)
	if !ok {
		return newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
	}
	if !HasLocalAuthAtPath(string(rs.providerKey), targetHome) {
		return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
	}
	if _, err := RelocateSessionFile(rs.providerKey, srcPath, targetHome, s.resumeSessionID(rs), rs.workspaceCwd); err != nil {
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	rs.providerAccountID = s.activeAccountID
	if snapErr := s.persistProviderSession(sessionStateOf(rs)); snapErr != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", snapErr.Error())
	}
	_ = srcHome
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

// seedTranscriptFromDisk loads the provider session file and populates rs.events
// so the SSE snapshot path replays the prior conversation to the desktop.
// Best-effort: any error is silently ignored to not block resume.
func (s *InteractiveService) seedTranscriptFromDisk(rs *interactiveRun) {
	if !rs.resumedFromDisk || rs.providerKey != ProviderKeyClaude {
		return
	}
	home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		return
	}
	sessionID := s.resumeSessionID(rs)
	filePath, found := LocateSessionFile(rs.providerKey, home, sessionID, rs.workspaceCwd)
	if !found {
		return
	}
	historical := loadClaudeTranscriptEvents(filePath)
	if len(historical) == 0 {
		return
	}
	stepID := "chat-" + rs.id
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(rs.events) > 0 {
		return // concurrent call guard
	}
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
}

func (s *InteractiveService) loadPersistedRun(runID string) (*interactiveRun, *apiErr) {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	st, found, err := reader.GetProviderSession(context.Background(), runID)
	if err != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if !found {
		return nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if st.RunKind != "chat" {
		return nil, newAPIErr(http.StatusConflict, "resume_unsupported", "only chat runs can be resumed in this version")
	}
	return s.reconstructRun(st)
}
