package runner

import (
	"context"
	"testing"
	"time"
)

// These tests reproduce the 421135/424302/430742 blank: a non-text frame
// arrives after session/prompt result and must NOT collapse the 8s wait.

// usage_update between result and the final text must not starve.
func TestOpencodeUsageUpdateDoesNotStarveText(t *testing.T) {
	ch := make(chan opencodeNotification, 2)
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	ctx := context.Background()
	// Start the blocking drain with empty lastText (the CA-707 8s path).
	done := make(chan string, 1)
	go func() {
		done <- a.drainOpencodeNotificationsBlocking(ctx, "ses_starve_usage", ch, bridge, "", 8*time.Second)
	}()
	// Non-text frame at 10ms (the documented live order: result then usage_update).
	time.Sleep(10 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "usage_update",
				"used":          100,
				"size":          8000,
			},
		},
	}
	// Text at 400ms — would be missed if usage_update reset to 150ms.
	time.Sleep(390 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "Chao Nam",
				},
			},
		},
	}
	select {
	case got := <-done:
		if got != "Chao Nam" {
			t.Fatalf("got %q want Chao Nam", got)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("drain timed out — usage_update starved the wait")
	}
	found := false
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && ev.Text == "Chao Nam" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bridge missing delta, events=%+v", bridge.events)
	}
}

func TestOpencodeToolUpdateDoesNotStarveText(t *testing.T) {
	ch := make(chan opencodeNotification, 2)
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	ctx := context.Background()
	done := make(chan string, 1)
	go func() {
		done <- a.drainOpencodeNotificationsBlocking(ctx, "ses_starve_tool", ch, bridge, "", 8*time.Second)
	}()
	time.Sleep(10 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"status":        "completed",
				"title":         "read_file",
			},
		},
	}
	time.Sleep(390 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "tool then text",
				},
			},
		},
	}
	select {
	case got := <-done:
		if got != "tool then text" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out")
	}
}

func TestOpencodeThoughtChunkDoesNotStarveText(t *testing.T) {
	ch := make(chan opencodeNotification, 2)
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	ctx := context.Background()
	done := make(chan string, 1)
	go func() {
		done <- a.drainOpencodeNotificationsBlocking(ctx, "ses_starve_thought", ch, bridge, "", 8*time.Second)
	}()
	time.Sleep(10 * time.Millisecond)
	// agent_thought_chunk is unmapped → apply returns same lastText
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_thought_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "thinking hidden",
				},
			},
		},
	}
	time.Sleep(390 * time.Millisecond)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "after thought",
				},
			},
		},
	}
	select {
	case got := <-done:
		if got != "after thought" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out")
	}
}
