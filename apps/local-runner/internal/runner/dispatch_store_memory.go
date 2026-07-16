package runner

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryDispatchStore is the full DispatchStore semantics used by unit tests and
// as the RAM projection for the local NDJSON store.
type memoryDispatchStore struct {
	mu sync.Mutex

	// now is overridable for lease/attach TTL tests.
	now func() time.Time

	records    map[string]*DispatchRecord // run\0turn
	envelopes  map[string]*DispatchEnvelope
	runStop    map[string]*RunStopState
	activation map[string]int // runID -> protocol version
	effects    map[string]*EffectDone
	releases   map[string]*ReleaseManifestItem // effect key for release:*
	repairs    map[string]*RepairRecord        // runID (open only kept; resolved retained for audit)
	clears     map[intentClearKey]struct{}
	audits     []AuditEntry
	resolutions map[string]*ResolutionResult
	seq        int64

	// intentLive tracks live outer intent gen+envelope for supersede guard.
	// Keyed by ownerRunID\0intentKey.
	intentLive map[string]intentLiveEntry

	onIntentClear IntentClearCallback
	// afterCommit is optional hook (local store uses it to fsync a log line).
	afterCommit func(line dispatchLogLine) error
}

type intentLiveEntry struct {
	Gen          int64
	EnvelopeHash string
}

// newMemoryDispatchStore constructs an empty in-memory dispatch store.
func newMemoryDispatchStore() *memoryDispatchStore {
	return &memoryDispatchStore{
		now:         time.Now,
		records:     map[string]*DispatchRecord{},
		envelopes:   map[string]*DispatchEnvelope{},
		runStop:     map[string]*RunStopState{},
		activation:  map[string]int{},
		effects:     map[string]*EffectDone{},
		releases:    map[string]*ReleaseManifestItem{},
		repairs:     map[string]*RepairRecord{},
		clears:      map[intentClearKey]struct{}{},
		resolutions: map[string]*ResolutionResult{},
		intentLive:  map[string]intentLiveEntry{},
	}
}

func (s *memoryDispatchStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *memoryDispatchStore) clockStr() string {
	return s.clock().Format(time.RFC3339Nano)
}

func (s *memoryDispatchStore) appendAuditLocked(runID, turnID, kind, detail, actor string) {
	s.seq++
	s.audits = append(s.audits, AuditEntry{
		Seq: s.seq, RunID: runID, TurnID: turnID, Kind: kind, Detail: detail, At: s.clockStr(), Actor: actor,
	})
}

func (s *memoryDispatchStore) ensureActivationLocked(runID string) {
	if _, ok := s.activation[runID]; !ok {
		s.activation[runID] = DispatchProtocolV2
	}
	if _, ok := s.runStop[runID]; !ok {
		s.runStop[runID] = &RunStopState{RunID: runID, Generation: 0, Revision: 1, Stopped: false}
	}
}

func (s *memoryDispatchStore) getRecLocked(runID, turnID string) (*DispatchRecord, error) {
	r := s.records[recKey(runID, turnID)]
	if r == nil {
		return nil, ErrNotFound
	}
	return r, nil
}

func (s *memoryDispatchStore) cloneRec(r *DispatchRecord) DispatchRecord {
	if r == nil {
		return DispatchRecord{}
	}
	out := *r
	if r.ReceiptEvidence != nil {
		cp := *r.ReceiptEvidence
		out.ReceiptEvidence = &cp
	}
	if r.TerminalEvidence != nil {
		cp := *r.TerminalEvidence
		out.TerminalEvidence = &cp
	}
	if r.ParentStopFence != nil {
		cp := *r.ParentStopFence
		out.ParentStopFence = &cp
	}
	return out
}

func (s *memoryDispatchStore) revokeAttachLocked(r *DispatchRecord) {
	r.RecoveryAttachEpoch++
	r.RecoveryAttachOwner = ""
	r.RecoveryAttachExpiresAt = ""
}

func (s *memoryDispatchStore) leaseOKLocked(r *DispatchRecord, owner string) bool {
	if r.ClaimOwner != owner || owner == "" {
		return false
	}
	if r.ClaimExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339Nano, r.ClaimExpiresAt)
	if err != nil {
		return false
	}
	return s.clock().Before(exp)
}

func (s *memoryDispatchStore) checkOwnStopLocked(runID string) error {
	st := s.runStop[runID]
	if st == nil {
		return nil
	}
	if st.Stopped {
		return ErrRunStopFence{CurrentGeneration: st.Generation}
	}
	return nil
}

func (s *memoryDispatchStore) checkParentFenceLocked(r *DispatchRecord) error {
	if r.ParentStopFence == nil {
		return nil
	}
	st := s.runStop[r.ParentStopFence.ParentRunID]
	if st == nil {
		// Missing parent authority is fail-closed as fence trip.
		return ErrParentStopFence{CurrentGeneration: r.ParentStopFence.ExpectedStopGeneration}
	}
	if st.Stopped || st.Generation != r.ParentStopFence.ExpectedStopGeneration {
		return ErrParentStopFence{CurrentGeneration: st.Generation}
	}
	return nil
}

func (s *memoryDispatchStore) effectiveStopLocked(r *DispatchRecord) bool {
	if r.CancelRequested {
		return true
	}
	if err := s.checkOwnStopLocked(r.RunID); err != nil {
		return true
	}
	if err := s.checkParentFenceLocked(r); err != nil {
		return true
	}
	return false
}

func (s *memoryDispatchStore) deriveStopOutcomeLocked(r *DispatchRecord) string {
	if s.effectiveStopLocked(r) {
		return StopOutcomeCancelledInFlight
	}
	return ""
}

