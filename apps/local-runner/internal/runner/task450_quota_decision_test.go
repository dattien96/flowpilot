package runner

import (
	"context"
	"testing"
	"time"
)

// Task-450 (CP-87 P-6): the quota gate card carries a structured candidate
// table (event + durable question state + decision projection + snapshot
// view), the committed event records policy/headroom/cooldown evidence, and
// the quota-audit endpoint correlates requested→resolved route with usage.

// TestTask450_GateCardCarriesStructuredCandidates: the emitted
// user_question_required event, the pending-question snapshot, and the
// persisted ProviderQuestionState all carry the same candidate table.
func TestTask450_GateCardCarriesStructuredCandidates(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	// Live event carries the structured decision.
	evs := task449Events(s, rs.id, EventUserQuestionRequired)
	if len(evs) != 1 || evs[0].QuotaDecision == nil {
		t.Fatalf("question event must carry QuotaDecision, got %+v", evs)
	}
	dec := evs[0].QuotaDecision
	if dec.ProviderKey != ProviderKeyCodex || dec.AccountID != "cx-0" {
		t.Fatalf("decision binding = %s/%s", dec.ProviderKey, dec.AccountID)
	}
	if len(dec.Candidates) == 0 {
		t.Fatal("decision must carry candidate rows")
	}
	// The healthy alternate is present with headroom evidence.
	var alt *QuotaRouteCandidateDTO
	for i := range dec.Candidates {
		if dec.Candidates[i].AccountID == "cx-1" {
			alt = &dec.Candidates[i]
		}
	}
	if alt == nil || alt.Headroom.RemainingPercent == nil || *alt.Headroom.RemainingPercent != 90 {
		t.Fatalf("cx-1 row missing or headroom wrong: %+v", alt)
	}
	// Persisted question state carries it (restart/replay source of truth).
	states, err := s.workflowStore.(interface {
		ListQuestionsByRun(ctx context.Context, runID string) ([]ProviderQuestionState, error)
	}).ListQuestionsByRun(context.Background(), rs.id)
	if err != nil || len(states) != 1 {
		t.Fatalf("persisted questions: %v len=%d", err, len(states))
	}
	if states[0].QuotaDecision == nil || len(states[0].QuotaDecision.Candidates) != len(dec.Candidates) {
		t.Fatalf("persisted question must carry identical decision: %+v", states[0].QuotaDecision)
	}
	// Decision-payload projection (attention inbox) exposes it too.
	s.mu.Lock()
	payloads := s.decisionPayloadsForRunLocked(rs)
	s.mu.Unlock()
	var qd *QuotaRouteDecision
	for _, p := range payloads {
		if p.Question != nil && p.Question.Quota != nil {
			qd = p.Question.Quota
		}
	}
	if qd == nil || len(qd.Candidates) != len(dec.Candidates) {
		t.Fatal("decision payload must project the quota table")
	}
}

// TestTask450_AuditCorrelatesRequestedResolvedAndUsage: after a committed
// rotation plus a usage event, the audit record reports from/to bindings,
// policy, headroom evidence, and the CP-86 usage figures.
func TestTask450_AuditCorrelatesRequestedResolvedAndUsage(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	// Actual usage lands on the run's durable stream.
	s.mu.Lock()
	s.emitLocked(rs, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Total:           &TokenUsageBreakdown{TotalTokens: 4321},
			EstPromptTokens: ptrInt64(2048),
		},
	})
	s.mu.Unlock()

	rec, err := s.quotaAuditForRun(context.Background(), rs.id)
	if err != nil {
		t.Fatalf("quotaAuditForRun: %v", err)
	}
	if rec.Outcome != "committed" {
		t.Fatalf("outcome = %q", rec.Outcome)
	}
	if rec.FromAccount != "cx-0" || rec.ToAccount != "cx-1" || rec.ToProvider != ProviderKeyCodex {
		t.Fatalf("route correlation wrong: %+v", rec)
	}
	if rec.PolicyVersion == 0 {
		t.Fatal("policy version must be recorded")
	}
	if rec.Headroom == nil || rec.Headroom.RemainingPercent == nil || *rec.Headroom.RemainingPercent != 90 {
		t.Fatalf("headroom evidence missing: %+v", rec.Headroom)
	}
	if rec.ActualUsage == nil || rec.ActualUsage.TotalTokens != 4321 {
		t.Fatalf("actual usage not correlated: %+v", rec.ActualUsage)
	}
	if rec.EstPromptTokens == nil || *rec.EstPromptTokens != 2048 {
		t.Fatalf("est prompt not correlated: %+v", rec.EstPromptTokens)
	}
	// The committed event itself carries the cooldown window minted by the
	// claim (server timestamps — the clients' countdown source).
	evs := task449Events(s, rs.id, EventQuotaRouteCommitted)
	if len(evs) != 1 || evs[0].QuotaRoute == nil {
		t.Fatalf("committed event missing: %v", evs)
	}
	if evs[0].QuotaRoute.CooldownUntil == "" || evs[0].QuotaRoute.CooldownReason != QuotaCooldownReason {
		t.Fatalf("committed event must carry the cooldown window: %+v", evs[0].QuotaRoute)
	}
}

func ptrInt64(v int64) *int64 { return &v }

// TestTask450_RehydratedCardRestoresQuotaDecision: a pending quota card
// rebuilt from the durable store carries the same candidate table.
func TestTask450_RehydratedCardRestoresQuotaDecision(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	// Simulate restart: drop the in-memory question record, keep the durable
	// state, then rehydrate as resume does.
	s.mu.Lock()
	for id, rec := range s.questions {
		if rec.runID == rs.id {
			delete(s.questions, id)
			rs.pendingQuestionID = ""
			rs.status = RunStatusIdle
		}
	}
	s.mu.Unlock()
	s.mu.Lock()
	s.rehydratePendingGatesLocked(rs.id)
	s.mu.Unlock()
	qs := task449Questions(s, rs.id)
	if len(qs) != 1 {
		t.Fatalf("rehydrated pending card missing: %d", len(qs))
	}
	if qs[0].quotaDecision == nil || len(qs[0].quotaDecision.Candidates) == 0 {
		t.Fatal("rehydrated card lost the structured decision")
	}
	if questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatal("rehydrated card lost its kind")
	}
}
