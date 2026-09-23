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
	"log"
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
	AccountHomePath   string
	CustomEnv         map[string]string
	ProxyURL          string
	TransportType     string
	ProviderSessionID string
	ProcessKey        string
	Pid               int
	BinaryPath        string
	Command           string
	WorkingDirectory  string
	Cmd               *exec.Cmd
	Stdin             io.WriteCloser
	StdoutScanner     *bufio.Scanner
	Stdout            io.ReadCloser
	Stderr            io.ReadCloser
	Status            string
	LastUsedAt        time.Time
	IdleTTL           time.Duration
	TerminationReason string
	Mu                sync.Mutex
	SendMu            sync.Mutex
	InFlight          bool
}

func (r *Runner) googleDriveProxyApprovalDeadline(processKey string) (time.Time, error) {
	trimmedProcessKey := strings.TrimSpace(processKey)
	if trimmedProcessKey == "" {
		return time.Time{}, errors.New("process key is required for Google Drive proxy approvals")
	}

	r.sessionsMu.Lock()
	session, exists := r.sessions[trimmedProcessKey]
	r.sessionsMu.Unlock()
	if !exists || session == nil {
		return time.Time{}, fmt.Errorf("session_dead: session %q not found or expired", trimmedProcessKey)
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	if session.Status == "completed" {
		if err := sessionTerminationErrorLocked(session); err != nil {
			return time.Time{}, err
		}
		return time.Time{}, fmt.Errorf("session_dead: session %q has been closed", trimmedProcessKey)
	}

	if session.IdleTTL <= 0 {
		return session.LastUsedAt.UTC(), nil
	}

	return session.LastUsedAt.UTC().Add(session.IdleTTL), nil
}

func sessionTerminationError(session *LiveSession) error {
	session.Mu.Lock()
	defer session.Mu.Unlock()

	return sessionTerminationErrorLocked(session)
}

func sessionTerminationErrorLocked(session *LiveSession) error {
	if strings.TrimSpace(session.TerminationReason) == "" {
		return nil
	}

	return fmt.Errorf("session_terminated: session %q was intentionally terminated", session.ProcessKey)
}

func markSessionTerminated(session *LiveSession, reason string) (io.WriteCloser, *exec.Cmd, string) {
	session.Mu.Lock()
	session.Status = "completed"
	if session.TerminationReason == "" {
		session.TerminationReason = reason
	}
	stdin := session.Stdin
	cmd := session.Cmd
	transport := session.TransportType
	session.Mu.Unlock()
	return stdin, cmd, transport
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

func writeJsonRpcResponse(w io.Writer, id interface{}, result interface{}) error {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, err := json.Marshal(resp)
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
		// BUG-381: ACP servers put the informative payload in error.data
		// ({http_status, message}) while top-level error.message is a generic
		// "Internal error". Prefer data.message; surface http_status when the
		// detail string is absent so quota/auth failures never collapse to a
		// bare "Internal error".
		if data, ok := errObj["data"].(map[string]interface{}); ok {
			if msg, ok := data["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return msg
			}
			top, _ := errObj["message"].(string)
			if status, ok := toInt64(data["http_status"]); ok && status > 0 {
				if strings.TrimSpace(top) != "" {
					return fmt.Sprintf("%s (http_status %d)", top, status)
				}
				return fmt.Sprintf("JSON-RPC error (http_status %d)", status)
			}
		}
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			if detail := jsonRpcErrorDataDetail(errObj["data"]); detail != "" && !strings.Contains(msg, detail) {
				return msg + ": " + detail
			}
			return msg
		}
	}

	if errStr, ok := message["error"].(string); ok && strings.TrimSpace(errStr) != "" {
		return errStr
	}

	return ""
}

