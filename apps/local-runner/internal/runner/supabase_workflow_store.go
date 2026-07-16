package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// Phase 5 (04-05): the live Supabase-backed WorkflowStore. The runner reads/writes
// run/step/log state directly via PostgREST (`{apiUrl}/rest/v1/...`), replacing the
// Admin-Web-server / edge-function path so orchestration is not split across tiers.
//
// Trust model: the runner is a local process; it authenticates to Supabase with a
// key held in the OS secret store (supabase:workspace:service-role-key). 04-05
// prefers a least-privilege / RLS-scoped key over a broad service-role key — the
// store takes whatever key it is handed, so the deployment chooses. The request
// shaping (URL, headers, body, PostgREST filters) is exercised by tests over a
// mocked httpRequestFn; end-to-end reads/writes against a real Supabase instance
// (and golden parity vs the TS path) are deferred to a live environment per the
// 04-05 DOD.
type SupabaseWorkflowStore struct {
	restURL string // "{apiUrl}/rest/v1"
	apiKey  string
}

// NewSupabaseWorkflowStore builds the store from the workspace config + the resolved
// API key (service-role or RLS-scoped).
func NewSupabaseWorkflowStore(cfg SupabaseWorkspaceConfig, apiKey string) *SupabaseWorkflowStore {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	return &SupabaseWorkflowStore{restURL: base + "/rest/v1", apiKey: apiKey}
}

// idempotencyKeysOrEmpty never returns nil so jsonb NOT NULL columns get {}.
func idempotencyKeysOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// providerSessionSelectRecovery is the PostgREST select list for full session
// recovery (stall-retry, gate settle, idempotency, runtime blob) — BUG-288 R17/R18.
const providerSessionSelectRecovery = "workflow_run_id,provider_key,provider_session_id,provider_account_id,working_directory,status,last_prompt,last_message,started_at,updated_at,run_kind,parent_run_id,agent_name,agent_role,agent_status,pending_restart_run_id,pending_restart_prompt,pending_restart_gen,flow_context_injected,idempotency_keys,session_runtime,workflow_runs(project_id,workflow_id)"

