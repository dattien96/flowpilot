package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// SettleDriver executes the gate settlement sub-lifecycle (SD-24 §5.3 / Task-251).
// Doctrine: run convergent durable effects FIRST, then CAS-advance SettlePhase.
type SettleDriver struct {
	Store DispatchStore
	// EvaluateGate is required at settle_pending / settle_none (fail-closed).
	// Production wires real gate state; unit tests pass a stub.
	EvaluateGate func(ctx context.Context, runID, turnID string) (allow bool, reprompt bool, err error)
	// Optional effect enrichers (Task-251 T-2). When set, replace default
	// phase payloads with durable projections (cohort / release / finalizer).
	// Default path still writes keyed RecordEffectDone with structured JSON.
	BuildEffect func(ctx context.Context, rec DispatchRecord, next SettlePhase, kind string) (payload []byte, err error)
	// OnFinalized is called after settle reaches finalized.
	OnFinalized func(ctx context.Context, runID, turnID string)
	// Test-only: called after each effect is recorded and after each phase CAS
	// (Task-255 settle sub-barriers B8a..B8e). Nil in production.
	testBarrierAfterEffect func(phase SettlePhase, kind string)
	testBarrierAfterCAS    func(from, to SettlePhase)
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
			payload := eff.payload
			if d.BuildEffect != nil {
				p, berr := d.BuildEffect(ctx, rec, next, eff.kind)
				if berr != nil {
					return berr
				}
				if len(p) > 0 {
					payload = p
				}
			}
			if _, err := d.Store.RecordEffectDone(ctx, runID, turnID, eff.kind, payload, HashBytes(payload)); err != nil {
				return err
			}
			if d.testBarrierAfterEffect != nil {
				d.testBarrierAfterEffect(rec.SettlePhase, eff.kind)
			}
		}
		// Then CAS phase.
		if _, err := d.Store.CASAdvanceSettle(ctx, runID, turnID, rev, rec.SettlePhase, next); err != nil {
			return err
		}
		if d.testBarrierAfterCAS != nil {
			d.testBarrierAfterCAS(rec.SettlePhase, next)
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

func settleEffectPayload(runID, turnID, kind string, extra map[string]any) []byte {
	// Stable payload (no wall-clock): convergent replay must hash-equal
	// across outage retries (RecordEffectDone equal-hash is no-op; divergent = conflict).
	m := map[string]any{
		"run_id":  runID,
		"turn_id": turnID,
		"kind":    kind,
	}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func (d *SettleDriver) planNext(ctx context.Context, rec DispatchRecord) (SettlePhase, []settleEffect, error) {
	switch rec.SettlePhase {
	case SettlePending, SettleNone:
		// Fail-closed: never default-allow without EvaluateGate.
		if d.EvaluateGate == nil {
			return "", nil, fmt.Errorf("settle: EvaluateGate required at phase %s (refuse default-allow)", rec.SettlePhase)
		}
		allow, reprompt, err := d.EvaluateGate(ctx, rec.RunID, rec.TurnID)
		if err != nil {
			return "", nil, err
		}
		payload := settleEffectPayload(rec.RunID, rec.TurnID, "gate_eval", map[string]any{
			"allow": allow, "reprompt": reprompt,
		})
		if reprompt || !allow {
			return SettleSupersededReprompt, []settleEffect{{kind: "gate_eval", payload: payload}}, nil
		}
		return SettleGateEvaluated, []settleEffect{{kind: "gate_eval", payload: payload}}, nil
	case SettleGateEvaluated:
		payload := settleEffectPayload(rec.RunID, rec.TurnID, "completion_event", map[string]any{
			"outcome": string(rec.State),
		})
		return SettleCompletionCommitted, []settleEffect{{kind: "completion_event", payload: payload}}, nil
	case SettleCompletionCommitted:
		payload := settleEffectPayload(rec.RunID, rec.TurnID, "graph_signal", map[string]any{
			"outcome": string(rec.State),
		})
		return SettleGraphSettled, []settleEffect{{kind: "graph_signal", payload: payload}}, nil
	case SettleGraphSettled:
		// dependents_release: stop-generation suppression recorded in payload
		// (full CreateReleaseManifestItem is owned by live releaseDependentAgents;
		// the effect marker is the durable audit + skip-optimization).
		payload := settleEffectPayload(rec.RunID, rec.TurnID, "dependents_release", map[string]any{
			"stop_outcome": rec.StopOutcome,
		})
		return SettleDependentsReleased, []settleEffect{{kind: "dependents_release", payload: payload}}, nil
	case SettleDependentsReleased:
		payload := settleEffectPayload(rec.RunID, rec.TurnID, "finalizer", map[string]any{
			"outcome": string(rec.State),
		})
		return SettleFinalized, []settleEffect{{kind: "finalizer", payload: payload}}, nil
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
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
		}
	}
	return last
}
