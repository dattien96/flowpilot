package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Live run-…/vibe-tasks on devin/swe-2-high: run-1's post-lock turns are pure
// orchestration (join sprint-child results, drive the chain) — they produce
// zero file deltas by design, so zero_delta_progress pins the drift score at
// 100 on every turn. applyVibeDriftOnlyResolver then mounted a fresh
// vibe-owner-debate on EVERY clean-gate turn: the debate stashed the sprint
// topology, verdicted reprompt, restored — and the very next zero-delta turn
// re-mounted before the reprompted coder could run. Sprint-1 never
// terminated, so the 5-task plan never advanced past Task-21.
//
// CA-1063 fixed only the in-debate window (suppression while
// vibeParkedNodes/chatFlowRef say the debate is active). This test pins the
// missing post-resolve half: once a drift-only debate has mounted for an
// accumulated debt, re-mounting requires the score to DECAY below the
// threshold and climb back — fresh drift evidence. A pinned-high score must
// not re-mount.
func TestVibeDriftOnlyResolver_DischargesUntilScoreRequalifies(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis"}}
	svc.mu.Unlock()

	st := driftStateFor(svc, parent.RunID)
	st.mu.Lock()
	st.lastScore = 100
	st.mu.Unlock()

	// First high-drift clean-gate turn mounts the debate.
	if !svc.applyVibeDriftOnlyResolver(parent.RunID, "", rs) {
		t.Fatal("first drift-only evaluation at score 100 must mount the owner debate")
	}

	// The debate resolves: the sprint topology is restored (parked nodes
	// cleared, chat flow ref back to the sprint) — CA-1063's inDebate guard
	// no longer applies.
	svc.mu.Lock()
	rs.vibeParkedNodes = nil
	rs.vibeParkedEdges = nil
	rs.vibeParkedAcceptance = nil
	rs.vibeParkedFlowRef = ""
	rs.vibeParkedGatedRunIDs = nil
	rs.chatFlowRef = ""
	svc.mu.Unlock()

	// Same pinned score, next clean-gate turn: the accumulated debt was
	// already escalated — re-mounting re-parks the sprint forever.
	if svc.applyVibeDriftOnlyResolver(parent.RunID, "", rs) {
		t.Fatal("drift-only debate must not re-mount while the score stays pinned — debt discharged, re-arm on decay+re-climb only")
	}
	if svc.applyVibeDriftOnlyResolver(parent.RunID, "", rs) {
		t.Fatal("drift-only debate must stay suppressed across repeated pinned-score turns")
	}

	// Decay: a clean turn with a file delta halves the carried score below
	// the threshold — the escalation re-arms for genuinely new drift.
	st.mu.Lock()
	st.lastScore = 50
	st.mu.Unlock()
	if svc.applyVibeDriftOnlyResolver(parent.RunID, "", rs) {
		t.Fatal("score below threshold must not mount")
	}

	// Re-qualify: fresh drift climbs back over the threshold → mounts again.
	st.mu.Lock()
	st.lastScore = 90
	st.mu.Unlock()
	if !svc.applyVibeDriftOnlyResolver(parent.RunID, "", rs) {
		t.Fatal("a score that decayed then re-climbed over the threshold must mount a fresh debate")
	}
}

// Companion edge: a pinned-score child turn (e.g. a sprint child's own gated
// turn) discharges on the CHILD's drift state — the suppression is per
// drifted run, not per hub.
func TestVibeDriftOnlyResolver_ChildDischargeIsPerGatedRun(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	childID := "run-vibe-drift-child"
	childRs := &interactiveRun{id: childID, parentRunID: parent.RunID, workingMode: workingmode.Vibe}
	svc.mu.Lock()
	svc.runs[childID] = childRs
	svc.mu.Unlock()

	cst := driftStateFor(svc, childID)
	cst.mu.Lock()
	cst.lastScore = 100
	cst.mu.Unlock()

	if !svc.applyVibeDriftOnlyResolver(childID, parent.RunID, childRs) {
		t.Fatal("first child drift-only evaluation must mount")
	}
	if svc.applyVibeDriftOnlyResolver(childID, parent.RunID, childRs) {
		t.Fatal("child drift-only debate must discharge after first mount")
	}
}
