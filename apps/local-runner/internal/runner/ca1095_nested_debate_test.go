package runner

// CA-1095 (nested owner-debate suppression): while an owner debate is active
// on a hub, a second mount request — from ANY trigger path — must not launch
// a second debate flow. Live run-3362: violation-routed escalations during a
// mounted debate spawned concurrent debates, stranding the parked sprint.
// The mount decision is atomic inside stashVibeFlowForDebate.

import (
	"sync"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func newDebateFixture(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.workingMode = workingmode.Vibe
	prs.chatFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "audit", Behavior: "command.audit"},
	}
	svc.mu.Unlock()
	return svc, parent.RunID
}

// First mount parks + reports true; a second mount (any trigger) reports
// false — no nested debate — while still recording the gated child for the
// post-debate reprompt.
func TestCA1095_NestedDebateMountSuppressed(t *testing.T) {
	svc, parentID := newDebateFixture(t)

	if !svc.stashVibeFlowForDebate(parentID, "child-a") {
		t.Fatal("first mount must own the park")
	}
	if svc.stashVibeFlowForDebate(parentID, "child-b") {
		t.Fatal("second mount while a debate is parked must be suppressed")
	}

	svc.mu.Lock()
	rs := svc.runs[parentID]
	gated := append([]string(nil), rs.vibeParkedGatedRunIDs...)
	parkedLen := len(rs.vibeParkedNodes)
	svc.mu.Unlock()
	if parkedLen != 2 {
		t.Fatalf("sprint topology must stay parked once, got %d nodes", parkedLen)
	}
	if len(gated) != 2 || gated[0] != "child-a" || gated[1] != "child-b" {
		t.Fatalf("every gated child must be recorded for reprompt, got %v", gated)
	}
}

// The same suppression holds when the debate is mounted as the chat flow but
// the parked topology was already consumed (restore edge): chatFlowRef alone
// proves a debate owns the run.
func TestCA1095_MountedDebateFlowRefSuppresses(t *testing.T) {
	svc, parentID := newDebateFixture(t)
	svc.mu.Lock()
	svc.runs[parentID].chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	svc.mu.Unlock()

	if svc.stashVibeFlowForDebate(parentID, "child-x") {
		t.Fatal("an active debate chat flow must suppress a new mount even with no parked nodes")
	}
	svc.mu.Lock()
	gated := svc.runs[parentID].vibeParkedGatedRunIDs
	svc.mu.Unlock()
	if len(gated) != 1 || gated[0] != "child-x" {
		t.Fatalf("suppressed mount must still record the gated child, got %v", gated)
	}
}

// The mount decision is atomic under s.mu: N concurrent mounts must yield
// exactly one park owner — concurrent gate fires cannot double-mount.
func TestCA1095_ConcurrentMountsSingleOwner(t *testing.T) {
	svc, parentID := newDebateFixture(t)

	var wg sync.WaitGroup
	var mu sync.Mutex
	owners := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if svc.stashVibeFlowForDebate(parentID, "child") {
				mu.Lock()
				owners++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if owners != 1 {
		t.Fatalf("exactly one concurrent mount must own the park, got %d", owners)
	}
}
