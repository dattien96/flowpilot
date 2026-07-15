package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BUG-281: provider CLIs (especially Grok with HOME=<account home>) spawn
// flowpilot telegram-mcp as a child that inherits a synthetic HOME. macOS
// keyring lookups then miss the runner's flowpilot-runner secrets
// ("secret not found in keyring"). Option 3 keeps provider HOME isolation
// and moves keyring + Bot API execution back into the main runner via an
// authenticated loop-back HTTP endpoint. The MCP child becomes a thin
// client when loop-back env is present.

const (
	// TelegramLoopbackSendPath is the runner-local route provider-spawned
	// telegram-mcp children call for send_message.
	TelegramLoopbackSendPath = "/internal/mcp/telegram/send"

	// Env vars injected into provider MCP server config (never the bot token).
	flowpilotRunnerURLEnv      = "FLOWPILOT_RUNNER_URL"
	flowpilotRunnerMCPTokenEnv = "FLOWPILOT_RUNNER_MCP_TOKEN"

	telegramLoopbackTokenFileName = "telegram-mcp-loopback-token"
	telegramLoopbackTokenBytes    = 32
)

// telegramLoopbackSendRequest is the body the MCP child posts to the runner.
type telegramLoopbackSendRequest struct {
	Text string `json:"text"`
	// ChatID is an optional per-call override of the connected integration's
	// default channel. The provider-spawned MCP child never sets it (it always
	// sends to the connected channel), so the loop-back HTTP path is unchanged;
	// only the in-runner telegram.notify flow node (runTelegramNotifyNode) sets
	// it, to honor the chatId bound on the node's telegram.v1 OUTPUT artifact.
	ChatID string `json:"chatId,omitempty"`
}

