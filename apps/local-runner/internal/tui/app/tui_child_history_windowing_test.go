package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// D12: opening a sub-agent transcript must lazy-load the tail like the main
// chat (chatReplayTailAfterSeq), not replay from seq 0 — long coder/reviewer
// legs otherwise fetch every event and stall the TUI open.

func TestCmdFocusAgent_UsesTailAfterSeq(t *testing.T) {
	var gotAfter []int64
	const lastSeq int64 = 1000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
			json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-child", LastEventSeq: lastSeq})
		case strings.Contains(r.URL.Path, "/events/stream"):
			a, err := strconv.ParseInt(r.URL.Query().Get("afterSeq"), 10, 64)
			if err != nil {
				t.Errorf("afterSeq parse: %v", err)
			}
			gotAfter = append(gotAfter, a)
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			ev := client.ProviderEvent{Seq: 601, Type: "turn_started", Prompt: "tail-prompt"}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	msg := m.cmdFocusAgent("run-child")()
	opened, ok := msg.(focusStreamOpenedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	want := chatReplayTailAfterSeq(lastSeq)
	if len(gotAfter) == 0 || gotAfter[0] != want {
		t.Fatalf("replay afterSeq=%v want first=%d (tail), not seq 0", gotAfter, want)
	}
	if opened.HistoryLoadedAfterSeq != want {
		t.Fatalf("HistoryLoadedAfterSeq=%d want %d — older chunks must stay fetchable", opened.HistoryLoadedAfterSeq, want)
	}
}

func TestFocusLoadEarlier_FetchesChildRunChunk(t *testing.T) {
	var fetchPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		fetchPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, ev := range makeTurnEvents(501, "child-older") {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	m.width, m.height = 100, 30
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.focusRunID = "run-child"
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = 600
	m.visiblePromptCount = 6

	cmd := m.loadEarlierPrompts()
	if cmd == nil {
		t.Fatal("expected fetch cmd for focused child with more history on server")
	}
	chunk := cmd().(HistoryChunkMsg)
	if chunk.RunID != "run-child" {
		t.Fatalf("chunk RunID=%q want run-child — must not fetch parent events into child view", chunk.RunID)
	}
	if !strings.Contains(fetchPath, "run-child") {
		t.Fatalf("fetch path=%q want child run", fetchPath)
	}
}

func TestHistoryChunkMsg_AcceptedWhileViewingChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.focusRunID = "run-child"
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = 600
	m.visiblePromptCount = 6

	m2, _ := m.Update(HistoryChunkMsg{
		RunID:             "run-child",
		Messages:          []ChatMessage{{Role: "user", Content: "child-older"}},
		NewLoadedAfterSeq: 200,
	})
	am := m2.(*AppModel)
	if am.historyLoadedAfterSeq != 200 {
		t.Fatalf("historyLoadedAfterSeq=%d want 200 — child chunk dropped", am.historyLoadedAfterSeq)
	}
	if am.messages[0].Content != "child-older" {
		t.Fatalf("prepend failed: %+v", am.messages[0])
	}
}

func TestHistoryChunkMsg_IgnoresStaleRunWhileViewingChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.focusRunID = "run-child"
	m.messages = makePromptHistory(2)
	m.historyLoadedAfterSeq = 600
	m.visiblePromptCount = 2

	m2, _ := m.Update(HistoryChunkMsg{
		RunID:             "run-main",
		Messages:          []ChatMessage{{Role: "user", Content: "parent-older"}},
		NewLoadedAfterSeq: 200,
	})
	am := m2.(*AppModel)
	if am.historyLoadedAfterSeq != 600 {
		t.Fatalf("parent chunk clobbered child cursor: %d", am.historyLoadedAfterSeq)
	}
	if am.messages[0].Content == "parent-older" {
		t.Fatal("parent chunk must not prepend into child view")
	}
}

// D12 perf: the chat-row cache signature must not hash hidden (pre-window)
// message content every frame — that made sub-agent views O(total bytes)/frame.
func TestChatRowsSig_DoesNotHashHiddenMessages(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.messages = makePromptHistory(12) // 12 user prompts → 6 hidden groups
	m.visiblePromptCount = 6
	before := m.chatRowsSig()

	hidden := m.windowStartIndex() - 1
	if hidden < 0 {
		t.Fatal("expected hidden messages above the window")
	}
	m.messages[hidden].Content += " mutated-but-hidden"
	if m.chatRowsSig() != before {
		t.Fatal("sig changed on hidden-message mutation — still hashing pre-window content")
	}

	m.messages[len(m.messages)-1].Content += " visible-mutation"
	if m.chatRowsSig() == before {
		t.Fatal("sig must change on visible-message mutation")
	}
}
