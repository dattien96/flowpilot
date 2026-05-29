package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var commandContextFn = exec.CommandContext
var lookupProcessNameFn = lookupProcessName
var killProcessByPIDFn = func(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

type LiveSession struct {
	SessionID         string
	WorkflowRunID     string
	Provider          string
	Model             string
	ReasoningEffort   string
	TransportType     string
	ProviderSessionID string
	ProcessKey        string
	Pid               int
	BinaryPath        string
	WorkingDirectory  string
	Cmd               *exec.Cmd
	Stdin             io.WriteCloser
	StdoutScanner     *bufio.Scanner
	Stdout            io.ReadCloser
	Stderr            io.ReadCloser
	Status            string
	LastUsedAt        time.Time
	IdleTTL           time.Duration
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
		return nil, fmt.Errorf("parse_error: %w", err)
	}
	return msg, nil
}

func readJsonRpcResponse(scanner *bufio.Scanner, expectedID interface{}) (map[string]interface{}, error) {
	return readJsonRpcResponseWithHandler(scanner, expectedID, nil)
}

func readJsonRpcResponseWithHandler(
	scanner *bufio.Scanner,
	expectedID interface{},
	handler func(map[string]interface{}),
) (map[string]interface{}, error) {
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

		if handler != nil {
			handler(msg)
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

func jsonRpcErrorMessage(message map[string]interface{}) string {
	if errObj, ok := message["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return msg
		}
	}

	if errStr, ok := message["error"].(string); ok && strings.TrimSpace(errStr) != "" {
		return errStr
	}

	return ""
}

func geminiACPInitializeParams() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": 1,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "flowpilot",
			"version": "1.0",
		},
	}
}

func geminiACPSessionNewParams(cwd string) map[string]interface{} {
	return map[string]interface{}{
		"cwd":        cwd,
		"mcpServers": []interface{}{},
	}
}

func geminiACPPromptParams(sessionID string, prompt string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{
				"type": "text",
				"text": prompt,
			},
		},
	}
}

func appendGeminiACPText(output *strings.Builder, message map[string]interface{}) {
	if output == nil {
		return
	}

	method, _ := message["method"].(string)
	if method != "session/update" {
		return
	}

	params, ok := message["params"].(map[string]interface{})
	if !ok {
		return
	}

	update, ok := params["update"].(map[string]interface{})
	if !ok {
		return
	}

	if updateType, _ := update["sessionUpdate"].(string); updateType != "agent_message_chunk" {
		return
	}

	content, ok := update["content"].(map[string]interface{})
	if !ok {
		return
	}

	if contentType, _ := content["type"].(string); contentType != "text" {
		return
	}

	if text, ok := content["text"].(string); ok {
		output.WriteString(text)
	}
}

func readClaudeStreamResult(output []byte) (map[string]interface{}, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lastResult map[string]interface{}
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("parse_error: %w", err)
		}

		msgType, _ := msg["type"].(string)
		if msgType == "result" {
			lastResult = msg
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if lastResult == nil {
		return nil, io.EOF
	}

	return lastResult, nil
}