// jsonRpcErrorDataDetail extracts the human-readable detail from a JSON-RPC
// error's `data` field (spec: "additional information about the error"). Grok
// ACP sends {"http_status":402,"message":"API error (status 402 ...)"} — the
// quota/billing text lives there while `message` is just "Internal error"
// (BUG-374). Returns "" when no usable detail exists.
func jsonRpcErrorDataDetail(data interface{}) string {
	switch v := data.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		for _, key := range []string{"message", "details", "detail", "reason"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func extractClaudeStreamText(line []byte) string {
	var msg map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(line), &msg); err != nil {
		return ""
	}

	if delta, ok := msg["delta"].(map[string]interface{}); ok {
		if text, ok := delta["text"].(string); ok {
			return text
		}
	}
	if text, ok := msg["text"].(string); ok {
		return text
	}
	if message, ok := msg["message"].(map[string]interface{}); ok {
		if content, ok := message["content"].([]interface{}); ok {
			var builder strings.Builder
			for _, entry := range content {
				block, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				if text, ok := block["text"].(string); ok {
					builder.WriteString(text)
				}
			}
			return builder.String()
		}
	}

	return ""
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
		args := []string{"mcp-server"}
		if strings.TrimSpace(model) != "" {
			args = append(args, "-c", fmt.Sprintf("model=%q", strings.TrimSpace(model)))
		}
		if strings.TrimSpace(reasoningEffort) != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%s", strings.ToLower(strings.TrimSpace(reasoningEffort))))
		}
		return "codex", args, "codex_mcp"
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
		return geminiBinaryName(), nil, "gemini_agy"
	default:
		return "", nil, ""
	}
}

func cloneSessionCustomEnv(customEnv map[string]string) map[string]string {
	if len(customEnv) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(customEnv))
	for key, value := range customEnv {
		cloned[key] = value
	}
	return cloned
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
	command := formatProviderCommand(binaryPath, args)

	sessionCustomEnv := cloneSessionCustomEnv(req.CustomEnv)
	processKey := strings.TrimSpace(sessionCustomEnv[googleDriveProxyProcessKeyEnv])
	if processKey == "" {
		processKey = newRunID()
	}
	sessionCustomEnv[googleDriveProxyProcessKeyEnv] = processKey
	session := &LiveSession{
		SessionID:        newRunID(),
		Provider:         req.ProviderKey,
		Model:            req.ModelName,
		ReasoningEffort:  reasoningEffortVal,
		AccountHomePath:  req.AccountHomePath,
		CustomEnv:        sessionCustomEnv,
		ProxyURL:         req.ProxyURL,
		TransportType:    transportType,
		ProcessKey:       processKey,
		BinaryPath:       binaryPath,
		Command:          command,
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
			Command:           command,
		}, nil
	}
	if transportType == "gemini_agy" {
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
			Command:           command,
		}, nil
	}

	cmd := commandContextFn(ctx, binaryPath, args...)
	cmd.Env = r.getEnvForExecution(req.ProviderKey, req.AccountHomePath, sessionCustomEnv, req.ProxyURL)
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
		Command:           command,
	}, nil
}

func (r *Runner) SendMessage(ctx context.Context, req AiSessionMessageRequest) (PromptExecutionResult, error) {
	return r.SendMessageWithCallback(ctx, req, nil)
}

