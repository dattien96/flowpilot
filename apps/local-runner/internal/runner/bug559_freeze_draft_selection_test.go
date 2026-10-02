package runner

// BUG-559 (live run-60899): contract.freeze resolved the planner draft via
// "latest FinalMessage of the matching planner child" — a later verdict /
// reasoning turn on the same leg masked an earlier valid JSON draft, so the
// freeze strict-parsed prose (live: "invalid character 'l' in literal false"
// from a verdict detail's {…flase…}) and escalated/parked.
//
// Companion defects fixed here:
//   - cachePreflightDraftLocked cleared the durable draft stash on ANY
//     unparseable scout-labeled completion — including engine-issued
//     verdict-reprompt turns that were never asked for a draft.
//   - With one planner leg per vibe sprint, several same-label children
//     exist; selection was unordered (map iteration) and unscoped — a prior
//     task's draft could freeze for the current task.
//   - reinvokeMatchingFlowChild walked children oldest-first, so a retried
//     delegate re-drove a stale earlier leg.
//   - A retried cohort-member delegate never re-armed its drained cohort, so
//     its completion was dropped and the join target was never re-dispatched.
//
// New file; no pre-existing test is modified.

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// bug559SeedPlannerChild registers a completed planner-labeled child whose
// turn events emit msgs oldest→newest (each becomes one FinalMessage).
// Call order defines spawn order — later calls are "newer" children.
func bug559SeedPlannerChild(svc *InteractiveService, runID, childID, label string, msgs ...string) *interactiveRun {
	child := &interactiveRun{
		id:          childID,
		parentRunID: runID,
		label:       label,
		agentName:   "contract-planner",
		status:      RunStatusCompleted,
		subs:        map[int64]chan ProviderEvent{},
	}
	for _, m := range msgs {
		child.events = append(child.events, ProviderEvent{Type: EventTurnCompleted, FinalMessage: m})
	}
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, child.id)
	return child
}

const bug559DraftTask24 = `{"feature_key":"shredder-ndk","intent":"task 24 draft","declared_paths":["a.cpp"],"source_doc_id":"Task-024"}`
const bug559DraftTask23 = `{"feature_key":"shredder-ndk","intent":"task 23 draft","declared_paths":["a.cpp"],"source_doc_id":"Task-023"}`

// The live repro: the planner child's earlier turn holds the valid draft;
// its latest turn is verdict prose. Selection must parse-gate and return the
// draft, not the newest message.
func TestBug559_FreezeFallbackPrefersEarlierDraftOverVerdictProse(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)

	bug559SeedPlannerChild(svc, runID, "run-planner-1", scoutNodeID,
		validPlannerDraft,
		"Verdict recorded: approved. The contract looks consistent with the audit findings.",
	)

	got := svc.findPlannerResultForFreeze(runID, edges, nodes, "preflight_contract_freeze")
	if got != validPlannerDraft {
		t.Fatalf("findPlannerResultForFreeze = %q, want the earlier parseable draft (latest prose must be skipped)", got)
	}
}

// Across vibe sprints several same-label planner legs exist; the current
// task's draft must come from the NEWEST leg — and a draft scoped to a
// different Task must never be frozen for this sprint.
func TestBug559_FreezeFallbackPrefersNewestChildAndScopesByTask(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeTaskPlan = []string{"Task-023-a.md", "Task-024-b.md"}
	rs.vibeSprintIndex = 2 // current = index-1 -> Task-024
	svc.mu.Unlock()

	// Older sprint's leg first, then the current sprint's leg (spawn order).
	bug559SeedPlannerChild(svc, runID, "run-planner-t23", scoutNodeID, bug559DraftTask23)
	bug559SeedPlannerChild(svc, runID, "run-planner-t24", scoutNodeID,
		bug559DraftTask24,
		"Verdict recorded: approved.",
	)

	got := svc.findPlannerResultForFreeze(runID, edges, nodes, "preflight_contract_freeze")
	if got != bug559DraftTask24 {
		t.Fatalf("findPlannerResultForFreeze = %q, want the current sprint's Task-024 draft", got)
	}
}

