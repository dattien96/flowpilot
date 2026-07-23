package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// DispatchV2EnvEnabled reports whether new runs use the durable V2 dispatch path.
// Default is ON (CP-51 product path). Opt out only with an explicit kill switch:
// FLOWPILOT_DISPATCH_V2=0|false|no|off. Once a run has V2 activation, the store
// remains authority even if the kill switch is set (SD-24 §9 — never reinterpret
// V2 records under V1 semantics; flag-off only blocks new V1-style accept of V2 runs).
func DispatchV2EnvEnabled() bool {
	v := strings.TrimSpace(os.Getenv("FLOWPILOT_DISPATCH_V2"))
	if v == "" {
		return true // default V2
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off", "disable", "disabled":
		return false
	default:
		// "1", "true", "yes", or any other value → V2 on
		return true
	}
}

// dispatchV2EnvEnabled is the unexported alias used inside the package.
func dispatchV2EnvEnabled() bool { return DispatchV2EnvEnabled() }

// dispatchV2ActiveForRun reports whether this run should use the V2 path.
func (s *InteractiveService) dispatchV2ActiveForRun(ctx context.Context, runID string) bool {
	if s == nil || s.dispatchStore == nil {
		return false
	}
	if ver, err := s.dispatchStore.GetRunProtocolVersion(ctx, runID); err == nil && ver >= DispatchProtocolV2 {
		return true
	}
	return dispatchV2EnvEnabled()
}

// providerV2Enabled reports whether a provider may enter V2 automated dispatch.
// Codex/Grok/Claude/fake: three-outcome only (no Accepted seam) by default — all
// three have recorded Task-257 evidence of unprovable-as-a-receipt ⇒ uncertain
// (see evidence/SD-24/{codex,grok,claude}.md).
// Gemini: disabled until its own Task-257 evidence, unless explicitly allow-listed
// via FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini (still no Accepted seam until evidence wires it).
func providerV2Enabled(key ProviderKey) bool {
	k := strings.ToLower(string(key))
	switch k {
	case "codex", "grok", "claude", "fake", "":
		return true
	case "gemini":
		return providerV2AllowListed(k)
	default:
		return true
	}
}

// providerV2AllowListed parses FLOWPILOT_DISPATCH_V2_PROVIDERS (comma-separated).
func providerV2AllowListed(key string) bool {
	raw := strings.TrimSpace(os.Getenv("FLOWPILOT_DISPATCH_V2_PROVIDERS"))
	if raw == "" {
		return false
	}
	for _, p := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(p), key) {
			return true
		}
	}
	return false
}

// buildDispatchEnvelope constructs the immutable redispatch payload at prepare.
func buildDispatchEnvelope(rs *interactiveRun, in TurnInput, turnID string) DispatchEnvelope {
	prompt := in.Prompt
	env := DispatchEnvelope{
		TurnID:                     turnID,
		RunID:                      rs.id,
		StepID:                     in.StepID,
		ProviderKey:                rs.providerKey,
		ProviderAccountID:          rs.providerAccountID,
		ProviderSessionIDAtPrepare: rs.providerSessionID,
		PromptRef:                  "turnLogKindPrompt:" + turnID,
		PromptSHA256:               HashBytes([]byte(prompt)),
		Model:                      rs.modelName,
		ReasoningEffort:            rs.reasoningEffort,
		Yolo:                       rs.yolo,
		SelectedSkills:             append([]SkillSelection(nil), in.SelectedSkills...),
		Scenario:                   "",
		FlowContextInjected:        rs.flowContextInjected,
		CreatedAt:                  nowRFC3339Nano(),
	}
	if in.Model != nil {
		env.Model = *in.Model
	}
	if in.ReasoningEffort != "" {
		env.ReasoningEffort = in.ReasoningEffort
	}
	if in.YoloMode != nil {
		env.Yolo = *in.YoloMode
	}
	h, _ := ComputeEnvelopeHash(&env)
	env.EnvelopeHash = h
	return env
}