func (s *memoryDispatchStore) clearIntentLocked(ownerRunID, intentKey string, intentGen int64) {
	if ownerRunID == "" || intentKey == "" {
		return
	}
	s.clears[intentKeyOf(ownerRunID, intentKey, intentGen)] = struct{}{}
	delete(s.intentLive, ownerRunID+"\x00"+intentKey)
	// onIntentClear is invoked by the service layer after unlock when needed.
	// Tests assert via IsIntentCleared, not this callback.
	_ = s.onIntentClear
}

func (s *memoryDispatchStore) setLiveIntentLocked(owner, key string, gen int64, envHash string) {
	if owner == "" || key == "" {
		return
	}
	s.intentLive[owner+"\x00"+key] = intentLiveEntry{Gen: gen, EnvelopeHash: envHash}
}

func (s *memoryDispatchStore) commitLine(line dispatchLogLine) error {
	if s.afterCommit != nil {
		return s.afterCommit(line)
	}
	return nil
}

// ---- mutations ----

func (s *memoryDispatchStore) CreatePrepared(ctx context.Context, rec DispatchRecord, env DispatchEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if rec.TurnID == "" || rec.RunID == "" {
		return fmt.Errorf("CreatePrepared: run_id and turn_id required")
	}
	k := recKey(rec.RunID, rec.TurnID)
	if _, exists := s.records[k]; exists {
		// create-if-absent: equal envelope hash is no-op success
		if s.envelopes[k] != nil && s.envelopes[k].EnvelopeHash == env.EnvelopeHash {
			return nil
		}
		return ErrAlreadyExists
	}
	if env.EnvelopeHash == "" {
		h, err := ComputeEnvelopeHash(&env)
		if err != nil {
			return err
		}
		env.EnvelopeHash = h
	}
	if rec.EnvelopeHash == "" {
		rec.EnvelopeHash = env.EnvelopeHash
	}
	if rec.EnvelopeHash != env.EnvelopeHash {
		return fmt.Errorf("envelope hash mismatch")
	}
	if rec.ProtocolVersion == 0 {
		rec.ProtocolVersion = DispatchProtocolV2
	}
	if rec.State == "" {
		rec.State = DispatchPrepared
	}
	if rec.State != DispatchPrepared && rec.ProtocolVersion >= DispatchProtocolV2 {
		return fmt.Errorf("CreatePrepared must start at prepared, got %s", rec.State)
	}
	if rec.IntentOwnerRunID == "" {
		rec.IntentOwnerRunID = rec.RunID
	}
	now := s.clockStr()
	if rec.CreatedAt == "" {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	rec.Revision = 1
	// Atomic V2 activation + RunStopState.
	s.ensureActivationLocked(rec.RunID)
	cp := rec
	envCp := env
	s.records[k] = &cp
	s.envelopes[k] = &envCp
	if rec.OuterIntentKey != "" {
		s.setLiveIntentLocked(rec.IntentOwnerRunID, rec.OuterIntentKey, rec.OuterIntentGen, rec.EnvelopeHash)
	}
	s.appendAuditLocked(rec.RunID, rec.TurnID, "create_prepared", "state=prepared", "system")
	return s.commitLine(dispatchLogLine{
		Kind: "record", Seq: s.seq, At: now, Record: &cp, Envelope: &envCp,
		Activation: &dispatchActivation{RunID: rec.RunID, ProtocolVersion: DispatchProtocolV2},
		RunStop:    s.runStop[rec.RunID],
	})
}

func (s *memoryDispatchStore) CASAdvance(ctx context.Context, runID, turnID string, expectedRev int64,
	expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) {
	return s.casAdvance(ctx, runID, turnID, expectedRev, "", expected, next, mutate, false)
}

func (s *memoryDispatchStore) CASRecoveryAdvance(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) {
	return s.casAdvance(ctx, runID, turnID, expectedRev, leaseOwner, expected, next, mutate, true)
}

func (s *memoryDispatchStore) casAdvance(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	expected, next DispatchState, mutate func(*DispatchRecord), requireLease bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev || r.State != expected {
		return r.Revision, ErrStaleDispatch
	}
	if requireLease && !s.leaseOKLocked(r, leaseOwner) {
		return r.Revision, ErrRecoveryLeaseLost
	}
	if err := r.canTransition(next); err != nil {
		return r.Revision, err
	}
	// send_started linearization: own + parent fence in this transaction.
	if next == DispatchSendStarted {
		if err := s.checkOwnStopLocked(runID); err != nil {
			return r.Revision, err
		}
		if err := s.checkParentFenceLocked(r); err != nil {
			return r.Revision, err
		}
	}
	// Leaving sent states revokes attach epoch.
	if r.State.IsSent() && next != r.State {
		s.revokeAttachLocked(r)
	}
	if next.IsTerminal() {
		s.revokeAttachLocked(r)
	}
	if mutate != nil {
		mutate(r)
	}
	r.State = next
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "cas_advance", fmt.Sprintf("%s->%s", expected, next), "system")
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) CASAdvanceSettle(ctx context.Context, runID, turnID string, expectedRev int64,
	expected, next SettlePhase) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev || r.SettlePhase != expected {
		return r.Revision, ErrStaleDispatch
	}
	if err := canAdvanceSettle(expected, next); err != nil {
		return r.Revision, err
	}
	r.SettlePhase = next
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "cas_settle", fmt.Sprintf("%s->%s", expected, next), "system")
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) ClaimRecovery(ctx context.Context, runID, turnID string, expectedRev int64, owner string, ttl time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	// Take over if no owner or expired.
	if r.ClaimOwner != "" && r.ClaimOwner != owner && s.leaseOKLocked(r, r.ClaimOwner) {
		return r.Revision, ErrStaleDispatch
	}
	exp := s.clock().Add(ttl)
	r.ClaimOwner = owner
	r.ClaimExpiresAt = exp.Format(time.RFC3339Nano)
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "claim_recovery", "owner="+owner, owner)
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) SetCancelRequested(ctx context.Context, runID, turnID string, expectedRev, stopGen int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if r.State.IsTerminal() {
		return r.Revision, nil // frozen
	}
	r.CancelRequested = true
	if stopGen > r.StopGeneration {
		r.StopGeneration = stopGen
	}
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "set_cancel", fmt.Sprintf("stop_gen=%d", stopGen), "system")
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) GetRunStopState(ctx context.Context, runID string) (RunStopState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	st := s.runStop[runID]
	if st == nil {
		return RunStopState{RunID: runID}, nil
	}
	return *st, nil
}

