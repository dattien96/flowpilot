package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Task-232 (CP-05-05 P-1/P-2): telegramProxyMcpServer is a FlowPilot-owned
// stdio MCP server exposing exactly ONE tool, `send_message`, wrapping the
// Telegram Bot API's `sendMessage` endpoint. The 2026-07-13 survey found no
// mature, purpose-built Bot-API MCP server worth trusting with a bot token
// (community options are either MTProto user-account clients — over-scoped,
// account-ban risk — or <25-star single-maintainer packages); a minimal
// self-hosted proxy with a single-tool surface is safer and easier to audit
// than importing one via `npx`. It reuses the JSON-RPC plumbing
// (mcpRequest/mcpResponse/mcpError/proxyMcpTool, textToolResult) already
// defined for the Google Drive proxy (google_drive_proxy_mcp.go, same
// package) rather than redefining it.
type telegramProxyMcpServer struct {
	botToken string
	chatID   string
	stdin    *bufio.Scanner
	stdout   io.Writer
	client   *http.Client

	// autoApprove gates every send_message call (flow runs and CLI). When false,
	// the tool returns MCP_TOOL_APPROVAL_REQUIRED until the user enables
	// auto-approve on the Telegram integration in MCP settings.
	autoApprove bool

	// BUG-281 loop-back mode: when set, send_message posts to the main
	// runner instead of reading keyring / calling Telegram Bot API in-process.
	loopbackBaseURL string
	loopbackToken   string
	loopbackMode    bool
}

// telegramBotAPIBase is a var (not const), matching the executeJiraRequestFn
// seam pattern in this codebase, so tests can point it at an httptest server
// instead of the real Telegram API.
var telegramBotAPIBase = "https://api.telegram.org"

// RunTelegramProxyMcpServer launches the stdio server.
//
// BUG-281: when FLOWPILOT_RUNNER_URL + FLOWPILOT_RUNNER_MCP_TOKEN are present
// (provider-spawned MCP entries), this process does NOT read the keyring or
// hold the bot token — it is a thin loop-back client to the main runner,
// which owns credential resolution under the real user HOME. Without those
// env vars (manual `flowpilot telegram-mcp`), direct keyring mode remains
// available for developer/offline use.
func (r *Runner) RunTelegramProxyMcpServer(ctx context.Context) error {
	server := &telegramProxyMcpServer{
		stdin:  bufio.NewScanner(os.Stdin),
		stdout: os.Stdout,
		client: &http.Client{Timeout: 20 * time.Second},
	}

	if baseURL, token, enabled := loopbackEnvFromProcess(); enabled {
		server.loopbackMode = true
		server.loopbackBaseURL = baseURL
		server.loopbackToken = token
	} else {
		creds, err := r.resolveConnectedTelegramCredential()
		if err != nil {
			return fmt.Errorf("telegram-mcp: %w", err)
		}
		server.botToken = strings.TrimSpace(creds.BotToken)
		server.chatID = strings.TrimSpace(creds.ChannelID)
		server.autoApprove = creds.AutoApprove
		server.client = &http.Client{Timeout: 15 * time.Second}
	}

	server.stdin.Buffer(make([]byte, 64*1024), 10*1024*1024)
	return server.serve(ctx)
}

func (s *telegramProxyMcpServer) serve(ctx context.Context) error {
	encoder := json.NewEncoder(s.stdout)
	encoder.SetEscapeHTML(false)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if !s.stdin.Scan() {
			if err := s.stdin.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(s.stdin.Text())
		if line == "" {
			continue
		}

		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(mcpResponse{
				JSONRPC: "2.0",
				Error:   &mcpError{Code: -32700, Message: "parse error", Data: err.Error()},
			})
			continue
		}
		if len(strings.TrimSpace(string(req.ID))) == 0 {
			continue
		}

		resp := s.handleRequest(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
}

func (s *telegramProxyMcpServer) handleRequest(ctx context.Context, req mcpRequest) mcpResponse {
	switch req.Method {
	case "initialize":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2025-06-18",
				"serverInfo":      map[string]any{"name": "flowpilot-telegram-mcp", "version": Version},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			},
		}
	case "notifications/initialized", "ping":
		return mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.tools()}}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			return mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32000, Message: err.Error()}}
		}
		return mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
	default:
		return mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32601, Message: "method not found"}}
	}
}

