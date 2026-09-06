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

// Capture is parse-gated: prose never overwrites a good draft, last
// parseable wins, and missing parents/children are safe no-ops.
func TestCachePreflightDraftLocked_ParseGated(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	svc.runs["parent"] = &interactiveRun{id: "parent"}
	scout := &interactiveRun{id: "scout", parentRunID: "parent", label: "preflight_contract_plan"}

	svc.cachePreflightDraftLocked(scout, validPlannerDraft)
	if got := svc.runs["parent"].preflightDraftResult; got != validPlannerDraft {
		t.Fatalf("draft = %q, want cached", got)
	}
	// Prose must not clobber the good draft.
	svc.cachePreflightDraftLocked(scout, "prose summary, not json")
	if got := svc.runs["parent"].preflightDraftResult; got != validPlannerDraft {
		t.Fatalf("prose overwrote draft: %q", got)
	}
	// A newer parseable draft wins (scout re-ran).
	svc.cachePreflightDraftLocked(scout, bug360SecondDraft)
	if got := svc.runs["parent"].preflightDraftResult; got != bug360SecondDraft {
		t.Fatalf("draft = %q, want second draft", got)
	}
	// Safe no-ops.
	svc.cachePreflightDraftLocked(nil, validPlannerDraft)
	svc.cachePreflightDraftLocked(&interactiveRun{id: "orphan"}, validPlannerDraft)
	svc.cachePreflightDraftLocked(&interactiveRun{id: "x", parentRunID: "missing"}, validPlannerDraft)
	var nilSvc *InteractiveService
	nilSvc.cachePreflightDraftLocked(scout, validPlannerDraft)
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
}
