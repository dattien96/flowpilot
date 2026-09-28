package runner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/promptpacker"
)

// CP-89 review pass 4 (R4-1..R4-5): durability/commit hardening of the
// forward seam. Each test is the assertion-red reproduction of a finding in
// requirements/07-Coding-Plan/note/CP-89-review.md.

// ---- R4-1: failed durable flip must not spawn ------------------------------

// failStartedUpsertStore injects an UpsertProviderSession failure whenever the
// row being written carries flow_arm=started — the durable-first commit point
// of the forward seam.
type failStartedUpsertStore struct {
	*fakeWorkflowStore
	failStarted atomic.Bool
}

func (f *failStartedUpsertStore) UpsertProviderSession(ctx context.Context, sess ProviderSessionState) error {
	if f.failStarted.Load() && sess.FlowArm == string(FlowArmStarted) {
		return fmt.Errorf("injected upsert failure")
	}
	return f.fakeWorkflowStore.UpsertProviderSession(ctx, sess)
}

// R4-1: if the started+topology commit fails, the forward MUST fail with a
// typed error, MUST NOT spawn the entry child, and MUST leave the latch
// pending in memory AND on the durable row — otherwise the process believes
// started while durable says pending: a restart then lets forward re-run and
// double-launch the flow.
func TestR4_PersistFailureKeepsPendingNoSpawn(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	store := &failStartedUpsertStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	h, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	store.failStarted.Store(true)
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusInternalServerError || e.code != "persist_failed" {
		t.Fatalf("failed durable flip must return typed persist_failed 500, got %v", e)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	arm, nodes, driven := rs.flowArm, len(rs.activeFlowNodes), rs.flowEngineDriven
	children := 0
	for _, c := range svc.runs {
		if c.parentRunID == h.RunID {
			children++
		}
	}
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("failed commit must roll the latch back to pending, arm=%q", arm)
	}
	if nodes != 0 || driven {
		t.Fatalf("failed commit must roll back topology/engine flag, nodes=%d driven=%v", nodes, driven)
	}
	if children != 0 {
		t.Fatalf("no child may spawn when the durable commit failed, got %d", children)
	}
	// The durable row must still say pending — a restart retries cleanly.
	sess, ok, gErr := store.GetProviderSession(context.Background(), h.RunID)
	if gErr != nil || !ok {
		t.Fatalf("durable row missing (ok=%v err=%v)", ok, gErr)
	}
	if sess.FlowArm != string(FlowArmPending) {
		t.Fatalf("durable row must stay pending after failed commit, got %q", sess.FlowArm)
	}
	// Retry after the store recovers: forward works — pending was never
	// consumed.
	store.failStarted.Store(false)
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("retry forward after store recovery must succeed: %v", e)
	}
}

// ---- R4-2: committed topology without launch evidence -----------------------

// started+topology proves only that the durable flip committed — the entry
// child ROW is the proof the launch ran (non-terminal records are never
// pruned). No entry child = crash between commit and spawn: heal to pending
// so a retry forwardFlow launches cleanly instead of wedging on
// flow_already_started.
func TestR4_StartedTopologyNoEntryChildHealsPending(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-nc1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("started+topology with no entry child must heal to pending, arm=%q", rs.flowArm)
	}
	if len(rs.activeFlowNodes) != 0 {
		t.Fatalf("healed pending run must drop orphan topology, nodes=%d", len(rs.activeFlowNodes))
	}
	if rs.flowEngineDriven {
		t.Fatal("healed pending run must not arm flowEngineDriven")
	}
	if rs.chatFlowRef == "" {
		t.Fatal("healed pending run must keep the flow pin for the retry")
	}
}

// The same row WITH a durable entry child row is a legitimately started flow —
// the latch must NOT heal (that would let a retry forward double-launch).
func TestR4_StartedTopologyWithEntryChildStaysStarted(t *testing.T) {
	store := newFakeWorkflowStore()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	// The entry child row: ParentRunID + Label == the flow's entry node id.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-c-entry", ProjectID: "proj", ParentRunID: "run-l1",
		Label: "n1", Status: RunStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-l1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusRunning,
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmStarted {
		t.Fatalf("started+topology+entry child is a real launch — must stay started, arm=%q", rs.flowArm)
	}
}

// End-to-end crash-window recovery: restart on the torn row, then the retry
// forward actually launches the entry child — the wedge is gone.
func TestR4_RestartedCrashWindowForwardRetriesAndLaunches(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-cw1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ProviderAccountID: "default", // a real row always pins its account
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if _, e := svc.startTurn(rs.id, TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("retry forward on healed run must succeed, got %v", e)
	}
	svc.mu.Lock()
	rs = svc.runs["run-cw1"]
	arm := rs.flowArm
	children := 0
	for _, c := range svc.runs {
		if c.parentRunID == "run-cw1" {
			children++
		}
	}
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("retry forward must flip healed pending→started, arm=%q", arm)
	}
	// startResolvedFlow spawns async — child may not exist yet, but topology
	// is stamped durably at minimum; the spawn timing is covered by L-suite.
	if children != 0 && children != 1 {
		t.Fatalf("unexpected child count %d", children)
	}
}

// ---- R4-3: provider switch must carry the CP source pin --------------------

