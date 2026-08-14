package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestChildFacingUserPrompt_ExtractsTask(t *testing.T) {
	p := "You are the implementation agent for a scoped FlowPilot node.\n\n" +
		"[FlowPilot sub-agent — agent: coder | role: coder]\n\n" +
		"[Context: use sections above as feature truth. Stay in scope.]\n\n" +
		"fix bug 1+1 != 2\n\n## Required file outputs (write contract)\nBefore you finish"
	got := childFacingUserPrompt(p)
	if !strings.Contains(got, "fix bug 1+1 != 2") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "Required file outputs") {
		t.Fatalf("should drop write contract: %q", got)
	}
}

func TestReplayChildHistoryMessages_HasUserAndAssistant(t *testing.T) {
	evs := []client.ProviderEvent{
		{Type: "turn_started", Prompt: "You are the implementation agent.\n[FlowPilot sub-agent — agent: coder]\n\n[Context: use sections above as feature truth. Stay in scope.]\n\nfix bug 1+1 != 2"},
		{Type: "message_completed", Text: "I'll fix Add()."},
		{Type: "turn_completed", FinalMessage: "I'll fix Add().\n\n## BUG-278 complete"},
	}
	msgs := replayChildHistoryMessages(evs)
	if len(msgs) < 2 {
		t.Fatalf("msgs=%+v", msgs)
	}
	if msgs[0].Role != "user" || !strings.Contains(msgs[0].Content, "fix bug") {
		t.Fatalf("user=%+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Content, "BUG-278") {
		t.Fatalf("assistant=%+v", msgs[1])
	}
}

func TestFormatAgentViewStatus_HighlightsChildName(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-98158", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	m.focusRunID = "run-98158"
	// Status is location-only (agent:name); open/back live on F2 steps panel.
	got := stripANSI(m.formatAgentViewStatus())
	if got != "agent:grok-coder" {
		t.Fatalf("view status=%q want agent:grok-coder", got)
	}
	forceStatusColorProfile(t)
	if !strings.Contains(m.formatAgentViewStatus(), styleStatusAgent.Render("grok-coder")) {
		t.Fatal("agent name must use styleStatusAgent (teal), not accent")
	}
}

func TestFocusStreamOpenedMsg_LoadsHistory(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-98153"}
	m.focusRunID = "run-98158"
	m.addMessage("system", "Child transcript: grok-coder", "")
	m2, _ := m.Update(focusStreamOpenedMsg{
		RunID: "run-98158",
		Messages: []ChatMessage{
			{Role: "user", Content: "fix bug 1+1 != 2"},
			{Role: "assistant", Content: "I'll fix Add()."},
		},
	})
	am := m2.(*AppModel)
	joined := ""
	for _, msg := range am.messages {
		joined += msg.Content + "\n"
	}
	if !strings.Contains(joined, "fix bug") || !strings.Contains(joined, "I'll fix Add") {
		t.Fatalf("missing child history:\n%s", joined)
	}
}
