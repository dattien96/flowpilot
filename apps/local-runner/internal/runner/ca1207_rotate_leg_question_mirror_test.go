package runner

import (
	"context"
	"testing"
	"time"
)

// CA-1207 (BUG-1186 residual, live run-204891 → run-225672): a rotate_leg
// answer mints a NEW root run for the same chat — parentRunID is empty, so
// the CA-642 flow-root mirror never reaches the superseded leg the operator
// still has open. The hub's ask_user sat pending ~25min: the old leg's
// detail view showed status=running and no card; the stall was only visible
// via the mux lane. A gating question on one leg must mirror onto every
// non-terminal sibling leg of the same chat; answering resolves globally by
// question id and the mirrored wait heals.
// Additive — legacy question/mirror tests untouched.

func chatLegForTest(t *testing.T, svc *InteractiveService, chatID string, legSeq int, status RunStatus) string {
	t.Helper()
	h, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ChatID: chatID, ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun leg: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	rs.legSeq = legSeq
	rs.status = status
	rs.agentStatus = string(status)
	svc.mu.Unlock()
	return h.RunID
}

func TestCA1207_RotatedLegQuestionMirrorsToSiblingLeg(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second

	oldLegID := chatLegForTest(t, svc, "chat-x", 1, RunStatusRunning)
	newLegID := chatLegForTest(t, svc, "chat-x", 2, RunStatusRunning)

	svc.mu.Lock()
	newLeg := svc.runs[newLegID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: newLeg, ctx: context.Background(), turnID: "turn-t"}
	done := make(chan []string, 1)
	errCh := make(chan error, 1)
	go func() {
		choice, err := bridge.AskQuestion("Which task next?", []QuestionOption{{Label: "a", Value: "a"}}, false)
		if err != nil {
			errCh <- err
			return
		}
		done <- choice
	}()

	waitFor(t, func() bool {
		for _, ev := range snapEvents(svc, oldLegID, nil) {
			if ev.Type == EventUserQuestionRequired {
				return true
			}
		}
		return false
	}, "sibling leg question mirror")

	var qid string
	for _, ev := range snapEvents(svc, oldLegID, nil) {
		if ev.Type == EventUserQuestionRequired {
			qid = ev.QuestionID
		}
	}
	if qid == "" {
		t.Fatalf("no mirrored question on old leg stream")
	}

	svc.mu.Lock()
	oldLeg := svc.runs[oldLegID]
	if oldLeg.status != RunStatusWaitingQuestion {
		svc.mu.Unlock()
		t.Fatalf("old leg status = %q, want waiting_question (mirrored card must surface the wait)", oldLeg.status)
	}
	if oldLeg.pendingQuestionID != "" {
		svc.mu.Unlock()
		t.Fatalf("mirror must not stamp pendingQuestionID on the sibling (got %q)", oldLeg.pendingQuestionID)
	}
	svc.mu.Unlock()

	if err := svc.AnswerQuestion(qid, []string{"a"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	select {
	case choice := <-done:
		if len(choice) != 1 || choice[0] != "a" {
			t.Fatalf("bridge got %v, want [a]", choice)
		}
	case err := <-errCh:
		t.Fatalf("bridge AskQuestion error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("bridge never unblocked after answering on sibling stream")
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := svc.runs[oldLegID].status; got != RunStatusRunning {
		t.Fatalf("old leg status after resolve = %q, want running (mirrored wait must heal)", got)
	}
}

func TestCA1207_TerminalSiblingLegReceivesNoMirror(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second

	deadLegID := chatLegForTest(t, svc, "chat-y", 1, RunStatusCompleted)
	newLegID := chatLegForTest(t, svc, "chat-y", 2, RunStatusRunning)

	svc.mu.Lock()
	newLeg := svc.runs[newLegID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: newLeg, ctx: context.Background(), turnID: "turn-t"}
	done := make(chan struct{})
	go func() {
		_, _ = bridge.AskQuestion("q?", []QuestionOption{{Label: "a", Value: "a"}}, false)
		close(done)
	}()

	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.questions) > 0
	}, "question registered")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, ev := range svc.runs[deadLegID].events {
		if ev.Type == EventUserQuestionRequired {
			t.Fatalf("terminal leg must not receive mirrored question, got %+v", ev)
		}
	}
	if svc.runs[deadLegID].status != RunStatusCompleted {
		t.Fatalf("terminal leg status flipped: %q", svc.runs[deadLegID].status)
	}

	qid := ""
	for id := range svc.questions {
		qid = id
	}
	svc.mu.Unlock()
	if err := svc.AnswerQuestion(qid, []string{"a"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	svc.mu.Lock()
	<-done
}
