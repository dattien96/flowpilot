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
// Codex/Grok/fake: three-outcome only (no Accepted seam) by default.
// Claude/Gemini: disabled until Task-257 evidence, unless explicitly allow-listed
// via FLOWPILOT_DISPATCH_V2_PROVIDERS=claude,gemini (still no Accepted seam until evidence wires it).
func providerV2Enabled(key ProviderKey) bool {
	k := strings.ToLower(string(key))
	switch k {
	case "codex", "grok", "fake", "":
		return true
	case "claude", "gemini":
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

// SetDispatchStore wires the durable dispatch store (production + tests).
func (s *InteractiveService) SetDispatchStore(store DispatchStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatchStore = store
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
// Returns false if the caller must not send (Stop won or concurrent advance).
func (s *InteractiveService) linearizeSendStarted(ctx context.Context, rs *interactiveRun, turnID string) bool {
	if s == nil || s.dispatchStore == nil || !s.dispatchV2ActiveForRun(ctx, rs.id) {
		return true // V1 path: no durable fence
	}
	rec := rs.dispatchByTurn(turnID)
	if rec == nil {
		got, rev, err := s.dispatchStore.Get(ctx, rs.id, turnID)
		if err != nil {
			log.Printf("[dispatch] linearize get failed run=%s turn=%s: %v", rs.id, turnID, err)
			return false
		}
		rec = &got
		rec.Revision = rev
	}
	if rec.State == DispatchSendStarted || rec.State == DispatchProviderAccepted || rec.State.IsTerminal() {
		return rec.State == DispatchSendStarted || rec.State == DispatchProviderAccepted
	}
	rev, err := s.dispatchStore.CASAdvance(ctx, rs.id, turnID, rec.Revision, DispatchSendClaimed, DispatchSendStarted, nil)
	if err == nil {
		rec.State = DispatchSendStarted
		rec.Revision = rev
		if rs.dispatch != nil {
			cp := *rec
			rs.dispatch[turnID] = &cp
		}
		return true
	}
	var ownStop ErrRunStopFence
	if errors.As(err, &ownStop) {
		s.commitPreSendCancel(ctx, rs.id, turnID, ownStop.CurrentGeneration, PreSendStopSelf)
		return false
	}
	var parentFence ErrParentStopFence
	if errors.As(err, &parentFence) {
		s.commitPreSendCancel(ctx, rs.id, turnID, parentFence.CurrentGeneration, PreSendStopParentFence)
		return false
	}
	// Reload and branch — never assume Stop won.
	cur, curRev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID)
	if gerr != nil {
		return false
	}
	switch {
	case cur.State == DispatchSendClaimed && cur.CancelRequested:
		s.commitPreSendCancel(ctx, rs.id, turnID, cur.StopGeneration, PreSendStopSelf)
		return false
	case cur.State == DispatchSendStarted || cur.State == DispatchProviderAccepted:
		if rs.dispatch != nil {
			cp := cur
			cp.Revision = curRev
			rs.dispatch[turnID] = &cp
		}
		return true
	default:
		return false
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
func (s *InteractiveService) requestRunStopV2(ctx context.Context, runID string) {
	if s == nil || s.dispatchStore == nil {
		return
	}
	st, err := s.dispatchStore.GetRunStopState(ctx, runID)
	if err != nil {
		return
	}
	exp := st.Revision
	if exp == 0 {
		exp = 1
	}
	newSt, err := s.dispatchStore.RequestRunStop(ctx, runID, exp, StopReasonUser)
	if err != nil {
		// stale: reload once
		st, _ = s.dispatchStore.GetRunStopState(ctx, runID)
		newSt, err = s.dispatchStore.RequestRunStop(ctx, runID, st.Revision, StopReasonUser)
		if err != nil {
			log.Printf("[dispatch] RequestRunStop failed run=%s: %v", runID, err)
			return
		}
	}
	list, _ := s.dispatchStore.ListRecoverable(ctx, runID)
	for _, r := range list {
		if r.State.IsTerminal() {
			continue
		}
		_, _ = s.dispatchStore.SetCancelRequested(ctx, runID, r.TurnID, r.Revision, newSt.Generation)
	}
}

func (rs *interactiveRun) dispatchByTurn(turnID string) *DispatchRecord {
	if rs == nil || rs.dispatch == nil {
		return nil
	}
	return rs.dispatch[turnID]
}

// ---- TurnBridge receipt / terminal seams (SD-24 §6.3a) ----

// Accepted is the sole caller of CommitReceiptAndClearIntent. No current
// Codex/Grok adapter calls this (Task-257 unprovable); future providers only
// after evidence-backed Task-257 rows.
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
