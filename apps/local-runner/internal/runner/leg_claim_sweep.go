package runner

import (
	"context"
	"fmt"
	"log"
	"time"
)

// BUG-543 residual: park/stop-cancelled and completed flow children keep
// leg_state=active forever because the claim is their re-drivable provider
// pin and no status transition may close it (the four explicit close seams —
// flow_done, member_skipped, dispatch_failed, worktree_swept — are the only
// transitions that know the member is done for good). Orphaned claims
// accumulate as durable garbage when a parent is sealed and never resumed.
//
// sweepStaleLegClaims is the conservative reclamation seam the BUG-543
// assessment calls for: a claim is closed only when EVERY signal says the
// child can never be re-driven — terminal status, no armed intents, no
// pending cards, no in-flight merge resolution, older than the reclaim
// window, and the parent loop is provably sealed (stopped/done), itself
// terminal and loopless, or absent entirely. Anything uncertain keeps the
// claim (mirrors the worktree GC's gcUnknown fail-closed rule).
//
// Every reclaim emits EventLegClaimReclaimed through the flow-events sidecar
// plus a leg_closed_reason=claim_reclaimed row update, so the decision is
// auditable post-restart from either record.

// legClaimReclaimMinAge is the "never resumed" evidence window: a sealed
// parent can still be extended and resumed, so only claims untouched for
// longer than this are treated as garbage. Var (not const) so tests can
// shrink it.
var legClaimReclaimMinAge = 24 * time.Hour

// legClaimSweepInterval is the in-session counterpart of the boot sweep — a
// sealed-loop claim orphaned mid-session converges without a restart.
var legClaimSweepInterval = 10 * time.Minute

// StartLegClaimSweep mirrors StartSettleSweep (BUG-540): periodic re-walk of
// stale leg claims so a mid-session seal does not need a restart to converge.
func (s *InteractiveService) StartLegClaimSweep(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		t := time.NewTicker(legClaimSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweepStaleLegClaims(ctx)
			}
		}
	}()
}

func (s *InteractiveService) sweepStaleLegClaims(ctx context.Context) {
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return
	}
	sessions, err := indexReader.ListAllProviderSessions(ctx)
	if err != nil {
		log.Printf("[leg-gc] session index unreadable: %v — deferring sweep", err)
		return
	}
	now := time.Now().UTC()
	for _, sess := range sessions {
		if verdict, _ := s.legClaimVerdict(ctx, sess, now); verdict {
			s.reclaimLegClaim(ctx, sess, now)
		}
	}
	// Resident runs may carry an active leg whose durable row predates the
	// claim (or whose row was never written): evaluate their fresh snapshot
	// through the same predicate so in-session residue converges too.
	s.mu.Lock()
	residents := make([]*interactiveRun, 0)
	for _, rs := range s.runs {
		if rs.legState == LegStateActive && rs.parentRunID != "" && !rs.turnInFlight {
			residents = append(residents, rs)
		}
	}
	s.mu.Unlock()
	for _, rs := range residents {
		s.mu.Lock()
		fresh := sessionStateOf(rs)
		s.mu.Unlock()
		if verdict, _ := s.legClaimVerdict(ctx, fresh, now); verdict {
			s.reclaimLegClaim(ctx, fresh, now)
		}
	}
}

// legClaimVerdict is the conservative predicate. Any doubt keeps the claim —
// closing a leg that could still be re-driven would surface as leg_closed on
// resume (the failure mode BUG-543's reverted broad close hit).
func (s *InteractiveService) legClaimVerdict(ctx context.Context, sess ProviderSessionState, now time.Time) (bool, string) {
	if sess.LegState != LegStateActive || sess.ParentRunID == "" {
		return false, ""
	}
	// Remote-origin rows describe a leg owned by another machine — the source
	// machine may still re-drive it; reclaiming here would poison that leg.
	if sess.SourceMachineID != "" || sess.RestoredFrom != "" {
		return false, ""
	}
	// Only provably-terminal rows: waiting_* statuses imply a live gate/card
	// surface, running/starting can still turn.
	switch sess.Status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
	default:
		return false, ""
	}
	// Stale enough to count as "never resumed": a sealed loop can still be
	// extended via extend-cap, so recency alone disqualifies the claim.
	updatedAt, err := time.Parse(time.RFC3339Nano, sess.UpdatedAt)
	if err != nil {
		if updatedAt, err = time.Parse(time.RFC3339, sess.UpdatedAt); err != nil {
			return false, "" // unreadable stamp — cannot prove staleness
		}
	}
	if now.Sub(updatedAt) < legClaimReclaimMinAge {
		return false, ""
	}
	// Armed durable intents are explicit re-drive promises.
	if sess.PendingResumePrompt != "" || sess.PendingResumeStepID != "" ||
		sess.PendingGateRepromptPrompt != "" || sess.PendingGateRepromptStepID != "" ||
		sess.PendingFlowGateSettle || sess.PendingRestartPrompt != "" || sess.PendingRestartRunID != "" {
		return false, ""
	}
	// A merge resolution in flight or a merge-pending binding is actionable
	// user-facing state — never reclaim under it.
	if sess.WorktreeResolutionPhase != "" || sess.WorktreeState == "merge_pending" {
		return false, ""
	}
	// Pending durable cards are re-drive surfaces (orphan-cure revives armed
	// children); a reader that cannot answer fails closed to keep.
	if s.legClaimHasPendingCard(ctx, sess.RunID) {
		return false, ""
	}
	// The decisive signal: the parent loop must be provably sealed — stopped,
	// done, terminal-and-loopless, or the row deleted entirely.
	if !s.legClaimParentSealed(ctx, sess.ParentRunID) {
		return false, ""
	}
	return true, fmt.Sprintf("parent=%s age=%s", sess.ParentRunID, now.Sub(updatedAt).Round(time.Minute))
}

