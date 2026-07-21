package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RegisterInteractiveRoutes wires the Phase 2 interactive + admin endpoints onto
// the runner's mux (04-02). Uses Go 1.22+ method+wildcard patterns.
func (s *InteractiveService) RegisterInteractiveRoutes(mux *http.ServeMux) {
	// interactive (client)
	mux.HandleFunc("GET /client/projects", s.handleListProjects)
	mux.HandleFunc("GET /client/workflows", s.handleListWorkflows)
	mux.HandleFunc("GET /client/projects/{projectId}/workflows", s.handleListWorkflows)
	mux.HandleFunc("GET /client/steps", s.handleListSteps)
	mux.HandleFunc("GET /client/workflows/{workflowId}/steps", s.handleListSteps)
	mux.HandleFunc("GET /client/chat/builtin-orchestration-options", s.handleListBuiltinOrchestrationOptions)
	mux.HandleFunc("GET /client/projects/{projectId}/workflow-runs", s.handleListProjectRunHistory)
	mux.HandleFunc("GET /client/projects/{projectId}/chat-sessions/remote", s.handleListRemoteChatSessions)
	mux.HandleFunc("GET /client/engine/tooling/status", s.handleGetGlobalEngineToolingStatus)
	mux.HandleFunc("POST /client/engine/tooling/install/libretranslate", s.handleInstallLibreTranslate)
	mux.HandleFunc("GET /client/projects/{projectId}/engine/status", s.handleGetEngineStatus)
	mux.HandleFunc("POST /client/projects/{projectId}/engine/init", s.handleInitEngine)
	mux.HandleFunc("GET /client/projects/{projectId}/engine/gate-config", s.handleGetEngineGateConfig)
	mux.HandleFunc("POST /client/projects/{projectId}/engine/gate-config", s.handleSetEngineGateConfig)
	mux.HandleFunc("GET /client/projects/{projectId}/engine/approval-allowlist", s.handleGetApprovalAllowlist)
	mux.HandleFunc("POST /client/projects/{projectId}/engine/approval-allowlist/remove", s.handleRemoveApprovalAllowRule)
	// Task-188 (CP-43 P-5): Canonical Head panel read endpoints.
	mux.HandleFunc("GET /client/projects/{projectId}/features/{featureKey}/canonical-head", s.handleGetCanonicalHead)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/steps/{stepId}/contract", s.handleGetStepContract)
	// Task-186 r-attach-spec / Task-187 r-retire: human-confirmed Head lifecycle actions.
	mux.HandleFunc("POST /client/projects/{projectId}/features/{featureKey}/canonical-head/rebaseline", s.handleRebaselineCanonicalHead)
	mux.HandleFunc("POST /client/projects/{projectId}/features/{featureKey}/canonical-head/retire", s.handleRetireCanonicalHead)
	mux.HandleFunc("POST /client/workflow-runs", s.handleStartRun)
	mux.HandleFunc("GET /client/workflow-runs/{runId}", s.handleGetRun)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/steps-runtime", s.handleGetWorkflowStepsRuntime)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/resume", s.handleResumeRun)
	mux.HandleFunc("DELETE /client/workflow-runs/{runId}", s.handleDeleteRun)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/sync-chat", s.handleSyncChatRun)
	mux.HandleFunc("POST /client/chat-sessions/restore", s.handleRestoreChatRun)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/turns", s.handleStartTurn)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/handoff-context", s.handleHandoffContext)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/chat-summary", s.handleGenerateChatSummary)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/events/stream", s.handleEventStream)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /client/approvals/{approvalId}/decision", s.handleApprovalDecision)
	mux.HandleFunc("POST /client/questions/{questionId}/answer", s.handleAnswerQuestion)
	mux.HandleFunc("GET /client/questions/{questionId}/google-drive-picker", s.handleGoogleDriveQuestionPicker)
	mux.HandleFunc("GET /client/questions/{questionId}/google-drive-picker-token", s.handleGoogleDriveQuestionPickerToken)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/artifacts", s.handleListArtifacts)
	mux.HandleFunc("GET /client/provider-skills", s.handleListSkills)
	mux.HandleFunc("GET /client/agents", s.handleListAgents)
	mux.HandleFunc("GET /client/active-account", s.handleGetActiveAccount)
	mux.HandleFunc("POST /client/active-account", s.handleSetActiveAccount)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/spawn-agent", s.handleSpawnAgent)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/agents", s.handleListAgentRuns)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/agent-graph", s.handleGetAgentGraph)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/agent-bus", s.handleGetAgentBus)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/pause", s.handlePauseAgentLoop)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/resume", s.handleResumeAgentLoop)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/feedback", s.handleInjectAgentFeedback)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/stop", s.handleStopAgentLoop)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/flow-control", s.handleSubmitFlowControl)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/extend-cap", s.handleExtendCap)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/agent-loop/continue", s.handleContinueFlow)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/gate-decision", s.handleGateDecision)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/gate-agreement", s.handleGateAgreement)

	// CP-51 Task-256: SS-17 operator resolution surface (attention / resolve / repair / audit).
	s.RegisterDispatchOperatorRoutes(mux)

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
	projects, err := s.catalog.ListProjects(r.Context())
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "catalog_unavailable", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, projects)
}

func (s *InteractiveService) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := s.catalog.ListWorkflows(r.Context())
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "catalog_unavailable", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, workflows)
}

func (s *InteractiveService) handleListSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := s.catalog.ListSteps(r.Context())
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "catalog_unavailable", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, steps)
}

func (s *InteractiveService) handleListSkills(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	cwd := r.URL.Query().Get("cwd")
	writeInteractiveJSON(w, http.StatusOK, s.skillsCatalog.listSkills(provider, cwd))
}

func (s *InteractiveService) handleGoogleDriveQuestionPicker(w http.ResponseWriter, r *http.Request) {
	questionID := r.PathValue("questionId")
	if strings.TrimSpace(questionID) == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_question", "questionId is required"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(RenderGoogleDriveQuestionPickerHTML(questionID))); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "picker_render_failed", err.Error()))
	}
}

