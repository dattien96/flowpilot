package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-525 — consecutive tool calls collapse into one click-to-expand summary row.
// A run is the maximal sequence of Role=="tool" messages; any other message
// (user / assistant / thinking / system) breaks it. A run of exactly one tool
// renders directly, matching Desktop timelineGrouping semantics for tools.

func toolGroupModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	return m
}

// TestToolGroup_SingleToolShowsDirectly: one tool call between two normal
// responses renders the plain "→ tool" line with no summary wrapper.
func TestToolGroup_SingleToolShowsDirectly(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.addMessage("user", "fix the bug", "")
			m.addMessage("assistant", "on it", "")
			m.addMessage("tool", "→ write", "tool")
			m.addMessage("assistant", "done", "")
			view := stripANSI(m.View())
			if !strings.Contains(view, "→ write") {
				t.Fatalf("%s: single tool call must render directly:\n%s", pk, view)
			}
			if strings.Contains(view, "tool call") {
				t.Fatalf("%s: single tool call must not be grouped:\n%s", pk, view)
			}
		})
	}
}

// TestToolGroup_MultiToolCollapsedByDefault: six consecutive tool calls between
// two normal responses render as one collapsed summary row; the individual
// "→ tool" lines stay hidden until expanded.
func TestToolGroup_MultiToolCollapsedByDefault(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.addMessage("user", "fix the bug", "")
			m.addMessage("assistant", "on it", "")
			for i := 0; i < 6; i++ {
				m.addMessage("tool", "→ read_file", "tool")
			}
			m.addMessage("assistant", "done", "")
			view := stripANSI(m.View())
			if !strings.Contains(view, "+ 6 tool calls") {
				t.Fatalf("%s: expected collapsed '+ 6 tool calls' summary:\n%s", pk, view)
			}
			if !strings.Contains(view, "click") {
				t.Fatalf("%s: summary row must advertise clickability:\n%s", pk, view)
			}
			if strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: collapsed group must hide individual tool lines:\n%s", pk, view)
			}
		})
	}
}

// TestToolGroup_ClickExpandsAndCollapses: clicking the summary row expands the
// run (individual → lines appear, marker flips), a second click collapses again.
func TestToolGroup_ClickExpandsAndCollapses(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.addMessage("user", "u", "")
			m.addMessage("assistant", "a", "")
			m.addMessage("tool", "→ read_file", "tool")
			m.addMessage("tool", "→ grep", "tool")
			m.addMessage("tool", "→ write", "tool")
			m.addMessage("assistant", "done", "")
			key := toolGroupKey([]ChatMessage{
				{Role: "tool", Content: "→ read_file"},
				{Role: "tool", Content: "→ grep"},
				{Role: "tool", Content: "→ write"},
			})
			x, y, ok := findClickTarget(m, "tool-group:"+key)
			if !ok {
				t.Fatalf("%s: summary row must be clickable", pk)
			}
			m2, _ := m.Update(clickLeft(x, y))
			am := m2.(*AppModel)
			view := stripANSI(am.View())
			if !strings.Contains(view, "- 3 tool calls") {
				t.Fatalf("%s: expanded summary must flip marker to '- 3 tool calls':\n%s", pk, view)
			}
			for _, line := range []string{"→ read_file", "→ grep", "→ write"} {
				if !strings.Contains(view, line) {
					t.Fatalf("%s: expanded group must show %q:\n%s", pk, line, view)
				}
			}
			// Second click collapses again.
			m3, _ := am.Update(clickLeft(x, y))
			view = stripANSI(m3.(*AppModel).View())
			if strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: second click must collapse the group:\n%s", pk, view)
			}
		})
	}
}

// TestToolGroup_UserBreaksRun: a user message between tool calls splits them —
// the first run collapses, the trailing single tool still renders directly.
func TestToolGroup_UserBreaksRun(t *testing.T) {
	m := toolGroupModel("codex")
	m.addMessage("user", "u1", "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ grep", "tool")
	m.addMessage("user", "u2", "")
	m.addMessage("tool", "→ write", "tool")
	view := stripANSI(m.View())
	if !strings.Contains(view, "+ 2 tool calls") {
		t.Fatalf("first run must be grouped:\n%s", view)
	}
	if !strings.Contains(view, "→ write") {
		t.Fatalf("single tool after user break must render directly:\n%s", view)
	}
	if strings.Contains(view, "→ read_file") || strings.Contains(view, "→ grep") {
		t.Fatalf("collapsed group must hide its members:\n%s", view)
	}
}

// TestToolGroup_ThinkingBreaksRun: a thinking placeholder between tool calls is a
// non-tool message and ends the run, so two tools split by thinking render as two
// direct lines instead of one group.
func TestToolGroup_ThinkingBreaksRun(t *testing.T) {
	m := toolGroupModel("grok")
	m.addMessage("user", "u", "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("assistant", "…", "thinking")
	m.addMessage("tool", "→ write", "tool")
	view := stripANSI(m.View())
	if !strings.Contains(view, "→ read_file") || !strings.Contains(view, "→ write") {
		t.Fatalf("split tools must render directly:\n%s", view)
	}
	if strings.Contains(view, "tool call") {
		t.Fatalf("thinking must break the tool run (no group may form):\n%s", view)
	}
}

// TestToolGroup_RunsDoNotCrossToggle: expanding one run must not expand a later
// run with different tool names, even when both are 2-tool runs.
func TestToolGroup_RunsDoNotCrossToggle(t *testing.T) {
	m := toolGroupModel("claude")
	m.addMessage("user", "u", "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("assistant", "mid", "")
	m.addMessage("tool", "→ write", "tool")
	m.addMessage("tool", "→ write", "tool")
	key1 := toolGroupKey([]ChatMessage{{Role: "tool", Content: "→ read_file"}, {Role: "tool", Content: "→ read_file"}})
	x, y, ok := findClickTarget(m, "tool-group:"+key1)
	if !ok {
		t.Fatal("first run summary must be clickable")
	}
	m2, _ := m.Update(clickLeft(x, y))
	view := stripANSI(m2.(*AppModel).View())
	if !strings.Contains(view, "→ read_file") {
		t.Fatalf("first run must expand:\n%s", view)
	}
	if strings.Contains(view, "→ write") {
		t.Fatalf("second run must stay collapsed:\n%s", view)
	}
}

// TestToolGroup_ChildReplayGroupsToo: replayChildHistoryMessages feeds the same
// Role:"tool" messages into buildChatRows, so a 3-tool child run collapses too.
func TestToolGroup_ChildReplayGroupsToo(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			evs := []client.ProviderEvent{
				{Type: "turn_started", Prompt: "fix the bug"},
				{Type: "tool_started", ToolName: "read_file"},
				{Type: "tool_started", ToolName: "grep"},
				{Type: "tool_started", ToolName: "write"},
				{Type: "message_completed", Text: "fixed"},
			}
			m := toolGroupModel(pk)
			m.messages = replayChildHistoryMessages(evs)
			view := stripANSI(m.View())
			if !strings.Contains(view, "+ 3 tool calls") {
				t.Fatalf("%s: child transcript run must collapse:\n%s", pk, view)
			}
			if strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: collapsed child run must hide tool lines:\n%s", pk, view)
			}
		})
	}
}
