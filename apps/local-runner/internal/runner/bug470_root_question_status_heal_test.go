package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-470: CA-642 mirrors a flow-engine child's ask_user question onto the
// root run's event stream so the operator can answer from the main timeline.
// The mirrored EventUserQuestionRequired flips the ROOT run's status to
// waiting_question (applyRunEventLocked), but AnswerQuestion and
// expireQuestion only heal the question's OWNER run — the root stays
// waiting_question forever with no pending question (live run-102429:
// slicer question q-103744 resolved on child run-103606, root status stuck
// waiting_question while the loop ran and the validator step flapped).
// Resolution/expiry must heal the mirrored wait on the root run too.
// Additive — CA-642 tests untouched.

func TestBUG470_AnswerClearsRootMirroredWaitingQuestion(t *testing.T) {
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

	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()
	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
	done := make(chan []string, 1)
	go func() {
		choice, err := bridge.AskQuestion("proceed?", []QuestionOption{{Label: "yes", Value: "yes"}}, false)
		if err == nil {
			done <- choice
		}
	}()

	var qid string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, ev := range svc.runs[parentH.RunID].events {
			if ev.Type == EventUserQuestionRequired && ev.QuestionID != "" {
				qid = ev.QuestionID
				return true
			}
		}
		return false
	}, "root stream question")

	svc.mu.Lock()
	rootStatus := svc.runs[parentH.RunID].status
	svc.mu.Unlock()
	if rootStatus != RunStatusWaitingQuestion {
		t.Fatalf("precondition: root status = %q, want waiting_question (mirror must stamp it)", rootStatus)
	}

	if err := svc.AnswerQuestion(qid, []string{"yes"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("child bridge never unblocked")
	}

	svc.mu.Lock()
	rootStatus = svc.runs[parentH.RunID].status
	svc.mu.Unlock()
	if rootStatus == RunStatusWaitingQuestion {
		t.Fatalf("root status still waiting_question after question %s resolved — mirrored wait leaked", qid)
	}
	if rootStatus != RunStatusRunning {
		t.Fatalf("root status = %q, want running after resolution", rootStatus)
	}
}

func TestBUG470_ExpiryClearsRootMirroredWaitingQuestion(t *testing.T) {
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

	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()
	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
	go func() { _, _ = bridge.AskQuestion("proceed?", []QuestionOption{{Label: "yes", Value: "yes"}}, false) }()

	var qid string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, ev := range svc.runs[parentH.RunID].events {
			if ev.Type == EventUserQuestionRequired && ev.QuestionID != "" {
				qid = ev.QuestionID
				return true
			}
		}
		return false
	}, "root stream question")

	svc.expireQuestion(qid)

	svc.mu.Lock()
	rootStatus := svc.runs[parentH.RunID].status
	svc.mu.Unlock()
	if rootStatus == RunStatusWaitingQuestion {
		t.Fatalf("root status still waiting_question after question %s expired — mirrored wait leaked", qid)
	}
}

func TestBUG470_RehydrateHealsPersistedPhantomWaitingQuestion(t *testing.T) {
	svc, _ := newTestServer(t)

	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parentH.RunID]
	rs.flowEngineDriven = true
	// Simulate the pre-fix persisted lie: status leaked to waiting_question
	// by a mirrored child question, no durable pending card on this run.
	rs.status = RunStatusWaitingQuestion
	rs.agentStatus = string(RunStatusWaitingQuestion)
	svc.rehydratePendingGatesLocked(parentH.RunID)
	got := svc.runs[parentH.RunID].status
	svc.mu.Unlock()
	if got == RunStatusWaitingQuestion {
		t.Fatalf("rehydrate resurrected phantom waiting_question with no durable pending card")
	}
	if got != RunStatusRunning {
		t.Fatalf("status after heal = %q, want running", got)
	}
}

func TestBUG470_AnswerKeepsRootWaitingWhileSiblingQuestionPending(t *testing.T) {
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
	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()

	qidOf := func(prompt string) string {
		bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
		go func() { _, _ = bridge.AskQuestion(prompt, []QuestionOption{{Label: "yes", Value: "yes"}}, false) }()
		var qid string
		waitFor(t, func() bool {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			for id, rec := range svc.questions {
				if rec.prompt == prompt && rec.status == "pending" {
					qid = id
					return true
				}
			}
			return false
		}, "pending question "+prompt)
		return qid
	}
	q1 := qidOf("first?")
	qidOf("second?")

	if err := svc.AnswerQuestion(q1, []string{"yes"}); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	svc.mu.Lock()
	rootStatus := svc.runs[parentH.RunID].status
	svc.mu.Unlock()
	if rootStatus != RunStatusWaitingQuestion {
		t.Fatalf("root status = %q, want waiting_question while sibling question still pending", rootStatus)
	}
}