func (s *InteractiveService) handleGoogleDriveQuestionPickerToken(w http.ResponseWriter, r *http.Request) {
	questionID := r.PathValue("questionId")
	token, err := s.GetGoogleDriveQuestionPickerToken(questionID)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "google_drive_picker_unavailable", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, token)
}

// handleListBuiltinOrchestrationOptions serves the Chat Mode "Built-in
// orchestration" picker options for a given subMode (CP-42/Task-177), driven
// entirely by embedded pack metadata rather than a hardcoded UI list. An
// unset or unrecognized subMode returns an empty list, which the desktop
// renders as "no picker" rather than an error.
func (s *InteractiveService) handleListBuiltinOrchestrationOptions(w http.ResponseWriter, r *http.Request) {
	subMode := r.URL.Query().Get("subMode")
	opts, err := BuiltinOrchestrationOptions(subMode)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "pack_unavailable", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, opts)
}

// handleListAgents serves the loadable sub-agent catalog (CP-19 / Task-081):
// project-local .claude/agents + .codex/agents, provider homes, and built-ins,
// merged by name precedence. `cwd` is the active project workspace.
func (s *InteractiveService) handleListAgents(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	writeInteractiveJSON(w, http.StatusOK, s.agentCatalog.listAgents(cwd))
}

func (s *InteractiveService) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	// Prefer real finalizer artifacts once a turn has finalized (04-04); fall back
	// to the fake catalog before the first finalize.
	if arts, ok := s.finalizer.artifactsForRun(runID); ok {
		writeInteractiveJSON(w, http.StatusOK, arts)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, fakeArtifacts(runID))
}

func (s *InteractiveService) handleListProjectRunHistory(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.projectRunHistory(r.PathValue("projectId")))
}

func (s *InteractiveService) handleListRemoteChatSessions(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.listRemoteChatSessions(r.Context(), r.PathValue("projectId"))
	if err != nil {
		writeInteractiveError(w, err)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, summaries)
}

// ---- run lifecycle handlers ------------------------------------------------

func (s *InteractiveService) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var in StartRunInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	handle, e := s.createRun(in)
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, handle)
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
	runID := r.PathValue("runId")
	log.Printf("[chat-history-open] request run_id=%q remote_addr=%q", runID, r.RemoteAddr)
	handle, e := s.resumeRun(runID)
	if e != nil {
		log.Printf("[chat-history-open] response failed run_id=%q status=%d code=%q message=%q", runID, e.status, e.code, e.msg)
		writeInteractiveError(w, e)
		return
	}
	log.Printf("[chat-history-open] response ok run_id=%q provider=%q provider_session_id=%q status=%q step_id=%q", handle.RunID, handle.ProviderKey, handle.ProviderSessionID, handle.Status, handle.StepID)
	writeInteractiveJSON(w, http.StatusOK, handle)
}

func (s *InteractiveService) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if e := s.deleteChatSession(r.PathValue("runId")); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *InteractiveService) handleSyncChatRun(w http.ResponseWriter, r *http.Request) {
	var body ChatSessionSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	result, apiErr := s.syncChatRunToDrive(r.Context(), r.PathValue("runId"), body)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}

func (s *InteractiveService) handleRestoreChatRun(w http.ResponseWriter, r *http.Request) {
	var body ChatSessionRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	result, apiErr := s.restoreChatRunFromDrive(r.Context(), body)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}

type turnBody struct {
	StepID         string           `json:"stepId"`
	Prompt         string           `json:"prompt"`
	ChangeType     string           `json:"changeType"`
	SourceDocID    string           `json:"sourceDocId"`
	SelectedSkills []SkillSelection `json:"selectedSkills"`
	// ReasoningEffort/Model/YoloMode are per-turn chat overrides (T-4 / BUG-063). Model and
	// YoloMode are pointers so an omitted field falls back to the run-level default rather
	// than being read as "" / false; the desktop sends them on every chat turn.
	ReasoningEffort string  `json:"reasoningEffort"`
	Model           *string `json:"model"`
	YoloMode        *bool   `json:"yoloMode"`
	// Attachments carries chat-turn image attachments (Task-052), inline base64.
	Attachments []PromptAttachment `json:"attachments,omitempty"`
	Scenario    string             `json:"scenario"` // P2 fake-adapter hint only
	// SubMode/FlowRef select an optional built-in Chat Mode orchestration
	// template (CP-42/Task-177). Both are optional and omitting them means
	// normal chat with no orchestration. handleStartTurn validates FlowRef
	// against BuiltinOrchestrationOptions(SubMode) before starting the turn;
	// startTurn (interactive_service.go) then resolves/executes it via
	// startResolvedFlow on the run's first turn, exactly like a Flow-Mode
	// workflow-picker launch.
	SubMode string `json:"subMode,omitempty"`
	FlowRef string `json:"flowRef,omitempty"`
}