func (r *Runner) SendMessageWithCallback(ctx context.Context, req AiSessionMessageRequest, callback SessionStreamCallback) (PromptExecutionResult, error) {
	if callback == nil {
		callback = req.StreamCallback
	}
	emit := func(event SessionStreamEvent) {
		if callback != nil {
			callback(event)
		}
	}

	if req.Session.ProcessKey == nil {
		return PromptExecutionResult{}, fmt.Errorf("session process key is required")
	}

	r.sessionsMu.Lock()
	session, exists := r.sessions[*req.Session.ProcessKey]
	r.sessionsMu.Unlock()

	if !exists {
		return PromptExecutionResult{}, fmt.Errorf("session_dead: session %q not found or expired", *req.Session.ProcessKey)
	}

	session.SendMu.Lock()
	defer session.SendMu.Unlock()

	session.Mu.Lock()
	if session.Status == "completed" {
		terminationErr := sessionTerminationErrorLocked(session)
		session.Mu.Unlock()
		if terminationErr != nil {
			return PromptExecutionResult{}, terminationErr
		}
		return PromptExecutionResult{}, fmt.Errorf("session_dead: session %q has been closed", *req.Session.ProcessKey)
	}
	session.LastUsedAt = time.Now().UTC()
	if req.IdleTTLSeconds != nil && *req.IdleTTLSeconds > 0 {
		session.IdleTTL = time.Duration(*req.IdleTTLSeconds) * time.Second
	}
	session.InFlight = true
	session.Mu.Unlock()

	defer func() {
		session.Mu.Lock()
		session.InFlight = false
		session.Mu.Unlock()
	}()

	startedAt := time.Now().UTC()
	actualPrompt := req.Prompt

	session.Mu.Lock()
	effectiveAccountHomePath := strings.TrimSpace(req.AccountHomePath)
	if effectiveAccountHomePath == "" {
		effectiveAccountHomePath = strings.TrimSpace(session.AccountHomePath)
	}
	session.Mu.Unlock()

	if len(req.RequiredMcps) > 0 {
		preparedPrompt, err := r.preparePromptForRequiredMcps(
			req.Prompt,
			req.RequiredMcps,
			session.Provider,
			effectiveAccountHomePath,
			req.AllowWrite,
			req.YoloMode,
		)
		if err != nil {
			return PromptExecutionResult{}, err
		}
		actualPrompt = preparedPrompt
	}

	var outputMarkdown string

	if session.TransportType == "codex_mcp" {
		toolName := "codex"
		args := map[string]interface{}{
			"prompt": actualPrompt,
		}
		session.Mu.Lock()
		providerSessionID := session.ProviderSessionID
		sessionID := session.SessionID
		modelName := strings.TrimSpace(session.Model)
		reasoningEffort := strings.TrimSpace(session.ReasoningEffort)
		session.Mu.Unlock()

		if providerSessionID != "" && providerSessionID != "codex_mcp_session_"+sessionID {
			toolName = "codex-reply"
			args["threadId"] = providerSessionID
		} else {
			if modelName != "" {
				args["model"] = modelName
			}
			if reasoningEffort != "" {
				args["config"] = map[string]interface{}{
					"model_reasoning_effort": strings.ToLower(reasoningEffort),
				}
			}
		}
		params := map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		}
		if err := writeJsonRpcRequest(session.Stdin, "tools/call", params, 3); err != nil {
			if terminationErr := sessionTerminationError(session); terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
			return PromptExecutionResult{}, fmt.Errorf("session_dead: failed to send MCP tool call: %w", err)
		}
		resp, err := readJsonRpcResponseWithHandler(session.StdoutScanner, 3, func(msg map[string]interface{}) {
			if raw, err := json.Marshal(msg); err == nil {
				emit(SessionStreamEvent{Type: "chunk", Stream: "stdout", Message: string(raw)})
			}
		})
		if err != nil {
			if terminationErr := sessionTerminationError(session); terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
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
				session.Mu.Lock()
				session.ProviderSessionID = threadID
				session.Mu.Unlock()
			}
		}
	} else if session.TransportType == "gemini_agy" {
		session.Mu.Lock()
		model := session.Model
		providerSessionID := session.ProviderSessionID
		accountHomePath := session.AccountHomePath
		customEnv := cloneSessionCustomEnv(session.CustomEnv)
		proxyURL := session.ProxyURL
		workingDirectory := session.WorkingDirectory
		session.Mu.Unlock()

		projectEnv := geminiProjectEnvHints(accountHomePath, customEnv)
		projectID, resume, err := geminiSessionProjectID(providerSessionID, workingDirectory, projectEnv)
		if err != nil {
			return PromptExecutionResult{}, err
		}
		if projectID != providerSessionID {
			session.Mu.Lock()
			session.ProviderSessionID = projectID
			session.Mu.Unlock()
		}

		// createProject is false: --project UUID alone is sufficient once the config file
		// exists at ~/.gemini/config/projects/UUID.json.  --new-project was previously
		// added because logs showed the wrong project being used, but that was caused by
		// the old arg order where --print consumed --project as the prompt value (the flag
		// was never reaching agy).  With --print moved to the end this is no longer needed,
		// and --new-project causes agy to emit no LLM response to stdout.
		args := geminiCLIArgs(workingDirectory, projectID, normalizeGeminiModelName(model), req.YoloMode, resume, false)
		logGeminiAgyLaunch("session", session.BinaryPath, args, geminiLaunchDebug{
			Cwd:               workingDirectory,
			ProjectID:         projectID,
			ProviderSessionID: providerSessionID,
			Resume:            resume,
			CreateProject:     false,
			Env:               projectEnv,
		})
		args = append(args, "--print", actualPrompt)
		cmd := commandContextFn(ctx, session.BinaryPath, args...)
		cmd.Env = r.getEnvForExecution(session.Provider, accountHomePath, customEnv, proxyURL)
		cmd.Dir = workingDirectory

		stdoutText, stderrText, err := captureAgyPrint(ctx, cmd)
		if err != nil {
			log.Printf("[gemini-agy] result stage=session status=error project_id=%q cwd=%q stdout_bytes=%d stderr=%q err=%v", projectID, workingDirectory, len(stdoutText), limitLogText(stderrText, 1000), err)
			return PromptExecutionResult{}, geminiProcessError("print", err, stderrText)
		}
		log.Printf("[gemini-agy] result stage=session status=ok project_id=%q cwd=%q stdout_bytes=%d stderr_bytes=%d", projectID, workingDirectory, len(stdoutText), len(stderrText))
		outputMarkdown = strings.TrimSpace(stdoutText)
		if outputMarkdown == "" {
			outputMarkdown = recoverGeminiAgyLatestMessageFn(workingDirectory, projectEnv)
			if outputMarkdown != "" {
				log.Printf("[gemini-agy] result stage=session recovery=conversation_db project_id=%q cwd=%q recovered_bytes=%d", projectID, workingDirectory, len(outputMarkdown))
			}
		}
		if outputMarkdown != "" {
			emit(SessionStreamEvent{Type: "chunk", Stream: "stdout", Message: outputMarkdown})
		}
	} else if session.TransportType == "claude_stream_json" {
		session.Mu.Lock()
		model := session.Model
		reasoningEffort := session.ReasoningEffort
		providerSessionID := session.ProviderSessionID
		accountHomePath := session.AccountHomePath
		customEnv := cloneSessionCustomEnv(session.CustomEnv)
		proxyURL := session.ProxyURL
		session.Mu.Unlock()

		args := buildClaudePrintArgs(model, reasoningEffort, providerSessionID)
		cmd := commandContextFn(ctx, session.BinaryPath, args...)
		cmd.Env = r.getEnvForExecution(session.Provider, accountHomePath, customEnv, proxyURL)
		cmd.Dir = session.WorkingDirectory

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to create Claude stdin pipe: %w", err)
		}

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			_ = stdin.Close()
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to create Claude stdout pipe: %w", err)
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			_ = stdin.Close()
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to create Claude stderr pipe: %w", err)
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		var captureMu sync.Mutex
		var captureWg sync.WaitGroup

		session.Mu.Lock()
		if session.Status == "completed" {
			terminationErr := sessionTerminationErrorLocked(session)
			session.Mu.Unlock()
			_ = stdin.Close()
			if terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
			return PromptExecutionResult{}, fmt.Errorf("session_dead: session has been closed")
		}
		if err := cmd.Start(); err != nil {
			session.Mu.Unlock()
			_ = stdin.Close()
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to start Claude print command: %w", err)
		}
		captureWg.Add(2)
		go func() {
			defer captureWg.Done()
			scanner := bufio.NewScanner(stdoutPipe)
			buf := make([]byte, 64*1024)
			scanner.Buffer(buf, 10*1024*1024)
			for scanner.Scan() {
				line := append([]byte(nil), scanner.Bytes()...)
				captureMu.Lock()
				stdout.Write(line)
				stdout.WriteByte('\n')
				captureMu.Unlock()
				if text := extractClaudeStreamText(line); text != "" {
					emit(SessionStreamEvent{Type: "chunk", Stream: "stdout", Message: text})
				} else {
					emit(SessionStreamEvent{Type: "chunk", Stream: "stdout", Message: string(line)})
				}
			}
		}()
		go func() {
			defer captureWg.Done()
			scanner := bufio.NewScanner(stderrPipe)
			buf := make([]byte, 64*1024)
			scanner.Buffer(buf, 10*1024*1024)
			for scanner.Scan() {
				line := append([]byte(nil), scanner.Bytes()...)
				captureMu.Lock()
				stderr.Write(line)
				stderr.WriteByte('\n')
				captureMu.Unlock()
				emit(SessionStreamEvent{Type: "chunk", Stream: "stderr", Message: string(line)})
			}
		}()
		session.Cmd = cmd
		if cmd.Process != nil {
			session.Pid = cmd.Process.Pid
		}
		session.Mu.Unlock()

		msg := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "text",
						"text": actualPrompt,
					},
				},
			},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			session.Mu.Lock()
			session.Cmd = nil
			session.Pid = 0
			session.Mu.Unlock()
			return PromptExecutionResult{}, err
		}

		if _, err := stdin.Write(append(data, '\n')); err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			session.Mu.Lock()
			session.Cmd = nil
			session.Pid = 0
			session.Mu.Unlock()
			if terminationErr := sessionTerminationError(session); terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to write Claude stream JSON: %w", err)
		}
		if err := stdin.Close(); err != nil {
			_ = cmd.Wait()
			session.Mu.Lock()
			session.Cmd = nil
			session.Pid = 0
			session.Mu.Unlock()
			if terminationErr := sessionTerminationError(session); terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: failed to close Claude stdin: %w", err)
		}

		err = cmd.Wait()
		captureWg.Wait()

		session.Mu.Lock()
		session.Cmd = nil
		session.Pid = 0
		session.Mu.Unlock()

		if err != nil {
			if terminationErr := sessionTerminationError(session); terminationErr != nil {
				return PromptExecutionResult{}, terminationErr
			}
			captureMu.Lock()
			stderrText := strings.TrimSpace(stderr.String())
			captureMu.Unlock()
			if stderrText != "" {
				if msg := claudeUsageLimitMessage(map[string]any{"message": stderrText}); msg != "" {
					return PromptExecutionResult{}, fmt.Errorf("provider error: %s", msg)
				}
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", stderrText)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude print command failed: %w", err)
		}

		captureMu.Lock()
		outputBytes := bytes.TrimSpace(append([]byte(nil), stdout.Bytes()...))
		stderrText := strings.TrimSpace(stderr.String())
		captureMu.Unlock()
		if len(outputBytes) == 0 {
			if stderrText != "" {
				if msg := claudeUsageLimitMessage(map[string]any{"message": stderrText}); msg != "" {
					return PromptExecutionResult{}, fmt.Errorf("provider error: %s", msg)
				}
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
			if msg := claudeUsageLimitMessage(resp); msg != "" {
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", msg)
			}
			if resultText, ok := resp["result"].(string); ok && strings.TrimSpace(resultText) != "" {
				if msg := claudeUsageLimitMessage(map[string]any{"message": resultText}); msg != "" {
					return PromptExecutionResult{}, fmt.Errorf("provider error: %s", msg)
				}
				return PromptExecutionResult{}, fmt.Errorf("provider error: %s", resultText)
			}
			return PromptExecutionResult{}, fmt.Errorf("provider error: Claude request failed")
		}

		if resultText, ok := resp["result"].(string); ok {
			outputMarkdown = resultText
		}
		if sID, ok := resp["session_id"].(string); ok && strings.TrimSpace(sID) != "" {
			session.Mu.Lock()
			session.ProviderSessionID = sID
			session.Mu.Unlock()
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

	session.Mu.Lock()
	pSessionID := session.ProviderSessionID
	session.Mu.Unlock()

	result := PromptExecutionResult{
		Status:            "success",
		RunID:             newRunID(),
		ProviderKey:       session.Provider,
		ModelName:         &session.Model,
		ProviderSessionID: pSessionID,
		Command:           session.Command,
		OutputMarkdown:    outputMarkdown,
		StartedAt:         startedAt.Format(time.RFC3339Nano),
		CompletedAt:       completedAt.Format(time.RFC3339Nano),
		ActualPromptText:  actualPrompt,
	}
	applyRequiredMcpFailureStatus(&result, req.RequiredMcps)

	return result, nil
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
		_ = r.retireGoogleDriveProxyApprovalsForProcess(*handle.ProcessKey, "approval expired because the session was no longer active")
		if handle.ProcessPid != nil {
			processName, err := lookupProcessNameFn(ctx, *handle.ProcessPid)
			if err == nil && processNameMatchesTransport(processName, handle.TransportType) {
				_ = killProcessByPIDFn(*handle.ProcessPid)
			}
		}
		return nil
	}

	stdin, cmd, transport := markSessionTerminated(session, "manual_close")
	_ = r.retireGoogleDriveProxyApprovalsForProcess(session.ProcessKey, "approval expired because the session closed before execution")

	if stdin != nil {
		stdin.Close()
	}
	if cmd == nil {
		return nil
	}

	if cmd.Process != nil {
		cmd.Process.Kill()
	}

	if transport != "claude_stream_json" {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
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
		stdin, cmd, transport := markSessionTerminated(session, "runner_cleanup")
		_ = r.retireGoogleDriveProxyApprovalsForProcess(session.ProcessKey, "approval expired because the runner cleaned up the session")

		if stdin != nil {
			stdin.Close()
		}
		if cmd != nil {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			if transport != "claude_stream_json" {
				go cmd.Wait()
			}
		}
	}

	// Provider processes spawned by the interactive layer are tracked outside
	// r.sessions — ACP agents (devin/opencode/grok), the claude pool, and the
	// shared codex app-server. Skipping them leaks orphans on every shutdown
	// (stray `devin acp`/`opencode acp` outliving `just dev` teardown).
	r.closeAllDevinProcesses()
	r.closeAllOpencodeProcesses()
	r.closeAllGrokProcesses()
	if r.claudePool != nil {
		r.claudePool.closeAll()
	}
	r.codexAppServerMu.Lock()
	if h := r.codexAppServer; h != nil {
		r.codexAppServer = nil
		h.close()
	}
	r.codexAppServerMu.Unlock()
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
		idle := !sess.InFlight && time.Since(sess.LastUsedAt) > sess.IdleTTL
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
		stdin, cmd, transport := markSessionTerminated(session, "idle_timeout")
		_ = r.retireGoogleDriveProxyApprovalsForProcess(session.ProcessKey, "approval expired because the session timed out before execution")

		if stdin != nil {
			stdin.Close()
		}
		if cmd != nil {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			if transport != "claude_stream_json" {
				go cmd.Wait()
			}
		}
	}
}

