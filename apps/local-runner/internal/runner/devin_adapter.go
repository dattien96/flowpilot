package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Task-401 (CP-70): the Devin provider adapter. Implements the
// provider-neutral ProviderRuntimeAdapter on top of the Task-400 dispatcher.
// Modeled on opencodeAdapter.
//
// Devin specifics honored here (all live-verified on 3000.10.31):
//   - sessionIds are slug-style names ("working-pentagon") persisted in the
//     account's SQLite store — resumable across processes via session/load.
//   - Model AND mode are session config options: after session/new|load the
//     adapter applies `session/set_config_option` {configId:"model"|"mode"}.
//   - Devin accepts stdio MCP only — the FlowPilot tool bridge reaches the
//     agent through the `flowpilot devin-mcp-stdio` shim (devin_mcp_stdio.go),
//     never through the HTTP loopback the other providers use.
//   - Modes map FlowPilot semantics onto Devin's real set: yolo→bypass,
//     scan→ask, plan→plan, gated flow nodes→ask, code/gated→accept-edits.

const devinAskUserReinforcement = "\n\n---\nWhen you need to ask the user a question, call the FlowPilot MCP tool `ask_user` on server `flowpilot` with arguments `prompt` (string), `options` (array of strings), and optional `multiSelect` (boolean). Do NOT use native `question` tools."

const devinSpawnAgentReinforcement = "\n\n---\nWhen you need to spawn a sub-agent for parallel or delegated work, call the FlowPilot MCP tool `spawn_agent` on server `flowpilot` with arguments `agent` (string), `prompt` (string), optional `provider` (string), and optional `wait` (boolean — true to block until the child completes). Do NOT use native `spawn_subagent` tools."

const devinToolReinforcements = devinAskUserReinforcement + devinSpawnAgentReinforcement

type devinAdapter struct {
	dispatcher *devinDispatcher
	cwd        string
	initResult map[string]any

	promptPrep func(TurnRequest) string

	sessionStore ProviderSessionStore

	mcpServer       *claudeMCPServer
	mcpBaseURL      func() string
	extraMCPServers func(yolo bool) map[string]claudeMcpServer
	// mcpShimCommand resolves the FlowPilot stdio shim command for the ACP
	// mcpServers entry (test seam — production resolves os.Executable()).
	mcpShimCommand func() (string, []string)

	mu                 sync.Mutex
	bridges            map[string]TurnBridge
	lastSessionID      string
	runSessions        *devinRunSessionIndex
	allowReviewOutcome map[string]bool
	yoloModes          map[string]bool
	permissionDenied   map[string]bool
	// toolCalls is the per-session toolCallId correlation index (BUG-374/375/
	// 434/436) — populated by session/update tool_call frames, read by
	// tool_call_update enrichment and session/request_permission handling.
	toolCalls map[string]*devinToolCallIndex
	// appliedModel is the session's ACTUAL model (configOptions currentValue)
	// — distinct from the requested id so records never claim a model that
	// was rejected by set_config_option (BUG-379/433).
	appliedModel map[string]string
	// modelRejected marks sessions whose model set_config_option was refused.
	// On a resumed session whose load result carried no currentValue the
	// applied model is then unobservable — records must persist a typed
	// unknown marker, not the rejected requested id (BUG-451).
	modelRejected map[string]bool
	// modelCatalog caches the configOptions "model" choices observed on this
	// process's sessions — the per-model supportsImages flags live there.
	modelCatalog []DevinConfigChoice
	// thoughtLevels caches the configOptions "thought_level" values (Task-438).
	// New-schema CLIs (3000.11.x) expose it as a session-level reasoning knob;
	// empty means the session predates it and reasoning falls back to the
	// effort-suffix model remap.
	thoughtLevels []string
}

// Post-result text-wait knobs, mirroring the CA-707/CA-712 opencode rules.
var devinEmptyTextWait = 8 * time.Second

var devinDeniedEmptyTextWait = 1500 * time.Millisecond

var (
	devinReplayLoadTimeout = 4 * time.Second
	devinReplayQuietWindow = 400 * time.Millisecond
	devinReplayHardCap     = 8 * time.Second
)

func newDevinAdapter(dispatcher *devinDispatcher, cwd string) *devinAdapter {
	a := &devinAdapter{
		dispatcher:         dispatcher,
		cwd:                cwd,
		bridges:            map[string]TurnBridge{},
		allowReviewOutcome: map[string]bool{},
		yoloModes:          map[string]bool{},
		toolCalls:          map[string]*devinToolCallIndex{},
	}
	if dispatcher != nil {
		dispatcher.setInbound(a.handleInbound)
	}
	return a
}

func (a *devinAdapter) Key() ProviderKey { return ProviderKeyDevin }

// devinRunSessionIndex mirrors opencodeRunSessionIndex for Devin ACP resume.
type devinRunSessionIndex struct {
	mu        sync.Mutex
	byRun     map[string]string
	bySession map[string]string
}

