package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/workingmode"
)

// Task-443 (CP-86 P-4): flag-gated context-pressure ladder + provider
// self-compaction detection. Ratio = Last.TotalTokens / ModelContextWindow
// (only when both known). Compaction = >30% TotalTokens drop WITHIN one leg.
// rotate_leg is a context reset on the SAME provider+account+model binding —
// leg lifecycle, not routing; it is offered only on long-lived root sessions
// (per-step child runs already get a fresh window on the next step).

func task443UsageEvent(window int64, last, total int64) ProviderEvent {
	return ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last:  &TokenUsageBreakdown{TotalTokens: last},
			Total: &TokenUsageBreakdown{TotalTokens: total},
			ModelContextWindow: &window,
		},
	}
}

func task443EventsOfType(rs *interactiveRun, typ ProviderEventType) []ProviderEvent {
	var out []ProviderEvent
	for _, e := range rs.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func task443PendingPressureQuestion(svc *InteractiveService, runID string) *questionRecord {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, rec := range svc.questions {
		if rec.runID == runID && rec.kind == contextPressureQuestionKind && rec.status == "pending" {
			return rec
		}
	}
	return nil
}

func task443NewHubRun(t *testing.T) (*InteractiveService, *interactiveRun) {
	t.Helper()
	t.Setenv(contextPressureEnvFlag, "1")
	svc, _ := newTestServer(t)
	svc.questionTTL = time.Hour
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	svc.mu.Unlock()
	return svc, rs
}

func TestTask443_FlagOff_NoPressureEvents(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	// Flag off must be byte-identical: nothing evaluated, nothing emitted.
	// (t.Setenv inside task443NewHubRun turned it on — clear it here.)
	t.Setenv(contextPressureEnvFlag, "")
	svc.emitLocked(rs, task443UsageEvent(200000, 195000, 195000))
	if got := task443EventsOfType(rs, EventContextPressure); len(got) != 0 {
		t.Fatalf("flag off emitted %d context_pressure events", len(got))
	}
	if got := task443EventsOfType(rs, EventProviderCompacted); len(got) != 0 {
		t.Fatalf("flag off emitted %d provider_compacted events", len(got))
	}
}

func TestTask443_Tier80_EmitsAwarenessOnly(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 170000, 170000)) // 85%
	svc.mu.Unlock()
	evs := task443EventsOfType(rs, EventContextPressure)
	if len(evs) != 1 {
		t.Fatalf("context_pressure events = %d, want 1", len(evs))
	}
	if evs[0].ContextPressure == nil || evs[0].ContextPressure.Tier != "aware" {
		t.Fatalf("tier = %+v, want aware", evs[0].ContextPressure)
	}
	if task443PendingPressureQuestion(svc, rs.id) != nil {
		t.Fatal("aware tier must never emit an ask card")
	}
}

func TestTask443_Tier90_EmitsAskUserCard(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 185000, 185000)) // 92.5%
	svc.mu.Unlock()
	evs := task443EventsOfType(rs, EventContextPressure)
	if len(evs) != 1 || evs[0].ContextPressure == nil || evs[0].ContextPressure.Tier != "ask" {
		t.Fatalf("ask tier events = %+v", evs)
	}
	rec := task443PendingPressureQuestion(svc, rs.id)
	if rec == nil {
		t.Fatal("≥90% must emit the context_pressure_90 card")
	}
	labels := task442OptionLabels(rec)
	// Root hub session: rotate_leg reachable → rotate_leg/continue/stop.
	want := []string{"rotate_leg", "continue", "stop"}
	for i, w := range want {
		if i >= len(labels) || labels[i] != w {
			t.Fatalf("options = %v, want %v", labels, want)
		}
	}
}

func TestTask443_TierFiresOncePerLeg(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	for i := 0; i < 3; i++ {
		svc.emitLocked(rs, task443UsageEvent(200000, 170000, 170000))
	}
	svc.mu.Unlock()
	if got := task443EventsOfType(rs, EventContextPressure); len(got) != 1 {
		t.Fatalf("aware tier fired %d times on one leg, want 1", len(got))
	}
}

