package runner

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// 07 plan tests (CL-01..CL-40). Process-level tests drive a scripted fake `claude`
// over PowerShell (same technique as sessions_test.go); the rest are pure unit tests
// on the event mapper / arg builder. No real `claude` binary is required.

// ---- fake bridge -----------------------------------------------------------

type fakeClaudeBridge struct {
	mu             sync.Mutex
	events         []ProviderEvent
	approvalCalls  []ApprovalDetails
	approval       string
	approvalErr    error
	questionPrompt string
	questionOpts   []QuestionOption
	answer         []string
}

func (b *fakeClaudeBridge) Emit(ev ProviderEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *fakeClaudeBridge) RequestApproval(d ApprovalDetails) (string, error) {
	b.mu.Lock()
	b.approvalCalls = append(b.approvalCalls, d)
	b.mu.Unlock()
	return b.approval, b.approvalErr
}

func (b *fakeClaudeBridge) AskQuestion(prompt string, options []QuestionOption, _ bool) ([]string, error) {
	b.mu.Lock()
	b.questionPrompt = prompt
	b.questionOpts = options
	b.mu.Unlock()
	return b.answer, nil
}

func (b *fakeClaudeBridge) types() []ProviderEventType {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProviderEventType, 0, len(b.events))
	for _, e := range b.events {
		out = append(out, e.Type)
	}
	return out
}

func (b *fakeClaudeBridge) firstOf(t ProviderEventType) (ProviderEvent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.Type == t {
			return e, true
		}
	}
	return ProviderEvent{}, false
}

func hasEventType(types []ProviderEventType, want ProviderEventType) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

// mockClaude swaps commandContextFn for a PowerShell script acting as a fake `claude`.
func mockClaude(t *testing.T, script string) func() {
	t.Helper()
	original := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}
	return func() { commandContextFn = original }
}

func newTestClaudeAdapter() *claudeAdapter {
	return newClaudeAdapter(newClaudeProcessPool(), ".", "test-account", nil, "")
}

// ---- CL-05/CL-07: stream → completion --------------------------------------

func TestClaudeAdapterStreamsToCompletion(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"s1"}'; ` +
		`Write-Output '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}}'; ` +
		`Write-Output '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}'; ` +
		`Write-Output '{"type":"result","subtype":"success","result":"Hello"}'`
	defer mockClaude(t, script)()

	a := newTestClaudeAdapter()
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	types := b.types()
	for _, want := range []ProviderEventType{EventTurnStarted, EventMessageDelta, EventMessageCompleted, EventTurnCompleted} {
		if !hasEventType(types, want) {
			t.Fatalf("missing %s in %v", want, types)
		}
	}
	if ev, _ := b.firstOf(EventTurnCompleted); ev.FinalMessage != "Hello" {
		t.Fatalf("final message = %q, want Hello", ev.FinalMessage)
	}
	if ev, _ := b.firstOf(EventMessageDelta); ev.Text != "Hel" {
		t.Fatalf("delta = %q, want Hel", ev.Text)
	}
}

// ---- CL-20: YOLO=false approval round trip (deny path proves blocking) ------

func TestClaudeAdapterApprovalRoundTrip(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"control_request","subtype":"can_use_tool","request_id":"r1","request":{"command":"rm -rf x","cwd":"."}}'; ` +
		`$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"result","subtype":"success","result":"done"}'`
	defer mockClaude(t, script)()

	a := newTestClaudeAdapter()
	b := &fakeClaudeBridge{approval: "deny"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "go"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	b.mu.Lock()
	calls := append([]ApprovalDetails{}, b.approvalCalls...)
	b.mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("RequestApproval called %d times, want 1", len(calls))
	}
	if calls[0].Command != "rm -rf x" {
		t.Fatalf("approval command = %q, want 'rm -rf x'", calls[0].Command)
	}
	if !hasEventType(b.types(), EventTurnCompleted) {
		t.Fatalf("turn did not complete after approval reply: %v", b.types())
	}
}

// ---- CL-25: ask_user round trip --------------------------------------------

func TestClaudeAdapterAskUserRoundTrip(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"control_request","subtype":"ask_user","request_id":"q1","request":{"prompt":"Pick one","options":["A","B"]}}'; ` +
		`$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"result","subtype":"success","result":"done"}'`
	defer mockClaude(t, script)()

	a := newTestClaudeAdapter()
	b := &fakeClaudeBridge{answer: []string{"A"}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "go"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	b.mu.Lock()
	prompt := b.questionPrompt
	opts := len(b.questionOpts)
	b.mu.Unlock()
	if prompt != "Pick one" || opts != 2 {
		t.Fatalf("AskQuestion got prompt=%q opts=%d, want 'Pick one' / 2", prompt, opts)
	}
}

// ---- CL-31: process death mid-turn → recoverable, no hang ------------------

func TestClaudeAdapterProcessDeathRecoverable(t *testing.T) {
	// Emits a delta then exits WITHOUT a result line → stream closes mid-turn.
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"s1"}'`
	defer mockClaude(t, script)()

	a := newTestClaudeAdapter()
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, b)
	if err == nil {
		t.Fatalf("expected a stream-closed error, got nil")
	}
	if !isRecoverableSendError(err) {
		t.Fatalf("stream-closed error should be recoverable: %v", err)
	}
}