func (x *devinRunSessionIndex) remember(runID, sessionID string) {
	if x == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	sessionID = strings.TrimSpace(sessionID)
	if runID == "" || !isDevinRealSessionID(sessionID) {
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

func (x *devinRunSessionIndex) lookup(runID string) string {
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

func (x *devinRunSessionIndex) ownerOf(sessionID string) string {
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

func (a *devinAdapter) rememberRunSession(runID, sessionID string) {
	if a == nil {
		return
	}
	if a.runSessions == nil {
		a.runSessions = &devinRunSessionIndex{byRun: make(map[string]string), bySession: make(map[string]string)}
	}
	a.runSessions.remember(runID, sessionID)
}

func (a *devinAdapter) lookupRunSession(runID string) string {
	if a == nil {
		return ""
	}
	return a.runSessions.lookup(runID)
}

// isDevinRealSessionID reports whether an id is a real Devin session id.
// Devin ids are lowercase slug names ("working-pentagon") — NOT ses_*, NOT
// UUIDs. FlowPilot synthetic ids (thread-*) and paths are excluded; every
// other non-empty slug-shaped token counts as real.
func isDevinRealSessionID(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" || strings.HasPrefix(id, "thread-") {
		return false
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func devinEnsureResumeID(req TurnRequest, lookup func(string) string) string {
	id := strings.TrimSpace(req.ProviderSessionID)
	if isDevinRealSessionID(id) {
		return id
	}
	if lookup != nil {
		if cached := strings.TrimSpace(lookup(req.RunID)); isDevinRealSessionID(cached) {
			return cached
		}
	}
	return id
}

// Capabilities advertises what the adapter supports (honest MVP set).
// Vision stays false at the provider level — Devin image support is
// per-MODEL (_meta["cognition.ai/supportsImages"] on the catalog option), so
// the UI gates on the per-model flag (same pattern as Grok/OpenCode).
func (a *devinAdapter) Capabilities() ProviderCapabilities {
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

func (a *devinAdapter) preparePrompt(req TurnRequest) string {
	if a.promptPrep != nil {
		return a.promptPrep(req)
	}
	return req.Prompt
}

// devinModelSupportsImages reports whether the selected Devin model advertises
// image input — from the process-captured catalog first, then the persisted
// detection cache (Task-402). Conservative: unknown model → false.
func (a *devinAdapter) devinModelSupportsImages(modelID string) bool {
	id := devinModelIDForACP(modelID)
	if id == "" {
		return false
	}
	if a != nil {
		a.mu.Lock()
		catalog := a.modelCatalog
		a.mu.Unlock()
		for _, c := range catalog {
			if strings.EqualFold(strings.TrimSpace(c.Value), id) {
				return devinChoiceSupportsImages(c)
			}
		}
	}
	for _, m := range readDevinModelsCache() {
		if strings.EqualFold(strings.TrimSpace(m.ID), id) {
			return m.InputImage
		}
	}
	return false
}

// devinTurnImageAttachments returns the turn's attachments only when the
// selected model is image-capable; otherwise nil (same defense-in-depth as
// opencodeTurnImageAttachments).
func (a *devinAdapter) devinTurnImageAttachments(req TurnRequest) []PromptAttachment {
	if len(req.Attachments) == 0 {
		return nil
	}
	if !a.devinModelSupportsImages(req.ModelName) {
		return nil
	}
	return req.Attachments
}

func (a *devinAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	a.mu.Lock()
	a.lastSessionID = ""
	a.mu.Unlock()

	// MCP wiring: Devin ACP accepts stdio servers only (mcpCapabilities
	// {http:false,sse:false} live-verified). The FlowPilot tool bridge rides
	// on the stdio shim — `flowpilot devin-mcp-stdio` proxies JSON-RPC to the
	// runner's HTTP MCP endpoint carrying this turn's token.
	var mcpToken string
	var mcpServers []interface{}
	if a.mcpServer != nil {
		mcpToken = a.mcpServer.register(bridge, req.OfferReviewOutcomeTool)
		if req.OfferVibeRequirementTool {
			a.mcpServer.setAllowVibeRequirement(mcpToken, true)
		}
		defer a.mcpServer.unregister(mcpToken)
		if entry := a.devinFlowPilotMCPEntry(mcpToken); entry != nil {
			mcpServers = append(mcpServers, entry)
		}
		if a.extraMCPServers != nil {
			mcpServers = append(mcpServers, devinACPExtraMCPServers(a.extraMCPServers(req.YoloMode))...)
		}
	}

	// session/load ignores the ACP mcpServers param (live-verified), so the
	// per-turn entries are also written to <cwd>/.devin/mcp_config.local.json —
	// the config scope a loaded session actually enumerates. Without it the
	// flowpilot shim (ask_user/spawn_agent/approve) is missing on resume.
	if err := ensureDevinLocalMcpServers(cwd, mcpServers); err != nil {
		log.Printf("[devin] project-local mcp_config write failed (MCP tools may be absent on resume) cwd=%q err=%v", cwd, err)
	}

	sessionID, err := a.ensureSession(ctx, req, cwd, mcpServers)
	if err != nil {
		return err
	}
	// Per-turn model + mode via session/set_config_option (live configOptions
	// carry both selects; session/new has no model/mode params).
	a.applyDevinSessionConfig(ctx, sessionID, req, bridge)
	// BUG-433: persist the APPLIED model — a rejected set_config_option must
	// not leave the record claiming the requested id ran.
	a.recordAppliedSessionModel(ctx, req, sessionID)

	// Devin must NOT wait for the Claude-style tools/list readiness signal:
	// Devin's ACP client starts stdio MCP servers lazily (mcp/serversChanged
	// fires only after session/prompt begins), so waitReady would burn the
	// full 30s timeout on every turn without ever proving the shim is up.
	// The token stays registered for the whole prompt, so MCP calls that
	// arrive mid-turn still resolve to this bridge.
	if mcpToken != "" {
		log.Printf("[devin-mcp] lazy readiness — prompt dispatched without tools/list gate session=%s", sessionID)
	}

	notif, err := a.dispatcher.registerSession(sessionID)
	if err != nil {
		return err
	}
	defer a.dispatcher.unregisterSession(sessionID)

	a.mu.Lock()
	a.bridges[sessionID] = bridge
	a.allowReviewOutcome[sessionID] = req.OfferReviewOutcomeTool
	a.yoloModes[sessionID] = req.YoloMode && !req.ForceShellBridge && !IsReadOnlyChatPosture(req.ChatPosture) && !IsGatedFlowNodePosture(req.FlowNodePosture)
	if a.permissionDenied == nil {
		a.permissionDenied = map[string]bool{}
	}
	a.permissionDenied[sessionID] = false
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.bridges, sessionID)
		delete(a.allowReviewOutcome, sessionID)
		delete(a.yoloModes, sessionID)
		delete(a.permissionDenied, sessionID)
		delete(a.toolCalls, sessionID)
		delete(a.appliedModel, sessionID)
		a.mu.Unlock()
	}()

	prompt := a.preparePrompt(req)
	promptParams := devinACPPromptParamsWithAttachments(sessionID, prompt, a.devinTurnImageAttachments(req))

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
				if isProviderUsageLimitError(outcome.err) {
					msg := strings.TrimSpace(outcome.err.Error())
					if runes := []rune(msg); len(runes) > 300 {
						msg = string(runes[:300]) + "…"
					}
					bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "Devin usage limit reached: " + msg, Recoverable: false})
					return nil
				}
				return outcome.err
			}
			lastText = a.drainDevinNotifications(sessionID, notif, bridge, lastText)
			denied := a.permissionDeniedFor(sessionID)
			start := time.Now()
			if lastText == "" {
				if resultText, _ := outcome.result["text"].(string); strings.TrimSpace(resultText) != "" {
					lastText = strings.TrimSpace(resultText)
				} else if denied {
					lastText = a.drainDevinNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, devinDeniedEmptyTextWait)
				} else {
					lastText = a.drainDevinNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, devinEmptyTextWait)
				}
			} else {
				lastText = a.drainDevinNotificationsBlocking(ctx, sessionID, notif, bridge, lastText, 150*time.Millisecond)
			}
			if waited := time.Since(start); waited > 200*time.Millisecond {
				log.Printf("[devin] post-result wait lastText_len=%d waited=%s session=%s", len(lastText), waited.Truncate(time.Millisecond), sessionID)
			}
			lastText = a.recoverDevinEmptyAnswer(ctx, req, sessionID, notif, bridge, outcome.result, lastText, cwd, mcpServers, denied)
			return a.emitTerminal(ctx, req, bridge, sessionID, outcome.result, lastText)
		case n, ok := <-notif:
			if !ok {
				return fmt.Errorf("devin acp stream closed mid-turn")
			}
			lastText = a.applyDevinNotification(sessionID, n, bridge, lastText)
		}
	}
}