// sessionRuntimeBlob holds flow-recovery fields that are not first-class
// Supabase columns (BUG-288 R18-3). Packed into session_runtime jsonb.
type sessionRuntimeBlob struct {
	Label                       string                 `json:"label,omitempty"`
	DependsOn                   []string               `json:"depends_on,omitempty"`
	ModelName                   string                 `json:"model_name,omitempty"`
	ChangeType                  string                 `json:"change_type,omitempty"`
	SourceDocID                 string                 `json:"source_doc_id,omitempty"`
	TurnCount                   int                    `json:"turn_count,omitempty"`
	PendingAgentContext         []string               `json:"pending_agent_context,omitempty"`
	LoopState                   AgentLoopState         `json:"loop_state,omitempty"`
	AutoOrchestrate             bool                   `json:"auto_orchestrate,omitempty"`
	FlowCohortID                string                 `json:"flow_cohort_id,omitempty"`
	ActiveFlowEdges             []agentpack.FlowEdge   `json:"active_flow_edges,omitempty"`
	ActiveFlowNodes             []agentpack.FlowNode   `json:"active_flow_nodes,omitempty"`
	ChatSubMode                 string                 `json:"chat_sub_mode,omitempty"`
	ChatFlowRef                 string                 `json:"chat_flow_ref,omitempty"`
	PendingFlowGateSettle       bool                   `json:"pending_flow_gate_settle,omitempty"`
	PendingFlowGateFinalMsg     string                 `json:"pending_flow_gate_final_msg,omitempty"`
	PendingFlowGateOccurredAt   string                 `json:"pending_flow_gate_occurred_at,omitempty"`
	PendingFlowGateTurnID       string                 `json:"pending_flow_gate_turn_id,omitempty"`
	TurnStartGitHead            string                 `json:"turn_start_git_head,omitempty"`
	TurnStartWorktree           map[string]string      `json:"turn_start_worktree,omitempty"`
	PendingGateChangedFiles     []string               `json:"pending_gate_changed_files,omitempty"`
	StepID                      string                 `json:"step_id,omitempty"`
	LastTurnStepID              string                 `json:"last_turn_step_id,omitempty"`
	PendingGateRepromptPrompt   string                 `json:"pending_gate_reprompt_prompt,omitempty"`
	PendingGateRepromptStepID   string                 `json:"pending_gate_reprompt_step_id,omitempty"`
	PendingGateCodePaths        []string               `json:"pending_gate_code_paths,omitempty"`
	RepromptAttempts            int                    `json:"reprompt_attempts,omitempty"`
	PendingResumePrompt         string                 `json:"pending_resume_prompt,omitempty"`
	PendingResumeStepID         string                 `json:"pending_resume_step_id,omitempty"`
	PendingResumeGen            int64                  `json:"pending_resume_gen,omitempty"`
	PendingGateRepromptGen      int64                  `json:"pending_gate_reprompt_gen,omitempty"`
	PendingResumeDeliveredGen   int64                  `json:"pending_resume_delivered_gen,omitempty"`
	PendingGateRepromptDeliveredGen int64              `json:"pending_gate_reprompt_delivered_gen,omitempty"`
	PendingResumeAcceptedTurn   string                 `json:"pending_resume_accepted_turn,omitempty"`
	PendingGateRepromptAcceptedTurn string             `json:"pending_gate_reprompt_accepted_turn,omitempty"`
	PendingResumeFailCount      int                    `json:"pending_resume_fail_count,omitempty"`
	PendingResumeFailGen        int64                  `json:"pending_resume_fail_gen,omitempty"`
	PendingGateRepromptFailCount int                   `json:"pending_gate_reprompt_fail_count,omitempty"`
	PendingGateRepromptFailGen  int64                  `json:"pending_gate_reprompt_fail_gen,omitempty"`
	PendingResumeApprovalID     string                 `json:"pending_resume_approval_id,omitempty"`
	PendingResumeDecision       string                 `json:"pending_resume_decision,omitempty"`
	PendingResumeQuestionChoices []string              `json:"pending_resume_question_choices,omitempty"`
	FlowStartGitHead            string                 `json:"flow_start_git_head,omitempty"`
	StopGeneration              int64                  `json:"stop_generation,omitempty"`
	ParentStopGenSeen           int64                  `json:"parent_stop_gen_seen,omitempty"`
	IntentBlockedKind           string                 `json:"intent_blocked_kind,omitempty"`
	IntentBlockedReason         string                 `json:"intent_blocked_reason,omitempty"`
	IntentBlockedAt             string                 `json:"intent_blocked_at,omitempty"`
	TransitionLogDegraded       bool                   `json:"transition_log_degraded,omitempty"`
	TransitionLogDegradedAt     string                 `json:"transition_log_degraded_at,omitempty"`
	TransitionLogDegradedReason string                 `json:"transition_log_degraded_reason,omitempty"`
}

