package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Task-207 (CP-46 P-3): the Grok provider adapter. Implements the
// provider-neutral ProviderRuntimeAdapter on top of the Task-206 dispatcher.
// Modeled on codexAdapter (codex_adapter.go) for the bridge-map/inbound-routing
// shape, but the turn loop itself is Gemini-ACP-flavored: `session/new` +
// `session/prompt` instead of `thread/start` + `turn/start`, and — crucially —
// Grok's `session/prompt` call blocks for the whole turn and its own response
// carries the terminal stopReason + token usage (live-verified), so there is no
// separate "turn completed" notification to wait for.
type grokAdapter struct {
	dispatcher *grokDispatcher
	cwd        string
	grokHome   string
	initResult map[string]any

	// promptPrep assembles the final prompt before session/prompt. Defaults to
	// raw req.Prompt; the live process registration (provider_registry.go,
	// Task-212) overrides it to run the shared skill/context injection path.
	promptPrep func(TurnRequest) string

	// sessionStore persists the real ACP sessionId for cross-restart resume
	// (Task-207 T-5). nil is a valid no-op (ProviderSessionStoreFor degrades to
	// noopProviderSessionStore when Supabase is unconfigured).
	sessionStore ProviderSessionStore

	// mcpServer/mcpBaseURL/extraMCPServers wire FlowPilot's runner-hosted MCP
	// tools (ask_user/spawn_agent/submit_review_outcome) + any configured
	// external server (Google Drive) into ACP's real mcpServers[] field
	// (Task-209 T-1/T-2). Reused, not mutated, from the Claude/Codex path
	// (claude_mcp_server.go, google_drive_mcp_provider_config.go) — CP-46 P-0.
	// All nil is a valid no-op (Task-207 MVP scope: no MCP servers at all).
	mcpServer       *claudeMCPServer
	mcpBaseURL      func() string
	extraMCPServers func(yolo bool) map[string]claudeMcpServer

	mu      sync.Mutex
	bridges map[string]TurnBridge // sessionId -> active turn bridge
	// allowReviewOutcome tracks, per sessionId, whether this turn actually
	// advertised submit_review_outcome (BUG-NOTE-CP42 #24 defense in depth,
	// mirrors codexAdapter.allowReviewOutcome).
	allowReviewOutcome map[string]bool
	// yoloModes tracks, per sessionId, the YOLO posture this turn started with
	// (Task-208): handleInbound reads it to decide whether an inbound
	// session/request_permission auto-approves via runner policy or blocks on
	// bridge.RequestApproval. Deliberately NOT derived from anything Grok's own
	// process/config reports (config.toml permission_mode, a stale
	// always-approve toggle) — mirrors the Claude BUG-069/CA-079 fix: the
	// runner's own YOLO toggle is the only source of truth.
	yoloModes map[string]bool
}

func newGrokAdapter(dispatcher *grokDispatcher, cwd string) *grokAdapter {
	a := &grokAdapter{
		dispatcher:         dispatcher,
		cwd:                cwd,
		bridges:            map[string]TurnBridge{},
		allowReviewOutcome: map[string]bool{},
		yoloModes:          map[string]bool{},
	}
	dispatcher.setInbound(a.handleInbound)
	return a
}

func (a *grokAdapter) Key() ProviderKey { return ProviderKeyGrok }

// Capabilities advertises only what has a passing test (CP-46 P-11): Streaming/
// Resume/FileEvents/Interrupt (Task-207), ApprovalEvents (Task-208 — the real
// session/request_permission decision policy below). Mcp lands in Task-209;
// SkillSelection lands in Task-214 (promptPrep already calls
// injectSelectedSkills unconditionally, so unlike Mcp it needs no
// instance-wiring check). Vision stays false (initialize reported
// promptCapabilities.image=false, live-verified).
func (a *grokAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming:      true,
		Resume:         true,
		FileEvents:     true,
		Interrupt:      true,
		ApprovalEvents: true,
		SkillSelection: true,
		// Mcp is only true once this instance is actually wired to the
		// runner-hosted MCP server (Task-209) — a bare newGrokAdapter() (tests,
		// or a not-yet-registered instance) truthfully reports false (CP-46 P-11).
		Mcp: a.mcpServer != nil,
	}
}

