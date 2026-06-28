package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func mockGemini(t *testing.T, script string, capture func(name string, args ...string)) func() {
	t.Helper()
	original := commandContextFn
	commandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if capture != nil {
			capture(name, args...)
		}
		return testShellCommand(ctx, script)
	}
	return func() { commandContextFn = original }
}

func TestGeminiACPArgsStayReadOnlyForMVP(t *testing.T) {
	got := strings.Join(geminiACPArgs("gemini-2.5-flash"), " ")
	for _, want := range []string{"--acp", "--approval-mode plan", "--model gemini-2.5-flash"} {
		if !strings.Contains(got, want) {
			t.Fatalf("geminiACPArgs = %q, missing %q", got, want)
		}
	}

	got = strings.Join(geminiACPArgs(""), " ")
	if strings.Contains(got, "--model") {
		t.Fatalf("geminiACPArgs with empty model = %q, should not include model", got)
	}
}

func TestGeminiAdapterStreamsToCompletion(t *testing.T) {
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"gemini-session"}}`) +
		shellReadLine() +
		shellOutputLines(
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hel"}}}}`,
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"lo"}}}}`,
			`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`,
		)
	var capturedArgs []string
	defer mockGemini(t, script, func(_ string, args ...string) {
		capturedArgs = append([]string{}, args...)
	})()

	a := newGeminiAdapter(".", "test-account", nil)
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", ModelName: "gemini-2.5-flash"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	args := strings.Join(capturedArgs, " ")
	for _, want := range []string{"--acp", "--approval-mode plan", "--model gemini-2.5-flash"} {
		if !strings.Contains(args, want) {
			t.Fatalf("captured Gemini args = %q, missing %q", args, want)
		}
	}
	types := b.types()
	for _, want := range []ProviderEventType{EventMessageDelta, EventMessageCompleted, EventTurnCompleted} {
		if !hasEventType(types, want) {
			t.Fatalf("events = %+v, missing %s", types, want)
		}
	}
	done, ok := b.firstOf(EventTurnCompleted)
	if !ok || done.FinalMessage != "Hello" {
		t.Fatalf("turn completed event = %+v, want final Hello", done)
	}
}

func TestGeminiAdapterUsesFinalResultWhenNoStreamedText(t *testing.T) {
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"gemini-session"}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Final"},{"type":"text","text":" answer"}]}}`)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(".", "test-account", nil)
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	done, ok := b.firstOf(EventTurnCompleted)
	if !ok || done.FinalMessage != "Final answer" {
		t.Fatalf("turn completed event = %+v, want final answer", done)
	}
}

func TestGeminiAdapterLoadsKnownSessionAndPersists(t *testing.T) {
	tmp := t.TempDir()
	sessionPath := filepath.Join(tmp, "session.json")
	promptPath := filepath.Join(tmp, "prompt.json")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		"IFS= read -r session_line\n" +
		"printf '%s' \"$session_line\" > " + strconv.Quote(sessionPath) + "\n" +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"real-gemini-session"}}`) +
		"IFS= read -r prompt_line\n" +
		"printf '%s' \"$prompt_line\" > " + strconv.Quote(promptPath) + "\n" +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"text":"ok"}}`)
	defer mockGemini(t, script, nil)()

	store := &fakeProviderSessionStore{ch: make(chan ProviderSessionRecord, 4)}
	sessions := newGeminiSessionMap()
	sessions.setRealSession("test-account", "thread-1", "real-gemini-session")
	a := newGeminiAdapter(tmp, "test-account", nil)
	a.sessions = sessions
	a.sessionStore = store
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID: "run-uuid", StepID: "step-uuid", ProviderSessionID: "thread-1",
		Prompt: "resume", ModelName: "gemini-2.5-flash",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	sessionReq, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session request: %v", err)
	}
	writtenSession := string(sessionReq)
	if !strings.Contains(writtenSession, `"method":"session/load"`) ||
		!strings.Contains(writtenSession, `"sessionId":"real-gemini-session"`) {
		t.Fatalf("session request = %q, want session/load real id", writtenSession)
	}
	promptReq, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt request: %v", err)
	}
	if !strings.Contains(string(promptReq), `"sessionId":"real-gemini-session"`) {
		t.Fatalf("prompt request = %q, want real session id", string(promptReq))
	}
	select {
	case rec := <-store.ch:
		if rec.WorkflowRunID != "run-uuid" || rec.WorkflowStepRunID != "" ||
			rec.ProviderKey != string(ProviderKeyGemini) ||
			rec.ProviderSessionID != "real-gemini-session" ||
			rec.ProviderThreadID != "real-gemini-session" {
			t.Fatalf("persisted record = %+v", rec)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for persisted Gemini session")
	}
}