func sessionRuntimeFromState(s ProviderSessionState) sessionRuntimeBlob {
	return sessionRuntimeBlob{
		Label: s.Label, DependsOn: s.DependsOn, ModelName: s.ModelName,
		ChangeType: s.ChangeType, SourceDocID: s.SourceDocID, TurnCount: s.TurnCount,
		PendingAgentContext: s.PendingAgentContext, LoopState: s.LoopState,
		AutoOrchestrate: s.AutoOrchestrate, FlowCohortID: s.FlowCohortID,
		ActiveFlowEdges: s.ActiveFlowEdges, ActiveFlowNodes: s.ActiveFlowNodes,
		ChatSubMode: s.ChatSubMode, ChatFlowRef: s.ChatFlowRef,
		PendingFlowGateSettle: s.PendingFlowGateSettle, PendingFlowGateFinalMsg: s.PendingFlowGateFinalMsg,
		PendingFlowGateOccurredAt: s.PendingFlowGateOccurredAt, PendingFlowGateTurnID: s.PendingFlowGateTurnID,
		TurnStartGitHead: s.TurnStartGitHead, TurnStartWorktree: s.TurnStartWorktree,
		PendingGateChangedFiles: s.PendingGateChangedFiles, StepID: s.StepID, LastTurnStepID: s.LastTurnStepID,
		PendingGateRepromptPrompt: s.PendingGateRepromptPrompt, PendingGateRepromptStepID: s.PendingGateRepromptStepID,
		PendingGateCodePaths: s.PendingGateCodePaths, RepromptAttempts: s.RepromptAttempts,
		PendingResumePrompt: s.PendingResumePrompt, PendingResumeStepID: s.PendingResumeStepID,
		PendingResumeGen: s.PendingResumeGen, PendingGateRepromptGen: s.PendingGateRepromptGen,
		PendingResumeDeliveredGen: s.PendingResumeDeliveredGen, PendingGateRepromptDeliveredGen: s.PendingGateRepromptDeliveredGen,
		PendingResumeAcceptedTurn: s.PendingResumeAcceptedTurn, PendingGateRepromptAcceptedTurn: s.PendingGateRepromptAcceptedTurn,
		PendingResumeFailCount: s.PendingResumeFailCount, PendingResumeFailGen: s.PendingResumeFailGen,
		PendingGateRepromptFailCount: s.PendingGateRepromptFailCount, PendingGateRepromptFailGen: s.PendingGateRepromptFailGen,
		PendingResumeApprovalID: s.PendingResumeApprovalID, PendingResumeDecision: s.PendingResumeDecision,
		PendingResumeQuestionChoices: s.PendingResumeQuestionChoices, FlowStartGitHead: s.FlowStartGitHead,
		StopGeneration: s.StopGeneration, ParentStopGenSeen: s.ParentStopGenSeen,
		IntentBlockedKind: s.IntentBlockedKind, IntentBlockedReason: s.IntentBlockedReason, IntentBlockedAt: s.IntentBlockedAt,
		TransitionLogDegraded: s.TransitionLogDegraded, TransitionLogDegradedAt: s.TransitionLogDegradedAt,
		TransitionLogDegradedReason: s.TransitionLogDegradedReason,
	}
}

func applySessionRuntime(sess *ProviderSessionState, raw json.RawMessage) {
	if sess == nil || len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return
	}
	var b sessionRuntimeBlob
	if err := json.Unmarshal(raw, &b); err != nil {
		return
	}
	sess.Label = b.Label
	sess.DependsOn = b.DependsOn
	sess.ModelName = b.ModelName
	sess.ChangeType = b.ChangeType
	sess.SourceDocID = b.SourceDocID
	sess.TurnCount = b.TurnCount
	sess.PendingAgentContext = b.PendingAgentContext
	sess.LoopState = b.LoopState
	sess.AutoOrchestrate = b.AutoOrchestrate
	sess.FlowCohortID = b.FlowCohortID
	sess.ActiveFlowEdges = b.ActiveFlowEdges
	sess.ActiveFlowNodes = b.ActiveFlowNodes
	sess.ChatSubMode = b.ChatSubMode
	sess.ChatFlowRef = b.ChatFlowRef
	sess.PendingFlowGateSettle = b.PendingFlowGateSettle
	sess.PendingFlowGateFinalMsg = b.PendingFlowGateFinalMsg
	sess.PendingFlowGateOccurredAt = b.PendingFlowGateOccurredAt
	sess.PendingFlowGateTurnID = b.PendingFlowGateTurnID
	sess.TurnStartGitHead = b.TurnStartGitHead
	sess.TurnStartWorktree = b.TurnStartWorktree
	sess.PendingGateChangedFiles = b.PendingGateChangedFiles
	sess.StepID = b.StepID
	sess.LastTurnStepID = b.LastTurnStepID
	sess.PendingGateRepromptPrompt = b.PendingGateRepromptPrompt
	sess.PendingGateRepromptStepID = b.PendingGateRepromptStepID
	sess.PendingGateCodePaths = b.PendingGateCodePaths
	sess.RepromptAttempts = b.RepromptAttempts
	sess.PendingResumePrompt = b.PendingResumePrompt
	sess.PendingResumeStepID = b.PendingResumeStepID
	sess.PendingResumeGen = b.PendingResumeGen
	sess.PendingGateRepromptGen = b.PendingGateRepromptGen
	sess.PendingResumeDeliveredGen = b.PendingResumeDeliveredGen
	sess.PendingGateRepromptDeliveredGen = b.PendingGateRepromptDeliveredGen
	sess.PendingResumeAcceptedTurn = b.PendingResumeAcceptedTurn
	sess.PendingGateRepromptAcceptedTurn = b.PendingGateRepromptAcceptedTurn
	sess.PendingResumeFailCount = b.PendingResumeFailCount
	sess.PendingResumeFailGen = b.PendingResumeFailGen
	sess.PendingGateRepromptFailCount = b.PendingGateRepromptFailCount
	sess.PendingGateRepromptFailGen = b.PendingGateRepromptFailGen
	sess.PendingResumeApprovalID = b.PendingResumeApprovalID
	sess.PendingResumeDecision = b.PendingResumeDecision
	sess.PendingResumeQuestionChoices = b.PendingResumeQuestionChoices
	sess.FlowStartGitHead = b.FlowStartGitHead
	sess.StopGeneration = b.StopGeneration
	sess.ParentStopGenSeen = b.ParentStopGenSeen
	sess.IntentBlockedKind = b.IntentBlockedKind
	sess.IntentBlockedReason = b.IntentBlockedReason
	sess.IntentBlockedAt = b.IntentBlockedAt
	sess.TransitionLogDegraded = b.TransitionLogDegraded
	sess.TransitionLogDegradedAt = b.TransitionLogDegradedAt
	sess.TransitionLogDegradedReason = b.TransitionLogDegradedReason
}

