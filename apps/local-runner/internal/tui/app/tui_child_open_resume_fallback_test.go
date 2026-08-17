package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestCmdFocusAgent_ResumeSuccessSeedsTranscript guards the CA-504 path: a
// working /resume seeds durable transcript (turn log / Grok JSONL) into the
// runner event buffer, then StreamLive tails from lastEventSeq.
func TestCmdFocusAgent_ResumeSuccessSeedsTranscript(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
					json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-child", LastEventSeq: 3, Status: "completed"})
				case strings.Contains(r.URL.Path, "/events/stream"):
					w.Header().Set("Content-Type", "text/event-stream")
					flusher, _ := w.(http.Flusher)
					for _, ev := range makeTurnEvents(1, "fix bug 1+1 != 2") {
						data, _ := json.Marshal(ev)
						fmt.Fprintf(w, "data: %s\n\n", data)
					}
					if flusher != nil {
						flusher.Flush()
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-parent"}
			msg := m.cmdFocusAgent("run-child")()
			opened, ok := msg.(focusStreamOpenedMsg)
			if !ok {
				t.Fatalf("msg type %T", msg)
			}
			if opened.Err != "" {
				t.Fatalf("resume success must not Err: %s", opened.Err)
			}
			if opened.Fallback != "" {
				t.Fatalf("resume success must not fall back: %s", opened.Fallback)
			}
			if opened.AfterSeq != 3 {
				t.Fatalf("AfterSeq=%d want 3", opened.AfterSeq)
			}
			if len(opened.Messages) < 2 {
				t.Fatalf("resume success must seed history, got %+v", opened.Messages)
			}
			if opened.Messages[0].Role != "user" || !strings.Contains(opened.Messages[0].Content, "fix bug") {
				t.Fatalf("user msg=%+v", opened.Messages[0])
			}
			if opened.EvCh == nil {
				t.Fatal("resume success must attach a live stream")
			}
			if opened.Cancel != nil {
				opened.Cancel()
			}
		})
	}
}

// TestCmdFocusAgent_ResumeFailFallsBackToLiveStream is the run-193749
// regression: /resume fails but the child is still live in the runner's memory,
// so the pre-CA-504 StreamLive path must still render its transcript instead of
// hard-failing the child open.
func TestCmdFocusAgent_ResumeFailFallsBackToLiveStream(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
					http.Error(w, `{"error":{"code":"run_not_found","message":"workflow run not found"}}`, http.StatusNotFound)
				case strings.Contains(r.URL.Path, "/events/stream"):
					w.Header().Set("Content-Type", "text/event-stream")
					flusher, _ := w.(http.Flusher)
					for _, ev := range makeTurnEvents(1, "fix bug 1+1 != 2") {
						data, _ := json.Marshal(ev)
						fmt.Fprintf(w, "data: %s\n\n", data)
					}
					if flusher != nil {
						flusher.Flush()
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-parent"}
			msg := m.cmdFocusAgent("run-child")()
			opened, ok := msg.(focusStreamOpenedMsg)
			if !ok {
				t.Fatalf("msg type %T", msg)
			}
			if opened.Err != "" {
				t.Fatalf("resume fail must NOT hard-Err when live stream can serve: %s", opened.Err)
			}
			if opened.Fallback == "" {
				t.Fatal("resume fail must surface a fallback note")
			}
			if opened.EvCh == nil {
				t.Fatal("resume fail must still attach the live stream")
			}
			if opened.Cancel != nil {
				opened.Cancel()
			}
		})
	}
}

