package runner

import (
	"context"
	"net/http"
	"os"
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

func (s *InteractiveService) ensureResumeReady(rs *interactiveRun) *apiErr {
	// "The active account" must be scoped to this run's provider, not the single
	// global activeAccountID: a Codex chat is resumed against the active Codex
	// account regardless of which Claude/Gemini account is active (Task-067 issue 1).
	activeAccountID := s.activeAccountForProvider(rs.providerKey)
	srcHome, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
	if !ok {
		if rs.providerAccountID == activeAccountID {
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

	if rs.providerAccountID == activeAccountID {
		if !HasLocalAuthAtPath(string(rs.providerKey), srcHome) {
			return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
		}
		return nil
	}
	return s.prepareCrossAccountResume(rs, srcPath, activeAccountID)
}

func (s *InteractiveService) prepareCrossAccountResume(rs *interactiveRun, srcPath, activeAccountID string) *apiErr {
	targetHome, ok := s.resolveAccountHome(rs.providerKey, activeAccountID)
	if !ok {
		return newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
	}
	if !HasLocalAuthAtPath(string(rs.providerKey), targetHome) {
		return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
	}
	if _, err := RelocateSessionFile(rs.providerKey, srcPath, targetHome, s.resumeSessionID(rs), rs.workspaceCwd); err != nil {
		return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
	}
	rs.providerAccountID = activeAccountID
	if snapErr := s.persistProviderSession(sessionStateOf(rs)); snapErr != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", snapErr.Error())
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

// seedTranscriptFromDisk loads the provider session file and populates rs.events
// so the SSE snapshot path replays the prior conversation to the desktop.
// Best-effort: any error is silently ignored to not block resume.
func (s *InteractiveService) seedTranscriptFromDisk(rs *interactiveRun) {
	if !rs.resumedFromDisk {
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
	if !ok {
		return
	}
	sessionID := s.resumeSessionID(rs)
	filePath, found := LocateSessionFile(rs.providerKey, home, sessionID, rs.workspaceCwd)
	if !found {
		return
	}
	historical := loader(filePath)
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