func resolveBinaryAndArgs(provider string, model string, reasoningEffort string) (string, []string, string) {
	switch provider {
	case "codex":
		return "codex", []string{"mcp-server"}, "codex_mcp"
	case "claude":
		args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "default"}
		if model != "" {
			args = append(args, "--model", normalizeClaudeModelName(model))
		}
		if reasoningEffort != "" {
			args = append(args, "--effort", normalizeClaudeEffort(reasoningEffort))
		}
		return "claude", args, "claude_stream_json"
	case "gemini":
		args := []string{"--acp"}
		if model != "" {
			args = append(args, "--model", normalizeGeminiModelName(model))
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

	workingDirectory := strings.TrimSpace(req.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory = r.workspace
	}

	resolvedWorkingDirectory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return AiSessionHandle{}, fmt.Errorf("failed to resolve working directory: %w", err)
	}

	processKey := newRunID()
	session := &LiveSession{
		SessionID:        newRunID(),
		Provider:         req.ProviderKey,
		Model:            req.ModelName,
		ReasoningEffort:  reasoningEffortVal,
		TransportType:    transportType,
		ProcessKey:       processKey,
		BinaryPath:       binaryPath,
		WorkingDirectory: resolvedWorkingDirectory,
		Status:           "active",
		LastUsedAt:       time.Now().UTC(),
	}

	if req.IdleTTLSeconds != nil && *req.IdleTTLSeconds > 0 {
		session.IdleTTL = time.Duration(*req.IdleTTLSeconds) * time.Second
	} else {
		session.IdleTTL = 2 * time.Hour
	}

	if transportType == "claude_stream_json" {
		providerSessionID := DetermineProviderSessionID(
			transportType,
			session.Provider,
			session.SessionID,
			"",
			req.ResumeProviderSessionID,
		)
		session.ProviderSessionID = providerSessionID
		r.sessions[processKey] = session
		return AiSessionHandle{
			TransportType:     transportType,
			ProviderSessionID: providerSessionID,
			ProcessKey:        &processKey,
		}, nil
	}

	cmd := commandContextFn(ctx, binaryPath, args...)
	cmd.Dir = resolvedWorkingDirectory

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
	session.Cmd = cmd
	session.Stdin = stdin
	session.StdoutScanner = stdoutScanner
	session.Stdout = stdout
	session.Stderr = stderr

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
		if err := writeJsonRpcRequest(stdin, "initialize", geminiACPInitializeParams(), 1); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write ACP initialize request: %w", err)
		}
		resp, err := readJsonRpcResponse(stdoutScanner, 1)
		if err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to read ACP initialize response: %w", err)
		}
		if errMsg := jsonRpcErrorMessage(resp); errMsg != "" {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed Gemini ACP initialize: %s", errMsg)
		}
		if err := writeJsonRpcRequest(stdin, "session/new", geminiACPSessionNewParams(resolvedWorkingDirectory), 2); err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to write session/new request: %w", err)
		}
		resp, err = readJsonRpcResponse(stdoutScanner, 2)
		if err != nil {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed to read session/new response: %w", err)
		}
		if errMsg := jsonRpcErrorMessage(resp); errMsg != "" {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed Gemini ACP session/new: %s", errMsg)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if sId, ok := result["sessionId"].(string); ok {
				providerSessionID = sId
			}
		}
		if providerSessionID == "" {
			cmd.Process.Kill()
			return AiSessionHandle{}, fmt.Errorf("failed Gemini ACP session/new: missing sessionId")
		}
	}
	providerSessionID = DetermineProviderSessionID(transportType, session.Provider, session.SessionID, providerSessionID, req.ResumeProviderSessionID)
	session.ProviderSessionID = providerSessionID
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	session.Pid = pid
	r.sessions[processKey] = session

	return AiSessionHandle{
		TransportType:     transportType,
		ProviderSessionID: providerSessionID,
		ProcessKey:        &processKey,
		ProcessPid:        &pid,
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
		return PromptExecutionResult{}, fmt.Errorf("session_dead: session %q not found or expired", *req.Session.ProcessKey)
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	session.LastUsedAt = time.Now().UTC()
	if req.IdleTTLSeconds != nil && *req.IdleTTLSeconds > 0 {
		session.IdleTTL = time.Duration(*req.IdleTTLSeconds) * time.Second
	}

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
			return PromptExecutionResult{}, fmt.Errorf("session_dead: failed to send MCP tool call: %w", err)
		}
		resp, err := readJsonRpcResponse(session.StdoutScanner, 3)
		if err != nil {
			if strings.HasPrefix(err.Error(), "parse_error:") {
				return PromptExecutionResult{}, fmt.Errorf("provider error: invalid json response: %v", err)
			}
			return PromptExecutionResult{}, fmt.Errorf("session_dead: failed to read MCP tool response: %w", err)
		}
		if errObj, ok := resp["error"].(map[string]interface{}); ok {
			errMsg := "unknown provider error"
			if msg, ok := errObj["message"].(string); ok {
				errMsg = msg
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: %s", errMsg)
		} else if errStr, ok := resp["error"].(string); ok {
			return PromptExecutionResult{}, fmt.Errorf("provider error: %s", errStr)
		}
		if result, ok := resp["result"].(map[string]interface{}); ok {
			threadID, output := extractCodexMcpResponse(result)
			outputMarkdown = output
			if threadID != "" {
				session.ProviderSessionID = threadID
			}
		}
	} else if session.TransportType == "gemini_acp" {
		params := geminiACPPromptParams(session.ProviderSessionID, req.Prompt)
		if err := writeJsonRpcRequest(session.Stdin, "session/prompt", params, 3); err != nil {
			return PromptExecutionResult{}, fmt.Errorf("session_dead: failed to send Gemini ACP prompt: %w", err)
		}
		var streamedOutput strings.Builder
		resp, err := readJsonRpcResponseWithHandler(session.StdoutScanner, 3, func(msg map[string]interface{}) {
			appendGeminiACPText(&streamedOutput, msg)
		})
		if err != nil {
			if strings.HasPrefix(err.Error(), "parse_error:") {
				return PromptExecutionResult{}, fmt.Errorf("provider error: invalid json response: %v", err)
			}
			return PromptExecutionResult{}, fmt.Errorf("session_dead: failed to read Gemini ACP response: %w", err)
		}
		if errObj, ok := resp["error"].(map[string]interface{}); ok {
			errMsg := "unknown provider error"
			if msg, ok := errObj["message"].(string); ok {
				errMsg = msg
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: %s", errMsg)
		} else if errStr, ok := resp["error"].(string); ok {
			return PromptExecutionResult{}, fmt.Errorf("provider error: %s", errStr)
		}
		outputMarkdown = streamedOutput.String()
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if outputMarkdown == "" {
				if text, ok := result["text"].(string); ok {
					outputMarkdown = text
				}
			}
			if outputMarkdown == "" {
				if content, ok := result["content"].([]interface{}); ok {
					for _, entry := range content {
						block, ok := entry.(map[string]interface{})
						if !ok {
							continue
						}
						if blockType, _ := block["type"].(string); blockType != "text" {
							continue
						}
						if text, ok := block["text"].(string); ok {
							outputMarkdown += text
						}
					}
				}
			}
			if providerSessionID, ok := result["sessionId"].(string); ok && strings.TrimSpace(providerSessionID) != "" {
				session.ProviderSessionID = providerSessionID
			}
			if text, ok := result["text"].(string); ok && outputMarkdown == "" {
				outputMarkdown = text
			}
		}
	} else if session.TransportType == "claude_stream_json" {
		args := buildClaudePrintArgs(session.Model, session.ReasoningEffort, session.ProviderSessionID)
		cmd := commandContextFn(ctx, session.BinaryPath, args...)
		cmd.Dir = session.WorkingDirectory

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to create Claude stdin pipe: %w", err)
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to start Claude print command: %w", err)
		}

		msg := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "text",
						"text": req.Prompt,
					},
				},
			},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return PromptExecutionResult{}, err
		}

		if _, err := stdin.Write(append(data, '\n')); err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to write Claude stream JSON: %w", err)
		}
		if err := stdin.Close(); err != nil {
			_ = cmd.Wait()
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to close Claude stdin: %w", err)
		}

		if err := cmd.Wait(); err != nil {
			stderrText := strings.TrimSpace(stderr.String())
			if stderrText != "" {
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", stderrText)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude print command failed: %w", err)
		}

		outputBytes := bytes.TrimSpace(stdout.Bytes())
		if len(outputBytes) == 0 {
			if stderrText := strings.TrimSpace(stderr.String()); stderrText != "" {
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", stderrText)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude response was empty")
		}

		resp, err := readClaudeStreamResult(outputBytes)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return PromptExecutionResult{}, fmt.Errorf("provider error: Claude response did not include a result event")
			}
			if strings.HasPrefix(err.Error(), "parse_error:") {
				return PromptExecutionResult{}, fmt.Errorf("provider error: invalid json response: %v", err)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to parse Claude stream response: %v", err)
		}

		if isError, _ := resp["is_error"].(bool); isError {
			if resultText, ok := resp["result"].(string); ok && strings.TrimSpace(resultText) != "" {
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", resultText)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude request failed")
		}

		if resultText, ok := resp["result"].(string); ok {
			outputMarkdown = resultText
		}
		if sID, ok := resp["session_id"].(string); ok && strings.TrimSpace(sID) != "" {
			session.ProviderSessionID = sID
		}
		if outputMarkdown == "" {
			if text, ok := resp["text"].(string); ok {
				outputMarkdown = text
			}
		}
		if outputMarkdown == "" {
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude response did not include any text output")
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
		if handle.ProcessPid != nil {
			processName, err := lookupProcessNameFn(ctx, *handle.ProcessPid)
			if err == nil && processNameMatchesTransport(processName, handle.TransportType) {
				_ = killProcessByPIDFn(*handle.ProcessPid)
			}
		}
		return nil
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	session.Status = "completed"

	if session.Stdin != nil {
		session.Stdin.Close()
	}
	if session.Cmd == nil {
		return nil
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

func (r *Runner) ListSessions() []AiSessionHandle {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	sessions := make([]AiSessionHandle, 0, len(r.sessions))
	for processKey, session := range r.sessions {
		processKeyCopy := processKey
		processPid := session.Pid
		sessions = append(sessions, AiSessionHandle{
			TransportType:     session.TransportType,
			ProviderSessionID: session.ProviderSessionID,
			ProcessKey:        &processKeyCopy,
			ProcessPid:        &processPid,
		})
	}

	return sessions
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
		if session.Cmd != nil {
			if session.Cmd.Process != nil {
				session.Cmd.Process.Kill()
			}
			go session.Cmd.Wait()
		}
		session.Mu.Unlock()
	}
}

func (r *Runner) StartIdleSweeper(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.sweepIdleSessions()
			}
		}
	}()
}

