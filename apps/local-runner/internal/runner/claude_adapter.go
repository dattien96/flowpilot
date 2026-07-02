package runner

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Phase 2 (07 plan): the Claude provider adapter. Implements the provider-neutral
// ProviderRuntimeAdapter (P2 contract) on top of the stream-json transport + event
// mapper. Per turn it spawns/uses a `claude -p` process for the run's (account, cwd,
// session), writes the user turn, pumps mapped events to the bridge, and routes inbound
// server→client control_requests (permission / ask_user) back through the bridge.
//
// In this environment the real `claude` binary need not run: the adapter is driven over
// a scripted fake process in tests (claude_adapter_test.go). The live registry stays on
// the placeholder until validated against a real build (07 Appendix A spike).
type claudeAdapter struct {
	pool      *claudeProcessPool
	cwd       string            // default cwd; req.Cwd is authoritative (04-06)
	scopeKey  string            // active provider account id (pool/process scope)
	env       map[string]string // provider auth/env (ANTHROPIC_API_KEY / CLAUDE_CONFIG_DIR / …)
	mcpConfig string            // --mcp-config path (empty until the live MCP server is wired)

	// sessionStore durably persists the (run, cwd, real session id) mapping for
	// cross-restart resume (07). nil/no-op when Supabase is unconfigured (offline/tests).
	sessionStore ProviderSessionStore

	// mcpServer + mcpBaseURL wire the per-turn permission MCP (07): on a gated turn the
	// adapter registers the bridge under a token and points claude's --mcp-config at the
	// runner's MCP endpoint with that token. nil in offline/tests (the in-stream
	// control_request handler remains as a fallback).
	mcpServer  *claudeMCPServer
	mcpBaseURL func() string

	// mcpReadyTimeout bounds how long SendTurn withholds the prompt waiting for claude's
	// MCP client to connect (tools/list). 0 => claudeMCPReadyDefaultTimeout. Tests set a
	// small value because the scripted fake process never connects to the MCP server.
	mcpReadyTimeout time.Duration

	// extraMCPServers returns FlowPilot-managed MCP servers (e.g. google-drive) to merge
	// into the per-turn --mcp-config alongside the flowpilot permission server. Needed
	// because --strict-mcp-config makes claude ignore the account's .claude.json mcpServers.
	// nil in offline/tests. yolo is the turn's resolved mode (drives proxy approval/token).
	extraMCPServers func(yolo bool) map[string]claudeMcpServer

	// promptPrep assembles the final prompt before the turn. Defaults to ask_user
	// reinforcement; the live registry overrides it to also run injectSkillContent.
	promptPrep func(TurnRequest) string

	mu      sync.Mutex
	bridges map[*claudeProcess]TurnBridge
}

func newClaudeAdapter(pool *claudeProcessPool, cwd, scopeKey string, env map[string]string, mcpConfig string) *claudeAdapter {
	return &claudeAdapter{
		pool: pool, cwd: cwd, scopeKey: scopeKey, env: env, mcpConfig: mcpConfig,
		bridges: map[*claudeProcess]TurnBridge{},
	}
}

// claudeAskUserReinforcement mirrors the Codex askUserReinforcement: it biases the model
// toward ACTING — complete the clear parts of the task first (normal tools + approval gates
// apply) and reserve ask_user for a required decision that genuinely blocks progress, so the
// model does not front-load clarifying questions instead of doing obvious work (best-effort, 04-04).
const claudeAskUserReinforcement = "\n\n---\nComplete the clear, unambiguous parts of the task directly — your normal tools and approval gates still apply. Only call the `ask_user` tool (prompt, options[], multiSelect?) when a required decision genuinely blocks you and you cannot reasonably infer the answer or make progress without it; do not use it for things you can do or reasonably assume first."

func (a *claudeAdapter) Key() ProviderKey { return ProviderKeyClaude }

func (a *claudeAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
		SkillSelection: true, Mcp: true, Interrupt: true, Vision: true,
	}
}

func (a *claudeAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt + claudeAskUserReinforcement
}

