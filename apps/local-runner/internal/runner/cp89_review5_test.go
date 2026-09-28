package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/promptpacker"
)

// CP-89 review pass 5 (R5-*): deep audit of the full CP-89 change set
// (e13c5509..aeee1ebc). Each test is the assertion-red reproduction of a
// finding captured in requirements/07-Coding-Plan/note/CP-89-review.md.

// ---- R5-1: a rejected forward must not consume the latch -------------------
//
// forwardPinnedFlow flipped pending→started BEFORE buildForwardPromptPackage
// ran. A pack rejection (forward_prompt_too_large / pack_failed) then left
// the run permanently started with no child — every retry answered
// flow_already_started. A rejected forward must leave the run untouched.
func TestR5_RejectedForwardKeepsLatchPending(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// task-harness entry (preflight_contract_plan) uses the scout profile,
	// maxEstPromptTokens=6000 — this forward text alone exceeds it.
	big := strings.Repeat("x", 6000*4+4096)
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: big}, "", ""); e == nil ||
		e.code != "forward_prompt_too_large" {
		t.Fatalf("oversized forward must reject with forward_prompt_too_large, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("rejected forward must leave the latch pending, arm=%q", arm)
	}
	// A valid retry must still launch — the rejection must not wedge the run.
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "go"}, "", ""); e != nil {
		t.Fatalf("retry forward after rejection must succeed, got %v", e)
	}
	svc.mu.Lock()
	arm = svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("retry forward must flip pending→started, arm=%q", arm)
	}
}

// ---- R5-2: the latch is the forward's idempotency record -------------------
//
// The idempotency replay check runs before the forward seam. A recorded key
// therefore short-circuits even when the earlier attempt never launched the
// flow: (a) a rejected forward already stored idem[key]=turnID for a
// non-durable key, poisoning the key for the session; (b) a durable key on a
// crash row healed back to pending replays the synthetic TurnCompleted and
// returns the old turnID without launching. While flowArm==pending a
// forwardFlow turn must ALWAYS re-enter the launch path (same turnID).
func TestR5_ForwardRetrySameKeyAfterRejectionLaunches(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	big := strings.Repeat("x", 6000*4+4096)
	if _, e := svc.startTurn(h.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true, Prompt: big}, "", "fwd-key-1"); e == nil {
		t.Fatal("oversized forward must reject")
	}
	// Retry the same idempotency key with an in-budget prompt — the client
	// contract is "same key = same intent", and that intent never launched.
	tid, e := svc.startTurn(h.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "go"}, "", "fwd-key-1")
	if e != nil {
		t.Fatalf("retry with the same key must not error, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("same-key retry on a pending latch must launch (replayed tid=%q, arm=%q)", tid, arm)
	}
}

// Post-restart variant: a durable row that committed started+topology but
// crashed before the entry child spawned heals to pending — but the durable
// idempotency key still carries the launched turnID and the synthetic
// TurnCompleted is in the event stream, so the replay path returns it without
// launching. The pending latch must override replay-safety for forward turns.
func TestR5_DurableKeyRetryOnHealedPendingLaunches(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-r5d", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ProviderAccountID: "default",
		ChatFlowRef:       "task-harness",
		FlowArm:           "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
		// The crash-window row: the durable key was already upgraded to the
		// launched value by the synthetic-completion persist.
		IdempotencyKeys: map[string]string{"durable-fwd-9": "turn-9"},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("fixture must heal to pending first (no entry child), arm=%q", rs.flowArm)
	}
	// The replay-safe evidence the crashed process left behind: the synthetic
	// terminal event for the forward turn.
	rs.events = append(rs.events, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: "turn-9"})
	tid, e := svc.startTurn(rs.id, TurnInput{StepID: "chat", ForwardFlow: true}, "", "durable-fwd-9")
	if e != nil {
		t.Fatalf("durable-key retry on healed pending must launch, got %v", e)
	}
	if tid != "turn-9" {
		t.Fatalf("relaunch must reuse the prepared turnID, got %q", tid)
	}
	svc.mu.Lock()
	arm := svc.runs[rs.id].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("healed pending + same durable key must re-enter the launch path, arm=%q", arm)
	}
}

