package runner

import (
	"context"
	"testing"
	"time"
)

// ---- R15-P0: stall-retry gen durable round-trip ----------------------------

func TestPendingRestartGenRoundTripLocalStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess := ProviderSessionState{
		RunID:                "p1",
		ProjectID:            "proj",
		ProviderKey:          ProviderKeyCodex,
		Status:               RunStatusRunning,
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339Nano),
		PendingRestartRunID:  "c1",
		PendingRestartPrompt: "retry",
		PendingRestartGen:    7,
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetProviderSession(context.Background(), "p1")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.PendingRestartGen != 7 {
		t.Fatalf("gen=%d want 7", got.PendingRestartGen)
	}
}

// ---- R15-P0: member retry stamps gen before cancel -------------------------

func TestMemberActionRetryStampsRestartGen(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "reviewer_security"
	svc.runs[child.RunID].turnInFlight = true
	svc.runs[child.RunID].turnCancel = func() {}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "member_stalled", ActiveNode: "reviewer_security", Cap: 3, Mode: "explicit",
	})

	_, handled, aerr := svc.handleMemberAction(parent.RunID, MemberAction{Action: "retry", Node: "reviewer_security"})
	if aerr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, aerr)
	}
	svc.mu.Lock()
	gen := svc.runs[parent.RunID].pendingRestartGen
	svc.mu.Unlock()
	if gen < 1 {
		t.Fatalf("pendingRestartGen=%d, want >=1", gen)
	}
	reader := svc.persistenceStore().(SessionHistoryReader)
	sess, found, _ := reader.GetProviderSession(context.Background(), parent.RunID)
	if !found || sess.PendingRestartGen < 1 {
		t.Fatalf("durable gen missing found=%v gen=%d", found, sess.PendingRestartGen)
	}
}

// ---- R15-P1: second InitRunMarkerSecretFromDir must not clobber -----------

func TestMarkerSecretInitOnceDoesNotClobber(t *testing.T) {
	// Process may already have Once fired from other tests; still verify that
	// a second directory init does not change the active MAC.
	d1 := t.TempDir()
	d2 := t.TempDir()
	InitRunMarkerSecretFromDir(d1)
	mac1 := runMarkerMAC("fcp", "run-stable")
	InitRunMarkerSecretFromDir(d2)
	mac2 := runMarkerMAC("fcp", "run-stable")
	if mac1 != mac2 {
		t.Fatalf("second Init clobbered secret: %q vs %q", mac1, mac2)
	}
}

// ---- R15-P1: gate settle checkpoint fail stamps blocked --------------------

func TestMarkPendingFlowGateSettlePersistFailureStampsBlocked(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.workflowStore = &alwaysFailingUpsertStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.currentTurnID = "turn-x"
	svc.markPendingFlowGateSettleLocked(rs, "done", time.Now().UTC().Format(time.RFC3339Nano))
	if !rs.pendingFlowGateSettle {
		t.Fatal("RAM settle must remain true so Completed is not fan-out")
	}
	if rs.intentBlockedKind != "gate_settle_checkpoint" {
		t.Fatalf("intentBlockedKind=%q, want gate_settle_checkpoint", rs.intentBlockedKind)
	}
	svc.mu.Unlock()
}
