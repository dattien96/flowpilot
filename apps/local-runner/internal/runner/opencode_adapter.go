package runner

import (
	"context"
	"encoding/json"
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

	// onVariantsCaptured (CA-689b) receives the REAL per-model effort options
	// opencode returns with every set_config_option response — the models CLI
	// cannot provide them, and per-model truth is what makes the reasoning
	// picker honest (hy3: default/none/low/high ≠ muse-spark's list).
	onVariantsCaptured func(modelID string, efforts []string, current string)

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
	permissionDenied   map[string]bool
}

// Post-result text-wait knobs. opencodeEmptyTextWait mirrors the CA-707 8s
// wait-for-first-text; opencodeDeniedEmptyTextWait shortens it for turns where
// a permission request was DENIED (CA-712): live wire captures show opencode
// 1.18.x never streams the answer after a denial, so the generic 8s is pure
// latency — the session/load replay recovery takes over after this grace.
var opencodeEmptyTextWait = 8 * time.Second

var opencodeDeniedEmptyTextWait = 1500 * time.Millisecond

// Replay recovery knobs (CA-712): the session/load RPC itself, the quiet window
// that ends collection once the replay burst stops, and a hard cap so a huge
// history cannot stall the turn.
var (
	opencodeReplayLoadTimeout = 4 * time.Second
	opencodeReplayQuietWindow = 400 * time.Millisecond
	opencodeReplayHardCap     = 8 * time.Second
)

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
		// Vision stays false at the provider level (Task-318/319): opencode
		// image support is per-MODEL (models.dev input.image), so the UI gates
		// and adapter use the per-model catalog (opencodeModelSupportsImages),
		// not this provider-wide flag — same pattern as Grok (grok_adapter.go).
		Vision: false,
	}
}

func (a *opencodeAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt
}

// opencodeModelSupportsImages reports whether the model's own capability says
// it accepts image input (models.dev capabilities.input.image, captured by the
// Task-319 verbose models parse). Conservative: unknown/absent model → false.
// The models cache is written on every successful detection and warmed async
// at boot, so it is populated by the time a user attaches an image.
func opencodeModelSupportsImages(modelID string) bool {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return false
	}
	cached, ok := readOpencodeModelsCache(true)
	if !ok {
		return false
	}
	for _, m := range cached {
		if strings.EqualFold(strings.TrimSpace(m.ID), id) {
			return m.InputImage
		}
	}
	return false
}

// opencodeTurnImageAttachments returns the turn's attachments only when the
// selected model is image-capable; otherwise nil — non-vision models silently
// ignore attachments (provider_registry contract), matching pre-Task-319
// behavior. The desktop/TUI gates prevent attaching to non-vision models; this
// is defense-in-depth so a stale UI can never blind-copy bytes to a model that
// cannot see them.
func opencodeTurnImageAttachments(req TurnRequest) []PromptAttachment {
	if len(req.Attachments) == 0 {
		return nil
	}
	if !opencodeModelSupportsImages(req.ModelName) {
		return nil
	}
	return req.Attachments
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
	if a.permissionDenied == nil {
		a.permissionDenied = map[string]bool{}
	}
	a.permissionDenied[sessionID] = false
	// Reference OpencodePermissionMode to keep YOLO SSOT in sync (future session/new permission wiring)
	_ = resolveYoloPosture(req.YoloMode).OpencodePermissionMode
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, sessionID)
		delete(a.allowReviewOutcome, sessionID)
		delete(a.yoloModes, sessionID)
		delete(a.permissionDenied, sessionID)
		a.mu.Unlock()
	}()

	prompt := a.preparePrompt(req)
	promptParams := opencodeACPPromptParamsWithAttachments(sessionID, prompt, opencodeTurnImageAttachments(req))

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
			// BUG-341: 421135/424302 blank — opencode's agent_message_chunk for
			// the final answer can arrive well after session/prompt's result
			// (tool burst + generation). The 600ms wait was too short for a
			// 2–5s generation. If we already have text, just wait for the burst
			// to finish (150ms quiet); if still empty, wait up to 8s for the
			// first chunk. A denied-permission turn gets a short grace instead
			// (CA-712: the answer never streams in that shape) and is recovered
			// from the session/load replay below.
			denied := a.permissionDeniedFor(sessionID)
			start := time.Now()
			if lastText == "" {
				if resultText, _ := outcome.result["text"].(string); strings.TrimSpace(resultText) != "" {
					lastText = strings.TrimSpace(resultText)
				} else if denied {
					lastText = a.drainOpencodeNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, opencodeDeniedEmptyTextWait)
				} else {
					lastText = a.drainOpencodeNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, opencodeEmptyTextWait)
				}
			} else {
				lastText = a.drainOpencodeNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, 150*time.Millisecond)
			}
			if waited := time.Since(start); waited > 200*time.Millisecond {
				log.Printf("[opencode] post-result wait lastText_len=%d waited=%s session=%s", len(lastText), waited.Truncate(time.Millisecond), sessionID)
			}
			lastText = a.recoverOpencodeEmptyAnswer(ctx, req, sessionID, notif, bridge, outcome.result, lastText, cwd, mcpServers, denied)
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

