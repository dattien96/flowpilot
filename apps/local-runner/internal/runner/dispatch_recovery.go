package runner

import (
	"context"
	"log"
	"time"
)

// RecoveryScanner reconciles non-terminal DispatchRecords under a lease (SD-24 §6.4 / Task-250).
type RecoveryScanner struct {
	Store  DispatchStore
	Owner  string
	Lease  time.Duration
}

// ScanRun claims and classifies every recoverable record for runID.
func (sc *RecoveryScanner) ScanRun(ctx context.Context, runID string) error {
	if sc == nil || sc.Store == nil {
		return nil
	}
	if sc.Owner == "" {
		sc.Owner = "recovery-scanner"
	}
	if sc.Lease <= 0 {
		sc.Lease = 30 * time.Second
	}
	list, err := sc.Store.ListRecoverable(ctx, runID)
	if err != nil {
		return err
	}
	for _, rec := range list {
		if err := sc.reconcileOne(ctx, rec); err != nil {
			log.Printf("[dispatch-recovery] run=%s turn=%s: %v", rec.RunID, rec.TurnID, err)
		}
	}
	return nil
}

func (sc *RecoveryScanner) reconcileOne(ctx context.Context, rec DispatchRecord) error {
	// Terminal with unfinalized settle → settle driver owns it (Task-251).
	if rec.State.IsTerminal() {
		return nil
	}
	rev, err := sc.Store.ClaimRecovery(ctx, rec.RunID, rec.TurnID, rec.Revision, sc.Owner, sc.Lease)
	if err != nil {
		return nil // concurrent claim — skip
	}
	// Re-read under lease.
	cur, rev, err := sc.Store.Get(ctx, rec.RunID, rec.TurnID)
	if err != nil {
		return err
	}
	_ = rev
	switch cur.State {
	case DispatchPrepared, DispatchSendClaimed:
		// Safely-retryable: live path will redispatch with same TurnID+envelope.
		// If cancel requested, pre-send cancel.
		if cur.CancelRequested {
			_, err = sc.Store.CommitRecoveryPreSendCancellationAndClearIntent(ctx, cur.RunID, cur.TurnID, cur.Revision, sc.Owner,
				cur.IntentOwnerRunID, cur.OuterIntentKey, cur.OuterIntentGen, cur.StopGeneration, PreSendStopSelf)
			return err
		}
		return nil
	case DispatchSendStarted, DispatchProviderAccepted:
		// No provider reconcile proof for Codex/Grok ⇒ uncertain or cancel-required.
		decision, _, err := sc.Store.CommitRecoveryUnknownOrRequireCancel(ctx, cur.RunID, cur.TurnID, cur.Revision, sc.Owner)
		if err != nil {
			return err
		}
		if decision == RecoveryCancelRequired {
			// Provider cancel would go here; without attach capability, pre-send is not legal.
			// Hold for operator via re-attempt after cancel.
			log.Printf("[dispatch-recovery] cancel required run=%s turn=%s", cur.RunID, cur.TurnID)
		}
		return nil
	case DispatchUncertain:
		// Surface only — operator resolution (Task-256).
		return nil
	default:
		return nil
	}
}

// ScanAllRecoverable walks attention items for uncertain/repair (boot recovery).
func (sc *RecoveryScanner) ScanAllRecoverable(ctx context.Context) error {
	if sc == nil || sc.Store == nil {
		return nil
	}
	items, err := sc.Store.ListAttention(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.Kind != "uncertain" || it.RunID == "" || seen[it.RunID] {
			continue
		}
		seen[it.RunID] = true
		_ = sc.ScanRun(ctx, it.RunID)
	}
	return nil
}
