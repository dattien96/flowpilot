package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOpencodeConnectedProvidersFromAuth_liveShape(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	authDir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{
  "xai": {"type":"oauth","access":"eyJhbGciOiJFUzI1NiJ9.eyJlbWFpbCI6InVzZXJAZXhhbXBsZS5jb20ifQ.sig","refresh":"r"},
  "opencode": {"type":"api","key":"sk-test"},
  "opencode-go": {"type":"api","key":"sk-test2"}
}`
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	labels, email := loadOpencodeConnectedProvidersFromAuth(home)
	if email != "user@example.com" {
		t.Fatalf("email = %q", email)
	}
	if len(labels) != 3 {
		t.Fatalf("labels = %v", labels)
	}
	got := opencodeAccountDisplayLabel(labels, email)
	if got != "user@example.com" {
		t.Fatalf("display = %q", got)
	}
}

func TestParseOpencodeProvidersListOutput(t *testing.T) {
	t.Parallel()

	raw := []byte(`┌  Credentials
●  xAI oauth
●  OpenCode Zen api
●  OpenCode Go api
└  3 credentials`)
	labels, _ := parseOpencodeProvidersListOutput(raw)
	if len(labels) != 3 {
		t.Fatalf("labels = %v", labels)
	}
}

func TestParseOpencodeModelsOutput_countsNamespaces(t *testing.T) {
	t.Parallel()

	raw := []byte("opencode/gpt-5.4\nopencode-go/gpt-5.4\nxai/grok-4.5\n")
	models, err := parseOpencodeModelsOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %+v", models)
	}
	if models[0].DisplayName == "" || models[1].DisplayName == "" {
		t.Fatalf("missing display names: %+v", models)
	}
}

func TestReadOpencodeModelsCache_roundTrip(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "opencode_models_cache.json")
	t.Setenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH", cachePath)

	models := []ProviderModel{
		{ID: "opencode/gpt-5.4", DisplayName: "gpt-5.4 (OpenCode Zen)", Source: "opencode_models"},
		{ID: "opencode-go/gpt-5.4", DisplayName: "gpt-5.4 (OpenCode Go)", Source: "opencode_models"},
	}
	writeOpencodeModelsCache(models)

	cached, ok := readOpencodeModelsCache(false)
	if !ok || len(cached) != 2 {
		t.Fatalf("cache read = %v ok=%v", len(cached), ok)
	}
}

func TestReadOpencodeModelsCache_acceptsSmallLiveCatalog(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "opencode_models_cache.json")
	t.Setenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH", cachePath)

	writeOpencodeModelsCache([]ProviderModel{{ID: "opencode-go/gpt-5.4", DisplayName: "gpt-5.4 (OpenCode Go)", Source: "opencode_models"}})
	cached, ok := readOpencodeModelsCache(false)
	if !ok || len(cached) != 1 {
		t.Fatalf("small live catalog should be cached, got ok=%v n=%d", ok, len(cached))
	}
}

func TestOpencodeModelsProbeBudget_usesParentDeadline(t *testing.T) {
	ctx, cancel := contextWithParentDeadline(8 * time.Second)
	defer cancel()
	budget := opencodeModelsProbeBudget(ctx)
	if budget < 5*time.Second {
		t.Fatalf("budget = %v, want >= 5s when parent has 8s", budget)
	}
}

func TestOpencodeDataHomeForAccount_preservesAmbientXDG(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", custom)

	got := opencodeDataHomeForAccount(home)
	if got != custom {
		t.Fatalf("ambient home data dir = %q, want %q", got, custom)
	}

	managed := filepath.Join(home, ".opencodeHome2")
	gotManaged := opencodeDataHomeForAccount(managed)
	wantManaged := filepath.Join(managed, ".local", "share")
	if gotManaged != wantManaged {
		t.Fatalf("managed home data dir = %q, want %q", gotManaged, wantManaged)
	}
}

func TestDiscoverOpencodeAccountHomes_honorsOpenCodeAuthPath(t *testing.T) {
	home := t.TempDir()
	authPath := filepath.Join(home, "custom-auth.json")
	if err := os.WriteFile(authPath, []byte(`{"opencode":{"type":"api","key":"sk-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OPENCODE_AUTH_PATH", authPath)
	t.Setenv("OPENCODE_HOME", "")
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("XDG_DATA_HOME", "")

	paths, err := discoverOpencodeAccountHomes()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Clean(home) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected home %q in discovered paths %v", home, paths)
	}
}

func contextWithParentDeadline(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
