package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

func TestRenderMessages_UserPromptHugsContent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 80
	m.asciiMode = true
	m.messages = []ChatMessage{{Role: "user", Content: "short ask"}}
	lines := m.renderMessages()
	if len(lines) < 3 {
		t.Fatalf("one-line prompt should be a full You box, got %v", lines)
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "You") || !strings.Contains(joined, "short ask") {
		t.Fatalf("missing You box chrome:\n%s", joined)
	}
	if !strings.Contains(joined, "+ You") && !strings.Contains(joined, "┌ You") {
		t.Fatalf("prompt should have a titled top stroke:\n%s", joined)
	}
	if !strings.Contains(joined, "+-") && !strings.Contains(joined, "└") {
		t.Fatalf("prompt should have a bottom stroke:\n%s", joined)
	}
	content := ""
	for _, line := range lines {
		plain := stripANSI(line)
		if strings.Contains(plain, "short ask") {
			content = strings.TrimSpace(plain)
			break
		}
	}
	if len([]rune(content)) > 40 {
		t.Fatalf("prompt box should hug content, got %q", content)
	}
}

func TestRenderMessages_AssistantHasNoMarkdownLabelBox(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 60
	m.asciiMode = true
	m.addMessage("assistant", "plain answer text", "")
	joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if strings.Contains(joined, "markdown") {
		t.Fatalf("plain answer must not be wrapped in a markdown box:\n%s", joined)
	}
	if !strings.Contains(joined, "plain answer text") {
		t.Fatalf("missing answer:\n%s", joined)
	}
}

func TestRenderMarkdown_CodeBlockHasLabeledBox(t *testing.T) {
	src := "Intro\n\n```go\nfunc Add(a, b int) int {\n\treturn a + b\n}\n```\n"
	lines := renderMarkdown(src, 40)
	joined := strings.Join(lines, "\n")
	plain := stripANSI(joined)
	if !strings.Contains(plain, "go") || !strings.Contains(plain, "func Add") {
		t.Fatalf("missing code box:\n%s", plain)
	}
	if !strings.Contains(plain, "+ go") && !strings.Contains(plain, "┌ go") {
		t.Fatalf("code fence should use a labeled box:\n%s", plain)
	}
	bg, ok := styleMdCodeBox.GetBackground().(lipgloss.Color)
	if !ok || string(bg) != colorCodeBg || colorCodeBg == colorBg3 {
		t.Fatalf("code box bg=%v want distinct %s", styleMdCodeBox.GetBackground(), colorCodeBg)
	}
}

func TestRenderMessages_NoExtraGapBeforeSystem(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "ask"},
		{Role: "system", Content: "Starting chat run..."},
		{Role: "assistant", Content: "ans"},
	}
	lines := m.renderMessages()
	sysIdx, ansIdx := -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		if sysIdx < 0 && strings.Contains(plain, "Starting chat run") {
			sysIdx = i
		}
		if strings.Contains(plain, "ans") {
			ansIdx = i
		}
	}
	if sysIdx < 0 || ansIdx < 0 {
		t.Fatalf("missing rows:\n%s", strings.Join(lines, "\n"))
	}
	gap := 0
	for i := sysIdx + 1; i < ansIdx; i++ {
		if strings.TrimSpace(stripANSI(lines[i])) == "" {
			gap++
		}
	}
	if gap > 1 {
		t.Fatalf("system→answer should not insert a double chat gap, gap=%d\n%s", gap, strings.Join(lines, "\n"))
	}
}