func newDispatchRecord(rs *interactiveRun, turnID string, env DispatchEnvelope, intentKey string, intentGen int64, ownerRunID string) DispatchRecord {
	if ownerRunID == "" {
		ownerRunID = rs.id
	}
	runMode := rs.runKind
	if runMode == "" {
		runMode = "workflow"
	}
	return DispatchRecord{
		ProtocolVersion:  DispatchProtocolV2,
		TurnID:           turnID,
		RunID:            rs.id,
		ProjectID:        rs.projectID,
		IntentOwnerRunID: ownerRunID,
		State:            DispatchPrepared,
		EnvelopeHash:     env.EnvelopeHash,
		OuterIntentKey:   intentKey,
		OuterIntentGen:   intentGen,
		SettleOwed:       requiresGateSettlement(runMode, env.StepID),
		CreatedAt:        nowRFC3339Nano(),
		UpdatedAt:        nowRFC3339Nano(),
	}
}

// ensureDispatchStore provides a memory store when tests enable V2 without wiring.
func (s *InteractiveService) ensureDispatchStore() DispatchStore {
	if s.dispatchStore != nil {
		return s.dispatchStore
	}
	s.dispatchStore = NewMemoryDispatchStore()
	return s.dispatchStore
}

// dispatchIDCeiling is implemented by durable dispatch stores that can report the
// largest numeric id suffix among their persisted records, so SetDispatchStore can
// seed the id counter past them (BUG-317). Optional: both the in-memory test backend
// and the production multi-project store satisfy it; a store that does not is skipped.
type dispatchIDCeiling interface {
	MaxPersistedIDSuffix(ctx context.Context) (int64, error)
}

// SetDispatchStore wires the durable dispatch store (production + tests).
func (s *InteractiveService) SetDispatchStore(store DispatchStore) {
	s.mu.Lock()
	s.dispatchStore = store
	s.mu.Unlock()
	// BUG-317: the constructor's seedIDCounter runs BEFORE the dispatch store is
	// wired and only sees the session index. Persisted dispatch records are a second
	// source of ids handed out before a restart — a chat deleted from history leaves
	// its dispatch records behind — so without this seed the id counter can re-mint a
	// run/turn id a surviving record still owns, and CreatePrepared then rejects it
	// (ErrAlreadyExists -> dispatch_prepare_failed) on the next flow start.
	if store != nil {
		s.seedIDCounterFromDispatchStore(store)
	}
}

// seedIDCounterFromDispatchStore lifts the id counter above every persisted dispatch
// id (BUG-317). Best-effort: a store without a ceiling, or a read error, or an empty
// store leaves the counter unchanged. Complements BUG-117's session-index seed.
func (s *InteractiveService) seedIDCounterFromDispatchStore(store DispatchStore) {
	ceil, ok := store.(dispatchIDCeiling)
	if !ok {
		return
	}
	max, err := ceil.MaxPersistedIDSuffix(context.Background())
	if err != nil || max <= 0 {
		return
	}
	for {
		cur := s.idCounter.Load()
		if cur >= max {
			return
		}
		if s.idCounter.CompareAndSwap(cur, max) {
			log.Printf("[runner] id counter seeded to %d from durable dispatch records (BUG-317)", max)
			return
		}
	}
}

