package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestRenderMessages_PadsBetweenPromptAndAnswer(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "hello right"},
		{Role: "assistant", Content: "hello left"},
	}
	lines := m.renderMessages()
	userIdx, assistIdx := -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		if userIdx < 0 && strings.Contains(plain, "hello right") {
			userIdx = i
		}
		if strings.Contains(plain, "hello left") {
			assistIdx = i
		}
	}
	if userIdx < 0 || assistIdx < 0 {
		t.Fatalf("missing bubbles:\n%s", strings.Join(lines, "\n"))
	}
	if assistIdx-userIdx < 2 {
		t.Fatalf("want a gap between prompt and answer, user=%d assist=%d\n%s", userIdx, assistIdx, strings.Join(lines, "\n"))
	}
	gap := 0
	for i := userIdx + 1; i < assistIdx; i++ {
		if strings.TrimSpace(stripANSI(lines[i])) == "" {
			gap++
		}
	}
	if gap < 1 {
		t.Fatalf("want a blank row between prompt and answer, gap=%d", gap)
	}
}

func TestRenderMessages_PadsBetweenAnswerAndNextPrompt(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "first ask"},
		{Role: "assistant", Content: "first ans"},
		{Role: "user", Content: "second ask"},
	}
	lines := m.renderMessages()
	ansIdx, nextIdx := -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		if strings.Contains(plain, "first ans") {
			ansIdx = i
		}
		if strings.Contains(plain, "second ask") && nextIdx < 0 {
			nextIdx = i
		}
	}
	if ansIdx < 0 || nextIdx < 0 || nextIdx <= ansIdx {
		t.Fatalf("missing turns:\n%s", strings.Join(lines, "\n"))
	}
	gap := 0
	for i := ansIdx + 1; i < nextIdx; i++ {
		if strings.TrimSpace(stripANSI(lines[i])) == "" {
			gap++
		}
	}
	if gap < 1 {
		t.Fatalf("want a blank row between answer and next prompt, gap=%d\n%s", gap, strings.Join(lines, "\n"))
	}
}

func TestRenderMarkdown_KeepsRawHeadingsAndFences(t *testing.T) {
	src := "# Title\n\n## Section\n\nA **bold** paragraph.\n\n```go\nfn()\n```\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 80), "\n"))
	if strings.Contains(joined, "# Title") || strings.Contains(joined, "## Section") || strings.Contains(joined, "```go") {
		t.Fatalf("raw heading/fence leaked:\n%s", joined)
	}
	if !strings.Contains(joined, "Title") || !strings.Contains(joined, "Section") || !strings.Contains(joined, "fn()") {
		t.Fatalf("missing styled content:\n%s", joined)
	}
}

func TestRenderMarkdown_KeepsRawCodeStars(t *testing.T) {
	src := "Use glob `**/*.go`.\n\n```\npointer **p = 0;\n```\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 80), "\n"))
	if strings.Contains(joined, "```") {
		t.Fatalf("fence leaked:\n%s", joined)
	}
	if !strings.Contains(joined, "pointer **p") {
		t.Fatalf("code stars must stay inside the fence:\n%s", joined)
	}
}
