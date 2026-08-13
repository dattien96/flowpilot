package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestChatReplayTailAfterSeq(t *testing.T) {
	if got := chatReplayTailAfterSeq(100); got != 0 {
		t.Fatalf("small run afterSeq=%d want 0", got)
	}
	if got := chatReplayTailAfterSeq(500); got != 100 {
		t.Fatalf("500 events afterSeq=%d want 100", got)
	}
	if got := chatReplayTailAfterSeq(1000); got != 600 {
		t.Fatalf("1000 events afterSeq=%d want 600", got)
	}
}

func TestChatReplayChunkBefore(t *testing.T) {
	if got := chatReplayChunkBefore(300); got != 0 {
		t.Fatalf("floor 300 chunkBefore=%d want 0", got)
	}
	if got := chatReplayChunkBefore(900); got != 500 {
		t.Fatalf("floor 900 chunkBefore=%d want 500", got)
	}
}

func TestTrimEventsFromTurnStart(t *testing.T) {
	evs := []client.ProviderEvent{
		{Seq: 1, Type: "message_delta", Text: "orphan"},
		{Seq: 2, Type: "turn_started", Prompt: "hi"},
		{Seq: 3, Type: "message_delta", Text: "hello"},
	}
	got := trimEventsFromTurnStart(evs)
	if len(got) != 2 || got[0].Type != "turn_started" {
		t.Fatalf("trimmed=%+v", got)
	}
}

func TestPrependReplayMessages(t *testing.T) {
	older := []ChatMessage{{Role: "user", Content: "old"}}
	existing := []ChatMessage{{Role: "user", Content: "new"}}
	got := prependReplayMessages(older, existing)
	if len(got) != 2 || got[0].Content != "old" || got[1].Content != "new" {
		t.Fatalf("prepend=%+v", got)
	}
}

func makeTurnEvents(seqBase int64, prompt string) []client.ProviderEvent {
	return []client.ProviderEvent{
		{Seq: seqBase, Type: "turn_started", Prompt: prompt},
		{Seq: seqBase + 1, Type: "message_delta", Text: "answer-" + prompt},
		{Seq: seqBase + 2, Type: "turn_completed", FinalMessage: "answer-" + prompt},
	}
}

