package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Task-402 (CP-70): the Devin model catalog cache. Devin's `devin models list`
// requires the REPL credential store (devin auth login), which is a different
// store from ACP PKCE — so the authoritative live catalog is the one Devin
// itself serves inside every ACP session: configOptions "model" options
// (live-verified ~380 entries, each with _meta supportsImages). The adapter
// captures it on every session/new|load (captureModelCatalog) and records it
// here; detection/status endpoints read the cache, and a dedicated probe
// (detectDevinModelsLive) refreshes it on demand.

const devinModelsCacheTTL = 30 * time.Minute

type devinModelsCacheFile struct {
	FetchedAt string          `json:"fetched_at"`
	Models    []ProviderModel `json:"models"`
}

var devinModelsWarmInFlight atomic.Bool

func devinModelsCachePath() string {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH")); override != "" {
		return override
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "devin_models_cache.json")
	}
	if homeDir := preferredUserHomeDir(); homeDir != "" {
		return filepath.Join(homeDir, ".flowpilot", "settings", "devin_models_cache.json")
	}
	return filepath.Join(".", ".flowpilot", "settings", "devin_models_cache.json")
}

func readDevinModelsCache() []ProviderModel {
	return readDevinModelsCacheStale(false)
}

func readDevinModelsCacheStale(allowStale bool) []ProviderModel {
	if runningUnderGoTest() && strings.TrimSpace(os.Getenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH")) == "" {
		return nil
	}
	path := devinModelsCachePath()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	var payload devinModelsCacheFile
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Models) == 0 {
		return nil
	}
	if !allowStale && strings.TrimSpace(payload.FetchedAt) != "" {
		fetchedAt, err := time.Parse(time.RFC3339Nano, payload.FetchedAt)
		if err != nil {
			fetchedAt, err = time.Parse(time.RFC3339, payload.FetchedAt)
		}
		if err == nil && time.Since(fetchedAt) > devinModelsCacheTTL {
			return nil
		}
	}
	return payload.Models
}

func writeDevinModelsCache(models []ProviderModel) {
	if runningUnderGoTest() && strings.TrimSpace(os.Getenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH")) == "" {
		return
	}
	if len(models) == 0 {
		return
	}
	path := devinModelsCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	payload := devinModelsCacheFile{
		FetchedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Models:    models,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// devinPrefixedProviderModels re-prefixes bare cached catalog ids with
// "devin/" for the provider inventory (Task-402): the cache stores bare ids
// because devinModelIDForACP strips the prefix before sending to the CLI.
func devinPrefixedProviderModels(models []ProviderModel) []ProviderModel {
	// Derive per-model effort options from the catalog's sibling variants even
	// for caches written before SupportedReasoningEfforts existed — otherwise a
	// stale cache would keep serving the picker fallback (every effort level on
	// every model) until the next refresh.
	efforts := devinModelEffortsByID(devinBareModelIDs(models))
	out := make([]ProviderModel, 0, len(models))
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		bare := strings.TrimPrefix(id, "devin/")
		if len(m.SupportedReasoningEfforts) == 0 {
			m.SupportedReasoningEfforts = efforts[bare]
		}
		if !strings.HasPrefix(id, "devin/") {
			id = "devin/" + id
		}
		m.ID = id
		if strings.TrimSpace(m.DisplayName) == "" {
			m.DisplayName = id
		}
		out = append(out, m)
	}
	return out
}

// devinBareModelIDs returns catalog ids without the "devin/" prefix for the
// effort-scope grouping (suffix parsing is prefix-agnostic only on bare ids).
func devinBareModelIDs(models []ProviderModel) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, strings.TrimPrefix(strings.TrimSpace(m.ID), "devin/"))
	}
	return ids
}

// devinCatalogChoicesToProviderModels maps ACP "model" config option choices
// to ProviderModel entries — ids keep the bare Devin catalog id; callers that
// display them through the provider registry add the "devin/" prefix.
func devinCatalogChoicesToProviderModels(choices []DevinConfigChoice) []ProviderModel {
	return devinCatalogChoicesToProviderModelsWithEfforts(choices, nil)
}

// devinCatalogChoicesToProviderModelsWithEfforts is the effort-aware variant:
// session-level "thought_level" option values (new-schema CLIs — Task-438)
// apply to every model, so they win over the sibling-suffix derivation; old
// catalogs (nil sessionEfforts) keep the suffix-derived effort lists.
func devinCatalogChoicesToProviderModelsWithEfforts(choices []DevinConfigChoice, sessionEfforts []string) []ProviderModel {
	models := make([]ProviderModel, 0, len(choices))
	ids := make([]string, 0, len(choices))
	for _, c := range choices {
		if strings.TrimSpace(c.Value) != "" {
			ids = append(ids, strings.TrimSpace(c.Value))
		}
	}
	efforts := devinModelEffortsByID(ids)
	for _, c := range choices {
		id := strings.TrimSpace(c.Value)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = id
		}
		supported := efforts[id]
		if len(sessionEfforts) > 0 {
			supported = sessionEfforts
		}
		models = append(models, ProviderModel{
			ID:                        id,
			DisplayName:               name,
			Available:                 true,
			Source:                    "devin-acp",
			InputImage:                devinChoiceSupportsImages(c),
			SupportedReasoningEfforts: supported,
		})
	}
	return models
}

// recordDevinModelCatalog persists the ACP-captured model catalog as
// ProviderModel entries (devinCatalogChoicesToProviderModels).
func recordDevinModelCatalog(choices []DevinConfigChoice) {
	recordDevinModelCatalogWithEfforts(choices, nil)
}

// recordDevinModelCatalogWithEfforts is the effort-aware variant used by
// captureModelCatalog — the session's thought_level values are persisted so
// detection surfaces the real reasoning knobs (Task-438).
func recordDevinModelCatalogWithEfforts(choices []DevinConfigChoice, sessionEfforts []string) {
	if len(choices) == 0 {
		return
	}
	writeDevinModelsCache(devinCatalogChoicesToProviderModelsWithEfforts(choices, sessionEfforts))
}