// tools returns exactly one tool — the whole point of this proxy (Task-232
// package doc): no read/delete/forward/list surface, only send.
func (s *telegramProxyMcpServer) tools() []proxyMcpTool {
	return []proxyMcpTool{
		{
			Name:        "send_message",
			Description: "Send a text message to the configured Telegram chat/channel for this run.",
			InputSchema: map[string]any{
				"type":       "object",
				"required":   []string{"text"},
				"properties": map[string]any{"text": map[string]any{"type": "string", "description": "Message text to send."}},
			},
		},
	}
}

func (s *telegramProxyMcpServer) callTool(ctx context.Context, rawParams json.RawMessage) (map[string]any, error) {
	var params struct {
		Name      string `json:"name"`
		Arguments struct {
			Text string `json:"text"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}
	if params.Name != "send_message" {
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}
	text := strings.TrimSpace(params.Arguments.Text)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	if s.loopbackMode {
		return s.callToolViaLoopback(ctx, text)
	}

	return s.callToolLocal(ctx, text)
}

// callToolLocal sends via the in-process Bot API (manual telegram-mcp or the
// runner loop-back handler after keyring resolution).
func (s *telegramProxyMcpServer) callToolLocal(ctx context.Context, text string) (map[string]any, error) {
	if s.botToken == "" || s.chatID == "" {
		return nil, fmt.Errorf("telegram proxy is not configured with a bot token/chat id")
	}

	if !s.autoApprove {
		return nil, fmt.Errorf("MCP_TOOL_APPROVAL_REQUIRED: sending Telegram messages requires auto-approve to be enabled for this integration (MCP Servers settings)")
	}

	messageID, err := s.sendMessage(ctx, text)
	if err != nil {
		return nil, err
	}
	return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", s.chatID, messageID)), nil
}

// callToolViaLoopback posts send_message to the main FlowPilot runner
// (BUG-281). The child never touches keyring or the bot token.
func (s *telegramProxyMcpServer) callToolViaLoopback(ctx context.Context, text string) (map[string]any, error) {
	if strings.TrimSpace(s.loopbackBaseURL) == "" || strings.TrimSpace(s.loopbackToken) == "" {
		return nil, fmt.Errorf("MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send (loopback env incomplete; refusing direct keyring fallback in provider mode)")
	}

	resp, err := postTelegramLoopbackSend(ctx, s.loopbackBaseURL, s.loopbackToken, telegramLoopbackSendRequest{Text: text}, s.client)
	if err != nil {
		return nil, err
	}

	if resp.ErrorCode == "MCP_TOOL_APPROVAL_REQUIRED" {
		msg := strings.TrimSpace(resp.Error)
		if msg == "" {
			msg = "MCP_TOOL_APPROVAL_REQUIRED: sending Telegram messages requires auto-approve to be enabled for this integration (MCP Servers settings)"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("telegram loopback send failed")
	}

	chatID := strings.TrimSpace(resp.ChatID)
	if chatID == "" {
		chatID = "configured-chat"
	}
	if resp.MessageID == 0 {
		return nil, fmt.Errorf("telegram sendMessage response did not include a message_id")
	}
	return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", chatID, resp.MessageID)), nil
}

// telegramSendMessageResponse mirrors the Telegram Bot API's sendMessage
// response shape (https://core.telegram.org/bots/api#sendmessage): a
// successful call's result.message_id is the hard evidence Task-233's verify
// gate looks for — a tool call alone doesn't prove Telegram accepted it.
type telegramSendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

func (s *telegramProxyMcpServer) sendMessage(ctx context.Context, text string) (int64, error) {
	body, err := json.Marshal(map[string]string{"chat_id": s.chatID, "text": text})
	if err != nil {
		return 0, fmt.Errorf("marshal sendMessage body: %w", err)
	}

	url := telegramBotAPIBase + "/bot" + s.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("telegram sendMessage request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("read sendMessage response: %w", err)
	}

	var parsed telegramSendMessageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, fmt.Errorf("parse sendMessage response: %w", err)
	}
	if !parsed.OK {
		desc := strings.TrimSpace(parsed.Description)
		if desc == "" {
			desc = fmt.Sprintf("telegram API HTTP %d", resp.StatusCode)
		}
		return 0, fmt.Errorf("telegram sendMessage failed: %s", desc)
	}
	if parsed.Result.MessageID == 0 {
		return 0, fmt.Errorf("telegram sendMessage response did not include a message_id")
	}
	return parsed.Result.MessageID, nil
}
