package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ---- T-30: deterministic workflow-driven question --------------------------

// A workflow step emits user_question_required directly (no model ask_user tool
// call); the same card appears and AnswerQuestion resumes the step.
func TestWorkflowDrivenQuestion(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	type result struct {
		choice []string
		err    *apiErr
	}
	resCh := make(chan result, 1)
	go func() {
		choice, e := svc.AskWorkflowQuestion(context.Background(), runID,
			"Deploy to production?",
			[]QuestionOption{{Label: "Yes", Value: "yes"}, {Label: "No", Value: "no"}}, false)
		resCh <- result{choice, e}
	}()

	// the card appears on the run snapshot (same machinery as the model-driven path)
	var qID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingQuestion != nil && s.PendingQuestion.Prompt == "Deploy to production?" {
			qID = s.PendingQuestion.QuestionID
			return true
		}
		return false
	}, "workflow-driven question card")

	if st := getSnapshot(t, srv.URL, runID).Status; st != RunStatusWaitingQuestion {
		t.Fatalf("run status = %s, want waiting_question", st)
	}

	// answering resumes the step
	if st, _ := doJSON(t, "POST", srv.URL+"/client/questions/"+qID+"/answer", map[string]any{"choice": "yes"}, nil); st != http.StatusOK {
		t.Fatalf("answer status = %d", st)
	}
	r := <-resCh
	if r.err != nil {
		t.Fatalf("AskWorkflowQuestion err: %v", r.err)
	}
	if len(r.choice) != 1 || r.choice[0] != "yes" {
		t.Fatalf("choice = %v, want [yes]", r.choice)
	}
}

func TestWorkflowDrivenQuestionExpires(t *testing.T) {
	svc, srv := newTestServer(t) // questionTTL = 50ms
	runID := startRun(t, srv.URL)
	_, e := svc.AskWorkflowQuestion(context.Background(), runID, "q?", []QuestionOption{{Label: "a", Value: "a"}}, false)
	if e == nil || e.code != "question_expired" {
		t.Fatalf("expected question_expired, got %v", e)
	}
}

// ---- Supabase PostgREST request shaping ------------------------------------

type capturedRequest struct {
	method   string
	endpoint string
	headers  map[string]string
	body     []byte
}

// withMockHTTP swaps the global httpRequestFn for the duration of a test, capturing
// requests and returning a scripted response.
func withMockHTTP(t *testing.T, status int, resp []byte) *[]capturedRequest {
	t.Helper()
	orig := httpRequestFn
	captured := &[]capturedRequest{}
	httpRequestFn = func(_ context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		*captured = append(*captured, capturedRequest{method, endpoint, headers, body})
		return status, resp, nil
	}
	t.Cleanup(func() { httpRequestFn = orig })
	return captured
}

func newTestSupabaseStore() *SupabaseWorkflowStore {
	return NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "test-key")
}

