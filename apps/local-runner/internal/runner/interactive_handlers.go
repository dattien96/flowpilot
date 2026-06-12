package runner

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// RegisterInteractiveRoutes wires the Phase 2 interactive + admin endpoints onto
// the runner's mux (04-02). Uses Go 1.22+ method+wildcard patterns.
func (s *InteractiveService) RegisterInteractiveRoutes(mux *http.ServeMux) {
	// interactive (client)
	mux.HandleFunc("GET /client/projects", s.handleListProjects)
	mux.HandleFunc("GET /client/projects/{projectId}/workflows", s.handleListWorkflows)
	mux.HandleFunc("GET /client/workflows/{workflowId}/steps", s.handleListSteps)
	mux.HandleFunc("POST /client/workflow-runs", s.handleStartRun)
	mux.HandleFunc("GET /client/workflow-runs/{runId}", s.handleGetRun)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/resume", s.handleResumeRun)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/turns", s.handleStartTurn)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/events/stream", s.handleEventStream)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /client/approvals/{approvalId}/decision", s.handleApprovalDecision)
	mux.HandleFunc("POST /client/questions/{questionId}/answer", s.handleAnswerQuestion)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/artifacts", s.handleListArtifacts)
	mux.HandleFunc("GET /client/provider-skills", s.handleListSkills)

	// admin
	mux.HandleFunc("GET /admin/providers", s.handleAdminProviders)
	mux.HandleFunc("GET /admin/workflow-runs/{runId}/provider-sessions", s.handleAdminSessions)
	mux.HandleFunc("GET /admin/workflow-runs/{runId}/events", s.handleAdminEvents)
	mux.HandleFunc("GET /admin/workflow-runs/{runId}/approvals", s.handleAdminApprovals)
	mux.HandleFunc("GET /admin/workflow-runs/{runId}/questions", s.handleAdminQuestions)
}

// ---- HTTP helpers ----------------------------------------------------------

func writeInteractiveJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeInteractiveError(w http.ResponseWriter, e *apiErr) {
	writeInteractiveJSON(w, e.status, map[string]any{
		"error": map[string]any{"code": e.code, "message": e.msg},
	})
}

// ---- catalog handlers ------------------------------------------------------

func (s *InteractiveService) handleListProjects(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.catalog.listProjects())
}

func (s *InteractiveService) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.catalog.listWorkflows(r.PathValue("projectId")))
}

func (s *InteractiveService) handleListSteps(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.catalog.listSteps(r.PathValue("workflowId")))
}

func (s *InteractiveService) handleListSkills(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.catalog.listSkills())
}

func (s *InteractiveService) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	writeInteractiveJSON(w, http.StatusOK, fakeArtifacts(runID))
}

// ---- run lifecycle handlers ------------------------------------------------

func (s *InteractiveService) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var in StartRunInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, s.createRun(in))
}

func (s *InteractiveService) handleGetRun(w http.ResponseWriter, r *http.Request) {
	view, e := s.runSnapshot(r.PathValue("runId"))
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, view)
}

func (s *InteractiveService) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	handle, e := s.resumeRun(r.PathValue("runId"))
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, handle)
}

type turnBody struct {
	StepID         string           `json:"stepId"`
	Prompt         string           `json:"prompt"`
	SelectedSkills []SkillSelection `json:"selectedSkills"`
	Scenario       string           `json:"scenario"` // P2 fake-adapter hint only
}

func (s *InteractiveService) handleStartTurn(w http.ResponseWriter, r *http.Request) {
	var body turnBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	turnID, e := s.startTurn(
		r.PathValue("runId"),
		TurnInput{StepID: body.StepID, Prompt: body.Prompt, SelectedSkills: body.SelectedSkills},
		body.Scenario,
		r.Header.Get("Idempotency-Key"),
	)
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"turnId": turnID})
}

func (s *InteractiveService) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	if e := s.Interrupt(r.PathValue("runId")); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
}

