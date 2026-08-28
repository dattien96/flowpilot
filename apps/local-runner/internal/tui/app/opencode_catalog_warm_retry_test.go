package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestNeedsOpencodeCatalogWarmRetry_installedWithFewModels(t *testing.T) {
	t.Parallel()

	providers := []client.Provider{{
		Key:       "opencode",
		Installed: true,
		Models: []client.ProviderModel{
			{ID: "opencode/gpt-5.4"},
		},
	}}
	if !needsOpencodeCatalogWarmRetry(providers) {
		t.Fatal("expected warm retry when opencode has undersized catalog")
	}
}

func TestNeedsOpencodeCatalogWarmRetry_fullCatalog(t *testing.T) {
	t.Parallel()

	models := make([]client.ProviderModel, minOpencodeCatalogModels)
	providers := []client.Provider{{Key: "opencode", Installed: true, Models: models}}
	if needsOpencodeCatalogWarmRetry(providers) {
		t.Fatal("expected no warm retry when catalog is full")
	}
}