func TestSupabaseStoreLoadRunStepsShaping(t *testing.T) {
	// BUG-NOTE-CP42 #7: the embedded workflow_steps(...) select now also
	// requests behavior_id, so RuntimeWorkflowStep can classify a CP-42
	// generic flow node by its declared behavior instead of only its
	// (dispatch-category) step_type.
	// BUG-155: the embed now also requests node_id/agent_ref, so the desktop
	// sidebar can show the actual per-node name instead of only the shared
	// generic step_type label.
	// BUG-164: workflow_steps has no provider_override/model_override of its
	// own anymore — Model/Provider are always derived from the step type's
	// own step_definitions.model, never from a per-workflow-instance override.
	rows := `[{"id":"s1","step_type":"flow-agent-delegate","status":"PENDING","started_at":null,"retry_count":0,"rejection_note":null,"workflow_steps":{"requires_approval":true,"behavior_id":"agent.delegate","node_id":"coder","agent_ref":"coder","step_definitions":{"yolo_mode":true,"model":"claude-haiku"}}}]`
	cap := withMockHTTP(t, 200, []byte(rows))

	steps, err := newTestSupabaseStore().LoadRunSteps(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "s1" || !steps[0].RequiresApproval {
		t.Fatalf("decoded steps = %+v", steps)
	}
	if steps[0].BehaviorID != "agent.delegate" {
		t.Fatalf("BehaviorID = %q, want agent.delegate", steps[0].BehaviorID)
	}
	if steps[0].NodeID != "coder" {
		t.Fatalf("NodeID = %q, want coder", steps[0].NodeID)
	}
	if steps[0].AgentRef != "coder" {
		t.Fatalf("AgentRef = %q, want coder", steps[0].AgentRef)
	}
	if steps[0].Model != "claude-haiku" {
		t.Fatalf("Model = %q, want claude-haiku (from step_definitions.model)", steps[0].Model)
	}
	if steps[0].Provider != "claude" {
		t.Fatalf("Provider = %q, want claude (derived from the resolved model)", steps[0].Provider)
	}
	if !steps[0].YoloMode {
		t.Fatalf("YoloMode = %v, want true", steps[0].YoloMode)
	}

	req := (*cap)[0]
	if req.method != http.MethodGet {
		t.Fatalf("method = %s, want GET", req.method)
	}
	for _, want := range []string{
		"https://proj.supabase.co/rest/v1/workflow_run_steps",
		"workflow_run_id=eq.run-1",
		"order=execution_order_index.asc",
		"workflow_steps(requires_approval,behavior_id,node_id,agent_ref,step_definitions(yolo_mode,model))",
	} {
		if !strings.Contains(req.endpoint, want) {
			t.Fatalf("endpoint missing %q: %s", want, req.endpoint)
		}
	}
	if req.headers["apikey"] != "test-key" || req.headers["Authorization"] != "Bearer test-key" {
		t.Fatalf("auth headers = %+v", req.headers)
	}
}

func TestSupabaseStoreApplyTransitionShaping(t *testing.T) {
	cap := withMockHTTP(t, 204, nil)

	tr := WorkflowStepTransition{
		StepID: "s1",
		Patch: WorkflowStepPatch{
			Status:        StepStatusDone,
			StartedAt:     strptr("2026-06-12T10:00:00Z"),
			FinishedAt:    strptr("2026-06-12T10:01:00Z"),
			RejectionNote: strptr(""), // null
		},
	}
	if err := newTestSupabaseStore().ApplyStepTransition(context.Background(), "run-1", tr); err != nil {
		t.Fatalf("apply: %v", err)
	}

	req := (*cap)[0]
	if req.method != http.MethodPatch {
		t.Fatalf("method = %s, want PATCH", req.method)
	}
	if !strings.Contains(req.endpoint, "workflow_run_steps?id=eq.s1") {
		t.Fatalf("endpoint = %s", req.endpoint)
	}
	var body map[string]any
	if err := json.Unmarshal(req.body, &body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if body["status"] != "DONE" {
		t.Fatalf("body status = %v", body["status"])
	}
	if body["started_at"] != "2026-06-12T10:00:00Z" {
		t.Fatalf("body started_at = %v", body["started_at"])
	}
	// rejection_note is the "" sentinel → JSON null
	if v, ok := body["rejection_note"]; !ok || v != nil {
		t.Fatalf("rejection_note should be null, got %v (present=%v)", v, ok)
	}
}

func TestSupabaseStoreAppendEventShaping(t *testing.T) {
	cap := withMockHTTP(t, 204, nil)

	event := ProviderEvent{
		ID:                "evt-1",
		Seq:               7,
		Type:              EventTurnCompleted,
		WorkflowRunID:     "run-1",
		WorkflowStepRunID: "step-1",
		ProviderSessionID: "sess-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderTurnID:    "turn-1",
		OccurredAt:        "2026-06-12T10:00:00Z",
		FinalMessage:      "done",
	}
	if err := newTestSupabaseStore().AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("append event: %v", err)
	}

	req := (*cap)[0]
	if req.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", req.method)
	}
	if !strings.Contains(req.endpoint, "workflow_provider_events") {
		t.Fatalf("endpoint = %s", req.endpoint)
	}
	var body map[string]any
	if err := json.Unmarshal(req.body, &body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if body["id"] != "evt-1" || body["seq"].(float64) != 7 {
		t.Fatalf("body id/seq = %#v", body)
	}
	if body["event_type"] != string(EventTurnCompleted) {
		t.Fatalf("body event_type = %v", body["event_type"])
	}
	if body["workflow_run_id"] != "run-1" || body["provider_session_id"] != "sess-1" {
		t.Fatalf("body correlation fields = %#v", body)
	}
}

func TestSupabaseStoreSurfacesHTTPError(t *testing.T) {
	withMockHTTP(t, 401, []byte(`{"message":"unauthorized"}`))
	_, err := newTestSupabaseStore().LoadRunSteps(context.Background(), "run-1")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}
