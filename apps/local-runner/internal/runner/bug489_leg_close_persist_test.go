package runner

// BUG-489: switchChatProvider Phase C dropped the leg-close
// persistProviderSession error (`_ =`) — a failed upsert left the old leg
// `active` on disk while memory said closed, so a restart resurrected
// dual-active legs for one chat. Contract: the failure must be retried once
// and, on final failure, surfaced via the SD26-X-6 degraded flag + a log —
// never invisible.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// bug489Store fails the first failN UpsertProviderSession calls.
type bug489Store struct {
	*fakeWorkflowStore
	failN  int32
	upsert int32
}

func (s *bug489Store) UpsertProviderSession(ctx context.Context, sess ProviderSessionState) error {
	n := atomic.AddInt32(&s.upsert, 1)
	if int(n) <= int(atomic.LoadInt32(&s.failN)) {
		return errors.New("upsert failed: disk busy")
	}
	return s.fakeWorkflowStore.UpsertProviderSession(ctx, sess)
}

func TestBUG489_LegClosePersistFailureMarksDegraded(t *testing.T) {
	store := &bug489Store{fakeWorkflowStore: newFakeWorkflowStore(), failN: 8}
	svc := &InteractiveService{
		workflowStore: store,
		chatRuns:      newChatRunRegistry(),
	}
	src := &interactiveRun{id: "run-src", runKind: "chat", chatID: "cht_1", legSeq: 0, legState: LegStateActive}
	svc.chatRuns.register(src.id, src.chatID)

	svc.persistSwitchLegCloseLocked(src)

	if !svc.chatRuns.isDegraded("run-src") {
		t.Fatal("final persist failure must mark the leg degraded — silent drop resurrects the leg on restart")
	}
	if got := atomic.LoadInt32(&store.upsert); got < 2 {
		t.Fatalf("expected at least one retry, got %d upsert attempt(s)", got)
	}
}

func TestBUG489_LegCloseTransientFailureRecovers(t *testing.T) {
	store := &bug489Store{fakeWorkflowStore: newFakeWorkflowStore(), failN: 1}
	svc := &InteractiveService{workflowStore: store, chatRuns: newChatRunRegistry()}
	src := &interactiveRun{id: "run-src", runKind: "chat", chatID: "cht_1", legSeq: 0, legState: LegStateClosed}
	svc.chatRuns.register(src.id, src.chatID)

	svc.persistSwitchLegCloseLocked(src)

	if svc.chatRuns.isDegraded("run-src") {
		t.Fatal("transient failure recovered by retry must not stay degraded")
	}
	got, _, _ := store.fakeWorkflowStore.GetProviderSession(context.Background(), "run-src")
	if got.LegState != LegStateClosed {
		t.Fatalf("persisted legState=%q, want closed", got.LegState)
	}
}

func TestBUG489_LegCloseHealthyStoreClean(t *testing.T) {
	store := &bug489Store{fakeWorkflowStore: newFakeWorkflowStore()}
	svc := &InteractiveService{workflowStore: store, chatRuns: newChatRunRegistry()}
	src := &interactiveRun{id: "run-src", runKind: "chat", chatID: "cht_1", legSeq: 0, legState: LegStateClosed}
	svc.chatRuns.register(src.id, src.chatID)

	svc.persistSwitchLegCloseLocked(src)

	if svc.chatRuns.isDegraded("run-src") {
		t.Fatal("healthy persist must not degrade")
	}
}
