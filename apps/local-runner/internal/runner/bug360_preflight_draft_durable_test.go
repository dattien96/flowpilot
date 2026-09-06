package runner

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// BUG-360: the scout's parseable preflight draft is cached on the parent at
// child completion and rides the session snapshot + runtime blob, so a
// post-restart freeze parses it after the transient scout child is gone.
// Without the cache, freeze strict-parses prose and escalates in a dead-end
// "no progress" loop (live run-584646). New file; no pre-existing test is
// modified.

const bug360SecondDraft = `{"feature_key":"calc-core","intent":"fix tie rule","declared_paths":["src/calc.go"]}`

// Capture is parse-gated: non-scout prose never overwrites a good draft,
// last parseable wins, and missing parents/children are safe no-ops.
// (Scout-labeled prose clears — covered by ScoutProseClears below.)
func TestCachePreflightDraftLocked_ParseGated(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	svc.runs["parent"] = &interactiveRun{id: "parent"}
	writer := &interactiveRun{id: "w", parentRunID: "parent", label: "plan_writer"}

	svc.cachePreflightDraftLocked(writer, validPlannerDraft)
	if got := svc.runs["parent"].preflightDraftResult; got != validPlannerDraft {
		t.Fatalf("draft = %q, want cached", got)
	}
	// Non-scout prose must not clobber the good draft.
	svc.cachePreflightDraftLocked(writer, "prose summary, not json")
	if got := svc.runs["parent"].preflightDraftResult; got != validPlannerDraft {
		t.Fatalf("prose overwrote draft: %q", got)
	}
	// A newer parseable draft wins.
	svc.cachePreflightDraftLocked(writer, bug360SecondDraft)
	if got := svc.runs["parent"].preflightDraftResult; got != bug360SecondDraft {
		t.Fatalf("draft = %q, want second draft", got)
	}
	// Safe no-ops.
	svc.cachePreflightDraftLocked(nil, validPlannerDraft)
	svc.cachePreflightDraftLocked(&interactiveRun{id: "orphan"}, validPlannerDraft)
	svc.cachePreflightDraftLocked(&interactiveRun{id: "x", parentRunID: "missing"}, validPlannerDraft)
	var nilSvc *InteractiveService
	nilSvc.cachePreflightDraftLocked(writer, validPlannerDraft)
}

// Fallback order: live children first, parent stash second, "" last.
func TestFindPlannerResultForFreeze_StashFallback(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	svc.runs["parent"] = &interactiveRun{id: "parent", preflightDraftResult: validPlannerDraft}
	edges, nodes := task325PlanTopology()

	// No live children: stash answers.
	if got := svc.findPlannerResultForFreeze("parent", edges, nodes, "preflight_contract_freeze"); got != validPlannerDraft {
		t.Fatalf("want stashed draft, got %q", got)
	}
	// Corrupt stash never satisfies the freeze (fail-closed).
	svc.runs["parent"].preflightDraftResult = "prose, not json"
	if got := svc.findPlannerResultForFreeze("parent", edges, nodes, "preflight_contract_freeze"); got != "" {
		t.Fatalf("corrupt stash must yield nothing, got %q", got)
	}
	// Live child beats stash (freshest wins).
	svc.runs["parent"].preflightDraftResult = validPlannerDraft
	svc.runs["scout"] = &interactiveRun{id: "scout", parentRunID: "parent", label: "preflight_contract_plan",
		events: []ProviderEvent{{Type: EventTurnCompleted, FinalMessage: bug360SecondDraft}}}
	if got := svc.findPlannerResultForFreeze("parent", edges, nodes, "preflight_contract_freeze"); got != bug360SecondDraft {
		t.Fatalf("live child must win, got %q", got)
	}
}

// The reported dead end, fixed: prose approval + dead scout child +
// stashed draft → freeze proceeds (no escalate).
func TestBug360FreezeProceedsFromStashWithoutScoutChild(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	svc.mu.Lock()
	delete(svc.runs, "run-planner-325") // post-restart: transient scout gone
	svc.runs[runID].preflightDraftResult = validPlannerDraft
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "prose approved summary"})
	if !handled {
		t.Fatal("must dispatch freeze from stashed draft")
	}
	if res.NextAction == "awaiting_user" {
		t.Fatalf("stashed draft must not park: %+v", res)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
		t.Fatalf("freeze = %v, want DONE", got)
	}
	waitLoop(t, "writer spawned after stash freeze", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "test_signatures") == 1
	})
	if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "escalate" {
		t.Fatalf("must not escalate with a stashed draft: %+v", st)
	}
}

// Fail-closed preserved: prose + no scout + no stash still escalates.
func TestBug360FreezeStillEscalatesWithoutAnyDraft(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	svc.mu.Lock()
	delete(svc.runs, "run-planner-325")
	svc.mu.Unlock()

	svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "prose approved summary"})
	if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason != "escalate" {
		t.Fatalf("loop = %+v, want blocked/escalate (fail-closed)", st)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
		t.Fatal("freeze must not complete without any draft")
	}
}

