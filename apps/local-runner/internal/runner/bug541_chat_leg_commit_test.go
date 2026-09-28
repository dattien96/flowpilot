package runner

// BUG-541 residual — chat-leg variant (live run-19946 / q-19948): the
// operator typed an invented route option
// "use_for_run|codex|fakeacct|gpt-5.4". ResumeQuotaGate parsed it without
// validating the account, commitQuotaRotation emitted quota_route_committed
// BEFORE the switch was proven, and switchChatLeg closed the grok source leg
// while the codex destination could never provision a session (lazy
// provisioning on the first turn). Result: run stayed running, leg closed,
// every turn 409 session_unavailable, and the quota card never re-surfaced —
// an unrecoverable zombie.
//
// Fix contract (mirrors the child-respawn path's spawn-first ordering,
// BUG-534 applied to chat legs):
//  1. An operator-supplied route account must resolve — invented/unconnected
//     account ids are rejected before any mutation, card rolls back to
//     pending.
//  2. quota_route_committed emits only after the switch succeeded.
//  3. A pinned-account switch whose destination leg cannot provision a
//     session aborts: the dead destination leg closes terminal, the source
//     leg stays open — never close-before-provision.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBug541Chat_InventedAccountRejectedCardPending(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cl-0": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-chat-541", "cx-0", "")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	card := bug541PendingQuotaCard(t, s, rs.id)
	if card == nil {
		t.Fatal("manual quota gate must park on a pending card")
	}

	// Live reproducer: operator types a route the card never offered —
	// provider claude exists but account "ghost-acct" was never a candidate.
	aerr := s.AnswerQuestion(card.id, []string{"use_for_run|claude|ghost-acct|claude-sonnet"})
	if aerr == nil || aerr.code != "quota_route_apply_failed" {
		t.Fatalf("invented account must fail the apply, got %+v", aerr)
	}
	if got := bug541PendingQuotaCard(t, s, rs.id); got == nil || got.id != card.id {
		t.Fatal("rejected route must leave the quota card pending")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs[rs.id].legState != LegStateActive {
		t.Fatalf("source leg must stay active after a rejected route, got %q", s.runs[rs.id].legState)
	}
	for _, r := range s.runs {
		if r.id != rs.id && r.chatID == rs.chatID {
			t.Fatalf("no new leg may be minted for a rejected route, found %s", r.id)
		}
	}
	for _, ev := range s.runs[rs.id].events {
		if ev.Type == EventQuotaRouteCommitted {
			t.Fatal("quota_route_committed must not emit for a rejected route")
		}
	}
}

func TestBug541Chat_ValidRouteCommitsAfterSwitch(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cl-0": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-chat-541ok", "cx-0", "")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	card := bug541PendingQuotaCard(t, s, rs.id)
	if card == nil {
		t.Fatal("manual quota gate must park on a pending card")
	}
	useForRun := ""
	for _, o := range card.options {
		if strings.HasPrefix(o.Label, "use_for_run|claude|cl-0|") {
			useForRun = o.Label
		}
	}
	if useForRun == "" {
		t.Fatalf("card must offer a claude use_for_run option, got %+v", card.options)
	}
	if aerr := s.AnswerQuestion(card.id, []string{useForRun}); aerr != nil {
		t.Fatalf("a listed candidate route must apply, got %+v", aerr)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var newLeg *interactiveRun
	for _, r := range s.runs {
		if r.id != rs.id && r.chatID == rs.chatID {
			newLeg = r
		}
	}
	if newLeg == nil {
		t.Fatal("committed route must mint the destination leg")
	}
	if newLeg.legState != LegStateActive || newLeg.providerKey != ProviderKeyClaude {
		t.Fatalf("destination leg must be active on claude, got state=%q provider=%q", newLeg.legState, newLeg.providerKey)
	}
	if s.runs[rs.id].legState != LegStateClosed {
		t.Fatalf("source leg must close only after the destination exists, got %q", s.runs[rs.id].legState)
	}
	committed := false
	for _, ev := range s.runs[rs.id].events {
		if ev.Type == EventQuotaRouteCommitted {
			committed = true
		}
	}
	if !committed {
		t.Fatal("quota_route_committed must record the proven switch durably")
	}
}

func TestBug541Chat_PinnedSwitchUnresolvableRejectedPreMutation(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	srcID := handle.RunID

	// Routing-commit shape: pinned account that cannot ever provision. The
	// Phase-A pin check must reject before the durable intent is stamped —
	// no leg minted, no switch marker, source leg untouched.
	_, aerr := svc.switchChatLeg(context.Background(), handle.ChatID, chatSwitchRequest{
		TargetProviderKey: ProviderKeyClaude,
		ProviderAccountID: "ghost-acct",
	}, false)
	if aerr == nil || aerr.code != "account_unavailable" {
		t.Fatalf("unresolvable pinned switch must fail honestly, got %+v", aerr)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	src := svc.runs[srcID]
	if src.legState != LegStateActive {
		t.Fatalf("source leg must be untouched by a rejected switch, got %q", src.legState)
	}
	if src.switchFromRunID != "" {
		t.Fatal("rejected switch must never stamp the durable intent marker")
	}
	for _, r := range svc.runs {
		if r.id != srcID && r.chatID == handle.ChatID {
			t.Fatalf("rejected switch must not mint a destination leg, found %s", r.id)
		}
	}
	if svc.chatSwitchInFlight[handle.ChatID] {
		t.Fatal("rejected switch must not hold the in-flight guard")
	}
}

// The Phase-B abort in switchChatLeg covers the TOCTOU window — pin resolves
// at Phase A but provisioning fails by the time the seed turn dispatches
// (account disconnected, session gone). Pin the classifier so provisioning
// codes abort while content failures stay non-fatal under SD26-X-7.
func TestBug541Chat_SeedErrorClassification(t *testing.T) {
	provisioning := []string{
		"session_unavailable", "account_unavailable", "account_not_signed_in",
		"provider_unavailable", "leg_closed",
	}
	for _, code := range provisioning {
		if !isProvisioningSeedError(newAPIErr(409, code, "x")) {
			t.Fatalf("%s must classify as provisioning-class", code)
		}
	}
	for _, code := range []string{"turn_in_progress", "gate_in_progress", "invalid_request", "quota_route_required"} {
		if isProvisioningSeedError(newAPIErr(409, code, "x")) {
			t.Fatalf("%s must stay non-fatal (SD26-X-7)", code)
		}
	}
	if isProvisioningSeedError(context.DeadlineExceeded) {
		t.Fatal("non-apiErr failures must stay non-fatal")
	}
}
