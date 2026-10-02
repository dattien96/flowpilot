package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Task-251 T-1/T-3: production SettleDriver wiring on InteractiveService.
// Gate evaluation stays in resumePendingFlowGate (BUG-288 guards preserved);
// the driver owns durable SettlePhase advancement + effect ledger once gate
// disposition is known.

var (
	settleRetryMu     sync.Mutex
	settleRetryQueued = map[string]bool{}
)

// errSettleGatePending marks the normal "gate still pending" defer — the gate
// eval owns the disposition and the periodic sweep re-drives. It is NOT an
// exhaustion surface: wedged drivers (CAS/store failures, unknown phases) are.
var errSettleGatePending = fmt.Errorf("settle: gate still pending")

// unstickSettleResidueLocked releases finalize-tail residue on a run whose
// settle-pending turn is already terminal in the dispatch ledger (BUG-540,
// live run-1663/turn-6304):
//   - a post-turn gate eval held past postTurnGateBusyBound is wedged — bump
//     gateEpoch so any late return discards its side effects, drop the cancel,
//     and release the resumePendingFlowGate claim so a fresh eval can take it.
//   - turnInFlight still pinned on the terminal turn (no live gate) is residue
//     — it refuses every reinvoke/flush with turn_in_progress.
//
// Caller holds s.mu and persists the returned snapshot if true.
func unstickSettleResidueLocked(rs *interactiveRun, turnID string) bool {
	if rs == nil {
		return false
	}
	released := false
	gateBusy := gateCancelLive(rs.postTurnGateStartedAt, rs.postTurnGateCancel)
	if rs.postTurnGateCancel != nil && !gateBusy {
		// Stale gate window — the eval goroutine never returned (or died).
		rs.gateEpoch++
		rs.postTurnGateCancel = nil
		released = true
	}
	if rs.gateClaimID != "" && !gateBusy {
		// A claim held with no live eval is a wedged resumePendingFlowGate
		// (crash between claim and postTurnGateCancel stamp). Clearing the
		// claim is self-correcting: the holder re-validates claimID before it
		// evaluates and aborts itself on mismatch.
		rs.gateClaimID = ""
		released = true
	}
	if !gateBusy && rs.turnInFlight &&
		(rs.currentTurnID == turnID || (rs.currentTurnID == "" && rs.lastTurnID == turnID)) {
		rs.turnInFlight = false
		released = true
	}
	return released
}

// surfaceSettleDriveExhausted records a settle-driver exhaustion durably —
// BUG-540 requires operator-visible surfacing, not log-and-drop. The
// settle_pending record stays in ListAttention either way; the diag entry
// names the turn so the wedge is attributable.
func (s *InteractiveService) surfaceSettleDriveExhausted(runID, turnID string, err error) {
	parent := runID
	s.mu.Lock()
	if rs := s.runs[runID]; rs != nil && rs.parentRunID != "" {
		parent = rs.parentRunID
	}
	s.mu.Unlock()
	s.flowDiagLog(parent, "settle_backoff_exhausted",
		"settle driver backoff exhausted; in-session sweep will re-drive",
		"run_id", runID, "turn_id", turnID, "error", err.Error())
}

// settleSweepInterval bounds how long a terminal+settle_owed record can sit
// without an in-session driver. Var (not const) so tests can shrink it.
var settleSweepInterval = 30 * time.Second

// StartSettleSweep is the in-session counterpart of the boot settle drive
// (BUG-540): before this, an owed settle whose driver was lost — backoff
// exhausted, wedged gate eval, blocked-loop defer with no re-kick — sat
// silently until process restart. The sweep re-walks recoverable settles on
// an interval and converges them through the same driveOwedSettleRecords path.
func (s *InteractiveService) StartSettleSweep(ctx context.Context) {
	if s == nil || s.dispatchStore == nil {
		return
	}
	go func() {
		t := time.NewTicker(settleSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.driveOwedSettles(ctx)
				// BUG-571/572/579/581: converge live-run armed/deferred
				// state that lost its driver — the settle sweep is the only
				// in-session watchdog, so piggyback it here.
				s.sweepWedgedFlowWork()
			}
		}
	}()
}

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
		loopDone := s.flowLoopDone(runID)
		if rs.pendingFlowGateSettle && !loopDone {
			go s.resumePendingFlowGate(runID)
			return false, false, fmt.Errorf("%w run=%s turn=%s (resume scheduled)", errSettleGatePending, runID, turnID)
		}
		if strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" {
			return false, true, nil
		}
		if rs.status == RunStatusCompleted || loopDone {
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
			if errors.Is(err, errSettleGatePending) {
				// Gate genuinely owns the disposition — the sweep re-drives.
				log.Printf("[settle] gate still pending run=%s turn=%s (deferring to gate/sweep)", runID, turnID)
				return
			}
			log.Printf("[settle] RetrySettleWithBackoff exhausted run=%s turn=%s: %v", runID, turnID, err)
			s.surfaceSettleDriveExhausted(runID, turnID, err)
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
	if pendingGate && !s.flowLoopDone(runID) {
		// Gate path owns the next step; do not auto-allow.
		return
	}
	s.scheduleSettleDrive(runID, turnID)
}

func (s *InteractiveService) flowLoopDone(runID string) bool {
	if s == nil || s.agentOrchestrator == nil {
		return false
	}
	return strings.TrimSpace(s.agentOrchestrator.loopStateFor(runID).Status) == "done"
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