func TestGeminiAdapterRejectsSyntheticResumeWithoutMapping(t *testing.T) {
	tmp := t.TempDir()
	sessionPath := filepath.Join(tmp, "session.json")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		"IFS= read -r session_line\n" +
		"printf '%s' \"$session_line\" > " + strconv.Quote(sessionPath) + "\n" +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"fresh-gemini-session"}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"text":"ok"}}`)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(tmp, "test-account", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resumeID := "gemini_acp_session_legacy-1"

	err := a.SendTurn(ctx, TurnRequest{
		RunID: "r", StepID: "s", Prompt: "hi", ProviderSessionID: resumeID,
	}, &fakeClaudeBridge{})
	if err == nil || !strings.Contains(err.Error(), "no scoped real mapping") {
		t.Fatalf("SendTurn error = %v, want synthetic resume failure", err)
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("expected no session request file, stat err=%v", err)
	}
}

func TestGeminiAdapterUpdatesSessionMappingWhenPromptReturnsNewSessionID(t *testing.T) {
	tmp := t.TempDir()
	sessions := newGeminiSessionMap()
	sessions.setRealSession("test-account", "thread-1", "real-gemini-session")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"real-gemini-session"}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"sessionId":"updated-gemini-session","text":"ok"}}`)
	store := &fakeProviderSessionStore{ch: make(chan ProviderSessionRecord, 4)}
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(tmp, "test-account", nil)
	a.sessions = sessions
	a.sessionStore = store
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID: "r", StepID: "s", Prompt: "resume", ProviderSessionID: "thread-1",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := sessions.realSession("test-account", "thread-1"); got != "updated-gemini-session" {
		t.Fatalf("updated mapping = %q, want updated-gemini-session", got)
	}
	var last ProviderSessionRecord
	timeout := time.After(time.Second)
	for {
		select {
		case rec := <-store.ch:
			last = rec
		case <-timeout:
			if last.ProviderSessionID != "updated-gemini-session" || last.WorkflowStepRunID != "" {
				t.Fatalf("persisted record = %+v", last)
			}
			return
		}
	}
}

func TestGeminiAdapterScopesSessionMappingByAccount(t *testing.T) {
	sessions := newGeminiSessionMap()
	sessions.setRealSession("acct-a", "thread-1", "real-a")
	sessions.setRealSession("acct-b", "thread-1", "real-b")
	if got := sessions.realSession("acct-a", "thread-1"); got != "real-a" {
		t.Fatalf("acct-a realSession = %q, want real-a", got)
	}
	if got := sessions.realSession("acct-b", "thread-1"); got != "real-b" {
		t.Fatalf("acct-b realSession = %q, want real-b", got)
	}
	if got := sessions.realSession("acct-c", "thread-1"); got != "" {
		t.Fatalf("acct-c realSession = %q, want empty", got)
	}
}

func TestGeminiAdapterUpdatesScopedMappingWhenLoadReturnsNewSessionID(t *testing.T) {
	tmp := t.TempDir()
	sessions := newGeminiSessionMap()
	sessions.setRealSession("acct-a", "thread-1", "real-a")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"real-b"}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"text":"ok"}}`)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(tmp, "acct-a", nil)
	a.sessions = sessions
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID: "r", StepID: "s", Prompt: "resume", ProviderSessionID: "thread-1",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	if got := sessions.realSession("acct-a", "thread-1"); got != "real-b" {
		t.Fatalf("updated mapping = %q, want real-b", got)
	}
}

func TestGeminiAdapterMapsToolEventsAndPermissionRequest(t *testing.T) {
	tmp := t.TempDir()
	replyPath := filepath.Join(tmp, "permission-reply.json")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"gemini-session"}}`) +
		shellReadLine() +
		shellOutputLines(
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-session","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"Write file","kind":"edit","status":"pending","rawInput":{"path":"a.txt"}}}}`,
			`{"jsonrpc":"2.0","id":99,"method":"session/request_permission","params":{"sessionId":"gemini-session","toolCall":{"toolCallId":"call-1","title":"Write file","kind":"edit","status":"pending"},"options":[{"optionId":"allow-once","name":"Allow once","kind":"allow_once"},{"optionId":"reject-once","name":"Reject once","kind":"reject_once"}]}}`,
		) +
		"IFS= read -r reply_line\n" +
		"printf '%s' \"$reply_line\" > " + strconv.Quote(replyPath) + "\n" +
		shellOutputLines(
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-session","update":{"sessionUpdate":"tool_call_update","toolCallId":"call-1","title":"Write file","kind":"edit","status":"failed","rawOutput":"denied"}}}`,
			`{"jsonrpc":"2.0","id":3,"result":{"text":"done"}}`,
		)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(".", "test-account", nil)
	b := &fakeClaudeBridge{approval: "deny"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	types := b.types()
	for _, want := range []ProviderEventType{EventToolStarted, EventToolCompleted, EventMessageCompleted, EventTurnCompleted} {
		if !hasEventType(types, want) {
			t.Fatalf("events = %+v, missing %s", types, want)
		}
	}
	if len(b.approvalCalls) != 1 || b.approvalCalls[0].Command != "Write file" {
		t.Fatalf("approval calls = %+v, want Write file", b.approvalCalls)
	}
	raw, err := os.ReadFile(replyPath)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	reply := string(raw)
	if !strings.Contains(reply, `"id":99`) || !strings.Contains(reply, `"optionId":"reject-once"`) {
		t.Fatalf("permission reply = %q, want id 99 reject-once", reply)
	}
}

