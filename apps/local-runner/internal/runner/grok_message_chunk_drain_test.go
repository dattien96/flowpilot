package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// run-96217: many agent_message_chunk frames + session/prompt reply must all
// reach the bridge (not be dropped when the RPC returns early vs notif queue).
func TestGrokSendTurn_DrainsMessageChunksAfterPromptReturns(t *testing.T) {
	sessionID := "session-drain-chunks"
	const nChunks = 400
	frames := make([]map[string]any, 0, nChunks)
	var want strings.Builder
	for i := 0; i < nChunks; i++ {
		piece := fmt.Sprintf("w%d ", i)
		want.WriteString(piece)
		frames = append(frames, map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": piece},
			},
		})
	}

	// Delay the RPC reply slightly so chunks are enqueued first, then complete.
	a, _ := newTestGrokAdapter(t, sessionID, liveGrokPromptResult(sessionID), frames, 30*time.Millisecond)
	bridge := &fakeGrokBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-drain", Prompt: "describe images"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	var gotDelta strings.Builder
	var final string
	bridge.mu.Lock()
	evs := append([]ProviderEvent(nil), bridge.events...)
	bridge.mu.Unlock()
	for _, ev := range evs {
		switch ev.Type {
		case EventMessageDelta:
			gotDelta.WriteString(ev.Text)
		case EventTurnCompleted:
			final = ev.FinalMessage
		}
	}
	if gotDelta.Len() < want.Len()/2 {
		t.Fatalf("too few deltas: got %d runes want ~%d (dropped chunks?)", gotDelta.Len(), want.Len())
	}
	if !strings.Contains(gotDelta.String(), "w0 ") || !strings.Contains(gotDelta.String(), fmt.Sprintf("w%d ", nChunks-1)) {
		t.Fatalf("missing first/last chunk in deltas (len=%d)", gotDelta.Len())
	}
	if !strings.Contains(final, "w0 ") {
		t.Fatalf("FinalMessage missing streamed text: %q", trunc(final, 120))
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