// prepareDispatchV2 creates the durable prepared record at the prep barrier.
func (s *InteractiveService) prepareDispatchV2(ctx context.Context, rs *interactiveRun, in TurnInput, turnID, idempotencyKey string) error {
	if !s.dispatchV2ActiveForRun(ctx, rs.id) {
		return nil
	}
	if !providerV2Enabled(rs.providerKey) {
		return fmt.Errorf("provider %s is V2-disabled pending Task-257 capability evidence", rs.providerKey)
	}
	store := s.ensureDispatchStore()
	env := buildDispatchEnvelope(rs, in, turnID)
	var intentGen int64
	if strings.HasPrefix(idempotencyKey, "durable-") {
		intentGen = durableIdempotencyKeyGen(idempotencyKey)
	}
	owner := rs.id
	// Parent-owned restart intents: key contains child run id.
	if strings.Contains(idempotencyKey, "-restart-") && rs.pendingRestartRunID != "" {
		// Intent lives on parent; the child run is the dispatch run when starting on child.
	}
	rec := newDispatchRecord(rs, turnID, env, idempotencyKey, intentGen, owner)
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		return err
	}
	if rs.dispatch == nil {
		rs.dispatch = map[string]*DispatchRecord{}
	}
	cp := rec
	cp.Revision = 1
	rs.dispatch[turnID] = &cp
	rs.dispatchProtocolVersion = DispatchProtocolV2
	return nil
}

// claimDispatchV2 advances prepared → send_claimed at the launch-ack barrier.
func (s *InteractiveService) claimDispatchV2(ctx context.Context, rs *interactiveRun, turnID string) error {
	if !s.dispatchV2ActiveForRun(ctx, rs.id) || s.dispatchStore == nil {
		return nil
	}
	rec := rs.dispatchByTurn(turnID)
	if rec == nil {
		got, rev, err := s.dispatchStore.Get(ctx, rs.id, turnID)
		if err != nil {
			return err
		}
		rec = &got
		rec.Revision = rev
	}
	if rec.State != DispatchPrepared {
		return nil
	}
	rev, err := s.dispatchStore.CASAdvance(ctx, rs.id, turnID, rec.Revision, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		return err
	}
	rec.State = DispatchSendClaimed
	rec.Revision = rev
	if rs.dispatch == nil {
		rs.dispatch = map[string]*DispatchRecord{}
	}
	cp := *rec
	rs.dispatch[turnID] = &cp
	return nil
}

// linearizeSendStarted is the Stop/send linearization point (SD-24 §7.2).
// Returns (ok, storeErr). ok=false means the caller must not send.
// storeErr=true means a NON-Stop store/CAS error (BUG-289 A1/F-6) — the turn
// must be terminalized rather than silently abandoned mid-RUNNING.
func (s *InteractiveService) linearizeSendStarted(ctx context.Context, rs *interactiveRun, turnID string) (ok bool, storeErr bool) {
	if s == nil || s.dispatchStore == nil || !s.dispatchV2ActiveForRun(ctx, rs.id) {
		return true, false // V1 path: no durable fence
	}
	rec := rs.dispatchByTurn(turnID)
	if rec == nil {
		got, rev, err := s.dispatchStore.Get(ctx, rs.id, turnID)
		if err != nil {
			log.Printf("[dispatch] linearize get failed run=%s turn=%s: %v", rs.id, turnID, err)
			return false, true
		}
		rec = &got
		rec.Revision = rev
	}
	if rec.State == DispatchSendStarted || rec.State == DispatchProviderAccepted || rec.State.IsTerminal() {
		return rec.State == DispatchSendStarted || rec.State == DispatchProviderAccepted, false
	}
	rev, err := s.dispatchStore.CASAdvance(ctx, rs.id, turnID, rec.Revision, DispatchSendClaimed, DispatchSendStarted, nil)
	if err == nil {
		rec.State = DispatchSendStarted
		rec.Revision = rev
		if rs.dispatch != nil {
			cp := *rec
			rs.dispatch[turnID] = &cp
		}
		return true, false
	}
	var ownStop ErrRunStopFence
	if errors.As(err, &ownStop) {
		s.commitPreSendCancel(ctx, rs.id, turnID, ownStop.CurrentGeneration, PreSendStopSelf)
		return false, false
	}
	var parentFence ErrParentStopFence
	if errors.As(err, &parentFence) {
		s.commitPreSendCancel(ctx, rs.id, turnID, parentFence.CurrentGeneration, PreSendStopParentFence)
		return false, false
	}
	// Reload and branch — never assume Stop won.
	cur, curRev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID)
	if gerr != nil {
		return false, true
	}
	switch {
	case cur.State == DispatchSendClaimed && cur.CancelRequested:
		s.commitPreSendCancel(ctx, rs.id, turnID, cur.StopGeneration, PreSendStopSelf)
		return false, false
	case cur.State == DispatchSendStarted || cur.State == DispatchProviderAccepted:
		if rs.dispatch != nil {
			cp := cur
			cp.Revision = curRev
			rs.dispatch[turnID] = &cp
		}
		return true, false
	default:
		// Unexpected state after CAS failure — treat as store/correctness error.
		log.Printf("[dispatch] linearize unexpected state run=%s turn=%s state=%s", rs.id, turnID, cur.State)
		return false, true
	}
}

