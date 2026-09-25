package runner

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RecoveryScanner reconciles non-terminal DispatchRecords under a lease (SD-24 §6.4 / Task-250).
type RecoveryScanner struct {
	Store DispatchStore
	Owner string
	Lease time.Duration

	// EnsureLiveAndRedispatch, if set, is called for a run whose record was
	// classified safely-retryable (prepared/send_claimed, no cancel pending) —
	// SD-24's "the live path will redispatch with same TurnID+envelope" clause.
	// It must reconstruct the run into RAM if it is not already live, then kick
	// the existing durable-intent relaunch channel (flushDurableTurnIntents),
	// which redrives startTurn with the record's own idempotency key — startTurn
	// itself hash-verifies the envelope and reuses the same TurnID. Nil-safe: if
	// unset, records are still correctly classified but no redispatch action is
	// taken (Codex review 2026-07-17: previously this branch classified only,
	// via a comment promising "live path will redispatch" with nothing wired).
	EnsureLiveAndRedispatch func(ctx context.Context, runID string)
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
	failed := 0
	for _, rec := range list {
		if err := sc.reconcileOne(ctx, rec); err != nil {
			failed++
			log.Printf("[dispatch-recovery] run=%s turn=%s: %v", rec.RunID, rec.TurnID, err)
		}
	}
	if failed > 0 {
		// BUG-484: per-record transient failures mark the pass failed so the
		// boot coordinator retries — lease ownership still gates each retry.
		return fmt.Errorf("%d record(s) failed reconciliation in run %s", failed, runID)
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
		if sc.EnsureLiveAndRedispatch != nil {
			sc.EnsureLiveAndRedispatch(ctx, cur.RunID)
		}
		return nil
	case DispatchSendStarted, DispatchProviderAccepted:
		// No provider reconcile proof for Codex/Grok ⇒ uncertain or cancel-required.
		decision, _, err := sc.Store.CommitRecoveryUnknownOrRequireCancel(ctx, cur.RunID, cur.TurnID, cur.Revision, sc.Owner)
		if err != nil {
			return err
		}
		if decision == RecoveryCancelRequired {
			// BUG-289 A2/F-7: durable marker so ListAttention / operators see
			// stop-then-crash stranded sends (not only log.Printf each boot).
			reason := fmt.Sprintf("cancel_required: send_started/provider_accepted after stop (turn=%s)", cur.TurnID)
			if _, oerr := sc.Store.OpenRepair(ctx, cur.RunID, reason, nil, ""); oerr != nil {
				log.Printf("[dispatch-recovery] open repair for cancel_required run=%s turn=%s: %v", cur.RunID, cur.TurnID, oerr)
			}
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

// ScanAllRecoverable walks every non-terminal record across every run (boot
// recovery). Fixed 2026-07-17 (Codex review): this previously enumerated via
// ListAttention, which only surfaces records already classified DispatchUncertain
// — so prepared/send_claimed/send_started/provider_accepted records (i.e.
// exactly the states a crash leaves most runs in) were invisible to boot
// recovery regardless of whether anything called this method. ListRecoverable
// with an empty runID returns every non-terminal record across every run/project
// on both the memory and local (multi-project) stores.
func (sc *RecoveryScanner) ScanAllRecoverable(ctx context.Context) error {
	if sc == nil || sc.Store == nil {
		return nil
	}
	list, err := sc.Store.ListRecoverable(ctx, "")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	failed := 0
	for _, rec := range list {
		if rec.RunID == "" || seen[rec.RunID] {
			continue
		}
		seen[rec.RunID] = true
		if err := sc.ScanRun(ctx, rec.RunID); err != nil {
			failed++
			log.Printf("[dispatch-recovery] boot scan run=%s: %v", rec.RunID, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d run(s) failed boot recovery scan", failed)
	}
	return nil
}
