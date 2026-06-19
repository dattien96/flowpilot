package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Phase 3 (04-03): the Codex provider adapter. Implements the provider-neutral
// ProviderRuntimeAdapter (P2 contract) on top of the async dispatcher + event
// mapper. It hosts threads on the shared app-server (cwd-per-thread), runs a turn,
// pumps mapped events to the bridge, and routes inbound server→client approval
// requests back through the bridge.
//
// In this environment the real `codex app-server` can't run, so this adapter is
// driven over a dispatcher backed by a scripted fake app-server in tests; the live
// registry stays on the fake adapter until validated against a real Codex build
// (06 Part D). The full YOLO SSOT resolver + approval policy + finalizer are P4.
type codexAdapter struct {
	dispatcher *codexDispatcher
	cwd        string

	// promptPrep assembles the final prompt before turn/start. Defaults to ask_user
	// reinforcement; the live process adapter overrides it to also run
	// injectSkillContent + preparePromptForRequiredMcps (04-03 runner-side assembly).
	promptPrep func(TurnRequest) string

	mu         sync.Mutex
	bridges    map[string]TurnBridge // threadId -> active turn bridge
	codexTurns map[string]string     // threadId -> Codex turn id (for interrupt)
}

func newCodexAdapter(dispatcher *codexDispatcher, cwd string) *codexAdapter {
	a := &codexAdapter{
		dispatcher: dispatcher,
		cwd:        cwd,
		bridges:    map[string]TurnBridge{},
		codexTurns: map[string]string{},
	}
	dispatcher.setInbound(a.handleInbound)
	return a
}

// askUserReinforcement is appended to every prompt so the model knows the FlowPilot-owned
// ask_user tool exists and when to use it (best-effort, 04-04). It deliberately biases toward
// ACTING: complete the clear parts of the task first (normal tools + approval gates apply) and
// reserve ask_user for a required decision that genuinely blocks progress — otherwise the model
// front-loads clarifying questions instead of doing obvious work (e.g. a plain file write).
const askUserReinforcement = "\n\n---\nComplete the clear, unambiguous parts of the task directly — your normal tools and approval gates still apply. Only call the `ask_user` tool (prompt, options[], multiSelect?) when a required decision genuinely blocks you and you cannot reasonably infer the answer or make progress without it; do not use it for things you can do or reasonably assume first."

// preparePrompt builds the final turn prompt. The default applies ask_user
// reinforcement; promptPrep (when set) replaces it with full runner-side assembly.
func (a *codexAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt + askUserReinforcement
}

// codexAskUserDynamicTool is the FlowPilot-owned ask_user tool registered on thread/start
// as a `DynamicToolSpec` (the real app-server registration channel). The model discovers it
// and a call arrives back as an `item/tool/call` server->client request (handleInbound).
func codexAskUserDynamicTool() any {
	return map[string]any{
		"name":        "ask_user",
		"description": "Ask the user a structured question and wait for their answer before continuing. Use when you need a decision or clarification instead of guessing.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":      map[string]any{"type": "string", "description": "The question to ask the user."},
				"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Selectable answer options."},
				"multiSelect": map[string]any{"type": "boolean", "description": "Allow selecting more than one option."},
			},
			"required": []any{"prompt"},
		},
	}
}

// codexSpawnAgentDynamicTool registers spawn_agent as a Codex DynamicToolSpec alongside
// ask_user so the model can create sub-agent runs (CP-19 / Task-082).
func codexSpawnAgentDynamicTool() any {
	return map[string]any{
		"name":        "spawn_agent",
		"description": "Spawn a sub-agent run for a focused task. The sub-agent runs with its own provider session and SSE stream. Use wait=true to block until the sub-agent's turn completes and receive its final message; use wait=false to fire-and-forget.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent":     map[string]any{"type": "string", "description": "Agent name from the catalog (e.g. 'coder', 'reviewer', 'tester')."},
				"prompt":    map[string]any{"type": "string", "description": "Initial prompt for the sub-agent."},
				"provider":  map[string]any{"type": "string", "description": "Override provider (optional, defaults to parent run's provider)."},
				"dependsOn": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Run IDs this agent depends on."},
				"wait":      map[string]any{"type": "boolean", "description": "If true, block until the sub-agent's turn completes and return its final message."},
			},
			"required": []any{"agent", "prompt", "wait"},
		},
	}
}

func (a *codexAdapter) Key() ProviderKey { return ProviderKeyCodex }

func (a *codexAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
		SkillSelection: true, Mcp: true, Interrupt: true, Vision: true,
	}
}