func (s *InteractiveService) commitPreSendCancel(ctx context.Context, runID, turnID string, stopGen int64, source PreSendStopSource) {
	if s.dispatchStore == nil {
		return
	}
	cur, curRev, err := s.dispatchStore.Get(ctx, runID, turnID)
	if err != nil {
		return
	}
	if cur.State != DispatchPrepared && cur.State != DispatchSendClaimed {
		return
	}
	_, cerr := s.dispatchStore.CommitPreSendCancellationAndClearIntent(ctx, runID, turnID, curRev,
		cur.IntentOwnerRunID, cur.OuterIntentKey, cur.OuterIntentGen, stopGen, source)
	if cerr != nil {
		log.Printf("[dispatch] pre-send cancel failed run=%s turn=%s: %v", runID, turnID, cerr)
	}
}

// requestRunStopV2 advances durable RunStopState then cancels active records.
// It is a no-op (nil error) for runs without V2 activation (no dispatch
// records) so a live Stop of a never-dispatched run does not spuriously
// create activation state.
//
// Fail-closed (Codex review 2026-07-17): the durable RunStopState write is the
// actual INV-3 fence — a concurrent send's send_claimed→send_started CAS only
// sees Stop if this commit landed. Previously this function returned void and
// swallowed every store error, so a durable-store failure let the live Stop
// API still report success via RAM cancel alone, with no fence ever persisted.
// It now returns an error for any failure on the two calls that establish the
// fence (GetRunStopState, RequestRunStop); the caller must treat that as Stop
// NOT durably guaranteed and must not report unqualified success. The
// best-effort cancel-request sweep (ListRecoverable/SetCancelRequested) is a
// courtesy for already-claimed records — the CAS-level fence in
// linearizeSendStarted is what actually blocks a send, so a failure there is
// logged, not fatal.
// releaseHubStopFenceForFollowUp clears durable run_stop.Stopped so a hub plain-chat
// turn admitted after user Stop (BUG-308) can pass send_started linearization.
// Generation is not reset — children that saw the Stop fence stay invalid.
// No-op when dispatch V2 is inactive or the fence is already clear.
func (s *InteractiveService) releaseHubStopFenceForFollowUp(ctx context.Context, runID string) {
	if s == nil || s.dispatchStore == nil || runID == "" {
		return
	}
	if ver, err := s.dispatchStore.GetRunProtocolVersion(ctx, runID); err != nil || ver < DispatchProtocolV2 {
		return
	}
	st, err := s.dispatchStore.GetRunStopState(ctx, runID)
	if err != nil || !st.Stopped {
		return
	}
	if _, err := s.dispatchStore.ReleaseRunStopFence(ctx, runID, st.Revision); err != nil {
		// stale: one retry
		st2, err2 := s.dispatchStore.GetRunStopState(ctx, runID)
		if err2 != nil || !st2.Stopped {
			return
		}
		if _, err3 := s.dispatchStore.ReleaseRunStopFence(ctx, runID, st2.Revision); err3 != nil {
			log.Printf("[dispatch] ReleaseRunStopFence failed run=%s: %v (follow-up may hit stop fence)", runID, err3)
		}
	}
}

