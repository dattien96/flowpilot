package runner

import (
	"fmt"
	"strings"
	"sync"

	"flowpilot-runner/internal/agentpack"
)

// AgentOrchestrator tracks the in-memory agent run tree for CP-19 Phase 1 (Task-082).
// It records parent→child edges and provides completion signaling for wait:true
// spawn_agent tool calls. There is no dependency graph or feedback loop yet (Task-084)
// and no Supabase persistence (Task-085).
type AgentOrchestrator struct {
	mu         sync.Mutex
	children   map[string][]string             // parentRunID → ordered []childRunIDs
	waiters    map[string]chan agentCompletion // childRunID → completion channel (wait:true only)
	historical map[string][]AgentRunSummary    // parentRunID → summaries restored from manifest
	summaries  map[string]map[string]AgentRunSummary
	edges      map[string][]AgentDependencyEdge // parentRunID → DAG edges
	bus        map[string][]AgentBusMessage     // parentRunID → message log
	loop       map[string]AgentLoopState        // parentRunID → loop state
	queued     map[string][]AgentBusMessage     // parentRunID → queued feedback
	// cohort accumulates per-member results for barrier delivery (Task-092).
	// keyed by "parentRunID/cohortId" so multiple cohorts on one parent don't collide.
	cohort         map[string][]cohortEntry
	cohortExpected map[string]int // expected member count per cohort key
}

// cohortEntry is one member's result within a flow cohort barrier.
type cohortEntry struct {
	Label        string
	Provider     string
	FinalMessage string
	Status       string // "completed" | "failed"
	Err          string
}

type agentCompletion struct {
	finalMessage string
	failed       bool
	errMsg       string
	status       RunStatus
}

func newAgentOrchestrator() *AgentOrchestrator {
	return &AgentOrchestrator{
		children:       make(map[string][]string),
		waiters:        make(map[string]chan agentCompletion),
		historical:     make(map[string][]AgentRunSummary),
		summaries:      make(map[string]map[string]AgentRunSummary),
		edges:          make(map[string][]AgentDependencyEdge),
		bus:            make(map[string][]AgentBusMessage),
		loop:           make(map[string]AgentLoopState),
		queued:         make(map[string][]AgentBusMessage),
		cohort:         make(map[string][]cohortEntry),
		cohortExpected: make(map[string]int),
	}
}

// registerCohortMember increments the expected member count for a cohort.
// Call once per child spawned with a non-empty FlowCohortID and no CohortSize.
func (o *AgentOrchestrator) registerCohortMember(parentRunID, cohortID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cohortExpected[cohortKey(parentRunID, cohortID)]++
	cohortDiagLog("registerCohortMember increment parent=%q cohort=%q expectedNow=%d",
		parentRunID, cohortID, o.cohortExpected[cohortKey(parentRunID, cohortID)])
}

// preRegisterCohort sets the expected count for a cohort to count on the first
// call (when expected == 0). Subsequent calls for the same cohort key are no-ops
// so that Wait=true sequential spawns all sharing the same CohortSize don't
// over-count.
func (o *AgentOrchestrator) preRegisterCohort(parentRunID, cohortID string, count int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := cohortKey(parentRunID, cohortID)
	if o.cohortExpected[k] == 0 {
		o.cohortExpected[k] = count
		cohortDiagLog("preRegisterCohort SET parent=%q cohort=%q expected=%d", parentRunID, cohortID, count)
	} else {
		cohortDiagLog("preRegisterCohort no-op (already %d) parent=%q cohort=%q requestedCount=%d",
			o.cohortExpected[k], parentRunID, cohortID, count)
	}
}

// cohortKey returns the map key used to index a cohort buffer.
func cohortKey(parentRunID, cohortID string) string { return parentRunID + "/" + cohortID }

