package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CP-62 P-3 (Task-345): the TUI arms the decision card on
// user_decision_card_requested, submits the matched option id as parked-run
// feedback, and falls back to prose for unmatched input (Q-1).

func decisionCardModel(t *testing.T, captured *[]string, mu *sync.Mutex) *AppModel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/agent-loop/continue") {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]string
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			*captured = append(*captured, payload["feedback"])
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[],"edges":[]}`))
	}))
	t.Cleanup(srv.Close)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.runnerURL = srv.URL
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-345", Status: "running"}
	return m
}

func decisionCardEvent() client.ProviderEvent {
	return client.ProviderEvent{
		Type:          "user_decision_card_requested",
		WorkflowRunID: "run-345",
		DecisionCard: &client.DecisionCardData{
			Question:    "JWT or Session?",
			Recommended: "opt_jwt",
			Options: []client.DecisionCardOption{
				{ID: "opt_jwt", Label: "Stateless JWT", Consequence: "No Redis dependency."},
				{ID: "opt_session", Label: "Redis Session", Consequence: "Instant revoke."},
			},
			Evidence: []client.DecisionCardEvidence{{Path: "docs/arch.md", Line: 45}},
		},
	}
}

func TestDecisionCard_EventArmsCardAndRendersOptions(t *testing.T) {
	var captured []string
	var mu sync.Mutex
	m := decisionCardModel(t, &captured, &mu)
	m2, _ := m.Update(EventMsg{Ev: decisionCardEvent()})
	am := m2.(*AppModel)
	if am.decisionCard == nil {
		t.Fatalf("event must arm the decision card")
	}
	if len(am.decisionCard.Options) != 2 || am.decisionCard.RunID != "run-345" {
		t.Fatalf("card state wrong: %+v", am.decisionCard)
	}
	if am.statusMsg != "decision" {
		t.Fatalf("statusMsg = %q, want decision", am.statusMsg)
	}
	var joined strings.Builder
	for _, msg := range am.messages {
		joined.WriteString(msg.Content + "\n")
	}
	for _, want := range []string{"Decision needed: JWT or Session?", "1. Stateless JWT [recommended]", "2. Redis Session", "docs/arch.md:45"} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("card message missing %q:\n%s", want, joined.String())
		}
	}
}

func TestDecisionCard_NumberInputSubmitsOptionIdAsFeedback(t *testing.T) {
	var captured []string
	var mu sync.Mutex
	m := decisionCardModel(t, &captured, &mu)
	m2, _ := m.Update(EventMsg{Ev: decisionCardEvent()})
	am := m2.(*AppModel)
	m3, cmd := am.handleDecisionCardInput("1")
	if cmd == nil {
		t.Fatalf("feedback command missing")
	}
	if (m3.(*AppModel)).decisionCard != nil {
		t.Fatalf("card must disarm after answering")
	}
	msg := cmd()
	if _, ok := msg.(AgentGraphHydratedMsg); !ok {
		t.Fatalf("want AgentGraphHydratedMsg, got %T", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 || captured[0] != "opt_jwt" {
		t.Fatalf("feedback = %v, want [opt_jwt]", captured)
	}
}

func TestDecisionCard_ProseFallbackSendsVerbatim(t *testing.T) {
	var captured []string
	var mu sync.Mutex
	m := decisionCardModel(t, &captured, &mu)
	m2, _ := m.Update(EventMsg{Ev: decisionCardEvent()})
	am := m2.(*AppModel)
	m3, cmd := am.handleDecisionCardInput("jwt but add refresh tokens")
	if cmd == nil {
		t.Fatalf("feedback command missing")
	}
	if (m3.(*AppModel)).decisionCard != nil {
		t.Fatalf("card must disarm after prose fallback")
	}
	msg := cmd()
	if _, ok := msg.(AgentGraphHydratedMsg); !ok {
		t.Fatalf("want AgentGraphHydratedMsg, got %T", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 || captured[0] != "jwt but add refresh tokens" {
		t.Fatalf("prose fallback must send verbatim, got %v", captured)
	}
}