func (s *SupabaseWorkflowStore) headers(prefer string) map[string]string {
	h := map[string]string{
		"apikey":        s.apiKey,
		"Authorization": "Bearer " + s.apiKey,
		"Content-Type":  "application/json",
	}
	if prefer != "" {
		h["Prefer"] = prefer
	}
	return h
}

// dbStep is the PostgREST row shape for workflow_run_steps. workflow_steps is
// only the workflow-to-step relation; node definition fields live on the joined
// step_definitions row. BUG-164: workflow_steps has no provider_override/
// model_override of its own anymore — a step type has exactly one configured
// model, not a per-workflow-instance override, so Provider/Model are always
// derived from step_definitions.model, never read from workflow_steps.
type dbStep struct {
	ID            string  `json:"id"`
	StepType      string  `json:"step_type"`
	Status        string  `json:"status"`
	StartedAt     *string `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
	RetryCount    int     `json:"retry_count"`
	RejectionNote *string `json:"rejection_note"`
	WorkflowSteps *struct {
		RequiresApproval bool    `json:"requires_approval"`
		BehaviorID       *string `json:"behavior_id"`
		NodeID           *string `json:"node_id"`
		AgentRef         *string `json:"agent_ref"`
		StepDefinitions  *struct {
			YoloMode   bool    `json:"yolo_mode"`
			Model      *string `json:"model"`
			BehaviorID *string `json:"behavior_id"`
			NodeID     *string `json:"node_id"`
			AgentRef   *string `json:"agent_ref"`
		} `json:"step_definitions"`
	} `json:"workflow_steps"`
}

// LoadRunSteps reads a run's steps in execution order.
func (s *SupabaseWorkflowStore) LoadRunSteps(ctx context.Context, runID string) ([]RuntimeWorkflowStep, error) {
	// BUG-NOTE-CP42 #7: behavior_id was missing from this select, so
	// RuntimeWorkflowStep never carried it and runtime helpers like
	// isCodingStepType/isPlanStepType could only classify a step by its
	// step_type — the reusable step_definitions key — which for a CP-42
	// generic flow node is a dispatch category (e.g. "flow-agent-delegate"),
	// not a value NormalizeBehaviorID's alias table recognizes at all. A
	// UI-authored generic flow's coding/plan steps were invisible to this
	// classification, disconnecting them from the Flow Mode context-handoff
	// path entirely.
	endpoint := fmt.Sprintf(
		"%s/workflow_run_steps?workflow_run_id=eq.%s&order=execution_order_index.asc&select=id,step_type,status,started_at,finished_at,retry_count,rejection_note,workflow_steps(requires_approval,step_definitions(yolo_mode,model,behavior_id,node_id,agent_ref))",
		s.restURL, runID,
	)
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("supabase load steps failed: status %d: %s", status, string(body))
	}
	var rows []dbStep
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase load steps decode: %w", err)
	}
	out := make([]RuntimeWorkflowStep, len(rows))
	for i, r := range rows {
		step := RuntimeWorkflowStep{
			ID:         r.ID,
			StepType:   r.StepType,
			Status:     RuntimeWorkflowStepStatus(r.Status),
			RetryCount: r.RetryCount,
		}
		if r.StartedAt != nil {
			step.StartedAt = *r.StartedAt
		}
		if r.FinishedAt != nil {
			step.FinishedAt = *r.FinishedAt
		}
		if r.RejectionNote != nil {
			step.RejectionNote = *r.RejectionNote
		}
		if r.WorkflowSteps != nil {
			step.RequiresApproval = r.WorkflowSteps.RequiresApproval
			if r.WorkflowSteps.StepDefinitions != nil {
				step.YoloMode = r.WorkflowSteps.StepDefinitions.YoloMode
				if r.WorkflowSteps.StepDefinitions.BehaviorID != nil {
					step.BehaviorID = *r.WorkflowSteps.StepDefinitions.BehaviorID
				}
				if r.WorkflowSteps.StepDefinitions.NodeID != nil {
					step.NodeID = *r.WorkflowSteps.StepDefinitions.NodeID
				}
				if r.WorkflowSteps.StepDefinitions.AgentRef != nil {
					step.AgentRef = *r.WorkflowSteps.StepDefinitions.AgentRef
				}
				// BUG-164: a step type has exactly one configured model
				// (step_definitions.model) — no per-workflow-instance override exists
				// anymore, so Model/Provider always come from the step type's own
				// catalog entry.
				if r.WorkflowSteps.StepDefinitions.Model != nil {
					step.Model = *r.WorkflowSteps.StepDefinitions.Model
				}
			}
			if step.BehaviorID == "" && r.WorkflowSteps.BehaviorID != nil {
				step.BehaviorID = *r.WorkflowSteps.BehaviorID
			}
			if step.NodeID == "" && r.WorkflowSteps.NodeID != nil {
				step.NodeID = *r.WorkflowSteps.NodeID
			}
			if step.AgentRef == "" && r.WorkflowSteps.AgentRef != nil {
				step.AgentRef = *r.WorkflowSteps.AgentRef
			}
			if step.Model != "" {
				if pk, ok := providerKeyFromModel(step.Model); ok {
					step.Provider = string(pk)
				}
			}
		}
		out[i] = step
	}
	return out, nil
}

// nilIfEmpty maps the "" == null sentinel to a JSON null.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// buildStepPatchBody renders a WorkflowStepPatch to a PostgREST PATCH body. Only
// fields the patch sets are included; a non-nil pointer to "" becomes JSON null.
func buildStepPatchBody(p WorkflowStepPatch) map[string]any {
	body := map[string]any{"status": string(p.Status)}
	if p.StartedAt != nil {
		body["started_at"] = nilIfEmpty(*p.StartedAt)
	}
	if p.FinishedAt != nil {
		body["finished_at"] = nilIfEmpty(*p.FinishedAt)
	}
	if p.RejectionNote != nil {
		body["rejection_note"] = nilIfEmpty(*p.RejectionNote)
	}
	if p.RetryCount != nil {
		body["retry_count"] = *p.RetryCount
	}
	return body
}

// ApplyStepTransition patches one step row (idempotent: re-applying the same patch
// converges to the same row).
//
// Deprecated: run data is persisted by localFileSessionStore (sessions.ndjson) and
// synced via Drive. The workflow_run_steps Supabase table is no longer written in
// production (CP-36 P-5 / Task-085). Retained for compile-time back-compat only.
func (s *SupabaseWorkflowStore) ApplyStepTransition(ctx context.Context, _ string, t WorkflowStepTransition) error {
	endpoint := fmt.Sprintf("%s/workflow_run_steps?id=eq.%s", s.restURL, t.StepID)
	payload, err := json.Marshal(buildStepPatchBody(t.Patch))
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase apply transition failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// SetRunStatus patches the run-level status + finished_at.
//
// Deprecated: see ApplyStepTransition. The workflow_runs Supabase table is no
// longer written in production (CP-36 P-5 / Task-085).
func (s *SupabaseWorkflowStore) SetRunStatus(ctx context.Context, runID string, runStatus WorkflowRunStatus, finishedAt string) error {
	endpoint := fmt.Sprintf("%s/workflow_runs?id=eq.%s", s.restURL, runID)
	payload, err := json.Marshal(map[string]any{
		"status":      string(runStatus),
		"finished_at": nilIfEmpty(finishedAt),
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase set run status failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// AppendLog inserts a step log row.
//
// Deprecated: see ApplyStepTransition. The workflow_run_logs Supabase table is no
// longer written in production (CP-36 P-5 / Task-085).
func (s *SupabaseWorkflowStore) AppendLog(ctx context.Context, stepID string, log WorkflowLog) error {
	endpoint := s.restURL + "/workflow_run_logs"
	payload, err := json.Marshal(map[string]any{
		"workflow_run_step_id": stepID,
		"log_level":            string(log.LogLevel),
		"message":              log.Message,
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase append log failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// AppendEvent inserts a provider event row.
//
// Deprecated: see ApplyStepTransition. The workflow_provider_events Supabase table
// is no longer written in production (CP-36 P-5 / Task-085).
func (s *SupabaseWorkflowStore) AppendEvent(ctx context.Context, event ProviderEvent) error {
	endpoint := s.restURL + "/workflow_provider_events"
	payload, err := json.Marshal(map[string]any{
		"id":                   event.ID,
		"seq":                  event.Seq,
		"workflow_run_id":      event.WorkflowRunID,
		"workflow_step_run_id": nilIfEmpty(event.WorkflowStepRunID),
		"provider_session_id":  event.ProviderSessionID,
		"provider_key":         string(event.ProviderKey),
		"provider_turn_id":     nilIfEmpty(event.ProviderTurnID),
		"event_type":           string(event.Type),
		"payload_json":         event,
		"occurred_at":          event.OccurredAt,
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase append event failed: status %d: %s", status, string(respBody))
	}
	return nil
}

func (s *SupabaseWorkflowStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	endpoint := s.restURL + "/workflow_provider_sessions"
	// BUG-288 R18-3: pack flow-recovery fields into session_runtime jsonb so
	// reconstruct after restart has gate/cohort/topology/reprompt intents.
	runtimeBlob, _ := json.Marshal(sessionRuntimeFromState(session))
	payload, err := json.Marshal(map[string]any{
		"workflow_run_id":     session.RunID,
		"project_id":          nilIfEmpty(session.ProjectID),
		"workflow_id":         nilIfEmpty(session.WorkflowID),
		"provider_session_id": session.ProviderSessionID,
		"provider_key":        string(session.ProviderKey),
		"provider_account_id": nilIfEmpty(session.ProviderAccountID),
		"working_directory":   nilIfEmpty(session.WorkingDirectory),
		"status":              string(session.Status),
		"last_prompt":         nilIfEmpty(session.LastPrompt),
		"last_message":        nilIfEmpty(session.LastMessage),
		"started_at":          nilIfEmpty(session.StartedAt),
		"updated_at":          nilIfEmpty(session.UpdatedAt),
		"run_kind":            nilIfEmpty(session.RunKind),
		"parent_run_id":       nilIfEmpty(session.ParentRunID),
		"agent_name":          nilIfEmpty(session.AgentName),
		"agent_role":          nilIfEmpty(session.Role),
		"agent_status":        nilIfEmpty(session.AgentStatus),
		// BUG-288 R13–R18 recovery columns.
		"pending_restart_run_id":  nilIfEmpty(session.PendingRestartRunID),
		"pending_restart_prompt":  nilIfEmpty(session.PendingRestartPrompt),
		"pending_restart_gen":     session.PendingRestartGen,
		"flow_context_injected":   session.FlowContextInjected,
		"idempotency_keys":        idempotencyKeysOrEmpty(session.IdempotencyKeys),
		"session_runtime":         json.RawMessage(runtimeBlob),
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase upsert provider session failed: status %d: %s", status, string(respBody))
	}
	return nil
}

func (s *SupabaseWorkflowStore) UpsertApproval(ctx context.Context, approval ProviderApprovalState) error {
	endpoint := s.restURL + "/workflow_provider_approvals"
	payload, err := json.Marshal(map[string]any{
		"id":               approval.ApprovalID,
		"workflow_run_id":  approval.RunID,
		"provider_key":     string(approval.ProviderKey),
		"provider_turn_id": nilIfEmpty(approval.ProviderTurnID),
		"command":          nilIfEmpty(approval.Command),
		"cwd":              nilIfEmpty(approval.Cwd),
		"reason":           nilIfEmpty(approval.Reason),
		"status":           approval.Status,
		"decision":         nilIfEmpty(approval.Decision),
		"policy":           nilIfEmpty(approval.Policy),
		"expires_at":       nilIfEmpty(approval.ExpiresAt),
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase upsert approval failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// dbProviderSessionRow is the PostgREST row shape returned by ListProviderSessionsByProject.
// The workflow_runs field is populated via the !inner join on workflow_run_id.
type dbProviderSessionRow struct {
	WorkflowRunID     string  `json:"workflow_run_id"`
	ProviderKey       string  `json:"provider_key"`
	ProviderSessionID *string `json:"provider_session_id"`
	ProviderAccountID *string `json:"provider_account_id"`
	WorkingDirectory  string  `json:"working_directory"`
	Status            string  `json:"status"`
	LastPrompt        string  `json:"last_prompt"`
	LastMessage       string  `json:"last_message"`
	StartedAt         string  `json:"started_at"`
	UpdatedAt         string  `json:"updated_at"`
	RunKind           string  `json:"run_kind"`
	ParentRunID       string  `json:"parent_run_id"`
	AgentName         string  `json:"agent_name"`
	AgentRole         string  `json:"agent_role"`
	AgentStatus       string  `json:"agent_status"`
	// BUG-288 R13-01 / R13-16 / R16-P0
	PendingRestartRunID  *string           `json:"pending_restart_run_id"`
	PendingRestartPrompt *string           `json:"pending_restart_prompt"`
	PendingRestartGen    *int64            `json:"pending_restart_gen"`
	FlowContextInjected  *bool             `json:"flow_context_injected"`
	IdempotencyKeys      map[string]string `json:"idempotency_keys"`
	SessionRuntime       json.RawMessage   `json:"session_runtime"`
	WorkflowRuns      *struct {
		ProjectID  string `json:"project_id"`
		WorkflowID string `json:"workflow_id"`
	} `json:"workflow_runs"`
}

// ListProviderSessionsByProject implements SessionHistoryReader. It queries
// workflow_provider_sessions joined with workflow_runs (inner) to filter by
// project_id, sorted newest-first. Satisfies the BUG-060 F-2 production gap.
// BUG-288 R17-P0: includes recovery fields (pending restart, idempotency).
func (s *SupabaseWorkflowStore) ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_provider_sessions?select=%s&workflow_runs.project_id=eq.%s&order=updated_at.desc",
		s.restURL,
		strings.Replace(providerSessionSelectRecovery, "workflow_runs(", "workflow_runs!inner(", 1),
		projectID,
	)
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("supabase list provider sessions failed: status %d: %s", code, string(body))
	}
	var rows []dbProviderSessionRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase list provider sessions decode: %w", err)
	}
	out := make([]ProviderSessionState, 0, len(rows))
	for _, r := range rows {
		out = append(out, providerSessionFromDBRow(r))
	}
	return out, nil
}

// ListAllProviderSessions implements SessionIndexReader (BUG-288 R17-P0) so
// flow restart can reconstruct parent+child sessions including stall-retry
// intents. Without this, deliverPendingRestart cannot find children after restart.
func (s *SupabaseWorkflowStore) ListAllProviderSessions(ctx context.Context) ([]ProviderSessionState, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_provider_sessions?select=%s&order=updated_at.desc",
		s.restURL, providerSessionSelectRecovery,
	)
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("supabase list all provider sessions failed: status %d: %s", code, string(body))
	}
	var rows []dbProviderSessionRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase list all provider sessions decode: %w", err)
	}
	out := make([]ProviderSessionState, 0, len(rows))
	for _, r := range rows {
		out = append(out, providerSessionFromDBRow(r))
	}
	return out, nil
}

func providerSessionFromDBRow(r dbProviderSessionRow) ProviderSessionState {
	sess := ProviderSessionState{
		RunID:            r.WorkflowRunID,
		ProviderKey:      ProviderKey(r.ProviderKey),
		WorkingDirectory: r.WorkingDirectory,
		Status:           RunStatus(r.Status),
		LastPrompt:       r.LastPrompt,
		LastMessage:      r.LastMessage,
		StartedAt:        r.StartedAt,
		UpdatedAt:        r.UpdatedAt,
		RunKind:          r.RunKind,
		ParentRunID:      r.ParentRunID,
		AgentName:        r.AgentName,
		Role:             r.AgentRole,
		AgentStatus:      r.AgentStatus,
	}
	if r.ProviderSessionID != nil {
		sess.ProviderSessionID = *r.ProviderSessionID
	}
	if r.ProviderAccountID != nil {
		sess.ProviderAccountID = *r.ProviderAccountID
	}
	if r.PendingRestartRunID != nil {
		sess.PendingRestartRunID = *r.PendingRestartRunID
	}
	if r.PendingRestartPrompt != nil {
		sess.PendingRestartPrompt = *r.PendingRestartPrompt
	}
	if r.PendingRestartGen != nil {
		sess.PendingRestartGen = *r.PendingRestartGen
	}
	if r.FlowContextInjected != nil {
		sess.FlowContextInjected = *r.FlowContextInjected
	}
	if len(r.IdempotencyKeys) > 0 {
		sess.IdempotencyKeys = copyStringMap(r.IdempotencyKeys)
	}
	if r.WorkflowRuns != nil {
		sess.ProjectID = r.WorkflowRuns.ProjectID
		sess.WorkflowID = r.WorkflowRuns.WorkflowID
	}
	applySessionRuntime(&sess, r.SessionRuntime)
	return sess
}

func (s *SupabaseWorkflowStore) GetProviderSession(ctx context.Context, runID string) (ProviderSessionState, bool, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_provider_sessions?workflow_run_id=eq.%s&select=%s&limit=1",
		s.restURL, runID, providerSessionSelectRecovery,
	)
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return ProviderSessionState{}, false, err
	}
	if code < 200 || code >= 300 {
		return ProviderSessionState{}, false, fmt.Errorf("supabase get provider session failed: status %d: %s", code, string(body))
	}
	var rows []dbProviderSessionRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return ProviderSessionState{}, false, fmt.Errorf("supabase get provider session decode: %w", err)
	}
	if len(rows) == 0 {
		return ProviderSessionState{}, false, nil
	}
	return providerSessionFromDBRow(rows[0]), true, nil
}

func (s *SupabaseWorkflowStore) UpsertQuestion(ctx context.Context, question ProviderQuestionState) error {
	endpoint := s.restURL + "/workflow_provider_questions"
	payload, err := json.Marshal(map[string]any{
		"id":               question.QuestionID,
		"workflow_run_id":  question.RunID,
		"provider_turn_id": nilIfEmpty(question.ProviderTurnID),
		"prompt":           question.Prompt,
		"options":          question.Options,
		"multi_select":     question.MultiSelect,
		"status":           question.Status,
		"choice":           question.Choice,
		"expires_at":       nilIfEmpty(question.ExpiresAt),
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase upsert question failed: status %d: %s", status, string(respBody))
	}
	return nil
}
