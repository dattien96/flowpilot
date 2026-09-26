package runner

// BUG-511 (deep review B-1, 2026-09-26): the durable quota ledger's blocked
// list is written by noteAccountBlockedLocked on every live-observed hard
// limit, and SameProviderCandidates reads it — but nothing consulted it
// before dispatch. A run pinned to a blocked account burned a turn that
// could only fail with the same limit error before the post-failure
// enterQuotaGate ran. CP-87 requires the resolver before hub/node
// dispatch, not only after a wasted provider call. Live tell in R2: three
// concurrent Devin runs all dispatched onto the sole account with no
// preflight at all.
//
// Fix: at the turn-admission boundary in startTurn — after idempotent
// replay and the pending-card guard, before the turn is dispatched — a run
// whose pinned account is recorded blocked enters the quota routing gate
// (enterQuotaGate) and the caller gets a quota_route_required conflict
// instead of a provider call. Child turns ride the same seam. An
// unblocked pin is untouched; idempotent replays short-circuit earlier
// and stay unaffected.

import (
	"testing"
	"time"
)

func TestBug511_BlockedAccountEntersQuotaGateBeforeDispatch(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")
	// Live-observed hard limit on the pinned account, recorded durably.
	s.mu.Lock()
	s.noteAccountBlockedLocked("codex", "cx-0", "quota_exhausted")
	s.mu.Unlock()

	tid, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", "")
	if apiErr == nil {
		t.Fatalf("turn on a blocked account must not dispatch, got turn %s", tid)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 1 || questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatal("blocked account must escalate into the quota routing gate before dispatch")
	}
}

func TestBug511_UnblockedAccountStillDispatches(t *testing.T) {
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	tid, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "hello"}, "", "")
	if apiErr != nil && apiErr.code == "quota_route_required" {
		t.Fatalf("unblocked pin must not enter the quota gate: %v", apiErr)
	}
	if len(task449Questions(s, rs.id)) != 0 {
		t.Fatal("unblocked pin must not produce a quota route card")
	}
	if apiErr == nil && tid == "" {
		t.Fatal("dispatch must return a turn id")
	}
}

func TestBug511_EmptyAccountBindingSkipsPreflight(t *testing.T) {
	// Runs without an account pin (unconfigured provider homes, dev beds)
	// must not be touched by the blocked list.
	s := task448Service(t, ProviderKeyCodex)
	rs := task449ChatRun(s, "run-1", "", "dev")

	tid, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "hello"}, "", "")
	if apiErr != nil && apiErr.code == "quota_route_required" {
		t.Fatalf("unpinned run must not enter the quota gate: %v", apiErr)
	}
	if len(task449Questions(s, rs.id)) != 0 {
		t.Fatal("unpinned run must not produce a quota route card")
	}
	if apiErr == nil && tid == "" {
		t.Fatal("dispatch must return a turn id")
	}
}

// Round 2 (review): the veto must also cover telemetry-exhausted accounts
// whose ledger block was never written — the three concurrent Devin runs in
// R2 dispatched because noteAccountBlockedLocked had not run yet. Manual
// mode with an alternate must park on the route card.
func TestBug511_TelemetryExhaustedPinEntersGateWithoutLedger(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, true),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	// cx-0 reports 0% remaining — exhausted — but the durable block ledger
	// has no entry for it (the limit was never observed mid-dispatch).
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	tid, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", "")
	if apiErr == nil {
		t.Fatalf("turn on a telemetry-exhausted pin must not dispatch, got turn %s", tid)
	}
	if apiErr.code != "quota_route_required" {
		t.Fatalf("manual mode with an alternate must emit the route card, got %v", apiErr)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 1 || questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatal("telemetry-exhausted pin must park on the quota route card")
	}
}

// Round 2 (review): a single-account environment has no route candidate —
// CommitQuotaResolution emits quota_route_blocked but no card. The reply
// must say so honestly (quota_route_blocked) instead of looping a 409 that
// references a card nobody can resolve.
func TestBug511_SingleAccountBlockedFailsHonestlyNoCard(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	_, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", "")
	if apiErr == nil {
		t.Fatal("turn on the only exhausted account must not dispatch")
	}
	if apiErr.code != "quota_route_blocked" {
		t.Fatalf("no-candidate outcome must surface quota_route_blocked honestly, got %v", apiErr)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 0 {
		t.Fatal("no-candidate outcome must NOT emit a route card — there is nothing to resolve")
	}
	// The structured blocked record still lands on the durable stream.
	s.mu.Lock()
	blocked := false
	for _, e := range rs.events {
		if e.Type == EventQuotaRouteBlocked {
			blocked = true
		}
	}
	s.mu.Unlock()
	if !blocked {
		t.Fatal("quota_route_blocked event must be emitted for the no-candidate outcome")
	}
	// A second turn repeats the honest failure — no card is ever invented.
	_, apiErr = s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", "")
	if apiErr == nil || apiErr.code != "quota_route_blocked" {
		t.Fatalf("repeat turn must stay quota_route_blocked, got %v", apiErr)
	}
}

// Round 2 (review): auto-rotate must not swallow the prompt — a committed
// same-provider repin lets the turn continue onto the healthy account.
func TestBug511_AutoRotateRepinsAndContinues(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, true),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	_, apiErr := s.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", "")
	if apiErr != nil && apiErr.code == "quota_route_required" {
		t.Fatalf("auto-rotate must not park on a card when a same-provider candidate exists: %v", apiErr)
	}
	s.mu.Lock()
	pin := rs.providerAccountID
	committed := false
	for _, e := range rs.events {
		if e.Type == EventQuotaRouteCommitted {
			committed = true
		}
	}
	s.mu.Unlock()
	if pin != "cx-1" {
		t.Fatalf("auto-rotate must repin the leg to the healthy account, pin=%q", pin)
	}
	if !committed {
		t.Fatal("quota_route_committed must record the committed rotation durably")
	}
}