// devinFlowPilotMCPEntry builds the stdio mcpServers entry pointing at the
// FlowPilot shim. The shim subcommand proxies stdio JSON-RPC to the runner's
// HTTP MCP endpoint; url+token travel as args (never env — args show in the
// ACP session config which is the documented mechanism anyway).
func (a *devinAdapter) devinFlowPilotMCPEntry(token string) map[string]interface{} {
	baseURL := ""
	if a.mcpBaseURL != nil {
		baseURL = a.mcpBaseURL()
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	command, args := devinMCPShimCommand()
	if a.mcpShimCommand != nil {
		command, args = a.mcpShimCommand()
	}
	if strings.TrimSpace(command) == "" {
		return nil
	}
	url := strings.TrimRight(baseURL, "/") + ClaudeMCPPath + "?token=" + token
	args = append(args, "--url", url)
	return devinACPStdioMCPServerEntry(claudeMCPServerName, command, args, nil)
}

// toolCallIndexFor returns the session's toolCallId correlation index,
// creating it lazily. Called from both the turn goroutine and the
// dispatcher's inbound goroutine.
func (a *devinAdapter) toolCallIndexFor(sessionID string) *devinToolCallIndex {
	if a == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolCalls == nil {
		a.toolCalls = map[string]*devinToolCallIndex{}
	}
	idx := a.toolCalls[sessionID]
	if idx == nil {
		idx = &devinToolCallIndex{}
		a.toolCalls[sessionID] = idx
	}
	return idx
}

func (a *devinAdapter) applyDevinNotification(sessionID string, n devinNotification, bridge TurnBridge, lastText string) string {
	n = devinCorrelateToolNotification(a.toolCallIndexFor(sessionID), n)
	events, mapped := mapDevinNotification(n)
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

func (a *devinAdapter) drainDevinNotifications(sessionID string, notif <-chan devinNotification, bridge TurnBridge, lastText string) string {
	for {
		select {
		case n, ok := <-notif:
			if !ok {
				return lastText
			}
			lastText = a.applyDevinNotification(sessionID, n, bridge, lastText)
		default:
			return lastText
		}
	}
}

func (a *devinAdapter) drainDevinNotificationsBlocking(ctx context.Context, sessionID string, notif <-chan devinNotification, bridge TurnBridge, lastText string, timeout time.Duration) string {
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
			lastText = a.applyDevinNotification(sessionID, n, bridge, lastText)
			if lastText != before {
				resetDevinTimer(timer, 150*time.Millisecond)
				hasText = true
			} else if !hasText {
				// Any session activity keeps the wait-for-text budget alive
				// (long generations emit usage_update between chunks); cap at
				// 15s so a genuinely textless turn still settles.
				resetDevinTimer(timer, 15*time.Second)
			} else {
				resetDevinTimer(timer, 150*time.Millisecond)
			}
		case <-timer.C:
			return lastText
		}
	}
}

func resetDevinTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

func (a *devinAdapter) permissionDeniedFor(sessionID string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.permissionDenied[sessionID]
}

func (a *devinAdapter) markDevinPermissionDenied(sessionID string) {
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

// recoverDevinEmptyAnswer is the CA-712/CA-713 analog for Devin: when a
// permission request was DENIED and nothing streamed, try a session/load
// replay first (Devin's loadSession replays history — live-verified), then
// emit an honest no-reply notice instead of a blank turn.
func (a *devinAdapter) recoverDevinEmptyAnswer(ctx context.Context, req TurnRequest, sessionID string, notif <-chan devinNotification, bridge TurnBridge, result map[string]any, lastText, cwd string, mcpServers []interface{}, denied bool) string {
	if a == nil || a.dispatcher == nil {
		return lastText
	}
	if strings.TrimSpace(lastText) != "" || !denied || result == nil {
		return lastText
	}
	stopReason, _ := result["stopReason"].(string)
	if devinStopReasonToEvent(stopReason) != EventTurnCompleted {
		return lastText
	}
	log.Printf("[devin] permission denied this turn and no text streamed — attempting session/load replay recovery run=%s session=%s stopReason=%q", req.RunID, sessionID, stopReason)
	recovered := strings.TrimSpace(a.recoverDevinMissingAnswer(ctx, sessionID, notif, cwd, mcpServers))
	if recovered != "" {
		bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: recovered})
		log.Printf("[devin] replay recovery captured answer len=%d run=%s session=%s", len(recovered), req.RunID, sessionID)
		return recovered
	}
	notice := "[no reply text] devin aborted the turn right after a tool permission was denied — the earlier steps still ran, but the model was never given another round, so no answer exists. Switch posture (plan/code) or approve the tool to continue."
	bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: notice})
	log.Printf("[devin] emitted no-reply notice after denied permission run=%s session=%s", req.RunID, sessionID)
	return notice
}

func (a *devinAdapter) recoverDevinMissingAnswer(ctx context.Context, sessionID string, notif <-chan devinNotification, cwd string, mcpServers []interface{}) string {
	loadCtx, cancel := context.WithTimeout(ctx, devinReplayLoadTimeout)
	defer cancel()
	if _, err := a.dispatcher.call(loadCtx, "session/load", devinACPSessionLoadParams(sessionID, cwd, mcpServers)); err != nil {
		log.Printf("[devin] replay recovery session/load failed session=%s err=%v", sessionID, err)
		return ""
	}
	return collectDevinReplayAnswer(ctx, notif)
}

// collectDevinReplayAnswer consumes replay frames until the burst goes quiet
// (or the hard cap / ctx fires) and returns the assistant text of the CURRENT
// turn — the agent_message_chunk tail after the last user_message_chunk.
func collectDevinReplayAnswer(ctx context.Context, notif <-chan devinNotification) string {
	timer := time.NewTimer(devinReplayQuietWindow)
	defer timer.Stop()
	hardCap := time.NewTimer(devinReplayHardCap)
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
			resetDevinTimer(timer, devinReplayQuietWindow)
			update, _ := n.Params["update"].(map[string]any)
			if update == nil {
				continue
			}
			switch kind, _ := update["sessionUpdate"].(string); kind {
			case "user_message_chunk":
				text = ""
			case "agent_message_chunk":
				text += devinTextContent(update["content"])
			}
		}
	}
}

func (a *devinAdapter) ensureSession(ctx context.Context, req TurnRequest, cwd string, mcpServers []interface{}) (string, error) {
	resumeID := devinEnsureResumeID(req, a.lookupRunSession)
	if isDevinRealSessionID(resumeID) && a.runSessions != nil {
		if owner := a.runSessions.ownerOf(resumeID); owner != "" && owner != strings.TrimSpace(req.RunID) {
			log.Printf("devin ensureSession: refusing session/load of %s owned by run %s (this run %s); forcing session/new", resumeID, owner, req.RunID)
			resumeID = ""
		}
	}
	method := "session/new"
	var result map[string]any
	var err error
	if isDevinRealSessionID(resumeID) {
		method = "session/load"
		result, err = a.dispatcher.call(ctx, method, devinACPSessionLoadParams(resumeID, cwd, mcpServers))
	} else {
		result, err = a.dispatcher.call(ctx, method, devinACPSessionNewParams(cwd, mcpServers))
	}
	if err != nil {
		return "", err
	}
	sessionID := devinACPResponseSessionIDFromResult(result)
	if sessionID == "" && method == "session/load" && isDevinRealSessionID(resumeID) {
		// session/load answers RPC-OK with a config-only result and no
		// sessionId on some shapes — adopt the requested id (BUG-329 rule).
		sessionID = resumeID
	}
	if sessionID == "" {
		return "", fmt.Errorf("devin %s returned no sessionId", method)
	}
	a.captureModelCatalog(result)
	a.mu.Lock()
	a.lastSessionID = sessionID
	a.mu.Unlock()
	if cur := devinConfigOptionCurrentValue(result, "model"); cur != "" {
		a.setAppliedModel(sessionID, cur)
	}
	a.rememberRunSession(req.RunID, sessionID)
	a.recordSession(ctx, req, sessionID)
	return sessionID, nil
}

