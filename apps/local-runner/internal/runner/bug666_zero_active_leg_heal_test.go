package runner

import (
	"context"
	"testing"
)

// BUG-666 (live run-752742): a committed provider switch closed the source
// leg (legClosedReason=provider_switch) but the successor mint failed — the
// chat ended with ZERO active legs. healChatLegsLocked's three rows
// (two-active, closed-no-record, orphan-intent) all require at least one
// active leg, so nothing heals and nothing surfaces: every turn fails
// leg_closed with no recovery surface — permanent silent deadlock.
//
// Fix contract: a chat whose legs are ALL closed with at least one
// provider-switch/context-reset closure must surface a typed
// leg_recovery_required record (once, idempotent) naming the newest closed
// leg as the reattach point. Heal never auto-mints — routing is a user
// decision — it surfaces honestly.
func TestBug666_ZeroActiveLegSurfacesRecoveryRecord(t *testing.T) {
	svc, writer := newSwitchTestService(t)

	old := &interactiveRun{id: "run-666-old", runKind: "chat", chatID: "cht_666", legSeq: 0, legState: LegStateClosed, legClosedReason: LegClosedReasonProviderSwitch, providerKey: ProviderKeyCodex}
	newer := &interactiveRun{id: "run-666-newer", runKind: "chat", chatID: "cht_666", legSeq: 1, legState: LegStateClosed, legClosedReason: LegClosedReasonContextReset, providerKey: ProviderKeyClaude}
	svc.runs[old.id] = old
	svc.runs[newer.id] = newer
	svc.chatRuns.register(old.id, "cht_666")
	svc.chatRuns.register(newer.id, "cht_666")

	svc.mu.Lock()
	svc.healChatLegsLocked("cht_666")
	svc.healChatLegsLocked("cht_666") // idempotent — heal runs on every switch entry
	svc.mu.Unlock()

	records, rerr := writer.store.ReadChatRecords(context.Background(), "cht_666", 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	recovery := 0
	for _, r := range records {
		if r.Type == EventTypeChatLegRecoveryRequired {
			recovery++
			if r.LegRunID != "run-666-newer" {
				t.Fatalf("recovery record points at %q, want newest closed leg run-666-newer", r.LegRunID)
			}
		}
	}
	if recovery != 1 {
		t.Fatalf("leg_recovery_required records = %d, want exactly 1 (idempotent)", recovery)
	}
}

// A chat with an active leg must NOT get the recovery surface — the record
// is strictly for the zero-active-leg state.
func TestBug666_ActiveLegPresentNoRecoveryRecord(t *testing.T) {
	svc, writer := newSwitchTestService(t)

	dead := &interactiveRun{id: "run-666d", runKind: "chat", chatID: "cht_666b", legSeq: 0, legState: LegStateClosed, legClosedReason: LegClosedReasonProviderSwitch}
	live := &interactiveRun{id: "run-666l", runKind: "chat", chatID: "cht_666b", legSeq: 1, legState: LegStateActive, switchFromRunID: "run-666d"}
	svc.runs[dead.id] = dead
	svc.runs[live.id] = live
	svc.chatRuns.register(dead.id, "cht_666b")
	svc.chatRuns.register(live.id, "cht_666b")

	svc.mu.Lock()
	svc.healChatLegsLocked("cht_666b")
	svc.mu.Unlock()

	records, rerr := writer.store.ReadChatRecords(context.Background(), "cht_666b", 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	for _, r := range records {
		if r.Type == EventTypeChatLegRecoveryRequired {
			t.Fatal("recovery record emitted while an active leg exists")
		}
	}
}
