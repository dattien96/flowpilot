package runner

// Additive regression for run-9034:
// Flow auto-spawn uses wait:false, so the Wait=true branch in spawnAgent never
// emitted agent_result_injected on the parent. After gate settle,
// settleFlowChildTurnCompletedLocked must annotate the parent so the main-chat
// agent card closes (finalMessage) and a later reinvoke can open a new card.

import (
	"testing"
	"time"
)

func TestRun9034SettleEmitsParentAgentResultWithChildRunID(t *testing.T) {
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parentID := "run-9034-parent"
	childID := "run-9039-coder"
	now := time.Now().UTC().Format(time.RFC3339Nano)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, status: RunStatusRunning,
		createdAt: now, updatedAt: now, flowEngineDriven: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.runs[childID] = &interactiveRun{
		id: childID, parentRunID: parentID, agentName: "coder", label: "grok-coder",
		role: "coder", providerKey: ProviderKeyGrok, status: RunStatusRunning,
		createdAt: now, updatedAt: now, waitForResult: false,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Unlock()

	svc.mu.Lock()
	child := svc.runs[childID]
	svc.settleFlowChildTurnCompletedLocked(child, "implemented round 0", ProviderEvent{
		Type: EventTurnCompleted, FinalMessage: "implemented round 0", OccurredAt: now,
	})
	parentEvents := append([]ProviderEvent(nil), svc.runs[parentID].events...)
	svc.mu.Unlock()

	var got *ProviderEvent
	for i := range parentEvents {
		if parentEvents[i].Type == EventAgentResultInjected {
			got = &parentEvents[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("parent events missing agent_result_injected: %+v", parentEvents)
	}
	if got.ChildRunID != childID {
		t.Fatalf("ChildRunID = %q, want %q (desktop binds result to the open card by child id)", got.ChildRunID, childID)
	}
	if got.AgentName != "coder" {
		t.Fatalf("AgentName = %q, want coder", got.AgentName)
	}
	if got.FinalMessage != "implemented round 0" {
		t.Fatalf("FinalMessage = %q, want implemented round 0", got.FinalMessage)
	}
}

func TestRun9034EmitParentAgentResultNoopWithoutParent(t *testing.T) {
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.mu.Lock()
	defer svc.mu.Unlock()
	// Must not panic when parent is missing / empty.
	svc.emitParentAgentResultLocked(&interactiveRun{id: "orphan", parentRunID: "", agentName: "coder"}, "msg")
	svc.emitParentAgentResultLocked(&interactiveRun{id: "orphan2", parentRunID: "missing", agentName: "coder"}, "msg")
	svc.emitParentAgentResultLocked(&interactiveRun{id: "child", parentRunID: "p", agentName: "coder"}, "   ")
}