func (r *Runner) sweepIdleSessions() {
	r.sessionsMu.Lock()
	var staleSessions []*LiveSession
	var staleKeys []string
	for key, sess := range r.sessions {
		sess.Mu.Lock()
		idle := time.Since(sess.LastUsedAt) > sess.IdleTTL
		sess.Mu.Unlock()
		if idle {
			staleKeys = append(staleKeys, key)
			staleSessions = append(staleSessions, sess)
		}
	}
	for _, key := range staleKeys {
		delete(r.sessions, key)
	}
	r.sessionsMu.Unlock()

	for _, session := range staleSessions {
		session.Mu.Lock()
		session.Status = "completed"
		if session.Stdin != nil {
			session.Stdin.Close()
		}
		if session.Cmd != nil && session.Cmd.Process != nil {
			session.Cmd.Process.Kill()
			go session.Cmd.Wait()
		}
		session.Mu.Unlock()
	}
}

func DetermineProviderSessionID(transportType, provider, sessionID, currentProviderSessionID string, resumeProviderSessionID *string) string {
	if transportType == "gemini_acp" {
		if currentProviderSessionID == "" {
			currentProviderSessionID = "gemini_acp_session_" + sessionID
		}
	} else if transportType == "claude_stream_json" {
		currentProviderSessionID = "claude_stream_session_" + sessionID
	}

	if resumeProviderSessionID != nil && *resumeProviderSessionID != "" && (provider == "codex" || provider == "claude") {
		currentProviderSessionID = *resumeProviderSessionID
	}
	return currentProviderSessionID
}