func (s *InteractiveService) requestRunStopV2(ctx context.Context, runID string) error {
	if s == nil || s.dispatchStore == nil || runID == "" {
		return nil
	}
	if ver, err := s.dispatchStore.GetRunProtocolVersion(ctx, runID); err != nil || ver < DispatchProtocolV2 {
		return nil
	}
	st, err := s.dispatchStore.GetRunStopState(ctx, runID)
	if err != nil {
		return fmt.Errorf("requestRunStopV2: GetRunStopState run=%s: %w", runID, err)
	}
	exp := st.Revision
	if exp == 0 {
		exp = 1
	}
	newSt, err := s.dispatchStore.RequestRunStop(ctx, runID, exp, StopReasonUser)
	if err != nil {
		// stale: reload once
		st, err = s.dispatchStore.GetRunStopState(ctx, runID)
		if err != nil {
			return fmt.Errorf("requestRunStopV2: GetRunStopState (retry) run=%s: %w", runID, err)
		}
		newSt, err = s.dispatchStore.RequestRunStop(ctx, runID, st.Revision, StopReasonUser)
		if err != nil {
			return fmt.Errorf("requestRunStopV2: RequestRunStop run=%s: %w", runID, err)
		}
	}
	list, err := s.dispatchStore.ListRecoverable(ctx, runID)
	if err != nil {
		log.Printf("[dispatch] ListRecoverable failed after durable stop run=%s: %v (fence still durable; cancel-request sweep skipped)", runID, err)
		return nil
	}
	for _, r := range list {
		if r.State.IsTerminal() {
			continue
		}
		if _, err := s.dispatchStore.SetCancelRequested(ctx, runID, r.TurnID, r.Revision, newSt.Generation); err != nil {
			log.Printf("[dispatch] SetCancelRequested failed run=%s turn=%s: %v (fence still durable)", runID, r.TurnID, err)
		}
	}
	return nil
}

func (rs *interactiveRun) dispatchByTurn(turnID string) *DispatchRecord {
	if rs == nil || rs.dispatch == nil {
		return nil
	}
	return rs.dispatch[turnID]
}

// ---- TurnBridge receipt / terminal seams (SD-24 §6.3a) ----

// Accepted is the sole caller of CommitReceiptAndClearIntent. No current
// Codex/Grok/Claude adapter calls this (Task-257 unprovable); future providers
// only after evidence-backed Task-257 rows.
func (b *turnBridge) Accepted(receipt ReceiptEvidence) {
	if b == nil || b.svc == nil || b.svc.dispatchStore == nil {
		return
	}
	s := b.svc
	rs := b.rs
	turnID := b.turnID
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	r := rs.dispatchByTurn(turnID)
	if r == nil {
		got, rev, err := s.dispatchStore.Get(ctx, rs.id, turnID)
		if err != nil {
			return
		}
		r = &got
		r.Revision = rev
	}
	if r.State != DispatchSendStarted && r.State != DispatchProviderAccepted {
		log.Printf("[dispatch] Accepted rejected: state=%s run=%s turn=%s", r.State, rs.id, turnID)
		return
	}
	_, err := s.dispatchStore.CommitReceiptAndClearIntent(ctx, rs.id, turnID, r.Revision,
		receipt, r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen)
	if err != nil {
		if errors.Is(err, ErrReceiptConflict) {
			log.Printf("[dispatch] receipt conflict run=%s turn=%s: %v", rs.id, turnID, err)
			return
		}
		log.Printf("[dispatch] receipt commit failed run=%s turn=%s: %v (intent held)", rs.id, turnID, err)
		s.scheduleDispatchCommitRetry(rs.id, turnID)
		return
	}
	// Refresh cache
	if got, rev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID); gerr == nil {
		cp := got
		cp.Revision = rev
		if rs.dispatch == nil {
			rs.dispatch = map[string]*DispatchRecord{}
		}
		rs.dispatch[turnID] = &cp
	}
}

