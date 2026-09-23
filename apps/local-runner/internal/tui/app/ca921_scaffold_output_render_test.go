package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// CA-921: scaffold stdout deltas carry re-inserted line boundaries (the
// dispatcher's stream joiner). The stream must render with LITERAL newlines —
// the markdown path collapses single \n into a space, which is exactly the
// "appends forever, never breaks" bug reported on devin -p narration.

func TestScaffoldProgress_OutputStreamUsesLiteralNewlineRendering(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true
	m.addMessage("system", "Starting AI Scaffold turn for App…", "")

	_, _ = m.handleEngineScaffoldProgressMsg(EngineScaffoldProgressMsg{Snapshot: &client.ScaffoldProgressSnapshot{
		Active: true,
		Events: []client.ScaffoldProgressEvent{
			{Seq: 1, Kind: "output", Phase: "ai_turn", Text: "I'll start by reading the skill files.\n"},
			{Seq: 2, Kind: "output", Phase: "ai_turn", Text: "All four skills read.\n"},
			{Seq: 3, Kind: "output", Phase: "ai_turn", Text: "Writing packages/ now."},
		},
		NextSeq: 4,
	}})

	msg := m.messages[len(m.messages)-1]
	if msg.Role != "assistant" {
		t.Fatalf("scaffold stream role = %q, want assistant", msg.Role)
	}
	if msg.FormatHint != "scaffold" {
		t.Fatalf("scaffold stream FormatHint = %q, want scaffold (plain wrap, literal newlines)", msg.FormatHint)
	}
	want := "I'll start by reading the skill files.\nAll four skills read.\nWriting packages/ now."
	if msg.Content != want {
		t.Fatalf("scaffold stream content = %q, want %q", msg.Content, want)
	}
	// Render path proof: the rows must contain three separate lines — the
	// markdown renderer would collapse this to one line with spaces.
	m.width = 120
	rows := m.chatRows()
	var rendered []string
	for _, r := range rows {
		rendered = append(rendered, stripANSI(r.Text))
	}
	joined := strings.Join(rendered, "\n")
	for _, line := range []string{
		"I'll start by reading the skill files.",
		"All four skills read.",
		"Writing packages/ now.",
	} {
		if !strings.Contains(joined, line) {
			t.Fatalf("rendered rows missing %q\n---\n%s", line, joined)
		}
	}
	if strings.Contains(joined, "files.All four") {
		t.Fatalf("statuses still glued together:\n%s", joined)
	}
}

// A scaffold stream that starts right after a normal assistant answer must not
// glue onto it — it gets its own message so the boundary stays visible.
func TestScaffoldProgress_OutputDoesNotAppendToPlainAssistantMessage(t *testing.T) {
	m := newInitTestModel(t, "http://unused", "p1", "react-native")
	m.scaffoldBusy = true
	m.addMessage("assistant", "previous chat answer", "")

	_, _ = m.handleEngineScaffoldProgressMsg(EngineScaffoldProgressMsg{Snapshot: &client.ScaffoldProgressSnapshot{
		Active: true,
		Events: []client.ScaffoldProgressEvent{
			{Seq: 1, Kind: "output", Phase: "ai_turn", Text: "scaffold narration"},
		},
		NextSeq: 2,
	}})

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want scaffold stream as its own message", len(m.messages))
	}
	if m.messages[0].Content != "previous chat answer" || m.messages[0].FormatHint != "" {
		t.Fatalf("prior assistant message mutated: %+v", m.messages[0])
	}
	last := m.messages[1]
	if last.FormatHint != "scaffold" || last.Content != "scaffold narration" {
		t.Fatalf("scaffold message = %+v", last)
	}
}
