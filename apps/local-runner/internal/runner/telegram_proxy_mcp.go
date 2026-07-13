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
	runner   *Runner

	// autoApprove is the static fallback gate used only when no run/step/
	// process scope is available (mirrors proxyMcpServer.hasNoApprovalScope's
	// auto-execute path) — sending a Telegram message is an irreversible,
	// outward-facing action, so out-of-scope calls still default to refuse.
	autoApprove bool

	// workflowRunID/workflowStepRunID/processKey scope this proxy instance to
	// one run (Task-233 DOD-6 revisit): when all three are present, callTool
	// uses the real live approval queue (telegram_proxy_approval.go, mirrors
	// google_drive_proxy_approval.go exactly) instead of the static
	// autoApprove flag — the AI's send_message call is refused with a
	// pending-approval id on first attempt, and only proceeds once a human
	// approves it via the runner's HTTP API/desktop UI and the AI retries the
	// identical call.
	workflowRunID     string
	workflowStepRunID string
	processKey        string
}

// telegramBotAPIBase is a var (not const), matching the executeJiraRequestFn
// seam pattern in this codebase, so tests can point it at an httptest server
// instead of the real Telegram API.
var telegramBotAPIBase = "https://api.telegram.org"

// RunTelegramProxyMcpServer launches the stdio server, resolving the
// connected Telegram credential from the runner keyring itself (mirrors
// RunGoogleDriveProxyMcpServer's shape/lifecycle) — the bot token never
// travels through provider config or process args, only this in-process
// resolution. Run-scope env vars (same constants Google Drive's proxy
// defines) are read here too, set by EnsureClaudeTelegramMcpConfig's
// per-turn config merge (flowpilotClaudeExtraMCPServers).
func (r *Runner) RunTelegramProxyMcpServer(ctx context.Context) error {
	creds, err := r.resolveConnectedTelegramCredential()
	if err != nil {
		return fmt.Errorf("telegram-mcp: %w", err)
	}
	server := &telegramProxyMcpServer{
		botToken:          strings.TrimSpace(creds.BotToken),
		chatID:            strings.TrimSpace(creds.ChannelID),
		stdin:             bufio.NewScanner(os.Stdin),
		stdout:            os.Stdout,
		client:            &http.Client{Timeout: 15 * time.Second},
		runner:            r,
		autoApprove:       creds.AutoApprove,
		workflowRunID:     strings.TrimSpace(os.Getenv(googleDriveProxyWorkflowRunIDEnv)),
		workflowStepRunID: strings.TrimSpace(os.Getenv(googleDriveProxyWorkflowStepIDEnv)),
		processKey:        strings.TrimSpace(os.Getenv(googleDriveProxyProcessKeyEnv)),
	}
	server.stdin.Buffer(make([]byte, 64*1024), 10*1024*1024)
	return server.serve(ctx)
}

// hasApprovalScope mirrors proxyMcpServer.hasApprovalScope.
func (s *telegramProxyMcpServer) hasApprovalScope() bool {
	return strings.TrimSpace(s.workflowRunID) != "" &&
		strings.TrimSpace(s.workflowStepRunID) != "" &&
		strings.TrimSpace(s.processKey) != ""
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

	if s.hasApprovalScope() {
		return s.callToolWithApprovalQueue(ctx, text)
	}

	// No run/step/process scope (e.g. a manual/offline invocation) — fall
	// back to the static autoApprove posture flag.
	if !s.autoApprove {
		return nil, fmt.Errorf("MCP_TOOL_APPROVAL_REQUIRED: sending Telegram messages requires auto-approve to be enabled for this integration (MCP Servers settings) — this run was not approved to send")
	}
	messageID, err := s.sendMessage(ctx, text)
	if err != nil {
		return nil, err
	}
	return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", s.chatID, messageID)), nil
}

// callToolWithApprovalQueue is the live, per-send human-approval path
// (Task-233 DOD-6 revisit): mirrors proxyMcpServer.handleWriteTool's
// pending/rejected/executed/approved state machine exactly. The AI's first
// call for a given (run, step, process, chat, text) tuple always comes back
// "pending" — it must stop and wait; the identical retry after a human
// decision either executes for real (approved) or reports a clean rejection
// (rejected), and a third identical call after execution just replays the
// already-recorded result instead of sending twice.
func (s *telegramProxyMcpServer) callToolWithApprovalQueue(ctx context.Context, text string) (map[string]any, error) {
	record, err := s.resolveTelegramToolApproval(s.chatID, text)
	if err != nil {
		return nil, err
	}

	switch record.Status {
	case "pending":
		return nil, fmt.Errorf(
			"MCP_TOOL_APPROVAL_REQUIRED: FlowPilot created approval request %s. Wait for user approval before retrying this exact send_message call.",
			record.ID,
		)
	case "rejected":
		return textToolResult(fmt.Sprintf("Message to chat %s was rejected by the user and was not sent.", s.chatID)), nil
	case "executed":
		return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", s.chatID, record.ResultMessageID)), nil
	case "approved":
		messageID, err := s.sendMessage(ctx, text)
		if err != nil {
			return nil, s.markTelegramApprovalFailed(record, err)
		}
		if err := s.markTelegramApprovalExecuted(record, messageID); err != nil {
			return nil, err
		}
		return textToolResult(fmt.Sprintf("Message sent to Telegram chat %s. message_id: %d", s.chatID, messageID)), nil
	default:
		return nil, fmt.Errorf("unsupported approval status %q", record.Status)
	}
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
