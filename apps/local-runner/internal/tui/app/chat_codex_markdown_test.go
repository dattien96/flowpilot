package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestRenderMarkdown_HeadingsListsAndInline(t *testing.T) {
	src := "# Heading\n\nUse `code` and **bold** and *italic*.\n\n- item one\n\nSee [docs](https://example.com)."
	joined := stripANSI(strings.Join(renderMarkdown(src, 40), "\n"))
	for _, want := range []string{"Heading", "bold", "italic", "item one", "docs", "code"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q:\n%s", want, joined)
		}
	}
	for _, raw := range []string{"# Heading", "**bold**", "[docs]("} {
		if strings.Contains(joined, raw) {
			t.Fatalf("raw marker %q leaked:\n%s", raw, joined)
		}
	}
}

func TestRenderMarkdown_FenceUnclosedDoesNotPanic(t *testing.T) {
	src := "Intro\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 40), "\n"))
	if !strings.Contains(joined, "func main()") {
		t.Fatalf("unclosed fence dropped body:\n%s", joined)
	}
}

func TestRenderMarkdown_TableFitsOrPipeFallback(t *testing.T) {
	src := "| Prop | Type |\n| --- | --- |\n| variant | string |\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 60), "\n"))
	if !strings.Contains(joined, "Prop") || !strings.Contains(joined, "variant") || !strings.Contains(joined, "string") {
		t.Fatalf("table missing cells:\n%s", joined)
	}
}

func TestRenderMarkdown_CacheHitOnUnchangedWidthAndHash(t *testing.T) {
	src := "# CacheMe\n\nA **bold** paragraph.\n"
	before := markdownParseCount()
	_ = renderMarkdown(src, 48)
	mid := markdownParseCount()
	_ = renderMarkdown(src, 48)
	after := markdownParseCount()
	if mid-before != 1 {
		t.Fatalf("first call should parse once, delta=%d", mid-before)
	}
	if after != mid {
		t.Fatalf("second call should hit cache, parse %d -> %d", mid, after)
	}
}

func TestChatRows_ScrollDoesNotIncrementMarkdownParseCount(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.addMessage("user", "scroll-md-ask", "")
	m.addMessage("assistant", "# ScrollMD\n\nA **bold** paragraph.\n\n```go\nfunc scroll() {}\n```\n", "")
	_ = m.chatRows()
	n := markdownParseCount()
	m.viewport.offset = 3
	_ = m.View()
	_ = m.chatRows()
	if markdownParseCount() != n {
		t.Fatalf("scroll/View re-parsed markdown (%d -> %d)", n, markdownParseCount())
	}
}

func TestCopyChip_StillCopiesRawMarkdownSource(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 80
	src := "# Title\n\nUse **bold** and `code`.\n"
	m.addMessage("assistant", src, "")
	joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if !strings.Contains(joined, "[copy]") {
		t.Fatalf("missing copy chip:\n%s", joined)
	}
	if m.messages[0].Content != src {
		t.Fatalf("copy source mutated: %q", m.messages[0].Content)
	}
	if !strings.Contains(m.messages[0].Content, "# Title") || !strings.Contains(m.messages[0].Content, "**bold**") {
		t.Fatal("raw markdown source must remain on the message")
	}
}

func TestRenderMarkdown_HugeFenceIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("```\n")
	for i := 0; i < 250; i++ {
		b.WriteString("line-")
		b.WriteString(strings.Repeat("x", 8))
		b.WriteByte('\n')
	}
	b.WriteString("```\n")
	lines := renderMarkdown(b.String(), 80)
	if len(lines) > 160 {
		t.Fatalf("fence was not bounded, lines=%d", len(lines))
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "truncated") {
		t.Fatalf("expected truncation marker:\n%s", joined)
	}
}
