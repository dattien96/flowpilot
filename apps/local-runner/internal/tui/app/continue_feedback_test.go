package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-325 follow-up: /continue carries trailing text as human feedback
// (plan_approval feedback re-enters the writer); bare /continue keeps the
// legacy body. The TUI previously sent hardcoded feedback:"continue" with no
// way to attach human text (Desktop already had the textarea).

func TestContinueFeedbackFromArgs(t *testing.T) {
	if got := continueFeedbackFromArgs(nil); got != "" {
		t.Fatalf("nil args = %q, want empty (legacy path)", got)
	}
	if got := continueFeedbackFromArgs([]string{"", "  "}); got != "" {
		t.Fatalf("blank args = %q, want empty", got)
	}
	if got := continueFeedbackFromArgs([]string{"switch", "to", "(a,", "nil)"}); got != "switch to (a, nil)" {
		t.Fatalf("joined = %q", got)
	}
}

func continueFeedbackTestServer(t *testing.T, hit *bool, body *map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/agent-loop/continue") {
			*hit = true
			_ = json.NewDecoder(r.Body).Decode(body)
			_ = json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
				ParentRunID: "run-1",
				LoopState:   client.AgentLoopState{Status: "running", Round: 4, RoundCap: 5},
			})
			return
		}
		http.NotFound(w, r)
	}))
}

func continueBlockedModel(pk, url string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, url)
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}
	return m
}

// /continue <text> on a parked flow POSTs the text as feedback.
func TestContinueWithTextPostsFeedback(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var hit bool
			var body map[string]string
			srv := continueFeedbackTestServer(t, &hit, &body)
			defer srv.Close()

			m := continueBlockedModel(pk, srv.URL)
			m.inputValue = "/continue switch to (a, nil) mirroring MaxChecked"
			_, cmd := m.handleSlashCommand("/continue switch to (a, nil) mirroring MaxChecked")
			if cmd == nil {
				t.Fatalf("%s: /continue with text must return a cmd", pk)
			}
			if msg, ok := cmd().(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: cmd returned %T, want AgentGraphHydratedMsg", pk, msg)
			}
			if !hit {
				t.Fatalf("%s: did not POST continue", pk)
			}
			if body["feedback"] != "switch to (a, nil) mirroring MaxChecked" {
				t.Fatalf("%s: feedback = %q", pk, body["feedback"])
			}
		})
	}
}

// Bare /continue keeps the legacy hardcoded body.
func TestContinueBareKeepsLegacyBody(t *testing.T) {
	var hit bool
	var body map[string]string
	srv := continueFeedbackTestServer(t, &hit, &body)
	defer srv.Close()

	m := continueBlockedModel("codex", srv.URL)
	_, cmd := m.handleSlashCommand("/continue")
	if cmd == nil {
		t.Fatal("bare /continue must return a cmd on a parked flow")
	}
	msg := cmd()
	if _, ok := msg.(AgentGraphHydratedMsg); !ok {
		t.Fatalf("cmd returned %T, want AgentGraphHydratedMsg", msg)
	}
	if body["feedback"] != "continue" {
		t.Fatalf("bare /continue feedback = %q, want legacy \"continue\"", body["feedback"])
	}
}

// Plain text while parked IS the feedback — no /continue prefix needed
// (live request run-577686; a chat turn would just 409 server-side).
func TestParkedPlainTextSendsFeedback(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var hit bool
			var body map[string]string
			srv := continueFeedbackTestServer(t, &hit, &body)
			defer srv.Close()

			m := continueBlockedModel(pk, srv.URL)
			_, cmd := m.processInput("switch the tie rule to (a, nil)")
			if cmd == nil {
				t.Fatalf("%s: parked plain text must return a cmd", pk)
			}
			if msg, ok := cmd().(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: cmd returned %T, want AgentGraphHydratedMsg", pk, msg)
			}
			if !hit {
				t.Fatalf("%s: did not POST continue", pk)
			}
			if body["feedback"] != "switch the tie rule to (a, nil)" {
				t.Fatalf("%s: feedback = %q", pk, body["feedback"])
			}
			// Echoed as a user message like any send.
			last := m.messages[len(m.messages)-1]
			if last.Content != "switch the tie rule to (a, nil)" {
				t.Fatalf("%s: feedback not echoed in transcript: %q", pk, last.Content)
			}
		})
	}
}