func (a *opencodeAdapter) drainOpencodeNotificationsBlocking(ctx context.Context, sessionID string, notif <-chan opencodeNotification, bridge TurnBridge, lastText string, timeout time.Duration) string {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	hasText := strings.TrimSpace(lastText) != ""
	for {
		select {
		case <-ctx.Done():
			return lastText
		case n, ok := <-notif:
			if !ok {
				return lastText
			}
			before := lastText
			lastText = a.applyOpencodeNotification(sessionID, n, bridge, lastText)
			if lastText != before {
				// Text grew: the answer is streaming — a short quiet window
				// (150ms) then return.
				resetOpencodeTimer(timer, 150*time.Millisecond)
				hasText = true
			} else if !hasText {
				// Still no text: the model may be generating after tools and
				// emitting only usage_update/tool frames (run-437116: 8s of
				// silence between usage updates, then the answer). Keep the
				// wait-for-text budget alive on ANY session activity so a
				// long generation is not cut off by a fixed 8s cap — but cap
				// at 15s so a genuinely textless turn still settles.
				resetOpencodeTimer(timer, 15*time.Second)
				if kind, content := opencodeNotificationKindAndContent(n); kind == "agent_message_chunk" {
					log.Printf("[opencode] DEBUG agent_message_chunk yielded no text session=%s content=%.400s", sessionID, content)
				}
			} else {
				// Text already present: stay in quiet mode even on non-text.
				resetOpencodeTimer(timer, 150*time.Millisecond)
			}
		case <-timer.C:
			return lastText
		}
	}
}

// resetOpencodeTimer safely re-arms t (which may have already fired).
func resetOpencodeTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// opencodeNotificationKindAndContent extracts the sessionUpdate kind and a
// compact content preview for diagnostics (empty-text agent_message_chunk).
func opencodeNotificationKindAndContent(n opencodeNotification) (string, string) {
	update, _ := n.Params["update"].(map[string]any)
	if update == nil {
		return "", ""
	}
	kind, _ := update["sessionUpdate"].(string)
	content := ""
	if raw, err := json.Marshal(update["content"]); err == nil {
		content = string(raw)
	}
	return kind, content
}

// permissionDeniedFor reports whether any permission request was DENIED during
// the current turn on this session (CA-712 recovery trigger).
func (a *opencodeAdapter) permissionDeniedFor(sessionID string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.permissionDenied[sessionID]
}

func (a *opencodeAdapter) markOpencodePermissionDenied(sessionID string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.permissionDenied == nil {
		a.permissionDenied = map[string]bool{}
	}
	a.permissionDenied[sessionID] = true
	a.mu.Unlock()
}

// recoverOpencodeEmptyAnswer is the CA-712 root-cause fix for BUG-341: when a
// permission request was DENIED during the turn, opencode 1.18.x completes
// session/prompt with stopReason end_turn but never emits the assistant's reply
// as agent_message_chunk — not before the result, not in any drain window
// (live-proven on 9 denied turns in cli-runner.log). The reply only exists in
// opencode's session store; a session/load on the SAME process replays history
// including this turn's prompt, so recover the answer from the replay instead
// of leaving the turn blank until the next turn's replay.
func (a *opencodeAdapter) recoverOpencodeEmptyAnswer(ctx context.Context, req TurnRequest, sessionID string, notif <-chan opencodeNotification, bridge TurnBridge, result map[string]any, lastText, cwd string, mcpServers []interface{}, denied bool) string {
	if a == nil || a.dispatcher == nil {
		return lastText
	}
	if strings.TrimSpace(lastText) != "" || !denied || result == nil {
		return lastText
	}
	stopReason, _ := result["stopReason"].(string)
	if opencodeStopReasonToEvent(stopReason) != EventTurnCompleted {
		return lastText
	}
	log.Printf("[opencode] permission denied this turn and no text streamed — attempting session/load replay recovery run=%s session=%s stopReason=%q", req.RunID, sessionID, stopReason)
	recovered := strings.TrimSpace(a.recoverOpencodeMissingAnswer(ctx, sessionID, notif, cwd, mcpServers))
	if recovered == "" {
		return lastText
	}
	bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: recovered})
	log.Printf("[opencode] replay recovery captured answer len=%d run=%s session=%s", len(recovered), req.RunID, sessionID)
	return recovered
}

