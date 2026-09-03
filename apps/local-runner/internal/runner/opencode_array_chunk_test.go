package runner

import "testing"

func TestOpencodeTextContentArray(t *testing.T) {
	// Real opencode can send content as an array of parts.
	n := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": []any{
					map[string]any{"type": "text", "text": "Chao "},
					map[string]any{"type": "text", "text": "Nam!"},
				},
			},
		},
	}
	evs, ok := mapOpencodeNotification(n)
	if !ok || len(evs) == 0 || evs[0].Text != "Chao Nam!" {
		t.Fatalf("array content not captured, ok=%v evs=%+v", ok, evs)
	}
}

func TestOpencodeTextContentOutputText(t *testing.T) {
	n := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "output_text",
					"text": "hello output",
				},
			},
		},
	}
	evs, ok := mapOpencodeNotification(n)
	if !ok || evs[0].Text != "hello output" {
		t.Fatalf("output_text not captured, ok=%v evs=%+v", ok, evs)
	}
}

func TestOpencodeArrayThenTextViaBlockingDrain(t *testing.T) {
	// Simulate tool burst then array chunk as late text (the 433929 shape).
	ch := make(chan opencodeNotification, 2)
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	// Late array chunk
	go func() {
		n := opencodeNotification{
			Method: "session/update",
			Params: map[string]any{
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": []any{
						map[string]any{"type": "text", "text": "array "},
						map[string]any{"type": "text", "text": "answer"},
					},
				},
			},
		}
		ch <- n
	}()
	// Use blocking drain directly
	ctx := t.Context()
	got := a.drainOpencodeNotificationsBlocking(ctx, "ses_arr", ch, bridge, "", 2_000_000_000)
	if got != "array answer" {
		t.Fatalf("got %q want array answer", got)
	}
}