func (a *claudeAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	posture := resolveYoloPosture(req.YoloMode)

	// Resume ONLY with the real Claude session id captured on a prior turn — never the
	// synthetic FlowPilot run session id (review finding 1). Empty on the first turn.
	resumeID := a.pool.realSession(req.ProviderSessionID)

	// Stand up the per-turn MCP config so claude can reach FlowPilot's approve + ask_user
	// tools. Done for ALL YOLO states — not only YOLO=false — so that ask_user works in
	// YOLO=true turns too. The approval gate (--permission-prompt-tool) is added separately
	// in claudeArgs only when YOLO=false. Falls back to a.mcpConfig (offline/tests) when
	// the server or base URL is unavailable.
	mcpConfig := a.mcpConfig
	var mcpToken string // non-empty only when a per-turn MCP config was actually written
	if a.mcpServer != nil {
		base := ""
		if a.mcpBaseURL != nil {
			base = a.mcpBaseURL()
		}
		if base == "" {
			if !posture.RunnerAutoApprove {
				// Gated turn: approval MCP is mandatory — fail closed (review finding 4).
				return fmt.Errorf("claude gated turn: runner MCP base URL not configured (fail closed)")
			}
			// YOLO=true: ask_user falls back to in-stream control_request path.
			// spawn_agent has no in-stream fallback — it won't be available for this turn.
			log.Printf("[claude-mcp] MCP base URL not set; FlowPilot tools (spawn_agent, ask_user) are unavailable for this turn — check runner startup")
		} else {
			var extra map[string]claudeMcpServer
			if a.extraMCPServers != nil {
				extra = a.extraMCPServers(req.YoloMode)
			}
			token := a.mcpServer.register(bridge, req.OfferReviewOutcomeTool)
			defer a.mcpServer.unregister(token)
			path, cleanup, err := writeClaudeMCPConfig(base, token, extra)
			if err != nil {
				if !posture.RunnerAutoApprove {
					return fmt.Errorf("claude gated turn: could not set up permission MCP (fail closed): %w", err)
				}
				// YOLO=true: proceed without per-turn MCP config; ask_user unavailable.
			} else {
				mcpConfig = path
				mcpToken = token // gate the prompt on this token's MCP connection below
				defer cleanup()
			}
		}
	}
	args := claudeArgs(posture, resumeID, mcpConfig, req.ModelName, req.ReasoningEffort, req.SelectedSkills)
	key := claudeProcKey{account: a.scopeKey, cwd: cwd, session: req.ProviderSessionID}

	proc, err := a.pool.spawn(ctx, key, args, a.env, cwd)
	if err != nil {
		return err
	}
	defer a.pool.release(proc)

	// Turn-scoped context so inbound (approval/question) goroutines are abandoned on
	// every return path and never block the pump or write to a released process
	// (review finding 5).
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.mu.Lock()
	a.bridges[proc] = bridge
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, proc)
		a.mu.Unlock()
	}()

	// Withhold the prompt until claude's MCP client has connected (tools/list fetched).
	// claude connects --mcp-config servers asynchronously, so delivering the prompt
	// immediately races the connection and the FIRST turn's tool set omits ask_user (and
	// any other FlowPilot MCP tool). The runner hosts the MCP server, so it knows exactly
	// when claude connects; bounded by a timeout so a slow/failed connect degrades to
	// sending anyway rather than hanging the turn.
	if mcpToken != "" {
		if !a.mcpServer.waitReady(turnCtx, mcpToken, a.mcpReadyTimeout) {
			log.Printf("[claude-mcp] MCP connection did not become ready within timeout; FlowPilot tools (spawn_agent, ask_user) may be missing from this turn — the prompt will be delivered anyway")
		}
	}

	if err := proc.stream.writeUserTurn(a.preparePrompt(req), req.Attachments); err != nil {
		return err
	}

	var capturedSession string
	for {
		select {
		case <-ctx.Done():
			// interrupt: release() (deferred) kills+reaps the process; finishTurn maps
			// ctx.Canceled to a clean cancel (07 W7 / interactive_service.finishTurn).
			return ctx.Err()

		case line, ok := <-proc.stream.lines:
			if !ok {
				// stream died mid-turn → recoverable (the turn can be re-sent, 07 W7).
				if e := proc.stream.getCloseErr(); e != nil {
					return e
				}
				return fmt.Errorf("claude stream closed mid-turn")
			}
			// Capture Claude's real session id (system/init|result) so later turns can
			// --resume the real session instead of the synthetic FlowPilot id, and persist
			// the (run, cwd, session) mapping once for cross-restart resume (07).
			if sid := claudeSessionIDFromLine(line); sid != "" {
				a.pool.setRealSession(req.ProviderSessionID, sid)
				if capturedSession == "" {
					capturedSession = sid
					a.persistSession(req, cwd, sid)
				}
			}
			if line.Type == "control_request" {
				// blocking handler on its own goroutine so a pending approval/question
				// never stalls the pump; bound to turnCtx so it cannot outlive the turn.
				go a.handleInbound(turnCtx, proc, bridge, line)
				continue
			}
			for _, ev := range mapClaudeLine(line) {
				ev.ProviderTurnID = "" // runner core stamps the canonical turn id + seq
				bridge.Emit(ev)
				if ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed {
					return nil
				}
			}
		}
	}
}

