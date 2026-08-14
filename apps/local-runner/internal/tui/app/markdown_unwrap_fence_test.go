package app

import (
	"strings"
	"testing"
)

func hasLabeledFenceBox(plain, title string) bool {
	return strings.Contains(plain, "+ "+title+" ") ||
		strings.Contains(plain, "┌ "+title+" ") ||
		strings.Contains(plain, "+ "+title+"\n") ||
		strings.Contains(plain, "┌ "+title+"\n")
}

func TestRenderMarkdown_UnwrapsMarkdownFenceAndBoxesNestedGo(t *testing.T) {
	src := "Intro\n\n```markdown\n# Heading 1\n\n## Code block\n\n```go\npackage main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n```\n\n**Tóm tắt component:**\n\n| Component | Cú pháp |\n| --- | --- |\n| Heading | `# H1` |\n```\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 72, true), "\n"))
	if hasLabeledFenceBox(plain, "markdown") {
		t.Fatalf("outer ```markdown must not become a markdown-labeled box:\n%s", plain)
	}
	if strings.Contains(plain, "# Heading 1") {
		t.Fatalf("unwrapped markdown still leaked raw heading:\n%s", plain)
	}
	if !strings.Contains(plain, "Heading 1") {
		t.Fatalf("expected styled heading:\n%s", plain)
	}
	if !hasLabeledFenceBox(plain, "go") {
		t.Fatalf("nested ```go must get its own labeled box:\n%s", plain)
	}
	if !strings.Contains(plain, "package main") || !strings.Contains(plain, "println") {
		t.Fatalf("missing go fence body:\n%s", plain)
	}
	if strings.Contains(plain, "```go") {
		t.Fatalf("nested fence ticks leaked:\n%s", plain)
	}
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("summary/table must not be stuffed into a code-labeled box:\n%s", plain)
	}
	if strings.Contains(plain, "**Tóm tắt") {
		t.Fatalf("summary markers leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "Tóm tắt") || !strings.Contains(plain, "Component") || !strings.Contains(plain, "Heading") {
		t.Fatalf("expected rendered summary/table:\n%s", plain)
	}
}

func TestRenderMarkdown_EmptyLangFenceWithTableIsNotCodeBox(t *testing.T) {
	src := "```\n**Tóm tắt component:**\n\n| Component | Cú pháp |\n| --- | --- |\n| Heading | `# H1` |\n| Bold | `**text**` |\n```\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 72, true), "\n"))
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("prose+table fence must not use a code-labeled box:\n%s", plain)
	}
	if strings.Contains(plain, "**Tóm tắt") {
		t.Fatalf("summary markers leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "Tóm tắt") || !strings.Contains(plain, "Component") {
		t.Fatalf("expected rendered summary/table:\n%s", plain)
	}
}

func TestRenderMarkdown_IndentedMarkdownDocIsNotCodeBox(t *testing.T) {
	src := "Lead\n\n    **Tóm tắt component:**\n\n    | Component | Cú pháp |\n    | --- | --- |\n    | List | `- item` |\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 72, true), "\n"))
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("indented markdown must not become a code-labeled box:\n%s", plain)
	}
	if !strings.Contains(plain, "Tóm tắt") || !strings.Contains(plain, "Component") {
		t.Fatalf("expected unwrapped indented markdown:\n%s", plain)
	}
}

func TestRenderMarkdown_RealEmptyLangCodeStillBoxed(t *testing.T) {
	src := "Intro\n\n```\nfmt.Println(1)\n```\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 40, true), "\n"))
	if !hasLabeledFenceBox(plain, "code") {
		t.Fatalf("real code fence with empty lang should stay boxed:\n%s", plain)
	}
	if !strings.Contains(plain, "fmt.Println(1)") {
		t.Fatalf("missing code body:\n%s", plain)
	}
}

func TestRenderMarkdown_LongerMarkdownFenceKeepsNestedGoAndSummary(t *testing.T) {
	src := "````markdown\n# Heading 1\n\n```go\npackage main\n```\n\n**Tóm tắt component:**\n\n| Component | Cú pháp |\n| --- | --- |\n| Heading | `# H1` |\n````\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 72, true), "\n"))
	if hasLabeledFenceBox(plain, "markdown") {
		t.Fatalf("4-backtick ```markdown must unwrap:\n%s", plain)
	}
	if !hasLabeledFenceBox(plain, "go") {
		t.Fatalf("nested ```go must be boxed:\n%s", plain)
	}
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("summary/table must not be a code box:\n%s", plain)
	}
	if strings.Contains(plain, "**Tóm tắt") || strings.Contains(plain, "# Heading 1") {
		t.Fatalf("raw markdown leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "Heading 1") || !strings.Contains(plain, "Tóm tắt") || !strings.Contains(plain, "package main") {
		t.Fatalf("missing unwrapped content:\n%s", plain)
	}
}

func TestRenderMarkdown_UnwrappedGoFenceKeepsCopyChip(t *testing.T) {
	src := "```markdown\n# Title\n\n```go\nfunc Add() {}\n```\n```\n"
	rows := renderMarkdownRows(src, 48, true)
	plain := stripANSI(strings.Join(mdTexts(rows), "\n"))
	if !strings.Contains(plain, "[copy]") {
		t.Fatalf("nested go fence should keep [copy]:\n%s", plain)
	}
	found := false
	for _, row := range rows {
		if row.CopyCode == "" {
			continue
		}
		found = true
		if !strings.Contains(row.CopyCode, "func Add") {
			t.Fatalf("copy payload missing go body: %q", row.CopyCode)
		}
		if strings.Contains(row.CopyCode, "# Title") || strings.Contains(row.CopyCode, "```") {
			t.Fatalf("copy payload should be go fence only: %q", row.CopyCode)
		}
	}
	if !found {
		t.Fatal("expected a nested go fence copy payload")
	}
}
