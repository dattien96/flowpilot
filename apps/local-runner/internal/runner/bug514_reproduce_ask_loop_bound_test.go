package runner

// BUG-514 (deep review B-4, 2026-09-26): a reproduce-gated child that cannot
// reproduce the bug has exactly two honest endings — produce a RED test or be
// escalated. The runner enforced neither: checkReproduceRule reprompts only
// when a turn ENDS without a red test, but ask_user parks inside the same
// turn and never touched repromptAttempts, so a child could park on
// "please unblock me" forever (live run-90420 asked twice, never reached the
// maxFlowGateReprompts cap, never escalated).
//
// Fix: reproduce-gated ask_user episodes count against the same reprompt
// budget the gate already enforces. Once attempts reach
// maxFlowGateReprompts the question is refused so the turn ends and the
// gate's existing exhausted-reprompt escalation fires. Non-reproduce
// children are untouched.

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func bug514ReproduceChild(t *testing.T, svc *InteractiveService, label, behavior string) *interactiveRun {
	t.Helper()
	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	parent := svc.runs[parentH.RunID]
	parent.flowEngineDriven = true
	parent.activeFlowNodes = []agentpack.FlowNode{{ID: label, Behavior: behavior}}
	svc.mu.Unlock()

	spawned, spawnErr := svc.spawnChildRun(context.Background(), parentH.RunID, SpawnAgentInput{Agent: "reproducer", Prompt: "reproduce it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	child.label = label
	svc.mu.Unlock()
	return child
}

func bug514AskOnce(t *testing.T, svc *InteractiveService, b *turnBridge) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		_, err := b.AskQuestion("stuck?", []QuestionOption{{Label: "ok", Value: "ok"}}, false)
		errCh <- err
	}()
	var qid string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, rec := range svc.questions {
			if rec.status == "pending" {
				qid = rec.id
				return true
			}
		}
		return false
	}, "question parked")
	if err := svc.AnswerQuestion(qid, []string{"ok"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("AskQuestion errored before budget exhausted: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AskQuestion never unblocked")
	}
}

func TestBug514_ReproduceChildAskUserBounded(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second
	child := bug514ReproduceChild(t, svc, "reproduce_test", "agent.reproduce")
	b := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "t-repro"}

	// maxFlowGateReprompts episodes park and resolve as usual.
	for i := 0; i < maxFlowGateReprompts; i++ {
		bug514AskOnce(t, svc, b)
	}
	// Round 2 (review): parked questions must NOT burn the GATE's reprompt
	// budget — two answered questions leaving repromptAttempts at the cap
	// would starve the actual gate reprompts so the escalate path never
	// fires. The bound counts EventUserQuestionRequired on the run's own
	// durable event stream instead.
	svc.mu.Lock()
	attempts := child.repromptAttempts
	asked := 0
	for _, e := range child.events {
		if e.Type == EventUserQuestionRequired {
			asked++
		}
	}
	svc.mu.Unlock()
	if attempts != 0 {
		t.Fatalf("repromptAttempts = %d, want 0 — reproduce questions must not consume the gate's reprompt budget", attempts)
	}
	if asked != maxFlowGateReprompts {
		t.Fatalf("durable question events = %d, want %d", asked, maxFlowGateReprompts)
	}

	// The next ask_user is refused: the loop is over budget, the turn must
	// end so the gate's exhausted-reprompt escalation can fire.
	if _, err := b.AskQuestion("still stuck?", []QuestionOption{{Label: "ok", Value: "ok"}}, false); err == nil {
		t.Fatal("ask_user past the reproduce budget must be refused, not parked forever")
	} else if !strings.Contains(strings.ToLower(err.Error()), "reproduce") {
		t.Fatalf("refusal should name the reproduce budget, got: %v", err)
	}
}

// Round 2 (review): the refusal must also unblock the ESCALATION path —
// with repromptAttempts untouched, the gate's own turn-end reprompts still
// get their full budget and the exhausted branch escalates (BUG-391). This
// test locks the durable-stream bound surviving a restart-shaped run
// (events rehydrated, in-memory counters gone).
func TestBug514_ReproduceAskBoundSurvivesRehydratedEvents(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second
	child := bug514ReproduceChild(t, svc, "reproduce_test", "agent.reproduce")

	// Simulate the post-restart shape: the run's durable event stream
	// already carries maxFlowGateReprompts parked-question episodes.
	svc.mu.Lock()
	for i := 0; i < maxFlowGateReprompts; i++ {
		child.events = append(child.events, ProviderEvent{Type: EventUserQuestionRequired, QuestionID: "q-old"})
	}
	svc.mu.Unlock()

	b := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "t-repro2"}
	if _, err := b.AskQuestion("still stuck?", []QuestionOption{{Label: "ok", Value: "ok"}}, false); err == nil {
		t.Fatal("a reproduce child whose durable stream already holds the budget must be refused immediately")
	}
}

func TestBug514_NonReproduceChildAskUserUnbounded(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second
	child := bug514ReproduceChild(t, svc, "coder", "agent.code")
	b := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "t-code"}

	// A normal coder child parks questions without touching the reproduce
	// budget — this guard is reproduce-only.
	for i := 0; i < maxFlowGateReprompts+1; i++ {
		bug514AskOnce(t, svc, b)
	}
	svc.mu.Lock()
	attempts := child.repromptAttempts
	svc.mu.Unlock()
	if attempts != 0 {
		t.Fatalf("non-reproduce child repromptAttempts = %d, want 0", attempts)
	}
}