func (a *codexAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	sandbox, approvalMode := codexYoloDerive(req.YoloMode)

	// Per-thread cwd is authoritative (04-06 multi-workspace): the run's cwd takes
	// precedence over the adapter default.
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}

	// Register the FlowPilot-owned ask_user tool as a thread dynamicTool (04-04): the model
	// discovers it at session start and a call returns as an item/tool/call request.
	//
	// Once a real rollout id exists, follow-up turns must rejoin that thread via app-server
	// `thread/resume` so approval/MCP/ask_user bridging stays available on resumed turns.
	dynamicTools := []any{codexAskUserDynamicTool(), codexSpawnAgentDynamicTool()}
	threadMethod := "thread/start"
	threadParams := codexThreadStartParams(cwd, sandbox, approvalMode, req.ModelName, req.ReasoningEffort, dynamicTools)
	if resumeID := strings.TrimSpace(req.ProviderSessionID); resumeID != "" && !strings.HasPrefix(resumeID, "thread-") {
		threadMethod = "thread/resume"
		threadParams = codexThreadResumeParams(resumeID, cwd, sandbox, approvalMode, req.ModelName, req.ReasoningEffort)
	}

	startRes, err := a.dispatcher.call(ctx, threadMethod, threadParams)
	if err != nil {
		return err
	}
	threadID := codexThreadIDFromResponse(startRes)
	if threadID == "" {
		return fmt.Errorf("codex thread/start returned no threadId")
	}

	notif, err := a.dispatcher.registerThread(threadID)
	if err != nil {
		return err
	}
	defer a.dispatcher.unregisterThread(threadID)

	a.mu.Lock()
	a.bridges[threadID] = bridge
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, threadID)
		delete(a.codexTurns, threadID)
		a.mu.Unlock()
	}()

	// Persist image attachments to a per-turn temp dir on the runner host so Codex can
	// read them by path (Task-052, D-3). Cleanup is deferred to turn return: SendTurn
	// blocks on the event pump below until a terminal event or interrupt, by which point
	// the app-server has already read the files — so deletion never races a read. A boot
	// sweep (sweepCodexImageAttachments) reclaims any orphans left by a hard crash.
	imagePaths, cleanupImages, err := writeCodexImageAttachments(req.ProviderTurnID, req.Attachments)
	if err != nil {
		return err
	}
	defer cleanupImages()

	// Runner-side prompt assembly before turn/start (04-03): skill reinforcement +
	// ask_user usage reinforcement. The pluggable promptPrep hook lets the live
	// process adapter inject full skill content (injectSkillContent) and required-MCP
	// instructions (preparePromptForRequiredMcps); the default reinforces ask_user.
	prompt := a.preparePrompt(req)

	// Fire turn/start; rely on notifications for completion (don't block the pump).
	turnErr := make(chan error, 1)
	go func() {
		res, e := a.dispatcher.call(ctx, "turn/start", codexTurnStartParams(threadID, prompt, req.SelectedSkills, imagePaths))
		if e == nil {
			if tid := codexTurnIDFromResponse(res); tid != "" {
				a.mu.Lock()
				a.codexTurns[threadID] = tid
				a.mu.Unlock()
			}
		}
		turnErr <- e
	}()

	for {
		select {
		case <-ctx.Done():
			// interrupt: best-effort interrupt of the in-flight turn (04-04 owns full semantics)
			_ = a.dispatcher.notify("turn/interrupt", codexInterruptParams(threadID, a.codexTurnID(threadID)))
			return ctx.Err()

		case e := <-turnErr:
			if e != nil {
				return e
			}
			turnErr = nil // ack received; disable this branch and keep pumping to terminal

		case n, ok := <-notif:
			if !ok {
				// dispatcher drained this thread → the shared stream died mid-turn
				return fmt.Errorf("codex app-server stream closed mid-turn")
			}
			ev, mapped := mapCodexNotification(n)
			if !mapped {
				continue
			}
			ev.ProviderTurnID = "" // let runner core stamp the canonical turn id
			bridge.Emit(ev)
			if ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed {
				return nil
			}
		}
	}
}

func (a *codexAdapter) codexTurnID(threadID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.codexTurns[threadID]
}

