package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

var commandContextFn = exec.CommandContext

type LiveSession struct {
	SessionID         string
	WorkflowRunID     string
	Provider          string
	Model             string
	TransportType     string
	ProviderSessionID string
	ProcessKey        string
	Cmd               *exec.Cmd
	Stdin             io.WriteCloser
	StdoutScanner     *bufio.Scanner
	Stdout            io.ReadCloser
	Stderr            io.ReadCloser
	Status            string
	Mu                sync.Mutex
}

func writeJsonRpcRequest(w io.Writer, method string, params interface{}, id interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	if id != nil {
		req["id"] = id
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func readJsonRpcMessage(scanner *bufio.Scanner) (map[string]interface{}, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func readJsonRpcResponse(scanner *bufio.Scanner, expectedID interface{}) (map[string]interface{}, error) {
	for {
		msg, err := readJsonRpcMessage(scanner)
		if err != nil {
			return nil, err
		}

		if expectedID == nil {
			return msg, nil
		}

		if msgID, ok := msg["id"]; ok && jsonRpcIDsEqual(msgID, expectedID) {
			return msg, nil
		}
	}
}

func jsonRpcIDsEqual(left interface{}, right interface{}) bool {
	switch leftValue := left.(type) {
	case float64:
		switch rightValue := right.(type) {
		case int:
			return leftValue == float64(rightValue)
		case int64:
			return leftValue == float64(rightValue)
		case float64:
			return leftValue == rightValue
		}
	case int:
		switch rightValue := right.(type) {
		case int:
			return leftValue == rightValue
		case int64:
			return int64(leftValue) == rightValue
		case float64:
			return float64(leftValue) == rightValue
		}
	case int64:
		switch rightValue := right.(type) {
		case int:
			return leftValue == int64(rightValue)
		case int64:
			return leftValue == rightValue
		case float64:
			return float64(leftValue) == rightValue
		}
	case string:
		if rightValue, ok := right.(string); ok {
			return leftValue == rightValue
		}
	}

	return false
}

func extractCodexMcpResponse(result map[string]interface{}) (string, string) {
	threadID := ""
	if value, ok := result["threadId"].(string); ok {
		threadID = value
	}

	output := ""
	if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]interface{}); ok {
			if text, ok := first["text"].(string); ok {
				output = text
			}
		}
	}

	if structuredContent, ok := result["structuredContent"].(map[string]interface{}); ok {
		if threadID == "" {
			if value, ok := structuredContent["threadId"].(string); ok {
				threadID = value
			}
		}
		if output == "" {
			if text, ok := structuredContent["content"].(string); ok {
				output = text
			}
		}
	}

	return threadID, output
}

func resolveBinaryAndArgs(provider string, model string, reasoningEffort string) (string, []string, string) {
	switch provider {
	case "codex":
		return "codex", []string{"mcp-server"}, "codex_mcp"
	case "claude":
		args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "default"}
		if model != "" {
			args = append(args, "--model", model)
		}
		if reasoningEffort != "" {
			args = append(args, "--effort", reasoningEffort)
		}
		return "claude", args, "claude_stream_json"
	case "gemini":
		args := []string{"--acp"}
		if model != "" {
			args = append(args, "--model", model)
		}
		return "gemini", args, "gemini_acp"
	default:
		return "", nil, ""
	}
}

func (r *Runner) StartSession(ctx context.Context, req AiSessionStartRequest) (AiSessionHandle, error) {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	reasoningEffortVal := ""
	if req.ReasoningEffort != nil {
		reasoningEffortVal = *req.ReasoningEffort
	}
	binary, args, transportType := resolveBinaryAndArgs(req.ProviderKey, req.ModelName, reasoningEffortVal)
	if binary == "" {
		return AiSessionHandle{}, fmt.Errorf("unsupported provider %q", req.ProviderKey)
	}

	binaryPath, err := lookPathFn(binary)
	if err != nil {
		return AiSessionHandle{}, fmt.Errorf("provider binary %q not found: %w", binary, err)
	}

	cmd := commandContextFn(ctx, binaryPath, args...)
	cmd.Dir = req.WorkingDirectory

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return AiSessionHandle{}, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return AiSessionHandle{}, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return AiSessionHandle{}, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return AiSessionHandle{}, fmt.Errorf("failed to start process: %w", err)
	}

	stdoutScanner := bufio.NewScanner(stdout)
	// Buffer allocation allowing up to 10 MB responses
	buf := make([]byte, 64*1024)
	stdoutScanner.Buffer(buf, 10*1024*1024)
	processKey := newRunID()

	session := &LiveSession{
		SessionID:     newRunID(),
		Provider:      req.ProviderKey,
		Model:         req.ModelName,
		TransportType: transportType,
		ProcessKey:    processKey,
		Cmd:           cmd,
		Stdin:         stdin,
		StdoutScanner: stdoutScanner,
		Stdout:        stdout,
		Stderr:        stderr,
		Status:        "active",
	}

	providerSessionID := ""
	if transportType == "codex_mcp" {
		initParams := map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "flowpilot",
				"version": "1.0",
			},
		}
		if err := writeJsonRpcRequest(stdin, "initialize", initParams, 1); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write initialize request: %w", err)
		}
		resp, err := readJsonRpcResponse(stdoutScanner, 1)
		if err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to read initialize response: %w", err)
		}
		if err := writeJsonRpcRequest(stdin, "notifications/initialized", nil, nil); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write initialized notification: %w", err)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if threadId, ok := result["threadId"].(string); ok {
				providerSessionID = threadId
			}
		}
		if providerSessionID == "" {
			providerSessionID = "codex_mcp_session_" + session.SessionID
		}
	} else if transportType == "gemini_acp" {
		if err := writeJsonRpcRequest(stdin, "initialize", map[string]interface{}{}, 1); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write ACP initialize request: %w", err)
		}
		_, err := readJsonRpcResponse(stdoutScanner, 1)
		if err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to read ACP initialize response: %w", err)
		}
		if err := writeJsonRpcRequest(stdin, "session/new", map[string]interface{}{}, 2); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write session/new request: %w", err)
		}
		resp, err := readJsonRpcResponse(stdoutScanner, 2)
		if err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to read session/new response: %w", err)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if sId, ok := result["sessionId"].(string); ok {
				providerSessionID = sId
			}
		}
		if providerSessionID == "" {
			providerSessionID = "gemini_acp_session_" + session.SessionID
		}
	} else if transportType == "claude_stream_json" {
		providerSessionID = "claude_stream_session_" + session.SessionID
	}

	session.ProviderSessionID = providerSessionID
	r.sessions[processKey] = session

	return AiSessionHandle{
		TransportType:     transportType,
		ProviderSessionID: providerSessionID,
		ProcessKey:        &processKey,
	}, nil
}

