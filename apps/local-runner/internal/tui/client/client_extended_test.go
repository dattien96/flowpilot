// CP-56 extended client contract tests — test signatures A0.1-A4.10.
// Tests exercise the full client API surface using httptest servers.
// No live runner required.
package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// ---- A0: Basic client API ---------------------------------------------------

func TestA0_1_HealthCheck_ParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"status": "online", "runnerVersion": "2.0.0",
			"cwd": "/ws", "os": "linux", "startedAt": "2026-08-12T00:00:00Z",
		})
	}))
	defer srv.Close()
	c := client.New(srv.URL)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != "online" {
		t.Errorf("Status=%q want online", h.Status)
	}
	if h.RunnerVersion != "2.0.0" {
		t.Errorf("RunnerVersion=%q want 2.0.0", h.RunnerVersion)
	}
}

func TestA0_2_HealthCheck_ErrorForNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "starting"})
	}))
	defer srv.Close()
	_, err := client.New(srv.URL).Health(context.Background())
	if err == nil {
		t.Fatal("expected error for 503")
	}
	ae, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T", err)
	}
	if ae.Status != 503 {
		t.Errorf("Status=%d want 503", ae.Status)
	}
}

func TestA0_3_ListProjects_ParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "p1", "name": "Project One", "path": "/work/p1"},
		})
	}))
	defer srv.Close()
	ps, err := client.New(srv.URL).ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(ps) != 1 || ps[0].ID != "p1" || ps[0].Name != "Project One" {
		t.Errorf("unexpected projects: %+v", ps)
	}
}

func TestA0_4_ListWorkflows_ParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "w1", "name": "Flow One", "projectId": "p1"},
		})
	}))
	defer srv.Close()
	ws, err := client.New(srv.URL).ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(ws) != 1 || ws[0].ID != "w1" {
		t.Errorf("unexpected workflows: %+v", ws)
	}
}

func TestA0_5_ListSteps_ParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "s1", "name": "Step One", "workflowId": "w1"},
		})
	}))
	defer srv.Close()
	ss, err := client.New(srv.URL).ListSteps(context.Background())
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(ss) != 1 || ss[0].ID != "s1" {
		t.Errorf("unexpected steps: %+v", ss)
	}
}

func TestA0_6_ListProviderAccounts_SnakeCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "a1", "provider_key": "codex", "display_label": "Codex", "is_active": true, "auth_status": "ok"},
		})
	}))
	defer srv.Close()
	accs, err := client.New(srv.URL).ListProviderAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListProviderAccounts: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("want 1 account, got %d", len(accs))
	}
	if accs[0].ProviderKey != "codex" {
		t.Errorf("ProviderKey=%q want codex", accs[0].ProviderKey)
	}
	if !accs[0].IsActive {
		t.Error("IsActive=false want true")
	}
}

func TestA0_7_ListBuiltinOrchestrationOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"flowRef": "bugfix-v1", "label": "Bug Fix", "description": "Fix a bug"},
		})
	}))
	defer srv.Close()
	opts, err := client.New(srv.URL).ListBuiltinOrchestrationOptions(context.Background(), "bug")
	if err != nil {
		t.Fatalf("ListBuiltinOrchestrationOptions: %v", err)
	}
	if len(opts) != 1 || opts[0].FlowRef != "bugfix-v1" {
		t.Errorf("unexpected opts: %+v", opts)
	}
}

func TestA0_8_StartRun_SendsPayload(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-001", Status: "running"})
	}))
	defer srv.Close()
	h, err := client.New(srv.URL).StartRun(context.Background(), client.StartRunInput{
		ProjectID: "p1", ProviderKey: "codex", ChatMode: "normal_chat",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if h.RunID != "run-001" {
		t.Errorf("RunID=%q want run-001", h.RunID)
	}
	if body["projectId"] != "p1" {
		t.Errorf("projectId not sent correctly: %v", body)
	}
	if body["chatMode"] != "normal_chat" {
		t.Errorf("chatMode not sent: %v", body)
	}
}

func TestA0_9_ResumeRun_SendsCorrectEndpoint(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-resume", LastEventSeq: 42})
	}))
	defer srv.Close()
	h, err := client.New(srv.URL).ResumeRun(context.Background(), "run-abc")
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if !strings.Contains(receivedPath, "run-abc") || !strings.Contains(receivedPath, "resume") {
		t.Errorf("unexpected path: %s", receivedPath)
	}
	if h.LastEventSeq != 42 {
		t.Errorf("LastEventSeq=%d want 42", h.LastEventSeq)
	}
}

