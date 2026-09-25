package runner

// BUG-488: resolveChatIdentity's detached-reattach path consulted
// ListProviderSessionsByChat but swallowed its error — on a store fault the
// max legSeq was derived from memory alone and the new leg could mint a
// legSeq that already exists on disk. Contract: an unreadable legSeq
// authority must fail the run start, never mint a colliding leg.

import (
	"context"
	"errors"
	"testing"
)

// bug488ErrStore wraps the fake store so the chat-reader path fails —
// the BUG-485-era "partial durable load surfaces as error" condition.
type bug488ErrStore struct {
	*fakeWorkflowStore
}

func (s *bug488ErrStore) ListProviderSessionsByChat(context.Context, string) ([]ProviderSessionState, error) {
	return nil, errors.New("sessions shard unreadable")
}

func TestBUG488_ReattachStoreErrorFailsIdentityResolution(t *testing.T) {
	fws := newFakeWorkflowStore()
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-9", RunKind: "chat", ChatID: "cht_x", LegSeq: 2})
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug488ErrStore{fws}}

	_, _, _, err := svc.resolveChatIdentity(StartRunInput{ChatID: "cht_x"})
	if err == nil {
		t.Fatal("store error must fail identity resolution — a minted legSeq is unprovably unique")
	}
}

func TestBUG488_ReattachHealthyStoreKeepsPersistedMax(t *testing.T) {
	fws := newFakeWorkflowStore()
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-9", RunKind: "chat", ChatID: "cht_x", LegSeq: 2})
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}

	chatID, legSeq, _, err := svc.resolveChatIdentity(StartRunInput{ChatID: "cht_x"})
	if err != nil {
		t.Fatalf("healthy store must not error: %v", err)
	}
	if chatID != "cht_x" || legSeq != 3 {
		t.Fatalf("reattach = (%q,%d), want (cht_x,3)", chatID, legSeq)
	}
}

func TestBUG488_ExplicitLegSeqBypassesStore(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug488ErrStore{newFakeWorkflowStore()}}
	// Caller-computed legSeq (Task-314 phase-A) is trusted as-is — no store
	// read happens, so a store fault cannot block it.
	chatID, legSeq, _, err := svc.resolveChatIdentity(StartRunInput{ChatID: "cht_x", LegSeq: 4})
	if err != nil || chatID != "cht_x" || legSeq != 4 {
		t.Fatalf("explicit path = (%q,%d,%v), want (cht_x,4,nil)", chatID, legSeq, err)
	}
}
