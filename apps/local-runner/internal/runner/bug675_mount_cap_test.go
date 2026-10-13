package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-675 (live run-752742): stashVibeFlowForDebate ledgers the mount budget
// BEFORE the debate overlay materializes. When the mount dies in flight —
// maybeReleaseVibeDebateClaimIfMountDied restores the claim because the
// overlay never landed — the spent mount is NOT refunded. Three dead mounts
// burn the per-entity cap (maxVibeDebateMountsPerSprint=3), the next real
// divert escalates vibe_debate_mount_cap, parks the sprint, and the park
// freeze cancels the live cohort underneath it: a mount that never ran
// kills real work. A released-as-dead mount must refund the budget.

func TestBug675_DeadDebateMountRefundsBudget(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-675a"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		chatFlowRef:      "flowpack:vibe-sprint",
		activeFlowNodes:  []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs["child-675a"] = &interactiveRun{
		id:          "child-675a",
		parentRunID: parentID,
		status:      RunStatusRunning,
		label:       "reviewer",
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.stashVibeFlowForDebate(parentID, "child-675a") {
		t.Fatal("stashVibeFlowForDebate refused — fixture broken")
	}
	svc.mu.Lock()
	rs := svc.runs[parentID]
	gotMounts := rs.vibeDebateMounts
	gotEntity := rs.vibeDebateMountsByEntity["node:reviewer"]
	svc.mu.Unlock()
	if gotMounts != 1 || gotEntity != 1 {
		t.Fatalf("stash did not ledger the mount: mounts=%d entity=%d", gotMounts, gotEntity)
	}

	// The mount goroutine dies before the overlay lands — claim released.
	svc.maybeReleaseVibeDebateClaimIfMountDied(parentID)

	svc.mu.Lock()
	rs = svc.runs[parentID]
	gotMounts = rs.vibeDebateMounts
	gotEntity = rs.vibeDebateMountsByEntity["node:reviewer"]
	svc.mu.Unlock()
	if gotMounts != 0 || gotEntity != 0 {
		t.Fatalf("dead mount kept the budget: mounts=%d entity=%d — three dead mounts park the sprint at cap", gotMounts, gotEntity)
	}
}

// A claim released because the overlay DID mount and conclude keeps its
// budget — only provably-dead mounts refund. Restore after a real debate
// (chatFlowRef swapped to the debate graph) must not touch the ledger.
func TestBug675_ConcludedDebateKeepsBudget(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-675b"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		chatFlowRef:      "flowpack:vibe-sprint",
		activeFlowNodes:  []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.stashVibeFlowForDebate(parentID, "") {
		t.Fatal("stashVibeFlowForDebate refused — fixture broken")
	}
	// Overlay materialized: the debate graph + ref are now live.
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.chatFlowRef = "flowpack:vibe-owner-debate"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "owner_1", Behavior: "agent.delegate"},
		{ID: "owner_2", Behavior: "agent.delegate"},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	rs.vibeDebateMountInFlight = false
	svc.mu.Unlock()

	svc.maybeReleaseVibeDebateClaimIfMountDied(parentID)

	svc.mu.Lock()
	got := svc.runs[parentID].vibeDebateMounts
	svc.mu.Unlock()
	if got != 1 {
		t.Fatalf("concluded debate mount refunded: mounts=%d — budget accounting regressed", got)
	}
}
