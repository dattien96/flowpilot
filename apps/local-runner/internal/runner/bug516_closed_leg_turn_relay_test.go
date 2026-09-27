package runner

// BUG-516 (deep review round 3, 2026-09-26): startTurn never checked
// rs.legState — a run whose leg was CLOSED by a committed route kept
// accepting new turns and dispatched them on the stale binding. Live
// evidence: a quota card answered use_once→grok minted leg run-31122 and
// durably closed run-30802, yet a turn re-sent to run-30802 completed on
// devin — silently bypassing the operator's route choice.
//
// Fix: admission on a closed leg relays to the chat's ACTIVE leg (same
// pattern as the BUG-511 cross-provider rotate relay); a closed leg with
// no active successor — or no chat at all (dead flow child) — fails
// honestly with 409 leg_closed.

import (
	"context"
	"strings"
	"testing"
)

func TestBug516_TurnOnClosedLegRelaysToActiveLeg(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// Provider switch closes leg 1 and mints leg 2 on claude (fake).
	resp, aerr := svc.switchChatLeg(context.Background(), handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude}, false)
	if aerr != nil {
		t.Fatalf("switch: %+v", aerr)
	}
	newLegID := resp.Handle.RunID
	svc.mu.Lock()
	if got := svc.runs[handle.RunID].legState; got != LegStateClosed {
		svc.mu.Unlock()
		t.Fatalf("old leg state = %q, want closed", got)
	}
	svc.mu.Unlock()

	// Turn sent to the CLOSED run id must land on the active leg.
	if _, terr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "after switch"}, "test", "bug516-relay"); terr != nil {
		t.Fatalf("relayed turn must not error, got %+v", terr)
	}
	svc.mu.Lock()
	got := svc.runs[newLegID].lastPrompt
	svc.mu.Unlock()
	if got != "after switch" {
		t.Fatalf("active leg lastPrompt = %q — the turn did not relay", got)
	}
}

func TestBug516_ClosedLegWithoutSuccessorFailsHonestly(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// Close the only leg without minting a successor (durable crash-shape:
	// close persisted, replacement never landed).
	svc.mu.Lock()
	svc.runs[handle.RunID].legState = LegStateClosed
	svc.runs[handle.RunID].legClosedReason = LegClosedReasonProviderSwitch
	svc.mu.Unlock()

	_, terr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "hello"}, "test", "bug516-dead")
	if terr == nil || terr.status != 409 || !strings.Contains(terr.code, "leg_closed") {
		t.Fatalf("closed leg with no successor must 409 leg_closed, got %+v", terr)
	}
}

func TestBug516_ClosedNonChatLegFailsHonestly(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", StepID: "s-1", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.legState = LegStateClosed
	rs.legClosedReason = LegClosedReasonProviderSwitch
	rs.chatID = "" // flow child / non-chat: no leg relay target exists
	svc.mu.Unlock()

	_, terr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "hello"}, "test", "bug516-child")
	if terr == nil || terr.status != 409 || !strings.Contains(terr.code, "leg_closed") {
		t.Fatalf("closed non-chat leg must 409 leg_closed, got %+v", terr)
	}
}

func TestBug516_ActiveLegUnaffected(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, terr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "normal"}, "test", "bug516-active"); terr != nil {
		t.Fatalf("active leg turn must dispatch, got %+v", terr)
	}
}