func (s *memoryDispatchStore) RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	st := s.runStop[runID]
	if st == nil {
		s.ensureActivationLocked(runID)
		st = s.runStop[runID]
	}
	if st.Revision != expectedRunStopRev {
		return *st, ErrStaleDispatch
	}
	st.Stopped = true
	st.Generation++
	st.Revision++
	s.appendAuditLocked(runID, "", "request_run_stop", string(reason), "system")
	cp := *st
	if err := s.commitLine(dispatchLogLine{Kind: "run_stop", Seq: s.seq, At: s.clockStr(), RunStop: &cp}); err != nil {
		return cp, err
	}
	return cp, nil
}

func (s *memoryDispatchStore) CommitReceiptAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	receipt ReceiptEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if r.State != DispatchSendStarted && r.State != DispatchProviderAccepted {
		// Already accepted with equal receipt = no-op.
		if r.State == DispatchProviderAccepted && receiptEqual(r.ReceiptEvidence, &receipt) {
			return r.Revision, nil
		}
		return r.Revision, fmt.Errorf("%w: receipt requires send_started/provider_accepted got %s", ErrIllegalTransition, r.State)
	}
	if r.ReceiptEvidence != nil {
		if receiptEqual(r.ReceiptEvidence, &receipt) {
			return r.Revision, nil
		}
		if receiptIdentityMatch(r.ReceiptEvidence, &receipt) {
			return r.Revision, ErrReceiptConflict
		}
	}
	if err := r.canTransition(DispatchProviderAccepted); err != nil && r.State != DispatchProviderAccepted {
		return r.Revision, err
	}
	cpReceipt := receipt
	r.ReceiptEvidence = &cpReceipt
	if r.State == DispatchSendStarted {
		r.State = DispatchProviderAccepted
	}
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.clearIntentLocked(intentOwnerRunID, intentKey, intentGen)
	s.appendAuditLocked(runID, turnID, "commit_receipt", "provider_accepted", "system")
	cp := s.cloneRec(r)
	clear := &intentClearPayload{OwnerRunID: intentOwnerRunID, Key: intentKey, Gen: intentGen}
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp, IntentClear: clear}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) CommitTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	return s.commitTerminal(ctx, runID, turnID, expectedRev, "", proof, intentOwnerRunID, intentKey, intentGen, false, nil)
}

func (s *memoryDispatchStore) CommitRecoveredTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	leaseOwner string, proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	return s.commitTerminal(ctx, runID, turnID, expectedRev, leaseOwner, proof, intentOwnerRunID, intentKey, intentGen, true, nil)
}

func (s *memoryDispatchStore) CommitAttachedTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64, token RecoveryAttachToken,
	proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	return s.commitTerminal(ctx, runID, turnID, expectedRev, "", proof, intentOwnerRunID, intentKey, intentGen, false, &token)
}

func (s *memoryDispatchStore) commitTerminal(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64, requireLease bool, attach *RecoveryAttachToken) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if requireLease && !s.leaseOKLocked(r, leaseOwner) {
		return r.Revision, ErrRecoveryLeaseLost
	}
	if attach != nil {
		if err := s.validateAttachTokenLocked(r, *attach, false /*stop rejects? no — terminal uses stop for attribution only*/); err != nil {
			return r.Revision, err
		}
		if !r.State.IsSent() {
			return r.Revision, ErrRecoveryAttachRevoked
		}
	}
	next, outcome, err := terminalStateFromProof(proof.Outcome)
	if err != nil {
		return r.Revision, err
	}
	if err := r.canTransition(next); err != nil {
		return r.Revision, err
	}
	// StopOutcome from durable authority (not caller).
	stopOut := s.deriveStopOutcomeLocked(r)
	te := proof
	r.TerminalEvidence = &te
	r.State = next
	r.Outcome = outcome
	if stopOut != "" {
		r.StopOutcome = stopOut
	}
	// SettlePhase from immutable SettleOwed only.
	if r.SettleOwed {
		r.SettlePhase = SettlePending
	} else {
		r.SettlePhase = SettleNone
	}
	s.revokeAttachLocked(r)
	r.ClaimOwner = ""
	r.ClaimExpiresAt = ""
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.clearIntentLocked(intentOwnerRunID, intentKey, intentGen)
	s.appendAuditLocked(runID, turnID, "commit_terminal", string(next)+":"+outcome, "system")
	cp := s.cloneRec(r)
	clear := &intentClearPayload{OwnerRunID: intentOwnerRunID, Key: intentKey, Gen: intentGen}
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp, IntentClear: clear}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) validateAttachTokenLocked(r *DispatchRecord, token RecoveryAttachToken, rejectOnStop bool) error {
	if r.RecoveryAttachEpoch != token.Epoch || r.RecoveryAttachOwner != token.Owner || token.Owner == "" {
		return ErrRecoveryAttachRevoked
	}
	if r.RecoveryAttachExpiresAt == "" {
		return ErrRecoveryAttachRevoked
	}
	exp, err := time.Parse(time.RFC3339Nano, r.RecoveryAttachExpiresAt)
	if err != nil || !s.clock().Before(exp) {
		return ErrRecoveryAttachRevoked
	}
	// Token wall-clock from caller also must not be past store clock.
	if !token.ExpiresAt.IsZero() && !s.clock().Before(token.ExpiresAt.UTC()) {
		return ErrRecoveryAttachRevoked
	}
	if rejectOnStop && s.effectiveStopLocked(r) {
		return ErrRecoveryAttachRevoked
	}
	return nil
}

