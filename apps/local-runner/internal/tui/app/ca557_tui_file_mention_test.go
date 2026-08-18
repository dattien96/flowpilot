package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestActiveAtFragment(t *testing.T) {
	q, start, ok := activeAtFragment("@ChatIn", 7)
	if !ok || start != 0 || q != "ChatIn" {
		t.Fatalf("start @ = %q %d %v", q, start, ok)
	}
	q, start, ok = activeAtFragment("see @apps/foo", 13)
	if !ok || start != 4 || q != "apps/foo" {
		t.Fatalf("mid @ = %q %d %v", q, start, ok)
	}
	if _, _, ok := activeAtFragment("a@b.ts", 6); ok {
		t.Fatal("email-style @ must not open picker")
	}
	if _, _, ok := activeAtFragment("@foo bar", 8); ok {
		t.Fatal("space after @ token must close picker")
	}
}

func TestIsAgentAtMentionKeepsStartOfPromptAgent(t *testing.T) {
	agents := []client.AgentRunSummary{{AgentName: "coder"}}
	if !isAgentAtMention(0, "coder", agents) {
		t.Fatal("start @coder is agent")
	}
	if isAgentAtMention(0, "apps/foo", agents) {
		t.Fatal("path is a file")
	}
	if isAgentAtMention(4, "coder", agents) {
		t.Fatal("mid-prompt @coder is a file search")
	}
}

func TestReplaceActiveAtWithInsertsPath(t *testing.T) {
	got := replaceActiveAtWith("see @ChatIn please", 11, "apps/foo/ChatInput.tsx")
	if got != "see apps/foo/ChatInput.tsx please" {
		t.Fatalf("got %q", got)
	}
}

func TestCollectSuggestionsOpensFilePicker(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.inputValue = "@ChatIn"
	m.workspaceFiles = []string{"apps/desktop-flowpilot/src/components/ChatInput.tsx"}
	m.workspaceFilesQuery = "ChatIn"
	items := m.collectSuggestions()
	if len(items) != 1 || items[0].kind != "file" || items[0].value != m.workspaceFiles[0] {
		t.Fatalf("items=%+v", items)
	}
}

func TestCollectSuggestionsLoadingAndEmptyFileRows(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.inputValue = "@zz"
	items := m.collectSuggestions()
	if len(items) != 1 || items[0].kind != "file" || items[0].detail != "loading files…" {
		t.Fatalf("loading=%+v", items)
	}
	m.workspaceFiles = []string{}
	m.workspaceFilesQuery = "zz"
	items = m.collectSuggestions()
	if len(items) != 1 || items[0].detail != "(no matching files)" {
		t.Fatalf("empty=%+v", items)
	}
}

func TestApplyFileMentionReplacesAtToken(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.inputValue = "look @ChatIn"
	m.applyFileMention("apps/foo/ChatInput.tsx")
	if m.inputValue != "look apps/foo/ChatInput.tsx" {
		t.Fatalf("input=%q", m.inputValue)
	}
}

func TestSuggestionAcceptValueFileIsPathOnly(t *testing.T) {
	if got := suggestionAcceptValue(suggestItem{kind: "file", value: "apps/foo.go"}); got != "apps/foo.go" {
		t.Fatalf("got %q", got)
	}
	if got := suggestionAcceptValue(suggestItem{kind: "file", value: ""}); got != "" {
		t.Fatalf("empty file should not accept, got %q", got)
	}
}

func TestCmdMaybePrefetchWorkspaceFiles(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.inputValue = "hello"
	if m.cmdMaybePrefetchWorkspaceFiles() != nil {
		t.Fatal("no @ should not prefetch")
	}
	m.inputValue = "@Chat"
	m.projectPath = "/tmp/ws"
	if m.cmdMaybePrefetchWorkspaceFiles() == nil {
		t.Fatal("active @ should prefetch")
	}
	m.workspaceFiles = []string{"a.go"}
	m.workspaceFilesQuery = "Chat"
	if m.cmdMaybePrefetchWorkspaceFiles() != nil {
		t.Fatal("same query should not refetch")
	}
}