// handleInbound routes a server→client request (the third dispatcher category).
// Approval requests are forwarded through the active turn's bridge and the chosen
// decision is replied back to Codex, so the model unblocks.
func (a *codexAdapter) handleInbound(req codexInboundRequest) {
	// Dynamic-tool call (the ask_user structured question): the model invoked a thread
	// dynamicTool, delivered as `item/tool/call` with DynamicToolCallParams. Route it to
	// the user-interaction bridge and reply with the answer as a DynamicToolCallResponse.
	if req.Method == "item/tool/call" {
		a.handleDynamicToolCall(req)
		return
	}
	if !codexInboundApprovalMethod(req.Method) {
		_ = a.dispatcher.replyError(req.ID, "unsupported Codex inbound request: "+req.Method)
		return
	}

	threadID := codexThreadIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[threadID]
	a.mu.Unlock()
	if bridge == nil {
		_ = a.dispatcher.replyError(req.ID, "no active turn for thread")
		return
	}

	details := codexApprovalDetails(req.Method, req.Params)
	decision, err := bridge.RequestApproval(details)
	if err != nil {
		// expiry/interrupt while pending → deny so Codex never hangs (04-04)
		_ = a.dispatcher.reply(req.ID, codexApprovalResponse(req.Method, req.Params, "deny"))
		return
	}
	_ = a.dispatcher.reply(req.ID, codexApprovalResponse(req.Method, req.Params, decision))
}

// handleDynamicToolCall services an `item/tool/call` for the ask_user tool: it forwards the
// structured question to the active turn's bridge (pause → user_question_required → options
// card → resume) and replies with a DynamicToolCallResponse. Unknown tool / no bridge / a
// bridge error all reply success=false so the model gets a result and never hangs.
func (a *codexAdapter) handleDynamicToolCall(req codexInboundRequest) {
	threadID := codexThreadIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[threadID]
	a.mu.Unlock()

	tool, _ := req.Params["tool"].(string)
	if bridge == nil {
		_ = a.dispatcher.reply(req.ID, codexDynamicToolResult("Tool is not available.", false))
		return
	}
	switch tool {
	case "ask_user":
		prompt, options, multi := codexAskUserArgs(req.Params)
		choice, err := bridge.AskQuestion(prompt, options, multi)
		if err != nil || len(choice) == 0 {
			// expiry/interrupt or no answer → return a result so the model continues.
			_ = a.dispatcher.reply(req.ID, codexDynamicToolResult("No answer was provided.", false))
			return
		}
		_ = a.dispatcher.reply(req.ID, codexDynamicToolResult(strings.Join(choice, ", "), true))
	case "spawn_agent":
		args, _ := req.Params["arguments"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		in, parseErr := parseSpawnAgentInput(args)
		if parseErr != nil {
			_ = a.dispatcher.reply(req.ID, codexDynamicToolResult(parseErr.Error(), false))
			return
		}
		result, spawnErr := bridge.SpawnAgent(in)
		if spawnErr != nil {
			_ = a.dispatcher.reply(req.ID, codexDynamicToolResult("spawn_agent failed: "+spawnErr.Error(), false))
			return
		}
		resultJSON, _ := json.Marshal(result)
		_ = a.dispatcher.reply(req.ID, codexDynamicToolResult(string(resultJSON), true))
	default:
		_ = a.dispatcher.reply(req.ID, codexDynamicToolResult("Tool is not available.", false))
	}
}

// codexAskUserArgs parses DynamicToolCallParams.arguments into AskQuestion inputs. Options
// may be plain strings or {label, description} objects (mirrors claudeAskUserParams).
func codexAskUserArgs(params map[string]any) (string, []QuestionOption, bool) {
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		return "", nil, false
	}
	prompt, _ := args["prompt"].(string)
	multi, _ := args["multiSelect"].(bool)
	var opts []QuestionOption
	if raw, ok := args["options"].([]any); ok {
		for _, o := range raw {
			switch v := o.(type) {
			case string:
				opts = append(opts, QuestionOption{Label: v, Value: v})
			case map[string]any:
				label, _ := v["label"].(string)
				desc, _ := v["description"].(string)
				opts = append(opts, QuestionOption{Label: label, Description: desc, Value: label})
			}
		}
	}
	return prompt, opts, multi
}

// codexDynamicToolResult builds the DynamicToolCallResponse reply shape
// ({ contentItems:[{type:"inputText",text}], success }).
func codexDynamicToolResult(text string, success bool) map[string]any {
	return map[string]any{
		"contentItems": []any{map[string]any{"type": "inputText", "text": text}},
		"success":      success,
	}
}

