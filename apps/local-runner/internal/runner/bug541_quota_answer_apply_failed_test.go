package runner

// BUG-541 (live run-12520, 2026-09-28): a quota_route_required card answer
// was consumed at the question layer — HTTP 200 "accepted", record resolved,
// card gone — while the apply inside (respawnChildOnRoute) refused because
// the parent loop was blocked(escalate) at that instant. The respawn never
// ran, nothing re-armed the card, and the candidate was silently dropped
// from the cohort.
//
// Fix contract: a failed apply must NOT consume the card. AnswerQuestion
// returns the apply error and rolls the question back to pending so the
// operator can re-answer once the parent unblocks.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func bug541PendingQuotaCard(t *testing.T, s *InteractiveService, runID string) *questionRecord {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.questions {
		if rec != nil && rec.runID == runID && rec.status == "pending" &&
			questionRecordKind(rec) == quotaRouteQuestionKind {
			return rec
		}
	}
	return nil
}

func TestBug541_QuotaAnswerSpawnRefusedKeepsCardPending(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cl-0": 90}, now.Format(time.RFC3339))
	parent, child := task449FlowChild(t, s, "cx-0")

	if err := s.enterQuotaGate(context.Background(), child.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	card := bug541PendingQuotaCard(t, s, child.id)
	if card == nil {
		t.Fatal("manual quota gate must park on a pending card")
	}
	// Pick the cross-provider use_for_run option from the card itself.
	useForRun := ""
	for _, o := range card.options {
		if strings.HasPrefix(o.Label, "use_for_run|claude|") {
			useForRun = o.Label
		}
	}
	if useForRun == "" {
		t.Fatalf("card must offer a claude use_for_run option, got %+v", card.options)
	}

	// Live reproducer: the parent loop re-blocked (sibling escalate) between
	// the card being raised and the operator answering — the spawn guard
	// (BUG-432) then refuses the respawn.
	s.agentOrchestrator.setLoop(parent.id, AgentLoopState{Status: "blocked", BlockReason: "escalate"})

	aerr := s.AnswerQuestion(card.id, []string{useForRun})
	if aerr == nil {
		t.Fatal("apply failure on a blocked parent must surface an error, not 200-accepted")
	}
	if got := bug541PendingQuotaCard(t, s, child.id); got == nil || got.id != card.id {
		t.Fatal("failed apply must leave the quota card pending — the decision must not be consumed")
	}
	s.mu.Lock()
	childStatus := s.runs[child.id].status
	childLeg := s.runs[child.id].legState
	successors := 0
	for _, rs := range s.runs {
		if rs.parentRunID == parent.id && rs.id != child.id {
			successors++
		}
	}
	s.mu.Unlock()
	if childStatus != RunStatusWaitingQuestion {
		t.Fatalf("child must re-park waiting_question, got %q", childStatus)
	}
	if childLeg != LegStateActive {
		t.Fatalf("vetoed child leg must stay active while the card is pending, got %q", childLeg)
	}
	if successors != 0 {
		t.Fatalf("no successor may exist after a refused respawn, got %d", successors)
	}
}

// Companion: once the parent unblocks, the SAME retained card answers
// cleanly — the operator retries the identical choice and the respawn runs.
func TestBug541_RetainedCardAnswersAfterUnblock(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cl-0": 90}, now.Format(time.RFC3339))
	parent, child := task449FlowChild(t, s, "cx-0")

	if err := s.enterQuotaGate(context.Background(), child.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	card := bug541PendingQuotaCard(t, s, child.id)
	if card == nil {
		t.Fatal("manual quota gate must park on a pending card")
	}
	useForRun := ""
	for _, o := range card.options {
		if strings.HasPrefix(o.Label, "use_for_run|claude|") {
			useForRun = o.Label
		}
	}
	if useForRun == "" {
		t.Fatalf("card must offer a claude use_for_run option, got %+v", card.options)
	}

	s.agentOrchestrator.setLoop(parent.id, AgentLoopState{Status: "blocked", BlockReason: "escalate"})
	if aerr := s.AnswerQuestion(card.id, []string{useForRun}); aerr == nil {
		t.Fatal("first answer while blocked must fail")
	}
	// Unblock, retry the same card — the respawn must now run.
	s.agentOrchestrator.setLoop(parent.id, AgentLoopState{Status: "running"})
	if aerr := s.AnswerQuestion(card.id, []string{useForRun}); aerr != nil {
		t.Fatalf("re-answer on the retained card must succeed: %v", aerr)
	}
	if got := bug541PendingQuotaCard(t, s, child.id); got != nil {
		t.Fatal("a successful apply resolves the card for real")
	}
	s.mu.Lock()
	var successor *interactiveRun
	for _, rs := range s.runs {
		if rs.parentRunID == parent.id && rs.id != child.id {
			successor = rs
		}
	}
	childLeg := s.runs[child.id].legState
	s.mu.Unlock()
	if successor == nil || successor.providerKey != ProviderKeyClaude {
		t.Fatal("retried answer must spawn the claude successor")
	}
	if childLeg != LegStateClosed {
		t.Fatalf("vetoed child leg must close after the respawn lands, got %q", childLeg)
	}
}