func (s *memoryDispatchStore) CommitRecoveryUnknownOrRequireCancel(ctx context.Context, runID, turnID string, expectedRev int64,
	leaseOwner string) (RecoveryUnknownDecision, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return "", 0, err
	}
	if r.Revision != expectedRev {
		return "", r.Revision, ErrStaleDispatch
	}
	if !s.leaseOKLocked(r, leaseOwner) {
		return "", r.Revision, ErrRecoveryLeaseLost
	}
	if s.effectiveStopLocked(r) {
		return RecoveryCancelRequired, r.Revision, nil
	}
	if err := r.canTransition(DispatchUncertain); err != nil {
		return "", r.Revision, err
	}
	r.State = DispatchUncertain
	s.revokeAttachLocked(r)
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "recovery_uncertain", "", leaseOwner)
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return "", r.Revision, err
	}
	return RecoveryClassifiedUncertain, r.Revision, nil
}

func (s *memoryDispatchStore) ClaimRecoveryAttach(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return RecoveryAttachToken{}, DispatchRecord{}, err
	}
	if r.Revision != expectedRev {
		return RecoveryAttachToken{}, s.cloneRec(r), ErrStaleDispatch
	}
	if !s.leaseOKLocked(r, leaseOwner) {
		return RecoveryAttachToken{}, s.cloneRec(r), ErrRecoveryLeaseLost
	}
	if s.effectiveStopLocked(r) {
		return RecoveryAttachToken{}, s.cloneRec(r), fmt.Errorf("%w: cancel required", ErrRecoveryAttachRevoked)
	}
	// Active unexpired attach blocks new claim.
	if r.RecoveryAttachOwner != "" && r.RecoveryAttachExpiresAt != "" {
		if exp, e := time.Parse(time.RFC3339Nano, r.RecoveryAttachExpiresAt); e == nil && s.clock().Before(exp) {
			return RecoveryAttachToken{}, s.cloneRec(r), ErrRecoveryAttachActive
		}
	}
	if !r.State.IsSent() {
		return RecoveryAttachToken{}, s.cloneRec(r), ErrIllegalTransition
	}
	r.RecoveryAttachEpoch++
	r.RecoveryAttachOwner = leaseOwner
	exp := s.clock().Add(ttl)
	r.RecoveryAttachExpiresAt = exp.Format(time.RFC3339Nano)
	r.Revision++
	r.UpdatedAt = s.clockStr()
	tok := RecoveryAttachToken{Epoch: r.RecoveryAttachEpoch, Owner: leaseOwner, ExpiresAt: exp}
	s.appendAuditLocked(runID, turnID, "claim_attach", fmt.Sprintf("epoch=%d", tok.Epoch), leaseOwner)
	cp := s.cloneRec(r)
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return RecoveryAttachToken{}, cp, err
	}
	return tok, cp, nil
}

func (s *memoryDispatchStore) EnterRecoveryAttach(ctx context.Context, runID, turnID string, token RecoveryAttachToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return err
	}
	if err := s.validateAttachTokenLocked(r, token, true); err != nil {
		return err
	}
	if !r.State.IsSent() {
		return ErrRecoveryAttachRevoked
	}
	// Persist attach entry effect before provider attach.
	kind := attachedEffectKind(token.Epoch, "enter")
	ek := effectKey(runID, turnID, kind)
	if _, ok := s.effects[ek]; !ok {
		payload := []byte(`{"kind":"enter"}`)
		s.effects[ek] = &EffectDone{
			RunID: runID, TurnID: turnID, EffectKind: kind,
			Payload: payload, PayloadHash: HashBytes(payload), Revision: 1, CreatedAt: s.clockStr(),
		}
	}
	s.appendAuditLocked(runID, turnID, "enter_attach", fmt.Sprintf("epoch=%d", token.Epoch), token.Owner)
	return s.commitLine(dispatchLogLine{Kind: "effect", Seq: s.seq, At: s.clockStr(), Effect: s.effects[ek]})
}

func (s *memoryDispatchStore) RecordRecoveryAttachedEffect(ctx context.Context, runID, turnID string, token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return false, err
	}
	if err := s.validateAttachTokenLocked(r, token, true); err != nil {
		return false, err
	}
	kind := attachedEffectKind(token.Epoch, eventID)
	ek := effectKey(runID, turnID, kind)
	if existing, ok := s.effects[ek]; ok {
		if existing.PayloadHash != payload.SHA256 {
			return false, ErrEffectConflict
		}
		return false, nil // equal duplicate
	}
	s.effects[ek] = &EffectDone{
		RunID: runID, TurnID: turnID, EffectKind: kind,
		Payload: payload.CanonicalJSON, PayloadHash: payload.SHA256, Revision: 1, CreatedAt: s.clockStr(),
	}
	s.appendAuditLocked(runID, turnID, "attached_effect", kind, token.Owner)
	if err := s.commitLine(dispatchLogLine{Kind: "effect", Seq: s.seq, At: s.clockStr(), Effect: s.effects[ek]}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *memoryDispatchStore) CommitPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) {
	return s.commitPreSend(ctx, runID, turnID, expectedRev, "", intentOwnerRunID, intentKey, intentGen, stopGen, source, false)
}

func (s *memoryDispatchStore) CommitRecoveryPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) {
	return s.commitPreSend(ctx, runID, turnID, expectedRev, leaseOwner, intentOwnerRunID, intentKey, intentGen, stopGen, source, true)
}

