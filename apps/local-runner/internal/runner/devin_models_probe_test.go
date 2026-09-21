package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Task-402 follow-up: the probe replays the live boot handshake
// (initialize -> authenticate -> session/new) and must harvest the session's
// "model" config option choices into the cache.
func TestProbeDevinModelCatalogCapturesSessionModelOptions(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "devin_models_cache.json")
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", cachePath)

	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		load := func(name string) map[string]any {
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", name))
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			return res
		}
		switch m["method"] {
		case "initialize":
			fd.reply(m["id"], load("initialize_result.json"))
		case "authenticate":
			fd.reply(m["id"], map[string]any{})
		case "session/new":
			fd.reply(m["id"], load("session_new_result.json"))
		}
	})

	models, err := probeDevinModelCatalog(context.Background(), d, t.TempDir())
	if err != nil {
		t.Fatalf("probeDevinModelCatalog: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected the session's model catalog, got none")
	}
	for _, m := range models {
		if m.ID == "" || m.Source != "devin-acp" {
			t.Fatalf("unexpected model entry: %+v", m)
		}
		// The probe stores bare catalog ids; the provider layer adds devin/.
		if len(m.ID) >= 6 && m.ID[:6] == "devin/" {
			t.Fatalf("probe must store bare catalog ids, got %q", m.ID)
		}
	}

	cached := readDevinModelsCacheStale(true)
	if len(cached) != len(models) {
		t.Fatalf("cache = %d models, want %d", len(cached), len(models))
	}
}

// Task-402 follow-up: a stale catalog still beats the two static registry
// fallbacks — detection must serve it (and kick a background refresh) instead
// of collapsing to swe-2-high/adaptive every time the 30-minute TTL lapses.
func TestResolveProviderModelsDevinServesStaleCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "devin_models_cache.json")
	stale := devinModelsCacheFile{
		FetchedAt: time.Now().Add(-2 * devinModelsCacheTTL).UTC().Format(time.RFC3339Nano),
		Models: []ProviderModel{
			{ID: "claude-opus-5-high", DisplayName: "Claude Opus 5 High", Source: "devin-acp"},
			{ID: "swe-2-max", DisplayName: "SWE-2 Max", Source: "devin-acp"},
		},
	}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", cachePath)

	spec, ok := lookupProviderSpec("devin")
	if !ok {
		t.Fatal("devin providerSpec not registered")
	}
	models := resolveProviderModels(context.Background(), spec, "devin")
	if len(models) != 2 {
		t.Fatalf("expected the 2 stale-cache entries, got %d: %+v", len(models), models)
	}
	for _, m := range models {
		if len(m.ID) < 6 || m.ID[:6] != "devin/" {
			t.Fatalf("provider-facing models must carry the devin/ prefix, got %q", m.ID)
		}
	}
	if models[0].ID != "devin/claude-opus-5-high" {
		t.Fatalf("first model = %q, want devin/claude-opus-5-high", models[0].ID)
	}
}

// No cache at all: detection falls back to the static registry entries (and a
// background warm is kicked — a no-op under `go test`).
func TestResolveProviderModelsDevinNoCacheFallsBackToStatic(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", filepath.Join(t.TempDir(), "missing.json"))

	spec, ok := lookupProviderSpec("devin")
	if !ok {
		t.Fatal("devin providerSpec not registered")
	}
	models := resolveProviderModels(context.Background(), spec, "devin")
	if len(models) != len(defaultDevinProviderModels()) {
		t.Fatalf("expected the static fallback list, got %+v", models)
	}
}
