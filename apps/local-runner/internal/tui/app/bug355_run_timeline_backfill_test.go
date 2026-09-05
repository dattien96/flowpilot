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

// TestCmdBackfillRunTimeline_FetchesRunTimeline is the BUG-355 F2 TUI read
// path: /open of a chat-less workflow run fetches the run-scoped timeline
// (not the chat timeline) and the Update renders its turns.
func TestCmdBackfillRunTimeline_FetchesRunTimeline(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chatId": "", "legs": []any{}, "records": []any{
				map[string]any{"chatId": "run-wf", "chatSeq": 1, "legRunId": "run-wf", "type": "turn_started", "payload": map[string]any{"prompt": "do the thing"}},
				map[string]any{"chatId": "run-wf", "chatSeq": 2, "legRunId": "run-wf", "type": "message_completed", "payload": map[string]any{"text": "did the thing"}},
			}, "nextSeq": 2, "truncated": false,
		})
	}))
	defer srv.Close()
	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdBackfillRunTimeline(client.RunHandle{RunID: "run-wf", RunKind: "workflow", ChatID: ""})
	if cmd == nil {
		t.Fatal("expected cmd for chat-less workflow handle")
	}
	msg := cmd()
	bm, ok := msg.(chatTimelineBackfillMsg)
	if !ok || bm.Err != nil {
		t.Fatalf("backfill msg err: %+v", msg)
	}
	if !bm.RunScoped {
		t.Fatal("run fetch must mark RunScoped so the renderer keeps own-leg records")
	}
	if gotPath != "/client/workflow-runs/run-wf/timeline" {
		t.Fatalf("path=%q want /client/workflow-runs/run-wf/timeline", gotPath)
	}
	m2, _ := m.Update(bm)
	am := m2.(*AppModel)
	if len(am.messages) != 2 || am.messages[0].Content != "do the thing" || am.messages[1].Content != "did the thing" {
		t.Fatalf("run records must render as the transcript: %+v", am.messages)
	}
}

// TestCmdBackfillRunTimeline_SkipsChatHandles pins the BUG-338 separation:
// handles WITH a chat id stay on the chat backfill cmd, never the run one.
func TestCmdBackfillRunTimeline_SkipsChatHandles(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	if cmd := m.cmdBackfillRunTimeline(client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}); cmd != nil {
		t.Fatal("chat handles must stay on cmdBackfillChatTimeline")
	}
	if cmd := m.cmdBackfillRunTimeline(client.RunHandle{}); cmd != nil {
		t.Fatal("empty handle must yield nil")
	}
}

// TestRenderRunScopedBackfill_SkipsReplayCoveredTurns pins the BUG-355 F2
// overlap guard: records whose eseq sits above the open replay cursor were
// already rendered from the tail replay — backfill covers only the rest, so
// the last turn never doubles.
func TestRenderRunScopedBackfill_SkipsReplayCoveredTurns(t *testing.T) {
	payload := func(v map[string]any) json.RawMessage { b, _ := json.Marshal(v); return b }
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.historyLoadedAfterSeq = 2 // open replay rendered events above seq 2
	m2, _ := m.Update(chatTimelineBackfillMsg{
		Current: "run-wf", RunScoped: true,
		Records: []client.ChatTranscriptRecord{
			{ChatID: "run-wf", ChatSeq: 1, LegRunID: "run-wf", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "old turn", "eseq": 1})},
			{ChatID: "run-wf", ChatSeq: 2, LegRunID: "run-wf", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "old reply", "eseq": 2})},
			{ChatID: "run-wf", ChatSeq: 3, LegRunID: "run-wf", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "new turn", "eseq": 3})},
			{ChatID: "run-wf", ChatSeq: 4, LegRunID: "run-wf", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "new reply", "eseq": 4})},
		},
	})
	am := m2.(*AppModel)
	if len(am.messages) != 2 || am.messages[0].Content != "old turn" || am.messages[1].Content != "old reply" {
		t.Fatalf("only pre-cursor turns must render: %+v", am.messages)
	}
	// Records rendered → Load-earlier cursor drops (nothing older left to page).
	if am.historyLoadedAfterSeq != 0 {
		t.Fatalf("cursor must reset after full backfill, got %d", am.historyLoadedAfterSeq)
	}
}

// run with no persisted transcript (e.g. ran before run-scoped capture)
// gets an explicit note instead of a silently blank transcript.
func TestRenderRunScopedBackfill_EmptyShowsNote(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m2, _ := m.Update(chatTimelineBackfillMsg{Current: "run-wf", Records: nil, RunScoped: true})
	am := m2.(*AppModel)
	found := false
	for _, mm := range am.messages {
		if strings.Contains(mm.Content, "No saved transcript for this run yet") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected no-transcript note, got %+v", am.messages)
	}

	// Chat path keeps its old silent-empty behavior.
	c := New(config.ChatConfig{}, "http://127.0.0.1:1")
	c2, _ := c.Update(chatTimelineBackfillMsg{Current: "run-1", Records: nil})
	if len(c2.(*AppModel).messages) != 0 {
		t.Fatalf("chat empty backfill must stay silent: %+v", c2.(*AppModel).messages)
	}
}
