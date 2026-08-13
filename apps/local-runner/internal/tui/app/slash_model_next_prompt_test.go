package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestSlashModel_SaysNextPromptUsesModel(t *testing.T) {
	m := New(config.ChatConfig{Provider: "claude", Model: "sonnet"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{
		Key:    "claude",
		Models: []client.ProviderModel{{ID: "sonnet"}, {ID: "opus"}},
	}}
	_, _ = m.handleSlashCommand("/model opus")
	got := m.messages[len(m.messages)-1].Content
	if !strings.Contains(got, "Model set to: opus") {
		t.Fatalf("missing set prefix: %q", got)
	}
	if !strings.Contains(got, "next prompt uses this model") {
		t.Fatalf("should say next prompt, not /new-only: %q", got)
	}
	if strings.Contains(got, "Grok starts a new provider session") {
		t.Fatalf("Claude must not get a Grok ACP note: %q", got)
	}
}

func TestSlashModel_GrokKeepsSameCopyAsOtherProviders(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-a"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{
		Key:    "grok",
		Models: []client.ProviderModel{{ID: "grok-a"}, {ID: "grok-b"}},
	}}
	_, _ = m.handleSlashCommand("/model grok-b")
	got := m.messages[len(m.messages)-1].Content
	if !strings.Contains(got, "Model set to: grok-b") {
		t.Fatalf("missing set prefix: %q", got)
	}
	if !strings.Contains(got, "next prompt uses this model") {
		t.Fatalf("Grok /model should match other providers: %q", got)
	}
	if strings.Contains(got, "Grok starts a new provider session") {
		t.Fatalf("must not claim a new ACP session (that drops Grok history): %q", got)
	}
}