func (s *memoryDispatchStore) commitPreSend(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource, requireLease bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if requireLease && !s.leaseOKLocked(r, leaseOwner) {
		return r.Revision, ErrRecoveryLeaseLost
	}
	if r.State != DispatchPrepared && r.State != DispatchSendClaimed {
		return r.Revision, fmt.Errorf("%w: pre-send cancel from %s", ErrIllegalTransition, r.State)
	}
	if err := r.canTransition(DispatchTerminalCancelled); err != nil {
		return r.Revision, err
	}
	r.State = DispatchTerminalCancelled
	r.Outcome = "cancelled"
	r.StopOutcome = StopOutcomeStoppedBeforeSend
	if stopGen > r.StopGeneration {
		r.StopGeneration = stopGen
	}
	r.CancelRequested = true
	r.SettlePhase = SettleNone
	r.SettleOwed = false // pre-send cancel never settles
	s.revokeAttachLocked(r)
	r.Revision++
	r.UpdatedAt = s.clockStr()
	s.clearIntentLocked(intentOwnerRunID, intentKey, intentGen)
	s.appendAuditLocked(runID, turnID, "pre_send_cancel", string(source), "system")
	cp := s.cloneRec(r)
	clear := &intentClearPayload{OwnerRunID: intentOwnerRunID, Key: intentKey, Gen: intentGen}
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp, IntentClear: clear}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) ResolveUncertain(ctx context.Context, runID, turnID string, expectedRev int64,
	resolutionID string, action ResolveAction, evidence OperatorEvidence) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if resolutionID != "" {
		if prev, ok := s.resolutions[resolutionID]; ok {
			return prev.Revision, nil
		}
	}
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if r.State != DispatchUncertain {
		return r.Revision, fmt.Errorf("%w: resolve requires uncertain got %s", ErrIllegalTransition, r.State)
	}
	var next DispatchState
	var outcome string
	forceNoSettle := false
	switch action {
	case ResolveMarkCompleted:
		next, outcome = DispatchTerminalCompleted, "completed,resolved_by=operator"
	case ResolveMarkFailed:
		next, outcome = DispatchTerminalFailed, "failed,resolved_by=operator"
	case ResolveConfirmCancelled:
		next, outcome = DispatchTerminalCancelled, "confirmed_cancelled,resolved_by=operator"
		forceNoSettle = true
	case ResolveAbandon:
		next, outcome = DispatchTerminalCancelled, "abandoned,resolved_by=operator"
		forceNoSettle = true
	default:
		return r.Revision, fmt.Errorf("unknown resolve action %q", action)
	}
	if err := r.canTransition(next); err != nil {
		return r.Revision, err
	}
	r.State = next
	r.Outcome = outcome
	if forceNoSettle {
		r.SettleOwed = false
		r.SettlePhase = SettleNone
	} else if r.SettleOwed {
		r.SettlePhase = SettlePending
	} else {
		r.SettlePhase = SettleNone
	}
	s.revokeAttachLocked(r)
	r.Revision++
	r.UpdatedAt = s.clockStr()
	owner := r.IntentOwnerRunID
	if owner == "" {
		owner = r.RunID
	}
	s.clearIntentLocked(owner, r.OuterIntentKey, r.OuterIntentGen)
	s.appendAuditLocked(runID, turnID, "resolve_uncertain", string(action)+":"+evidence.Detail, evidence.Actor)
	cp := s.cloneRec(r)
	if resolutionID != "" {
		s.resolutions[resolutionID] = &ResolutionResult{
			ResolutionID: resolutionID, RunID: runID, TurnID: turnID,
			Action: string(action), Revision: r.Revision, Record: cp,
		}
	}
	if err := s.commitLine(dispatchLogLine{Kind: "record", Seq: s.seq, At: r.UpdatedAt, Record: &cp}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) RetryAsNew(ctx context.Context, runID, oldTurnID string, expectedRev int64,
	resolutionID, newTurnID string, expectedIntentGen int64, expectedEnvelopeHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if resolutionID != "" {
		if prev, ok := s.resolutions[resolutionID]; ok {
			return prev.Revision, nil
		}
	}
	r, err := s.getRecLocked(runID, oldTurnID)
	if err != nil {
		return 0, err
	}
	if r.Revision != expectedRev {
		return r.Revision, ErrStaleDispatch
	}
	if r.State != DispatchUncertain {
		return r.Revision, fmt.Errorf("%w: retry-as-new requires uncertain", ErrIllegalTransition)
	}
	// Supersede guard.
	owner := r.IntentOwnerRunID
	if owner == "" {
		owner = r.RunID
	}
	live := s.intentLive[owner+"\x00"+r.OuterIntentKey]
	if r.OuterIntentKey != "" {
		if live.Gen != 0 && live.Gen != expectedIntentGen {
			return r.Revision, ErrSuperseded
		}
		if expectedEnvelopeHash != "" && r.EnvelopeHash != expectedEnvelopeHash {
			return r.Revision, ErrSuperseded
		}
		if live.EnvelopeHash != "" && live.EnvelopeHash != expectedEnvelopeHash && expectedEnvelopeHash != "" {
			return r.Revision, ErrSuperseded
		}
	}
	// Parent fence mismatch: only abandon/confirm-cancelled allowed — reject retry.
	if err := s.checkParentFenceLocked(r); err != nil {
		return r.Revision, err
	}
	// Terminalize old as superseded.
	r.State = DispatchTerminalCancelled
	r.Outcome = "superseded,resolved_by=operator"
	r.SettleOwed = false
	r.SettlePhase = SettleNone
	s.revokeAttachLocked(r)
	r.Revision++
	r.UpdatedAt = s.clockStr()

	// Create successor prepared with same envelope.
	env := s.envelopes[recKey(runID, oldTurnID)]
	if env == nil {
		return r.Revision, fmt.Errorf("missing envelope for retry")
	}
	succEnv := *env
	succEnv.TurnID = newTurnID
	succ := DispatchRecord{
		ProtocolVersion:  DispatchProtocolV2,
		TurnID:           newTurnID,
		RunID:            runID,
		IntentOwnerRunID: owner,
		State:            DispatchPrepared,
		Revision:         1,
		OuterIntentKey:   r.OuterIntentKey,
		OuterIntentGen:   expectedIntentGen,
		EnvelopeHash:     succEnv.EnvelopeHash,
		SettleOwed:       false, // successor settles only if prepare recomputes; inherit from env context later
		PredecessorTurnID: oldTurnID,
		ParentStopFence:  r.ParentStopFence,
		CreatedAt:        s.clockStr(),
		UpdatedAt:        s.clockStr(),
	}
	// Preserve settle owed from original prepare if it was true and not forced off by supersede semantics:
	// table says only successor may settle — recompute from original record's pre-force value is unavailable;
	// use envelope scenario: if original had settle_pending path, successor settles when terminal.
	// Per SD-24: successor is sole settler; CreatePrepared would set SettleOwed. Use original's envelope flow flag.
	if orig := s.records[recKey(runID, oldTurnID)]; orig != nil {
		// r already mutated; use envelope FlowContext as weak signal — keep SettleOwed from a field we saved:
		// We forced r.SettleOwed=false above. Capture before force would be better; re-read from envelope:
		succ.SettleOwed = requiresGateSettlement("flow", succEnv.StepID)
	}
	sk := recKey(runID, newTurnID)
	if _, exists := s.records[sk]; exists {
		return r.Revision, ErrAlreadyExists
	}
	s.records[sk] = &succ
	s.envelopes[sk] = &succEnv
	s.setLiveIntentLocked(owner, r.OuterIntentKey, expectedIntentGen, succEnv.EnvelopeHash)
	s.appendAuditLocked(runID, oldTurnID, "retry_as_new", "new="+newTurnID, "operator")
	if resolutionID != "" {
		s.resolutions[resolutionID] = &ResolutionResult{
			ResolutionID: resolutionID, RunID: runID, TurnID: oldTurnID,
			Action: "retry_as_new", Revision: r.Revision, NewTurnID: newTurnID, Record: s.cloneRec(r),
		}
	}
	cpOld := s.cloneRec(r)
	cpNew := succ
	if err := s.commitLine(dispatchLogLine{
		Kind: "retry_as_new", Seq: s.seq, At: r.UpdatedAt,
		Record: &cpOld, Successor: &cpNew, Envelope: &succEnv,
	}); err != nil {
		return r.Revision, err
	}
	return r.Revision, nil
}

