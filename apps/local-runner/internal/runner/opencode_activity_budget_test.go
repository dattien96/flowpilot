package runner

import (
	"context"
	"testing"
	"time"
)

// run-437116: after tools the model generated for ~8s emitting only
// usage_update frames, then the answer. CA-708 (no reset on activity) let the
// fixed 8s cap fire mid-generation → empty turn_completed. This locks the new
// behavior: session activity (usage/tool) re-arms a 15s budget so text arriving
// after the initial timeout is still captured.
func TestOpencodeActivityKeepsBudgetPastInitialCap(t *testing.T) {
	ch := make(chan opencodeNotification, 3)
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	ctx := context.Background()
	// Initial budget 2s (mirrors the 8s live value but keeps the test fast).
	done := make(chan string, 1)
	go func() {
		done <- a.drainOpencodeNotificationsBlocking(ctx, "ses_437116", ch, bridge, "", 2*time.Second)
	}()
	usage := func() opencodeNotification {
		return opencodeNotification{
			Method: "session/update",
			Params: map[string]any{
				"update": map[string]any{
					"sessionUpdate": "usage_update",
					"used":          100,
					"size":          8000,
				},
			},
		}
	}
	// Activity at 0.1s (keeps budget alive), activity again at 2.2s (past the
	// initial 2s cap — CA-708 code would already have returned empty here).
	time.Sleep(100 * time.Millisecond)
	ch <- usage()
	time.Sleep(2100 * time.Millisecond)
	ch <- usage()
	// Text arrives at ~2.5s, after the initial cap would have fired.
	time.Sleep(300 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "answer after long generation",
				},
			},
		},
	}
	select {
	case got := <-done:
		if got != "answer after long generation" {
			t.Fatalf("got %q want answer after long generation", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("drain hung or returned empty — activity budget did not extend the wait")
	}
	found := false
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && ev.Text == "answer after long generation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bridge missing late delta, events=%+v", bridge.events)
	}
}