func (a *grokAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt
}

func (a *grokAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}

	// Task-209: register this turn's bridge on the runner-hosted FlowPilot MCP
	// server (same mechanism Claude/Codex use — reused, not mutated) and build
	// the ACP mcpServers[] entries for it plus any configured external server
	// (Google Drive). A nil mcpServer keeps Task-207's MVP no-MCP behavior.
	var mcpToken string
	var mcpServers []interface{}
	if a.mcpServer != nil {
		mcpToken = a.mcpServer.register(bridge, req.OfferReviewOutcomeTool)
		defer a.mcpServer.unregister(mcpToken)
		baseURL := ""
		if a.mcpBaseURL != nil {
			baseURL = a.mcpBaseURL()
		}
		mcpServers = append(mcpServers, grokACPFlowPilotMCPServers(baseURL, mcpToken)...)
		if a.extraMCPServers != nil {
			mcpServers = append(mcpServers, grokACPExtraMCPServers(a.extraMCPServers(req.YoloMode))...)
		}
	}

	sessionID, err := a.ensureSession(ctx, req, cwd, mcpServers)
	if err != nil {
		return err
	}

	// MCP-ready-before-prompt gate (Task-209 T-7, BUG-114 class): Grok connects
	// mcpServers asynchronously (live-verified: `_x.ai/mcp/init_progress`), so
	// without this the first turn can drop ask_user/spawn_agent. Bounded by the
	// same default timeout Claude uses; degrades to sending anyway on timeout
	// rather than hanging the turn.
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
	a.yoloModes[sessionID] = req.YoloMode
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, sessionID)
		delete(a.allowReviewOutcome, sessionID)
		delete(a.yoloModes, sessionID)
		a.mu.Unlock()
	}()

	prompt := a.preparePrompt(req)
	promptParams := grokACPPromptParams(sessionID, prompt)

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
			// Best-effort ACP cancel; call()'s own ctx-select already returns
			// ctx.Err() to the goroutine above, so the turn does not hang even if
			// this notify is dropped.
			_ = a.dispatcher.notify("session/cancel", map[string]any{"sessionId": sessionID})
			return ctx.Err()

		case outcome := <-done:
			if outcome.err != nil {
				return outcome.err
			}
			return a.emitTerminal(ctx, req, bridge, sessionID, outcome.result, lastText)

		case n, ok := <-notif:
			if !ok {
				return fmt.Errorf("grok agent stdio stream closed mid-turn")
			}
			events, mapped := mapGrokNotification(n)
			if !mapped {
				continue
			}
			for _, ev := range events {
				if ev.Type == EventMessageDelta {
					lastText += ev.Text
				}
				bridge.Emit(ev)
			}
		}
	}
}

// ensureSession creates a fresh ACP session (`session/new`) or resumes one
// (`session/load`) when req.ProviderSessionID is a real (non-empty,
// non-synthetic) ACP session id. A resume attempt with an id this adapter has
// no record of is still passed through to session/load as-is (Grok, not
// FlowPilot, is the source of truth for whether that id is resumable).
//
// The "thread-" prefix check below guards against FlowPilot's own synthetic
// per-run placeholder id (assigned before any real provider session exists —
// same shape codex_adapter.go already guards against at its thread/resume
// call). Live-verified bug (2026-07-10): the caller does NOT reliably filter
// this out for Grok — a brand-new run's first turn arrived here with
// ProviderSessionID="thread-<n>", which went straight into session/load and
// Grok correctly replied with a hard error (FS_NOT_FOUND / "Path not found.",
// since no session by that id had ever been created) instead of silently
// starting a new one. Do not remove this guard on the assumption the caller
// handles it.
func (a *grokAdapter) ensureSession(ctx context.Context, req TurnRequest, cwd string, mcpServers []interface{}) (string, error) {
	resumeID := strings.TrimSpace(req.ProviderSessionID)
	method := "session/new"
	var result map[string]any
	var err error
	if resumeID != "" && !strings.HasPrefix(resumeID, "thread-") {
		method = "session/load"
		result, err = a.dispatcher.call(ctx, method, grokACPSessionLoadParams(resumeID, cwd, mcpServers))
	} else {
		result, err = a.dispatcher.call(ctx, method, grokACPSessionNewParams(cwd, mcpServers))
	}
	if err != nil {
		return "", err
	}
	sessionID := grokACPResponseSessionID(map[string]interface{}{"result": result})
	if sessionID == "" {
		return "", fmt.Errorf("grok %s returned no sessionId", method)
	}
	a.recordSession(ctx, req, sessionID)
	return sessionID, nil
}