func TestA0_10_GetRun_ParsesSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.RunSnapshot{RunID: "run-snap", Status: "waiting"})
	}))
	defer srv.Close()
	snap, err := client.New(srv.URL).GetRun(context.Background(), "run-snap")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if snap.RunID != "run-snap" || snap.Status != "waiting" {
		t.Errorf("unexpected snapshot: %+v", snap)
	}
}

func TestA0_11_SubmitApproval_SendsEndpoint(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	err := client.New(srv.URL).SubmitApproval(context.Background(), "app-1", "approve", false)
	if err != nil {
		t.Fatalf("SubmitApproval: %v", err)
	}
	if !strings.Contains(capturedPath, "app-1") {
		t.Errorf("path %q missing approval ID", capturedPath)
	}
	if capturedBody["decision"] != "approve" {
		t.Errorf("decision=%v want approve", capturedBody["decision"])
	}
}

func TestA0_12_AnswerQuestion_SendsEndpoint(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	err := client.New(srv.URL).AnswerQuestion(context.Background(), "q-abc", "yes")
	if err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	if !strings.Contains(capturedPath, "q-abc") {
		t.Errorf("path %q missing question ID", capturedPath)
	}
	if capturedBody["answer"] != "yes" {
		t.Errorf("answer=%v want yes", capturedBody["answer"])
	}
}

func TestA0_13_Interrupt_SendsEndpoint(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	err := client.New(srv.URL).Interrupt(context.Background(), "run-xyz")
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if !strings.Contains(capturedPath, "run-xyz") || !strings.Contains(capturedPath, "interrupt") {
		t.Errorf("unexpected path: %s", capturedPath)
	}
}

func TestA0_14_SubmitGateDecision_SendsEndpoint(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	err := client.New(srv.URL).SubmitGateDecision(context.Background(), "run-gate", "continue")
	if err != nil {
		t.Fatalf("SubmitGateDecision: %v", err)
	}
	if !strings.Contains(capturedPath, "run-gate") || !strings.Contains(capturedPath, "gate") {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if capturedBody["decision"] != "continue" {
		t.Errorf("decision=%v want continue", capturedBody["decision"])
	}
}

func TestA0_15_ListProviders_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/providers" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"key": "codex", "label": "OpenAI Codex", "models": []map[string]any{
				{"id": "o4-mini", "display_name": "o4-mini", "available": true, "supported_reasoning_efforts": []string{"high", "medium", "low"}},
			}},
		})
	}))
	defer srv.Close()
	ps, err := client.New(srv.URL).ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(ps) != 1 || ps[0].Key != "codex" {
		t.Errorf("unexpected providers: %+v", ps)
	}
	if len(ps[0].Models) != 1 || ps[0].Models[0].ModelID() != "o4-mini" {
		t.Errorf("unexpected models: %+v", ps[0].Models)
	}
}

func TestA0_16_ListSkills_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "coding", "source": "builtin"},
			{"name": "testing", "source": "builtin"},
		})
	}))
	defer srv.Close()
	skills, err := client.New(srv.URL).ListSkills(context.Background(), "codex", "/ws")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	if skills[0].Name != "coding" {
		t.Errorf("unexpected first skill: %+v", skills[0])
	}
}

func TestA0_17_ListAgents_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"runId": "r1", "agentName": "main", "status": "running"},
		})
	}))
	defer srv.Close()
	agents, err := client.New(srv.URL).ListAgents(context.Background(), "/ws")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].AgentName != "main" {
		t.Errorf("unexpected agents: %+v", agents)
	}
}

