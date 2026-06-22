package runner

import (
	"fmt"
	"sync"
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
}

type agentCompletion struct {
	finalMessage string
	failed       bool
	errMsg       string
	status       RunStatus
}

func newAgentOrchestrator() *AgentOrchestrator {
	return &AgentOrchestrator{
		children:   make(map[string][]string),
		waiters:    make(map[string]chan agentCompletion),
		historical: make(map[string][]AgentRunSummary),
		summaries:  make(map[string]map[string]AgentRunSummary),
		edges:      make(map[string][]AgentDependencyEdge),
		bus:        make(map[string][]AgentBusMessage),
		loop:       make(map[string]AgentLoopState),
		queued:     make(map[string][]AgentBusMessage),
	}
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
	Role        string    `json:"role"`
	Status      RunStatus `json:"status"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	CreatedAt   string    `json:"createdAt"`
	DependsOn   []string  `json:"dependsOn,omitempty"`
	AgentStatus string    `json:"agentStatus,omitempty"`
	ProviderKey string    `json:"providerKey,omitempty"`
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
	runs := append([]AgentRunSummary(nil), o.historical[parentRunID]...)
	runs = append(runs, o.runsForParentLocked(parentRunID)...)
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
	if rawDeps, ok := args["dependsOn"].([]any); ok {
		for _, d := range rawDeps {
			if s, ok := d.(string); ok && s != "" {
				in.DependsOn = append(in.DependsOn, s)
			}
		}
	}
	return in, nil
}