// Terminal is the sole automatic caller of CommitTerminalAndSettleIntent.
func (b *turnBridge) Terminal(proof TerminalEvidence) {
	if b == nil || b.svc == nil || b.svc.dispatchStore == nil {
		return
	}
	s := b.svc
	rs := b.rs
	turnID := b.turnID
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	r := rs.dispatchByTurn(turnID)
	if r == nil {
		got, rev, err := s.dispatchStore.Get(ctx, rs.id, turnID)
		if err != nil {
			return
		}
		r = &got
		r.Revision = rev
	}
	if r.State.IsTerminal() {
		return
	}
	_, err := s.dispatchStore.CommitTerminalAndSettleIntent(ctx, rs.id, turnID, r.Revision,
		proof, r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen)
	if err != nil {
		log.Printf("[dispatch] terminal commit failed run=%s turn=%s: %v", rs.id, turnID, err)
		s.scheduleDispatchCommitRetry(rs.id, turnID)
		return
	}
	if got, rev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID); gerr == nil {
		cp := got
		cp.Revision = rev
		if rs.dispatch == nil {
			rs.dispatch = map[string]*DispatchRecord{}
		}
		rs.dispatch[turnID] = &cp
	}
	// Task-251: advance settle phases when gate is not still pending.
	s.maybeScheduleSettleAfterTerminal(rs.id, turnID)
}

func (s *InteractiveService) recordDispatchTransportError(ctx context.Context, runID, turnID string, err error) {
	if s == nil || s.dispatchStore == nil || err == nil {
		return
	}
	payload := []byte(err.Error())
	_, _ = s.dispatchStore.RecordEffectDone(ctx, runID, turnID, "transport_error", payload, HashBytes(payload))
}

func (s *InteractiveService) scheduleDispatchReconcile(runID, turnID string) {
	// Task-250 owns the full scanner; enqueue a best-effort wake.
	go s.reconcileDispatchTurn(context.Background(), runID, turnID)
}

func (s *InteractiveService) scheduleDispatchCommitRetry(runID, turnID string) {
	go s.reconcileDispatchTurn(context.Background(), runID, turnID)
}

// pending commit retries (in-process).
var dispatchRetryMu sync.Mutex
var dispatchRetryQueued = map[string]bool{}

func (s *InteractiveService) reconcileDispatchTurn(ctx context.Context, runID, turnID string) {
	key := runID + "/" + turnID
	dispatchRetryMu.Lock()
	if dispatchRetryQueued[key] {
		dispatchRetryMu.Unlock()
		return
	}
	dispatchRetryQueued[key] = true
	dispatchRetryMu.Unlock()
	defer func() {
		dispatchRetryMu.Lock()
		delete(dispatchRetryQueued, key)
		dispatchRetryMu.Unlock()
	}()
	if s == nil || s.dispatchStore == nil {
		return
	}
	// Leave recovery scanner (Task-250) to classify; this is a wake marker only.
	_, _, _ = s.dispatchStore.Get(ctx, runID, turnID)
}