func TestA0_18_NormalizeImage_RejectsUnsupportedFormat(t *testing.T) {
	// Use raw text data that image.Decode cannot parse.
	unsupportedData := []byte("this is definitely not an image file")
	_, err := client.NormalizeImage(unsupportedData, "test.txt")
	if err == nil {
		t.Error("expected error for non-image data, got nil")
	}
}

func TestA0_19_NormalizeImage_ResizesOversizedImage(t *testing.T) {
	// Create a PNG that exceeds 1568px on the long edge.
	const testW, testH = 2000, 1000
	imgData := makeTestPNG(t, testW, testH)

	att, err := client.NormalizeImage(imgData, "large.png")
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	if att.Width > 1568 || att.Height > 1568 {
		t.Errorf("output dims %dx%d exceed 1568px cap", att.Width, att.Height)
	}
	if att.MimeType != "image/png" {
		t.Errorf("MimeType=%q want image/png (input was PNG)", att.MimeType)
	}
}

// ---- A1: SendTurn -----------------------------------------------------------

func TestA1_1_SendTurn_SendsBasicPrompt(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-1", nil)
	defer srv.Close()
	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "hello world"})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedBody["prompt"] != "hello world" {
		t.Errorf("prompt=%v want 'hello world'", capturedBody["prompt"])
	}
}

func TestA1_2_SendTurn_SendsSkillSelection(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-2", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:          "r1",
		Prompt:         "p",
		SelectedSkills: []client.SkillSelection{{Name: "coding", Source: "builtin"}},
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	skills, ok := capturedBody["selectedSkills"]
	if !ok || skills == nil {
		t.Error("selectedSkills not sent")
	}
}

func TestA1_3_SendTurn_SendsPromptAttachment(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-3", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:  "r1",
		Prompt: "p",
		Attachments: []client.PromptAttachment{{
			ID: "att-1", Kind: "image", OriginalName: "test.png",
			MimeType: "image/png", Data: "abc123", SizeBytes: 100,
		}},
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	atts, ok := capturedBody["attachments"]
	if !ok || atts == nil {
		t.Error("attachments not sent")
	}
}

func TestA1_4_SendTurn_SendsIdempotencyKey(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-4", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:          "r1",
		Prompt:         "p",
		IdempotencyKey: "idem-key-123",
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedBody["idempotencyKey"] != "idem-key-123" {
		t.Errorf("idempotencyKey=%v want idem-key-123", capturedBody["idempotencyKey"])
	}
}

func TestA1_5_SendTurn_SendsSubModeAndFlowRef(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-5", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:      "r1",
		Prompt:     "fix the bug",
		SubMode:    "bug",
		FlowRef:    "bugfix-v1",
		ChangeType: "bugfix",
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedBody["subMode"] != "bug" {
		t.Errorf("subMode=%v want bug", capturedBody["subMode"])
	}
	if capturedBody["flowRef"] != "bugfix-v1" {
		t.Errorf("flowRef=%v want bugfix-v1", capturedBody["flowRef"])
	}
	if capturedBody["changeType"] != "bugfix" {
		t.Errorf("changeType=%v want bugfix", capturedBody["changeType"])
	}
}

func TestA1_6_SendTurn_SendsYoloModeTrue(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-6", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	yolo := true
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:    "r1",
		Prompt:   "p",
		YoloMode: &yolo,
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedBody["yoloMode"] != true {
		t.Errorf("yoloMode=%v want true", capturedBody["yoloMode"])
	}
}

func TestA1_7_SendTurn_SendsReasoningEffort(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-7", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:           "r1",
		Prompt:          "p",
		ReasoningEffort: "high",
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedBody["reasoningEffort"] != "high" {
		t.Errorf("reasoningEffort=%v want high", capturedBody["reasoningEffort"])
	}
}