// ---- R5-3: Drive manifest must carry the CP-89 pin fields ------------------
//
// FlowArm/ChatFlowRef/turnCount already round-trip through the Drive manifest
// so a device restore matches a local restart. SourceDocID, WorkingMode and
// ChangeType are equally run-scoped launch state for a pending vibe run —
// without them a restored pending vibe-cp-ingest leg comes back as a dev chat
// with no source pin and the forward fence wedges on invalid_cp_source.
func TestR5_DriveManifestRoundTripsPendingPin(t *testing.T) {
	m := ChatSessionSyncManifest{
		SchemaVersion: chatSessionManifestSchemaVersion,
		SourceRunID:   "run-src",
		ProjectID:     "proj",
		ProviderKey:   ProviderKeyCodex,
		RunKind:       "chat",
		ChatFlowRef:   "flowpilot-core-flow-pack/vibe-cp-ingest",
		FlowArm:       "pending",
		ChatSubMode:   "vibe",
		WorkingMode:   "vibe",
		SourceDocID:   "requirements/07-Coding-Plan/CP-89.md",
		ChangeType:    "feature",
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back ChatSessionSyncManifest
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if back.SourceDocID != m.SourceDocID {
		t.Fatalf("manifest lost sourceDocID: %q", back.SourceDocID)
	}
	if back.WorkingMode != "vibe" {
		t.Fatalf("manifest lost workingMode: %q", back.WorkingMode)
	}
	if back.ChangeType != "feature" {
		t.Fatalf("manifest lost changeType: %q", back.ChangeType)
	}
}

// ---- R5-4: assembled forward prompt stays under the hard cap ---------------
//
// PackPrompt counts only section CONTENT against TotalMaxTokens; the section
// headers/joiners are added on top at assembly. The transcript cap already
// reserves forwardPromptReserveBytes for that framing, so the assembled
// prompt must stay strictly under the byte cap — this pins the invariant.
func TestR5_ForwardPackageAssembledWithinBudget(t *testing.T) {
	svc := task451Service(t)
	rs := &interactiveRun{events: []ProviderEvent{
		{Type: EventTurnStarted, Seq: 1, Prompt: "earlier question"},
		{Type: EventTurnCompleted, Seq: 2, FinalMessage: "earlier answer"},
	}}
	const budget = 200
	// Forward text sized so the ASSEMBLED prompt (headers + sections) still
	// fits the hard cap — the invariant this test pins.
	fwd := strings.Repeat("y", int(budget)*4-256)
	pkg, e := svc.buildForwardPromptPackage(rs, fwd, budget)
	if e != nil {
		t.Fatalf("in-budget forward must pack, got %v", e)
	}
	if int64(promptpacker.EstimateTokens(pkg.Prompt)) > budget {
		t.Fatalf("assembled prompt %d tokens exceeds hard budget %d", promptpacker.EstimateTokens(pkg.Prompt), budget)
	}
	// The over-cap case: a forward text that fits the content budget but not
	// the assembled cap must be rejected, not shipped oversize.
	edge := strings.Repeat("y", int(budget)*4-64)
	if _, e := svc.buildForwardPromptPackage(rs, edge, budget); e == nil ||
		e.code != "forward_prompt_too_large" {
		t.Fatalf("over-cap assembled prompt must reject typed, got %v", e)
	}
}

// ---- R5-5: executor-progress evidence without a child ----------------------
//
// A child row is the only launch commit — the spawn persists it
// synchronously. The executor ALSO writes durable step transitions/step rows
// before the delegate spawn commit (inline-entry DONE, entry RUNNING), so
// transition/step evidence with NO child row is a partially-engaged launch,
// not a running flow. The original version of this test pinned "evidence →
// stays started"; R6-2 review showed that is the wedged outcome (nothing to
// resume + forward blocked on flow_already_started), so the corrected
// contract is: evidence without a child heals to pending so the forward
// retry relaunches. This test pins the step-row variant of the same rule
// (the transition-line variant lives in cp89_review6_test.go).
type r5TransitionStore struct {
	*fakeWorkflowStore
	lines map[string][]stepTransitionLine
}

func (s *r5TransitionStore) AppendStepTransition(_ context.Context, runID string, l stepTransitionLine) error {
	if s.lines == nil {
		s.lines = map[string][]stepTransitionLine{}
	}
	s.lines[runID] = append(s.lines[runID], l)
	return nil
}

func (s *r5TransitionStore) LoadStepTransitions(_ context.Context, runID string) ([]stepTransitionLine, error) {
	return s.lines[runID], nil
}

func (s *r5TransitionStore) DeleteStepTransitions(_ context.Context, runID string) error {
	delete(s.lines, runID)
	return nil
}

func TestR5_StepRowEvidenceWithoutChildHealsPending(t *testing.T) {
	store := &r5TransitionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc := task451ServiceWithStore(t, store)
	// Executor progress the crash left behind: the entry node's step row shows
	// real work (status DONE + StartedAt), but the child row is absent — kill
	// between the inline dispatch and the delegate spawn commit.
	store.seed("run-r5s", []RuntimeWorkflowStep{{
		ID: "n1", Status: StepStatusDone, StartedAt: "2026-01-01T00:00:00Z",
	}})
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-r5s", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted,
		ProviderAccountID: "default",
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("step-row evidence with no child row must heal to pending — staying started wedges with nothing to resume, arm=%q", rs.flowArm)
	}
}