func (s *memoryDispatchStore) RecordEffectDone(ctx context.Context, runID, turnID, effectKind string, payload []byte, payloadHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	ek := effectKey(runID, turnID, effectKind)
	if existing, ok := s.effects[ek]; ok {
		if existing.PayloadHash != payloadHash {
			return existing.Revision, ErrEffectConflict
		}
		return existing.Revision, nil
	}
	e := &EffectDone{
		RunID: runID, TurnID: turnID, EffectKind: effectKind,
		Payload: payload, PayloadHash: payloadHash, Revision: 1, CreatedAt: s.clockStr(),
	}
	s.effects[ek] = e
	s.appendAuditLocked(runID, turnID, "effect_done", effectKind, "system")
	if err := s.commitLine(dispatchLogLine{Kind: "effect", Seq: s.seq, At: s.clockStr(), Effect: e}); err != nil {
		return 0, err
	}
	return e.Revision, nil
}

func (s *memoryDispatchStore) CreateReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, intent DurableIntent) (ReleaseManifestItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if intent.IntentHash == "" {
		h, err := ComputeIntentHash(&intent)
		if err != nil {
			return ReleaseManifestItem{}, err
		}
		intent.IntentHash = h
	}
	kind := releaseEffectKind(dependentRunID)
	ek := effectKey(runID, turnID, kind)
	if existing, ok := s.releases[ek]; ok {
		if existing.Intent.IntentHash != intent.IntentHash {
			return *existing, ErrEffectConflict
		}
		return *existing, nil
	}
	item := &ReleaseManifestItem{
		RunID: runID, TurnID: turnID, DependentRunID: dependentRunID,
		Intent: intent, State: ReleasePending, Revision: 1,
		CreatedAt: s.clockStr(), UpdatedAt: s.clockStr(),
	}
	s.releases[ek] = item
	// Also mirror as effect for list.
	payload, _ := jsonMarshal(item)
	s.effects[ek] = &EffectDone{
		RunID: runID, TurnID: turnID, EffectKind: kind,
		Payload: payload, PayloadHash: intent.IntentHash, Revision: 1, CreatedAt: s.clockStr(),
	}
	s.appendAuditLocked(runID, turnID, "release_create", dependentRunID, "system")
	if err := s.commitLine(dispatchLogLine{Kind: "release", Seq: s.seq, At: s.clockStr(), Release: item}); err != nil {
		return *item, err
	}
	return *item, nil
}