func TestA1_8_SendTurn_FiltersByProviderTurnId(t *testing.T) {
	const wantID, otherID = "turn-want", "turn-other"
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, wantID, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "message_delta", ProviderTurnID: otherID, Text: "skip"},
		{ID: "e2", Seq: 2, Type: "message_delta", ProviderTurnID: wantID, Text: "keep"},
		{ID: "e3", Seq: 3, Type: "turn_completed", ProviderTurnID: wantID},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	var received []client.ProviderEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	for _, ev := range received {
		if ev.ProviderTurnID != "" && ev.ProviderTurnID != wantID {
			t.Errorf("received event from wrong turn %q", ev.ProviderTurnID)
		}
	}
	// Should receive the 2 events for wantID (e2 + e3), not e1.
	if len(received) != 2 {
		t.Errorf("got %d events, want 2", len(received))
	}
}

func TestA1_9_SendTurn_HandlesEmptyStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": "t-empty"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			// Close immediately with no events.
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	// Should return without error (empty stream is not an error).
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error for empty stream: %v", err)
	}
}

// ---- A2: SSE stream parsing -------------------------------------------------

func TestA2_1_StreamRun_ReceivesEventsInOrder(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "turn_started"},
		{ID: "e2", Seq: 2, Type: "message_delta", Text: "hello"},
		{ID: "e3", Seq: 3, Type: "turn_completed"},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3", len(evs))
	}
	if evs[0].Type != "turn_started" || evs[1].Type != "message_delta" || evs[2].Type != "turn_completed" {
		t.Errorf("unexpected event types: %v", evs)
	}
}

func TestA2_2_StreamRun_ParsesMessageDelta(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "message_delta", Text: "world"},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].Text != "world" {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestA2_3_StreamRun_ParsesTurnCompleted(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "turn_completed", FinalMessage: "final answer"},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].FinalMessage != "final answer" {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestA2_4_StreamRun_ParsesPermissionRequired(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "permission_required", ApprovalID: "app-99"},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].ApprovalID != "app-99" {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestA2_5_StreamRun_ParsesUserQuestionRequired(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "user_question_required",
			QuestionID: "q-1", Prompt: "Proceed?",
			Options: []map[string]string{{"label": "Yes"}, {"label": "No"}},
		},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].QuestionID != "q-1" || len(evs[0].Options) != 2 {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestA2_6_StreamRun_ParsesFlowGateViolation(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "flow_gate_violation",
			GateOptions:        []string{"continue", "stop"},
			GateRegressedTests: []string{"TestFoo"},
		},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || len(evs[0].GateOptions) != 2 || evs[0].GateRegressedTests[0] != "TestFoo" {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestA2_7_StreamRun_ParsesTokenUsage(t *testing.T) {
	ctxTokens := int64(1024)
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "token_usage_updated",
			TokenUsage: &client.TokenUsageSnapshot{
				Last: &client.TokenUsageBreakdown{TotalTokens: 512},
				ModelContextWindow: &ctxTokens,
			},
		},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].TokenUsage == nil {
		t.Fatalf("expected token_usage_updated event with TokenUsage")
	}
	if evs[0].TokenUsage.Last.TotalTokens != 512 {
		t.Errorf("TotalTokens=%d want 512", evs[0].TokenUsage.Last.TotalTokens)
	}
}

