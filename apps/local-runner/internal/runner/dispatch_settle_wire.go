package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

// Task-251 T-1/T-3: production SettleDriver wiring on InteractiveService.
// Gate evaluation stays in resumePendingFlowGate (BUG-288 guards preserved);
// the driver owns durable SettlePhase advancement + effect ledger once gate
// disposition is known.

var (
	settleRetryMu     sync.Mutex
	settleRetryQueued = map[string]bool{}
)

// newSettleDriver builds a production-wired SettleDriver (EvaluateGate required).
func (s *InteractiveService) newSettleDriver() *SettleDriver {
	if s == nil || s.dispatchStore == nil {
		return nil
	}
	return &SettleDriver{
		Store:        s.dispatchStore,
		EvaluateGate: s.evaluateSettleGate,
		OnFinalized:  s.onSettleFinalized,
	}
}

// evaluateSettleGate implements Task-251 T-1 fail-closed gate disposition for settle.
//
// Rules:
//  1. If the live/reconstructed run still has pendingFlowGateSettle, refuse settle
//     and schedule resumePendingFlowGate — never default-allow.
//  2. If a reprompt intent is armed, return (false, true) → settle_superseded_reprompt.
//  3. If the run is Completed (gate already passed live), allow.
//  4. Prior durable gate_eval effect is authoritative for replay.
//  5. Terminal cancelled → allow bookkeeping (T-2b).
//  6. No run state and no prior gate_eval → fail-closed (leave settle_pending).
func (s *InteractiveService) evaluateSettleGate(ctx context.Context, runID, turnID string) (allow bool, reprompt bool, err error) {
	if s == nil {
		return false, false, fmt.Errorf("settle: nil service")
	}
	if s.dispatchStore == nil {
		return false, false, fmt.Errorf("settle: no dispatch store")
	}

	// Prior gate_eval effect is durable proof (replay-safe across restarts).
	if effects, lerr := s.dispatchStore.ListEffects(ctx, runID, turnID); lerr == nil {
		for _, e := range effects {
			if e.EffectKind != "gate_eval" {
				continue
			}
			if strings.Contains(string(e.Payload), `"allow":true`) {
				return true, false, nil
			}
			if strings.Contains(string(e.Payload), `"reprompt":true`) || strings.Contains(string(e.Payload), `"allow":false`) {
				return false, true, nil
			}
		}
	}

	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		// One-shot reconstruct for boot path (no recursive re-entry loop).
		if _, apiErr := s.loadPersistedRun(runID); apiErr == nil {
			s.mu.Lock()
			rs = s.runs[runID]
			s.mu.Unlock()
		}
	}
	if rs != nil {
		if rs.pendingFlowGateSettle {
			go s.resumePendingFlowGate(runID)
			return false, false, fmt.Errorf("settle: gate still pending run=%s turn=%s (resume scheduled)", runID, turnID)
		}
		if strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" {
			return false, true, nil
		}
		if rs.status == RunStatusCompleted {
			return true, false, nil
		}
		// Live run, gate not pending, not completed: allow bookkeeping after
		// terminal commit when gate was never armed (or already cleared).
		return true, false, nil
	}

	rec, _, gerr := s.dispatchStore.Get(ctx, runID, turnID)
	if gerr != nil {
		return false, false, gerr
	}
	// Terminal cancelled: bookkeeping may complete without a live run (T-2b).
	if rec.State == DispatchTerminalCancelled {
		return true, false, nil
	}
	// No session, no prior gate_eval → fail-closed (do not invent allow).
	return false, false, fmt.Errorf("settle: no run state for gate eval run=%s turn=%s state=%s", runID, turnID, rec.State)
}

func (s *InteractiveService) onSettleFinalized(ctx context.Context, runID, turnID string) {
	_ = ctx
	log.Printf("[settle] finalized run=%s turn=%s", runID, turnID)
	// Wake idle flushes / hub watchdogs after durable settle completes.
	s.notifyTurnIdle(runID)
}

// scheduleSettleDrive queues a single-flight DriveSettle with backoff (T-3).
func (s *InteractiveService) scheduleSettleDrive(runID, turnID string) {
	if s == nil || s.dispatchStore == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(turnID) == "" {
		return
	}
	key := runID + "/" + turnID
	settleRetryMu.Lock()
	if settleRetryQueued[key] {
		settleRetryMu.Unlock()
		return
	}
	settleRetryQueued[key] = true
	settleRetryMu.Unlock()

	go func() {
		defer func() {
			settleRetryMu.Lock()
			delete(settleRetryQueued, key)
			settleRetryMu.Unlock()
		}()
		d := s.newSettleDriver()
		if d == nil {
			return
		}
		ctx := context.Background()
		if err := d.RetrySettleWithBackoff(ctx, runID, turnID, 5); err != nil {
			log.Printf("[settle] RetrySettleWithBackoff exhausted run=%s turn=%s: %v", runID, turnID, err)
		}
	}()
}

// maybeScheduleSettleAfterTerminal is called after CommitTerminalAndSettleIntent.
// If the run still needs gate evaluation, leave settle_pending for
// resumePendingFlowGate → scheduleSettleDrive; otherwise start the driver.
func (s *InteractiveService) maybeScheduleSettleAfterTerminal(runID, turnID string) {
	if s == nil || s.dispatchStore == nil {
		return
	}
	ctx := context.Background()
	rec, _, err := s.dispatchStore.Get(ctx, runID, turnID)
	if err != nil || !rec.SettleOwed || rec.SettlePhase.IsSettleFinal() {
		return
	}
	s.mu.Lock()
	rs := s.runs[runID]
	pendingGate := rs != nil && rs.pendingFlowGateSettle
	s.mu.Unlock()
	if pendingGate {
		// Gate path owns the next step; do not auto-allow.
		return
	}
	s.scheduleSettleDrive(runID, turnID)
}

// scheduleSettleAfterGatePass is called when resumePendingFlowGate (or live
// post-gate) successfully passes and persisted completion — durable settle
// phases can now advance with EvaluateGate allow.
func (s *InteractiveService) scheduleSettleAfterGatePass(runID, turnID string) {
	if strings.TrimSpace(turnID) == "" {
		return
	}
	s.scheduleSettleDrive(runID, turnID)
}

// scheduleSettleAfterGateBlock marks settle as superseded-reprompt when the
// gate blocks/reprompts (durable disposition T-2b).
func (s *InteractiveService) scheduleSettleAfterGateBlock(runID, turnID string) {
	if s == nil || s.dispatchStore == nil || strings.TrimSpace(turnID) == "" {
		return
	}
	go func() {
		d := &SettleDriver{
			Store: s.dispatchStore,
			EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
				return false, true, nil
			},
		}
		if err := d.DriveSettle(context.Background(), runID, turnID); err != nil {
			log.Printf("[settle] gate-block disposition run=%s turn=%s: %v", runID, turnID, err)
		}
	}()
}
