package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestCopyChip_AttachesToAnswerBeforeNextChatPadding(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "first ask"},
		{Role: "assistant", Content: "first ans"},
		{Role: "user", Content: "second ask"},
	}
	lines := m.renderMessages()
	ansIdx, copyIdx, nextIdx := -1, -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		if strings.Contains(plain, "first ans") {
			ansIdx = i
		}
		if copyIdx < 0 && strings.Contains(plain, "[copy]") && ansIdx >= 0 && (nextIdx < 0 || i < nextIdx) {
			copyIdx = i
		}
		if strings.Contains(plain, "second ask") && nextIdx < 0 {
			nextIdx = i
		}
	}
	if ansIdx < 0 || copyIdx < 0 || nextIdx < 0 {
		t.Fatalf("missing ans/copy/next:\n%s", strings.Join(lines, "\n"))
	}
	if copyIdx < ansIdx || copyIdx > ansIdx+1 {
		t.Fatalf("copy must sit on the answer line or next row in the box, ans=%d copy=%d\n%s", ansIdx, copyIdx, strings.Join(lines, "\n"))
	}
	if nextIdx-copyIdx < 2 {
		t.Fatalf("padding belongs after copy, before next prompt; copy=%d next=%d\n%s", copyIdx, nextIdx, strings.Join(lines, "\n"))
	}
}

func TestRenderMarkdown_TableStaysRaw(t *testing.T) {
	src := "| Prop | Type |\n| --- | --- |\n| variant | string |\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 60), "\n"))
	if !strings.Contains(joined, "Prop") || !strings.Contains(joined, "variant") || !strings.Contains(joined, "string") {
		t.Fatalf("expected table cells:\n%s", joined)
	}
}

func TestRenderInputLine_RoundedStrokeFrame(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.sessionDefaultsLoaded = true
	m.sessionLoading = false
	m.model = "grok-4.6"
	m.inputValue = "hello"
	m.cursorOn = false
	line := m.renderInputLine()
	if !strings.Contains(line, "╭") && !strings.Contains(line, "+") {
		t.Fatalf("expected rounded/ascii stroke frame: %q", line)
	}
	if !strings.Contains(line, "hello") || !strings.Contains(strings.ToLower(stripANSI(line)), "chat") {
		t.Fatalf("missing body/title: %q", line)
	}
}