func TestA2_8_StreamRun_ParsesAgentGraphUpdated(t *testing.T) {
	srv := newStreamSrv(t, []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "agent_graph_updated",
			AgentGraph: &client.AgentGraphSnapshot{
				ParentRunID: "run-p",
				Runs: []client.AgentRunSummary{
					{RunID: "r-main", AgentName: "main", Status: "running"},
				},
			},
		},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamRun(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if len(evs) != 1 || evs[0].AgentGraph == nil {
		t.Fatalf("expected agent_graph_updated event with AgentGraph")
	}
	if len(evs[0].AgentGraph.Runs) != 1 || evs[0].AgentGraph.Runs[0].AgentName != "main" {
		t.Errorf("unexpected agent graph: %+v", evs[0].AgentGraph)
	}
}

func TestA2_9_StreamWithReconnect_ReconnectsOnDisconnect(t *testing.T) {
	var connCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		cnt := atomic.AddInt32(&connCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)
		if cnt == 1 {
			// First connection: send one event then close.
			ev := client.ProviderEvent{ID: "e1", Seq: 1, Type: "message_delta", Text: "hi"}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		// Second+ connection: send turn_completed.
		ev := client.ProviderEvent{ID: "e2", Seq: 2, Type: "turn_completed", FinalMessage: "done"}
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamWithReconnect(ctx, "r1", 0)
	var evs []client.ProviderEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	if atomic.LoadInt32(&connCount) < 2 {
		t.Errorf("expected at least 2 connections (reconnect), got %d", connCount)
	}
	if len(evs) < 2 {
		t.Errorf("got %d events, want at least 2 (across reconnects)", len(evs))
	}
}

// ---- A3: RetryableCode ------------------------------------------------------

func TestA3_1_RetryableCode_TurnInProgress(t *testing.T) {
	if !client.RetryableCode("turn_in_progress") {
		t.Error("turn_in_progress should be retryable")
	}
}

func TestA3_2_RetryableCode_GateInProgress(t *testing.T) {
	if !client.RetryableCode("gate_in_progress") {
		t.Error("gate_in_progress should be retryable")
	}
}

func TestA3_3_RetryableCode_HubParked(t *testing.T) {
	if !client.RetryableCode("hub_parked") {
		t.Error("hub_parked should be retryable")
	}
}

func TestA3_4_RetryableCode_ReturnsFalseForUnknown(t *testing.T) {
	for _, code := range []string{"not_found", "http_error", "internal_error", ""} {
		if client.RetryableCode(code) {
			t.Errorf("code %q should NOT be retryable", code)
		}
	}
}

func TestA3_5_SendTurn_ExhaustsRetriesAndErrors(t *testing.T) {
	// Server always returns turn_in_progress.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "turn_in_progress", "message": "busy"},
			})
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	err := <-errCh
	if err == nil {
		t.Error("expected error after exhausting retries")
	}
}

// ---- A3b: Retry with live server --------------------------------------------

func TestA3b_1_SendTurn_RetriesOnTurnInProgress(t *testing.T) {
	attempts := 0
	srv := newRetryTurnSrv(t, &attempts, "turn_in_progress", 3)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 4 {
		t.Errorf("attempts=%d want 4 (3 retries + 1 success)", attempts)
	}
}

func TestA3b_2_SendTurn_RetriesOnGateInProgress(t *testing.T) {
	attempts := 0
	srv := newRetryTurnSrv(t, &attempts, "gate_in_progress", 2)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts=%d want 3", attempts)
	}
}

func TestA3b_3_SendTurn_RetriesOnHubParked(t *testing.T) {
	attempts := 0
	srv := newRetryTurnSrv(t, &attempts, "hub_parked", 1)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts=%d want 2", attempts)
	}
}

func TestA3b_4_SendTurn_NoRetryOnNonRetryableError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns") {
			attempts++
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "not_found", "message": "run not found"},
			})
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err == nil {
		t.Error("expected error for not_found")
	}
	// Should only attempt once (no retry).
	if attempts != 1 {
		t.Errorf("attempts=%d want 1 (no retry for not_found)", attempts)
	}
}

func TestA3b_5_SendTurn_SucceedsAfter3Retries(t *testing.T) {
	attempts := 0
	srv := newRetryTurnSrv(t, &attempts, "turn_in_progress", 3)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestA3b_6_SendTurn_FailsAfterMaxRetries(t *testing.T) {
	// Returns turn_in_progress every time (7+ failures > maxTurnRetries=6).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "turn_in_progress", "message": "busy"},
			})
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	if err := <-errCh; err == nil {
		t.Error("expected error after max retries exhausted")
	}
}