// If the current sprint's leg produced only prose, a stale draft scoped to a
// different task must NOT be resurrected — fail closed instead.
func TestBug559_FreezeFallbackRejectsDraftScopedToOtherTask(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeTaskPlan = []string{"Task-023-a.md", "Task-024-b.md"}
	rs.vibeSprintIndex = 2
	svc.mu.Unlock()

	bug559SeedPlannerChild(svc, runID, "run-planner-t23", scoutNodeID, bug559DraftTask23)
	bug559SeedPlannerChild(svc, runID, "run-planner-t24", scoutNodeID, "Verdict recorded: approved.")

	if got := svc.findPlannerResultForFreeze(runID, edges, nodes, "preflight_contract_freeze"); got != "" {
		t.Fatalf("a draft scoped to Task-023 must never freeze Task-024's contract; got %q", got)
	}
}

// A verdict-bearing completion (submit_review_outcome buffered) is not a
// draft attempt — it must not clear the durable stash.
func TestBug559_VerdictCompletionDoesNotClearDraftStash(t *testing.T) {
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.preflightDraftResult = validPlannerDraft
	prs.pendingReviewVerdictByLabel = map[string]string{scoutNodeID: "approved"}
	child := &interactiveRun{
		id:          "run-planner-v",
		parentRunID: parent.RunID,
		label:       scoutNodeID,
		status:      RunStatusCompleted,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs[child.id] = child
	mutated := svc.cachePreflightDraftLocked(child, "Verdict recorded: approved.")
	stash := prs.preflightDraftResult
	svc.mu.Unlock()

	if mutated {
		t.Fatal("a verdict-only scout completion must not mutate the draft stash")
	}
	if stash != validPlannerDraft {
		t.Fatalf("preflightDraftResult = %q, want stash preserved across a verdict turn", stash)
	}
}

// The marker-based variant: an engine-issued verdict reprompt turn (flag set
// at schedule time) must not clear the stash either.
func TestBug559_VerdictRepromptTurnDoesNotClearDraftStash(t *testing.T) {
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.preflightDraftResult = validPlannerDraft
	child := &interactiveRun{
		id:                      "run-planner-vr",
		parentRunID:             parent.RunID,
		label:                   scoutNodeID,
		status:                  RunStatusCompleted,
		verdictRepromptInFlight: true,
		subs:                    map[int64]chan ProviderEvent{},
	}
	svc.runs[child.id] = child
	svc.cachePreflightDraftLocked(child, "verdict prose without a draft")
	stash := prs.preflightDraftResult
	stale := prs.preflightDraftStale
	svc.mu.Unlock()

	if stash != validPlannerDraft {
		t.Fatalf("verdict-reprompt completion cleared the stash: %q", stash)
	}
	if stale {
		t.Fatal("a verdict-reprompt turn never attempted a draft — stale must not be set")
	}
}

// A genuine scout re-run that completes without a parseable draft supersedes
// the stash AND marks it stale so event-scan cannot resurrect the old draft.
func TestBug559_FailedScoutRerunMarksStaleAndBlocksResurrection(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)

	child := bug559SeedPlannerChild(svc, runID, "run-planner-x", scoutNodeID, validPlannerDraft)
	svc.mu.Lock()
	svc.runs[runID].preflightDraftResult = validPlannerDraft
	svc.mu.Unlock()

	// The scout's re-run completes with prose (no verdict, no marker): a real
	// draft attempt that failed.
	svc.mu.Lock()
	mutated := svc.cachePreflightDraftLocked(child, "sorry, here is prose instead")
	stale := svc.runs[runID].preflightDraftStale
	stash := svc.runs[runID].preflightDraftResult
	svc.mu.Unlock()

	if !mutated || stash != "" {
		t.Fatalf("failed scout re-run must clear the stash (mutated=%v stash=%q)", mutated, stash)
	}
	if !stale {
		t.Fatal("failed scout re-run must mark the draft stale")
	}
	if got := svc.findPlannerResultForFreeze(runID, edges, nodes, "preflight_contract_freeze"); got != "" {
		t.Fatalf("event-scan must not resurrect a superseded draft; got %q", got)
	}
}