// persistSession durably records the (run, cwd, real session id) mapping so the run can
// resume after a runner restart (07). Fire-and-forget with its own bounded context so it
// neither blocks the pump nor is cancelled when the turn ends; no-op when no store is
// wired (offline/tests) or Supabase is unconfigured. workflow_step_run_id is intentionally
// omitted (req.StepID is a step-definition id, not the run-step uuid the FK expects).
func (a *claudeAdapter) persistSession(req TurnRequest, cwd, claudeSessionID string) {
	if a.sessionStore == nil {
		return
	}
	store := a.sessionStore
	rec := ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyClaude),
		ProviderSessionID: claudeSessionID,
		ProviderThreadID:  claudeSessionID,
		WorkingDirectory:  cwd,
		Status:            "active",
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = store.UpsertSession(ctx, rec)
	}()
}

// handleInbound routes a server→client control_request through the bridge and replies so
// the engine unblocks. Permission requests map to RequestApproval (deny blocks the tool);
// ask_user maps to AskQuestion. This wires BOTH the approval and the question paths
// (the Codex handleInbound currently wires only approvals).
func (a *claudeAdapter) handleInbound(ctx context.Context, proc *claudeProcess, bridge TurnBridge, line claudeLine) {
	reqID, payload := claudeControlRequest(line.Raw)
	subtype := line.Subtype
	if subtype == "" {
		subtype, _ = payload["subtype"].(string)
	}

	switch subtype {
	case "ask_user":
		prompt, options, multi := claudeAskUserParams(payload)
		type res struct {
			choice []string
			err    error
		}
		ch := make(chan res, 1)
		go func() { c, e := bridge.AskQuestion(prompt, options, multi); ch <- res{c, e} }()
		select {
		case <-ctx.Done():
			// turn ended (completed/interrupt/stream death) → stop waiting; reply best-
			// effort so the engine (if alive) does not hang. The detached bridge call
			// ends when the runner cancels its own turn ctx (finishTurn).
			_ = proc.stream.replyControl(reqID, map[string]any{"error": "turn ended"})
		case r := <-ch:
			if r.err != nil {
				// expiry/interrupt while pending → error result so the model does not hang.
				_ = proc.stream.replyControl(reqID, map[string]any{"error": r.err.Error()})
				return
			}
			_ = proc.stream.replyControl(reqID, map[string]any{"answer": r.choice})
		}

	default: // can_use_tool / permission
		details := claudeApprovalDetails(payload)
		type res struct {
			decision string
			err      error
		}
		ch := make(chan res, 1)
		go func() { d, e := bridge.RequestApproval(details); ch <- res{d, e} }()
		select {
		case <-ctx.Done():
			_ = proc.stream.replyControl(reqID, map[string]any{"behavior": "deny", "message": "turn ended"})
		case r := <-ch:
			if r.err != nil {
				// expiry/interrupt → deny so the engine never hangs (04-04).
				_ = proc.stream.replyControl(reqID, map[string]any{"behavior": "deny", "message": "expired"})
				return
			}
			behavior := "allow"
			if r.decision == "deny" {
				behavior = "deny"
			}
			_ = proc.stream.replyControl(reqID, map[string]any{"behavior": behavior})
		}
	}
}