func TestA3b_7_SendTurn_AbortsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns") {
			// Simulate slow response.
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "turn_in_progress", "message": "busy"},
			})
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	drainEvents(evCh)
	err := <-errCh
	if err == nil {
		t.Error("expected error after context cancel")
	}
}

func TestA3b_8_SendTurn_DeliversEventsCorrectly(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-ok", []client.ProviderEvent{
		{ID: "e1", Seq: 1, Type: "message_delta", ProviderTurnID: "turn-ok", Text: "hello"},
		{ID: "e2", Seq: 2, Type: "turn_completed", ProviderTurnID: "turn-ok", FinalMessage: "hello"},
	})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{RunID: "r1", Prompt: "p"})
	var evs []client.ProviderEvent
	for ev := range evCh {
		evs = append(evs, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evs) != 2 {
		t.Errorf("got %d events, want 2", len(evs))
	}
	if evs[0].Text != "hello" {
		t.Errorf("first event text=%q want hello", evs[0].Text)
	}
}

func TestA3b_9_SendTurn_WithSkillsSendsSelectedSkills(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-skills", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:  "r1",
		Prompt: "p",
		SelectedSkills: []client.SkillSelection{
			{Name: "coding", Source: "builtin"},
			{Name: "testing", Source: "user"},
		},
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	skills, ok := capturedBody["selectedSkills"].([]any)
	if !ok || len(skills) != 2 {
		t.Errorf("selectedSkills not sent correctly: %v", capturedBody["selectedSkills"])
	}
}

func TestA3b_10_SendTurn_WithAttachmentsSendsAttachments(t *testing.T) {
	var capturedBody map[string]any
	srv := newTurnSrv(t, &capturedBody, "turn-attach", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evCh, errCh := client.New(srv.URL).SendTurn(ctx, client.TurnInput{
		RunID:  "r1",
		Prompt: "look at this image",
		Attachments: []client.PromptAttachment{
			{ID: "att-1", Kind: "image", OriginalName: "screenshot.png",
				MimeType: "image/png", Data: "abc123", SizeBytes: 100, Width: 100, Height: 100},
		},
	})
	drainEvents(evCh)
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	atts, ok := capturedBody["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Errorf("attachments not sent correctly: %v", capturedBody["attachments"])
	}
}

// ---- A4: Provider/Skill/Agent/YOLO ------------------------------------------

func TestA4_1_ListProviders_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"key": "codex", "name": "Codex"},
			{"key": "claude", "name": "Claude"},
		})
	}))
	defer srv.Close()
	ps, err := client.New(srv.URL).ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("want 2 providers, got %d", len(ps))
	}
}

func TestA4_2_ListProviders_ParsesModelReasoningEfforts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"key": "codex", "label": "Codex", "models": []map[string]any{
				{"id": "o3", "display_name": "o3", "available": true,
					"supported_reasoning_efforts": []string{"high", "medium", "low"},
					"context_window_tokens":       128000},
			}},
		})
	}))
	defer srv.Close()
	ps, err := client.New(srv.URL).ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(ps[0].Models) != 1 {
		t.Fatalf("want 1 model, got %d", len(ps[0].Models))
	}
	m := ps[0].Models[0]
	if m.ModelID() != "o3" {
		t.Errorf("ModelID=%q want o3", m.ModelID())
	}
	if len(m.SupportedReasoningEfforts) != 3 {
		t.Errorf("want 3 reasoning efforts, got %v", m.SupportedReasoningEfforts)
	}
	if m.ContextWindowTokens != 128000 {
		t.Errorf("ContextWindowTokens=%v want 128000", m.ContextWindowTokens)
	}
}

func TestA4_3_ListSkills_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "coding", "source": "builtin"},
		})
	}))
	defer srv.Close()
	skills, err := client.New(srv.URL).ListSkills(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "coding" {
		t.Errorf("unexpected skills: %+v", skills)
	}
}

func TestA4_4_ListSkills_PassesProviderParam(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()
	client.New(srv.URL).ListSkills(context.Background(), "codex", "") //nolint:errcheck
	if !strings.Contains(receivedQuery, "provider=codex") {
		t.Errorf("query %q missing provider=codex", receivedQuery)
	}
}