// legClaimHasPendingCard reports whether the run still owns an unresolved
// approval/question card. Reader errors fail closed (treated as pending) —
// mirroring the gcUnknown rule that "could not check" is never safe.
func (s *InteractiveService) legClaimHasPendingCard(ctx context.Context, runID string) bool {
	if reader, ok := s.workflowStore.(ApprovalHistoryReader); ok {
		approvals, err := reader.ListApprovalsByRun(ctx, runID)
		if err != nil {
			return true
		}
		for _, a := range approvals {
			if a.Status == "pending" {
				return true
			}
		}
	}
	if reader, ok := s.workflowStore.(QuestionHistoryReader); ok {
		questions, err := reader.ListQuestionsByRun(ctx, runID)
		if err != nil {
			return true
		}
		for _, q := range questions {
			if q.Status == "pending" {
				return true
			}
		}
	}
	return false
}

// legClaimParentSealed resolves whether the child's parent can never legally
// re-drive it again. A resident parent's live loop state wins over its
// persisted row (the row may lag); a durable-only parent is sealed on
// stopped/done loop status, or on terminal run status with no loop at all.
// A missing parent row means the parent was deleted — the child is garbage.
// Any read failure keeps the claim.
func (s *InteractiveService) legClaimParentSealed(ctx context.Context, parentID string) bool {
	s.mu.Lock()
	parent := s.runs[parentID]
	s.mu.Unlock()
	if parent != nil {
		loop := s.agentOrchestrator.loopStateFor(parentID)
		if loop.Status == "stopped" || loop.Status == "done" {
			return true
		}
		s.mu.Lock()
		status := parent.status
		s.mu.Unlock()
		return loop.Status == "" && isTerminalRunStatus(status)
	}
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return false
	}
	sess, found, err := reader.GetProviderSession(ctx, parentID)
	if err != nil {
		return false
	}
	if !found {
		return true // parent row deleted — nothing can re-drive the child
	}
	if sess.LoopState.Status == "stopped" || sess.LoopState.Status == "done" {
		return true
	}
	return sess.LoopState.Status == "" && isTerminalRunStatus(sess.Status)
}

func isTerminalRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	}
	return false
}

// reclaimLegClaim closes the claim on the resident run when present, always
// flips the durable row, and emits the durable audit event. Idempotent: a
// leg already closed by a racing explicit seam is left alone.
func (s *InteractiveService) reclaimLegClaim(ctx context.Context, sess ProviderSessionState, now time.Time) {
	detail := fmt.Sprintf("leg claim reclaimed: parent=%s sealed; run status=%s", sess.ParentRunID, sess.Status)
	s.mu.Lock()
	if rs := s.runs[sess.RunID]; rs != nil {
		if rs.legState != LegStateActive {
			s.mu.Unlock()
			return // an explicit seam already closed it — nothing to reclaim
		}
		rs.legState = LegStateClosed
		rs.legClosedReason = LegClosedReasonReclaimed
		clearClosedLegPendingLocked(rs)
		snap := sessionStateOf(rs)
		s.emitLocked(rs, ProviderEvent{Type: EventLegClaimReclaimed, Text: detail})
		s.mu.Unlock()
		if err := s.persistProviderSession(snap); err != nil {
			log.Printf("[leg-gc] persist reclaimed leg run=%s: %v", sess.RunID, err)
		}
		log.Printf("[leg-gc] reclaimed leg claim run=%s parent=%s status=%s", sess.RunID, sess.ParentRunID, sess.Status)
		return
	}
	s.mu.Unlock()

	// Durable-only row: flip the claim on the session record itself.
	sess.LegState = LegStateClosed
	sess.LegClosedReason = LegClosedReasonReclaimed
	clearClosedLegPendingSession(&sess)
	sess.UpdatedAt = now.Format(time.RFC3339Nano)
	store := s.persistenceStore()
	if store == nil {
		return
	}
	if err := store.UpsertProviderSession(ctx, sess); err != nil {
		log.Printf("[leg-gc] persist reclaimed leg run=%s: %v", sess.RunID, err)
		return
	}
	_ = s.persistEvent(ProviderEvent{
		ID:                s.nextID("evt"),
		Type:              EventLegClaimReclaimed,
		WorkflowRunID:     sess.RunID,
		ProviderSessionID: sess.ProviderSessionID,
		ProviderKey:       sess.ProviderKey,
		OccurredAt:        now.Format(time.RFC3339Nano),
		Text:              detail,
	})
	log.Printf("[leg-gc] reclaimed leg claim run=%s parent=%s status=%s", sess.RunID, sess.ParentRunID, sess.Status)
}