func codexApprovalDetails(method string, params map[string]any) ApprovalDetails {
	str := func(k string) string {
		if params == nil {
			return ""
		}
		s, _ := params[k].(string)
		return s
	}
	details := ApprovalDetails{
		Command: str("command"),
		Cwd:     str("cwd"),
		Reason:  str("reason"),
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
	if method == "mcpServer/elicitation/request" {
		serverName := str("serverName")
		message := str("message")
		if serverName != "" {
			details.Command = "MCP server: " + serverName
		}
		details.Reason = message
	}
	return details
}

func codexInboundApprovalMethod(method string) bool {
	switch method {
	case "approval/request",
		"execCommandApproval",
		"applyPatchApproval",
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
		"item/permissions/requestApproval",
		"mcpServer/elicitation/request":
		return true
	default:
		return false
	}
}

func codexApprovalResponse(method string, params map[string]any, decision string) map[string]any {
	switch method {
	case "item/permissions/requestApproval":
		return codexPermissionsApprovalResponse(params, decision)
	case "mcpServer/elicitation/request":
		return codexMcpElicitationApprovalResponse(decision)
	default:
		return map[string]any{"decision": codexReviewDecision(method, decision)}
	}
}

// codexReviewDecision maps FlowPilot's internal approval vocabulary to the
// decision enum expected by the specific Codex app-server request method.
func codexReviewDecision(method string, decision string) string {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		return codexV2ReviewDecision(decision)
	default:
		return codexLegacyReviewDecision(decision)
	}
}

// codexPermissionsApprovalResponse maps Codex v2 permission approval requests to
// the response shape expected by item/permissions/requestApproval:
// { permissions, scope }. Approve grants the requested per-turn profile; deny
// returns an empty profile so Codex can continue without the extra permission.
func codexPermissionsApprovalResponse(params map[string]any, decision string) map[string]any {
	if !codexDecisionApproved(decision) {
		return map[string]any{
			"permissions": map[string]any{},
			"scope":       "turn",
		}
	}
	permissions, _ := params["permissions"].(map[string]any)
	granted := map[string]any{}
	if network, ok := permissions["network"].(map[string]any); ok && network != nil {
		granted["network"] = network
	}
	if fileSystem, ok := permissions["fileSystem"].(map[string]any); ok && fileSystem != nil {
		granted["fileSystem"] = fileSystem
	}
	scope := "turn"
	if decision == "approve_for_session" || decision == "approved_for_session" {
		scope = "session"
	}
	return map[string]any{
		"permissions": granted,
		"scope":       scope,
	}
}

func codexDecisionApproved(decision string) bool {
	switch decision {
	case "approve", "approved", "approve_for_session", "approved_for_session", "accept", "acceptForSession":
		return true
	default:
		return false
	}
}

// codexMcpElicitationApprovalResponse maps MCP server elicitations to the
// app-server response shape: { action, content, _meta }.
func codexMcpElicitationApprovalResponse(decision string) map[string]any {
	action := "decline"
	if codexDecisionApproved(decision) {
		action = "accept"
	} else if decision == "abort" || decision == "cancel" {
		action = "cancel"
	}
	return map[string]any{
		"action":  action,
		"content": nil,
		"_meta":   nil,
	}
}

// codexLegacyReviewDecision maps FlowPilot's internal approval vocabulary (approve/deny,
// the values the desktop card and the runner bridge speak) to Codex's app-server
// `ReviewDecision` serde values: approved | approved_for_session | denied | abort.
// The real app-server cannot deserialize "approve"/"deny" — it rejects the inbound
// approval as if the user declined (BUG-064; BUG-066 covers the v2 enum). Any
// unknown/empty decision fails safe to "denied".
func codexLegacyReviewDecision(decision string) string {
	switch decision {
	case "approve", "approved":
		return "approved"
	case "approve_for_session", "approved_for_session":
		return "approved_for_session"
	case "deny", "denied":
		return "denied"
	case "abort":
		return "abort"
	default:
		return "denied"
	}
}

// codexV2ReviewDecision maps FlowPilot's internal values to the v2 command/file
// approval enums used by item/commandExecution/requestApproval and
// item/fileChange/requestApproval: accept | acceptForSession | decline | cancel.
func codexV2ReviewDecision(decision string) string {
	switch decision {
	case "approve", "approved", "accept":
		return "accept"
	case "approve_for_session", "approved_for_session", "acceptForSession":
		return "acceptForSession"
	case "deny", "denied", "decline":
		return "decline"
	case "abort", "cancel":
		return "cancel"
	default:
		return "decline"
	}
}

// codexYoloDerive maps YOLO → Codex thread params via the SSOT resolver
// (yolo_resolver.go, P4). The same posture drives the runner approval bridge, so
// the two layers can never drift.
func codexYoloDerive(yolo bool) (sandbox, approvalMode string) {
	p := resolveYoloPosture(yolo)
	return p.CodexSandbox, p.CodexApprovalMode
}
