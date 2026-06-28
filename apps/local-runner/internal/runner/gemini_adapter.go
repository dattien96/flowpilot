package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
)

var geminiBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_GEMINI_BIN")); b != "" {
		return b
	}
	return "gemini"
}

// geminiAdapter is the conservative controlled-mode Gemini MVP for Task-165.
// It uses ACP for one turn at a time; permission and MCP protocol scaffolding are
// present, but full live approval/MCP/resume parity stays unadvertised until
// authenticated validation proves those paths end to end.
type geminiAdapter struct {
	cwd        string
	scopeKey   string
	env        map[string]string
	promptPrep func(TurnRequest) string

	sessions     *geminiSessionMap
	sessionStore ProviderSessionStore
	mcpServer    *claudeMCPServer
	mcpBaseURL   func() string
}

func newGeminiAdapter(cwd, scopeKey string, env map[string]string) *geminiAdapter {
	return &geminiAdapter{cwd: cwd, scopeKey: scopeKey, env: env}
}

func (a *geminiAdapter) Key() ProviderKey { return ProviderKeyGemini }

func (a *geminiAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming:      true,
		SkillSelection: true,
		Interrupt:      true,
	}
}

func (a *geminiAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt
}

func (a *geminiAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}

	args := geminiACPArgs(req.ModelName)
	cmd := commandContextFn(ctx, geminiBinaryName(), args...)
	cmd.Env = os.Environ()
	for key, value := range a.env {
		if strings.TrimSpace(key) != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("gemini stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("gemini stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("gemini start: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	if err := writeJsonRpcRequest(stdin, "initialize", geminiACPInitializeParams(), 1); err != nil {
		return fmt.Errorf("gemini initialize write: %w", err)
	}
	resp, err := readJsonRpcResponse(scanner, 1)
	if err != nil {
		return geminiProcessError("initialize", err, stderr.String())
	}
	if errMsg := jsonRpcErrorMessage(resp); errMsg != "" {
		return fmt.Errorf("gemini initialize: %s", errMsg)
	}

	resumeID, err := a.resumeSessionID(req.ProviderSessionID)
	if err != nil {
		return err
	}
	var mcpServers []interface{}
	if a.mcpServer != nil && a.mcpBaseURL != nil {
		if base := a.mcpBaseURL(); strings.TrimSpace(base) != "" {
			token := a.mcpServer.register(bridge)
			defer a.mcpServer.unregister(token)
			mcpServers = geminiACPFlowPilotMCPServers(base, token)
		}
	}
	sessionMethod := "session/new"
	sessionParams := geminiACPSessionNewParamsWithMCP(cwd, mcpServers)
	if resumeID != "" {
		sessionMethod = "session/load"
		sessionParams = geminiACPSessionLoadParams(resumeID, cwd, mcpServers)
	}
	if err := writeJsonRpcRequest(stdin, sessionMethod, sessionParams, 2); err != nil {
		return fmt.Errorf("gemini %s write: %w", sessionMethod, err)
	}
	resp, err = readJsonRpcResponse(scanner, 2)
	if err != nil {
		return geminiProcessError(sessionMethod, err, stderr.String())
	}
	if errMsg := jsonRpcErrorMessage(resp); errMsg != "" {
		return fmt.Errorf("gemini %s: %s", sessionMethod, errMsg)
	}
	sessionID := geminiACPResponseSessionID(resp)
	if sessionID == "" {
		return fmt.Errorf("gemini %s: missing sessionId", sessionMethod)
	}
	a.recordSession(ctx, req, sessionID, cwd)

	if err := writeJsonRpcRequest(stdin, "session/prompt", geminiACPPromptParams(sessionID, a.preparePrompt(req)), 3); err != nil {
		return fmt.Errorf("gemini session/prompt write: %w", err)
	}
	var streamed strings.Builder
	resp, err = readJsonRpcResponseWithHandler(scanner, 3, func(msg map[string]interface{}) {
		if id, params, ok := geminiACPPermissionRequest(msg); ok {
			details := geminiACPApprovalDetails(params)
			decision, err := bridge.RequestApproval(details)
			_ = writeJsonRpcResponse(stdin, id, geminiACPPermissionResponse(params, decision, err))
			return
		}
		if text := extractGeminiACPText(msg); text != "" {
			streamed.WriteString(text)
			bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: text})
			return
		}
		for _, ev := range mapGeminiACPUpdate(msg) {
			bridge.Emit(ev)
		}
	})
	if err != nil {
		return geminiProcessError("session/prompt", err, stderr.String())
	}
	if errMsg := jsonRpcErrorMessage(resp); errMsg != "" {
		return fmt.Errorf("gemini session/prompt: %s", errMsg)
	}
	if promptSessionID := geminiACPResponseSessionID(resp); promptSessionID != "" && promptSessionID != sessionID {
		sessionID = promptSessionID
		a.recordSession(ctx, req, sessionID, cwd)
	}

	finalMessage := streamed.String()
	if result, ok := resp["result"].(map[string]interface{}); ok && finalMessage == "" {
		finalMessage = geminiACPResultText(result)
	}
	if finalMessage != "" {
		bridge.Emit(ProviderEvent{Type: EventMessageCompleted, Text: finalMessage})
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMessage})
	return nil
}

func (a *geminiAdapter) resumeSessionID(providerSessionID string) (string, error) {
	id := strings.TrimSpace(providerSessionID)
	if id == "" {
		return "", nil
	}
	if real := a.sessions.realSession(a.scopeKey, id); real != "" {
		return real, nil
	}
	if strings.HasPrefix(id, "thread-") || isSyntheticGeminiSessionID(id) {
		return "", fmt.Errorf("gemini resume session %q has no scoped real mapping", id)
	}
	return id, nil
}

func (a *geminiAdapter) recordSession(ctx context.Context, req TurnRequest, sessionID, cwd string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if a.sessions != nil {
		a.sessions.setRealSession(a.scopeKey, req.ProviderSessionID, sessionID)
		a.sessions.setRealSession(a.scopeKey, sessionID, sessionID)
	}
	if a.sessionStore == nil {
		return
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyGemini),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  cwd,
		ModelName:         req.ModelName,
		Status:            "active",
	})
}

func geminiACPArgs(model string) []string {
	args := []string{"--acp", "--approval-mode", "plan"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	return args
}

func geminiProcessError(stage string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg != "" {
		return fmt.Errorf("gemini %s: %w: %s", stage, err, msg)
	}
	return fmt.Errorf("gemini %s: %w", stage, err)
}
