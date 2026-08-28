package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestRenderMarkdown_CodeFenceHasCopyChip(t *testing.T) {
	src := "Intro\n\n```go\nfunc Add(a, b int) int {\n\treturn a + b\n}\n```\n"
	rows := renderMarkdownRows(src, 48, true)
	joined := stripANSI(strings.Join(mdTexts(rows), "\n"))
	if !strings.Contains(joined, "[copy]") {
		t.Fatalf("code fence should show [copy]:\n%s", joined)
	}
	found := false
	for _, row := range rows {
		if row.CopyCode == "" {
			continue
		}
		found = true
		if !strings.Contains(row.CopyCode, "func Add") {
			t.Fatalf("copy payload missing code: %q", row.CopyCode)
		}
		if strings.Contains(row.CopyCode, "Intro") || strings.Contains(row.CopyCode, "```") {
			t.Fatalf("copy payload should be fence body only: %q", row.CopyCode)
		}
	}
	if !found {
		t.Fatal("expected a fence copy payload")
	}
}

func TestRenderMessages_CodeFenceCopyKeepsAnswerCopy(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	src := "hello\n\n```\nfmt.Println(1)\n```\n"
	m.addMessage("assistant", src, "")
	if _, _, ok := findClickTarget(m, "copyfence:0:0"); !ok {
		t.Fatal("expected clickable [copy] on the code fence")
	}
	if _, _, ok := findClickTarget(m, "copy:0"); ok {
		t.Fatal("trailing [copy] on answer should be removed (user request)")
	}
	var payload string
	for _, row := range m.chatRows() {
		if row.CopyText != "" {
			payload = row.CopyText
			break
		}
	}
	if payload != "fmt.Println(1)" {
		t.Fatalf("fence copy=%q", payload)
	}
}

func TestRenderMessages_TwoCodeFencesGetDistinctCopy(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 28
	m.asciiMode = true
	m.sessionLoading = false
	src := "```\none\n```\n\n```\ntwo\n```\n"
	m.addMessage("assistant", src, "")
	if _, _, ok := findClickTarget(m, "copyfence:0:0"); !ok {
		t.Fatal("missing first fence copy")
	}
	if _, _, ok := findClickTarget(m, "copyfence:0:1"); !ok {
		t.Fatal("missing second fence copy")
	}
	got := map[int]string{}
	for _, row := range m.chatRows() {
		if row.CopyText != "" {
			got[row.FenceIdx] = row.CopyText
		}
	}
	if got[0] != "one" || got[1] != "two" {
		t.Fatalf("fence payloads=%v", got)
	}
}
