package runner

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// CA-712 root cause (BUG-341): after a DENIED session/request_permission,
// opencode 1.18.x completes session/prompt with stopReason end_turn but never
// streams the assistant reply as agent_message_chunk — live or in any drain
// window (9/9 denied turns blank in cli-runner.log). The reply only resurfaces
// via a session/load replay. These tests lock the recovery: replay text after
// the current turn's user_message_chunk becomes FinalMessage; everything else
// in the replay stays local (no duplicate tool events, no old answers).

func recoveryUpdate(sessionID, kind string, extra map[string]any) map[string]any {
	update := map[string]any{"sessionUpdate": kind}
	for k, v := range extra {
		update[k] = v
	}
	return map[string]any{"sessionId": sessionID, "update": update}
}

func TestCollectOpencodeReplayAnswerKeepsOnlyCurrentTurn(t *testing.T) {
	ch := make(chan opencodeNotification, 16)
	// Old turn replay: prompt → answer → tool noise (all must be dropped).
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "user_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "old prompt"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "old answer"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "tool_call", map[string]any{"title": "read"})}
	// Current turn replay: the just-finished prompt then the lost answer.
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "user_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "current prompt"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "Toi la Nam"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": " — da ghi nho."}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "tool_call", map[string]any{"title": "read"})}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := collectOpencodeReplayAnswer(ctx, ch)
	if got != "Toi la Nam — da ghi nho." {
		t.Fatalf("replay recovery text = %q, want %q", got, "Toi la Nam — da ghi nho.")
	}
}

// A turn that genuinely produced no message part (e.g. pure tool run after the
// denial) must recover "" — never an older turn's answer.
func TestCollectOpencodeReplayAnswerNoCurrentAnswer(t *testing.T) {
	ch := make(chan opencodeNotification, 8)
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "user_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "old prompt"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "old answer"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "user_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "current prompt"}})}
	ch <- opencodeNotification{Method: "session/update", Params: recoveryUpdate("ses_rec", "tool_call", map[string]any{"title": "bash"})}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if got := collectOpencodeReplayAnswer(ctx, ch); got != "" {
		t.Fatalf("replay recovery text = %q, want empty (no answer this turn)", got)
	}
}

// fakeOpencodeDenyBridge denies every approval, mirroring the scan-posture
// read-only auto-deny that deterministically trips opencode's missing-answer bug.
type fakeOpencodeDenyBridge struct {
	fakeOpencodeBridge
}

func (b *fakeOpencodeDenyBridge) RequestApproval(details ApprovalDetails) (string, error) {
	b.approvals = append(b.approvals, details)
	return "deny", nil
}

