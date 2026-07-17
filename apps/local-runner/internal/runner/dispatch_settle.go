package runner

import (
	"context"
	"fmt"
	"log"
	"time"
)

// SettleDriver executes the gate settlement sub-lifecycle (SD-24 §5.3 / Task-251).
// Doctrine: run convergent durable effects FIRST, then CAS-advance SettlePhase.
type SettleDriver struct {
	Store DispatchStore
	// Optional hooks for the live gate path (wired by InteractiveService).
	EvaluateGate func(ctx context.Context, runID, turnID string) (allow bool, reprompt bool, err error)
	// OnFinalized is called after settle reaches finalized (notify waiters, etc.).
	OnFinalized func(ctx context.Context, runID, turnID string)
}

// DriveSettle advances one turn's settle phases until finalized/superseded or error.
func (d *SettleDriver) DriveSettle(ctx context.Context, runID, turnID string) error {
	if d == nil || d.Store == nil {
		return nil
	}
	for {
		rec, rev, err := d.Store.Get(ctx, runID, turnID)
		if err != nil {
			return err
		}
		if !rec.State.IsTerminal() {
			return nil
		}
		if !rec.SettleOwed || rec.SettlePhase.IsSettleFinal() {
			return nil
		}
		next, effects, err := d.planNext(ctx, rec)
		if err != nil {
			return err
		}
		// Effects first (convergent).
		for _, eff := range effects {
			if _, err := d.Store.RecordEffectDone(ctx, runID, turnID, eff.kind, eff.payload, HashBytes(eff.payload)); err != nil {
				return err
			}
		}
		// Then CAS phase.
		if _, err := d.Store.CASAdvanceSettle(ctx, runID, turnID, rev, rec.SettlePhase, next); err != nil {
			return err
		}
		if next == SettleFinalized || next == SettleSupersededReprompt {
			if d.OnFinalized != nil && next == SettleFinalized {
				d.OnFinalized(ctx, runID, turnID)
			}
			return nil
		}
	}
}

type settleEffect struct {
	kind    string
	payload []byte
}

func (d *SettleDriver) planNext(ctx context.Context, rec DispatchRecord) (SettlePhase, []settleEffect, error) {
	switch rec.SettlePhase {
	case SettlePending, SettleNone:
		// Fail-closed (BUG-289 A3 residual): never default-allow when
		// EvaluateGate is missing. A nil hook previously made every
		// terminal+settle_pending look like a green gate and finalized
		// without resumePendingFlowGate — wrong for reprompt/block paths.
		// Unit tests that only exercise the phase machine must pass a stub
		// EvaluateGate that returns (true, false, nil). Production must wire
		// real gate re-eval (Task-251 T-1).
		if d.EvaluateGate == nil {
			return "", nil, fmt.Errorf("settle: EvaluateGate required at phase %s (refuse default-allow)", rec.SettlePhase)
		}
		allow, reprompt, err := d.EvaluateGate(ctx, rec.RunID, rec.TurnID)
		if err != nil {
			return "", nil, err
		}
		payload := []byte(fmt.Sprintf(`{"allow":%v,"reprompt":%v}`, allow, reprompt))
		if reprompt || !allow {
			return SettleSupersededReprompt, []settleEffect{{kind: "gate_eval", payload: payload}}, nil
		}
		return SettleGateEvaluated, []settleEffect{{kind: "gate_eval", payload: payload}}, nil
	case SettleGateEvaluated:
		return SettleCompletionCommitted, []settleEffect{{kind: "completion_event", payload: []byte(`{"ok":true}`)}}, nil
	case SettleCompletionCommitted:
		return SettleGraphSettled, []settleEffect{{kind: "graph_signal", payload: []byte(`{"ok":true}`)}}, nil
	case SettleGraphSettled:
		// dependents_released: release manifest items already created/suppressed by stop.
		return SettleDependentsReleased, []settleEffect{{kind: "dependents_release", payload: []byte(`{"ok":true}`)}}, nil
	case SettleDependentsReleased:
		return SettleFinalized, []settleEffect{{kind: "finalizer", payload: []byte(`{"ok":true}`)}}, nil
	default:
		return "", nil, fmt.Errorf("unknown settle phase %q", rec.SettlePhase)
	}
}

// RetrySettleWithBackoff retries DriveSettle with exponential backoff (gate checkpoint outage).
func (d *SettleDriver) RetrySettleWithBackoff(ctx context.Context, runID, turnID string, attempts int) error {
	if attempts <= 0 {
		attempts = 5
	}
	var last error
	delay := 50 * time.Millisecond
	for i := 0; i < attempts; i++ {
		if err := d.DriveSettle(ctx, runID, turnID); err == nil {
			return nil
		} else {
			last = err
			log.Printf("[settle] attempt %d run=%s turn=%s: %v", i+1, runID, turnID, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay *= 2
		}
	}
	return last
}
