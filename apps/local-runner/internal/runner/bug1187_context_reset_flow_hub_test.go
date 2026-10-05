package runner

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// BUG-1187 (live run-204891): contextResetEligibleLocked only checked
// parentRunID=="" && chatID!="" && legState==active — a sprint hub whose
// provider compacted mid-sprint was offered rotate_leg, the committed reset
// minted a plain normal_chat leg via switchChatLeg, and the in-flight agent
// loop stayed keyed on the CLOSED source run. Every later admission relayed
// onto the orphan leg whose own (fresh) loop was blocked → six consecutive
// hub_reinvoke_start_failed flow_awaiting_user records and a permanently
// wedged sprint. A run that owns a LIVE agent loop must never be leg
// switch/reset eligible; once the loop seals the run is plain chat
// (BUG-302/308) and rotation is safe again.
func TestBug1187_LiveLoopOwnerNotResetEligible(t *testing.T) {
	svc := bug289Service(t)
	for _, status := range []string{"running", "blocked", "paused", "waiting_review"} {
		rs := &interactiveRun{
			id:       "run-1187-" + status,
			chatID:   "chat-1187",
			legState: LegStateActive,
			status:   RunStatusRunning,
			subs:     map[int64]chan ProviderEvent{},
		}
		svc.mu.Lock()
		svc.runs[rs.id] = rs
		svc.mu.Unlock()
		svc.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
			st.Status = status
			return st
		})
		if svc.contextResetEligibleLocked(rs) {
			t.Fatalf("loop status %q: a live flow-loop owner must not be leg-reset eligible — rotation orphans the sprint", status)
		}
	}
}

// Near-miss: the guard must not over-refuse. A run with NO loop record
// (plain chat) or a SEALED loop (done/stopped — plain chat per BUG-302/308)
// stays eligible; a pending flow latch (flowArm set, never launched, no
// loop) must keep riding the switch per CP-89 Task-451.
func TestBug1187_NoLoopOrSealedLoopStaysEligible(t *testing.T) {
	svc := bug289Service(t)
	for _, status := range []string{"", "done", "stopped"} {
		rs := &interactiveRun{
			id:       "run-1187-ok-" + status,
			chatID:   "chat-ok",
			legState: LegStateActive,
			status:   RunStatusRunning,
			subs:     map[int64]chan ProviderEvent{},
		}
		svc.mu.Lock()
		svc.runs[rs.id] = rs
		svc.mu.Unlock()
		if status != "" {
			svc.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
				st.Status = status
				return st
			})
		}
		if !svc.contextResetEligibleLocked(rs) {
			t.Fatalf("loop status %q: sealed/no-loop run is plain chat — must stay reset eligible", status)
		}
	}
}

// The provider-switch endpoint shares switchChatLeg: the same orphan hole
// exists there, so the Phase-A guard lives in switchChatLeg itself — every
// leg-minting caller is covered, not only context rotate_leg.
func TestBug1187_SwitchChatLegRefusesLiveLoopOwner(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.mutateLoop(handle.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "flow_in_progress") {
		t.Fatalf("provider switch on a live flow-loop owner must refuse 409 flow_in_progress, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// A committed rotate_leg intent that meets a live loop DEFERS — it is not
// dropped (the user committed the reset; the degraded leg still wants it)
// and it never executes mid-flow. After the loop seals the intent is
// consumable at the next admission boundary.
func TestBug1187_PendingContextResetDefersNotDropsOnLiveLoop(t *testing.T) {
	svc := bug289Service(t)
	rs := &interactiveRun{
		id:       "run-1187-defer",
		chatID:   "chat-defer",
		legState: LegStateActive,
		status:   RunStatusRunning,
		subs:     map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	svc.mu.Lock()
	rs.contextResetPending = true
	svc.mu.Unlock()

	if got := svc.consumePendingContextReset(context.Background(), rs.id); got != "" {
		t.Fatalf("consumePendingContextReset minted leg %q during a live loop — the orphan hole itself", got)
	}
	svc.mu.Lock()
	if !rs.contextResetPending {
		t.Fatal("committed reset intent dropped on a live-loop deferral — must stay armed for the post-seal admission")
	}
	svc.mu.Unlock()

	svc.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "done"
		return st
	})
	svc.mu.Lock()
	eligible := svc.contextResetEligibleLocked(rs)
	svc.mu.Unlock()
	if !eligible {
		t.Fatal("still reset-ineligible after the loop sealed — the defer must lift")
	}
}
