package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestBUG371_ComposerHidesStopWhenLoopDone(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:0")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "vibe-ingest", Label: "vibe-ingest"}
			m.runHandle = &client.RunHandle{RunID: "run-678326", Status: "running"}
			m.connStatus = ConnWaiting
			m.orchStream = &orchStreamState{}
			m.flowLoopStatus = "done"
			m.flowStepsActive = "audit" // leftover active-step chip must not keep [stop]
			m.statusMsg = "done"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-678326", AgentName: "main", Role: "main", Status: "completed"},
				{RunID: "run-coder", AgentName: "coder", Status: "waiting_user_approval"},
			}
			if m.turnIsActive() {
				t.Fatalf("%s: done loop must not arm [stop]", pk)
			}
			title := stripANSI(m.chatFrameTitle())
			if strings.Contains(title, "[stop]") {
				t.Fatalf("%s: composer still shows [stop] after done: %q", pk, title)
			}
			if !strings.Contains(title, "done") && !strings.Contains(strings.ToLower(title), "vibe-ingest") {
				t.Fatalf("%s: composer=%q", pk, title)
			}
		})
	}
}

func TestBUG371_ComposerHidesStopWhileAskUser(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:0")
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, Label: "vibe-ingest"}
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.connStatus = ConnWaiting
	m.question = &QuestionState{Prompt: "what is the FlowPilot failure?"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-1", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-c", AgentName: "coder", Status: "waiting_question"},
	}
	if m.turnIsActive() {
		t.Fatal("ask_user overlay must not arm [stop]")
	}
	if strings.Contains(stripANSI(m.chatFrameTitle()), "[stop]") {
		t.Fatal("composer [stop] during question")
	}
}

func TestBUG371_RunningLoopStillShowsStop(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:0")
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, Label: "vibe-ingest"}
	m.runHandle = &client.RunHandle{RunID: "run-live", Status: "running"}
	m.connStatus = ConnRunning
	m.flowLoopStatus = "running"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-live", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-c", AgentName: "coder", Status: "running"},
	}
	if !m.turnIsActive() {
		t.Fatal("live coder must still arm [stop]")
	}
}
