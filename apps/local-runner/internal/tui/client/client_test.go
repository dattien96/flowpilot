// Tests A0-A4: CP-56 TUI client contract tests.
// These tests use httptest servers — no live runner required.
package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// A0: HealthCheck parses the /health JSON response correctly.
func TestA0_HealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"status":        "online",
			"runnerVersion": "1.2.3",
			"cwd":           "/workspace",
			"os":            "linux",
			"startedAt":     "2026-08-12T02:00:00Z",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != "online" {
		t.Errorf("Status = %q, want %q", h.Status, "online")
	}
	if h.RunnerVersion != "1.2.3" {
		t.Errorf("RunnerVersion = %q, want %q", h.RunnerVersion, "1.2.3")
	}
	if h.Cwd != "/workspace" {
		t.Errorf("Cwd = %q, want %q", h.Cwd, "/workspace")
	}
}

// A0b: Health returns APIError for non-200 responses.
func TestA0b_HealthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "runner starting"})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusServiceUnavailable)
	}
}

// A1: SendTurn posts the turn payload and filters SSE events by providerTurnId.
func TestA1_SendTurn_FiltersByProviderTurnId(t *testing.T) {
	const wantTurnID = "turn-abc"
	const otherTurnID = "turn-xyz"

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewDecoder(r.Body).Decode(&capturedBody)
			json.NewEncoder(w).Encode(map[string]string{"turnId": wantTurnID})

		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)

			events := []client.ProviderEvent{
				{ID: "e1", Seq: 1, Type: "message_delta", ProviderTurnID: otherTurnID, Text: "other"},
				{ID: "e2", Seq: 2, Type: "message_delta", ProviderTurnID: wantTurnID, Text: "hello"},
				{ID: "e3", Seq: 3, Type: "turn_completed", ProviderTurnID: wantTurnID, FinalMessage: "hello"},
			}
			for _, ev := range events {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{
		RunID:  "run-1",
		Prompt: "test prompt",
	})

	var received []client.ProviderEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn error: %v", err)
	}

	if len(received) != 2 {
		t.Fatalf("received %d events, want 2 (filtered to wantTurnID)", len(received))
	}
	for _, ev := range received {
		if ev.ProviderTurnID != wantTurnID {
			t.Errorf("received event with unexpected turnId %q", ev.ProviderTurnID)
		}
	}
	// Verify the prompt was sent
	if capturedBody["prompt"] != "test prompt" {
		t.Errorf("prompt not sent correctly: %v", capturedBody)
	}
}

// A2: SSE stream parses multi-event frames correctly.
func TestA2_SSEStreamParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []client.ProviderEvent{
			{ID: "e1", Seq: 1, Type: "turn_started"},
			{ID: "e2", Seq: 2, Type: "message_delta", Text: "world"},
			{ID: "e3", Seq: 3, Type: "turn_completed"},
		}
		for _, ev := range events {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := c.StreamRun(ctx, "run-1", 0)

	var received []client.ProviderEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 3 {
		t.Fatalf("received %d events, want 3", len(received))
	}
	if received[1].Text != "world" {
		t.Errorf("event 2 text = %q, want %q", received[1].Text, "world")
	}
	if received[2].Type != "turn_completed" {
		t.Errorf("event 3 type = %q, want turn_completed", received[2].Type)
	}
}

// A3: RetryableCode identifies turn_in_progress, gate_in_progress, hub_parked.
func TestA3_RetryableCode(t *testing.T) {
	cases := []struct {
		code      string
		retryable bool
	}{
		{"turn_in_progress", true},
		{"gate_in_progress", true},
		{"hub_parked", true},
		{"not_found", false},
		{"http_error", false},
		{"", false},
	}
	for _, tc := range cases {
		got := client.RetryableCode(tc.code)
		if got != tc.retryable {
			t.Errorf("RetryableCode(%q) = %v, want %v", tc.code, got, tc.retryable)
		}
	}
}

// A3b: SendTurn retries on turn_in_progress up to 6 times.
func TestA3b_SendTurn_RetriesOnTurnInProgress(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			attempts++
			if attempts <= 3 {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"code": "turn_in_progress", "message": "busy"},
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
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-1", Prompt: "hi"})
	for range evCh {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if attempts != 4 {
		t.Errorf("attempts = %d, want 4 (3 retryable + 1 success)", attempts)
	}
}

// A4: ApplyGrokYoloPosture calls POST /provider-accounts/grok-yolo-posture.
func TestA4_ApplyGrokYoloPosture(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider-accounts/grok-yolo-posture" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	if err := c.ApplyGrokYoloPosture(context.Background(), true); err != nil {
		t.Fatalf("ApplyGrokYoloPosture: %v", err)
	}
	if capturedBody["yolo"] != true {
		t.Errorf("yolo field = %v, want true", capturedBody["yolo"])
	}

	if err := c.ApplyGrokYoloPosture(context.Background(), false); err != nil {
		t.Fatalf("ApplyGrokYoloPosture(false): %v", err)
	}
	if capturedBody["yolo"] != false {
		t.Errorf("yolo field = %v, want false", capturedBody["yolo"])
	}
}

// A4b: ListProviderAccounts parses snake_case JSON correctly.
func TestA4b_ListProviderAccounts_SnakeCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":            "acc-1",
				"provider_key":  "codex",
				"display_label": "Codex (default)",
				"is_active":     true,
				"auth_status":   "authenticated",
			},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	accounts, err := c.ListProviderAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListProviderAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	acc := accounts[0]
	if acc.ProviderKey != "codex" {
		t.Errorf("ProviderKey = %q, want %q", acc.ProviderKey, "codex")
	}
	if !acc.IsActive {
		t.Error("IsActive = false, want true")
	}
	if acc.DisplayLabel != "Codex (default)" {
		t.Errorf("DisplayLabel = %q", acc.DisplayLabel)
	}
}
