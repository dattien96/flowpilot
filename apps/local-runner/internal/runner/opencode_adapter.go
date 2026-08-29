package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Task-301 (CP-57 P-3): the Opencode provider adapter. Implements the
// provider-neutral ProviderRuntimeAdapter on top of the Task-300 dispatcher.
// Modeled on grokAdapter but simplified for MVP (no full approval policy yet,
// MCP false).

const opencodeAskUserReinforcement = "\n\n---\nWhen you need to ask the user a question, call the FlowPilot MCP tool `ask_user` on server `flowpilot` with arguments `prompt` (string), `options` (array of strings), and optional `multiSelect` (boolean). Do NOT use native `question` tools."

const opencodeSpawnAgentReinforcement = "\n\n---\nWhen you need to spawn a sub-agent for parallel or delegated work, call the FlowPilot MCP tool `spawn_agent` on server `flowpilot` with arguments `agent` (string), `prompt` (string), optional `provider` (string), and optional `wait` (boolean — true to block until the child completes). Do NOT use native `spawn_subagent` tools."

const opencodeToolReinforcements = opencodeAskUserReinforcement + opencodeSpawnAgentReinforcement

type opencodeAdapter struct {
	dispatcher *opencodeDispatcher
	cwd        string
	initResult map[string]any

	promptPrep func(TurnRequest) string

	sessionStore ProviderSessionStore

	mcpServer       *claudeMCPServer
	mcpBaseURL      func() string
	extraMCPServers func(yolo bool) map[string]claudeMcpServer

	mu                 sync.Mutex
	bridges            map[string]TurnBridge
	lastSessionID      string
	runSessions        *opencodeRunSessionIndex
	allowReviewOutcome map[string]bool
	yoloModes          map[string]bool
}

func newOpencodeAdapter(dispatcher *opencodeDispatcher, cwd string) *opencodeAdapter {
	a := &opencodeAdapter{
		dispatcher:         dispatcher,
		cwd:                cwd,
		bridges:            map[string]TurnBridge{},
		allowReviewOutcome: map[string]bool{},
		yoloModes:          map[string]bool{},
	}
	if dispatcher != nil {
		dispatcher.setInbound(a.handleInbound)
	}
	return a
}

func (a *opencodeAdapter) Key() ProviderKey { return ProviderKeyOpencode }

// opencodeRunSessionIndex mirrors grokRunSessionIndex for Opencode ACP resume.
type opencodeRunSessionIndex struct {
	mu        sync.Mutex
	byRun     map[string]string
	bySession map[string]string
}