// TestCmdFocusAgent_ResumeRetriedOnceOnDialError guards the transient-dial
// retry: a first connection-refused resume is retried once, and a healthy
// second attempt seeds the transcript (CA-517 resilience against supervisor
// restart / port handoff).
func TestCmdFocusAgent_ResumeRetriedOnceOnDialError(t *testing.T) {
	var resumeHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
			resumeHits++
			if resumeHits == 1 {
				resetConn(t, w)
				return
			}
			json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-child", LastEventSeq: 3, Status: "completed"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, ev := range makeTurnEvents(1, "fix bug 1+1 != 2") {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			if flusher != nil {
				flusher.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-parent"}
	msg := m.cmdFocusAgent("run-child")()
	opened, ok := msg.(focusStreamOpenedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if resumeHits < 2 {
		t.Fatalf("resume must be retried after dial failure, got %d hits", resumeHits)
	}
	if opened.Err != "" {
		t.Fatalf("retried resume must succeed: %s", opened.Err)
	}
	if len(opened.Messages) < 2 {
		t.Fatalf("retried resume must seed history, got %+v", opened.Messages)
	}
	if opened.Cancel != nil {
		opened.Cancel()
	}
}

// TestCmdFocusAgent_TotalFailureNoStuckChrome drives the full handler path when
// resume fails at the dial level (runner unreachable) AND the live stream cannot
// serve the child: the main transcript must be restored, not a stuck empty
// child chrome (CA-517).
func TestCmdFocusAgent_TotalFailureNoStuckChrome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resetConn(t, w)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-parent"}
	m.messages = []ChatMessage{
		{Role: "user", Content: "main question"},
		{Role: "assistant", Content: "main answer"},
	}

	// Drive cmdFocusAgent as Update does: it saves mainTranscript, clears
	// messages, sets focusRunID, then the async msg resolves.
	msg := m.cmdFocusAgent("run-child")()
	opened, ok := msg.(focusStreamOpenedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if opened.Err == "" {
		t.Fatalf("dial-level failure must carry Err, got Fallback=%q", opened.Fallback)
	}
	if opened.Cancel != nil {
		opened.Cancel()
	}

	// Focus is now set (Update would have processed cmdFocusAgent sync part).
	m.focusRunID = "run-child"
	m.messages = nil
	m2, _ := m.Update(opened)
	am := m2.(*AppModel)
	if am.focusRunID != "" {
		t.Fatalf("focusRunID=%q want restored to main", am.focusRunID)
	}
	joined := ""
	for _, msg := range am.messages {
		joined += msg.Content + "\n"
	}
	if !strings.Contains(joined, "main question") || !strings.Contains(joined, "main answer") {
		t.Fatalf("main transcript must be restored on total failure:\n%s", joined)
	}
	if !strings.Contains(joined, "Open child transcript failed") {
		t.Fatalf("error banner missing:\n%s", joined)
	}
}

// resetConn hijacks the response and RST-closes the underlying TCP connection,
// simulating the runner process being unreachable (dial/reset failure) so the
// TUI's runnerDialDeadErr classifier trips on a real transport error.
func resetConn(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetLinger(0) // force RST instead of FIN
	}
	_ = conn.Close()
}

// TestFocusStreamOpenedMsg_ErrRestoresMainTranscript locks the handler
// contract directly: an Err must restore the saved main transcript and clear
// focus, never leave an empty child view.
func TestFocusStreamOpenedMsg_ErrRestoresMainTranscript(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-parent"}
	m.focusRunID = "run-child"
	m.mainTranscript = []ChatMessage{
		{Role: "user", Content: "main question"},
		{Role: "assistant", Content: "main answer"},
	}
	m.messages = nil
	m2, _ := m.Update(focusStreamOpenedMsg{RunID: "run-child", Err: "dial tcp 127.0.0.1:4317: connection refused"})
	am := m2.(*AppModel)
	if am.focusRunID != "" {
		t.Fatalf("focusRunID=%q want cleared on Err", am.focusRunID)
	}
	joined := ""
	for _, msg := range am.messages {
		joined += msg.Content + "\n"
	}
	if !strings.Contains(joined, "main question") || !strings.Contains(joined, "main answer") {
		t.Fatalf("main transcript must be restored:\n%s", joined)
	}
	if !strings.Contains(joined, "Open child transcript failed") {
		t.Fatalf("error banner missing:\n%s", joined)
	}
}

// TestFocusStreamOpenedMsg_FallbackNoteShowsOnEmptyFallback locks the soft
// fallback: when resume fails and the fallback live stream yields no events,
// the chrome shows the fallback note instead of a bare "(no transcript events)".
func TestFocusStreamOpenedMsg_FallbackNoteShowsOnEmptyFallback(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-parent"}
	m.focusRunID = "run-child"
	m.mainTranscript = []ChatMessage{{Role: "user", Content: "main question"}}
	m.messages = nil
	m2, _ := m.Update(focusStreamOpenedMsg{
		RunID:    "run-child",
		Fallback: "child resume failed (dial tcp connection refused); showing live events",
	})
	am := m2.(*AppModel)
	joined := ""
	for _, msg := range am.messages {
		joined += msg.Content + "\n"
	}
	if !strings.Contains(joined, "child resume failed") {
		t.Fatalf("fallback note missing:\n%s", joined)
	}
}

// TestResumeChildRunForFocus_RetriesOnlyDialErrors verifies the retry helper
// classifies errors: a dial error is retried once, a non-dial error (404) is
// returned immediately without a second attempt.
func TestResumeChildRunForFocus_RetriesOnlyDialErrors(t *testing.T) {
	t.Run("non-dial error returns immediately", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			http.Error(w, `{"error":{"code":"run_not_found","message":"workflow run not found"}}`, http.StatusNotFound)
		}))
		defer srv.Close()
		_, err := resumeChildRunForFocus(client.New(srv.URL), "run-child")
		if err == nil {
			t.Fatal("expected error")
		}
		if hits != 1 {
			t.Fatalf("non-dial error must not retry, got %d hits", hits)
		}
	})
}