func (s *InteractiveService) handleStartTurn(w http.ResponseWriter, r *http.Request) {
	var body turnBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if err := validateChatOrchestrationSelection(body.SubMode, body.FlowRef); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_flow_ref", err.Error()))
		return
	}
	// BUG-174: a Flow-Mode workflow-picker launch sends a workflowID but no
	// flowRef, so the flow executor never engaged and the hub did all the work
	// inline. If the run's selected workflow resolves to a flow-engine flow with
	// a spawnable entry node, adopt its canonical flowRef so it runs through the
	// same startResolvedFlow path an explicit flowRef uses and its step timeline
	// is driven node-by-node. The explicit chat/flowRef path and plain
	// workflows are untouched (resolve returns false for them).
	//
	// startTurn itself marks the run flow-engine-driven once it sees a non-empty
	// FlowRef (both this branch, via body.FlowRef below, and the explicit chat
	// flowRef path converge there) — this handler no longer needs its own
	// markFlowEngineDriven call.
	if strings.TrimSpace(body.FlowRef) == "" {
		if flowRef, ok := s.resolveWorkflowFlowRef(r.Context(), r.PathValue("runId")); ok {
			body.FlowRef = flowRef
		} else if invalidErr := s.takePendingFlowRefInvalidErr(r.PathValue("runId")); invalidErr != nil {
			// BUG-270: the selected workflow resolved to a real flow
			// definition that failed validation (e.g. BUG-269's unknown
			// context-source id) — a genuine data problem, not "this isn't a
			// flow." Surface it instead of silently falling through to a
			// normal chat turn with no explanation.
			writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "invalid_flow_definition", invalidErr.Error()))
			return
		}
	} else if !s.explicitFlowRefResolves(r.Context(), body.FlowRef) {
		// BUG-261: an explicit chat flowRef (Bug sub-mode picker) that passed
		// validateChatOrchestrationSelection's option check can still fail to
		// resolve if its stored definition is corrupted/invalid. Clear it here,
		// synchronously, so startTurn never suppresses the hub's own turn for a
		// flow that can't actually start — the run falls through to a normal
		// chat turn instead of getting stuck at "completed" with no reply.
		log.Printf("[chat-flow-ref] flowRef %q for run %q failed to resolve; falling back to a normal chat turn", body.FlowRef, r.PathValue("runId"))
		body.FlowRef = ""
	}
	turnID, e := s.startTurn(
		r.PathValue("runId"),
		TurnInput{StepID: body.StepID, Prompt: body.Prompt, ChangeType: body.ChangeType, SourceDocID: body.SourceDocID, SelectedSkills: body.SelectedSkills, ReasoningEffort: body.ReasoningEffort, Model: body.Model, YoloMode: body.YoloMode, Attachments: body.Attachments, SubMode: body.SubMode, FlowRef: body.FlowRef},
		body.Scenario,
		r.Header.Get("Idempotency-Key"),
	)
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"turnId": turnID})
}

func (s *InteractiveService) handleGetActiveAccount(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, map[string]string{"activeAccountId": s.ActiveAccount()})
}