func TestA4_5_ListSkills_PassesCwdParam(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()
	client.New(srv.URL).ListSkills(context.Background(), "", "/work/project") //nolint:errcheck
	if !strings.Contains(receivedQuery, "cwd=") {
		t.Errorf("query %q missing cwd param", receivedQuery)
	}
}

func TestA4_6_ListAgents_ParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"runId": "r1", "agentName": "main", "status": "running", "role": "orchestrator"},
		})
	}))
	defer srv.Close()
	agents, err := client.New(srv.URL).ListAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Role != "orchestrator" {
		t.Errorf("unexpected agents: %+v", agents)
	}
}

func TestA4_7_ListAgents_PassesCwdParam(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()
	client.New(srv.URL).ListAgents(context.Background(), "/my/workspace") //nolint:errcheck
	if !strings.Contains(receivedQuery, "cwd=") {
		t.Errorf("query %q missing cwd param", receivedQuery)
	}
}

func TestA4_8_ApplyGrokYoloPosture_True(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	if err := client.New(srv.URL).ApplyGrokYoloPosture(context.Background(), true); err != nil {
		t.Fatalf("ApplyGrokYoloPosture: %v", err)
	}
	if body["yolo"] != true {
		t.Errorf("yolo=%v want true", body["yolo"])
	}
}

func TestA4_9_ApplyGrokYoloPosture_False(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	if err := client.New(srv.URL).ApplyGrokYoloPosture(context.Background(), false); err != nil {
		t.Fatalf("ApplyGrokYoloPosture(false): %v", err)
	}
	if body["yolo"] != false {
		t.Errorf("yolo=%v want false", body["yolo"])
	}
}

func TestA4_10_ListProviderAccounts_SnakeCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "a1", "provider_key": "claude", "display_label": "Claude (Sonnet)",
				"is_active": true, "auth_status": "authenticated"},
		})
	}))
	defer srv.Close()
	accs, err := client.New(srv.URL).ListProviderAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListProviderAccounts: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("want 1, got %d", len(accs))
	}
	if accs[0].ProviderKey != "claude" || accs[0].DisplayLabel != "Claude (Sonnet)" {
		t.Errorf("unexpected account: %+v", accs[0])
	}
}

// ---- Helpers ----------------------------------------------------------------

// newTurnSrv creates a test server for SendTurn. It captures the body into
// capturedBody and serves the given events (if non-nil) on the SSE stream.
// The POST response uses turnID as the turnId field.
func newTurnSrv(t *testing.T, capturedBody *map[string]any, turnID string, events []client.ProviderEvent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewDecoder(r.Body).Decode(capturedBody)
			json.NewEncoder(w).Encode(map[string]string{"turnId": turnID})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)
			if events == nil {
				// Default: single turn_completed for turnID.
				events = []client.ProviderEvent{
					{ID: "e1", Seq: 1, Type: "turn_completed", ProviderTurnID: turnID},
				}
			}
			for _, ev := range events {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

// newStreamSrv creates a test server that serves the given events on the SSE stream.
func newStreamSrv(t *testing.T, events []client.ProviderEvent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)
		for _, ev := range events {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
}

// newRetryTurnSrv creates a server that returns errCode the first failCount
// times, then succeeds with turnID="turn-ok".
func newRetryTurnSrv(t *testing.T, attempts *int, errCode string, failCount int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			*attempts++
			if *attempts <= failCount {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"code": errCode, "message": "retryable"},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-ok"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			ev := client.ProviderEvent{ID: "e1", Seq: 1, Type: "turn_completed", ProviderTurnID: "turn-ok"}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
}

// drainEvents reads all events from evCh (discarding them).
func drainEvents(ch <-chan client.ProviderEvent) {
	for range ch {
	}
}

// makeTestPNG creates a minimal all-black PNG of dimensions w×h for testing.
func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("makeTestPNG encode: %v", err)
	}
	return buf.Bytes()
}
