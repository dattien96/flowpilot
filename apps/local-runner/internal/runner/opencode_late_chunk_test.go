package runner

import (
	"context"
	"testing"
	"time"
)

// testBridge is a minimal TurnBridge for opencode drain tests.
type testBridge struct {
	events []ProviderEvent
}

func (b *testBridge) Emit(ev ProviderEvent) { b.events = append(b.events, ev) }
func (b *testBridge) Accepted(receipt ReceiptEvidence) {}
func (b *testBridge) Terminal(proof TerminalEvidence) {}
func (b *testBridge) RequestApproval(details ApprovalDetails) (string, error) { return "", nil }
func (b *testBridge) AskQuestion(prompt string, options []QuestionOption, multiSelect bool) ([]string, error) {
	return nil, nil
}
func (b *testBridge) SpawnAgent(in SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}
func (b *testBridge) SubmitFlowControl(in FlowControlInput) (FlowControlResult, error) {
	return FlowControlResult{}, nil
}

// 421135 blank: late agent_message_chunk missed by non-blocking drain.
func TestOpencodeLateChunkIsCaptured(t *testing.T) {
	ch := make(chan opencodeNotification, 2)
	ch <- opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         "read_file",
			},
		},
	}
	go func() {
		time.Sleep(80 * time.Millisecond)
		ch <- opencodeNotification{
			Method: "session/update",
			Params: map[string]any{
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": "text",
						"text": "Chao Nam! Da ghi nho.",
					},
				},
			},
		}
	}()
	a := &opencodeAdapter{}
	bridge := &testBridge{}
	lastText := ""
	// Non-blocking drain should only see the tool, not the late text.
	lastText = a.drainOpencodeNotifications("ses_late1", ch, bridge, lastText)
	if lastText != "" {
		t.Fatalf("non-blocking drain should not have late text yet, got %q", lastText)
	}
	// Blocking drain should capture it.
	ctx := context.Background()
	lastText = a.drainOpencodeNotificationsBlocking(ctx, "ses_late1", ch, bridge, lastText, 600*time.Millisecond)
	if lastText != "Chao Nam! Da ghi nho." {
		t.Fatalf("late chunk not captured, got %q", lastText)
	}
	found := false
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && ev.Text == "Chao Nam! Da ghi nho." {
			found = true
		}
	}
	if !found {
		t.Fatalf("bridge missing late delta, events=%+v", bridge.events)
	}
}
