package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// CA-916: the scaffold progress feed must render like a chat run — streamed
// output deltas land in an assistant message, phase milestones label the busy
// line, and polling rides the thinking ticker while scaffoldBusy.

// newScaffoldProgressServer serves only /scaffold/progress with canned events.
func newScaffoldProgressServer(t *testing.T, snap client.ScaffoldProgressSnapshot) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/scaffold/progress") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(snap)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func lastAssistantText(m *AppModel) string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "assistant" {
			return m.messages[i].Content
		}
	}
	return ""
}

func TestScaffoldProgress_TUIStreamsOutputAsAssistantMessage(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true
	m.addMessage("system", "Starting AI Scaffold turn for App…", "")

	_, _ = m.handleEngineScaffoldProgressMsg(EngineScaffoldProgressMsg{Snapshot: &client.ScaffoldProgressSnapshot{
		Active: true,
		Events: []client.ScaffoldProgressEvent{
			{Seq: 1, Kind: "phase", Phase: "ai_turn", Attempt: 1, Text: "AI scaffold turn started"},
			{Seq: 2, Kind: "output", Phase: "ai_turn", Text: "Writing "},
		},
		NextSeq: 3,
	}})
	_, _ = m.handleEngineScaffoldProgressMsg(EngineScaffoldProgressMsg{Snapshot: &client.ScaffoldProgressSnapshot{
		Active: true,
		Events: []client.ScaffoldProgressEvent{
			{Seq: 3, Kind: "output", Phase: "ai_turn", Text: "src/App.tsx"},
		},
		NextSeq: 4,
	}})

	got := lastAssistantText(m)
	if got != "Writing src/App.tsx" {
		t.Fatalf("assistant stream = %q, want concatenated deltas", got)
	}
	if m.scaffoldProgressSeq != 3 {
		t.Fatalf("progress seq = %d, want 3", m.scaffoldProgressSeq)
	}
	if m.scaffoldPhase != "ai_turn" {
		t.Fatalf("phase = %q, want ai_turn", m.scaffoldPhase)
	}
}

func TestScaffoldProgress_PhaseAppearsOnBusyLine(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true

	_, _ = m.handleEngineScaffoldProgressMsg(EngineScaffoldProgressMsg{Snapshot: &client.ScaffoldProgressSnapshot{
		Active: true,
		Events: []client.ScaffoldProgressEvent{
			{Seq: 1, Kind: "phase", Phase: "gate", Attempt: 1, Text: "compiler gate attempt 1"},
		},
		NextSeq: 2,
	}})

	line := m.renderInputLine()
	if !strings.Contains(line, "gate") {
		t.Fatalf("busy line = %q, want current phase label", line)
	}
	if !strings.Contains(line, "chat disabled") {
		t.Fatalf("busy line = %q, want chat disabled banner retained", line)
	}
}

func TestScaffoldProgress_ThinkingTickSchedulesPollWhileBusy(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true
	m.thinkingFrame = 7 // next tick lands on %8 boundary

	_, cmd := m.Update(thinkingTickMsg{})
	if !m.scaffoldProgressInFlight {
		t.Fatal("tick at poll boundary did not arm a progress fetch")
	}
	if cmd == nil {
		t.Fatal("tick at poll boundary returned no command")
	}
}

func TestScaffoldProgress_NoPollWhenNotBusyOrFetchInFlight(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.thinkingFrame = 7

	// Not busy: no progress fetch even at the boundary.
	_, _ = m.Update(thinkingTickMsg{})
	if m.scaffoldProgressInFlight {
		t.Fatal("progress fetch armed while idle")
	}

	// Busy but a fetch is already in flight: do not stack requests.
	m.scaffoldBusy = true
	m.thinkingFrame = 7
	m.scaffoldProgressInFlight = true
	_, _ = m.Update(thinkingTickMsg{})
	if !m.scaffoldProgressInFlight {
		t.Fatal("in-flight guard cleared without a response")
	}
}

func TestScaffoldProgress_FetchRendersEventsFromServer(t *testing.T) {
	snap := client.ScaffoldProgressSnapshot{
		Active: true,
		Phase:  "ai_turn",
		Events: []client.ScaffoldProgressEvent{
			{Seq: 1, Kind: "phase", Phase: "ai_turn", Attempt: 1},
			{Seq: 2, Kind: "output", Phase: "ai_turn", Text: "generated theme tokens"},
		},
		NextSeq: 3,
	}
	srv := newScaffoldProgressServer(t, snap)
	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	m.scaffoldBusy = true

	fetch := m.cmdScaffoldProgress()
	if fetch == nil {
		t.Fatal("no progress command")
	}
	msg := fetch().(EngineScaffoldProgressMsg)
	if msg.Err != nil {
		t.Fatalf("progress fetch: %v", msg.Err)
	}
	_, _ = m.handleEngineScaffoldProgressMsg(msg)
	if got := lastAssistantText(m); got != "generated theme tokens" {
		t.Fatalf("assistant stream = %q", got)
	}
}

func TestScaffoldProgress_FinalFlushAfterResult(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true
	m.scaffoldProgressInFlight = false

	_, cmd := m.handleEngineScaffoldMsg(EngineScaffoldMsg{
		ProjectID: "p1",
		Result:    &client.ScaffoldResult{Status: "done", Message: "scaffold: done"},
	})
	if cmd == nil {
		t.Fatal("no final flush command after scaffold result")
	}
	if !m.scaffoldProgressInFlight {
		t.Fatal("final flush did not arm the in-flight guard")
	}
	if m.scaffoldBusy {
		t.Fatal("scaffoldBusy still set after terminal result")
	}
}