func DetermineProviderSessionID(transportType, provider, sessionID, currentProviderSessionID string, resumeProviderSessionID *string) string {
	if transportType == "gemini_acp" || transportType == "gemini_agy" {
		if currentProviderSessionID == "" {
			currentProviderSessionID = "gemini_agy_session_" + sessionID
		}
	} else if transportType == "claude_stream_json" {
		currentProviderSessionID = "claude_stream_session_" + sessionID
	}

	if resumeProviderSessionID != nil && *resumeProviderSessionID != "" && (provider == "codex" || provider == "claude" || provider == "gemini") {
		currentProviderSessionID = *resumeProviderSessionID
	}
	return currentProviderSessionID
}

func isSyntheticGeminiSessionID(providerSessionID string) bool {
	trimmed := strings.TrimSpace(providerSessionID)
	return strings.HasPrefix(trimmed, "gemini_acp_session_") ||
		strings.HasPrefix(trimmed, "gemini_agy_session_") ||
		strings.HasPrefix(trimmed, "flowpilot-gemini-") ||
		strings.HasPrefix(trimmed, "thread-")
}

// normalizeClaudeEffort lowercases/trims a reasoning-effort value for the
// claude CLI's --effort flag. Previously mapped "xhigh"->"max", on the
// assumption they were synonyms; live inspection of the installed
// @anthropic-ai/claude-code binary (2.1.191) during Task-215 found the CLI's
// actual --effort validator accepts five DISTINCT values
// (GD=["low","medium","high","xhigh","max"]) — "xhigh" and "max" are
// different effort tiers, not aliases, so selecting "Extra High" must send
// "xhigh", not silently upgrade to "max".
func normalizeClaudeEffort(reasoningEffort string) string {
	return strings.ToLower(strings.TrimSpace(reasoningEffort))
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
	case "gemini_acp", "gemini_agy":
		return []string{"agy", "agy.exe", "gemini", "gemini.exe"}
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