// appendCohortResult adds one member's result to the cohort buffer.
func (o *AgentOrchestrator) appendCohortResult(parentRunID, cohortID string, e cohortEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := cohortKey(parentRunID, cohortID)
	o.cohort[k] = append(o.cohort[k], e)
	cohortDiagLog("appendCohortResult parent=%q cohort=%q label=%q status=%q bufferLen=%d expected=%d",
		parentRunID, cohortID, e.Label, e.Status, len(o.cohort[k]), o.cohortExpected[k])
}

// cohortComplete returns true when the number of buffered results equals the registered
// expected member count for the cohort (and expected > 0).
func (o *AgentOrchestrator) cohortComplete(parentRunID, cohortID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := cohortKey(parentRunID, cohortID)
	exp := o.cohortExpected[k]
	complete := exp > 0 && len(o.cohort[k]) >= exp
	cohortDiagLog("cohortComplete check parent=%q cohort=%q bufferLen=%d expected=%d result=%t",
		parentRunID, cohortID, len(o.cohort[k]), exp, complete)
	return complete
}

// drainCohort removes the cohort buffer and expected-count entry, returning the
// buffered entries. Clearing cohortExpected allows the same cohort key to be
// reused after delivery.
func (o *AgentOrchestrator) drainCohort(parentRunID, cohortID string) []cohortEntry {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := cohortKey(parentRunID, cohortID)
	entries := o.cohort[k]
	if cohortDiagEnabled() {
		labels := make([]string, len(entries))
		for i, e := range entries {
			labels[i] = e.Label + ":" + e.Status
		}
		cohortDiagLog("drainCohort parent=%q cohort=%q entries=%v", parentRunID, cohortID, labels)
	}
	delete(o.cohort, k)
	delete(o.cohortExpected, k)
	return entries
}

// registerChild records the parent→child edge.
func (o *AgentOrchestrator) registerChild(parentRunID, childRunID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.children[parentRunID] = append(o.children[parentRunID], childRunID)
}

