package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// customCatalogStore is a test CatalogStore returning canned data or an error.
type customCatalogStore struct {
	projects  []Project
	workflows []Workflow
	err       error
}

func (c customCatalogStore) ListProjects(context.Context) ([]Project, error) {
	return c.projects, c.err
}
func (c customCatalogStore) ListWorkflows(context.Context) ([]Workflow, error) {
	return c.workflows, c.err
}
func (c customCatalogStore) ListSteps(context.Context, string) ([]Step, error) {
	return nil, c.err
}

func newCatalogTestServer(t *testing.T, catalog CatalogStore) *httptest.Server {
	t.Helper()
	svc := NewInteractiveServiceWith(DefaultProviderRegistry(), catalog)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// The service serves projects from the injected CatalogStore, not the fake catalog.
func TestServiceUsesInjectedCatalogStore(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{
		projects: []Project{{ID: "p9", Name: "My Real Project", Path: "/work/real"}},
	})
	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("projects status=%d", status)
	}
	if !strings.Contains(string(body), "My Real Project") || strings.Contains(string(body), "Acme Web App") {
		t.Fatalf("expected injected projects (not the fake catalog), got %s", body)
	}
}

// The desktop workflow selector mirrors Admin Web's /workflows screen: workflows
// are a global catalog and are not scoped to the selected project.
func TestServiceListsInjectedWorkflowsGlobally(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{
		workflows: []Workflow{{ID: "wf9", Name: "Global Workflow"}},
	})
	status, body := doJSON(t, "GET", srv.URL+"/client/workflows", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Global Workflow") {
		t.Fatalf("expected injected global workflows, got status=%d body=%s", status, body)
	}
}

// A catalog read error surfaces as a typed 502 catalog_unavailable.
func TestServiceCatalogErrorSurfaces(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{err: context.DeadlineExceeded})
	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusBadGateway || !strings.Contains(string(body), "catalog_unavailable") {
		t.Fatalf("expected 502 catalog_unavailable, got status=%d body=%s", status, body)
	}
}

// Skills still come from the local catalog regardless of the projects store.
func TestSkillsServedLocallyWithInjectedStore(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{})
	status, body := doJSON(t, "GET", srv.URL+"/client/provider-skills", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "architect") {
		t.Fatalf("skills status=%d body=%s", status, body)
	}
}

// CatalogStoreFor falls back to the fake catalog with no runner / no Supabase config.
func TestCatalogStoreForFallsBackToFake(t *testing.T) {
	if _, ok := CatalogStoreFor(nil).(*interactiveCatalog); !ok {
		t.Fatal("nil runner should yield the fake catalog")
	}
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := CatalogStoreFor(r).(*interactiveCatalog); !ok {
		t.Fatal("a runner with no Supabase config should yield the fake catalog")
	}
}

// CatalogStoreFor returns the live Supabase store once a workspace config is present.
func TestCatalogStoreForUsesSupabaseWhenConfigured(t *testing.T) {
	tmp := t.TempDir()
	r, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Write a minimal config with an anon key (no secret needed for the anon path).
	cfg := SupabaseWorkspaceConfig{Version: 1, APIURL: "https://proj.supabase.co", AnonKey: "anon-key"}
	raw, _ := json.Marshal(cfg)
	dir := filepath.Join(tmp, ".flowpilot", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "supabase-config.json"), raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, ok := CatalogStoreFor(r).(*SupabaseCatalogStore); !ok {
		t.Fatalf("a configured runner should yield SupabaseCatalogStore, got %T", CatalogStoreFor(r))
	}
}
