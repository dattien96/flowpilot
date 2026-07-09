package runner

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ---- fake bridge (mirrors fakeClaudeBridge, claude_adapter_test.go) -------

type fakeGrokBridge struct {
	mu            sync.Mutex
	events        []ProviderEvent
	approvalCalls []ApprovalDetails
	approval      string
	approvalErr   error
}

func (b *fakeGrokBridge) Emit(ev ProviderEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}
func (b *fakeGrokBridge) RequestApproval(d ApprovalDetails) (string, error) {
	b.mu.Lock()
	b.approvalCalls = append(b.approvalCalls, d)
	b.mu.Unlock()
	return b.approval, b.approvalErr
}
func (b *fakeGrokBridge) AskQuestion(string, []QuestionOption, bool) ([]string, error) {
	return nil, nil
}
func (b *fakeGrokBridge) SpawnAgent(SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}
func (b *fakeGrokBridge) SubmitFlowControl(FlowControlInput) (FlowControlResult, error) {
	return FlowControlResult{}, nil
}

func (b *fakeGrokBridge) types() []ProviderEventType {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProviderEventType, 0, len(b.events))
	for _, e := range b.events {
		out = append(out, e.Type)
	}
	return out
}

func (b *fakeGrokBridge) approvalCallCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.approvalCalls)
}

// newTestGrokAdapter wires a grokAdapter to a fakeGrok server that answers
// initialize/session/new/session/prompt with the live-captured shapes, so
// SendTurn can be exercised end to end without a real grok binary. When
// promptReplyDelay > 0, the session/prompt reply is held back so a
// session/request_permission round-trip can complete first (mirrors the real
// live behavior: the prompt call blocks for the whole turn).
func newTestGrokAdapter(t *testing.T, sessionID string, promptResult map[string]any, extraFrames []map[string]any, promptReplyDelay time.Duration) (*grokAdapter, *fakeGrok) {
	t.Helper()
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()

	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new", "session/load":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			go func() {
				if promptReplyDelay > 0 {
					time.Sleep(promptReplyDelay)
				}
				for _, frame := range extraFrames {
					fg.notify("session/update", frame)
				}
				fg.reply(m["id"], promptResult)
			}()
		}
	})
	return a, fg
}

// ---- Task-207: chat MVP -----------------------------------------------------

func TestGrokAdapterSendTurnStreamsAndCompletes(t *testing.T) {
	sessionID := "session-mvp-1"
	a, _ := newTestGrokAdapter(t, sessionID, liveGrokPromptResult(sessionID), []map[string]any{
		{"sessionId": sessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "OK"}}},
	}, 0)

	bridge := &fakeGrokBridge{}
	err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", Prompt: "hi"}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	types := bridge.types()
	sawDelta, sawTokenUsage, sawCompleted := false, false, false
	for _, ty := range types {
		switch ty {
		case EventMessageDelta:
			sawDelta = true
		case EventTokenUsageUpdated:
			sawTokenUsage = true
		case EventTurnCompleted:
			sawCompleted = true
		}
	}
	if !sawDelta || !sawTokenUsage || !sawCompleted {
		t.Fatalf("expected delta+token_usage+completed events, got %v", types)
	}
}

func TestGrokAdapterCapabilitiesMatchProvenSet(t *testing.T) {
	a := newGrokAdapter(&grokDispatcher{waiters: map[int64]chan grokResponse{}, sessionSubs: map[string]chan grokNotification{}, done: make(chan struct{})}, "/tmp/x")
	caps := a.Capabilities()
	if !caps.Streaming || !caps.Resume || !caps.FileEvents || !caps.Interrupt || !caps.ApprovalEvents {
		t.Fatalf("expected streaming/resume/fileEvents/interrupt/approvalEvents true, got %+v", caps)
	}
	if caps.Mcp || caps.Vision || caps.SkillSelection {
		t.Fatalf("expected mcp/vision/skillSelection false (not yet proven), got %+v", caps)
	}
}

func TestGrokAdapterInterruptReturnsCtxErr(t *testing.T) {
	sessionID := "session-interrupt-1"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			// Never reply — simulate a long-running turn; the ctx cancel below
			// must unblock SendTurn without waiting for a response.
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	bridge := &fakeGrokBridge{}
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-2", Prompt: "hi"}, bridge)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ---- Task-208: permission channel + YOLO -----------------------------------

func TestGrokAdapterYoloOffBlocksOnBridgeAndDeniesWithoutApproval(t *testing.T) {
	sessionID := "session-perm-1"
	a, fg := newTestGrokAdapter(t, sessionID, liveGrokPromptResult(sessionID), nil, 300*time.Millisecond)

	bridge := &fakeGrokBridge{approval: "deny"}
	go func() {
		time.Sleep(80 * time.Millisecond)
		fg.send(map[string]any{"jsonrpc": "2.0", "id": 501, "method": "session/request_permission", "params": map[string]any{
			"sessionId": sessionID,
			"toolCall":  map[string]any{"title": "run_terminal_command", "_meta": map[string]any{"x.ai/tool": map[string]any{"name": "run_terminal_command", "kind": "execute"}}},
			"options": []any{
				map[string]any{"optionId": "allow-once-x", "kind": "allow_once"},
				map[string]any{"optionId": "reject-once-x", "kind": "reject_once"},
			},
		}})
	}()

	err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-3", Prompt: "run a command", YoloMode: false}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if bridge.approvalCallCount() != 1 {
		t.Fatalf("expected exactly 1 RequestApproval call, got %d", bridge.approvalCallCount())
	}
	if bridge.approvalCalls[0].Kind != "exec" {
		t.Fatalf("expected Kind=exec for run_terminal_command, got %q (GR-04/BUG-246)", bridge.approvalCalls[0].Kind)
	}
}

func TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge(t *testing.T) {
	sessionID := "session-perm-2"
	a, fg := newTestGrokAdapter(t, sessionID, liveGrokPromptResult(sessionID), nil, 300*time.Millisecond)

	replyCh := make(chan map[string]any, 1)
	// Intercept the reply Grok would receive by watching fg.requests via a
	// second serve wrapper is awkward here; instead assert indirectly: under
	// YOLO the bridge must NEVER be asked (Task-208 T-4 — runner policy
	// auto-answers, the request is not disabled, but it must not surface a UI
	// approval card).
	bridge := &fakeGrokBridge{approval: "deny"} // if bridge were consulted, it would deny — proving it wasn't
	go func() {
		time.Sleep(80 * time.Millisecond)
		fg.send(map[string]any{"jsonrpc": "2.0", "id": 502, "method": "session/request_permission", "params": map[string]any{
			"sessionId": sessionID,
			"toolCall":  map[string]any{"title": "write", "_meta": map[string]any{"x.ai/tool": map[string]any{"name": "write", "kind": "write"}}},
			"options": []any{
				map[string]any{"optionId": "allow-once-y", "kind": "allow_once"},
				map[string]any{"optionId": "reject-once-y", "kind": "reject_once"},
			},
		}})
		replyCh <- nil
	}()

	err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-4", Prompt: "write a file", YoloMode: true}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	<-replyCh
	time.Sleep(50 * time.Millisecond)
	if bridge.approvalCallCount() != 0 {
		t.Fatalf("YOLO=true must auto-approve via runner policy without ever calling bridge.RequestApproval; got %d calls", bridge.approvalCallCount())
	}
}

func TestGrokAdapterUnknownSessionDeniesRatherThanHang(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	_ = newGrokAdapter(d, "/tmp/x") // wires handleInbound as the dispatcher's inbound callback

	replied := make(chan map[string]any, 1)
	go func() {
		for m := range fg.requests {
			if m["id"] == float64(900) {
				replied <- m
				return
			}
		}
	}()
	fg.send(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "no-such-session",
		"options": []any{
			map[string]any{"optionId": "allow-once-z", "kind": "allow_once"},
			map[string]any{"optionId": "reject-once-z", "kind": "reject_once"},
		},
	}})

	select {
	case <-replied:
	case <-time.After(2 * time.Second):
		t.Fatal("no active turn for session must still be replied to, not left hanging")
	}
}

// ---- decision-vocabulary mapper unit tests ---------------------------------

func TestGrokEncodePermissionDecisionPicksOfferedOptionOnly(t *testing.T) {
	options := []any{
		map[string]any{"optionId": "allow-once-1", "kind": "allow_once"},
		map[string]any{"optionId": "allow-session-1", "kind": "allow_always"},
		map[string]any{"optionId": "reject-once-1", "kind": "reject_once"},
	}
	if got := grokEncodePermissionDecision(options, true); got != "allow-once-1" {
		t.Fatalf("approve: expected allow-once-1, got %q", got)
	}
	if got := grokEncodePermissionDecision(options, false); got != "reject-once-1" {
		t.Fatalf("deny: expected reject-once-1, got %q", got)
	}
}

func TestGrokEncodePermissionDecisionNoMatchReturnsEmpty(t *testing.T) {
	options := []any{map[string]any{"optionId": "weird-1", "kind": "unknown"}}
	if got := grokEncodePermissionDecision(options, false); got != "" {
		t.Fatalf("expected empty optionId for no match, got %q", got)
	}
}

func TestGrokPermissionKindClassifiesExecFileMcp(t *testing.T) {
	cases := []struct {
		name string
		call map[string]any
		want string
	}{
		{"read", map[string]any{"_meta": map[string]any{"x.ai/tool": map[string]any{"kind": "read", "name": "read_file"}}}, "file"},
		{"write", map[string]any{"_meta": map[string]any{"x.ai/tool": map[string]any{"kind": "write", "name": "write"}}}, "file"},
		{"exec", map[string]any{"_meta": map[string]any{"x.ai/tool": map[string]any{"kind": "execute", "name": "run_terminal_command"}}}, "exec"},
		{"mcp", map[string]any{"_meta": map[string]any{"x.ai/tool": map[string]any{"namespace": "google-drive", "name": "gdrive_search"}}}, "mcp"},
		{"other", map[string]any{}, "other"},
	}
	for _, c := range cases {
		if got := grokPermissionKind(c.call); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