func (s *InteractiveService) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if e := s.SubmitApprovalDecision(r.PathValue("approvalId"), body.Decision); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (s *InteractiveService) handleAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Choice json.RawMessage `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	choice, ok := parseChoice(raw.Choice)
	if !ok {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_decision", "choice must be a string or string array"))
		return
	}
	if e := s.AnswerQuestion(r.PathValue("questionId"), choice); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// parseChoice accepts a JSON string or string array (contract: string | string[]).
func parseChoice(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, true
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, true
	}
	return nil, false
}

// ---- SSE event stream ------------------------------------------------------

func (s *InteractiveService) handleEventStream(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	after := parseAfterSeq(r)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "stream_unsupported", "streaming not supported"))
		return
	}

	subID, ch, snapshot, found := s.subscribe(runID, after)
	if !found {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found"))
		return
	}
	defer s.unsubscribe(runID, subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var lastSeq int64 = after
	for _, ev := range snapshot {
		writeSSE(w, ev)
		lastSeq = ev.Seq
	}
	flusher.Flush()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Seq <= lastSeq { // dedupe overlap between snapshot and live
				continue
			}
			writeSSE(w, ev)
			lastSeq = ev.Seq
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, ev ProviderEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	// id: <seq> lets the browser send Last-Event-ID on reconnect.
	_, _ = w.Write([]byte("id: " + strconv.FormatInt(ev.Seq, 10) + "\n"))
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
}

func parseAfterSeq(r *http.Request) int64 {
	if v := r.URL.Query().Get("afterSeq"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// ---- admin handlers --------------------------------------------------------

func (s *InteractiveService) handleAdminProviders(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.registry.List())
}

func (s *InteractiveService) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[r.PathValue("runId")]
	if rs == nil {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found"))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, []map[string]any{{
		"providerSessionId": rs.providerSessionID,
		"providerKey":       rs.providerKey,
		"providerAccountId": rs.providerAccountID,
		"status":            rs.status,
	}})
}

func (s *InteractiveService) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[r.PathValue("runId")]
	if rs == nil {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found"))
		return
	}
	out := make([]ProviderEvent, len(rs.events))
	copy(out, rs.events)
	writeInteractiveJSON(w, http.StatusOK, out)
}

func (s *InteractiveService) handleAdminApprovals(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for _, rec := range s.approvals {
		if rec.runID != runID {
			continue
		}
		out = append(out, map[string]any{
			"id": rec.id, "status": rec.status, "decision": rec.decision, "details": rec.details,
		})
	}
	writeInteractiveJSON(w, http.StatusOK, out)
}

func (s *InteractiveService) handleAdminQuestions(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for _, rec := range s.questions {
		if rec.runID != runID {
			continue
		}
		out = append(out, map[string]any{
			"id": rec.id, "status": rec.status, "prompt": rec.prompt,
			"options": rec.options, "multiSelect": rec.multiSelect, "choice": rec.choice,
		})
	}
	writeInteractiveJSON(w, http.StatusOK, out)
}

// ---- run creation / snapshot / fake artifacts ------------------------------

func (s *InteractiveService) createRun(in StartRunInput) RunHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID := s.nextID("run")
	sessionID := s.nextID("thread")
	rs := &interactiveRun{
		id:                runID,
		providerKey:       ProviderKeyCodex,
		providerSessionID: sessionID,
		providerAccountID: s.activeAccountID,
		yolo:              in.YoloMode,
		status:            RunStatusIdle,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	s.runs[runID] = rs
	return RunHandle{RunID: runID, ProviderSessionID: sessionID, ProviderKey: ProviderKeyCodex, Status: rs.status}
}

func (s *InteractiveService) resumeRun(runID string) (RunHandle, *apiErr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return RunHandle{}, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if rs.providerAccountID != s.activeAccountID {
		return RunHandle{}, newAPIErr(http.StatusConflict, "provider_account_changed", "active provider account changed since the run started")
	}
	return RunHandle{RunID: rs.id, ProviderSessionID: rs.providerSessionID, ProviderKey: rs.providerKey, Status: rs.status}, nil
}

type pendingApprovalView struct {
	ApprovalID string          `json:"approvalId"`
	Details    ApprovalDetails `json:"details"`
}

type pendingQuestionView struct {
	QuestionID  string           `json:"questionId"`
	Prompt      string           `json:"prompt"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

type runSnapshotView struct {
	RunID             string               `json:"runId"`
	ProviderSessionID string               `json:"providerSessionId"`
	ProviderKey       ProviderKey          `json:"providerKey"`
	Status            RunStatus            `json:"status"`
	PendingApproval   *pendingApprovalView `json:"pendingApproval,omitempty"`
	PendingQuestion   *pendingQuestionView `json:"pendingQuestion,omitempty"`
}

func (s *InteractiveService) runSnapshot(runID string) (runSnapshotView, *apiErr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return runSnapshotView{}, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	view := runSnapshotView{
		RunID:             rs.id,
		ProviderSessionID: rs.providerSessionID,
		ProviderKey:       rs.providerKey,
		Status:            rs.status,
	}
	if rs.pendingApprovalID != "" {
		if rec := s.approvals[rs.pendingApprovalID]; rec != nil {
			view.PendingApproval = &pendingApprovalView{ApprovalID: rec.id, Details: rec.details}
		}
	}
	if rs.pendingQuestionID != "" {
		if rec := s.questions[rs.pendingQuestionID]; rec != nil {
			view.PendingQuestion = &pendingQuestionView{QuestionID: rec.id, Prompt: rec.prompt, Options: rec.options, MultiSelect: rec.multiSelect}
		}
	}
	return view, nil
}

func fakeArtifacts(runID string) []Artifact {
	return []Artifact{
		{ID: runID + "-final", RunID: runID, Kind: "final_response", Name: "final-response.md", Preview: "Implemented the feature.", CreatedAt: "2026-06-12T10:00:00Z"},
		{ID: runID + "-diff", RunID: runID, Kind: "diff_snapshot", Name: "changes.diff", Preview: "3 files changed", CreatedAt: "2026-06-12T10:00:01Z"},
	}
}