// captureModelCatalog stores the session result's "model" config option
// choices — the live per-model catalog (with supportsImages flags) used by
// the vision gate and the Task-402 detection path — plus the "thought_level"
// option's values (Task-438: the new-schema session-level reasoning knob; nil
// when the CLI predates it).
func (a *devinAdapter) captureModelCatalog(result map[string]any) {
	if a == nil || result == nil {
		return
	}
	opt := devinConfigOptionFromResult(result, "model")
	choices := devinConfigOptionChoices(opt)
	thoughts := devinConfigOptionChoices(devinConfigOptionFromResult(result, "thought_level"))
	levels := make([]string, 0, len(thoughts))
	for _, c := range thoughts {
		if v := strings.TrimSpace(c.Value); v != "" {
			levels = append(levels, v)
		}
	}
	a.mu.Lock()
	a.thoughtLevels = levels
	a.mu.Unlock()
	if len(choices) == 0 {
		return
	}
	a.mu.Lock()
	a.modelCatalog = choices
	a.mu.Unlock()
	recordDevinModelCatalogWithEfforts(choices, levels)
}

func (a *devinAdapter) recordSession(ctx context.Context, req TurnRequest, sessionID string) {
	if a.sessionStore == nil {
		return
	}
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyDevin),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  cwd,
		ModelName:         req.ModelName,
		Status:            "active",
	})
}

// applyDevinSessionConfig applies per-turn model + mode (+ thought_level on
// new-schema CLIs) via session/set_config_option. Devin exposes all three as
// configOptions selects — model takes a bare catalog id (the FlowPilot
// "devin/" prefix is stripped), mode takes accept-edits/smart/ask/plan/bypass
// resolved from the turn's YOLO + posture (resolveDevinSessionMode), and
// thought_level takes the session's advertised reasoning values (Task-438:
// medium/high/max — the knob that actually produces "SWE-2 Max" etc.).
// Rejections/coercions surface to the user via bridge (BUG-379/433).
func (a *devinAdapter) applyDevinSessionConfig(ctx context.Context, sessionID string, req TurnRequest, bridge TurnBridge) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || a.dispatcher == nil {
		return
	}
	a.mu.Lock()
	thoughtLevels := append([]string(nil), a.thoughtLevels...)
	catalog := append([]DevinConfigChoice(nil), a.modelCatalog...)
	a.mu.Unlock()
	var modelName string
	if len(thoughtLevels) > 0 {
		// New schema: effort lives on thought_level, so the model id only
		// needs to be catalog-valid — resolve stale effort-suffixed ids to
		// their family sibling instead of remapping the suffix (those ids no
		// longer exist).
		modelName = devinCatalogModelFor(devinModelIDForACP(req.ModelName), catalog)
	} else {
		modelName = devinModelForTurn(req)
	}
	if modelName != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			res, err := a.dispatcher.call(cfgCtx, "session/set_config_option", devinACPSessionSetConfigParams(sessionID, "model", modelName))
			if err != nil {
				// BUG-379/433: an Invalid-params rejection was log-only — the
				// turn ran on the session's previous model with no user-facing
				// signal while records kept claiming the requested model.
				// BUG-451: mark the rejection so recordAppliedSessionModel can
				// degrade to a typed unknown when no applied model was observed.
				a.markModelRejected(sessionID)
				applied := a.appliedModelFor(sessionID)
				log.Printf("[devin] session/set_config_option model=%q session=%q err=%v applied=%q", modelName, sessionID, err, applied)
				if bridge != nil {
					note := fmt.Sprintf("[model] requested %q was rejected by Devin (%v); this turn runs on the session's configured model", modelName, err)
					if applied != "" {
						note += fmt.Sprintf(" %q", applied)
					}
					bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: note + "."})
				}
				return
			}
			if cur := devinConfigOptionCurrentValue(res, "model"); cur != "" {
				a.setAppliedModel(sessionID, cur)
				if !strings.EqualFold(cur, modelName) && bridge != nil {
					// Accepted but coerced to a different value — surface it
					// rather than recording a model that did not run.
					bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: fmt.Sprintf("[model] requested %q was applied as %q by Devin.", modelName, cur)})
				}
			} else {
				a.setAppliedModel(sessionID, modelName)
			}
			log.Printf("[devin] session/set_config_option ok model=%q session=%q", modelName, sessionID)
		}()
	}
	if mode := resolveDevinSessionMode(req.YoloMode, req.ForceShellBridge, req.ChatPosture, req.FlowNodePosture); mode != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			res, err := a.dispatcher.call(cfgCtx, "session/set_config_option", devinACPSessionSetConfigParams(sessionID, "mode", mode))
			if err != nil {
				log.Printf("[devin] session/set_config_option mode=%q session=%q err=%v", mode, sessionID, err)
				if bridge != nil {
					bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: fmt.Sprintf("[mode] requested session mode %q was rejected by Devin (%v); the session keeps its previous mode.", mode, err)})
				}
				return
			}
			if cur := devinConfigOptionCurrentValue(res, "mode"); cur != "" && !strings.EqualFold(cur, mode) && bridge != nil {
				bridge.Emit(ProviderEvent{Type: EventMessageDelta, Text: fmt.Sprintf("[mode] requested session mode %q was applied as %q by Devin.", mode, cur)})
			}
		}()
	}
	if level := devinThoughtLevelForEffort(req.ReasoningEffort, thoughtLevels); level != "" {
		func() {
			cfgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if _, err := a.dispatcher.call(cfgCtx, "session/set_config_option", devinACPSessionSetConfigParams(sessionID, "thought_level", level)); err != nil {
				log.Printf("[devin] session/set_config_option thought_level=%q session=%q err=%v", level, sessionID, err)
			}
		}()
	}
}