// Full SendTurn wiring: permission denied → prompt completes end_turn with
// outputTokens but zero agent_message_chunk → adapter must issue exactly one
// session/load and surface the replayed answer as delta + FinalMessage.
func TestOpencodeReplayRecoveryAfterDeniedPermission(t *testing.T) {
	origWait := opencodeDeniedEmptyTextWait
	origQuiet := opencodeReplayQuietWindow
	opencodeDeniedEmptyTextWait = 200 * time.Millisecond
	opencodeReplayQuietWindow = 100 * time.Millisecond
	t.Cleanup(func() {
		opencodeDeniedEmptyTextWait = origWait
		opencodeReplayQuietWindow = origQuiet
	})

	d, fg := startFakeOpencode(t, nil)
	a := newOpencodeAdapter(d, "/tmp")
	bridge := &fakeOpencodeDenyBridge{}

	var loadCalls int32
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_recover1"})
		case "session/prompt":
			go func() {
				// Denied permission request (scan posture auto-deny on the real wire).
				fg.send(map[string]any{"jsonrpc": "2.0", "id": "perm-1", "method": "session/request_permission", "params": map[string]any{
					"sessionId": "ses_recover1",
					"options": []any{
						map[string]any{"optionId": "once", "kind": "allow_once", "name": "Allow once"},
						map[string]any{"optionId": "reject", "kind": "reject_once", "name": "Reject"},
					},
					"toolCall": map[string]any{"kind": "execute", "title": "ls", "rawInput": map[string]any{"command": "ls"}},
				}})
				time.Sleep(30 * time.Millisecond)
				// end_turn with NO agent_message_chunk — the CA-712 wire shape.
				fg.reply(m["id"], map[string]any{"stopReason": "end_turn", "usage": map[string]any{"outputTokens": float64(98)}})
			}()
		case "session/load":
			atomic.AddInt32(&loadCalls, 1)
			go func() {
				// Replay burst: old turn, then current prompt, then the answer.
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"availableCommands": []any{}}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": "old prompt"}}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "old answer"}}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "tool_call", "title": "read"}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": "test B4 in-place, toi la Nam"}}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Chao Nam, da xem xong"}}})
				fg.notify("session/update", map[string]any{"sessionId": "ses_recover1", "update": map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "thinking"}}})
			}()
			fg.reply(m["id"], map[string]any{"sessionId": "ses_recover1"})
		}
	})

	req := TurnRequest{RunID: "run-rec1", Prompt: "test B4 in-place, toi la Nam", Cwd: "/tmp"}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := atomic.LoadInt32(&loadCalls); got != 1 {
		t.Fatalf("session/load recovery calls = %d, want 1", got)
	}
	var final *ProviderEvent
	deltas := 0
	for i := range bridge.events {
		ev := bridge.events[i]
		if ev.Type == EventMessageDelta {
			deltas++
			if ev.Text != "Chao Nam, da xem xong" {
				t.Fatalf("unexpected delta text %q", ev.Text)
			}
		}
		if ev.Type == EventTurnCompleted {
			final = &bridge.events[i]
		}
	}
	if deltas != 1 {
		t.Fatalf("recovery deltas = %d, want 1 (events=%+v)", deltas, bridge.events)
	}
	if final == nil {
		t.Fatalf("no turn_completed event, events=%+v", bridge.events)
	}
	if final.FinalMessage != "Chao Nam, da xem xong" {
		t.Fatalf("FinalMessage = %q, want recovered answer", final.FinalMessage)
	}
	// Replay tool frames must stay local — no duplicate tool events, no old text.
	for _, ev := range bridge.events {
		if ev.Type == EventToolStarted || ev.Type == EventToolCompleted {
			t.Fatalf("replay leaked a tool event into the bridge: %+v", ev)
		}
		if ev.Type == EventMessageDelta && ev.Text == "old answer" {
			t.Fatalf("recovery emitted an older turn's answer")
		}
	}
}

// Without a denial the recovery must NOT fire — the generic wait and mapper
// path (CA-706..711) stays the only handler for empty text.
func TestOpencodeReplayRecoverySkippedWithoutDenial(t *testing.T) {
	origWait := opencodeEmptyTextWait
	opencodeEmptyTextWait = 150 * time.Millisecond
	t.Cleanup(func() { opencodeEmptyTextWait = origWait })

	d, fg := startFakeOpencode(t, nil)
	a := newOpencodeAdapter(d, "/tmp")
	bridge := &fakeOpencodeBridge{}

	var loadCalls int32
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_nodeny"})
		case "session/prompt":
			go func() {
				fg.notify("session/update", map[string]any{"sessionId": "ses_nodeny", "update": map[string]any{"sessionUpdate": "usage_update", "used": float64(100), "size": float64(8000)}})
				fg.reply(m["id"], map[string]any{"stopReason": "end_turn", "usage": map[string]any{"outputTokens": float64(0)}})
			}()
		case "session/load":
			atomic.AddInt32(&loadCalls, 1)
			fg.reply(m["id"], map[string]any{"sessionId": "ses_nodeny"})
		}
	})

	req := TurnRequest{RunID: "run-rec2", Prompt: "hello", Cwd: "/tmp"}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := atomic.LoadInt32(&loadCalls); got != 0 {
		t.Fatalf("session/load issued %d times without a denial, want 0", got)
	}
	for _, ev := range bridge.events {
		if ev.Type == EventTurnCompleted && ev.FinalMessage != "" {
			t.Fatalf("unexpected FinalMessage %q on a no-answer turn", ev.FinalMessage)
		}
	}
}