func normalizeClaudeEffort(reasoningEffort string) string {
	normalized := strings.ToLower(strings.TrimSpace(reasoningEffort))
	if normalized == "xhigh" {
		return "max"
	}
	return normalized
}

func normalizeClaudeModelName(model string) string {
	trimmed := strings.TrimSpace(model)
	lowerModel := strings.ToLower(trimmed)
	if strings.HasPrefix(lowerModel, "claude-") {
		return strings.TrimPrefix(lowerModel, "claude-")
	}
	return trimmed
}

func isSyntheticClaudeSessionID(providerSessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(providerSessionID), "claude_stream_session_")
}

func buildClaudePrintArgs(model string, reasoningEffort string, providerSessionID string) []string {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "default"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", normalizeClaudeModelName(model))
	}
	if effort := normalizeClaudeEffort(reasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if strings.TrimSpace(providerSessionID) != "" && !isSyntheticClaudeSessionID(providerSessionID) {
		args = append(args, "--resume", providerSessionID)
	}
	return args
}

func expectedProcessNamesForTransport(transportType string) []string {
	switch transportType {
	case "codex_mcp":
		return []string{"codex", "codex.exe"}
	case "claude_stream_json":
		return []string{"claude", "claude.exe"}
	case "gemini_acp":
		return []string{"gemini", "gemini.exe"}
	default:
		return nil
	}
}

func processNameMatchesTransport(processName string, transportType string) bool {
	normalizedProcessName := strings.ToLower(strings.TrimSpace(processName))
	if normalizedProcessName == "" {
		return false
	}

	for _, expectedName := range expectedProcessNamesForTransport(transportType) {
		if normalizedProcessName == expectedName {
			return true
		}
	}

	return false
}

func lookupProcessName(ctx context.Context, pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid pid %d", pid)
	}

	if strings.EqualFold(runtimeGOOS(), "windows") {
		output, err := runCommandFn(
			ctx,
			"tasklist",
			"/FI",
			fmt.Sprintf("PID eq %d", pid),
			"/FO",
			"CSV",
			"/NH",
		)
		if err != nil {
			return "", err
		}

		line := strings.TrimSpace(string(output))
		if line == "" || strings.Contains(strings.ToLower(line), "no tasks are running") {
			return "", fmt.Errorf("process %d not found", pid)
		}

		record, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			return "", err
		}
		if len(record) == 0 {
			return "", fmt.Errorf("process %d not found", pid)
		}

		return strings.ToLower(strings.TrimSpace(record[0])), nil
	}

	output, err := runCommandFn(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	if err != nil {
		return "", err
	}

	processName := strings.ToLower(strings.TrimSpace(string(output)))
	if processName == "" {
		return "", fmt.Errorf("process %d not found", pid)
	}

	return processName, nil
}

func runtimeGOOS() string {
	return strings.ToLower(strings.TrimSpace(runtime.GOOS))
}