// recordSession persists the real ACP sessionId (Task-207 T-5).
func (a *grokAdapter) recordSession(ctx context.Context, req TurnRequest, sessionID string) {
	if a.sessionStore == nil {
		return
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyGrok),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  a.cwd,
		ModelName:         req.ModelName,
		Status:            "active",
	})
}

// emitTerminal builds and emits the turn_completed/turn_failed event from a
// session/prompt result (GR-32: if the result's sessionId differs from the one
// this turn started with — e.g. Grok forked/rotated it — the new id is what
// gets persisted as the adopted session id).
func (a *grokAdapter) emitTerminal(ctx context.Context, req TurnRequest, bridge TurnBridge, sessionID string, result map[string]any, lastText string) error {
	if result == nil {
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "grok session/prompt returned no result", Recoverable: true})
		return nil
	}
	if adopted := grokACPResponseSessionID(map[string]interface{}{"result": result}); adopted != "" && adopted != sessionID {
		a.recordSession(ctx, req, adopted)
	}

	meta, _ := result["_meta"].(map[string]any)
	if usage := grokPromptResultTokenUsage(meta, grokContextWindowFromInit(a.initResult)); usage != nil {
		bridge.Emit(ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: usage})
	}

	stopReason, _ := result["stopReason"].(string)
	finalText := grokACPPromptResultText(result)
	if finalText == "" {
		finalText = lastText
	}
	if strings.EqualFold(stopReason, "refusal") || strings.EqualFold(stopReason, "error") {
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: fmt.Sprintf("grok turn ended: %s", stopReason), Recoverable: false})
		return nil
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalText})
	return nil
}

// handleInbound routes a server->client request. session/request_permission
// (Task-208) is the only inbound method Grok's ACP surface uses for tool-call
// gating (live-verified); anything else replies with a controlled error
// rather than leaving Grok's own request hanging. The dispatcher already
// invokes this handler on its own goroutine per inbound frame (grok_process.go
// dispatch(): `go d.inbound(...)`), so N concurrent gated tool calls each
// resolve independently without wedging the read loop or each other (GR-29).
func (a *grokAdapter) handleInbound(req grokInboundRequest) {
	if req.Method != "session/request_permission" {
		_ = a.dispatcher.replyError(req.ID, "unsupported grok inbound request: "+req.Method)
		return
	}
	sessionID := grokSessionIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[sessionID]
	yolo := a.yoloModes[sessionID]
	a.mu.Unlock()

	options, _ := req.Params["options"].([]any)
	if bridge == nil {
		_ = a.dispatcher.reply(req.ID, grokPermissionOutcomeResponse(grokEncodePermissionDecision(options, false)))
		return
	}

	details := grokApprovalDetailsFromRequest(req.Params)

	// YOLO=true: still process the request through this same channel (never
	// disable it) but auto-answer with the runner's own auto-approve decision
	// (Task-208 T-4) — the same "runner policy, not the provider's own bypass"
	// discipline as Codex/Claude. ask_user is a distinct MCP/native-tool path
	// (Task-209), never routed here, so unconditionally approving every
	// session/request_permission under YOLO can never auto-answer a question.
	if yolo {
		_ = a.dispatcher.reply(req.ID, grokPermissionOutcomeResponse(grokEncodePermissionDecision(options, true)))
		return
	}

	decision, err := bridge.RequestApproval(details)
	if err != nil {
		// expiry/interrupt while pending -> deny so Grok never hangs.
		_ = a.dispatcher.reply(req.ID, grokPermissionOutcomeResponse(grokEncodePermissionDecision(options, false)))
		return
	}
	approve := decision == "approve" || decision == "approved" || decision == "approve_for_session"
	_ = a.dispatcher.reply(req.ID, grokPermissionOutcomeResponse(grokEncodePermissionDecision(options, approve)))
}