// Reinvoke must pick the NEWEST same-label child: with one planner leg per
// sprint, retrying the delegate must re-drive the current sprint's leg.
func TestBug559_ReinvokePrefersNewestMatchingChild(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)

	old := bug559SeedPlannerChild(svc, runID, "run-planner-old", scoutNodeID, "done")
	newer := bug559SeedPlannerChild(svc, runID, "run-planner-new", scoutNodeID, "done")

	if !svc.reinvokeMatchingFlowChild(runID, "retry the draft", func(c *interactiveRun) bool {
		return c.label == scoutNodeID
	}) {
		t.Fatal("expected a matching child to be reinvoked")
	}

	svc.mu.Lock()
	oldTouched := old.status == RunStatusRunning || old.activationSeq > 0
	newTouched := newer.status == RunStatusRunning || newer.activationSeq > 0
	svc.mu.Unlock()
	if !newTouched || oldTouched {
		t.Fatalf("reinvoke must pick the newest leg: old=%v new=%v", oldTouched, newTouched)
	}
}

// A retried cohort-member delegate must re-arm its drained cohort so the new
// completion rejoins the barrier instead of being dropped.
func TestBug559_RearmDrainedCohort(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)

	cid := "flow-adhoc-" + scoutNodeID
	child := bug559SeedPlannerChild(svc, runID, "run-planner-c", scoutNodeID, "done")
	child.flowCohortId = cid

	// Deliver and drain the cohort (tombstoned).
	svc.agentOrchestrator.preRegisterCohort(runID, cid, 1)
	svc.agentOrchestrator.appendCohortResult(runID, cid, cohortEntry{Label: scoutNodeID, Status: "completed"})
	if entries := svc.agentOrchestrator.drainCohort(runID, cid); len(entries) != 1 {
		t.Fatalf("drain returned %d entries, want 1", len(entries))
	}
	if got := svc.agentOrchestrator.cohortExpectedCount(runID, cid); got != 0 {
		t.Fatalf("post-drain expected = %d, want 0", got)
	}

	svc.mu.Lock()
	svc.rearmCohortIfDrainedLocked(child)
	svc.mu.Unlock()

	if got := svc.agentOrchestrator.cohortExpectedCount(runID, cid); got != 1 {
		t.Fatalf("rearmed cohort expected = %d, want 1 (retried member rejoins)", got)
	}
	// And the member's next completion must actually append + complete.
	svc.agentOrchestrator.appendCohortResult(runID, cid, cohortEntry{Label: scoutNodeID, Status: "completed"})
	if !svc.agentOrchestrator.cohortComplete(runID, cid) {
		t.Fatal("re-armed cohort must complete on the retried member's result")
	}
}

// A still-open cohort must never be inflated by the rearm helper.
func TestBug559_RearmDoesNotInflateLiveCohort(t *testing.T) {
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cid := "flow-adhoc-" + scoutNodeID
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, cid, 3)

	svc.mu.Lock()
	child := &interactiveRun{id: "c1", parentRunID: parent.RunID, label: scoutNodeID, flowCohortId: cid}
	svc.rearmCohortIfDrainedLocked(child)
	svc.mu.Unlock()

	if got := svc.agentOrchestrator.cohortExpectedCount(parent.RunID, cid); got != 3 {
		t.Fatalf("live cohort expected = %d, want 3 unchanged", got)
	}
}

// Guard the scope helper itself: only a bare/extractable Task-NNN reference
// participates in the gate; anything else is unscoped.
func TestBug559_VibeTaskDocID(t *testing.T) {
	cases := map[string]string{
		"Task-023-shredder-ndk-fdguard-ringbuffer.md": "Task-023",
		"Task-023.md":  "Task-023",
		"Task-023":     "Task-023",
		"Task-7-x.md":  "Task-7",
		"CP-02-core.md": "",
		"plan.md":      "",
		"":             "",
		"Task-x.md":    "",
	}
	for in, want := range cases {
		if got := vibeTaskDocID(in); got != want {
			t.Fatalf("vibeTaskDocID(%q) = %q, want %q", in, got, want)
		}
	}
}

// Sanity: the drafts used above actually parse under the strict gate.
func TestBug559_FixturesParse(t *testing.T) {
	for name, msg := range map[string]string{"t23": bug559DraftTask23, "t24": bug559DraftTask24, "valid": validPlannerDraft} {
		if _, err := changecontract.ParsePreflightDraft(msg); err != nil {
			t.Fatalf("fixture %s must parse: %v", name, err)
		}
	}
	if _, err := changecontract.ParsePreflightDraft("Verdict recorded: approved."); err == nil {
		t.Fatal("verdict prose must NOT parse as a draft")
	}
	if !strings.HasPrefix(bug559DraftTask24, "{") {
		t.Fatal("fixture shape")
	}
}