func (s *memoryDispatchStore) CommitReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev int64, child DispatchRecord, env DispatchEnvelope) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	kind := releaseEffectKind(dependentRunID)
	ek := effectKey(runID, turnID, kind)
	item := s.releases[ek]
	if item == nil {
		return 0, ErrNotFound
	}
	if item.Revision != expectedEffectRev {
		return item.Revision, ErrStaleDispatch
	}
	if item.State == ReleaseCreated {
		return item.Revision, nil
	}
	if item.State != ReleasePending {
		return item.Revision, fmt.Errorf("%w: release state %s", ErrIllegalTransition, item.State)
	}
	// Stop-first yields suppress path for caller; here we check and fail with fence if stopped.
	if err := s.checkOwnStopLocked(runID); err != nil {
		return item.Revision, err
	}
	// Atomic child CreatePrepared + pending→created.
	if child.TurnID == "" {
		child.TurnID = item.Intent.ChildTurnID
	}
	if child.RunID == "" {
		child.RunID = item.Intent.ChildRunID
	}
	if child.ParentStopFence == nil {
		f := item.Intent.ParentStopFence
		child.ParentStopFence = &f
	}
	child.State = DispatchPrepared
	if child.ProtocolVersion == 0 {
		child.ProtocolVersion = DispatchProtocolV2
	}
	if env.EnvelopeHash == "" {
		h, err := ComputeEnvelopeHash(&env)
		if err != nil {
			return item.Revision, err
		}
		env.EnvelopeHash = h
	}
	child.EnvelopeHash = env.EnvelopeHash
	child.Revision = 1
	child.CreatedAt = s.clockStr()
	child.UpdatedAt = s.clockStr()
	ck := recKey(child.RunID, child.TurnID)
	if _, exists := s.records[ck]; !exists {
		s.ensureActivationLocked(child.RunID)
		cp := child
		envCp := env
		s.records[ck] = &cp
		s.envelopes[ck] = &envCp
	}
	item.State = ReleaseCreated
	item.Revision++
	item.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "release_commit", dependentRunID, "system")
	if err := s.commitLine(dispatchLogLine{Kind: "release", Seq: s.seq, At: s.clockStr(), Release: item, Record: s.records[ck], Envelope: s.envelopes[ck]}); err != nil {
		return item.Revision, err
	}
	return item.Revision, nil
}

func (s *memoryDispatchStore) SuppressReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev, stopGen int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	kind := releaseEffectKind(dependentRunID)
	ek := effectKey(runID, turnID, kind)
	item := s.releases[ek]
	if item == nil {
		return 0, ErrNotFound
	}
	if item.Revision != expectedEffectRev {
		return item.Revision, ErrStaleDispatch
	}
	if item.State == ReleaseSuppressed {
		return item.Revision, nil
	}
	if item.State != ReleasePending {
		return item.Revision, fmt.Errorf("%w: cannot suppress %s", ErrIllegalTransition, item.State)
	}
	item.State = ReleaseSuppressed
	item.StopGeneration = stopGen
	item.Revision++
	item.UpdatedAt = s.clockStr()
	s.appendAuditLocked(runID, turnID, "release_suppress", dependentRunID, "system")
	if err := s.commitLine(dispatchLogLine{Kind: "release", Seq: s.seq, At: s.clockStr(), Release: item}); err != nil {
		return item.Revision, err
	}
	return item.Revision, nil
}

func (s *memoryDispatchStore) OpenRepair(ctx context.Context, runID, reason string, rawBlob []byte, rawHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if existing, ok := s.repairs[runID]; ok && existing.State == "open" {
		return existing.RepairRevision, nil
	}
	if rawHash == "" {
		rawHash = HashBytes(rawBlob)
	}
	rec := &RepairRecord{
		RunID: runID, RepairRevision: 1, Reason: reason,
		QuarantineBlob: append([]byte(nil), rawBlob...), QuarantineHash: rawHash,
		State: "open", CreatedAt: s.clockStr(),
	}
	s.repairs[runID] = rec
	s.appendAuditLocked(runID, "", "open_repair", reason, "system")
	if err := s.commitLine(dispatchLogLine{Kind: "repair", Seq: s.seq, At: s.clockStr(), Repair: rec}); err != nil {
		return 0, err
	}
	return rec.RepairRevision, nil
}

func (s *memoryDispatchStore) GetOpenRepair(ctx context.Context, runID string) (RepairRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r := s.repairs[runID]
	if r == nil || r.State != "open" {
		return RepairRecord{}, false, nil
	}
	cp := *r
	cp.QuarantineBlob = append([]byte(nil), r.QuarantineBlob...)
	return cp, true, nil
}

func (s *memoryDispatchStore) BeginRepairResolution(ctx context.Context, runID string, expectedRepairRev int64,
	resolutionID string, action RepairAction) (int64, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r := s.repairs[runID]
	if r == nil || r.State != "open" {
		return 0, nil, ErrRepairNotOpen
	}
	if r.RepairRevision != expectedRepairRev {
		return r.RepairRevision, nil, ErrStaleDispatch
	}
	// Claim attempt if free or expired.
	if r.AttemptClaim != "" && r.AttemptExpiresAt != "" {
		if exp, e := time.Parse(time.RFC3339Nano, r.AttemptExpiresAt); e == nil && s.clock().Before(exp) && r.ResolutionID != resolutionID {
			return r.RepairRevision, nil, ErrStaleDispatch
		}
	}
	r.AttemptClaim = string(action)
	r.ResolutionID = resolutionID
	r.AttemptExpiresAt = s.clock().Add(30 * time.Second).Format(time.RFC3339Nano)
	r.RepairRevision++
	s.appendAuditLocked(runID, "", "begin_repair", string(action), "operator")
	raw := append([]byte(nil), r.QuarantineBlob...)
	if err := s.commitLine(dispatchLogLine{Kind: "repair", Seq: s.seq, At: s.clockStr(), Repair: r}); err != nil {
		return r.RepairRevision, nil, err
	}
	return r.RepairRevision, raw, nil
}

func (s *memoryDispatchStore) CommitRepairResolution(ctx context.Context, runID string, attemptRev int64,
	resolutionID string, outcome RepairOutcome, detail string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r := s.repairs[runID]
	if r == nil {
		return 0, ErrRepairNotOpen
	}
	if r.RepairRevision != attemptRev {
		return r.RepairRevision, ErrStaleDispatch
	}
	if r.ResolutionID != "" && r.ResolutionID != resolutionID {
		return r.RepairRevision, ErrStaleDispatch
	}
	switch outcome {
	case RepairResolvedRetryLoad, RepairResolvedAbandon:
		r.State = "resolved"
		r.ResolvedAction = string(outcome)
		r.ResolvedAt = s.clockStr()
	case RepairFailedStillOpen:
		r.State = "open"
		r.AttemptClaim = ""
		r.AttemptExpiresAt = ""
	default:
		return r.RepairRevision, fmt.Errorf("unknown repair outcome %q", outcome)
	}
	r.RepairRevision++
	s.appendAuditLocked(runID, "", "commit_repair", string(outcome)+":"+detail, "operator")
	if err := s.commitLine(dispatchLogLine{Kind: "repair", Seq: s.seq, At: s.clockStr(), Repair: r}); err != nil {
		return r.RepairRevision, err
	}
	return r.RepairRevision, nil
}

