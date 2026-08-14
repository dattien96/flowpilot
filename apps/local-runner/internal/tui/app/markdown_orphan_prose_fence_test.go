package app

import (
	"strings"
	"testing"
)

// Regression for run-95315: nested ```markdown + ```bash with same-length fences
// leaves a stray empty-lang open fence that swallowed the closing chat sentence
// into a labeled "code" box.
func TestRenderMarkdown_OrphanEmptyLangFenceProseNotCodeBox(t *testing.T) {
	src := "Chào anh, đây là một đoạn markdown mẫu:\n\n" +
		"```markdown\n" +
		"# Gate Sandbox\n\n" +
		"Thư viện Go tối giản với hai nhóm chức năng:\n\n" +
		"| Feature key   | File        | Functions                                      |\n" +
		"|---------------|-------------|------------------------------------------------|\n" +
		"| `calc-core`   | `calc.go`   | `Add`, `Subtract`, `Multiply`, `Divide`, `Abs` |\n" +
		"| `calc-format` | `format.go` | `Sign`, `Clamp`                                |\n\n" +
		"## Chạy test\n\n" +
		"```bash\n" +
		"go test ./...\n" +
		"```\n\n" +
		"> **Lưu ý:** Không sửa file trong `.flowpilot/` — đó là state của engine.\n" +
		"```\n\n" +
		"Anh cần đoạn khác (README, commit template, CA note, v.v.) thì nói a chỉnh luôn."

	plain := stripANSI(strings.Join(renderMarkdownMode(src, 72, true), "\n"))
	if hasLabeledFenceBox(plain, "markdown") {
		t.Fatalf("outer ```markdown must unwrap:\n%s", plain)
	}
	if !hasLabeledFenceBox(plain, "bash") {
		t.Fatalf("nested ```bash must stay boxed:\n%s", plain)
	}
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("trailing prose must not land in a code box:\n%s", plain)
	}
	if !strings.Contains(plain, "Anh cần đoạn khác") {
		t.Fatalf("missing closing prose:\n%s", plain)
	}
	if !strings.Contains(plain, "Lưu ý") || !strings.Contains(plain, "Gate Sandbox") {
		t.Fatalf("missing unwrapped sample body:\n%s", plain)
	}
	if strings.Contains(plain, "```") {
		t.Fatalf("fence ticks leaked:\n%s", plain)
	}
}

func TestRenderMarkdown_EmptyLangProseOnlyFenceNotCodeBox(t *testing.T) {
	src := "Lead\n\n```\nThanks — say if you want a README template next.\n```\n"
	plain := stripANSI(strings.Join(renderMarkdownMode(src, 60, true), "\n"))
	if hasLabeledFenceBox(plain, "code") {
		t.Fatalf("plain prose empty-lang fence must not box as code:\n%s", plain)
	}
	if !strings.Contains(plain, "Thanks") || !strings.Contains(plain, "README") {
		t.Fatalf("expected prose body:\n%s", plain)
	}
}

func TestRenderMarkdown_EmptyLangCodeStillBoxedAfterProseHeuristic(t *testing.T) {
	// Guard: prose-orphan unwrap must not steal real empty-lang code.
	cases := []string{
		"```\nfmt.Println(1)\n```\n",
		"```\ngo test ./...\n```\n",
		"```\nconst x = 1\n```\n",
		"```\n{\"a\": 1}\n```\n",
	}
	for _, src := range cases {
		plain := stripANSI(strings.Join(renderMarkdownMode(src, 40, true), "\n"))
		if !hasLabeledFenceBox(plain, "code") {
			t.Fatalf("real code must stay boxed for %q:\n%s", src, plain)
		}
	}
}