// ---- CL-30: resume uses the REAL session id, never the synthetic one -------

func TestClaudeAdapterResumeUsesRealSessionID(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"real-abc"}'; ` +
		`Write-Output '{"type":"result","subtype":"success","result":"ok"}'`
	var mu sync.Mutex
	var argsByCall [][]string
	original := commandContextFn
	defer func() { commandContextFn = original }()
	commandContextFn = func(ctx context.Context, _ string, arg ...string) *exec.Cmd {
		mu.Lock()
		argsByCall = append(argsByCall, append([]string{}, arg...))
		mu.Unlock()
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	pool := newClaudeProcessPool()
	a := newClaudeAdapter(pool, ".", "acct", nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// req.ProviderSessionID is the SYNTHETIC FlowPilot id (interactive_handlers seeds
	// it via nextID("thread")). It must never be passed to --resume.
	req := TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", ProviderSessionID: "thread-1"}

	if err := a.SendTurn(ctx, req, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("turn1: %v", err)
	}
	if got := pool.realSession("thread-1"); got != "real-abc" {
		t.Fatalf("captured real session = %q, want real-abc", got)
	}
	if err := a.SendTurn(ctx, req, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("turn2: %v", err)
	}

	mu.Lock()
	calls := argsByCall
	mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected 2 spawns, got %d", len(calls))
	}
	if argIndex(calls[0], "--resume") >= 0 {
		t.Fatalf("turn1 must NOT resume (no real session id captured yet): %v", calls[0])
	}
	if !flagHasValue(calls[1], "--resume", "real-abc") {
		t.Fatalf("turn2 must --resume the real session id: %v", calls[1])
	}
}

// ---- CL-21: YOLO=true → bypassPermissions, no permission_required ----------

func TestClaudeAdapterYoloTrueNoPermission(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"s1"}'; ` +
		`Write-Output '{"type":"result","subtype":"success","result":"ok"}'`
	defer mockClaude(t, script)()

	a := newTestClaudeAdapter()
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", YoloMode: true}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if hasEventType(b.types(), EventPermissionRequired) {
		t.Fatalf("YOLO=true must not surface permission_required: %v", b.types())
	}
	if !hasEventType(b.types(), EventTurnCompleted) {
		t.Fatalf("expected turn_completed, got %v", b.types())
	}
}

// ---- CL-35: two sessions run concurrently as separate processes ------------

func TestClaudeAdapterConcurrentSessions(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"s"}'; ` +
		`Write-Output '{"type":"result","subtype":"success","result":"ok"}'`
	defer mockClaude(t, script)()

	pool := newClaudeProcessPool()
	a := newClaudeAdapter(pool, ".", "acct", nil, "")
	cwds := []string{t.TempDir(), t.TempDir()}
	errs := make([]error, len(cwds))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := range cwds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = a.SendTurn(ctx, TurnRequest{
				RunID: "r-" + cwds[i], StepID: "s", Prompt: "hi",
				Cwd: cwds[i], ProviderSessionID: "sess-" + cwds[i],
			}, &fakeClaudeBridge{})
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("concurrent turn %d (%s): %v", i, cwds[i], e)
		}
	}
}

// ---- persistence: captured real session id reaches the store --------------

type fakeProviderSessionStore struct{ ch chan ProviderSessionRecord }

func (f *fakeProviderSessionStore) UpsertSession(_ context.Context, rec ProviderSessionRecord) error {
	f.ch <- rec
	return nil
}

func TestClaudeAdapterPersistsSession(t *testing.T) {
	script := `$null=[Console]::In.ReadLine(); ` +
		`Write-Output '{"type":"system","subtype":"init","session_id":"real-xyz"}'; ` +
		`Write-Output '{"type":"result","subtype":"success","result":"ok"}'`
	defer mockClaude(t, script)()

	store := &fakeProviderSessionStore{ch: make(chan ProviderSessionRecord, 4)}
	a := newClaudeAdapter(newClaudeProcessPool(), ".", "acct-1", nil, "")
	a.sessionStore = store
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-uuid", StepID: "s", Prompt: "hi", ProviderSessionID: "thread-1"}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	select {
	case rec := <-store.ch:
		if rec.WorkflowRunID != "run-uuid" || rec.ProviderSessionID != "real-xyz" {
			t.Fatalf("persisted record = %+v", rec)
		}
		if rec.WorkingDirectory != "." || rec.ProviderKey != string(ProviderKeyClaude) {
			t.Fatalf("persisted record (cwd/provider) = %+v", rec)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("expected a persisted session record")
	}
}

// ---- CL-40: registry gating ------------------------------------------------