func (x *opencodeRunSessionIndex) remember(runID, sessionID string) {
	if x == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	sessionID = strings.TrimSpace(sessionID)
	if runID == "" || !isOpencodeRealSessionID(sessionID) {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.byRun == nil {
		x.byRun = make(map[string]string)
	}
	if x.bySession == nil {
		x.bySession = make(map[string]string)
	}
	if owner := strings.TrimSpace(x.bySession[sessionID]); owner != "" && owner != runID {
		return
	}
	if prev := strings.TrimSpace(x.byRun[runID]); prev != "" && prev != sessionID {
		delete(x.bySession, prev)
	}
	x.byRun[runID] = sessionID
	x.bySession[sessionID] = runID
}

func (x *opencodeRunSessionIndex) lookup(runID string) string {
	if x == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return strings.TrimSpace(x.byRun[runID])
}

func (x *opencodeRunSessionIndex) ownerOf(sessionID string) string {
	if x == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return strings.TrimSpace(x.bySession[sessionID])
}

func (a *opencodeAdapter) rememberRunSession(runID, sessionID string) {
	if a == nil {
		return
	}
	if a.runSessions == nil {
		a.runSessions = &opencodeRunSessionIndex{byRun: make(map[string]string), bySession: make(map[string]string)}
	}
	a.runSessions.remember(runID, sessionID)
}

func (a *opencodeAdapter) lookupRunSession(runID string) string {
	if a == nil {
		return ""
	}
	return a.runSessions.lookup(runID)
}

func isOpencodeRealSessionID(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" || strings.HasPrefix(id, "thread-") {
		return false
	}
	if id == "." || id == ".." {
		return false
	}
	if strings.ContainsAny(id, `/\`) {
		return false
	}
	// Opencode session IDs are ses_ prefix (live 1.18.18). Enforce prefix to avoid
	// treating random strings as real (see review M-3).
	return strings.HasPrefix(id, "ses_")
}

func opencodeEnsureResumeID(req TurnRequest, lookup func(string) string) string {
	id := strings.TrimSpace(req.ProviderSessionID)
	if isOpencodeRealSessionID(id) {
		return id
	}
	if lookup != nil {
		if cached := strings.TrimSpace(lookup(req.RunID)); isOpencodeRealSessionID(cached) {
			return cached
		}
	}
	return id
}

// Capabilities advertises only what has a passing test (MVP honest).
// After Task-303 wiring, Mcp/ApprovalEvents become true when mcpServer is wired (like Grok).
func (a *opencodeAdapter) Capabilities() ProviderCapabilities {
	hasMCP := a != nil && a.mcpServer != nil
	return ProviderCapabilities{
		Streaming:      true,
		Resume:         true,
		FileEvents:     true,
		Interrupt:      true,
		SkillSelection: true,
		ApprovalEvents: hasMCP,
		Mcp:            hasMCP,
		Vision:         false,
	}
}

func (a *opencodeAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt
}

func (a *opencodeAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	a.mu.Lock()
	a.lastSessionID = ""
	a.mu.Unlock()

	// MCP wiring (Task-301 MVP: nil → no MCP, same as Grok MVP)
	var mcpToken string
	var mcpServers []interface{}
	if a.mcpServer != nil {
		mcpToken = a.mcpServer.register(bridge, req.OfferReviewOutcomeTool)
		defer a.mcpServer.unregister(mcpToken)
		baseURL := ""
		if a.mcpBaseURL != nil {
			baseURL = a.mcpBaseURL()
		}
		mcpServers = append(mcpServers, opencodeACPFlowPilotMCPServers(baseURL, mcpToken)...)
		if a.extraMCPServers != nil {
			mcpServers = append(mcpServers, opencodeACPExtraMCPServers(a.extraMCPServers(req.YoloMode))...)
		}
	}

	sessionID, err := a.ensureSession(ctx, req, cwd, mcpServers)
	if err != nil {
		return err
	}
	// CP-57: per-turn model/effort via session/set_config_option (live configOptions model/effort)
	// Extra model field on session/new is ignored (verified), so we apply after ensureSession.
	a.applyOpencodeSessionConfig(ctx, sessionID, req.ModelName, req.ReasoningEffort)

	if mcpToken != "" {
		_ = a.mcpServer.waitReady(ctx, mcpToken, claudeMCPReadyDefaultTimeout)
	}

	notif, err := a.dispatcher.registerSession(sessionID)
	if err != nil {
		return err
	}
	defer a.dispatcher.unregisterSession(sessionID)

	a.mu.Lock()
	a.bridges[sessionID] = bridge
	a.allowReviewOutcome[sessionID] = req.OfferReviewOutcomeTool
	a.yoloModes[sessionID] = req.YoloMode && !req.ForceShellBridge && !IsReadOnlyChatPosture(req.ChatPosture)
	// Reference OpencodePermissionMode to keep YOLO SSOT in sync (future session/new permission wiring)
	_ = resolveYoloPosture(req.YoloMode).OpencodePermissionMode
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, sessionID)
		delete(a.allowReviewOutcome, sessionID)
		delete(a.yoloModes, sessionID)
		a.mu.Unlock()
	}()

	prompt := a.preparePrompt(req)
	promptParams := opencodeACPPromptParams(sessionID, prompt)

	type promptOutcome struct {
		result map[string]any
		err    error
	}
	done := make(chan promptOutcome, 1)
	go func() {
		res, e := a.dispatcher.call(ctx, "session/prompt", promptParams)
		done <- promptOutcome{result: res, err: e}
	}()

	var lastText string
	for {
		select {
		case <-ctx.Done():
			_ = a.dispatcher.notify("session/cancel", map[string]any{"sessionId": sessionID})
			return ctx.Err()
		case outcome := <-done:
			if outcome.err != nil {
				return outcome.err
			}
			lastText = a.drainOpencodeNotifications(sessionID, notif, bridge, lastText)
			return a.emitTerminal(ctx, req, bridge, sessionID, outcome.result, lastText)
		case n, ok := <-notif:
			if !ok {
				return fmt.Errorf("opencode acp stream closed mid-turn")
			}
			lastText = a.applyOpencodeNotification(sessionID, n, bridge, lastText)
		}
	}
}

func (a *opencodeAdapter) applyOpencodeNotification(sessionID string, n opencodeNotification, bridge TurnBridge, lastText string) string {
	events, mapped := mapOpencodeNotification(n)
	if !mapped {
		return lastText
	}
	for _, ev := range events {
		if ev.Type == EventMessageDelta {
			lastText += ev.Text
		}
		bridge.Emit(ev)
	}
	return lastText
}

func (a *opencodeAdapter) drainOpencodeNotifications(sessionID string, notif <-chan opencodeNotification, bridge TurnBridge, lastText string) string {
	for {
		select {
		case n, ok := <-notif:
			if !ok {
				return lastText
			}
			lastText = a.applyOpencodeNotification(sessionID, n, bridge, lastText)
		default:
			return lastText
		}
	}
}

func (a *opencodeAdapter) ensureSession(ctx context.Context, req TurnRequest, cwd string, mcpServers []interface{}) (string, error) {
	resumeID := opencodeEnsureResumeID(req, a.lookupRunSession)
	// Grok-proven (run-75035/Task-210): synthetic thread-* never goes to session/load.
	// First turn has providerSessionID=thread-* and no cached ses_* → session/new.
	// Subsequent turns use cached ses_* via lookupRunSession → session/load.
	// Never error on thread-*; just force a fresh session.
	if isOpencodeRealSessionID(resumeID) && a.runSessions != nil {
		if owner := a.runSessions.ownerOf(resumeID); owner != "" && owner != strings.TrimSpace(req.RunID) {
			log.Printf("opencode ensureSession: refusing session/load of %s owned by run %s (this run %s); forcing session/new", resumeID, owner, req.RunID)
			resumeID = ""
		}
	}
	method := "session/new"
	var result map[string]any
	var err error
	if isOpencodeRealSessionID(resumeID) {
		method = "session/load"
		result, err = a.dispatcher.call(ctx, method, opencodeACPSessionLoadParams(resumeID, cwd, mcpServers))
	} else {
		result, err = a.dispatcher.call(ctx, method, opencodeACPSessionNewParams(cwd, mcpServers, nil, "", ""))
	}
	if err != nil {
		return "", err
	}
	sessionID := opencodeACPResponseSessionIDFromResult(result)
	if sessionID == "" && method == "session/load" && isOpencodeRealSessionID(resumeID) {
		// BUG-329: on a fresh `opencode acp` process, session/load of a ses_*
		// created by another process instance answers RPC-OK with a config-only
		// result and NO sessionId (live-probed 1.18.18). The RPC succeeded, so
		// adopt the requested resume id instead of failing the turn — and never
		// fall back to session/new here, which would silently lose history.
		// With the BUG-329 process-reuse the id does live in this process; the
		// config-only shape is a response quirk, not a load failure.
		sessionID = resumeID
	}
	if sessionID == "" {
		return "", fmt.Errorf("opencode %s returned no sessionId", method)
	}
	a.mu.Lock()
	a.lastSessionID = sessionID
	a.mu.Unlock()
	a.rememberRunSession(req.RunID, sessionID)
	a.recordSession(ctx, req, sessionID)
	return sessionID, nil
}

func (a *opencodeAdapter) recordSession(ctx context.Context, req TurnRequest, sessionID string) {
	if a.sessionStore == nil {
		return
	}
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyOpencode),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  cwd,
		ModelName:         req.ModelName,
		Status:            "active",
	})
}

// applyOpencodeSessionConfig applies per-turn model/effort via ACP session/set_config.
// Live session/new returns configOptions {id:"model", id:"effort", id:"mode"} but extra
// model field on session/new is ignored (verified). We set via RPC after ensureSession
// and degrade gracefully so older builds still complete the turn.
func (a *opencodeAdapter) applyOpencodeSessionConfig(ctx context.Context, sessionID, modelName, effort string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || a.dispatcher == nil {
		return
	}
	modelName = strings.TrimSpace(modelName)
	// Use a bounded per-RPC context so a missing handler (test fake) does not block the turn
	// and the second RPC still gets a full budget (review I-2).
	if modelName != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := a.dispatcher.call(cfgCtx, "session/set_config_option", opencodeACPSessionSetConfigParams(sessionID, "model", modelName)); err != nil {
				_, _ = a.dispatcher.call(cfgCtx, "session/set_config", opencodeACPSessionSetConfigParamsAlt(sessionID, "model", modelName))
			}
		}()
	}
	variant, ok := opencodeReasoningVariantID(effort)
	if ok && variant != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := a.dispatcher.call(cfgCtx, "session/set_config_option", opencodeACPSessionSetConfigParams(sessionID, "effort", variant)); err != nil {
				_, _ = a.dispatcher.call(cfgCtx, "session/set_config", opencodeACPSessionSetConfigParamsAlt(sessionID, "effort", variant))
			}
		}()
	}
}

func (a *opencodeAdapter) emitTerminal(ctx context.Context, req TurnRequest, bridge TurnBridge, sessionID string, result map[string]any, lastText string) error {
	if result == nil {
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "opencode session/prompt returned no result", Recoverable: true})
		return nil
	}
	if adopted := opencodeACPResponseSessionIDFromResult(result); adopted != "" && adopted != sessionID {
		a.mu.Lock()
		a.lastSessionID = adopted
		a.mu.Unlock()
		a.rememberRunSession(req.RunID, adopted)
		a.recordSession(ctx, req, adopted)
	}
	if usage, ok := result["usage"].(map[string]any); ok {
		if token := opencodePromptResultTokenUsage(usage, opencodeContextWindowFromInit(a.initResult)); token != nil {
			bridge.Emit(ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: token})
		}
	}
	stopReason, _ := result["stopReason"].(string)
	finalText := lastText
	if finalText == "" {
		// Try to extract from result if present
		if text, ok := result["text"].(string); ok {
			finalText = text
		}
	}
	evType := opencodeStopReasonToEvent(stopReason)
	if evType == EventTurnFailed {
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: fmt.Sprintf("opencode turn ended: %s", stopReason), Recoverable: false})
		return nil
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalText})
	return nil
}

func (a *opencodeAdapter) LastOpencodeSessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSessionID
}

// handleInbound routes server->client requests. Task-303 T-4: permission channel → bridge.RequestApproval
func (a *opencodeAdapter) handleInbound(req opencodeInboundRequest) {
	if !opencodeACPIsPermissionRequest(req.Method) {
		_ = a.dispatcher.replyError(req.ID, "unsupported opencode inbound request: "+req.Method)
		return
	}
	sessionID := opencodeSessionIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[sessionID]
	yolo := a.yoloModes[sessionID]
	a.mu.Unlock()

	options, _ := req.Params["options"].([]any)
	if bridge == nil {
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opencodeEncodePermissionDecision(options, false)}})
		return
	}

	details := opencodeApprovalDetailsFromRequest(req.Params)

	// YOLO auto-approve only for real tool permission (session/request_permission).
	// Ask-user via "question" must never be auto-approved (spec Q-3).
	isToolPermission := strings.TrimSpace(req.Method) == "session/request_permission"
	if yolo && isToolPermission {
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opencodeEncodePermissionDecision(options, true)}})
		return
	}

	decision, err := bridge.RequestApproval(details)
	if err != nil {
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opencodeEncodePermissionDecision(options, false)}})
		return
	}
	approve := decision == "approve" || decision == "approved" || decision == "approve_for_session"
	_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opencodeEncodePermissionDecision(options, approve)}})
}

// opencodeEncodePermissionDecision picks an optionId from options that matches approve/deny.
func opencodeEncodePermissionDecision(options []any, approve bool) string {
	// Options are [{optionId, kind, name}] where optionId is "once"/"always"/"reject" and kind is allow_once etc.
	// Prefer kind matching.
	for _, raw := range options {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		kind, _ := m["kind"].(string)
		optionID, _ := m["optionId"].(string)
		if approve {
			if strings.Contains(strings.ToLower(kind), "allow") {
				return optionID
			}
		} else {
			if strings.Contains(strings.ToLower(kind), "reject") {
				return optionID
			}
		}
	}
	// Fallback: first option for approve, last for deny
	if len(options) > 0 {
		if approve {
			if m, ok := options[0].(map[string]any); ok {
				if id, _ := m["optionId"].(string); id != "" {
					return id
				}
			}
			return "once"
		}
		if m, ok := options[len(options)-1].(map[string]any); ok {
			if id, _ := m["optionId"].(string); id != "" {
				return id
			}
		}
		return "reject"
	}
	if approve {
		return "once"
	}
	return "reject"
}

func opencodeApprovalDetailsFromRequest(params map[string]any) ApprovalDetails {
	toolCall, _ := params["toolCall"].(map[string]any)
	title, _ := toolCall["title"].(string)
	rawInput, _ := toolCall["rawInput"].(map[string]any)
	var command string
	if rawInput != nil {
		if cmd, _ := rawInput["command"].(string); cmd != "" {
			command = cmd
		} else if cmd, _ := rawInput["cmd"].(string); cmd != "" {
			command = cmd
		} else if fp, _ := rawInput["filepath"].(string); fp != "" {
			command = fp
		} else if fp, _ := rawInput["filePath"].(string); fp != "" {
			command = fp
		} else if fp, _ := rawInput["path"].(string); fp != "" {
			command = fp
		}
	}
	if command == "" {
		command = title
	}
	// Map to exec/file/mcp/other for BUG-246 allowlist: only exec is eligible for "don't ask again"
	approvalKind := opencodePermissionKind(toolCall, rawInput)
	return ApprovalDetails{
		Command: command,
		Reason:  title,
		Kind:    approvalKind,
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
}

func opencodePermissionKind(toolCall, rawInput map[string]any) string {
	if toolCall == nil {
		return "other"
	}
	kind, _ := toolCall["kind"].(string)
	lowerKind := strings.ToLower(strings.TrimSpace(kind))
	switch lowerKind {
	case "edit", "write", "create", "delete":
		return "file"
	case "read":
		return "file"
	case "execute", "run", "shell", "terminal", "bash", "exec":
		return "exec"
	}
	title, _ := toolCall["title"].(string)
	lowerTitle := strings.ToLower(title)
	if strings.Contains(lowerTitle, "bash") || strings.Contains(lowerTitle, "shell") || strings.Contains(lowerTitle, "exec") || strings.Contains(lowerTitle, "command") {
		return "exec"
	}
	if rawInput != nil {
		if _, hasCmd := rawInput["command"]; hasCmd {
			return "exec"
		}
		if _, hasCmd := rawInput["cmd"]; hasCmd {
			return "exec"
		}
		if _, hasFile := rawInput["filePath"]; hasFile {
			return "file"
		}
		if _, hasFile := rawInput["filepath"]; hasFile {
			return "file"
		}
		if _, hasMCP := rawInput["mcp"]; hasMCP {
			return "mcp"
		}
	}
	if strings.Contains(lowerTitle, ".") && (strings.Contains(lowerTitle, "/") || strings.Contains(lowerTitle, "\\")) {
		return "file"
	}
	return "other"
}
