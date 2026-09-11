package changecontract

import "testing"

func TestBUG370_IsMarkdownDocPath(t *testing.T) {
	docs := []string{
		"change-audit/FEATURE-KEYS.md",
		"requirements/.flowpilot/vibe/tdd-signatures.md",
		"requirements/08-Task/todo/Task-912.md",
		"README.md",
		"docs/guide.md",
	}
	for _, p := range docs {
		if !IsMarkdownDocPath(p) {
			t.Errorf("%q must be markdown-doc (BUG-370)", p)
		}
	}
	code := []string{
		"snake/game.go",
		"src/extra.go",
		".flowpilot/settings/flow-rules.json",
		"Makefile",
	}
	for _, p := range code {
		if IsMarkdownDocPath(p) {
			t.Errorf("%q must still be in frozen-scope (not markdown)", p)
		}
	}
}
