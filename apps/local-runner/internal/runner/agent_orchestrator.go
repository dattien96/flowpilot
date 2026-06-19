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