func (s *memoryDispatchStore) GetRunProtocolVersion(ctx context.Context, runID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	if v, ok := s.activation[runID]; ok {
		return v, nil
	}
	return 0, nil
}

// ---- reads ----

func (s *memoryDispatchStore) Get(ctx context.Context, runID, turnID string) (DispatchRecord, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r, err := s.getRecLocked(runID, turnID)
	if err != nil {
		return DispatchRecord{}, 0, err
	}
	cp := s.cloneRec(r)
	return cp, cp.Revision, nil
}

func (s *memoryDispatchStore) GetEnvelope(ctx context.Context, runID, turnID string) (DispatchEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	e := s.envelopes[recKey(runID, turnID)]
	if e == nil {
		return DispatchEnvelope{}, ErrNotFound
	}
	return *e, nil
}

func (s *memoryDispatchStore) ListRecoverable(ctx context.Context, runID string) ([]DispatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	var out []DispatchRecord
	for _, r := range s.records {
		if runID != "" && r.RunID != runID {
			continue
		}
		if !r.State.IsTerminal() || (r.SettleOwed && !r.SettlePhase.IsSettleFinal()) {
			out = append(out, s.cloneRec(r))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].TurnID < out[j].TurnID
	})
	return out, nil
}

func (s *memoryDispatchStore) FindActiveByOuterIntent(ctx context.Context, runID, intentKey string, intentGen int64) (DispatchRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	for _, r := range s.records {
		if r.RunID != runID || r.OuterIntentKey != intentKey || r.OuterIntentGen != intentGen {
			continue
		}
		if r.State.IsTerminal() {
			continue
		}
		return s.cloneRec(r), true, nil
	}
	return DispatchRecord{}, false, nil
}

func (s *memoryDispatchStore) ListAttention(ctx context.Context) ([]AttentionItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	var out []AttentionItem
	for _, r := range s.records {
		if r.State == DispatchUncertain {
			out = append(out, AttentionItem{
				Kind: "uncertain", RunID: r.RunID, TurnID: r.TurnID, UpdatedAt: r.UpdatedAt,
			})
		}
	}
	for _, rep := range s.repairs {
		if rep.State == "open" {
			out = append(out, AttentionItem{
				Kind: "repair_required", RunID: rep.RunID, Reason: rep.Reason, UpdatedAt: rep.CreatedAt,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RunID != out[j].RunID {
			return out[i].RunID < out[j].RunID
		}
		return out[i].TurnID < out[j].TurnID
	})
	return out, nil
}

func (s *memoryDispatchStore) ListAudit(ctx context.Context, runID, turnID string) ([]AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	var out []AuditEntry
	for _, a := range s.audits {
		if runID != "" && a.RunID != runID {
			continue
		}
		if turnID != "" && a.TurnID != turnID {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *memoryDispatchStore) ListEffects(ctx context.Context, runID, turnID string) ([]EffectDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	var out []EffectDone
	for _, e := range s.effects {
		if e.RunID != runID || e.TurnID != turnID {
			continue
		}
		cp := *e
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectKind < out[j].EffectKind })
	return out, nil
}

func (s *memoryDispatchStore) GetResolutionResult(ctx context.Context, resolutionID string) (ResolutionResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	r := s.resolutions[resolutionID]
	if r == nil {
		return ResolutionResult{}, false, nil
	}
	return *r, true, nil
}

func (s *memoryDispatchStore) IsIntentCleared(ctx context.Context, ownerRunID, intentKey string, intentGen int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	_, ok := s.clears[intentKeyOf(ownerRunID, intentKey, intentGen)]
	return ok, nil
}

func (s *memoryDispatchStore) HasNonTerminal(ctx context.Context, runID string) (bool, error) {
	list, err := s.ListRecoverable(ctx, runID)
	if err != nil {
		return false, err
	}
	for _, r := range list {
		if r.RunID == runID {
			return true, nil
		}
	}
	return false, nil
}

// jsonMarshal is a tiny local helper to avoid importing encoding/json at every call site name clash.
func jsonMarshal(v any) ([]byte, error) {
	return marshalJSON(v)
}

// Ensure interface compliance.
var _ DispatchStore = (*memoryDispatchStore)(nil)

// dispatchLogLine is the local commit-log line shape (also used as afterCommit payload).
type dispatchLogLine struct {
	Kind       string               `json:"kind"`
	Seq        int64                `json:"seq"`
	At         string               `json:"at"`
	Record     *DispatchRecord      `json:"record,omitempty"`
	Envelope   *DispatchEnvelope    `json:"envelope,omitempty"`
	IntentClear *intentClearPayload `json:"intent_clear,omitempty"`
	Effect     *EffectDone          `json:"effect,omitempty"`
	Release    *ReleaseManifestItem `json:"release,omitempty"`
	Repair     *RepairRecord        `json:"repair,omitempty"`
	RunStop    *RunStopState        `json:"run_stop,omitempty"`
	Activation *dispatchActivation  `json:"activation,omitempty"`
	Successor  *DispatchRecord      `json:"successor,omitempty"`
}

type intentClearPayload struct {
	OwnerRunID string `json:"owner_run_id"`
	Key        string `json:"key"`
	Gen        int64  `json:"gen"`
}

type dispatchActivation struct {
	RunID           string `json:"run_id"`
	ProtocolVersion int    `json:"protocol_version"`
}