// telegramLoopbackSendResponse is the runner's reply. The MCP child maps
// Status/Error into the same tool error/result strings Task-233 already
// expects (message_id evidence, MCP_TOOL_APPROVAL_REQUIRED, …).
type telegramLoopbackSendResponse struct {
	OK        bool   `json:"ok"`
	Status    string `json:"status,omitempty"` // sent|error
	MessageID int64  `json:"messageId,omitempty"`
	ChatID    string `json:"chatId,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// telegramLoopbackToken holds the workspace-scoped auth token the main
// runner accepts on the loop-back route. Persisted under .flowpilot so
// Configure Providers and a later runner restart share the same value.
type telegramLoopbackTokenState struct {
	mu    sync.Mutex
	token string
}

func (r *Runner) telegramLoopbackTokenPath() string {
	return filepath.Join(r.workspace, ".flowpilot", telegramLoopbackTokenFileName)
}

// ensureTelegramLoopbackToken returns the runner-issued auth token used by
// provider-spawned telegram-mcp children. It is NOT the Telegram bot token.
func (r *Runner) ensureTelegramLoopbackToken() (string, error) {
	if r == nil {
		return "", errors.New("runner is not configured")
	}
	if r.telegramLoopback == nil {
		r.telegramLoopback = &telegramLoopbackTokenState{}
	}
	r.telegramLoopback.mu.Lock()
	defer r.telegramLoopback.mu.Unlock()

	if strings.TrimSpace(r.telegramLoopback.token) != "" {
		return r.telegramLoopback.token, nil
	}

	path := r.telegramLoopbackTokenPath()
	if raw, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(raw))
		if token != "" {
			r.telegramLoopback.token = token
			return token, nil
		}
	}

	b := make([]byte, telegramLoopbackTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate telegram loopback token: %w", err)
	}
	token := hex.EncodeToString(b)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	r.telegramLoopback.token = token
	return token, nil
}

// setTelegramLoopbackTokenForTest injects a fixed token (tests only).
func (r *Runner) setTelegramLoopbackTokenForTest(token string) {
	if r.telegramLoopback == nil {
		r.telegramLoopback = &telegramLoopbackTokenState{}
	}
	r.telegramLoopback.mu.Lock()
	r.telegramLoopback.token = strings.TrimSpace(token)
	r.telegramLoopback.mu.Unlock()
}

func (r *Runner) telegramLoopbackTokenMatches(provided string) bool {
	expected, err := r.ensureTelegramLoopbackToken()
	if err != nil || expected == "" {
		return false
	}
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

// telegramLoopbackBaseURL is the URL the MCP child should call. Prefer the
// live serve-time mcpBaseURL; fall back to FLOWPILOT_RUNNER_* env / default
// port so Configure Providers still writes a usable value before the first
// turn registers the base URL.
func (r *Runner) telegramLoopbackBaseURL() string {
	if r != nil {
		if base := strings.TrimSpace(r.mcpBaseURLValue()); base != "" {
			return strings.TrimRight(base, "/")
		}
	}
	return strings.TrimRight(googleDriveRuntimeRunnerURL(), "/")
}

// telegramLoopbackMcpEnv is the env map written into provider MCP entries and
// live merge (telegramLiveMCPServer). Never includes the bot token.
func (r *Runner) telegramLoopbackMcpEnv() map[string]string {
	token, err := r.ensureTelegramLoopbackToken()
	if err != nil || strings.TrimSpace(token) == "" {
		return nil
	}
	return map[string]string{
		flowpilotRunnerURLEnv:      r.telegramLoopbackBaseURL(),
		flowpilotRunnerMCPTokenEnv: token,
	}
}

// TelegramLoopbackSendHandler mounts POST TelegramLoopbackSendPath.
func (r *Runner) TelegramLoopbackSendHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !isLoopbackRequest(req) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := extractTelegramLoopbackAuthToken(req)
		if !r.telegramLoopbackTokenMatches(token) {
			writeTelegramLoopbackJSON(w, http.StatusUnauthorized, telegramLoopbackSendResponse{
				OK:        false,
				Status:    "error",
				ErrorCode: "MCP_AUTH_FAILED",
				Error:     "invalid or missing FLOWPILOT_RUNNER_MCP_TOKEN",
			})
			return
		}

		body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		if err != nil {
			writeTelegramLoopbackJSON(w, http.StatusBadRequest, telegramLoopbackSendResponse{
				OK: false, Status: "error", ErrorCode: "MCP_BAD_REQUEST", Error: "failed to read body",
			})
			return
		}
		var payload telegramLoopbackSendRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			writeTelegramLoopbackJSON(w, http.StatusBadRequest, telegramLoopbackSendResponse{
				OK: false, Status: "error", ErrorCode: "MCP_BAD_REQUEST", Error: "invalid JSON body",
			})
			return
		}

		resp, status := r.executeTelegramLoopbackSend(req.Context(), payload)
		writeTelegramLoopbackJSON(w, status, resp)
	})
}

func extractTelegramLoopbackAuthToken(req *http.Request) string {
	if h := strings.TrimSpace(req.Header.Get("Authorization")); h != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(h, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(h, prefix))
		}
		return h
	}
	if h := strings.TrimSpace(req.Header.Get("X-FlowPilot-Runner-Token")); h != "" {
		return h
	}
	return strings.TrimSpace(req.URL.Query().Get("token"))
}

func writeTelegramLoopbackJSON(w http.ResponseWriter, status int, resp telegramLoopbackSendResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// executeTelegramLoopbackSend resolves the bot token from the runner keyring
// (correct HOME context) and sends when auto-approve is enabled.
func (r *Runner) executeTelegramLoopbackSend(ctx context.Context, payload telegramLoopbackSendRequest) (telegramLoopbackSendResponse, int) {
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return telegramLoopbackSendResponse{
			OK: false, Status: "error", ErrorCode: "MCP_BAD_REQUEST", Error: "text is required",
		}, http.StatusBadRequest
	}

	creds, err := r.resolveConnectedTelegramCredential()
	if err != nil {
		return telegramLoopbackSendResponse{
			OK: false, Status: "error", ErrorCode: "MCP_NOT_CONNECTED", Error: err.Error(),
		}, http.StatusServiceUnavailable
	}

	chatID := strings.TrimSpace(payload.ChatID)
	if chatID == "" {
		chatID = strings.TrimSpace(creds.ChannelID)
	}
	server := &telegramProxyMcpServer{
		botToken:    strings.TrimSpace(creds.BotToken),
		chatID:      chatID,
		client:      &http.Client{Timeout: 15 * time.Second},
		autoApprove: creds.AutoApprove,
	}

	result, callErr := server.callToolLocal(ctx, text)
	if callErr != nil {
		msg := callErr.Error()
		code := "MCP_SEND_FAILED"
		status := http.StatusBadRequest
		if strings.Contains(msg, "MCP_TOOL_APPROVAL_REQUIRED") {
			code = "MCP_TOOL_APPROVAL_REQUIRED"
			status = http.StatusOK
			return telegramLoopbackSendResponse{
				OK:        false,
				Status:    "error",
				ChatID:    server.chatID,
				ErrorCode: code,
				Error:     msg,
			}, status
		}
		return telegramLoopbackSendResponse{
			OK: false, Status: "error", ErrorCode: code, Error: msg, ChatID: server.chatID,
		}, http.StatusBadRequest
	}

	messageID := extractMessageIDFromToolResult(result)
	if messageID == 0 {
		return telegramLoopbackSendResponse{
			OK: false, Status: "error", ErrorCode: "MCP_SEND_FAILED",
			Error: "telegram sendMessage response did not include a message_id", ChatID: server.chatID,
		}, http.StatusBadRequest
	}
	return telegramLoopbackSendResponse{
		OK:        true,
		Status:    "sent",
		MessageID: messageID,
		ChatID:    server.chatID,
	}, http.StatusOK
}

func toolResultText(result map[string]any) string {
	if result == nil {
		return ""
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	return text
}

func extractMessageIDFromToolResult(result map[string]any) int64 {
	if result == nil {
		return 0
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return 0
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	const marker = "message_id: "
	idx := strings.Index(text, marker)
	if idx < 0 {
		return 0
	}
	var id int64
	_, _ = fmt.Sscanf(text[idx+len(marker):], "%d", &id)
	return id
}

// postTelegramLoopbackSend is used by the MCP child process.
func postTelegramLoopbackSend(ctx context.Context, baseURL, token string, payload telegramLoopbackSendRequest, client *http.Client) (telegramLoopbackSendResponse, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return telegramLoopbackSendResponse{}, errors.New("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send (missing FLOWPILOT_RUNNER_URL)")
	}
	if strings.TrimSpace(token) == "" {
		return telegramLoopbackSendResponse{}, errors.New("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send (missing FLOWPILOT_RUNNER_MCP_TOKEN)")
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return telegramLoopbackSendResponse{}, err
	}
	url := baseURL + TelegramLoopbackSendPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return telegramLoopbackSendResponse{}, fmt.Errorf("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := client.Do(req)
	if err != nil {
		return telegramLoopbackSendResponse{}, fmt.Errorf("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return telegramLoopbackSendResponse{}, fmt.Errorf("MCP_UNAVAILABLE: failed to read runner response: %w", err)
	}

	var parsed telegramLoopbackSendResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return telegramLoopbackSendResponse{}, fmt.Errorf("MCP_UNAVAILABLE: invalid runner response (HTTP %d)", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		if parsed.Error == "" {
			parsed.Error = "invalid or missing FLOWPILOT_RUNNER_MCP_TOKEN"
		}
		return parsed, fmt.Errorf("MCP_AUTH_FAILED: %s", parsed.Error)
	}
	if resp.StatusCode == http.StatusForbidden {
		return parsed, errors.New("MCP_UNAVAILABLE: FlowPilot runner rejected non-loopback Telegram send")
	}
	if resp.StatusCode >= 500 {
		msg := parsed.Error
		if msg == "" {
			msg = fmt.Sprintf("runner HTTP %d", resp.StatusCode)
		}
		return parsed, fmt.Errorf("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send: %s", msg)
	}
	return parsed, nil
}

// loopbackEnvFromProcess reads the loop-back configuration from the MCP
// child environment.
func loopbackEnvFromProcess() (baseURL, token string, enabled bool) {
	baseURL = strings.TrimSpace(os.Getenv(flowpilotRunnerURLEnv))
	token = strings.TrimSpace(os.Getenv(flowpilotRunnerMCPTokenEnv))
	// Both must be present for provider-spawned loopback mode. Partial env
	// is treated as misconfiguration and surfaces as MCP_UNAVAILABLE rather
	// than falling back to child keyring (which is broken under Grok HOME).
	if baseURL != "" || token != "" {
		return baseURL, token, true
	}
	return "", "", false
}