func (s *InteractiveService) handleSetActiveAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID string `json:"accountId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if body.AccountID == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "accountId is required"))
		return
	}
	interrupted := s.SetActiveAccount(body.AccountID)
	writeInteractiveJSON(w, http.StatusOK, map[string]any{
		"activeAccountId":   body.AccountID,
		"interruptedTurns":  interrupted,
		"recoverableResend": true,
	})
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
		// Remember persists a "don't ask again" rule for this shell command
		// (BUG-246). Ignored for non-exec approvals and compound commands.
		Remember bool `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if e := s.submitApprovalDecision(r.PathValue("approvalId"), body.Decision, body.Remember); e != nil {
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
		"workingDirectory":  rs.workspaceCwd,
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
			"id": rec.id, "status": rec.status, "decision": rec.decision,
			"policy": rec.policy, "details": rec.details,
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

func (s *InteractiveService) createRun(in StartRunInput) (RunHandle, *apiErr) {
	stepID := in.StepID
	runKind := "workflow"
	if in.ChatMode == "normal_chat" {
		runKind = "chat"
	}

	seedSteps := []RuntimeWorkflowStep{{
		ID:               stepID,
		StepType:         stepID,
		Status:           StepStatusPending,
		RequiresApproval: false,
	}}

	resolvedModel := in.Model
	resolvedYolo := in.YoloMode
	if in.WorkflowID != "" && (stepID == "" || stepID == in.WorkflowID) {
		stepCatalog, ok := s.catalog.(WorkflowStepCatalogStore)
		if !ok {
			return RunHandle{}, newAPIErr(http.StatusBadGateway, "catalog_unavailable", "workflow step catalog is unavailable")
		}
		steps, err := stepCatalog.ListWorkflowSteps(context.Background(), in.WorkflowID)
		if err != nil {
			return RunHandle{}, newAPIErr(http.StatusBadGateway, "catalog_unavailable", err.Error())
		}
		if len(steps) == 0 {
			return RunHandle{}, newAPIErr(http.StatusUnprocessableEntity, "workflow_has_no_steps", "workflow has no enabled steps")
		}
		stepID = steps[0].ID
		seedSteps = make([]RuntimeWorkflowStep, len(steps))
		for i, step := range steps {
			stepType := step.ID
			if step.Name != "" {
				stepType = step.Name
			}
			seedSteps[i] = RuntimeWorkflowStep{
				ID:               step.ID,
				StepType:         stepType,
				Status:           StepStatusPending,
				RequiresApproval: false,
			}
		}
		// BUG-165: the desktop client sends no explicit model for a workflow/step
		// run (Req 2 — Flow Mode has no chat-controller model picker to source one
		// from), so the run must resolve its own model here rather than run with
		// an empty one for its whole lifetime. Step > Flow > Project > default, per
		// SS-05 §2.2/§3 and SD-06 §6: the entry step's own step_definitions.model
		// wins if set, else the workflow's model_override, else the project's
		// default_model, else the hard floor "gpt-5.4".
		if resolvedModel == "" {
			resolvedModel = strings.TrimSpace(steps[0].Model)
		}
		// BUG-235-follow-up: the entry step's own step_definitions row is a
		// flow-pack mirror row keyed by a per-flow/per-node step_type (e.g.
		// "review-loop__coder") that mirror-sync never writes a model onto, so
		// the direct steps[0].Model read above is always empty for a flow-pack
		// workflow's entry node. Fall back to the same node_id/role lookup
		// resolveFlowNodeModel uses for child spawns, so the run's own Step
		// tier actually sees the model configured on the purpose-named role
		// row (e.g. "Flow: Coder" / flow-agent-delegate-coder) instead of
		// skipping straight past it to the Flow/Project tiers.
		if resolvedModel == "" {
			resolvedModel = s.resolveConfiguredModelForAgent(context.Background(), steps[0].NodeID, agentNameFromRef(steps[0].AgentRef), "")
		}
		// BUG-183: Flow Mode has two distinct default sources. A normal workflow/flow
		// execution inherits YOLO from the workflow definition itself, while a direct
		// single-step execution inherits from that selected step. Do not let the entry
		// step's yolo_mode override a workflow launch.
		if catalog, ok := s.catalog.(CatalogStore); ok {
			if workflows, err := catalog.ListWorkflows(context.Background()); err == nil {
				for _, wf := range workflows {
					if wf.ID != in.WorkflowID {
						continue
					}
					resolvedYolo = resolvedYolo || wf.YoloMode
					if resolvedModel == "" {
						resolvedModel = strings.TrimSpace(wf.Model)
					}
					break
				}
			}
		}
		if resolvedModel == "" {
			if catalog, ok := s.catalog.(CatalogStore); ok {
				if projects, err := catalog.ListProjects(context.Background()); err == nil {
					for _, proj := range projects {
						if proj.ID == in.ProjectID {
							resolvedModel = strings.TrimSpace(proj.Model)
							break
						}
					}
				}
			}
		}
		if resolvedModel == "" {
			return RunHandle{}, newAPIErr(http.StatusBadRequest, "no_model_configured", "no model configured for this workflow")
		}
	} else if runKind != "chat" && stepID != "" {
		// BUG-229: a direct single-step launch resolves its model from the
		// selected step ONLY — no Flow tier applies (there is no workflow
		// context) and, unlike the normal-flow branch above, no Project
		// fallback either. Falling back to the project default here let a
		// step with no model configured silently run on an unrelated
		// project-wide model instead of surfacing as non-runnable.
		if catalog, ok := s.catalog.(CatalogStore); ok {
			if steps, err := catalog.ListSteps(context.Background()); err == nil {
				for _, step := range steps {
					if step.ID != stepID {
						continue
					}
					if step.Name != "" {
						seedSteps[0].StepType = step.Name
					}
					if resolvedModel == "" {
						resolvedModel = strings.TrimSpace(step.Model)
					}
					resolvedYolo = resolvedYolo || step.YoloMode
					break
				}
			}
		}
		if resolvedModel == "" {
			return RunHandle{}, newAPIErr(http.StatusBadRequest, "no_model_configured", "no model configured for this step")
		}
	} else if stepID == "" && runKind != "chat" {
		// Normal chat: synthetic step is minted after the runID is known (see below).
		// Workflow/step mode: stepId is required.
		return RunHandle{}, newAPIErr(http.StatusBadRequest, "invalid_request", "stepId is required")
	}

	// Resolve + enforce the provider runner-side (04-07): an empty key takes the
	// default available provider; an explicitly requested disabled/placeholder
	// provider is rejected with the typed UnsupportedProviderRuntimeError envelope.
	providerKey := in.ProviderKey
	// BUG-171: a resolved model with a known provider prefix (e.g. "claude-haiku",
	// "gpt-5.4", "gemini-3-flash") is ONLY runnable on its matching provider, so the
	// model is authoritative — it wins over a conflicting explicit/inherited provider.
	// The trigger: a workflow/flow launch sends providerKey=selectedProvider (e.g. codex)
	// but no model, and the Step>Flow>Project>default resolution above lands on a Claude
	// model (BUG-162 makes coder/reviewer default to claude-haiku). Without this, the run
	// was stamped codex + claude-haiku and the child turn died with "the 'claude-haiku'
	// model is not supported when using Codex". Previously the provider was only derived
	// from the model when providerKey was empty, which the desktop's flow launch never is.
	// This is safe for normal_chat too: the UI couples provider+model, so they never
	// conflict there (a gpt-* model already implies codex, a claude-* model implies claude),
	// making this a no-op for chat while fixing the workflow/flow mismatch.
	if pk, ok := providerKeyFromModel(resolvedModel); ok {
		providerKey = pk
	} else if providerKey == "" {
		// No explicit provider and no model to infer one from → default available provider.
		def, ok := s.registry.DefaultProviderKey()
		if !ok {
			return RunHandle{}, newAPIErr(http.StatusServiceUnavailable, "provider_unavailable", "no provider runtime is available")
		}
		providerKey = def
	}
	if _, err := s.registry.Selectable(providerKey); err != nil {
		return RunHandle{}, newAPIErr(http.StatusUnprocessableEntity, "provider_unavailable", err.Error())
	}
	if providerKey == ProviderKeyGemini {
		cwd := strings.TrimSpace(in.Cwd)
		if cwd == "" {
			return RunHandle{}, newAPIErr(http.StatusBadRequest, "workspace_required", "Gemini requires a bound project workspace path; set the project's local path before starting chat")
		}
		if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
			return RunHandle{}, newAPIErr(http.StatusBadRequest, "workspace_unavailable", "Gemini project workspace path must point to an existing directory")
		}
	}
	// Stamp the run with the account that is active for THIS provider, not the
	// single global activeAccountID (Task-067 issue 1). Resolved before the lock
	// since it may read the provider-accounts store.
	stampAccount := s.activeAccountForProvider(providerKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	runID := s.nextID("run")
	sessionID := s.nextID("thread")
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// For normal_chat, mint a synthetic step id now that we have the runID.
	if runKind == "chat" && stepID == "" {
		stepID = "chat-" + runID
		seedSteps = []RuntimeWorkflowStep{{
			ID:               stepID,
			StepType:         "chat",
			Status:           StepStatusPending,
			RequiresApproval: false,
		}}
	}

	// BUG-299 residual (run-35329): Flow/Workflow always YOLO=true at create time
	// regardless of a stale catalog row (pre-CA-378 default false) or a missing
	// start-request field. Desktop workflow launches omit yoloMode entirely.
	// Normal chat keeps resolvedYolo from the request/toggle.
	resolvedYolo = resolveEffectiveYolo(resolvedYolo, runKind, in.WorkflowID, false)
	rs := &interactiveRun{
		id:                runID,
		projectID:         in.ProjectID,
		workflowID:        in.WorkflowID,
		providerKey:       providerKey,
		providerSessionID: sessionID,
		providerAccountID: stampAccount,
		workspaceCwd:      in.Cwd,
		modelName:         resolvedModel,
		yolo:              resolvedYolo,
		reasoningEffort:   in.ReasoningEffort,
		runKind:           runKind,
		status:            RunStatusIdle,
		createdAt:         now,
		updatedAt:         now,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	s.runs[runID] = rs
	if seeder, ok := s.workflowStore.(workflowRunSeeder); ok {
		seeder.seed(runID, seedSteps)
	}
	if err := s.persistProviderSession(ProviderSessionState{
		RunID:             runID,
		ProjectID:         in.ProjectID,
		WorkflowID:        in.WorkflowID,
		ProviderSessionID: sessionID,
		ProviderKey:       providerKey,
		ProviderAccountID: stampAccount,
		WorkingDirectory:  in.Cwd,
		Status:            rs.status,
		StartedAt:         now,
		UpdatedAt:         now,
		RunKind:           runKind,
		Yolo:              resolvedYolo,
	}); err != nil {
		delete(s.runs, runID)
		return RunHandle{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	return RunHandle{RunID: runID, ProviderSessionID: sessionID, ProviderKey: providerKey, Status: rs.status, StepID: stepID}, nil
}

// skipsResumeSessionValidation reports whether resumeRun should skip the
// strict ensureResumeReady/LocateSessionFile check for rs.
//
// Two independent reasons a check would be pointless:
//   - the run is still resident in memory with a turn genuinely in flight (or
//     is a read-only-viewable Gemini run) — refreshResumeHandleLocked only
//     resolves realProviderSessionID once a turn finishes, so validating mid-turn
//     would spuriously fail on the synthetic "thread-<n>" placeholder.
//   - BUG-250: rs's session id is STILL that placeholder, full stop — a
//     flow-engine-driven hub's own first provider turn is deliberately
//     suppressed (CP-42; it only spawns the flow's entry node and is
//     reinvoked later), so provider_session_id never advances past the
//     placeholder assigned at spawn. That placeholder was never a real
//     provider session, so LocateSessionFile can never resolve it — whether rs
//     is still resident in memory or was just rebuilt from sessions.ndjson
//     after a restart, which is exactly when the in-memory reason above stops
//     applying (inMemory is permanently false for a rebuilt run, regardless of
//     status). Codex is excluded from this second reason: its own
//     ensureProviderResumeHandle already actively re-discovers a real rollout
//     session id from disk even when the stored id is still a placeholder, so
//     that stronger, self-healing path should still run for Codex.
func (s *InteractiveService) skipsResumeSessionValidation(rs *interactiveRun, inMemory bool) bool {
	isActiveInMemory := inMemory && rs.status != RunStatusCompleted && rs.status != RunStatusFailed && rs.status != RunStatusCancelled
	isReadOnlyGeminiInMemory := inMemory && rs.providerKey == ProviderKeyGemini && len(rs.events) > 0
	hasNoRealSession := strings.HasPrefix(s.resumeSessionID(rs), "thread-") &&
		(rs.providerKey != ProviderKeyCodex || s.shouldTreatCodexFlowHubSessionAsSynthetic(rs))
	return isActiveInMemory || isReadOnlyGeminiInMemory || hasNoRealSession
}

func (s *InteractiveService) resumeRun(runID string) (RunHandle, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	log.Printf("[chat-history-open] resume start run_id=%q in_memory=%t", runID, rs != nil)
	inMemory := rs != nil
	if rs == nil {
		rebuilt, err := s.loadPersistedRun(runID)
		if err != nil {
			log.Printf("[chat-history-open] resume reconstruction failed run_id=%q code=%q message=%q", runID, err.code, err.msg)
			return RunHandle{}, err
		}
		rs = rebuilt
	}
	if !s.skipsResumeSessionValidation(rs, inMemory) {
		if err := s.ensureResumeReady(rs); err != nil {
			readOnlyChat := rs.runKind == "chat" && (err.code == "account_not_signed_in" || err.code == "account_unavailable")
			if !readOnlyChat {
				log.Printf("[chat-history-open] resume readiness failed run_id=%q provider=%q code=%q message=%q", runID, rs.providerKey, err.code, err.msg)
				return RunHandle{}, err
			}
			log.Printf("[chat-history-open] resume continuing read-only run_id=%q provider=%q code=%q message=%q", runID, rs.providerKey, err.code, err.msg)
		}
	}
	s.seedTranscriptFromDisk(rs)
	handle := RunHandle{RunID: rs.id, ProviderSessionID: s.resumeSessionID(rs), ProviderKey: rs.providerKey, Status: rs.status}
	// Surface the synthetic chat step so the desktop can continue a resumed normal_chat
	// run; its turns need a stepId and the chat step id is deterministic (T-7). Workflow
	// runs resume as before (the desktop drives the step via the navigator selection).
	if rs.runKind == "chat" {
		handle.StepID = "chat-" + rs.id
	}
	s.mu.Lock()
	eventCount := len(rs.events)
	if eventCount > 0 {
		// Seq of the last persisted event — the desktop replays from 0 and stops here so
		// a multi-turn transcript is replayed in full instead of being truncated at the
		// first turn_completed. (BUG-112)
		handle.LastEventSeq = rs.events[eventCount-1].Seq
	}
	s.mu.Unlock()
	log.Printf("[chat-history-open] resume complete run_id=%q provider=%q provider_session_id=%q status=%q events=%d last_seq=%d", rs.id, rs.providerKey, handle.ProviderSessionID, rs.status, eventCount, handle.LastEventSeq)
	return handle, nil
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

type runHistoryItem struct {
	RunID       string      `json:"runId"`
	ProjectID   string      `json:"projectId"`
	WorkflowID  string      `json:"workflowId,omitempty"`
	ProviderKey ProviderKey `json:"providerKey"`
	Status      RunStatus   `json:"status"`
	StartedAt   string      `json:"startedAt"`
	UpdatedAt   string      `json:"updatedAt"`
	LastPrompt  string      `json:"lastPrompt,omitempty"`
	LastMessage string      `json:"lastMessage,omitempty"`
	// RunKind distinguishes normal chat runs from workflow runs so chat runs
	// are excluded from workflow catalogs and labeled correctly in history (T-7).
	RunKind         string `json:"runKind,omitempty"`
	SourceMachineID string `json:"sourceMachineId,omitempty"`
	SourceRunID     string `json:"sourceRunId,omitempty"`
	SyncStatus      string `json:"syncStatus,omitempty"`
	ParentRunID     string `json:"parentRunId,omitempty"`
	AgentName       string `json:"agentName,omitempty"`
	Role            string `json:"role,omitempty"`
	AgentStatus     string `json:"agentStatus,omitempty"`
	// SubMode/FlowRef expose the Chat-Mode orchestration picker selection a
	// run was started with (BUG-263), so reopening it from history can
	// restore the Chat Intent panel's Bug tab / Built-in orchestration
	// selection instead of silently falling back to "Normal".
	SubMode string `json:"subMode,omitempty"`
	FlowRef string `json:"flowRef,omitempty"`
}

func (s *InteractiveService) projectRunHistory(projectID string) []runHistoryItem {
	s.mu.Lock()
	seen := map[string]bool{}
	out := make([]runHistoryItem, 0, len(s.runs))
	for _, rs := range s.runs {
		if rs.projectID != projectID || isLiveAgentHistoryRun(rs.parentRunID, rs.lastPrompt) {
			continue
		}
		out = append(out, runHistoryItem{
			RunID:       rs.id,
			ProjectID:   rs.projectID,
			WorkflowID:  rs.workflowID,
			ProviderKey: rs.providerKey,
			// Prefer flow loop status over raw rs.status: after markFlowRunComplete
			// the loop is "done" while the hub provider run can still sit at
			// "running" until the last SSE settles — history then shows a spinner
			// for every completed flow that is not the active chat (image-8).
			Status:      s.historyStatusForLiveRun(rs),
			StartedAt:   rs.createdAt,
			UpdatedAt:   rs.updatedAt,
			LastPrompt:  rs.lastPrompt,
			LastMessage: rs.lastMessage,
			RunKind:     rs.runKind,
			ParentRunID: rs.parentRunID,
			AgentName:   rs.agentName,
			Role:        rs.role,
			AgentStatus: rs.agentStatus,
			SubMode:     rs.chatSubMode,
			FlowRef:     rs.chatFlowRef,
		})
		seen[rs.id] = true
	}
	s.mu.Unlock()

	// BUG-060 F-1: augment with persisted sessions not in the current in-memory
	// map (e.g. after a runner/app-server restart). The store holds the truth;
	// s.runs is a write-through cache that is empty on a new service instance.
	if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
		sessions, err := reader.ListProviderSessionsByProject(context.Background(), projectID)
		if err == nil {
			for _, sess := range sessions {
				if seen[sess.RunID] || isAgentHistoryRun(sess.ParentRunID, sess.AgentName, sess.Role, sess.AgentStatus, sess.LastPrompt) {
					continue
				}
				out = append(out, runHistoryItem{
					RunID:       sess.RunID,
					ProjectID:   sess.ProjectID,
					WorkflowID:  sess.WorkflowID,
					ProviderKey: sess.ProviderKey,
					// Persisted-only runs are not in the in-memory map, so an
					// in-flight status is stale after a restart (T-067 4.4).
					// BUG-StaleCancel: use normalizeResumedFlowStatus, not
					// normalizeResumedStatus, for persisted sessions. A flow
					// whose LoopState.Status is "done" must show as completed
					// even when the raw persisted status is still "running"
					// (AnswerQuestion's persist can overwrite the early
					// flowStartOnly "completed" persist before the async
					// applyFlowControl goroutine has a chance to write the
					// final "completed" record).
					Status:          normalizeResumedFlowStatus(sess),
					StartedAt:       sess.StartedAt,
					UpdatedAt:       sess.UpdatedAt,
					LastPrompt:      sess.LastPrompt,
					LastMessage:     sess.LastMessage,
					RunKind:         sess.RunKind,
					SourceMachineID: sess.SourceMachineID,
					SourceRunID:     sess.SourceRunID,
					SyncStatus:      sess.SyncStatus,
					ParentRunID:     sess.ParentRunID,
					AgentName:       sess.AgentName,
					Role:            sess.Role,
					AgentStatus:     sess.AgentStatus,
					SubMode:         sess.ChatSubMode,
					FlowRef:         sess.ChatFlowRef,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

// historyStatusForLiveRun maps an in-memory run to the status the history list
// should show. Flow hubs keep rs.status=running through the final synthesis
// stream while LoopState is already "done" — list inactive chats must not spin.
func (s *InteractiveService) historyStatusForLiveRun(rs *interactiveRun) RunStatus {
	if rs == nil {
		return RunStatusIdle
	}
	if rs.flowEngineDriven && strings.TrimSpace(rs.parentRunID) == "" {
		switch s.agentOrchestrator.loopStateFor(rs.id).Status {
		case "done":
			return RunStatusCompleted
		case "stopped":
			// BUG-308 residual (run-33289 UI): Stop ends the FLOW loop, not the
			// CHAT. After a plain-chat follow-up rs.status is completed (or
			// running mid-turn). Prefer that over blanket Cancelled so history
			// does not stay "Cancelled" forever after the user keeps chatting.
			switch rs.status {
			case RunStatusCompleted, RunStatusFailed, RunStatusRunning,
				RunStatusWaitingApproval, RunStatusWaitingQuestion:
				return rs.status
			default:
				return RunStatusCancelled
			}
		case "blocked":
			if rs.status == RunStatusFailed || rs.status == RunStatusCancelled {
				return rs.status
			}
			return RunStatus("blocked")
		}
	}
	return rs.status
}

func isAgentHistoryRun(parentRunID, agentName, role, agentStatus, lastPrompt string) bool {
	if strings.TrimSpace(parentRunID) != "" {
		return true
	}
	return hasBuiltInAgentPromptPrefix(lastPrompt)
}

func isLiveAgentHistoryRun(parentRunID, lastPrompt string) bool {
	return isAgentHistoryRun(parentRunID, "", "", "", lastPrompt)
}

func hasBuiltInAgentPromptPrefix(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	for _, marker := range []string{
		"you are the coder sub-agent.",
		"you are the reviewer sub-agent.",
		"you are the tester sub-agent.",
	} {
		if strings.HasPrefix(normalized, marker) {
			return true
		}
	}
	return false
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

// workflowStepRuntimeView is the client-facing DTO for BUG-153: it projects Go's
// authoritative RuntimeWorkflowStep list (workflowStore.LoadRunSteps) into the
// shape the desktop Flow-mode sidebar renders, without deriving anything from
// AI prose or child agent messages (F-4). RejectionNote doubles as the
// display-only retry reason (F-6) rather than a separate detector.
type workflowStepRuntimeView struct {
	StepID           string                    `json:"stepId"`
	StepType         string                    `json:"stepType"`
	Status           RuntimeWorkflowStepStatus `json:"status"`
	RetryCount       int                       `json:"retryCount"`
	RejectionNote    string                    `json:"rejectionNote,omitempty"`
	StartedAt        string                    `json:"startedAt,omitempty"`
	FinishedAt       string                    `json:"finishedAt,omitempty"`
	RequiresApproval bool                      `json:"requiresApproval"`
	BehaviorID       string                    `json:"behaviorId,omitempty"`
	// NodeID/AgentRef/Provider/Model/YoloMode round-trip RuntimeWorkflowStep's
	// per-node identity and config (BUG-155) so the desktop sidebar can show
	// the actual step name instead of the shared generic step_type label, plus
	// which provider/model/agent/yolo posture that node runs under.
	NodeID   string `json:"nodeId,omitempty"`
	AgentRef string `json:"agentRef,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	YoloMode bool   `json:"yoloMode,omitempty"`
}

type workflowStepsRuntimeSnapshot struct {
	RunID string                    `json:"runId"`
	Steps []workflowStepRuntimeView `json:"steps"`
	// Provider/Model/YoloMode are the RUN's own posture (BUG-158) — a built-in
	// flow node has no per-node model/provider override of its own (its agent
	// definition inherits the parent run's), and yolo is a run-wide toggle, not
	// a per-step-type default — so these are surfaced once here rather than
	// repeated per step.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	YoloMode bool   `json:"yoloMode,omitempty"`
}

// workflowStepsRuntime loads the ordered runtime step list for runID (F-2). It
// 404s for an unknown run the same way runSnapshot does, so normal chat runs
// and stale history entries degrade the same way the run snapshot endpoint
// already does.
func (s *InteractiveService) workflowStepsRuntime(ctx context.Context, runID string) (workflowStepsRuntimeSnapshot, *apiErr) {
	s.mu.Lock()
	rs, exists := s.runs[runID]
	var runProvider, runModel string
	var runYolo bool
	if exists {
		runProvider = string(rs.providerKey)
		runModel = rs.modelName
		runYolo = rs.yolo
	}
	s.mu.Unlock()
	if !exists {
		return workflowStepsRuntimeSnapshot{}, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	steps, err := s.workflowStore.LoadRunSteps(ctx, runID)
	if err != nil {
		return workflowStepsRuntimeSnapshot{}, newAPIErr(http.StatusInternalServerError, "load_steps_failed", err.Error())
	}
	out := make([]workflowStepRuntimeView, len(steps))
	for i, st := range steps {
		out[i] = workflowStepRuntimeView{
			StepID:           st.ID,
			StepType:         st.StepType,
			Status:           st.Status,
			RetryCount:       st.RetryCount,
			RejectionNote:    st.RejectionNote,
			StartedAt:        st.StartedAt,
			FinishedAt:       st.FinishedAt,
			RequiresApproval: st.RequiresApproval,
			BehaviorID:       st.BehaviorID,
			NodeID:           st.NodeID,
			AgentRef:         st.AgentRef,
			Provider:         st.Provider,
			Model:            st.Model,
			YoloMode:         st.YoloMode,
		}
	}
	return workflowStepsRuntimeSnapshot{RunID: runID, Steps: out, Provider: runProvider, Model: runModel, YoloMode: runYolo}, nil
}

func (s *InteractiveService) handleGetWorkflowStepsRuntime(w http.ResponseWriter, r *http.Request) {
	view, e := s.workflowStepsRuntime(r.Context(), r.PathValue("runId"))
	if e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, view)
}

// handleSpawnAgent allows a desktop client to programmatically spawn a child agent run for
// a given parent run. Equivalent to the spawn_agent provider tool but HTTP-initiated.
func (s *InteractiveService) handleSpawnAgent(w http.ResponseWriter, r *http.Request) {
	var in SpawnAgentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	// This is a desktop UI spawn: tell spawnChildRun to surface the child to the parent's
	// provider conversation (the AI tool path constructs SpawnAgentInput directly). (BUG-122)
	in.UIInitiated = true
	result, err := s.spawnChildRun(r.Context(), r.PathValue("runId"), in)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "spawn_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}

// handleListAgentRuns returns the agent run summaries that are children of the given run.
func (s *InteractiveService) handleListAgentRuns(w http.ResponseWriter, r *http.Request) {
	parentRunID := r.PathValue("runId")
	summaries := s.listAgentRunSummaries(parentRunID)
	if cohortDiagEnabled() {
		runs := make([]string, len(summaries))
		for i, sum := range summaries {
			runs[i] = fmt.Sprintf("%s(%s):%s", sum.RunID, sum.AgentName, sum.Status)
		}
		cohortDiagLog("handleListAgentRuns HTTP response parent=%q runs=%v", parentRunID, runs)
	}
	writeInteractiveJSON(w, http.StatusOK, summaries)
}

func (s *InteractiveService) handleGetAgentGraph(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.agentGraphSnapshot(r.PathValue("runId")))
}

func (s *InteractiveService) handleGetAgentBus(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.agentBusHistory(r.PathValue("runId")))
}

func (s *InteractiveService) handlePauseAgentLoop(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.pauseAgentLoop(r.PathValue("runId"), "paused by user"))
}

func (s *InteractiveService) handleResumeAgentLoop(w http.ResponseWriter, r *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, s.resumeAgentLoop(r.PathValue("runId")))
}

func (s *InteractiveService) handleInjectAgentFeedback(w http.ResponseWriter, r *http.Request) {
	var body struct{ Message, ToRunID string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, s.injectAgentFeedback(r.PathValue("runId"), body.ToRunID, body.Message))
}

func (s *InteractiveService) handleStopAgentLoop(w http.ResponseWriter, r *http.Request) {
	snap, err := s.stopAgentLoop(r.PathValue("runId"))
	if err != nil {
		// V10R4 P0 fail-closed on durable fence/persist, but still return the
		// in-memory graph snapshot so the desktop can flip loop status to
		// stopped / children cancelled (CP-51 A1: Stop on hub looked like a
		// no-op when only the error body was returned and the UI kept "running").
		if snap.ParentRunID != "" || len(snap.Runs) > 0 || snap.LoopState.Status != "" {
			writeInteractiveJSON(w, err.status, map[string]any{
				"error": map[string]any{
					"code":    err.code,
					"message": err.msg,
				},
				"snapshot": snap,
			})
			return
		}
		writeInteractiveError(w, err)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, snap)
}

// handleGateDecision handles POST /client/workflow-runs/{runId}/gate-decision.
// Body: {"option": "keep-test-fix-code"|"suggest-requirement-change"|"custom", "customText": "..."}
// Clears the pending gate block and fires the appropriate reprompt turn. (Task-155)
func (s *InteractiveService) handleGateDecision(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	var body struct {
		Option     string `json:"option"`
		CustomText string `json:"customText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(400, "invalid_body", "invalid JSON body"))
		return
	}
	if e := s.SubmitGateDecision(runID, body.Option, body.CustomText); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleGateAgreement handles POST /client/workflow-runs/{runId}/gate-agreement.
// Body: {"testNames": ["TestFoo", "TestBar"]}
// Records human agreement to the AI-proposed requirement change and writes overrides. (Task-155)
func (s *InteractiveService) handleGateAgreement(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	var body struct {
		TestNames []string `json:"testNames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(400, "invalid_body", "invalid JSON body"))
		return
	}
	if e := s.RecordGateAgreement(runID, body.TestNames); e != nil {
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleSubmitFlowControl handles POST /client/workflow-runs/{runId}/flow-control.
// Accepts either ReviewOutcomeInput {"outcome","issues",...} (the declared face used by
// the board and by submit_review_outcome tool calls) or the raw FlowControlInput
// {"status","summary","payload"}. Always returns an AgentGraphSnapshot so the caller
// can update its board state without a separate refresh round-trip.
func (s *InteractiveService) handleSubmitFlowControl(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	var in FlowControlInput
	// BUG-NOTE-CP42 #32: ReviewOutcomeInput's canonical wire field is "status"
	// (json:"status" on the Go struct; "outcome" is only the board/legacy
	// alias parseReviewOutcomeInput also accepts). Routing purely on presence
	// of the literal "outcome" key meant a caller sending the canonical
	// {"status":"approved"} body fell through to the raw FlowControlInput
	// parser instead, which only recognizes status values continue|done|
	// escalate and rejects "approved" outright. Route to the ReviewOutcomeInput
	// parser whenever either shape is present.
	_, hasOutcome := body["outcome"]
	statusVal, _ := body["status"].(string)
	if hasOutcome || reviewOutcomeStatuses[statusVal] {
		// Declared face: ReviewOutcomeInput → FlowControlInput via the face registry.
		roi, err := parseReviewOutcomeInput(body)
		if err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_outcome", err.Error()))
			return
		}
		var mapErr error
		in, mapErr = reviewOutcomeToFlowControl(roi)
		if mapErr != nil {
			writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_outcome", mapErr.Error()))
			return
		}
	} else {
		var err error
		in, err = parseFlowControlInput(body)
		if err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_status", err.Error()))
			return
		}
	}
	runID := r.PathValue("runId")
	s.mu.Lock()
	_, runExists := s.runs[runID]
	s.mu.Unlock()
	if !runExists {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found"))
		return
	}
	if _, err := s.applyFlowControl(runID, in); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "flow_control_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, s.agentGraphSnapshot(runID))
}

// handleExtendCap handles POST /client/workflow-runs/{runId}/agent-loop/extend-cap.
// Body: {} (empty — no parameters needed; limits come from the flow policy defaults).
// Returns an AgentGraphSnapshot so the board can update without a separate refresh.
func (s *InteractiveService) handleExtendCap(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	if _, err := s.extendCap(runID); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "extend_cap_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, s.agentGraphSnapshot(runID))
}

// handleContinueFlow handles POST /client/workflow-runs/{runId}/agent-loop/continue.
// Body: {"feedback": "...", "memberAction": {"action":"retry|skip","node":"..."}} (both optional).
// BUG-231's unified "Continue" action: resumes a blocked/awaiting-user loop.
// Task-241: when blockReason=member_stalled, memberAction selects Retry/Skip.
// Returns an AgentGraphSnapshot so the caller can update without a separate
// refresh round-trip, matching handleExtendCap's contract.
func (s *InteractiveService) handleContinueFlow(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	var body struct {
		Feedback     string       `json:"feedback"`
		MemberAction MemberAction `json:"memberAction"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
			return
		}
	}
	if strings.TrimSpace(body.MemberAction.Action) != "" {
		snap, handled, err := s.handleMemberAction(runID, body.MemberAction)
		if err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "member_action_failed", err.Error()))
			return
		}
		if handled {
			writeInteractiveJSON(w, http.StatusOK, snap)
			return
		}
	}
	// Task-241: opportunistically check stalls before resume (lazy Q-2).
	_ = s.checkAndBlockStalledMembers(runID)
	snap, err := s.resumeFlowWithFeedback(runID, body.Feedback)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusUnprocessableEntity, "continue_flow_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, snap)
}

func fakeArtifacts(runID string) []Artifact {
	return []Artifact{
		{ID: runID + "-final", RunID: runID, Kind: "final_response", Name: "final-response.md", Preview: "Implemented the feature.", CreatedAt: "2026-06-12T10:00:00Z"},
		{ID: runID + "-diff", RunID: runID, Kind: "diff_snapshot", Name: "changes.diff", Preview: "3 files changed", CreatedAt: "2026-06-12T10:00:01Z"},
	}
}
