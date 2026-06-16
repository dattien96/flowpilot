package runner

import (
	"context"
	"fmt"
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
	dispatcher        *codexDispatcher
	cwd               string
	defaultMcpServers []any

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

// askUserReinforcement is appended to every prompt so the model knows the
// FlowPilot-owned ask_user tool exists and when to use it (best-effort, 04-04).
const askUserReinforcement = "\n\n---\nIf you need a decision or clarification before continuing, call the `ask_user` tool (prompt, options[], multiSelect?) instead of guessing."

// preparePrompt builds the final turn prompt. The default applies ask_user
// reinforcement; promptPrep (when set) replaces it with full runner-side assembly.
func (a *codexAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt + askUserReinforcement
}

// codexAskUserMcpServer is the registration entry for the FlowPilot-owned ask_user
// custom MCP tool, carried on thread/start so the provider lists it via tools/list.
func codexAskUserMcpServer() any {
	return map[string]any{
		"name": "flowpilot",
		"tools": []any{
			map[string]any{
				"name":        "ask_user",
				"description": "Ask the user a structured question and wait for their answer before continuing. Use when you need a decision or clarification.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"prompt":      map[string]any{"type": "string"},
						"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"multiSelect": map[string]any{"type": "boolean"},
					},
					"required": []any{"prompt"},
				},
			},
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

	// Register the FlowPilot proxy + required MCPs + the ask_user custom tool on the
	// thread (04-04): the model discovers ask_user via tools/list at session start.
	mcpServers := append(append([]any{}, a.defaultMcpServers...), codexAskUserMcpServer())

	startRes, err := a.dispatcher.call(ctx, "thread/start", codexThreadStartParams(cwd, sandbox, approvalMode, req.ModelName, req.ReasoningEffort, mcpServers))
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

	var skill *SkillSelection
	if len(req.SelectedSkills) > 0 {
		skill = &req.SelectedSkills[0]
	}

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
		res, e := a.dispatcher.call(ctx, "turn/start", codexTurnStartParams(threadID, prompt, skill, imagePaths))
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