// devinConfigOptionCurrentValue reads currentValue for one config option out
// of a session/new|load|set_config_option result's configOptions array.
func devinConfigOptionCurrentValue(result map[string]any, id string) string {
	opt := devinConfigOptionFromResult(result, id)
	if opt == nil {
		return ""
	}
	cur, _ := opt["currentValue"].(string)
	return strings.TrimSpace(cur)
}

func (a *devinAdapter) setAppliedModel(sessionID, model string) {
	if a == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(model) == "" {
		return
	}
	a.mu.Lock()
	if a.appliedModel == nil {
		a.appliedModel = map[string]string{}
	}
	a.appliedModel[sessionID] = strings.TrimSpace(model)
	a.mu.Unlock()
}

func (a *devinAdapter) appliedModelFor(sessionID string) string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.appliedModel[strings.TrimSpace(sessionID)]
}

func (a *devinAdapter) markModelRejected(sessionID string) {
	if a == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	a.mu.Lock()
	if a.modelRejected == nil {
		a.modelRejected = map[string]bool{}
	}
	a.modelRejected[strings.TrimSpace(sessionID)] = true
	a.mu.Unlock()
}

func (a *devinAdapter) modelRejectedFor(sessionID string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.modelRejected[strings.TrimSpace(sessionID)]
}

// devinUnknownModelMarker is the typed degradation persisted when the session's
// applied model is genuinely unobservable (config-only session/load with no
// currentValue, followed by a rejected model set — BUG-451). It cannot collide
// with a real catalog id (ids are [a-z0-9./-]).
const devinUnknownModelMarker = "devin/(unknown)"

// recordAppliedSessionModel re-upserts the provider session record with the
// model Devin actually applied (configOptions currentValue), so run metadata
// never claims a rejected id (BUG-433). Bare catalog values get the "devin/"
// prefix to match the requested-model record format.
func (a *devinAdapter) recordAppliedSessionModel(ctx context.Context, req TurnRequest, sessionID string) {
	if a == nil || a.sessionStore == nil {
		return
	}
	applied := a.appliedModelFor(sessionID)
	if applied == "" {
		// BUG-451: a rejected model set on a session whose applied model was
		// never observed must not leave recordSession's optimistic requested id
		// on the durable record — re-upsert the typed unknown marker instead.
		if !a.modelRejectedFor(sessionID) {
			return
		}
		applied = devinUnknownModelMarker
	}
	if !strings.HasPrefix(applied, "devin/") {
		applied = "devin/" + applied
	}
	if !strings.HasPrefix(applied, "devin/") {
		applied = "devin/" + applied
	}
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyDevin),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  cwd,
		ModelName:         applied,
		Status:            "active",
	})
}

// devinModelForTurn resolves the Devin catalog model id for a turn: strips
// the "devin/" FlowPilot prefix, and when a reasoning effort is requested
// swaps the model's effort suffix (devinModelForEffort).
func devinModelForTurn(req TurnRequest) string {
	model := devinModelIDForACP(req.ModelName)
	if model == "" {
		return ""
	}
	if effort := strings.TrimSpace(req.ReasoningEffort); effort != "" {
		if mapped := devinModelForEffort(model, effort); mapped != "" {
			// Only remap to ids Devin actually lists — suffix arithmetic can
			// produce ids absent from the catalog (bare families like
			// "adaptive" have no effort variants at all). Under tests the
			// catalog is empty and the guard is skipped.
			if catalog := readDevinModelsCacheStale(true); len(catalog) == 0 {
				return mapped
			} else {
				for _, entry := range catalog {
					if strings.EqualFold(strings.TrimPrefix(entry.ID, "devin/"), mapped) {
						return mapped
					}
				}
			}
		}
	}
	return model
}

// devinModelIDForACP strips the FlowPilot "devin/" provider prefix. Bare ids
// (tests, pack-authored models) pass through unchanged.
func devinModelIDForACP(model string) string {
	m := strings.TrimSpace(model)
	m = strings.TrimPrefix(m, "devin/")
	return strings.TrimSpace(m)
}

// resolveDevinSessionMode maps the turn's YOLO + posture onto Devin's real
// session modes (accept-edits/smart/ask/plan/bypass — live-verified
// availableModes). The mapping keeps FlowPilot's approval contract honest:
//
//   - scan  → "ask"   (Devin itself blocks code changes — read-only is
//     enforced provider-side AND the bridge never sees writes)
//   - plan  → "plan"  (plan-only mode)
//   - gated flow node (read_only/verdict_only) → "ask"
//   - YOLO on, ungated → "bypass" (auto-approve all tool calls)
//   - YOLO on + forceShellBridge → "accept-edits" — workspace edits
//     auto-approved, exec still raises session/request_permission so the
//     commit denylist / approval card / read-only posture can all fire.
//
// resolveDevinSessionMode maps FlowPilot posture to Devin's session "mode"
// config option (live-verified values: accept-edits/smart/ask/plan/bypass —
// 3000.10.31 configOptions; "auto" is a CLI --permission-mode value, NOT a
// session mode, and is silently coerced to accept-edits which auto-approves
// everything — live probe 2026-09-21). Mirrors resolveYoloPostureForFlowNode
// semantics:
//
//	read-only chat posture (plan/scan)  → "plan"/"ask" (writes never requested)
//	gated flow-node posture             → "ask"  (bridge decides everything)
//	yolo && !forceShellBridge           → "bypass" (auto-approve all tools)
//	yolo && forceShellBridge (V9-21)    → "accept-edits" — workspace edits
//	                                      auto-approved, exec still raises
//	                                      session/request_permission so the
//	                                      git-commit denylist can fire
//	default (non-yolo chat)             → "smart" — safe ops auto-run;
//	                                      dangerous ops raise
//	                                      session/request_permission →
//	                                      approval bridge (DV-04 ceiling:
//	                                      routine writes are auto-approved
//	                                      under smart; only provider-judged
//	                                      dangerous actions prompt)
//
// Returning "" leaves the session's saved mode untouched.
func resolveDevinSessionMode(yolo, forceShellBridge bool, chatPosture, flowNodePosture string) string {
	if IsReadOnlyChatPosture(chatPosture) {
		if strings.TrimSpace(chatPosture) == ChatPosturePlan {
			return "plan"
		}
		return "ask"
	}
	if IsGatedFlowNodePosture(flowNodePosture) {
		return "ask"
	}
	if yolo {
		if forceShellBridge {
			return "accept-edits"
		}
		return "bypass"
	}
	return "smart"
}

