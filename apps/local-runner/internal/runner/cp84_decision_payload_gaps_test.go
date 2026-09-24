package runner

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// CP-84 / Task-430 gap-fill: per-kind payload coverage named in
// CP-84-Test-Steps §2 that had no test — gate bounded context, worktree merge
// durable revision + conflict evidence, question native options, dispatch
// actionable mapping, terminal-but-actionable lane retention, and the
// reconnect-after-terminal-remove reconcile (subscribe-side). Additive file.
// ============================================================================

// pendingQuestionRec mirrors pendingApprovalRec for the question surface.
func pendingQuestionRec(id, runID string) *questionRecord {
	return &questionRecord{
		id:        id,
		runID:     runID,
		prompt:    "Which deploy target?",
		multiSelect: true,
		status:    "pending",
		revision:  3,
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		options: []QuestionOption{
			{Label: "Staging", Description: "safe", Value: "staging"},
			{Label: "Prod", Description: "careful", Value: "prod"},
		},
		resolve: make(chan questionResolveResult, 1),
	}
}

// TestDecisionPayloads_GateCarriesBoundedContext: the parked flow-gate surface
// (rs.pendingGateBlock) must project a gate decision whose regressedTests are
// bounded (decisionListMaxItems items, each ≤decisionFieldMaxLen) and whose
// options survive — the inbox renders Fix/Suggest/Open from these.
func TestDecisionPayloads_GateCarriesBoundedContext(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-gate", "proj-1")
	regressed := make([]string, decisionListMaxItems+10)
	for i := range regressed {
		regressed[i] = "TestRegression_" + strings.Repeat("x", 300)
	}
	rs.pendingGateBlock = &gateBlockInfo{
		regressedTests: regressed,
		stepID:         "step-impl",
		message:        "Regression gate failed — " + strings.Repeat("m", 2000),
		options:        []string{"fix", "suggest", "open"},
		createdAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	if len(got) != 1 || got[0].Kind != DecisionKindGate {
		t.Fatalf("expected 1 gate decision, got %+v", got)
	}
	d := got[0]
	if d.ID != "gate:run-430-gate" || !d.Actionable || d.Gate == nil {
		t.Fatalf("gate decision malformed: %+v", d)
	}
	if len(d.Gate.RegressedTests) != decisionListMaxItems {
		t.Fatalf("regressedTests = %d, want capped %d", len(d.Gate.RegressedTests), decisionListMaxItems)
	}
	for _, rt := range d.Gate.RegressedTests {
		if len(rt) > decisionFieldMaxLen+4 { // boundDecisionText appends "…" (3 bytes UTF-8)
			t.Fatalf("regressed test not bounded: len=%d", len(rt))
		}
	}
	if len(d.Gate.Options) != 3 || d.Gate.StepID != "step-impl" {
		t.Fatalf("gate options/stepId lost: %+v", d.Gate)
	}
	if len(d.Prompt) > decisionPromptMaxLen+4 {
		t.Fatalf("prompt not bounded: len=%d", len(d.Prompt))
	}
}

// TestDecisionPayloads_VibeGateSurfacesProject: the two vibe user.confirm
// locks (sprint-boundary, resume-confirm) must project gate decisions so a
// parked vibe run surfaces in the inbox without opening the run.
func TestDecisionPayloads_VibeGateSurfacesProject(t *testing.T) {
	svc := NewInteractiveService()

	rsB := mkDecisionRun(svc, "run-430-vb", "proj-1")
	rsB.vibeSprintBoundaryPending = true
	rsB.vibeSprintIndex = 1
	rsB.vibeTaskPlan = []string{"t1", "t2", "t3"}
	rsB.vibeSprintBoundaryTask = "t2"

	rsR := mkDecisionRun(svc, "run-430-vr", "proj-1")
	rsR.vibeResumeConfirm = true
	rsR.vibeResumeFromNode = "node-impl-2"

	svc.mu.Lock()
	gotB := svc.decisionPayloadsForRunLocked(rsB)
	gotR := svc.decisionPayloadsForRunLocked(rsR)
	svc.mu.Unlock()

	if len(gotB) != 1 || gotB[0].Kind != DecisionKindGate || gotB[0].ID != "vibe-gate-boundary:run-430-vb" {
		t.Fatalf("sprint-boundary decision malformed: %+v", gotB)
	}
	if gotB[0].Gate == nil || gotB[0].Gate.ResumeFrom == "" {
		t.Fatalf("sprint-boundary missing resumeFrom: %+v", gotB[0])
	}
	if len(gotR) != 1 || gotR[0].Kind != DecisionKindGate || gotR[0].ID != "vibe-gate-resume:run-430-vr" {
		t.Fatalf("resume-confirm decision malformed: %+v", gotR)
	}
	if gotR[0].Gate == nil || gotR[0].Gate.ResumeFrom != "node-impl-2" {
		t.Fatalf("resume-confirm resumeFrom lost: %+v", gotR[0])
	}
}

// TestDecisionPayloads_QuestionKeepsNativeQuestionOptions: a pending question
// must carry its options + multiSelect into the payload — the inbox renders
// the native option list without a per-run round-trip.
func TestDecisionPayloads_QuestionKeepsNativeQuestionOptions(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-q", "proj-1")
	rec := pendingQuestionRec("q-430-1", rs.id)
	svc.mu.Lock()
	svc.questions[rec.id] = rec
	svc.mu.Unlock()

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	if len(got) != 1 || got[0].Kind != DecisionKindQuestion {
		t.Fatalf("expected 1 question decision, got %+v", got)
	}
	d := got[0]
	if d.ID != "question:q-430-1" || d.Revision != "3" || !d.Actionable {
		t.Fatalf("question decision malformed: %+v", d)
	}
	if d.Question == nil || len(d.Question.Options) != 2 || !d.Question.MultiSelect {
		t.Fatalf("options/multiSelect not preserved: %+v", d.Question)
	}
	if d.Question.Options[1].Value != "prod" {
		t.Fatalf("option values lost: %+v", d.Question.Options)
	}
}

// TestDecisionPayloads_MergeUsesDurableBindingRevision: a merge_pending
// worktree binding must project a worktree_merge decision whose revision is
// derived from the durable binding state (not a volatile counter), carrying
// the conflict evidence from the last merge-requested event.
func TestDecisionPayloads_MergeUsesDurableBindingRevision(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-wt", "proj-1")
	rs.worktree = &worktreeBinding{
		OwnerID:    "chat-1",
		Path:       "C:/repo/.flowpilot/worktrees/chat-1",
		Branch:     "run/chat-1",
		BaseCommit: "abc123",
		State:      "merge_pending",
		Enabled:    true,
	}
	rs.events = append(rs.events, ProviderEvent{
		Type: EventWorktreeMergeRequested,
		Input: map[string]any{
			"conflictPaths":    []string{"src/a.go", "src/b.go"},
			"patchArtifactRef": "patch-ref-9",
		},
	})

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	if len(got) != 1 || got[0].Kind != DecisionKindWorktree {
		t.Fatalf("expected 1 worktree_merge decision, got %+v", got)
	}
	d := got[0]
	if d.ID != "worktree_merge:run-430-wt" || !d.Actionable || d.Worktree == nil {
		t.Fatalf("worktree decision malformed: %+v", d)
	}
	if !strings.HasPrefix(d.Revision, "wt:merge_pending:") {
		t.Fatalf("revision must derive from durable binding state, got %q", d.Revision)
	}
	if len(d.Worktree.ConflictPaths) != 2 || d.Worktree.PatchRef != "patch-ref-9" {
		t.Fatalf("conflict evidence lost: %+v", d.Worktree)
	}
	if d.Worktree.Path == "" || d.Worktree.Branch != "run/chat-1" {
		t.Fatalf("binding fields lost: %+v", d.Worktree)
	}
}

// TestDecisionPayloads_DispatchAttentionActionableMatrix: settle_pending is
// operational (non-actionable) while uncertain/repair_required/cancel_required
// are human decisions; repair items key on the run, turn-scoped kinds on the
// turn id.
func TestDecisionPayloads_DispatchAttentionActionableMatrix(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-d", "proj-1")

	cases := []struct {
		kind        string
		turnID      string
		wantID      string
		actionable  bool
	}{
		{"uncertain", "turn-1", "dispatch:turn-1", true},
		{"cancel_required", "turn-2", "dispatch:turn-2", true},
		{"settle_pending", "turn-3", "dispatch:turn-3", false},
		{"repair_required", "", "dispatch-repair:run-430-d", true},
	}
	for _, c := range cases {
		item := AttentionItem{Kind: c.kind, RunID: rs.id, TurnID: c.turnID, Reason: "r", Revision: 7}
		d := dispatchDecisionFromAttention(rs, item)
		if d.ID != c.wantID || d.Actionable != c.actionable || d.Kind != DecisionKindDispatch {
			t.Fatalf("kind=%q → %+v, want id=%q actionable=%v", c.kind, d, c.wantID, c.actionable)
		}
		if d.Revision != "7" || d.Dispatch == nil || d.Dispatch.AttentionKind != c.kind {
			t.Fatalf("kind=%q payload malformed: %+v", c.kind, d)
		}
	}
}

// TestRunUpdates_TerminalButActionableStaysInSnapshot: a terminal run that
// still carries an actionable decision (merge_pending worktree) must appear in
// a NEW subscriber's snapshot — isLaneRelevant is the only thing keeping the
// decision reachable after terminal.
func TestRunUpdates_TerminalButActionableStaysInSnapshot(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-term-act", "proj-1")
	rs.status = RunStatusCompleted
	rs.worktree = &worktreeBinding{OwnerID: "c1", Path: "/wt", State: "merge_pending", Enabled: true}

	rsDead := mkDecisionRun(svc, "run-430-term-dead", "proj-1")
	rsDead.status = RunStatusCompleted // no decisions → must be excluded

	_, _, snap := svc.subscribeRunUpdates()
	foundActionable, foundDead := false, false
	for _, p := range snap {
		if p.RunID == rs.id {
			foundActionable = true
			if len(p.Decisions) != 1 || p.Decisions[0].Kind != DecisionKindWorktree {
				t.Fatalf("terminal-actionable lane lost its decision: %+v", p)
			}
		}
		if p.RunID == rsDead.id {
			foundDead = true
		}
	}
	if !foundActionable {
		t.Fatal("terminal run with actionable decision missing from snapshot")
	}
	if foundDead {
		t.Fatal("plain terminal run must not appear in snapshot")
	}
}

// TestRunUpdates_ReconnectSnapshotReconcilesMissedTerminalRemove: the named
// spec test — a subscriber that missed a remove (disconnect, sleep) gets a
// snapshot on reconnect that simply omits the terminal lane; the client's
// authoritative reconcile drops it (T-6 reconnect-by-snapshot).
func TestRunUpdates_ReconnectSnapshotReconcilesMissedTerminalRemove(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-reconn", "proj-1")

	// First subscription sees the live lane.
	subID, _, snap := svc.subscribeRunUpdates()
	found := false
	for _, p := range snap {
		if p.RunID == rs.id {
			found = true
		}
	}
	if !found {
		t.Fatal("initial snapshot must contain the live lane")
	}
	svc.unsubscribeRunUpdates(subID) // disconnect — the remove will be missed

	svc.mu.Lock()
	rs.status = RunStatusCompleted
	svc.mu.Unlock()

	// Reconnect: snapshot must silently exclude the terminal lane — that IS
	// the reconcile (no remove frame needed across a reconnect).
	_, _, snap2 := svc.subscribeRunUpdates()
	for _, p := range snap2 {
		if p.RunID == rs.id {
			t.Fatalf("terminal lane resurfaced after reconnect: %+v", p)
		}
	}
}
