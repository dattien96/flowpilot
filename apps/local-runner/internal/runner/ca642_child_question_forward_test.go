package runner

import (
	"context"
	"testing"
	"time"
)

// CA-642: a model ask_user question raised on a flow-engine CHILD run
// (planner/tester/coder) was emitted only on the child's event stream. The
// TUI/desktop subscribe to the root run stream, so the operator saw the flow
// step WAITING_USER_APPROVAL but no question card and could not answer —
// the flow hung (run-260082: coder ask_user on format.go/calc_test.go scope
// conflict). The question must be mirrored onto the root flow run's stream;
// AnswerQuestion already resolves globally by question id.
// Additive — legacy question/approval tests untouched.

func TestChildAskQuestionForwardedToFlowRootStream(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second

	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parentH.RunID].flowEngineDriven = true
	svc.mu.Unlock()

	spawned, spawnErr := svc.spawnChildRun(context.Background(), parentH.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	subID, _, snap, found := svc.subscribe(parentH.RunID, 0)
	if !found {
		t.Fatalf("parent subscribe: run not found")
	}
	defer svc.unsubscribe(parentH.RunID, subID)

	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()
	if child == nil || child.parentRunID != parentH.RunID {
		t.Fatalf("child run missing/parent wrong: %+v", child)
	}

	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
	done := make(chan []string, 1)
	errCh := make(chan error, 1)
	go func() {
		choice, err := bridge.AskQuestion("Where should callerSum go?", []QuestionOption{{Label: "calc.go", Value: "calc.go"}, {Label: "format.go", Value: "format.go"}}, false)
		if err != nil {
			errCh <- err
			return
		}
		done <- choice
	}()

	waitFor(t, func() bool {
		for _, ev := range snapEvents(svc, parentH.RunID, snap) {
			if ev.Type == EventUserQuestionRequired {
				return true
			}
		}
		return false
	}, "parent stream question")

	evs := snapEvents(svc, parentH.RunID, snap)
	var qid string
	for _, ev := range evs {
		if ev.Type == EventUserQuestionRequired {
			qid = ev.QuestionID
			if len(ev.Options) != 2 || ev.Prompt == "" {
				t.Fatalf("parent question event incomplete: %+v", ev)
			}
		}
	}
	if qid == "" {
		t.Fatalf("no question id on parent stream")
	}

	svc.mu.Lock()
	childRec := svc.questions[qid]
	childRunID := ""
	if childRec != nil {
		childRunID = childRec.runID
	}
	svc.mu.Unlock()
	if childRunID != spawned.RunID {
		t.Fatalf("question %s registered on run %q, want child %q", qid, childRunID, spawned.RunID)
	}

	if err := svc.AnswerQuestion(qid, []string{"format.go"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	select {
	case choice := <-done:
		if len(choice) != 1 || choice[0] != "format.go" {
			t.Fatalf("child bridge got %v, want [format.go]", choice)
		}
	case err := <-errCh:
		t.Fatalf("child bridge AskQuestion error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("child bridge never unblocked after answering on parent stream")
	}
}

func TestNonFlowParentDoesNotReceiveChildQuestion(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = 5 * time.Second

	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	spawned, spawnErr := svc.spawnChildRun(context.Background(), parentH.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
	done := make(chan struct{})
	go func() {
		_, _ = bridge.AskQuestion("q?", []QuestionOption{{Label: "a", Value: "a"}}, false)
		close(done)
	}()
	<-done

	svc.mu.Lock()
	defer svc.mu.Unlock()
	parent := svc.runs[parentH.RunID]
	for _, ev := range parent.events {
		if ev.Type == EventUserQuestionRequired {
			t.Fatalf("non-flow parent must NOT receive child question, got %+v", ev)
		}
	}
}

// snapEvents merges a subscribe snapshot with live events for the run.
func snapEvents(svc *InteractiveService, runID string, snap []ProviderEvent) []ProviderEvent {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil {
		return snap
	}
	out := append([]ProviderEvent(nil), snap...)
	return append(out, rs.events...)
}