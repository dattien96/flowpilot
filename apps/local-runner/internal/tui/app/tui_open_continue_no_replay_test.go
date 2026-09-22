package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestChatOpenedMsg_SeedsClientCursor is the CA-520 anchor: opening a completed
// workflow run whose resume handle carries LastEventSeq=50 must seed the
// per-run client SSE cursor so a later continue turn streams from 50, not 0.
// Without the seed, the old turn's events (empty ProviderTurnID — typical for
// seeded/Grok JSONL) pass the turnId filter and are re-rendered as the new
// assistant bubble (user report: "nó reply lại toàn bộ response của lần trước").
func TestChatOpenedMsg_SeedsClientCursor(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var gotAfter atomic.Value
			streamDone := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
					json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-2"})
				case strings.Contains(r.URL.Path, "/events/stream"):
					gotAfter.Store(r.URL.Query().Get("afterSeq"))
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					flusher, _ := w.(http.Flusher)
					fmt.Fprintf(w, "data: [DONE]\n\n")
					if flusher != nil {
						flusher.Flush()
					}
					close(streamDone)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle: client.RunHandle{
					RunID: "run-193749", RunKind: "workflow",
					WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
					ProviderKey: pk, LastEventSeq: 50,
				},
				Snapshot:    client.RunSnapshot{RunID: "run-193749", Status: "completed"},
				HistoryMeta: client.RunHistoryItem{RunID: "run-193749", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed"},
				Messages: []ChatMessage{
					{Role: "user", Content: "fix bug 1+1 != 2"},
					{Role: "assistant", Content: "old response"},
				},
			})
			am := m2.(*AppModel)

			cmd := am.openTurnStream("continue the fix")
			if cmd == nil {
				t.Fatal("openTurnStream returned nil cmd")
			}
			openedMsg := cmd()
			if _, ok := openedMsg.(turnStreamOpenedMsg); !ok {
				t.Fatalf("msg type %T", openedMsg)
			}

			// The app's own client opened the stream; the server must have seen
			// afterSeq=50 (the resume snapshot), proving continue does not replay
			// the old turn from 0.
			select {
			case <-streamDone:
			case <-time.After(3 * time.Second):
				t.Fatal("no SSE stream request captured")
			}
			if got := gotAfter.Load(); got != "50" {
				t.Fatalf("%s: afterSeq=%v want 50 (continue must not replay from 0)", pk, got)
			}
		})
	}
}

// TestOpenContinue_DoesNotReplayOldTurn drives the full /open → continue path
// against a fake runner that filters events by afterSeq (real runner contract).
// Old turn events (seq <= 50, empty ProviderTurnID) must NOT be re-rendered as a
// fresh assistant bubble; only the new turn's deltas render.
func TestOpenContinue_DoesNotReplayOldTurn(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var afterSeq atomic.Value
			streamDone := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
					json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-2"})
					return
				case strings.Contains(r.URL.Path, "/events/stream"):
					after := r.URL.Query().Get("afterSeq")
					afterSeq.Store(after)
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					flusher, _ := w.(http.Flusher)
					all := []client.ProviderEvent{
						{ID: "e40", Seq: 40, Type: "message_delta", Text: "OLD RESPONSE"},
						{ID: "e50", Seq: 50, Type: "turn_completed", FinalMessage: "OLD RESPONSE"},
						{ID: "e51", Seq: 51, Type: "turn_started", Prompt: "continue the fix"},
						{ID: "e52", Seq: 52, Type: "message_delta", ProviderTurnID: "turn-2", Text: "NEW ANSWER"},
						{ID: "e53", Seq: 53, Type: "turn_completed", ProviderTurnID: "turn-2", FinalMessage: "NEW ANSWER"},
					}
					for _, ev := range all {
						if ev.Seq <= 50 && after == "50" {
							continue
						}
						data, _ := json.Marshal(ev)
						fmt.Fprintf(w, "data: %s\n\n", data)
					}
					if flusher != nil {
						flusher.Flush()
					}
					close(streamDone)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle: client.RunHandle{
					RunID: "run-193749", RunKind: "workflow",
					WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
					ProviderKey: pk, LastEventSeq: 50,
				},
				Snapshot:    client.RunSnapshot{RunID: "run-193749", Status: "completed"},
				HistoryMeta: client.RunHistoryItem{RunID: "run-193749", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed"},
				Messages: []ChatMessage{
					{Role: "user", Content: "fix bug 1+1 != 2"},
					{Role: "assistant", Content: "old response"},
				},
			})
			am := m2.(*AppModel)

			cmd := am.openTurnStream("continue the fix")
			if cmd == nil {
				t.Fatal("openTurnStream returned nil cmd")
			}
			openedMsg := cmd()
			opened, ok := openedMsg.(turnStreamOpenedMsg)
			if !ok {
				t.Fatalf("msg type %T", openedMsg)
			}

			// Drain the turn stream, applying each event through the model.
			// SendTurn closes evCh before errCh, so drain EvCh fully first.
			for ev := range opened.EvCh {
				am3, _ := am.Update(turnStreamEventMsg{Ev: ev})
				am = am3.(*AppModel)
			}
			if err := <-opened.ErrCh; err != nil {
				t.Fatalf("stream error: %v", err)
			}

			if got := afterSeq.Load(); got != "50" {
				t.Fatalf("%s: afterSeq=%v want 50", pk, got)
			}

			// The old response must never be re-appended; only the new answer is a
			// fresh assistant bubble.
			var assistantTexts []string
			for _, mm := range am.messages {
				if mm.Role == "assistant" {
					assistantTexts = append(assistantTexts, mm.Content)
				}
			}
			joined := strings.Join(assistantTexts, "\n")
			if strings.Contains(joined, "OLD RESPONSE") {
				t.Fatalf("%s: old turn replayed into transcript:\n%q", pk, joined)
			}
			if !strings.Contains(joined, "NEW ANSWER") {
				t.Fatalf("%s: new turn answer missing from transcript:\n%q", pk, joined)
			}
		})
	}
}

// TestChatOpenedMsg_NoLastSeqLeavesCursorAtZero locks the no-regression side: a
// /open whose handle carries no LastEventSeq must not invent one (the client
// cursor stays at the pre-existing watermark so nothing is skipped).
func TestChatOpenedMsg_NoLastSeqLeavesCursorAtZero(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-193749", RunKind: "workflow",
			WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
			ProviderKey: "codex",
		},
		Snapshot:    client.RunSnapshot{RunID: "run-193749", Status: "completed"},
		HistoryMeta: client.RunHistoryItem{RunID: "run-193749", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed"},
	})
	am := m2.(*AppModel)
	if am.lastEventSeq != 0 {
		t.Fatalf("lastEventSeq=%d want 0 when handle carries none", am.lastEventSeq)
	}
}