// Durability round-trip: sessionStateOf → local file store → reconstruct
// restores the cached draft (mirrors the run63960 restart pattern).
func TestBug360DraftSurvivesRestartRoundTrip(t *testing.T) {
	svc := NewInteractiveService()
	svc.mu.Lock()
	svc.runs["run-360"] = &interactiveRun{id: "run-360", preflightDraftResult: validPlannerDraft}
	live := svc.runs["run-360"]
	st := sessionStateOf(live)
	st.LoopState = svc.agentOrchestrator.loopStateFor("run-360")
	svc.mu.Unlock()
	if st.PreflightDraftResult != validPlannerDraft {
		t.Fatalf("snapshot lost the draft: %q", st.PreflightDraftResult)
	}

	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	st.WorkingDirectory = root
	st.ProviderKey = ProviderKeyCodex
	st.ProjectID = "proj"
	if err := store.UpsertProviderSession(context.Background(), st); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	loaded, ok, getErr := store.GetProviderSession(context.Background(), st.RunID)
	if getErr != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, getErr)
	}
	if loaded.PreflightDraftResult != validPlannerDraft {
		t.Fatalf("store round-trip lost the draft: %q", loaded.PreflightDraftResult)
	}
	svc2 := NewInteractiveService()
	rec, apiErr := svc2.reconstructRun(loaded)
	if apiErr != nil {
		t.Fatalf("reconstruct: %s", apiErr.msg)
	}
	if rec.preflightDraftResult != validPlannerDraft {
		t.Fatalf("reconstructed run lost the draft: %q", rec.preflightDraftResult)
	}
	// Blob leg (Supabase path needs no migration): encode + decode preserves it.
	blob, err := json.Marshal(sessionRuntimeFromState(loaded))
	if err != nil {
		t.Fatalf("blob encode: %v", err)
	}
	var back ProviderSessionState
	applySessionRuntime(&back, blob)
	if back.PreflightDraftResult != validPlannerDraft {
		t.Fatalf("blob round-trip lost the draft: %q", back.PreflightDraftResult)
	}
	// Disk leg (review C1): a FRESH store on the same dir must restore it —
	// the same-store Get above only proves the memory hit.
	store2, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("second store: %v", err)
	}
	loaded2, ok, getErr := store2.GetProviderSession(context.Background(), st.RunID)
	if getErr != nil || !ok {
		t.Fatalf("disk get: ok=%v err=%v", ok, getErr)
	}
	if loaded2.PreflightDraftResult != validPlannerDraft {
		t.Fatalf("disk reload lost the draft: %q", loaded2.PreflightDraftResult)
	}
	rec2, apiErr := NewInteractiveService().reconstructRun(loaded2)
	if apiErr != nil {
		t.Fatalf("reconstruct after disk reload: %s", apiErr.msg)
	}
	_ = rec2
}

// A scout-labeled completion with unparseable output clears a stale stash so
// a re-run scout failure fails closed (escalate) instead of freezing on
// outdated scope. Non-scout prose never clears.
func TestCachePreflightDraftLocked_ScoutProseClears(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	svc.runs["parent"] = &interactiveRun{id: "parent", preflightDraftResult: validPlannerDraft}
	scout := &interactiveRun{id: "scout", parentRunID: "parent", label: "preflight_contract_plan"}
	other := &interactiveRun{id: "other", parentRunID: "parent", label: "implement"}

	svc.cachePreflightDraftLocked(other, "prose from coder")
	if got := svc.runs["parent"].preflightDraftResult; got != validPlannerDraft {
		t.Fatalf("non-scout prose must not clear: %q", got)
	}
	svc.cachePreflightDraftLocked(scout, "prose, scout failed")
	if got := svc.runs["parent"].preflightDraftResult; got != "" {
		t.Fatalf("failed scout re-run must clear stale stash, got %q", got)
	}
}

// Settle wiring: a real scout child completing through the central settle
// path caches its draft on the parent (proves the hook, not just the helper).
func TestSettleCachesScoutDraftOnParent(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.mu.Lock()
	svc.runs["scout-360"] = &interactiveRun{
		id: "scout-360", parentRunID: runID, label: "preflight_contract_plan",
		agentName: "contract-planner", status: RunStatusRunning,
		subs: map[int64]chan ProviderEvent{},
	}
	child := svc.runs["scout-360"]
	svc.settleFlowChildTurnCompletedLocked(child, validPlannerDraft, ProviderEvent{Type: EventTurnCompleted})
	got := svc.runs[runID].preflightDraftResult
	svc.mu.Unlock()
	if got != validPlannerDraft {
		t.Fatalf("settle must cache scout draft, got %q", got)
	}
}
