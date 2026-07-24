package runner

import (
	"context"
	"fmt"
	"testing"
)

// seedOrphanDispatchRecord writes ONE surviving dispatch record for (runID, turnID)
// into ds with the given provider, mirroring a run that was deleted from history
// (its session-index entry + turns file are gone) but whose per-project
// dispatch.ndjson records were never purged. CreatePrepared also materializes the
// non-prunable run activation + stop state for runID in the same commit.
func seedOrphanDispatchRecord(t *testing.T, ds DispatchStore, pk ProviderKey, projectID, runID, turnID string) {
	t.Helper()
	env := DispatchEnvelope{
		TurnID:       turnID,
		RunID:        runID,
		StepID:       "coding",
		ProviderKey:  pk,
		PromptRef:    "prompt-orphan:" + turnID,
		PromptSHA256: HashBytes([]byte("orphan-" + turnID)),
		Model:        "orphan-model",
		CreatedAt:    "2026-07-23T07:11:36Z",
	}
	h, err := ComputeEnvelopeHash(&env)
	if err != nil {
		t.Fatalf("hash orphan envelope: %v", err)
	}
	env.EnvelopeHash = h
	rec := DispatchRecord{
		ProtocolVersion:  DispatchProtocolV2,
		TurnID:           turnID,
		RunID:            runID,
		ProjectID:        projectID,
		IntentOwnerRunID: runID,
		State:            DispatchPrepared,
		EnvelopeHash:     h,
		OuterIntentKey:   "durable-" + runID + "-resume-1",
		OuterIntentGen:   1,
		SettleOwed:       false,
	}
	if err := ds.CreatePrepared(context.Background(), rec, env); err != nil {
		t.Fatalf("seed orphan dispatch record: %v", err)
	}
}

// TestSetDispatchStoreSeedsIDCounterPastOrphanDispatchRecords is the BUG-317
// regression. Deleting a chat from history removes its session-index entry (so
// BUG-117's seedIDCounter no longer counts it) but NOT its per-project
// dispatch.ndjson records. On the next start the id counter re-seeds below the
// orphan's numeric suffix, so nextID re-mints a run/turn id a surviving dispatch
// record still owns. prepareDispatchV2 -> CreatePrepared then rejects the reused
// id with ErrAlreadyExists -> dispatch_prepare_failed (HTTP 502) on the very
// first flow turn (hub dies, 0/N steps). Wiring the durable dispatch store must
// lift the id counter past every persisted dispatch id. Provider-agnostic: the
// orphan record may belong to any provider (the reused run is often a different
// one entirely, e.g. an old grok run's id re-minted for a new claude flow).
func TestSetDispatchStoreSeedsIDCounterPastOrphanDispatchRecords(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyGrok, ProviderKeyClaude, ProviderKeyCodex} {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			chatsRoot := t.TempDir()
			// Real orphan shape from the live repro: run-53892 / turn-53903.
			const orphanSuffix = 53903
			ds := newMultiProjectDispatchStore(chatsRoot)
			defer ds.Close()
			runID := fmt.Sprintf("run-%d", orphanSuffix-11)
			turnID := fmt.Sprintf("turn-%d", orphanSuffix)
			seedOrphanDispatchRecord(t, ds, pk, "proj-orphan", runID, turnID)

			// Session index lists only a LOW run (the high one was deleted), so
			// BUG-117's session-index seed lands well below the orphan suffix.
			sessions := newFakeWorkflowStore()
			if err := sessions.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID: "run-9", ProviderKey: pk, Status: RunStatusCompleted, RunKind: "chat",
			}); err != nil {
				t.Fatalf("seed session index: %v", err)
			}
			svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), sessions)

			// Precondition: the counter is seeded low (from the session index only),
			// so without the dispatch-store seed a re-mint into the orphan range is
			// possible. If this ever fails the test can no longer prove the fix.
			if got := numericIDSuffix(svc.nextID("probe")); got > orphanSuffix {
				t.Fatalf("precondition broken: counter already above orphan (%d > %d)", got, orphanSuffix)
			}

			svc.SetDispatchStore(ds)

			// The fix: every id minted after the durable dispatch store is wired
			// clears the orphan suffix, so no re-mint + CreatePrepared can collide.
			if got := numericIDSuffix(svc.nextID("turn")); got <= orphanSuffix {
				t.Fatalf("nextID after SetDispatchStore = suffix %d, want > %d "+
					"(orphan dispatch id re-minted -> CreatePrepared ErrAlreadyExists -> dispatch_prepare_failed)",
					got, orphanSuffix)
			}
		})
	}
}

// TestMaxPersistedIDSuffixReflectsPersistedDispatchRecords covers the store method
// BUG-317's seed depends on, including the production boot path: records written by
// one store instance and reloaded by a fresh newMultiProjectDispatchStore (exactly
// what happens on every runner restart) must still report the max id suffix.
func TestMaxPersistedIDSuffixReflectsPersistedDispatchRecords(t *testing.T) {
	ctx := context.Background()

	t.Run("empty store is zero", func(t *testing.T) {
		got, err := NewMemoryDispatchStore().(dispatchIDCeiling).MaxPersistedIDSuffix(ctx)
		if err != nil || got != 0 {
			t.Fatalf("empty store MaxPersistedIDSuffix = %d, err=%v; want 0, nil", got, err)
		}
	})

	t.Run("memory store max over both run and turn ids", func(t *testing.T) {
		ds := newMemoryDispatchStore()
		seedOrphanDispatchRecord(t, ds, ProviderKeyClaude, "p", "run-30", "turn-88")
		seedOrphanDispatchRecord(t, ds, ProviderKeyClaude, "p", "run-120", "turn-45")
		got, err := ds.MaxPersistedIDSuffix(ctx)
		if err != nil || got != 120 {
			t.Fatalf("MaxPersistedIDSuffix = %d, err=%v; want 120 (max of run-120 and turn-88)", got, err)
		}
	})

	t.Run("multi-project reload from disk", func(t *testing.T) {
		chatsRoot := t.TempDir()
		writer := newMultiProjectDispatchStore(chatsRoot)
		seedOrphanDispatchRecord(t, writer, ProviderKeyGrok, "proj-a", "run-500", "turn-777")
		seedOrphanDispatchRecord(t, writer, ProviderKeyCodex, "proj-b", "run-1234", "turn-90")
		if err := writer.Close(); err != nil {
			t.Fatalf("close writer: %v", err)
		}
		// A fresh instance loads every shard from disk, exactly like a runner restart.
		reloaded := newMultiProjectDispatchStore(chatsRoot)
		defer reloaded.Close()
		got, err := reloaded.MaxPersistedIDSuffix(ctx)
		if err != nil || got != 1234 {
			t.Fatalf("reloaded MaxPersistedIDSuffix = %d, err=%v; want 1234 (max across both project shards)", got, err)
		}
	})
}