func TestTask443_UnknownWindow_Silent(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last:  &TokenUsageBreakdown{TotalTokens: 999999},
			Total: &TokenUsageBreakdown{TotalTokens: 999999},
			// ModelContextWindow nil — unknown stays silent, never guessed.
		},
	})
	svc.mu.Unlock()
	if got := task443EventsOfType(rs, EventContextPressure); len(got) != 0 {
		t.Fatalf("unknown window emitted %d pressure events", len(got))
	}
}

func TestTask443_TokenDropSameLeg_EmitsProviderCompacted(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 190000, 190000))
	svc.emitLocked(rs, task443UsageEvent(200000, 18000, 18000)) // -90% same leg
	svc.mu.Unlock()
	evs := task443EventsOfType(rs, EventProviderCompacted)
	if len(evs) != 1 {
		t.Fatalf("provider_compacted events = %d, want 1", len(evs))
	}
	cp := evs[0].ContextPressure
	if cp == nil || cp.PrevTokens != 190000 || cp.UsedTokens != 18000 {
		t.Fatalf("compaction payload = %+v", cp)
	}
}

func TestTask443_PostCompaction_LegMarkedDegraded(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 190000, 190000))
	svc.emitLocked(rs, task443UsageEvent(200000, 18000, 18000))
	degraded := rs.contextDegradedLegs[rs.providerSessionID]
	svc.mu.Unlock()
	if !degraded {
		t.Fatal("compacted leg must carry context_degraded for next-admission handling")
	}
}

func TestTask443_RotateLegKeepsSameBinding(t *testing.T) {
	t.Setenv(contextPressureEnvFlag, "1")
	svc, _ := newSwitchTestService(t)
	svc.questionTTL = time.Hour
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	rs.modelName = "gpt-5.3-codex"
	svc.emitLocked(rs, task443UsageEvent(200000, 185000, 185000))
	svc.mu.Unlock()
	rec := task443PendingPressureQuestion(svc, rs.id)
	if rec == nil {
		t.Fatal("no pressure card")
	}
	oldChatID := rs.chatID
	oldSession := rs.providerSessionID
	if e := svc.AnswerQuestion(rec.id, []string{"rotate_leg"}); e != nil {
		t.Fatalf("AnswerQuestion rotate_leg: %v", e)
	}
	// The pending reset intent is committed durably and consumed at the next
	// admission — drive that boundary directly.
	newLeg := svc.consumePendingContextReset(context.Background(), rs.id)
	if newLeg == "" {
		t.Fatal("rotate_leg consumed nothing — expected a new same-binding leg")
	}
	svc.mu.Lock()
	nl := svc.runs[newLeg]
	svc.mu.Unlock()
	if nl == nil {
		t.Fatal("new leg run missing")
	}
	if nl.providerKey != ProviderKeyCodex || nl.modelName != "gpt-5.3-codex" {
		t.Fatalf("new leg binding changed: provider=%s model=%s", nl.providerKey, nl.modelName)
	}
	if nl.chatID != oldChatID || nl.legSeq != rs.legSeq+1 || nl.switchFromRunID != rs.id {
		t.Fatalf("new leg lineage wrong: chat=%s legSeq=%d from=%s", nl.chatID, nl.legSeq, nl.switchFromRunID)
	}
	if nl.providerSessionID == oldSession || nl.providerSessionID == "" {
		t.Fatal("new leg must carry a fresh provider session")
	}
	svc.mu.Lock()
	oldClosed := rs.legState
	svc.mu.Unlock()
	if oldClosed != LegStateClosed {
		t.Fatalf("old leg state = %q, want closed after rotation", oldClosed)
	}
}