// listChildren returns the child run IDs of a parent run in spawn order.
func (o *AgentOrchestrator) listChildren(parentRunID string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	src := o.children[parentRunID]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// openWaiter registers a buffered completion channel for a child run (wait:true).
// Must be called BEFORE the child's first turn starts to eliminate the completion race.
func (o *AgentOrchestrator) openWaiter(childRunID string) <-chan agentCompletion {
	ch := make(chan agentCompletion, 1)
	o.mu.Lock()
	o.waiters[childRunID] = ch
	o.mu.Unlock()
	return ch
}

// signalChild fires any registered wait:true channel for childRunID. Called by
// InteractiveService.emitLocked when EventTurnCompleted or EventTurnFailed fires.
// Idempotent: a second call after the channel is consumed is a no-op.
func (o *AgentOrchestrator) signalChild(childRunID, finalMessage string, failed bool, errMsg string, status RunStatus) {
	o.mu.Lock()
	ch, ok := o.waiters[childRunID]
	if ok {
		delete(o.waiters, childRunID)
	}
	o.mu.Unlock()
	if ok {
		ch <- agentCompletion{finalMessage: finalMessage, failed: failed, errMsg: errMsg, status: status}
		close(ch)
	}
}

// ---- Request / response types ------------------------------------------------

// SpawnAgentInput is the tool call schema for spawn_agent (Codex + Claude) and the
// HTTP request body for POST …/{runId}/spawn-agent.
type SpawnAgentInput struct {
	Agent     string   `json:"agent"`
	Prompt    string   `json:"prompt"`
	Provider  string   `json:"provider,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"`
	Wait      bool     `json:"wait"`
	// UIInitiated is set by the desktop HTTP spawn handler (not decoded from the wire).
	// It tells spawnChildRun to inject a context note into the parent's next provider
	// turn so the parent agent learns about a child it did not spawn itself. The AI
	// spawn_agent tool leaves this false — its spawns are already in provider history. (BUG-122)
	UIInitiated bool `json:"-"`
	// FlowCohortID groups sibling children into a barrier: when every member is
	// terminal the engine delivers ONE consolidated note to the hub rather than N
	// individual lines (Task-092 / CP-36 P-6).
	FlowCohortID string `json:"flowCohortId,omitempty"`
	// Label is the display name used in the consolidated note header; defaults to
	// Agent when empty.
	Label string `json:"label,omitempty"`
	// CohortSize pre-declares the total number of members in this cohort so the
	// barrier fires only after all siblings complete, even with sequential Wait=true
	// spawning. Only the first spawn for a given cohort key uses this value; all
	// subsequent spawns with the same cohort key are no-ops on the expected count.
	// When zero, membership is counted one-by-one via registerCohortMember.
	CohortSize int `json:"cohortSize,omitempty"`
	// AutoOrchestrate enables bounded hub auto-reinvocation (Task-093 / CP-36 P-7).
	// When true on the FIRST spawn of a flow, sets autoOrchestrate on the parent run
	// so the engine re-prompts the hub after each cohort join, bounded by the cap.
	// Normal chat runs (autoOrchestrate=false) are never auto-reinvoked.
	AutoOrchestrate bool `json:"autoOrchestrate,omitempty"`
	// AgentDefOverride bypasses spawnChildRun's name-based catalog lookup
	// entirely when set. Used by the flow executor (flow_executor.go) so a
	// pack-declared node (e.g. review-loop.yaml's agent: agents/coder.md)
	// always runs the pack's own bundled agent, immune to a same-named
	// project-local (.claude/agents, .codex/agents) or provider-home agent
	// silently shadowing it via AgentCatalog.listAgents' precedence order
	// (BUG-NOTE-CP42 #23). Never set from the wire — internal-only, like
	// UIInitiated above.
	AgentDefOverride *AgentDefinition `json:"-"`
	// ParentContextNote injects a parent-visible note immediately after the child
	// run is created but before its first asynchronous turn is launched. Internal
	// only: used by flow startup so the hub reliably sees "work already started"
	// even when a fast child turn would otherwise race ahead and drain pending
	// context before the caller can append the notice.
	ParentContextNote string `json:"-"`
	// Model gives this spawn its own model, taking priority over both the
	// agent definition's model and the parent run's inherited model (BUG-228).
	// Set by the flow executor for an agent.delegate node whose role has its
	// own purpose-named step_definitions row (e.g. "flow-agent-delegate-
	// reviewer"), so that node's children run on their own configured
	// model/provider instead of always inheriting the flow's single resolved
	// model. Never set from the wire — internal-only, like AgentDefOverride.
	Model string `json:"-"`
}

// SpawnAgentResult is the tool call result and HTTP response body.
// FinalMessage is populated only when Wait==true and the child turn completed.
type SpawnAgentResult struct {
	RunID             string `json:"runId"`
	ProviderSessionID string `json:"providerSessionId"`
	ProviderKey       string `json:"providerKey"`
	Status            string `json:"status"`
	FinalMessage      string `json:"finalMessage,omitempty"`
}

// AgentRunSummary is one row in GET …/{runId}/agents.
type AgentRunSummary struct {
	RunID       string    `json:"runId"`
	AgentName   string    `json:"agentName"`
	Label       string    `json:"label,omitempty"`
	Role        string    `json:"role"`
	Status      RunStatus `json:"status"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	CreatedAt   string    `json:"createdAt"`
	DependsOn   []string  `json:"dependsOn,omitempty"`
	AgentStatus string    `json:"agentStatus,omitempty"`
	ProviderKey string    `json:"providerKey,omitempty"`
	ModelName   string    `json:"modelName,omitempty"`
	// WaitForResult mirrors the spawn's wait flag so the desktop can tell which running
	// children block the main run (wait=true) vs. run in the background (wait=false) (BUG-133).
	WaitForResult bool `json:"waitForResult,omitempty"`
	// ActivationSeq is incremented each time a reinvoke-lifecycle child is reactivated
	// (completed → running again) so the desktop can distinguish a genuine reinvoke from
	// a stale HTTP snapshot that BUG-235's terminal-status guard would otherwise block.
	// Zero for the first activation; omitted from JSON when zero.
	ActivationSeq int `json:"activationSeq,omitempty"`
}

// setHistoricalChildren stores agent summaries from a restored sync manifest so that
// listAgentRunSummaries can return them even when the live runs are no longer in memory.
func (o *AgentOrchestrator) setHistoricalChildren(parentRunID string, summaries []AgentRunSummary) {
	if len(summaries) == 0 {
		return
	}
	o.mu.Lock()
	o.historical[parentRunID] = summaries
	o.mu.Unlock()
}

// historicalChildren returns the agent summaries previously stored by setHistoricalChildren.
func (o *AgentOrchestrator) historicalChildren(parentRunID string) []AgentRunSummary {
	o.mu.Lock()
	src := o.historical[parentRunID]
	o.mu.Unlock()
	if len(src) == 0 {
		return nil
	}
	out := make([]AgentRunSummary, len(src))
	copy(out, src)
	return out
}

func (o *AgentOrchestrator) upsertSummary(parentRunID string, summary AgentRunSummary) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.summaries[parentRunID] == nil {
		o.summaries[parentRunID] = make(map[string]AgentRunSummary)
	}
	o.summaries[parentRunID][summary.RunID] = summary
}

func (o *AgentOrchestrator) graphSnapshot(parentRunID string) AgentGraphSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	// Dedup by runId, preferring the live summary (current status) over a historical one.
	// graphSnapshot previously concatenated historical+live unconditionally, so a child
	// present in both (e.g. after a sync/restore round-trip) appeared twice in the Agents
	// panel. Mirrors the dedup listAgentRunSummaries already does. (BUG-116)
	seen := make(map[string]struct{})
	runs := make([]AgentRunSummary, 0)
	for _, r := range o.runsForParentLocked(parentRunID) {
		if _, dup := seen[r.RunID]; dup {
			continue
		}
		seen[r.RunID] = struct{}{}
		runs = append(runs, r)
	}
	for _, h := range o.historical[parentRunID] {
		if _, dup := seen[h.RunID]; dup {
			continue
		}
		seen[h.RunID] = struct{}{}
		runs = append(runs, h)
	}
	edges := append([]AgentDependencyEdge(nil), o.edges[parentRunID]...)
	bus := append([]AgentBusMessage(nil), o.bus[parentRunID]...)
	loop := o.loop[parentRunID]
	if loop.RoundCap == 0 {
		loop.RoundCap = 3
	}
	return AgentGraphSnapshot{ParentRunID: parentRunID, Runs: runs, Edges: edges, BusMessages: bus, LoopState: loop}
}

func (o *AgentOrchestrator) runsForParentLocked(parentRunID string) []AgentRunSummary {
	children := o.children[parentRunID]
	out := make([]AgentRunSummary, 0, len(children))
	for _, childID := range children {
		if summary, ok := o.summaries[parentRunID][childID]; ok {
			out = append(out, summary)
			continue
		}
		out = append(out, AgentRunSummary{RunID: childID, ParentRunID: parentRunID})
	}
	return out
}

func (o *AgentOrchestrator) ensureLoopLocked(parentRunID string) AgentLoopState {
	st := o.loop[parentRunID]
	if st.RoundCap == 0 {
		st.RoundCap = 3
	}
	return st
}

func (o *AgentOrchestrator) setLoop(parentRunID string, state AgentLoopState) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if state.RoundCap == 0 {
		state.RoundCap = 3
	}
	o.loop[parentRunID] = state
}

// loopStateFor returns the AgentLoopState for parentRunID under o.mu, safe to call from
// any goroutine. Use this instead of reading o.loop directly outside AgentOrchestrator.
func (o *AgentOrchestrator) loopStateFor(parentRunID string) AgentLoopState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.loop[parentRunID]
}

func (o *AgentOrchestrator) pause(parentRunID, reason string) AgentGraphSnapshot {
	return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState { st.Status = "paused"; st.GateReason = reason; return st })
}
func (o *AgentOrchestrator) resume(parentRunID string) AgentGraphSnapshot {
	return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState { st.Status = "running"; st.GateReason = ""; return st })
}
func (o *AgentOrchestrator) stop(parentRunID string) AgentGraphSnapshot {
	return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState { st.Status = "stopped"; st.GateReason = "stopped"; return st })
}

func (o *AgentOrchestrator) mutateLoop(parentRunID string, f func(AgentLoopState) AgentLoopState) AgentGraphSnapshot {
	o.mu.Lock()
	st := o.ensureLoopLocked(parentRunID)
	st = f(st)
	o.loop[parentRunID] = st
	o.mu.Unlock()
	return o.graphSnapshot(parentRunID)
}

func (o *AgentOrchestrator) currentSummary(parentRunID, childRunID string) (AgentRunSummary, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if byParent := o.summaries[parentRunID]; byParent != nil {
		summary, ok := byParent[childRunID]
		return summary, ok
	}
	return AgentRunSummary{}, false
}

func (o *AgentOrchestrator) addBus(parentRunID string, msg AgentBusMessage) AgentGraphSnapshot {
	o.mu.Lock()
	o.bus[parentRunID] = append(o.bus[parentRunID], msg)
	st := o.ensureLoopLocked(parentRunID)
	o.loop[parentRunID] = st
	o.mu.Unlock()
	return o.graphSnapshot(parentRunID)
}

func (o *AgentOrchestrator) queueFeedback(parentRunID string, msg AgentBusMessage) AgentGraphSnapshot {
	o.mu.Lock()
	o.queued[parentRunID] = append(o.queued[parentRunID], msg)
	o.bus[parentRunID] = append(o.bus[parentRunID], msg)
	st := o.ensureLoopLocked(parentRunID)
	o.loop[parentRunID] = st
	o.mu.Unlock()
	return o.graphSnapshot(parentRunID)
}

func (o *AgentOrchestrator) nextQueuedFeedback(parentRunID string) *AgentBusMessage {
	o.mu.Lock()
	defer o.mu.Unlock()
	q := o.queued[parentRunID]
	if len(q) == 0 {
		return nil
	}
	msg := q[0]
	o.queued[parentRunID] = q[1:]
	return &msg
}

func (o *AgentOrchestrator) nextQueuedFeedbackFor(parentRunID, toRunID string) *AgentBusMessage {
	o.mu.Lock()
	defer o.mu.Unlock()
	q := o.queued[parentRunID]
	for i, msg := range q {
		if toRunID != "" && msg.ToRunID != "" && msg.ToRunID != toRunID {
			continue
		}
		o.queued[parentRunID] = append(q[:i], q[i+1:]...)
		return &msg
	}
	return nil
}

func (o *AgentOrchestrator) advanceRound(parentRunID string) AgentGraphSnapshot {
	return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status == "" {
			st.Status = "running"
		}
		st.Round++
		if st.Round >= st.RoundCap && st.RoundCap > 0 {
			st.Status = "stopped"
			st.GateReason = "round cap reached"
		}
		return st
	})
}

func (o *AgentOrchestrator) transition(parentRunID, kind string) AgentGraphSnapshot {
	return o.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		paused := st.Status == "paused"
		switch kind {
		case "ready-for-review":
			if !paused {
				st.Status = "waiting_review"
			}
			st.GateReason = "ready for review"
		case "changes-requested":
			if !paused {
				st.Status = "running"
			}
			st.GateReason = "changes requested"
		case "approved":
			st.Status = "approved"
			st.GateReason = "approved"
		case "rejected":
			st.Status = "rejected"
			st.GateReason = "rejected"
		case "handoff":
			if !paused {
				st.Status = "running"
			}
			st.GateReason = "handoff to parent"
		}
		return st
	})
}

// ---- Flow vocabulary (Task-089) -----------------------------------------------
//
// Domain-free node/edge/policy types used by the flow executor (Task-090).
// No role or use-case strings appear here; those live in declared-face registries.

// FlowNode describes one step in an execution graph.
type FlowNode struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	Run       string `json:"run"`       // "inline" | "delegate"
	Lifecycle string `json:"lifecycle"` // "once" | "reinvoke" | "spawn"
	Join      string `json:"join"`      // "all" | "any" | "quorum(n)"
}

// FlowEdge is a directed connection between two FlowNodes.
type FlowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when"` // generic status: "continue" | "done" | "escalate"
	Kind string `json:"kind"` // "forward" | "back"
}

// FlowPolicy configures cap + bounded-extend behaviour for a flow.
type FlowPolicy struct {
	Cap       int    `json:"cap"`
	OnCap     string `json:"onCap"` // "escalate" | "done"
	ExtendBy  int    `json:"extendBy"`
	ExtendMax int    `json:"extendMax"`
}

// FlowControlInput is the one generic signal an agent sends to the engine.
type FlowControlInput struct {
	Status  string         `json:"status"` // "continue" | "done" | "escalate"
	Summary string         `json:"summary,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// FlowControlResult is the engine's reply after processing a FlowControlInput.
type FlowControlResult struct {
	Status     string `json:"status"`
	Round      int    `json:"round"`
	Cap        int    `json:"cap"`
	OpenIssues int    `json:"openIssues,omitempty"`
	NextAction string `json:"nextAction,omitempty"`
}

// FlowControlFace maps a domain tool's status strings onto the three generic ones.
type FlowControlFace struct {
	Tool string            `json:"tool"`
	Map  map[string]string `json:"map"`
}

var flowControlStatuses = map[string]bool{
	"continue": true,
	"done":     true,
	"escalate": true,
}

// parseFlowControlInput extracts FlowControlInput from a tool-call arguments map,
// mirroring parseSpawnAgentInput.
func parseFlowControlInput(args map[string]any) (FlowControlInput, error) {
	var in FlowControlInput
	status, _ := args["status"].(string)
	if !flowControlStatuses[status] {
		return in, fmt.Errorf("flow_control: status must be continue|done|escalate, got %q", status)
	}
	in.Status = status
	in.Summary, _ = args["summary"].(string)
	if raw, ok := args["payload"].(map[string]any); ok {
		in.Payload = raw
	}
	return in, nil
}

// applyFlowNodeDefaults fills zero-value fields with engine defaults.
func applyFlowNodeDefaults(n *FlowNode) {
	if n.Run == "" {
		n.Run = "delegate"
	}
	if n.Lifecycle == "" {
		n.Lifecycle = "reinvoke"
	}
	if n.Join == "" {
		n.Join = "all"
	}
}

// applyFlowPolicyDefaults fills zero-value fields with engine defaults.
func applyFlowPolicyDefaults(p *FlowPolicy) {
	if p.Cap == 0 {
		p.Cap = 3
	}
	if p.OnCap == "" {
		p.OnCap = "escalate"
	}
	if p.ExtendBy == 0 {
		p.ExtendBy = 2
	}
	if p.ExtendMax == 0 {
		p.ExtendMax = 2
	}
}

// parseJoin parses "all", "any", or "quorum(n)" into a mode string and threshold.
// Returns ("all", 0) / ("any", 0) / ("quorum", n≥1) or an error.
func parseJoin(s string) (mode string, n int, err error) {
	switch s {
	case "", "all":
		return "all", 0, nil
	case "any":
		return "any", 0, nil
	}
	if _, scanErr := fmt.Sscanf(s, "quorum(%d)", &n); scanErr == nil && n > 0 {
		return "quorum", n, nil
	}
	return "", 0, fmt.Errorf("parseJoin: invalid join %q; expected all|any|quorum(n)", s)
}

// resolveFaceStatus maps a domain status through a FlowControlFace to a generic status.
func resolveFaceStatus(face FlowControlFace, domainStatus string) (genericStatus string, ok bool) {
	genericStatus, ok = face.Map[domainStatus]
	return
}

// reviewOutcomeFace returns the declared face for the submit_review_outcome
// tool. BUG-NOTE-CP42 #11: submit-review-outcome.yaml already declares this
// exact mapping as pack data (statusMap: approved->done, changes_requested->
// continue, blocked->escalate) — CP-42 P-5 requires declared faces to
// actually become pack data, not just be parsed and then ignored in favor of
// a hardcoded Go literal duplicating the same values. Reads the pack's own
// face when available; falls back to the literal (matching the pack's
// current content) only if the pack can't be loaded or has no such face,
// so a packaging problem degrades gracefully instead of breaking the review
// loop outright.
func reviewOutcomeFace() FlowControlFace {
	if face, ok, err := agentpack.LoadBuiltinToolFace("submit_review_outcome"); err == nil && ok && len(face.StatusMap) > 0 {
		return FlowControlFace{Tool: face.ID, Map: face.StatusMap}
	}
	return FlowControlFace{
		Tool: "submit_review_outcome",
		Map: map[string]string{
			"approved":          "done",
			"changes_requested": "continue",
			"blocked":           "escalate",
		},
	}
}

// validateFlowEdges rejects configurations where more than one back-edge fires
// on the same generic status (which would create an ambiguous routing decision).
func validateFlowEdges(edges []FlowEdge) error {
	backTargets := map[string]string{} // when → from (first seen)
	for _, e := range edges {
		if e.Kind != "back" {
			continue
		}
		if prev, ok := backTargets[e.When]; ok {
			return fmt.Errorf("validateFlowEdges: duplicate back-edge for status %q (from %q and %q)", e.When, prev, e.From)
		}
		backTargets[e.When] = e.From
	}
	return nil
}

// effectiveCap returns the flow-engine cap for a loop state: Cap if set, else RoundCap, else 3.
func effectiveCap(st AgentLoopState) int {
	if st.Cap > 0 {
		return st.Cap
	}
	if st.RoundCap > 0 {
		return st.RoundCap
	}
	return 3
}

// effectiveExtendBy returns st's configured cap-extension step (seeded from
// the flow's own Definition.Policy.ExtendBy at startResolvedFlow), falling
// back to 2 for loops that never went through that path (e.g. an AI-driven
// spawn_agent run with no tracked flow topology).
func effectiveExtendBy(st AgentLoopState) int {
	if st.ExtendBy > 0 {
		return st.ExtendBy
	}
	return 2
}

// ---- Review-loop template (Task-091) ------------------------------------------
//
// submit_review_outcome is the declared face of flow_control for the review loop.
// It maps {approved→done, changes_requested→continue, blocked→escalate} via
// reviewOutcomeFace() (Task-089) and places issues in FlowControlInput.Payload.

// ReviewIssue is one code-review finding from the reviewer agent.
type ReviewIssue struct {
	ID         string `json:"id,omitempty"`
	Title      string `json:"title"`
	Severity   string `json:"severity,omitempty"` // "error"|"warning"|"info"
	File       string `json:"file,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

// ReviewOutcomeInput is the schema for the submit_review_outcome tool call.
type ReviewOutcomeInput struct {
	Status   string        `json:"status"` // "approved"|"changes_requested"|"blocked"
	Issues   []ReviewIssue `json:"issues,omitempty"`
	Feedback string        `json:"feedback,omitempty"`
}

// ReviewOutcomeResult is the tool call result: mirrors FlowControlResult with open count.
type ReviewOutcomeResult struct {
	FlowControlResult
	OpenIssues int `json:"openIssues,omitempty"`
}

var reviewOutcomeStatuses = map[string]bool{
	"approved":          true,
	"changes_requested": true,
	"blocked":           true,
}

// parseReviewOutcomeInput validates and extracts ReviewOutcomeInput from a tool-call args map.
//
// Field aliases (for TS/board compatibility):
//   - "outcome" accepted as alias for "status"
//   - Per-issue "description" accepted as alias for "title"
//   - Per-issue "location" accepted as alias for "file"
//
// feedback is required for changes_requested only on the MCP/agent path (args["status"]);
// the board path (args["outcome"]) omits feedback and that is fine.
func parseReviewOutcomeInput(args map[string]any) (ReviewOutcomeInput, error) {
	var in ReviewOutcomeInput
	boardPath := false
	in.Status, _ = args["status"].(string)
	if in.Status == "" {
		in.Status, _ = args["outcome"].(string)
		boardPath = true
	}
	if !reviewOutcomeStatuses[in.Status] {
		return in, fmt.Errorf("submit_review_outcome: status must be approved|changes_requested|blocked, got %q", in.Status)
	}
	in.Feedback, _ = args["feedback"].(string)
	if !boardPath && in.Status == "changes_requested" && strings.TrimSpace(in.Feedback) == "" {
		return in, fmt.Errorf("submit_review_outcome: feedback is required when status=changes_requested")
	}
	if raw, ok := args["issues"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			issue := ReviewIssue{}
			issue.ID, _ = m["id"].(string)
			// Accept both "title" (Go/MCP) and "description" (TS/board) as issue title.
			issue.Title, _ = m["title"].(string)
			if issue.Title == "" {
				issue.Title, _ = m["description"].(string)
			}
			issue.Severity, _ = m["severity"].(string)
			// Accept both "file" (Go/MCP) and "location" (TS/board) as issue location.
			issue.File, _ = m["file"].(string)
			if issue.File == "" {
				issue.File, _ = m["location"].(string)
			}
			issue.Resolution, _ = m["resolution"].(string)
			if issue.Title != "" {
				in.Issues = append(in.Issues, issue)
			}
		}
	}
	return in, nil
}

// reviewOutcomeToFlowControl maps a ReviewOutcomeInput to a FlowControlInput using the
// declared face registry. Issues and feedback ride in Payload for the coder re-entry note.
func reviewOutcomeToFlowControl(in ReviewOutcomeInput) (FlowControlInput, error) {
	face := reviewOutcomeFace()
	generic, ok := resolveFaceStatus(face, in.Status)
	if !ok {
		return FlowControlInput{}, fmt.Errorf("reviewOutcomeToFlowControl: unknown status %q", in.Status)
	}
	payload := map[string]any{
		"issues":   in.Issues,
		"feedback": in.Feedback,
	}
	return FlowControlInput{
		Status:  generic,
		Summary: in.Feedback,
		Payload: payload,
	}, nil
}

// loopMode returns the Mode field of a parent run's loop state (e.g. "keyword"|"explicit").
func (o *AgentOrchestrator) loopMode(parentRunID string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.loop[parentRunID].Mode
}

// parseSpawnAgentInput extracts SpawnAgentInput from a tool call arguments map.
func parseSpawnAgentInput(args map[string]any) (SpawnAgentInput, error) {
	var in SpawnAgentInput
	agent, _ := args["agent"].(string)
	if agent == "" {
		return in, fmt.Errorf("spawn_agent: agent is required")
	}
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return in, fmt.Errorf("spawn_agent: prompt is required")
	}
	in.Agent = agent
	in.Prompt = prompt
	in.Provider, _ = args["provider"].(string)
	in.Wait, _ = args["wait"].(bool)
	in.FlowCohortID, _ = args["flowCohortId"].(string)
	in.Label, _ = args["label"].(string)
	in.AutoOrchestrate, _ = args["autoOrchestrate"].(bool)
	if cs, ok := args["cohortSize"].(float64); ok {
		in.CohortSize = int(cs)
	}
	if rawDeps, ok := args["dependsOn"].([]any); ok {
		for _, d := range rawDeps {
			if s, ok := d.(string); ok && s != "" {
				in.DependsOn = append(in.DependsOn, s)
			}
		}
	}
	return in, nil
}
