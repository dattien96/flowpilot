package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func init() { fakeAdapterDelay = 0 }

func newTestServer(t *testing.T) (*InteractiveService, *httptest.Server) {
	t.Helper()
	svc := NewInteractiveService()
	svc.approvalTTL = 50 * time.Millisecond
	svc.questionTTL = 50 * time.Millisecond
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

func doJSON(t *testing.T, method, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func startRun(t *testing.T, base string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: "proj-web", WorkflowID: "wf-feature", StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if h.RunID == "" || h.ProviderSessionID == "" {
		t.Fatalf("empty handle: %+v", h)
	}
	return h.RunID
}

func startProjectRun(t *testing.T, base, projectID, workflowID string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: projectID, WorkflowID: workflowID, StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start project run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	return h.RunID
}

func sendTurn(t *testing.T, base, runID, scenario string, headers map[string]string) (int, string) {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs/"+runID+"/turns",
		map[string]any{"stepId": "step-plan", "prompt": "hi", "scenario": scenario}, headers)
	var out struct {
		TurnID string `json:"turnId"`
	}
	_ = json.Unmarshal(body, &out)
	return status, out.TurnID
}

func getSnapshot(t *testing.T, base, runID string) runSnapshotView {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/client/workflow-runs/"+runID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", status, body)
	}
	var v runSnapshotView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return v
}

type captureTurnAdapter struct {
	ch chan TurnRequest
}

func (a *captureTurnAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *captureTurnAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, SkillSelection: true}
}
func (a *captureTurnAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.ch <- req
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}

func TestChatModeSelectedControlsReachProviderTurnRequest(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:         ProviderKeyClaude,
		DisplayName: "Claude",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, SkillSelection: true, ApprovalEvents: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:       "proj-web",
		ProviderKey:     ProviderKeyClaude,
		Model:           "claude-sonnet-4-5",
		ReasoningEffort: "high",
		YoloMode:        true,
		ChatMode:        "normal_chat",
		Cwd:             "/Users/dev/project",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if handle.StepID == "" {
		t.Fatalf("chat run must return synthetic step id: %+v", handle)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"prompt": "hello",
		"selectedSkills": []map[string]any{
			{"name": "planner", "source": "slash_picker"},
			{"name": "code-review", "source": "slash_picker"},
		},
		"reasoningEffort": "high",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	select {
	case req := <-capture.ch:
		if req.ModelName != "claude-sonnet-4-5" {
			t.Fatalf("ModelName = %q, want claude-sonnet-4-5", req.ModelName)
		}
		if req.ReasoningEffort != "high" {
			t.Fatalf("ReasoningEffort = %q, want high", req.ReasoningEffort)
		}
		if !req.YoloMode {
			t.Fatal("YoloMode = false, want true")
		}
		if req.Cwd != "/Users/dev/project" {
			t.Fatalf("Cwd = %q, want /Users/dev/project", req.Cwd)
		}
		if len(req.SelectedSkills) != 2 || req.SelectedSkills[0].Name != "planner" || req.SelectedSkills[1].Name != "code-review" {
			t.Fatalf("SelectedSkills = %+v, want planner + code-review", req.SelectedSkills)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured provider turn request")
	}
}

// TestChatModeTurnLevelControlsOverrideRunDefaults locks BUG-063: model + YOLO are
// per-turn in chat mode, so changing them between prompts reaches the provider turn
// request — they are no longer frozen at the run-level values captured at startRun.
func TestChatModeTurnLevelControlsOverrideRunDefaults(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-web",
		ProviderKey: ProviderKeyClaude,
		Model:       "model-a",
		YoloMode:    true,
		ChatMode:    "normal_chat",
		Cwd:         "/w",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// Turn 1 omits model/yolo → the run-level defaults (model-a, yolo=true) are used.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "one",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 1 status=%d body=%s", status, body)
	}
	req1 := <-capture.ch
	if req1.ModelName != "model-a" || !req1.YoloMode {
		t.Fatalf("turn 1 req = {model:%q yolo:%v}, want {model-a true}", req1.ModelName, req1.YoloMode)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "turn 1 to finish")

	// Turn 2 supplies new per-turn values → they override the run-level defaults.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "two", "model": "model-b", "yoloMode": false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 2 status=%d body=%s", status, body)
	}
	req2 := <-capture.ch
	if req2.ModelName != "model-b" || req2.YoloMode {
		t.Fatalf("turn 2 req = {model:%q yolo:%v}, want {model-b false}", req2.ModelName, req2.YoloMode)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func adminEvents(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/admin/workflow-runs/"+runID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin events status=%d", status)
	}
	var evs []ProviderEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	return evs
}

