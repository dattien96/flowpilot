package runner

import (
	"context"
	"time"
)

// supabaseDispatchStore implements DispatchStore with the same semantics as the
// memory store. Production wiring routes mutations through transactional RPCs
// (dispatch_cas_advance, dispatch_commit_receipt_clear_intent, ...). Until those
// migrations land, the in-process projection preserves contract parity for the
// shared suite (ledger PAR); real-PG tests can swap a DSN backend later.
type supabaseDispatchStore struct {
	*memoryDispatchStore
}

// NewSupabaseDispatchStore returns a DispatchStore using in-process semantics.
func NewSupabaseDispatchStore() *supabaseDispatchStore {
	return &supabaseDispatchStore{memoryDispatchStore: newMemoryDispatchStore()}
}

// NewMemoryDispatchStore exposes the memory backend for tests and fakes.
func NewMemoryDispatchStore() DispatchStore {
	return newMemoryDispatchStore()
}

// SetDispatchStoreNow overrides the store clock (lease/attach TTL tests).
func SetDispatchStoreNow(store DispatchStore, now func() time.Time) {
	switch s := store.(type) {
	case *memoryDispatchStore:
		s.now = now
	case *localDispatchStore:
		s.now = now
	case *supabaseDispatchStore:
		s.now = now
	}
}

// applyIntentClearFilter drops cleared Pending* fields from a session snapshot
// before write (SD-24 §6.2 intent clears win on write).
func applyIntentClearFilter(ctx context.Context, store DispatchStore, session *ProviderSessionState) {
	if store == nil || session == nil {
		return
	}
	if session.PendingResumePrompt != "" && session.PendingResumeGen > 0 {
		key := "durable-" + session.RunID + "-resume-" + itoa64(session.PendingResumeGen)
		if ok, _ := store.IsIntentCleared(ctx, session.RunID, key, session.PendingResumeGen); ok {
			session.PendingResumePrompt = ""
			session.PendingResumeStepID = ""
		}
	}
	if session.PendingGateRepromptPrompt != "" && session.PendingGateRepromptGen > 0 {
		key := "durable-" + session.RunID + "-reprompt-" + itoa64(session.PendingGateRepromptGen)
		if ok, _ := store.IsIntentCleared(ctx, session.RunID, key, session.PendingGateRepromptGen); ok {
			session.PendingGateRepromptPrompt = ""
			session.PendingGateRepromptStepID = ""
		}
	}
	if session.PendingRestartPrompt != "" && session.PendingRestartGen > 0 {
		key := "durable-" + session.PendingRestartRunID + "-restart-" + itoa64(session.PendingRestartGen)
		if ok, _ := store.IsIntentCleared(ctx, session.RunID, key, session.PendingRestartGen); ok {
			session.PendingRestartPrompt = ""
			session.PendingRestartRunID = ""
		}
	}
}

var _ DispatchStore = (*supabaseDispatchStore)(nil)