func (a *devinAdapter) emitTerminal(ctx context.Context, req TurnRequest, bridge TurnBridge, sessionID string, result map[string]any, lastText string) error {
	if result == nil {
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "devin session/prompt returned no result", Recoverable: true})
		return nil
	}
	if adopted := devinACPResponseSessionIDFromResult(result); adopted != "" && adopted != sessionID {
		a.mu.Lock()
		a.lastSessionID = adopted
		a.mu.Unlock()
		a.rememberRunSession(req.RunID, adopted)
		a.recordSession(ctx, req, adopted)
	}
	if usage, ok := result["usage"].(map[string]any); ok {
		if token := devinPromptResultTokenUsage(usage, nil); token != nil {
			bridge.Emit(ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: token})
		}
	}
	stopReason, _ := result["stopReason"].(string)
	finalText := lastText
	if finalText == "" {
		if text, ok := result["text"].(string); ok {
			finalText = text
		}
	}
	evType := devinStopReasonToEvent(stopReason)
	if evType == EventTurnFailed {
		errMsg := fmt.Sprintf("devin turn ended: %s", stopReason)
		if devinIsQuotaStopReason(stopReason) {
			errMsg = fmt.Sprintf("Devin usage limit reached (stopReason=%s)", strings.TrimSpace(stopReason))
		}
		bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: errMsg, Recoverable: false})
		return nil
	}
	if strings.TrimSpace(finalText) == "" {
		log.Printf("[devin] WARN turn_completed with EMPTY finalMessage run=%s session=%s stopReason=%q lastText_len=%d permissionDenied=%t", req.RunID, sessionID, stopReason, len(lastText), a.permissionDeniedFor(sessionID))
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalText})
	return nil
}

func (a *devinAdapter) LastDevinSessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSessionID
}

// handleInbound routes server->client requests. session/request_permission
// goes through the approval bridge (or YOLO auto-approve); every other
// method — including the _cognition.ai/* surface and fs/terminal capability
// calls FlowPilot never advertised — gets a JSON-RPC error reply so the agent
// is never left hanging on an unanswered request.
func (a *devinAdapter) handleInbound(req devinInboundRequest) {
	if !devinACPIsPermissionRequest(req.Method) {
		_ = a.dispatcher.replyError(req.ID, "unsupported devin inbound request: "+req.Method)
		return
	}
	sessionID := devinSessionIDFromParams(req.Params)
	a.mu.Lock()
	bridge := a.bridges[sessionID]
	yolo := a.yoloModes[sessionID]
	a.mu.Unlock()

	options, _ := req.Params["options"].([]any)
	if bridge == nil {
		a.markDevinPermissionDenied(sessionID)
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": devinEncodePermissionDecision(options, false)}})
		return
	}

	details := devinApprovalDetailsFromRequest(req.Params, a.toolCallIndexFor(sessionID))

	if yolo {
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": devinEncodePermissionDecision(options, true)}})
		return
	}

	decision, err := bridge.RequestApproval(details)
	if err != nil {
		a.markDevinPermissionDenied(sessionID)
		_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": devinEncodePermissionDecision(options, false)}})
		return
	}
	approve := decision == "approve" || decision == "approved" || decision == "approve_for_session"
	if !approve {
		a.markDevinPermissionDenied(sessionID)
	}
	_ = a.dispatcher.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": devinEncodePermissionDecision(options, approve)}})
}

