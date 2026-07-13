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

	// autoApprove gates the actual send (Task-233 P-5): sending a Telegram
	// message is an irreversible, outward-facing action, so the default is
	// to REFUSE the send and return an explicit, actionable error rather
	// than silently succeeding. This is a v1, config-driven approval gate —
	// a coarse per-connection posture flag, mirroring the *level* of Google
	// Drive's yolo-mode flag (google_drive_proxy_mcp.go), not a live
	// per-message human-approval round-trip: that would require this
	// separately-spawned proxy process to call back into the running
	// InteractiveService's question mechanism (AskWorkflowQuestion) over
	// HTTP, which is real additional design/plumbing this task does not
	// build. Documented as a follow-up, not silently skipped.
	autoApprove bool
}

// telegramBotAPIBase is a var (not const), matching the executeJiraRequestFn
// seam pattern in this codebase, so tests can point it at an httptest server
// instead of the real Telegram API.
var telegramBotAPIBase = "https://api.telegram.org"

// RunTelegramProxyMcpServer launches the stdio server, resolving the
// connected Telegram credential from the runner keyring itself (mirrors
// RunGoogleDriveProxyMcpServer's shape/lifecycle) — the bot token never
// travels through provider config or process args, only this in-process
// resolution.
func (r *Runner) RunTelegramProxyMcpServer(ctx context.Context) error {
	creds, err := r.resolveConnectedTelegramCredential()
	if err != nil {
		return fmt.Errorf("telegram-mcp: %w", err)
	}
	server := &telegramProxyMcpServer{
		botToken:    strings.TrimSpace(creds.BotToken),
		chatID:      strings.TrimSpace(creds.ChannelID),
		stdin:       bufio.NewScanner(os.Stdin),
		stdout:      os.Stdout,
		client:      &http.Client{Timeout: 15 * time.Second},
		autoApprove: creds.AutoApprove,
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
	if s.botToken == "" || s.chatID == "" {
		return nil, fmt.Errorf("telegram proxy is not configured with a bot token/chat id")
	}
	if !s.autoApprove {
		return nil, fmt.Errorf("MCP_TOOL_APPROVAL_REQUIRED: sending Telegram messages requires auto-approve to be enabled for this integration (MCP Servers settings) — this run was not approved to send")
	}

	messageID, err := s.sendMessage(ctx, text)
	if err != nil {
		return nil, err
	}
	return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", s.chatID, messageID)), nil
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
		return 0, fmt.Errorf("sendMessage request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed telegramSendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("parse sendMessage response: %w", err)
	}
	if !parsed.OK {
		if strings.TrimSpace(parsed.Description) != "" {
			return 0, fmt.Errorf("telegram sendMessage failed: %s", parsed.Description)
		}
		return 0, fmt.Errorf("telegram sendMessage failed with status %d", resp.StatusCode)
	}
	if parsed.Result.MessageID == 0 {
		return 0, fmt.Errorf("telegram sendMessage response did not include a message_id")
	}
	return parsed.Result.MessageID, nil
}