func waitTerminal(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	var evs []ProviderEvent
	waitFor(t, func() bool {
		evs = adminEvents(t, base, runID)
		if len(evs) == 0 {
			return false
		}
		last := evs[len(evs)-1].Type
		return last == EventTurnCompleted || last == EventTurnFailed
	}, "terminal event")
	return evs
}

func assertSeqContiguous(t *testing.T, evs []ProviderEvent) {
	t.Helper()
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq not contiguous at %d: got %d (%s)", i, e.Seq, e.Type)
		}
	}
}

// ---- tests -----------------------------------------------------------------

func TestCatalogAndRegistry(t *testing.T) {
	svc, srv := newTestServer(t)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Acme Web App") {
		t.Fatalf("projects status=%d body=%s", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/client/workflows/wf-feature/steps", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "step-plan") {
		t.Fatalf("steps status=%d body=%s", status, body)
	}

	// registry: codex available, claude/gemini placeholders
	status, body = doJSON(t, "GET", srv.URL+"/admin/providers", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("providers status=%d", status)
	}
	for _, want := range []string{`"codex"`, `"claude"`, `"gemini"`, `"available"`, `"placeholder"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("providers missing %s in %s", want, body)
		}
	}
	// disabled/placeholder adapters surface the typed error
	if _, err := svc.registry.Adapter(ProviderKeyClaude); err == nil {
		t.Fatal("expected UnsupportedProviderRuntimeError for claude")
	} else if _, ok := err.(*UnsupportedProviderRuntimeError); !ok {
		t.Fatalf("expected UnsupportedProviderRuntimeError, got %T", err)
	}
}

func TestNormalTurnPersistsWithSeq(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	if status, turnID := sendTurn(t, srv.URL, runID, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	evs := waitTerminal(t, srv.URL, runID)

	assertSeqContiguous(t, evs)
	if evs[0].Type != EventTurnStarted {
		t.Fatalf("first event = %s, want turn_started", evs[0].Type)
	}
	if evs[0].Prompt != "hi" {
		t.Fatalf("turn_started prompt = %q, want hi", evs[0].Prompt)
	}
	if last := evs[len(evs)-1]; last.Type != EventTurnCompleted {
		t.Fatalf("last event = %s, want turn_completed", last.Type)
	}
	if got := getSnapshot(t, srv.URL, runID).Status; got != RunStatusCompleted {
		t.Fatalf("status = %s, want completed", got)
	}

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	steps := store.steps[runID]
	if len(steps) != 1 || steps[0].Status != StepStatusDone {
		t.Fatalf("workflow steps = %+v, want one DONE step", steps)
	}
	if store.runStatus[runID] != RunStatusEngineDone {
		t.Fatalf("run status = %s, want DONE", store.runStatus[runID])
	}
	if store.applies < 2 {
		t.Fatalf("expected running + orchestrator transitions, got applies=%d", store.applies)
	}
	events := store.events[runID]
	if len(events) != 3 {
		t.Fatalf("persisted events = %d, want 3 non-delta events", len(events))
	}
	if events[0].Type != EventTurnStarted || events[1].Type != EventMessageCompleted || events[2].Type != EventTurnCompleted {
		t.Fatalf("persisted events = %+v, want turn_started/message_completed/turn_completed", events)
	}
	if events[0].Prompt != "hi" {
		t.Fatalf("persisted turn_started prompt = %q, want hi", events[0].Prompt)
	}
	for _, ev := range events {
		if ev.Type == EventMessageDelta {
			t.Fatalf("delta event should not be persisted: %+v", ev)
		}
	}
	replayed := streamEvents(t, srv.URL, runID, 0, 1)
	if len(replayed) != 1 || replayed[0].Type != EventTurnStarted || replayed[0].Prompt != "hi" {
		t.Fatalf("replayed first event = %+v, want turn_started with prompt", replayed)
	}
}

func TestProjectRunHistoryFiltersRunsByProject(t *testing.T) {
	_, srv := newTestServer(t)
	first := startProjectRun(t, srv.URL, "proj-web", "wf-feature")
	other := startProjectRun(t, srv.URL, "proj-mobile", "wf-feature")
	second := startProjectRun(t, srv.URL, "proj-web", "wf-feature")

	if status, turnID := sendTurn(t, srv.URL, first, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	waitTerminal(t, srv.URL, first)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history len=%d want 2: %+v", len(history), history)
	}
	for _, item := range history {
		if item.ProjectID != "proj-web" {
			t.Fatalf("history included other project: %+v", item)
		}
		if item.RunID == other {
			t.Fatalf("history included excluded run %s", other)
		}
	}
	if history[0].RunID != first {
		t.Fatalf("first history run=%s want updated run %s (second created run was %s)", history[0].RunID, first, second)
	}
	if history[0].LastPrompt != "hi" || history[0].LastMessage == "" || history[0].Status != RunStatusCompleted {
		t.Fatalf("updated history item missing summary: %+v", history[0])
	}
}

func TestEventStreamReplayNoGapsOrDupes(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "tool-heavy", nil)
	all := waitTerminal(t, srv.URL, runID)
	maxSeq := all[len(all)-1].Seq

	// reconnect from the middle: replay only seq > after
	after := int64(3)
	want := int(maxSeq - after)
	got := streamEvents(t, srv.URL, runID, after, want)
	if len(got) != want {
		t.Fatalf("replay got %d events, want %d", len(got), want)
	}
	for i, e := range got {
		if e.Seq != after+int64(i)+1 {
			t.Fatalf("replay gap/dupe at %d: seq=%d", i, e.Seq)
		}
	}
}

func TestApprovalDenyThenApprove(t *testing.T) {
	_, srv := newTestServer(t)

	// deny
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	var approvalID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingApproval != nil {
			approvalID = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval")

	// invalid decision → 400
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "bogus"}, nil); st != http.StatusBadRequest {
		t.Fatalf("invalid decision status=%d, want 400", st)
	}
	// deny
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "deny"}, nil); st != http.StatusOK {
		t.Fatalf("deny status=%d", st)
	}
	// idempotent: second submit still 200 (first-write-wins)
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "approve"}, nil); st != http.StatusOK {
		t.Fatalf("idempotent resubmit status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if !hasToolStatus(evs, "shell", "cancelled") {
		t.Fatal("deny: expected shell tool_completed cancelled")
	}

	// approve (fresh run)
	runID2 := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID2, "approval-required", nil)
	var approvalID2 string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID2)
		if s.PendingApproval != nil {
			approvalID2 = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval 2")
	doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID2+"/decision", map[string]string{"decision": "approve"}, nil)
	evs2 := waitTerminal(t, srv.URL, runID2)
	if !hasToolStatus(evs2, "shell", "success") {
		t.Fatal("approve: expected shell tool_completed success")
	}
}

func TestQuestionAnswer(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	var qID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingQuestion != nil {
			qID = s.PendingQuestion.QuestionID
			return true
		}
		return false
	}, "pending question")
	if st, _ := doJSON(t, "POST", srv.URL+"/client/questions/"+qID+"/answer", map[string]any{"choice": "css-modules"}, nil); st != http.StatusOK {
		t.Fatalf("answer status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if evs[len(evs)-1].Type != EventTurnCompleted {
		t.Fatal("question: expected turn_completed after answer")
	}
}

func TestInterruptCancelsTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "waiting_question")

	if st, _ := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/interrupt", nil, nil); st != http.StatusOK {
		t.Fatalf("interrupt status=%d", st)
	}
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusCancelled }, "cancelled")
}

func TestConcurrentTurnConflict(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil) // blocks in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("concurrent turn status=%d, want 409", st)
	}
}

func TestIdempotentTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	h := map[string]string{"Idempotency-Key": "k1"}
	_, t1 := sendTurn(t, srv.URL, runID, "question-required", h) // in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")
	st, t2 := sendTurn(t, srv.URL, runID, "question-required", h)
	if st != http.StatusOK || t2 != t1 {
		t.Fatalf("idempotent turn status=%d t1=%s t2=%s", st, t1, t2)
	}
}

func TestApprovalExpiry(t *testing.T) {
	_, srv := newTestServer(t) // TTL = 50ms
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)
	last := evs[len(evs)-1]
	if last.Type != EventTurnFailed || !last.Recoverable {
		t.Fatalf("expiry: last event=%s recoverable=%v, want turn_failed recoverable", last.Type, last.Recoverable)
	}
}

func TestAccountMismatch(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	svc.activeAccountID = "switched"

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("turn after account switch status=%d, want 409", st)
	}
	if st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil); st != http.StatusConflict || !strings.Contains(string(body), "provider_account_changed") {
		t.Fatalf("resume after switch status=%d body=%s", st, body)
	}
}

// ---- helpers ---------------------------------------------------------------

func hasToolStatus(evs []ProviderEvent, tool, status string) bool {
	for _, e := range evs {
		if e.Type == EventToolCompleted && e.ToolName == tool && e.Status == status {
			return true
		}
	}
	return false
}

// streamEvents connects to the SSE endpoint and returns up to wantCount events,
// then cancels.
func streamEvents(t *testing.T, base, runID string, afterSeq int64, wantCount int) []ProviderEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	url := base + "/client/workflow-runs/" + runID + "/events/stream?afterSeq=" +
		strconv.FormatInt(afterSeq, 10)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream connect: %v", err)
	}
	defer resp.Body.Close()

	var out []ProviderEvent
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev ProviderEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		out = append(out, ev)
		if len(out) >= wantCount {
			cancel()
			return out
		}
	}
	return out
}
