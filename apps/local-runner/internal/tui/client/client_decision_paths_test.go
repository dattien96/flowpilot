package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitApproval_PostsDecisionPath(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	if err := New(srv.URL).SubmitApproval(t.Context(), "app-1", "approve", false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(capturedPath, "/client/approvals/app-1/decision") {
		t.Fatalf("path=%q want .../client/approvals/app-1/decision", capturedPath)
	}
	if capturedBody["decision"] != "approve" {
		t.Fatalf("body=%v", capturedBody)
	}
}

func TestAnswerQuestion_PostsAnswerPath(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	if err := New(srv.URL).AnswerQuestion(t.Context(), "q-abc", "yes"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(capturedPath, "/client/questions/q-abc/answer") {
		t.Fatalf("path=%q", capturedPath)
	}
	if capturedBody["choice"] != "yes" {
		t.Fatalf("body=%v", capturedBody)
	}
}

func TestSubmitGateDecision_PostsGateDecisionPath(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	if err := New(srv.URL).SubmitGateDecision(t.Context(), "run-gate", "continue"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capturedPath, "/gate-decision") {
		t.Fatalf("path=%q", capturedPath)
	}
	if capturedBody["option"] != "continue" {
		t.Fatalf("body=%v", capturedBody)
	}
}
