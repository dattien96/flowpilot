package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-618 (live run-150388, Task-032 rewind): the resume-from-node advance
// path rejected the vibe-sprint `tdd` node — behavior `agent.scaffold` — as
// "not a spawnable delegate" (flow_advance_target_not_spawnable). The
// spawnable filter listed delegate/code/reproduce but missed scaffold even
// though agentpack.ProviderBackedBehavior already treats it as the same
// provider-turn family — so "Resume from context" consumed the gate, flipped
// the loop running, and silently spawned nothing. Fix: use
// ProviderBackedBehavior so the check stays in sync with the scope-delegate
// registry.
func TestTryAdvanceFlowFromNodeSpawnsScaffold(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			var mu sync.Mutex
			var prompts []string
			reg := newProviderRegistry()
			reg.register(ProviderRegistration{
				Key: pk, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
						mu.Lock()
						prompts = append(prompts, req.Prompt)
						mu.Unlock()
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "scaffolded"})
						return nil
					})
				},
			})

			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 20, RoundCap: 20})
			svc.mu.Lock()
			svc.runs[parent.RunID].flowEngineDriven = true
			svc.runs[parent.RunID].autoOrchestrate = true
			svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{{From: "context", To: "tdd", When: "done", Kind: "forward"}}
			svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
				{ID: "context", Behavior: "context.produce"},
				{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md", Lifecycle: "reinvoke"},
			}
			svc.mu.Unlock()

			if !svc.tryAdvanceFlowFromNode(parent.RunID, "context", "context result") {
				t.Fatal("tryAdvanceFlowFromNode returned false for scaffold target")
			}
			waitLoop(t, "tdd scaffold leg spawned", 5*time.Second, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				for _, run := range svc.runs {
					if run.parentRunID == parent.RunID && run.label == "tdd" {
						return true
					}
				}
				return false
			})
		})
	}
}