func TestCmdOpenChat_UsesTailAfterSeq(t *testing.T) {
	var gotAfterSeq int64 = -1
	const lastSeq int64 = 1000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
			json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-tail", LastEventSeq: lastSeq})
		case strings.Contains(r.URL.Path, "/events/stream"):
			var err error
			gotAfterSeq, err = strconv.ParseInt(r.URL.Query().Get("afterSeq"), 10, 64)
			if err != nil {
				t.Errorf("afterSeq parse: %v", err)
			}
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
	msg := m.cmdOpenChat("run-tail")()
	opened, ok := msg.(ChatOpenedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	wantAfter := chatReplayTailAfterSeq(lastSeq)
	if gotAfterSeq != wantAfter {
		t.Fatalf("stream afterSeq=%d want %d", gotAfterSeq, wantAfter)
	}
	if opened.HistoryLoadedAfterSeq != wantAfter {
		t.Fatalf("HistoryLoadedAfterSeq=%d want %d", opened.HistoryLoadedAfterSeq, wantAfter)
	}
	if len(opened.Messages) != 1 || opened.Messages[0].Content != "tail-prompt" {
		t.Fatalf("messages=%+v", opened.Messages)
	}
}

func TestLoadEarlier_FetchesOlderChunkWhenMemoryExhausted(t *testing.T) {
	const floor int64 = 600
	var fetchAfter int64 = -1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		var err error
		fetchAfter, err = strconv.ParseInt(r.URL.Query().Get("afterSeq"), 10, 64)
		if err != nil {
			t.Errorf("afterSeq parse: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, ev := range makeTurnEvents(501, "older-prompt") {
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
	m.runHandle = &client.RunHandle{RunID: "run-chunk"}
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = floor
	m.visiblePromptCount = 6

	cmd := m.loadEarlierPrompts()
	if cmd == nil {
		t.Fatal("expected fetch cmd when memory window exhausted and more on server")
	}
	chunk := cmd().(HistoryChunkMsg)
	if chunk.NewLoadedAfterSeq != chatReplayChunkBefore(floor) {
		t.Fatalf("NewLoadedAfterSeq=%d want %d", chunk.NewLoadedAfterSeq, chatReplayChunkBefore(floor))
	}
	if fetchAfter != chatReplayChunkBefore(floor) {
		t.Fatalf("fetch afterSeq=%d want %d", fetchAfter, chatReplayChunkBefore(floor))
	}
	if len(chunk.Messages) != 2 || chunk.Messages[0].Content != "older-prompt" {
		t.Fatalf("chunk messages=%+v", chunk.Messages)
	}

	m2, _ := m.Update(chunk)
	am := m2.(*AppModel)
	if am.historyLoadedAfterSeq != chatReplayChunkBefore(floor) {
		t.Fatalf("historyLoadedAfterSeq=%d", am.historyLoadedAfterSeq)
	}
	if !strings.Contains(am.messages[0].Content, "older-prompt") {
		t.Fatalf("prepended messages=%+v", am.messages)
	}
}

func TestCollectReplayEvents_StopsAtUntilSeq(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := int64(1); i <= 5; i++ {
			ev := client.ProviderEvent{Seq: i, Type: "message_delta", Text: fmt.Sprintf("e%d", i)}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cl := client.New(srv.URL)
	got := collectReplayEvents(ctx, cl, "run-1", 0, 3, 100)
	if len(got) != 3 || got[2].Seq != 3 {
		t.Fatalf("collected=%+v", got)
	}
}

func TestChatWindow_ShowsLoadMoreWhenTailOnly(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.messages = makePromptHistory(4)
	m.historyLoadedAfterSeq = 500
	m.syncVisiblePromptCount()

	rows := m.chatRows()
	if len(rows) == 0 || !rows[0].LoadEarlier {
		t.Fatal("expected load-earlier row when more history on server")
	}
	if !strings.Contains(stripANSI(rows[0].Text), "Load earlier prompts (more)") {
		t.Fatalf("label=%q", stripANSI(rows[0].Text))
	}
}

func TestHistoryCursorAfterReplay_RewindsToDroppedPrefix(t *testing.T) {
	collected := []client.ProviderEvent{
		{Seq: 601, Type: "message_delta", Text: "orphan"},
		{Seq: 620, Type: "turn_started", Prompt: "next"},
	}
	trimmed := trimEventsFromTurnStart(collected)
	got := historyCursorAfterReplay(600, collected, trimmed)
	if got != 619 {
		t.Fatalf("cursor=%d want 619 so next chunk re-fetches seq 601-619", got)
	}
}

func TestCmdOpenChat_ReturnsWhenLastEventSeqReachedOnLiveStream(t *testing.T) {
	const lastSeq int64 = 3
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume"):
			json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-live", LastEventSeq: lastSeq})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, ev := range []client.ProviderEvent{
				{Seq: 1, Type: "turn_started", Prompt: "p1"},
				{Seq: 2, Type: "message_delta", Text: "a1"},
				{Seq: 3, Type: "turn_completed", FinalMessage: "a1"},
			} {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	done := make(chan any, 1)
	go func() { done <- m.cmdOpenChat("run-live")() }()
	select {
	case msg := <-done:
		opened, ok := msg.(ChatOpenedMsg)
		if !ok {
			t.Fatalf("msg type %T", msg)
		}
		if opened.Err != "" {
			t.Fatalf("err=%q", opened.Err)
		}
		if len(opened.Messages) < 1 || opened.Messages[0].Content != "p1" {
			t.Fatalf("messages=%+v", opened.Messages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cmdOpenChat hung waiting on idle SSE after lastEventSeq")
	}
}

func TestLoadEarlier_SecondClickDoesNotFetchWhileInFlight(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.runHandle = &client.RunHandle{RunID: "run-chunk"}
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = 600
	m.visiblePromptCount = 6

	cmd1 := m.loadEarlierPrompts()
	if cmd1 == nil {
		t.Fatal("first click should fetch")
	}
	if !m.historyChunkInFlight {
		t.Fatal("expected in-flight after first click")
	}
	cmd2 := m.loadEarlierPrompts()
	if cmd2 != nil {
		t.Fatal("second click must not start another fetch")
	}
}

func TestHistoryChunkMsg_IgnoresWrongRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-b"}
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = 600

	m2, _ := m.Update(HistoryChunkMsg{
		RunID:             "run-a",
		Messages:          []ChatMessage{{Role: "user", Content: "from-a"}},
		NewLoadedAfterSeq: 200,
	})
	am := m2.(*AppModel)
	if am.historyLoadedAfterSeq != 600 {
		t.Fatalf("cursor moved=%d", am.historyLoadedAfterSeq)
	}
	if strings.Contains(am.messages[0].Content, "from-a") {
		t.Fatalf("wrong-run chunk prepended: %+v", am.messages[0])
	}
}

func TestHistoryChunkMsg_DropsDuplicateCursor(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-dup"}
	m.messages = makePromptHistory(6)
	m.historyLoadedAfterSeq = 200

	older := []ChatMessage{{Role: "user", Content: "dup-prompt"}}
	m2, _ := m.Update(HistoryChunkMsg{
		RunID: "run-dup", Messages: older, NewLoadedAfterSeq: 200,
	})
	am := m2.(*AppModel)
	if strings.Contains(am.messages[0].Content, "dup-prompt") {
		t.Fatal("stale duplicate chunk must not prepend")
	}
}