func (r *Runner) SendMessage(ctx context.Context, req AiSessionMessageRequest) (PromptExecutionResult, error) {
	if req.Session.ProcessKey == nil {
		return PromptExecutionResult{}, fmt.Errorf("session process key is required")
	}

	r.sessionsMu.Lock()
	session, exists := r.sessions[*req.Session.ProcessKey]
	r.sessionsMu.Unlock()

	if !exists {
		return PromptExecutionResult{}, fmt.Errorf("session %q not found or expired", *req.Session.ProcessKey)
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	startedAt := time.Now().UTC()
	var outputMarkdown string

	if session.TransportType == "codex_mcp" {
		toolName := "codex"
		args := map[string]interface{}{
			"prompt": req.Prompt,
		}
		if session.ProviderSessionID != "" && session.ProviderSessionID != "codex_mcp_session_"+session.SessionID {
			toolName = "codex-reply"
			args["threadId"] = session.ProviderSessionID
		}
		params := map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		}
		if err := writeJsonRpcRequest(session.Stdin, "tools/call", params, 3); err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to send MCP tool call: %w", err)
		}
		resp, err := readJsonRpcResponse(session.StdoutScanner, 3)
		if err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to read MCP tool response: %w", err)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			threadID, output := extractCodexMcpResponse(result)
			outputMarkdown = output
			if threadID != "" {
				session.ProviderSessionID = threadID
			}
		}
	} else if session.TransportType == "gemini_acp" {
		params := map[string]interface{}{
			"sessionId": session.ProviderSessionID,
			"prompt":    req.Prompt,
		}
		if err := writeJsonRpcRequest(session.Stdin, "session/prompt", params, 3); err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to send Gemini ACP prompt: %w", err)
		}
		resp, err := readJsonRpcResponse(session.StdoutScanner, 3)
		if err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to read Gemini ACP response: %w", err)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if text, ok := result["text"].(string); ok {
				outputMarkdown = text
			}
		}
	} else if session.TransportType == "claude_stream_json" {
		msg := map[string]interface{}{
			"prompt": req.Prompt,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return PromptExecutionResult{}, err
		}
		if _, err := session.Stdin.Write(append(data, '\n')); err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to write Claude stream JSON: %w", err)
		}
		resp, err := readJsonRpcResponse(session.StdoutScanner, nil)
		if err != nil {
			return PromptExecutionResult{}, fmt.Errorf("failed to read Claude stream response: %w", err)
		}
		if text, ok := resp["text"].(string); ok {
			outputMarkdown = text
		} else if result, ok := resp["result"].(map[string]interface{}); ok {
			if text, ok := result["text"].(string); ok {
				outputMarkdown = text
			}
			if sId, ok := result["session_id"].(string); ok {
				session.ProviderSessionID = sId
			}
		}
	}

	completedAt := time.Now().UTC()

	return PromptExecutionResult{
		Status:            "success",
		RunID:             newRunID(),
		ProviderKey:       session.Provider,
		ModelName:         &session.Model,
		ProviderSessionID: session.ProviderSessionID,
		Command:           session.Provider + " session message",
		OutputMarkdown:    outputMarkdown,
		StartedAt:         startedAt.Format(time.RFC3339Nano),
		CompletedAt:       completedAt.Format(time.RFC3339Nano),
	}, nil
}

func (r *Runner) CloseSession(ctx context.Context, handle AiSessionHandle) error {
	if handle.ProcessKey == nil {
		return nil
	}

	r.sessionsMu.Lock()
	session, exists := r.sessions[*handle.ProcessKey]
	delete(r.sessions, *handle.ProcessKey)
	r.sessionsMu.Unlock()

	if !exists {
		return nil
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	session.Status = "completed"

	if session.Stdin != nil {
		session.Stdin.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		if session.Cmd.Process != nil {
			session.Cmd.Process.Kill()
		}
	}

	return nil
}

func (r *Runner) CleanupSessions() {
	r.sessionsMu.Lock()
	activeSessions := make([]*LiveSession, 0, len(r.sessions))
	for _, sess := range r.sessions {
		activeSessions = append(activeSessions, sess)
	}
	r.sessions = make(map[string]*LiveSession)
	r.sessionsMu.Unlock()

	for _, session := range activeSessions {
		session.Mu.Lock()
		if session.Stdin != nil {
			session.Stdin.Close()
		}
		if session.Cmd != nil && session.Cmd.Process != nil {
			session.Cmd.Process.Kill()
		}
		session.Mu.Unlock()
	}
}