// devinEncodePermissionDecision picks an optionId matching approve/deny from
// the request's options array (kind-based first, positional fallback).
func devinEncodePermissionDecision(options []any, approve bool) string {
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

// devinApprovalDetailsFromRequest builds ApprovalDetails for a Devin
// session/request_permission. Live wire (lt-evidence cp46/cp70): toolCall
// carries ONLY {toolCallId, _meta.cognition.ai/editableCommand} — no title,
// no kind, no rawInput. Two recovery paths cover the identity gap
// (BUG-374/434):
//
//   - idx correlation: the earlier session/update `tool_call` frame for the
//     same toolCallId carries title/kind/rawInput/_meta.cognition.ai/toolName.
//   - options fallback: Devin's option labels deterministically embed the MCP
//     tool name ("allow calling submit_review_outcome on the flowpilot MCP
//     server") — used when no correlated frame exists (e.g. request arrived
//     before the notification).
func devinApprovalDetailsFromRequest(params map[string]any, idx *devinToolCallIndex) ApprovalDetails {
	toolCall, _ := params["toolCall"].(map[string]any)
	toolCallID, _ := toolCall["toolCallId"].(string)
	var meta devinPendingToolCall
	haveMeta := false
	if idx != nil {
		meta, haveMeta = idx.lookup(toolCallID)
	}

	// Build an effective toolCall view: wire fields first, correlated frame
	// as fallback for anything absent.
	effective := make(map[string]any, len(toolCall)+3)
	for k, v := range toolCall {
		effective[k] = v
	}
	if haveMeta {
		if _, has := effective["title"]; !has && meta.title != "" {
			effective["title"] = meta.title
		}
		if _, has := effective["kind"]; !has && meta.kind != "" {
			effective["kind"] = meta.kind
		}
	}
	title, _ := effective["title"].(string)
	rawInput, _ := toolCall["rawInput"].(map[string]any)
	if rawInput == nil && haveMeta {
		rawInput = meta.rawInput
	}

	toolName := ""
	if haveMeta {
		toolName = meta.toolName
	}
	if toolName == "" {
		toolName = devinToolNameFromPermissionOptions(params["options"])
	}

	// editableCommand is Devin's canonical user-editable shell-command surface
	// and the only field carrying the command on exec approvals (BUG-434).
	command := devinCognitionMetaString(toolCall["_meta"], "cognition.ai/editableCommand")
	if command == "" {
		command = devinCommandFromRawInput(rawInput)
	}
	if command == "" {
		command = title
	}
	if command == "" {
		command = toolName
	}

	reason := title
	if toolName != "" {
		// The tool NAME is what verdict/ask_user/read-only matchers key on —
		// keep it in Reason even when a display title exists.
		reason = toolName
	}
	if reason == "" {
		reason = command
	}

	kind := devinPermissionKind(effective, rawInput)
	if kind == "other" {
		switch {
		case strings.TrimSpace(devinCognitionMetaString(toolCall["_meta"], "cognition.ai/editableCommand")) != "":
			kind = "exec"
		case strings.HasPrefix(strings.ToLower(toolName), "mcp__"):
			kind = "mcp"
		}
	}
	return ApprovalDetails{
		Command: command,
		Reason:  reason,
		Kind:    kind,
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
}

// devinCognitionMetaString reads a `cognition.ai/*` key from a _meta map.
func devinCognitionMetaString(meta any, key string) string {
	m, _ := meta.(map[string]any)
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func devinCommandFromRawInput(rawInput map[string]any) string {
	if rawInput == nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "filepath", "filePath", "path", "file_path"} {
		if s, _ := rawInput[k].(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// devinToolNameFromPermissionOptions extracts the MCP tool name embedded in
// Devin's deterministic option labels — e.g. "Yes, allow calling
// submit_review_outcome on the flowpilot MCP server (this session)" yields
// "mcp__flowpilot__submit_review_outcome". Returns "" when no label matches.
func devinToolNameFromPermissionOptions(v any) string {
	options, _ := v.([]any)
	for _, raw := range options {
		m, _ := raw.(map[string]any)
		name, _ := m["name"].(string)
		const marker = "calling "
		const suffix = " on the flowpilot MCP server"
		i := strings.Index(name, marker)
		if i < 0 {
			continue
		}
		rest := name[i+len(marker):]
		j := strings.Index(rest, suffix)
		if j <= 0 {
			continue
		}
		tool := strings.Trim(rest[:j], " `'\"")
		if tool == "" || strings.ContainsAny(tool, " \t") {
			continue
		}
		return "mcp__flowpilot__" + tool
	}
	return ""
}

func devinPermissionKind(toolCall, rawInput map[string]any) string {
	if toolCall == nil {
		return "other"
	}
	kind, _ := toolCall["kind"].(string)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "edit", "write", "create", "delete", "read":
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

// devinAccountEnv builds the launch env for one managed account — the same
// HOME/XDG isolation + ExtraEnv mapping devinLaunchEnv applies — so callers
// that warm a specific account (Task-439 prewarm) get an identical env even
// when that account is not the active slot yet.
func devinAccountEnv(account ProviderAccount) map[string]string {
	env := map[string]string{}
	for k, v := range account.ExtraEnv {
		env[k] = v
	}
	if account.HomePath != "" {
		env["HOME"] = account.HomePath
		env["XDG_CONFIG_HOME"] = filepath.Join(account.HomePath, ".config")
		env["XDG_DATA_HOME"] = filepath.Join(account.HomePath, ".local", "share")
		if drive, path, ok := windowsHomeDriveAndPath(account.HomePath); ok {
			env["USERPROFILE"] = account.HomePath
			env["APPDATA"] = filepath.Join(account.HomePath, "AppData", "Roaming")
			env["LOCALAPPDATA"] = filepath.Join(account.HomePath, "AppData", "Local")
			env["HOMEDRIVE"] = drive
			env["HOMEPATH"] = path
		}
	}
	return env
}

// devinLaunchEnv resolves the account-scoped scopeKey + launch env for a
// `devin acp` process (shared by the turn adapter factory and probes, mirroring
// opencodeLaunchEnv). Devin resolves config under XDG_CONFIG_HOME and
// credentials/sessions under XDG_DATA_HOME.
func (r *Runner) devinLaunchEnv() (string, map[string]string, error) {
	scopeKey := "default"
	account, err := r.ResolveProviderAccount(string(ProviderKeyDevin), "")
	if err == nil {
		return account.ID, devinAccountEnv(account), nil
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return "", nil, err
	}
	scopeKey = "env:" + home
	env := map[string]string{}
	env["HOME"] = home
	env["XDG_CONFIG_HOME"] = filepath.Join(home, ".config")
	env["XDG_DATA_HOME"] = filepath.Join(home, ".local", "share")
	return scopeKey, env, nil
}
