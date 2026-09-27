package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func TestBug522_TournamentWorktreeCreateFailureDoesNotSpawnInMainWorkspace(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	candidate := agentpack.FlowNode{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament"}
	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, []agentpack.FlowNode{candidate}, "solve")
	time.Sleep(100 * time.Millisecond)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, child := range svc.runs {
		if child.parentRunID == parent.RunID && child.label == candidate.ID {
			t.Fatalf("candidate spawned after worktree creation failed: cwd=%q", child.workspaceCwd)
		}
	}
}

func TestBug524_TournamentRetryJoinIgnoresTerminalChildrenFromPriorCohort(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatal(aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].tournamentAttempt = 1
	svc.runs["old-a"] = &interactiveRun{id: "old-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-0", status: RunStatusCompleted}
	svc.runs["old-b"] = &interactiveRun{id: "old-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-0", status: RunStatusCompleted}
	svc.runs["new-a"] = &interactiveRun{id: "new-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-1", status: RunStatusCompleted}
	svc.runs["new-b"] = &interactiveRun{id: "new-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-1", status: RunStatusRunning}
	svc.mu.Unlock()

	if svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("retry join was satisfied by terminal children from the prior cohort")
	}
}

// BUG-524 residual: a restart landing between the round-2 child spawn and its
// RUNNING sidecar stamp leaves the round-1 DONE stamp as the last durable
// record. The live current-attempt child in s.runs is authoritative — the
// sidecar fallback must not satisfy the join while it is still running.
func TestBug524_SidecarStampDoesNotBypassLiveRetryChild(t *testing.T) {
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatal(aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	// Round-1 terminal stamps survived the restart; the round-2 RUNNING stamp
	// never reached the sidecar before the kill.
	for _, id := range []string{"candidate-a", "candidate-b"} {
		if err := store.AppendStepTransition(context.Background(), parent.RunID, stepTransitionLine{
			RunID: parent.RunID, NodeID: id, Status: string(StepStatusDone), TS: "2026-09-26T11:17:44Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].tournamentAttempt = 1
	svc.runs["new-a"] = &interactiveRun{id: "new-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-1", status: RunStatusRunning}
	svc.runs["new-b"] = &interactiveRun{id: "new-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-1", status: RunStatusCompleted}
	svc.mu.Unlock()

	if svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("stale prior-round sidecar stamp satisfied the join while a current-attempt child is still running")
	}
}

func TestBug525_AutomaticMergeIgnoresDiffInAgentResultMessage(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, _, parentID := bug515ParkedConflict(t, repo)
	recorded := bug478RealPatch(t, repo)

	injectedPath := filepath.Join(repo, "injected.go")
	agentDiff := "diff --git a/injected.go b/injected.go\nnew file mode 100644\nindex 0000000..f48f0ad\n--- /dev/null\n+++ b/injected.go\n@@ -0,0 +1,3 @@\n+package tournamentmini\n+\n+func Injected() bool { return true }\n"

	svc.mu.Lock()
	svc.runs[parentID].tournamentWinner = "candidate-a"
	svc.runs[parentID].tournamentPatches = map[string]string{"candidate-a": recorded}
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], agentDiff)
	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err != nil {
		t.Fatalf("recorded winner patch was not applied: %v", err)
	}
	if _, err := os.Stat(injectedPath); !os.IsNotExist(err) {
		t.Fatalf("agent result diff overrode the recorded winner patch: err=%v", err)
	}
}
