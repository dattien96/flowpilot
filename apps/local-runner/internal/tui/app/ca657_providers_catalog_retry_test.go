package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-657: session load unlocks at 8s even when /providers is empty (CA-535).
// Providers must still backfill so /provider is not stuck on "No catalog loaded".

func TestSessionDefaults_EmptyProviders_SchedulesBackgroundRetry(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	next, cmd := m.Update(SessionDefaultsMsg{
		Provider: "grok",
		Model:    "grok-4.5",
		Projects: []client.Project{{ID: "p1", Name: "proj", Path: m.cfg.ProjectPath}},
		Project:  &client.Project{ID: "p1", Name: "proj", Path: m.cfg.ProjectPath},
	})
	am := next.(*AppModel)
	if am.sessionLoading {
		t.Fatal("empty providers must not keep sessionLoading (CA-535 unlock)")
	}
	if len(am.providers) != 0 {
		t.Fatalf("expected empty providers on first msg, got %d", len(am.providers))
	}
	if cmd == nil {
		t.Fatal("empty providers after session load must schedule background retry")
	}
	blob := ""
	for _, msg := range am.messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "Provider catalog still loading") {
		t.Fatalf("operator must see background-load notice, got:\n%s", blob)
	}

	am2, _ := am.Update(ProvidersCatalogMsg{
		Providers: []client.Provider{
			{Key: "grok", Label: "Grok", Installed: true},
			{Key: "opencode", Label: "Opencode", Installed: true},
		},
	})
	got := am2.(*AppModel)
	if len(got.providers) != 2 {
		t.Fatalf("retry must fill providers, got %d", len(got.providers))
	}
	if got.providers[1].Key != "opencode" {
		t.Fatalf("expected opencode in backfill, got %+v", got.providers)
	}
}

func TestSessionDefaults_ProvidersPresent_NoRetryNotice(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	next, _ := m.Update(SessionDefaultsMsg{
		Provider:  "grok",
		Model:     "grok-4.5",
		Providers: []client.Provider{{Key: "grok", Label: "Grok", Installed: true}},
	})
	am := next.(*AppModel)
	blob := ""
	for _, msg := range am.messages {
		blob += msg.Content + "\n"
	}
	if strings.Contains(blob, "Provider catalog still loading") {
		t.Fatalf("loaded catalog must not schedule empty-retry notice:\n%s", blob)
	}
	if len(am.providers) != 1 {
		t.Fatalf("expected 1 provider kept, got %d", len(am.providers))
	}
}