func TestGeminiAdapterSurfacesACPError(t *testing.T) {
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"error":{"message":"bad initialize"}}`)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(".", "test-account", nil)
	err := a.SendTurn(context.Background(), TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, &fakeClaudeBridge{})
	if err == nil || !strings.Contains(err.Error(), "gemini initialize: bad initialize") {
		t.Fatalf("SendTurn error = %v, want initialize error", err)
	}
}

func TestGeminiAdapterPromptPrepAndEnv(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "prompt.json")
	homePath := filepath.Join(tmp, "gemini-home")
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"gemini-session"}}`) +
		"IFS= read -r prompt_line\n" +
		"printf '%s' \"$prompt_line\" > " + strconv.Quote(promptPath) + "\n" +
		"printf '%s\\n' '{\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"text\":\"ok\"}}'\n"
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(tmp, "test-account", map[string]string{"GEMINI_HOME": homePath})
	a.promptPrep = func(req TurnRequest) string { return "prepared: " + req.Prompt }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	raw, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	written := string(raw)
	if !strings.Contains(written, `"text":"prepared: hi"`) {
		t.Fatalf("prompt request = %q, want prepared prompt", written)
	}
}

func TestGeminiAdapterPassesFlowPilotMCPServerToSessionNew(t *testing.T) {
	tmp := t.TempDir()
	sessionNewPath := filepath.Join(tmp, "session-new.json")
	script := shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`) +
		"IFS= read -r session_new_line\n" +
		"printf '%s' \"$session_new_line\" > " + strconv.Quote(sessionNewPath) + "\n" +
		shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"gemini-session"}}`) +
		shellReadLine() +
		shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{"text":"ok"}}`)
	defer mockGemini(t, script, nil)()

	a := newGeminiAdapter(".", "test-account", nil)
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi"}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	raw, err := os.ReadFile(sessionNewPath)
	if err != nil {
		t.Fatalf("read session/new: %v", err)
	}
	written := string(raw)
	if !strings.Contains(written, `"method":"session/new"`) ||
		!strings.Contains(written, `"name":"flowpilot"`) ||
		!strings.Contains(written, `"type":"http"`) ||
		!strings.Contains(written, ClaudeMCPPath+"?token=") {
		t.Fatalf("session/new request = %q, want FlowPilot MCP server", written)
	}
}

func TestGeminiRegistryGating(t *testing.T) {
	def := DefaultProviderRegistry()
	if reg, _ := def.Get(ProviderKeyGemini); reg.Status != ProviderStatusPlaceholder {
		t.Fatalf("default gemini status = %q, want placeholder", reg.Status)
	}
	if _, err := def.Selectable(ProviderKeyGemini); err == nil {
		t.Fatalf("placeholder gemini must not be selectable")
	}

	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_HOME", "")
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reg := ProviderRegistryFor(r)
	got, ok := reg.Get(ProviderKeyGemini)
	if !ok || got.Status != ProviderStatusAvailable {
		t.Fatalf("live gemini status = %q (ok=%v), want available", got.Status, ok)
	}
	if !got.Capabilities.Streaming || !got.Capabilities.SkillSelection || !got.Capabilities.Interrupt {
		t.Fatalf("live gemini must advertise MVP capabilities: %+v", got.Capabilities)
	}
	if got.Capabilities.ApprovalEvents || got.Capabilities.FileEvents || got.Capabilities.Mcp || got.Capabilities.Resume || got.Capabilities.Vision {
		t.Fatalf("live gemini must not advertise unproven capabilities: %+v", got.Capabilities)
	}
	if _, err := reg.Selectable(ProviderKeyGemini); err != nil {
		t.Fatalf("live gemini should be selectable: %v", err)
	}
	adapter, err := reg.Adapter(ProviderKeyGemini)
	if err != nil {
		t.Fatalf("live gemini adapter: %v", err)
	}
	if adapter.Key() != ProviderKeyGemini {
		t.Fatalf("adapter key = %s, want gemini", adapter.Key())
	}
}

func TestRefreshResumeHandleCapturesGeminiRealSession(t *testing.T) {
	svc := NewInteractiveService()
	sessions := newGeminiSessionMap()
	sessions.setRealSession("test-account", "thread-1", "real-gemini-session")
	a := newGeminiAdapter(".", "test-account", nil)
	a.sessions = sessions
	rs := &interactiveRun{providerKey: ProviderKeyGemini, providerSessionID: "thread-1"}

	if got := svc.refreshResumeHandleLocked(rs, a); got != "" {
		t.Fatalf("refreshResumeHandleLocked returned %q, want empty Gemini delta", got)
	}
	if rs.realProviderSessionID != "real-gemini-session" {
		t.Fatalf("realProviderSessionID = %q, want real-gemini-session", rs.realProviderSessionID)
	}
}
