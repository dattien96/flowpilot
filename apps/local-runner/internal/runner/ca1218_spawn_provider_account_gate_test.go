package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CA-1218 (live run-295277): a hub spawn_agent call carrying an explicit
// provider override resolved to a provider with no connected local account —
// the devin hub passed provider:"codex" straight from the tool-schema
// examples, the child run was created and then died at adapter construction,
// and the flow node had to be unblocked through a verdict + recovery cycle.
// The spawn must fail-closed at the tool boundary with an actionable error
// naming the connected providers, so the caller can re-pick or omit the
// override to inherit.
//
// Scope: only the explicit in.Provider channel is gated — pack-configured
// model routing and plain parent inheritance keep their existing behavior.

// isolateProviderHome points the account/auth detection at a fresh temp HOME
// so no real machine credential (~/.grok, ~/.local/share/devin/…) leaks into
// the provider-account sync — the connected set is exactly what the test
// plants via fakeAuthFile.
func isolateProviderHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	configPath := filepath.Join(home, "provider-accounts.json")
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", configPath)
	if err := os.WriteFile(configPath, []byte(`{"accounts":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return home
}

// plantCodexAuth drops a minimal auth.json that hasValidProviderAuthFile
// accepts, so the account sync marks the codex default account connected.
func plantCodexAuth(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"id_token":"fake"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestSpawnProviderOverrideUnconnectedFailsClosed(t *testing.T) {
	svc, _ := newTestServer(t)
	home := isolateProviderHome(t)
	plantCodexAuth(t, home)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:    "contract-planner",
		Prompt:   "plan the contract",
		Provider: "claude",
	})
	if spawnErr == nil {
		t.Fatal("spawnChildRun with unconnected provider override must fail")
	}
	if !strings.Contains(spawnErr.Error(), `provider "claude" has no connected local account`) {
		t.Fatalf("error must name the unconnected provider, got: %v", spawnErr)
	}
	if !strings.Contains(spawnErr.Error(), "connected:") || !strings.Contains(spawnErr.Error(), "codex") {
		t.Fatalf("error must list connected providers for re-pick, got: %v", spawnErr)
	}

	// No child row must exist — the failure happens before createRun.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for id, rs := range svc.runs {
		if rs != nil && rs.parentRunID == parent.RunID {
			t.Fatalf("failed spawn left a child run behind: %s", id)
		}
	}
}

func TestSpawnProviderOverrideConnectedAllowed(t *testing.T) {
	svc, _ := newTestServer(t)
	home := isolateProviderHome(t)
	plantCodexAuth(t, home)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:    "researcher",
		Prompt:   "summarise the repo",
		Provider: "codex",
	})
	if spawnErr != nil {
		t.Fatalf("connected provider override must spawn: %v", spawnErr)
	}
	if result.RunID == "" {
		t.Fatal("expected a child run id")
	}
	svc.mu.Lock()
	child := svc.runs[result.RunID]
	svc.mu.Unlock()
	if child == nil || child.providerKey != ProviderKeyCodex {
		t.Fatalf("child provider = %v, want codex", child)
	}
}

func TestSpawnInheritSkipsProviderAccountGate(t *testing.T) {
	svc, _ := newTestServer(t)
	// No connected accounts at all — an inheriting spawn (no explicit
	// provider override) must keep its legacy path; the gate is scoped to
	// the override channel only.
	isolateProviderHome(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Wait=true so the child's first turn settles inside the isolated HOME
	// before TempDir cleanup runs (the child's session writes would
	// otherwise race RemoveAll).
	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "researcher",
		Prompt: "summarise the repo",
		Wait:   true,
	})
	if spawnErr != nil {
		t.Fatalf("inherit spawn must not be gated: %v", spawnErr)
	}
	if result.RunID == "" {
		t.Fatal("expected a child run id")
	}
}

