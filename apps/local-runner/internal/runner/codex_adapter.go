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
		SkillSelection: true, Mcp: true, Interrupt: true,
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

	startRes, err := a.dispatcher.call(ctx, "thread/start", codexThreadStartParams(cwd, sandbox, approvalMode, mcpServers))
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

	// Runner-side prompt assembly before turn/start (04-03): skill reinforcement +
	// ask_user usage reinforcement. The pluggable promptPrep hook lets the live
	// process adapter inject full skill content (injectSkillContent) and required-MCP
	// instructions (preparePromptForRequiredMcps); the default reinforces ask_user.
	prompt := a.preparePrompt(req)

	// Fire turn/start; rely on notifications for completion (don't block the pump).
	turnErr := make(chan error, 1)
	go func() {
		res, e := a.dispatcher.call(ctx, "turn/start", codexTurnStartParams(threadID, prompt, skill))
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
	threadID := codexThreadIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[threadID]
	a.mu.Unlock()
	if bridge == nil {
		_ = a.dispatcher.replyError(req.ID, "no active turn for thread")
		return
	}

	str := func(k string) string {
		if req.Params == nil {
			return ""
		}
		s, _ := req.Params[k].(string)
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
	decision, err := bridge.RequestApproval(details)
	if err != nil {
		// expiry/interrupt while pending → deny so Codex never hangs (04-04)
		_ = a.dispatcher.reply(req.ID, map[string]any{"decision": "deny"})
		return
	}
	_ = a.dispatcher.reply(req.ID, map[string]any{"decision": decision})
}

// codexYoloDerive maps YOLO → Codex thread params via the SSOT resolver
// (yolo_resolver.go, P4). The same posture drives the runner approval bridge, so
// the two layers can never drift.
func codexYoloDerive(yolo bool) (sandbox, approvalMode string) {
	p := resolveYoloPosture(yolo)
	return p.CodexSandbox, p.CodexApprovalMode
}