// recoverOpencodeMissingAnswer issues session/load (same process — BUG-329
// keeps sessions process-local) and returns the current turn's assistant text
// from the replay. Replay frames are consumed locally: tool/thought/usage
// frames are NOT re-emitted to the bridge, so history never duplicates in the
// live transcript.
func (a *opencodeAdapter) recoverOpencodeMissingAnswer(ctx context.Context, sessionID string, notif <-chan opencodeNotification, cwd string, mcpServers []interface{}) string {
	loadCtx, cancel := context.WithTimeout(ctx, opencodeReplayLoadTimeout)
	defer cancel()
	if _, err := a.dispatcher.call(loadCtx, "session/load", opencodeACPSessionLoadParams(sessionID, cwd, mcpServers)); err != nil {
		log.Printf("[opencode] replay recovery session/load failed session=%s err=%v", sessionID, err)
		return ""
	}
	return collectOpencodeReplayAnswer(ctx, notif)
}

// collectOpencodeReplayAnswer consumes replay frames until the burst goes quiet
// (or the hard cap / ctx fires) and returns the assistant text of the CURRENT
// turn. Live replay shape (1.18.25): history streams chronologically and every
// message — including the just-finished prompt — appears as a
// user_message_chunk; the answer is the agent_message_chunk text that follows
// the LAST user frame. Resetting on every user frame keeps exactly that tail
// and drops every older answer.
func collectOpencodeReplayAnswer(ctx context.Context, notif <-chan opencodeNotification) string {
	timer := time.NewTimer(opencodeReplayQuietWindow)
	defer timer.Stop()
	hardCap := time.NewTimer(opencodeReplayHardCap)
	defer hardCap.Stop()
	var text string
	for {
		select {
		case <-ctx.Done():
			return text
		case <-hardCap.C:
			return text
		case <-timer.C:
			return text
		case n, ok := <-notif:
			if !ok {
				return text
			}
			resetOpencodeTimer(timer, opencodeReplayQuietWindow)
			update, _ := n.Params["update"].(map[string]any)
			if update == nil {
				continue
			}
			switch kind, _ := update["sessionUpdate"].(string); kind {
			case "user_message_chunk":
				// A newer prompt boundary: anything collected before it belongs
				// to an older turn (or is the pre-answer frame order) — drop it.
				text = ""
			case "agent_message_chunk":
				text += opencodeTextContent(update["content"])
			}
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
		log.Printf("[opencode] applySessionConfig skip sessionId empty model=%q", modelName)
		return
	}
	modelName = strings.TrimSpace(modelName)
	// Use a bounded per-RPC context so a missing handler (test fake) does not block the turn
	// and the second RPC still gets a full budget (review I-2).
	if modelName != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			result, err := a.dispatcher.call(cfgCtx, "session/set_config_option", opencodeACPSessionSetConfigParams(sessionID, "model", modelName))
			if err != nil {
				log.Printf("[opencode] session/set_config_option model=%q session=%q err=%v — trying fallback set_config", modelName, sessionID, err)
				_, _ = a.dispatcher.call(cfgCtx, "session/set_config", opencodeACPSessionSetConfigParamsAlt(sessionID, "model", modelName))
				return
			}
			log.Printf("[opencode] session/set_config_option ok model=%q session=%q", modelName, sessionID)
			// CA-689b: the response's configOptions echo the effort select for
			// the freshly-selected model — the only live per-model variant truth.
			if a.onVariantsCaptured != nil {
				if efforts, current := opencodeEffortOptionsFromConfig(result); len(efforts) > 0 {
					a.onVariantsCaptured(modelName, efforts, current)
				}
			}
		}()
	} else {
		log.Printf("[opencode] applySessionConfig no model change session=%q effort=%q", sessionID, effort)
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
	if strings.TrimSpace(finalText) == "" {
		// Diagnostic for the BUG-341 class (blank first turn after tools): a
		// non-empty FinalMessage is the only thing that lets TUI/Desktop paint
		// the reply. If this fires after the 8s wait + array-text mapper +
		// CA-712 replay recovery, the model produced no message part at all —
		// permissionDenied tells whether opencode's denied-turn shape applied.
		log.Printf("[opencode] WARN turn_completed with EMPTY finalMessage run=%s session=%s stopReason=%q lastText_len=%d permissionDenied=%t", req.RunID, sessionID, stopReason, len(lastText), a.permissionDeniedFor(sessionID))
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
		a.markOpencodePermissionDenied(sessionID)
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
		a.markOpencodePermissionDenied(sessionID)
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opencodeEncodePermissionDecision(options, false)}})
		return
	}
	approve := decision == "approve" || decision == "approved" || decision == "approve_for_session"
	if !approve {
		// CA-712: a denied permission is what trips opencode's missing-answer
		// bug — remember it so the post-result recovery can fire.
		a.markOpencodePermissionDenied(sessionID)
	}
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