// A provider-compacted leg is context-degraded: the NEXT admission boundary
// treats it like the ask tier and offers rotate_leg (spec: degraded → treat
// as ≥90%). The offer is once per leg — a user who chose continue is not
// re-nagged on every subsequent turn admission.
func TestTask443_DegradedLeg_OfferedRotateAtNextAdmission(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	// Keep Last below the aware threshold so no ask card fires — isolate the
	// degraded-leg offer path. Total drops 190k→18k within the leg = compacted.
	svc.emitLocked(rs, task443UsageEvent(200000, 100000, 190000))
	svc.emitLocked(rs, task443UsageEvent(200000, 50000, 18000))
	svc.mu.Unlock()
	if task443PendingPressureQuestion(svc, rs.id) != nil {
		t.Fatal("compaction alone must not emit the ask card mid-turn")
	}
	svc.mu.Lock()
	svc.maybeOfferContextResetAtAdmissionLocked(rs)
	svc.mu.Unlock()
	rec := task443PendingPressureQuestion(svc, rs.id)
	if rec == nil {
		t.Fatal("degraded leg must offer the rotate_leg card at next admission")
	}
	var hasRotate bool
	for _, o := range rec.options {
		if o.Label == "rotate_leg" {
			hasRotate = true
		}
	}
	if !hasRotate {
		t.Fatalf("degraded-leg card missing rotate_leg option: %+v", rec.options)
	}
	// Resolve as continue → the same leg must not be re-offered next admission.
	if err := svc.AnswerQuestion(rec.id, []string{"continue"}); err != nil {
		t.Fatalf("answer continue: %v", err)
	}
	svc.mu.Lock()
	svc.maybeOfferContextResetAtAdmissionLocked(rs)
	svc.mu.Unlock()
	if task443PendingPressureQuestion(svc, rs.id) != nil {
		t.Fatal("continue on a degraded leg must not re-offer rotate_leg every admission")
	}
}

func TestTask443_EventsPersistedToNdjson(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 190000, 190000))
	svc.emitLocked(rs, task443UsageEvent(200000, 18000, 18000))
	svc.mu.Unlock()
	// emitLocked's persistEvent path appends to the durable store — the fake
	// store's event map stands in for the ndjson log.
	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatal("test service must carry fakeWorkflowStore")
	}
	store.mu.Lock()
	var sawPressure, sawCompacted bool
	for _, e := range store.events[rs.id] {
		if e.Type == EventContextPressure {
			sawPressure = true
		}
		if e.Type == EventProviderCompacted {
			sawCompacted = true
		}
	}
	store.mu.Unlock()
	if !sawPressure || !sawCompacted {
		t.Fatalf("persisted events missing: pressure=%v compacted=%v", sawPressure, sawCompacted)
	}
}

func TestTask443_TokenDropAcrossLeg_NotCompacted(t *testing.T) {
	svc, rs := task443NewHubRun(t)
	svc.mu.Lock()
	rs.providerSessionID = "leg-a"
	svc.emitLocked(rs, task443UsageEvent(200000, 190000, 190000))
	rs.providerSessionID = "leg-b" // rotation → new session restarts counters
	svc.emitLocked(rs, task443UsageEvent(200000, 18000, 18000))
	svc.mu.Unlock()
	if got := task443EventsOfType(rs, EventProviderCompacted); len(got) != 0 {
		t.Fatalf("cross-leg drop emitted %d compaction events — normal reset must not flag", len(got))
	}
}

func TestTask443_ChildRun_RotateLegSuppressed(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	t.Setenv(contextPressureEnvFlag, "1")
	svc.mu.Lock()
	svc.emitLocked(child, task443UsageEvent(200000, 185000, 185000))
	svc.mu.Unlock()
	rec := task443PendingPressureQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("child node ≥90% must still emit the card")
	}
	for _, l := range task442OptionLabels(rec) {
		if l == "rotate_leg" {
			t.Fatal("rotate_leg on a per-step child is a no-op — next step gets a fresh window anyway")
		}
	}
}

func TestTask443_ClaudeCodexGrok_Parity(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		svc, rs := task443NewHubRun(t)
		svc.mu.Lock()
		rs.providerKey = pk
		svc.emitLocked(rs, task443UsageEvent(200000, 170000, 170000))
		svc.mu.Unlock()
		if got := task443EventsOfType(rs, EventContextPressure); len(got) != 1 {
			t.Fatalf("provider %s: pressure events = %d, want 1", pk, len(got))
		}
	}
}
