package runner

import (
	"testing"
)

// BUG-391 (live runs 169/442/4014): rs.repromptAttempts was zeroed at every
// new-turn start — but a gate reprompt is delivered AS a new turn, so the
// maxFlowGateReprompts cap never accumulated and gate reprompts looped
// forever (every logged reprompt showed attempt=0). The counter must carry
// through a reprompt-delivery turn and reset on a fresh (non-reprompt) turn.
func TestBug391_RepromptDeliveryPreservesAttemptCounter(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[run.RunID].repromptAttempts = 1
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "fix the gate violations"}, scenarioGateReprompt, "durable-"+run.RunID+"-reprompt-00000000000000000001"); apiErr != nil {
		t.Fatalf("reprompt-delivery startTurn failed: %v", apiErr.msg)
	}
	svc.mu.Lock()
	got := svc.runs[run.RunID].repromptAttempts
	svc.mu.Unlock()
	if got != 1 {
		t.Fatalf("reprompt-delivery turn must not reset the loop counter; got %d want 1", got)
	}
}

func TestBug391_FreshTurnResetsAttemptCounter(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[run.RunID].repromptAttempts = 2
	svc.mu.Unlock()
	if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "new user prompt"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn failed: %v", apiErr.msg)
	}
	svc.mu.Lock()
	got := svc.runs[run.RunID].repromptAttempts
	svc.mu.Unlock()
	if got != 0 {
		t.Fatalf("a fresh user turn must reset the counter; got %d", got)
	}
}
