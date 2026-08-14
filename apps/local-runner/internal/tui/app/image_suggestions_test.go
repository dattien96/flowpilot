package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFilterImageSuggestions_Subcommands(t *testing.T) {
	got := filterImageSuggestions("/image ", nil)
	if len(got) < 4 {
		t.Fatalf("subs=%d want >=4", len(got))
	}
	names := map[string]string{}
	for _, it := range got {
		names[it.value] = it.kind
	}
	if names["open"] != "image-sub-next" {
		t.Fatalf("open kind=%q want image-sub-next", names["open"])
	}
	if names["paste"] != "image-sub" {
		t.Fatalf("paste kind=%q", names["paste"])
	}
	// Tab accept open → trailing space for index picker
	if got := suggestionAcceptValue(suggestItem{value: "open", kind: "image-sub-next"}); got != "/image open " {
		t.Fatalf("accept open=%q", got)
	}
	if got := suggestionAcceptValue(suggestItem{value: "paste", kind: "image-sub"}); got != "/image paste" {
		t.Fatalf("accept paste=%q", got)
	}
}

func TestFilterImageSuggestions_OpenPicker(t *testing.T) {
	pending := []client.PromptAttachment{
		samplePendingAtt("a", "alpha.png"),
		samplePendingAtt("b", "beta.jpg"),
	}
	got := filterImageSuggestions("/image open ", pending)
	if len(got) != 2 {
		t.Fatalf("open list=%d want 2: %+v", len(got), got)
	}
	if got[0].value != "1" || got[0].kind != "image-open" {
		t.Fatalf("first=%+v", got[0])
	}
	if suggestionAcceptValue(got[0]) != "/image open 1" {
		t.Fatalf("accept=%q", suggestionAcceptValue(got[0]))
	}
	// Filter by name
	filtered := filterImageSuggestions("/image open beta", pending)
	if len(filtered) != 1 || !strings.Contains(filtered[0].detail, "beta") {
		t.Fatalf("filter beta: %+v", filtered)
	}
}

func TestFilterImageSuggestions_OpenEmpty(t *testing.T) {
	got := filterImageSuggestions("/image open ", nil)
	if len(got) != 1 || got[0].value != "" {
		t.Fatalf("empty open: %+v", got)
	}
	if suggestionAcceptValue(got[0]) != "" {
		t.Fatal("empty open must not be actionable")
	}
}

func TestCollectSuggestions_ImageFlow(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "/image "
	subs := m.collectSuggestions()
	if len(subs) < 4 {
		t.Fatalf("subs=%d", len(subs))
	}
	m.appendPendingAttachment(samplePendingAtt("z", "shot.png"))
	m.inputValue = "/image open "
	opens := m.collectSuggestions()
	if len(opens) != 1 || opens[0].kind != "image-open" {
		t.Fatalf("opens=%+v", opens)
	}
}