func TestClaudeRegistryGating(t *testing.T) {
	// Default registry: claude is a non-selectable placeholder.
	def := DefaultProviderRegistry()
	if reg, _ := def.Get(ProviderKeyClaude); reg.Status != ProviderStatusPlaceholder {
		t.Fatalf("default claude status = %q, want placeholder", reg.Status)
	}
	if _, err := def.Selectable(ProviderKeyClaude); err == nil {
		t.Fatalf("placeholder claude must not be selectable")
	}

	// Flag on: claude becomes an available, selectable provider.
	t.Setenv(claudeAdapterEnvFlag, "1")
	r, err := New(".")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reg := ProviderRegistryFor(r)
	got, ok := reg.Get(ProviderKeyClaude)
	if !ok || got.Status != ProviderStatusAvailable {
		t.Fatalf("gated-on claude status = %q (ok=%v), want available", got.Status, ok)
	}
	if !got.Capabilities.Streaming || !got.Capabilities.ApprovalEvents {
		t.Fatalf("gated-on claude must advertise capabilities: %+v", got.Capabilities)
	}
	if _, err := reg.Selectable(ProviderKeyClaude); err != nil {
		t.Fatalf("gated-on claude should be selectable: %v", err)
	}
}

// ---- CL-01/CL-07/CL-10/CL-12: event mapper units ---------------------------

func TestMapClaudeLineInit(t *testing.T) {
	evs := mapClaudeLine(claudeLine{Type: "system", Subtype: "init", Raw: map[string]any{"session_id": "s1"}})
	if len(evs) != 1 || evs[0].Type != EventTurnStarted {
		t.Fatalf("init mapping = %+v, want one turn_started", evs)
	}
}

func TestMapClaudeLineDelta(t *testing.T) {
	raw := map[string]any{"event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hi"}}}
	evs := mapClaudeLine(claudeLine{Type: "stream_event", Raw: raw})
	if len(evs) != 1 || evs[0].Type != EventMessageDelta || evs[0].Text != "hi" {
		t.Fatalf("delta mapping = %+v", evs)
	}
}

func TestMapClaudeLineAssistantToolUseAndFileChange(t *testing.T) {
	raw := map[string]any{"message": map[string]any{"content": []any{
		map[string]any{"type": "tool_use", "name": "Edit", "input": map[string]any{"file_path": "a.go"}},
		map[string]any{"type": "text", "text": "done"},
	}}}
	evs := mapClaudeLine(claudeLine{Type: "assistant", Raw: raw})
	var sawTool, sawFile, sawMsg bool
	for _, e := range evs {
		switch e.Type {
		case EventToolStarted:
			sawTool = e.ToolName == "Edit"
		case EventFileChanged:
			sawFile = e.Path == "a.go" && e.ChangeType == "modified"
		case EventMessageCompleted:
			sawMsg = e.Text == "done"
		}
	}
	if !sawTool || !sawFile || !sawMsg {
		t.Fatalf("assistant mapping incomplete: tool=%v file=%v msg=%v (%+v)", sawTool, sawFile, sawMsg, evs)
	}
}

func TestMapClaudeLineToolResult(t *testing.T) {
	raw := map[string]any{"message": map[string]any{"content": []any{
		map[string]any{"type": "tool_result", "is_error": true, "content": "boom"},
	}}}
	evs := mapClaudeLine(claudeLine{Type: "user", Raw: raw})
	if len(evs) != 1 || evs[0].Type != EventToolCompleted || evs[0].Status != "failed" {
		t.Fatalf("tool_result mapping = %+v, want one failed tool_completed", evs)
	}
}

func TestMapClaudeLineResultError(t *testing.T) {
	evs := mapClaudeLine(claudeLine{Type: "result", Subtype: "error_max_turns", Raw: map[string]any{"subtype": "error_max_turns", "is_error": true}})
	if len(evs) != 1 || evs[0].Type != EventTurnFailed || evs[0].Recoverable {
		t.Fatalf("error result mapping = %+v, want non-recoverable turn_failed", evs)
	}
}

// ---- CL-21 arg builder (YOLO SSOT → CLI flags) -----------------------------

func TestClaudeArgsYoloPosture(t *testing.T) {
	yes := claudeArgs(resolveYoloPosture(true), "", "", nil)
	if !flagHasValue(yes, "--permission-mode", "bypassPermissions") {
		t.Fatalf("yolo=true args missing bypassPermissions: %v", yes)
	}
	if argIndex(yes, "--permission-prompt-tool") >= 0 {
		t.Fatalf("yolo=true must not add a permission-prompt-tool: %v", yes)
	}

	gated := claudeArgs(resolveYoloPosture(false), "sess-1", "cfg.json", nil)
	if !flagHasValue(gated, "--permission-mode", "default") {
		t.Fatalf("yolo=false args missing default mode: %v", gated)
	}
	if !flagHasValue(gated, "--permission-prompt-tool", claudeApproveToolName) {
		t.Fatalf("yolo=false + mcpConfig must add the permission-prompt-tool: %v", gated)
	}
	if !flagHasValue(gated, "--resume", "sess-1") {
		t.Fatalf("session id must produce --resume: %v", gated)
	}
}

func argIndex(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}

func flagHasValue(args []string, flag, val string) bool {
	i := argIndex(args, flag)
	return i >= 0 && i+1 < len(args) && args[i+1] == val
}