// ScanDispatchRecoveryOnBoot runs the CP-51 recovery scanner over every
// non-terminal dispatch record at server startup (Task-250 T-6 wiring).
// Best-effort, mirrors the existing ScanPersistedChatsForSummaries boot-pass
// pattern (chat_summary.go) — call as `go interactive.ScanDispatchRecoveryOnBoot(ctx)`
// right after SetDispatchStore in the server bootstrap.
//
// Scope (Codex review 2026-07-17): this closes the boot-wiring + enumeration
// gaps — every non-terminal record across every run is now visited
// (ScanAllRecoverable no longer misses prepared/send_claimed/send_started), and
// prepared/send_claimed records with no pending cancel trigger a real
// reconstruct+redispatch via ensureLiveAndRedispatch/flushDurableTurnIntents
// instead of only being labeled "safely-retryable" with no action taken. What
// remains out of scope (still tracked as NOT DONE in Task-250): live provider
// reconciliation for send_started/provider_accepted — no adapter exposes a
// query-by-operation-id, so those records correctly fall through to
// CommitRecoveryUnknownOrRequireCancel (uncertain/cancel-required), matching
// the Task-257 evidenced guarantee class for every enabled provider.
func (s *InteractiveService) ScanDispatchRecoveryOnBoot(ctx context.Context) {
	if s == nil || s.dispatchStore == nil {
		return
	}
	sc := &RecoveryScanner{
		Store:                   s.dispatchStore,
		Owner:                   "boot-recovery-scanner",
		EnsureLiveAndRedispatch: s.ensureLiveAndRedispatch,
	}
	if err := sc.ScanAllRecoverable(ctx); err != nil {
		log.Printf("[dispatch-recovery] boot scan failed: %v", err)
	}
	// Task-251 T-1/T-3: production settle drive with EvaluateGate (fail-closed
	// when gate still pending — schedules resumePendingFlowGate instead of
	// default-allow). RetrySettleWithBackoff is the sole boot settle driver.
	s.drivePendingSettlesOnBoot(ctx)
}

// drivePendingSettlesOnBoot walks terminal+settle_owed records and drives
// SettleDriver with production EvaluateGate (Task-251). Gate-pending turns
// are handed to resumePendingFlowGate; others RetrySettleWithBackoff.
func (s *InteractiveService) drivePendingSettlesOnBoot(ctx context.Context) {
	if s == nil || s.dispatchStore == nil {
		return
	}
	list, err := s.dispatchStore.ListRecoverable(ctx, "")
	if err != nil {
		log.Printf("[dispatch-settle] boot list recoverable: %v", err)
		return
	}
	n := 0
	for _, rec := range list {
		if !rec.State.IsTerminal() || !rec.SettleOwed || rec.SettlePhase.IsSettleFinal() {
			continue
		}
		n++
		// Prefer reconstruct so evaluateSettleGate sees pendingFlowGateSettle.
		s.mu.Lock()
		_, live := s.runs[rec.RunID]
		s.mu.Unlock()
		if !live {
			if _, apiErr := s.loadPersistedRun(rec.RunID); apiErr != nil {
				log.Printf("[dispatch-settle] boot reconstruct run=%s: %v (will still attempt settle)", rec.RunID, apiErr)
			}
		}
		s.mu.Lock()
		pendingGate := false
		if rs := s.runs[rec.RunID]; rs != nil {
			pendingGate = rs.pendingFlowGateSettle
		}
		s.mu.Unlock()
		if pendingGate {
			log.Printf("[dispatch-settle] boot resume gate first run=%s turn=%s", rec.RunID, rec.TurnID)
			go s.resumePendingFlowGate(rec.RunID)
			continue
		}
		s.scheduleSettleDrive(rec.RunID, rec.TurnID)
	}
	if n > 0 {
		log.Printf("[dispatch-settle] boot drive: %d terminal+settle_owed queued", n)
	}
}

// ensureLiveAndRedispatch reconstructs runID into RAM if it is not already
// live, then flushes its durable turn intent — the existing mechanism
// (flushDurableTurnIntents / startTurnClearingIntent) that redrives startTurn
// with the run's own durable idempotency key. startTurn's own duplicate-key
// handling (durableIdemReplaySafe / preparedReuseTurnID) is what actually
// re-enters the launch path with the SAME TurnID once envelope/state agree —
// this function's job is only to get the run into RAM and call that channel.
func (s *InteractiveService) ensureLiveAndRedispatch(ctx context.Context, runID string) {
	_ = ctx
	if s == nil || runID == "" {
		return
	}
	s.mu.Lock()
	_, live := s.runs[runID]
	s.mu.Unlock()
	if !live {
		if _, apiErr := s.loadPersistedRun(runID); apiErr != nil {
			log.Printf("[dispatch-recovery] reconstruct run=%s: %v", runID, apiErr)
			return
		}
	}
	s.flushDurableTurnIntents(runID)
}
