package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func telegramConnectedRunner(t *testing.T) *Runner {
	t.Helper()
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	instance.SetMCPBaseURL("http://127.0.0.1:4317")
	connectTestTelegramBackend(t, instance)
	// Opt-in auto-approve for out-of-scope sends used by most loopback unit tests.
	auto := true
	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProjectID:           "project-alpha",
		ProviderType:        "telegram",
		Action:              "test",
		TelegramAutoApprove: &auto,
	})
	if err != nil {
		t.Fatalf("enable auto-approve: %v", err)
	}
	return instance
}

func TestTelegramMcpLoopbackRequiresRunnerToken(t *testing.T) {
	instance := telegramConnectedRunner(t)
	instance.setTelegramLoopbackTokenForTest("good-token")

	handler := instance.TelegramLoopbackSendHandler()

	// Missing token.
	req := httptest.NewRequest(http.MethodPost, TelegramLoopbackSendPath, strings.NewReader(`{"text":"hi"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Wrong token — must not call Telegram (no fake server registered; would fail if sent).
	req = httptest.NewRequest(http.MethodPost, TelegramLoopbackSendPath, strings.NewReader(`{"text":"hi"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer bad-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTelegramMcpLoopbackWorksUnderProviderHome(t *testing.T) {
	instance := telegramConnectedRunner(t)
	token, err := instance.ensureTelegramLoopbackToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
	})

	// Simulate the main runner endpoint (has real keyring via memory store).
	ts := httptest.NewServer(instance.TelegramLoopbackSendHandler())
	t.Cleanup(ts.Close)

	// Child under synthetic provider HOME: loopback only, no bot token / runner keyring.
	child := &telegramProxyMcpServer{
		loopbackMode:    true,
		loopbackBaseURL: ts.URL,
		loopbackToken:   token,
		client:          ts.Client(),
		// Intentionally no botToken/chatID/runner — proves child does not need them.
	}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "from provider home"}})
	result, err := child.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("loopback callTool: %v", err)
	}
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "message_id: 77") {
		t.Fatalf("expected message_id in result, got %q", text)
	}
}

func TestTelegramMcpLoopbackDoesNotReadChildKeyring(t *testing.T) {
	instance := telegramConnectedRunner(t)
	token, err := instance.ensureTelegramLoopbackToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	var sawBotPath bool
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/bot") {
			sawBotPath = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":5}}`))
	})

	ts := httptest.NewServer(instance.TelegramLoopbackSendHandler())
	t.Cleanup(ts.Close)

	child := &telegramProxyMcpServer{
		loopbackMode:    true,
		loopbackBaseURL: ts.URL,
		loopbackToken:   token,
		client:          ts.Client(),
	}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "no child keyring"}})
	if _, err := child.callTool(context.Background(), params); err != nil {
		t.Fatalf("callTool: %v", err)
	}
	if !sawBotPath {
		t.Fatal("expected runner-side Telegram API call")
	}
}

func TestTelegramMcpLoopbackRequiresAutoApprove(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	instance.SetMCPBaseURL("http://127.0.0.1:4317")
	connectTestTelegramBackend(t, instance)
	token, err := instance.ensureTelegramLoopbackToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	sendCount := 0
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		sendCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":12}}`))
	})

	ts := httptest.NewServer(instance.TelegramLoopbackSendHandler())
	t.Cleanup(ts.Close)

	child := &telegramProxyMcpServer{
		loopbackMode:    true,
		loopbackBaseURL: ts.URL,
		loopbackToken:   token,
		client:          ts.Client(),
	}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "blocked until auto-approve"}})

	_, err = child.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_TOOL_APPROVAL_REQUIRED") {
		t.Fatalf("expected MCP_TOOL_APPROVAL_REQUIRED, got %v", err)
	}
	if sendCount != 0 {
		t.Fatalf("must not send when auto-approve is off, sendCount=%d", sendCount)
	}

	auto := true
	if _, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProjectID:           "project-alpha",
		ProviderType:        "telegram",
		Action:              "test",
		TelegramAutoApprove: &auto,
	}); err != nil {
		t.Fatalf("enable auto-approve: %v", err)
	}

	result, err := child.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("auto-approved send: %v", err)
	}
	text, _ := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "message_id: 12") {
		t.Fatalf("expected message_id, got %q", text)
	}
	if sendCount != 1 {
		t.Fatalf("expected exactly one Telegram send, got %d", sendCount)
	}
}

func TestTelegramMcpDirectModeDisabledOrExplicitWhenRunnerEndpointMissing(t *testing.T) {
	// Partial loopback env → refuse direct keyring fallback.
	child := &telegramProxyMcpServer{
		loopbackMode:    true,
		loopbackBaseURL: "",
		loopbackToken:   "tok",
		client:          http.DefaultClient,
	}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "x"}})
	_, err := child.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_UNAVAILABLE") {
		t.Fatalf("expected MCP_UNAVAILABLE for incomplete loopback env, got %v", err)
	}

	// Unreachable runner.
	child = &telegramProxyMcpServer{
		loopbackMode:    true,
		loopbackBaseURL: "http://127.0.0.1:1",
		loopbackToken:   "tok",
		client:          &http.Client{},
	}
	_, err = child.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_UNAVAILABLE") {
		t.Fatalf("expected MCP_UNAVAILABLE for unreachable runner, got %v", err)
	}
}

func TestGrokTelegramLiveMCPServerIncludesLoopbackEnv(t *testing.T) {
	instance := telegramConnectedRunner(t)
	server, ok := instance.telegramLiveMCPServer()
	if !ok {
		t.Fatal("expected live telegram MCP server when connected")
	}
	if strings.TrimSpace(server.Env[flowpilotRunnerURLEnv]) == "" {
		t.Fatalf("missing %s in live server env: %#v", flowpilotRunnerURLEnv, server.Env)
	}
	if strings.TrimSpace(server.Env[flowpilotRunnerMCPTokenEnv]) == "" {
		t.Fatalf("missing %s in live server env: %#v", flowpilotRunnerMCPTokenEnv, server.Env)
	}
	// Live merge must keep stdio launcher shape.
	if server.Type != "stdio" || !telegramMcpArgsLookHealthy(server.Args) {
		t.Fatalf("unexpected live server shape: %#v", server)
	}
}

func TestTelegramMcpLoopbackRejectsNonLoopbackCaller(t *testing.T) {
	instance := telegramConnectedRunner(t)
	token, err := instance.ensureTelegramLoopbackToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, TelegramLoopbackSendPath, strings.NewReader(`{"text":"hi"}`))
	req.RemoteAddr = "203.0.113.9:9999"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	instance.TelegramLoopbackSendHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback, got %d", rec.Code)
	}
}

func TestEnsureGrokTelegramMcpConfigWritesLoopbackEnv(t *testing.T) {
	instance := telegramConnectedRunner(t)
	grokHome := t.TempDir()
	if _, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
	}); err != nil {
		t.Fatalf("ensure grok: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(grokHome, "config.toml"))
	if err != nil {
		t.Fatalf("read grok config: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, flowpilotRunnerURLEnv) || !strings.Contains(body, flowpilotRunnerMCPTokenEnv) {
		t.Fatalf("expected loopback env keys in grok config.toml, got:\n%s", body)
	}
	if strings.Contains(body, "123456:ABC-fake-token") {
		t.Fatal("bot token must not appear in grok config")
	}
}
