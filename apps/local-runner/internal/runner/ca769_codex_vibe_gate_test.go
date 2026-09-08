package runner

import (
	"context"
	"testing"
)

func TestCodexAdapterRejectsVibeRequirementWhenOnlyReviewOffered(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	toolReplies := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th-vibe-rej"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct-vibe-rej"})
			fc.send(map[string]any{
				"jsonrpc": "2.0", "id": 801, "method": "item/tool/call",
				"params": map[string]any{
					"threadId": "th-vibe-rej", "turnId": "ct-vibe-rej", "callId": "call_vibe_rej",
					"tool":      codexVibeRequirementToolName,
					"arguments": map[string]any{"verdict": "aligned"},
				},
			})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				if _, ok := res["contentItems"]; ok {
					toolReplies <- res
					fc.notify("turn.completed", map[string]any{"threadId": "th-vibe-rej", "finalMessage": "done"})
				}
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	if err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID: "r-vibe-rej", Prompt: "go", OfferReviewOutcomeTool: true,
	}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	bridge.mu.Lock()
	capturedFC := bridge.flowControlInput
	bridge.mu.Unlock()
	if capturedFC != nil {
		t.Fatalf("vibe-requirement-outcome must not map when OfferVibeRequirementTool is false, got %+v", capturedFC)
	}

	select {
	case res := <-toolReplies:
		if res["success"] == true {
			t.Fatalf("dynamic tool result success = %v, want false", res["success"])
		}
	default:
		t.Fatal("rejection reply was not sent to Codex")
	}
}
