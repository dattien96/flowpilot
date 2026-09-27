package runner

import (
	"path/filepath"
	"testing"
)

func TestCodexScopedPoolRegistryReturnsLiveAdapter(t *testing.T) {
	t.Setenv(codexAppServerEnvFlag, "1")
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	writeCodexAuth(t, home)
	writeProviderAccountsConfig(t, filepath.Join(root, "accounts.json"), []ProviderAccount{
		{ID: "codex-1", ProviderKey: string(ProviderKeyCodex), HomePath: home, AuthStatus: "connected", IsActive: true},
	})
	defer mockCodexInitProcess(t)()
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.CleanupSessions()
	adapter, err := ProviderRegistryFor(r).Adapter(ProviderKeyCodex, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if failed, ok := adapter.(errorAdapter); ok {
		t.Fatalf("live codex adapter setup failed: %v", failed.err)
	}
	if _, ok := adapter.(*codexAdapter); !ok {
		t.Fatalf("adapter=%T want *codexAdapter", adapter)
	}
}