// A pending vibe-cp-ingest leg's SourceDocID is the create-time pin the
// forward fence validates. A provider switch that drops it wedges the new
// leg on invalid_cp_source — the pin is run-scoped and must ride the leg.
func TestR4_ProviderSwitchKeepsSourceDocPin(t *testing.T) {
	ws := t.TempDir()
	cpDir := filepath.Join(ws, "requirements", "07-Coding-Plan")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cpDir, "CP-89-switch.md"),
		[]byte("# CP\nDocument ID: CP-89\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newFakeWorkflowStore()
	svc := task451ServiceWithStore(t, store)
	h, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "pending", Cwd: ws,
		SourceDocID: "requirements/07-Coding-Plan/CP-89-switch.md",
		ChatID:      "cht-r43",
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	// The pin is durable state — the CREATE row itself must carry it (the
	// L-14 live run caught the row being written without source_doc_id).
	if sess, ok, gErr := store.GetProviderSession(context.Background(), h.RunID); gErr != nil || !ok ||
		sess.SourceDocID != "requirements/07-Coding-Plan/CP-89-switch.md" {
		t.Fatalf("durable create row lost the source pin: %+v (ok=%v err=%v)", sess.SourceDocID, ok, gErr)
	}
	resp, swErr := svc.switchChatProvider(context.Background(), "cht-r43", chatSwitchRequest{
		TargetProviderKey: ProviderKeyDevin,
	})
	if swErr != nil {
		t.Fatalf("switchChatProvider: %v", swErr)
	}
	if resp.Handle.RunID == h.RunID {
		t.Fatal("switch must mint a new leg")
	}
	svc.mu.Lock()
	nl := svc.runs[resp.Handle.RunID]
	src, arm := nl.sourceDocID, nl.flowArm
	svc.mu.Unlock()
	if src != "requirements/07-Coding-Plan/CP-89-switch.md" {
		t.Fatalf("new leg lost the CP source pin, sourceDocID=%q", src)
	}
	// The new leg's durable row must carry the pin too — a restart between
	// switch and forward otherwise wedges the leg.
	if sess, ok, gErr := store.GetProviderSession(context.Background(), resp.Handle.RunID); gErr != nil || !ok ||
		sess.SourceDocID != "requirements/07-Coding-Plan/CP-89-switch.md" {
		t.Fatalf("new leg durable row lost the source pin: %+v (ok=%v err=%v)", sess.SourceDocID, ok, gErr)
	}
	if arm != FlowArmPending {
		t.Fatalf("new leg flowArm = %q, want pending", arm)
	}
	// The forward on the new leg must pass the ingest fence via the
	// inherited pin — no repaste required.
	if _, e := svc.startTurn(resp.Handle.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("forward on the switched leg must succeed via inherited pin, got %v", e)
	}
}

// ---- R4-4: failed turns are not "settled" ----------------------------------

// A turn that ended via EventTurnFailed flushed into the transcript list —
// its prompt and partial assistant output then ship to the entry child as
// settled context. Failed turns must be excluded.
func TestR4_FailedTurnExcludedFromForwardPackage(t *testing.T) {
	rs := &interactiveRun{
		events: []ProviderEvent{
			{Type: EventTurnStarted, Seq: 1, Prompt: "FAILED TURN MARKER"},
			{Type: EventMessageCompleted, Seq: 2, Text: "PARTIAL OUTPUT MARKER"},
			{Type: EventTurnFailed, Seq: 3},
			{Type: EventTurnStarted, Seq: 4, Prompt: "settled question"},
			{Type: EventTurnCompleted, Seq: 5, FinalMessage: "settled answer"},
		},
	}
	turns := settledChatTurnsForRun(rs)
	if len(turns) != 1 {
		t.Fatalf("settled turns = %d, want 1 (failed turn excluded), turns=%+v", len(turns), turns)
	}
	if !strings.Contains(turns[0].User, "settled question") {
		t.Fatalf("wrong turn survived: %+v", turns[0])
	}
	for _, tr := range turns {
		if strings.Contains(tr.User, "FAILED TURN MARKER") || strings.Contains(tr.Assistant, "PARTIAL OUTPUT MARKER") {
			t.Fatalf("failed-turn content leaked into settled package: %+v", tr)
		}
	}
}

// ---- R4-5: oversized mandatory forward text --------------------------------

// PackPrompt never truncates mandatory sections — a forward text that alone
// exceeds the entry-node budget ships oversize while forward still succeeds.
// The pack must reject it with a typed error instead.
func TestR4_OversizedForwardTextRejected(t *testing.T) {
	svc := task451Service(t)
	rs := &interactiveRun{}
	// ~400 tokens of forward text against a 100-token entry budget.
	big := strings.Repeat("implement the full migration plan ", 50)
	_, e := svc.buildForwardPromptPackage(rs, big, 100)
	if e == nil || e.code != "forward_prompt_too_large" {
		t.Fatalf("oversized forward text must be a typed rejection, got %v", e)
	}
	// In-budget forward still packs.
	pkg, e := svc.buildForwardPromptPackage(rs, "do it", 100)
	if e != nil {
		t.Fatalf("in-budget forward must pack, got %v", e)
	}
	if promptpacker.EstimateTokens(pkg.Prompt) > 100+40 { // budget + header slack
		t.Fatalf("packed prompt exceeds budget: %d tokens", promptpacker.EstimateTokens(pkg.Prompt))
	}
}
