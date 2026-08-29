package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CA-689b: per-model Opencode reasoning variants observed LIVE from ACP
// set_config_option responses. `opencode models` (CLI) cannot report per-model
// variants — every model was stamped with one guessed list, which lied for
// models like opencode-go/hy3 (real variants: default/none/low/high). Each
// chat turn against a model records that model's real effort options here;
// detectOpencodeModels then overrides its guess with the observed truth.

type opencodeVariantEntry struct {
	Efforts []string `json:"efforts"`
	Default string   `json:"default,omitempty"`
	// SeenAt keeps the freshest observation first (debugging aid).
	SeenAt string `json:"seen_at,omitempty"`
}

type opencodeVariantCache struct {
	Variants map[string]opencodeVariantEntry `json:"variants"`
}

var (
	opencodeVariantMu sync.Mutex
	opencodeVariantInMemory = map[string]opencodeVariantEntry{}
)

func opencodeVariantsCachePath() string {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_OPENCODE_VARIANTS_FILE")); override != "" {
		return override
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "opencode_variants_cache.json")
	}
	if homeDir := preferredUserHomeDir(); homeDir != "" {
		return filepath.Join(homeDir, ".flowpilot", "settings", "opencode_variants_cache.json")
	}
	return filepath.Join(".", ".flowpilot", "settings", "opencode_variants_cache.json")
}

// recordOpencodeModelVariants stores the observed variant list (memory + disk).
func recordOpencodeModelVariants(modelID string, efforts []string, current string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || len(efforts) == 0 {
		return
	}
	opencodeVariantMu.Lock()
	opencodeVariantInMemory[modelID] = opencodeVariantEntry{
		Efforts: efforts,
		Default: strings.TrimSpace(current),
		SeenAt:  nowRFC3339Nano(),
	}
	snapshot := opencodeVariantCache{Variants: map[string]opencodeVariantEntry{}}
	for k, v := range opencodeVariantInMemory {
		snapshot.Variants[k] = v
	}
	opencodeVariantMu.Unlock()

	if path := opencodeVariantsCachePath(); path != "" {
		if raw, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, raw, 0o600)
		}
	}
}

// cachedOpencodeModelVariants loads observed variants (file first, then the
// in-memory overlay for this process lifetime).
func cachedOpencodeModelVariants() map[string]opencodeVariantEntry {
	merged := map[string]opencodeVariantEntry{}
	if path := opencodeVariantsCachePath(); path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			var disk opencodeVariantCache
			if json.Unmarshal(raw, &disk) == nil && disk.Variants != nil {
				for k, v := range disk.Variants {
					merged[k] = v
				}
			}
		}
	}
	opencodeVariantMu.Lock()
	defer opencodeVariantMu.Unlock()
	for k, v := range opencodeVariantInMemory {
		merged[k] = v
	}
	return merged
}

// mergeOpencodeVariantOverrides stamps observed per-model variants onto the
// detected catalog list (used by detectOpencodeModels).
func mergeOpencodeVariantOverrides(models []ProviderModel) {
	observed := cachedOpencodeModelVariants()
	if len(observed) == 0 {
		return
	}
	for i := range models {
		if entry, ok := observed[models[i].ID]; ok && len(entry.Efforts) > 0 {
			models[i].SupportedReasoningEfforts = entry.Efforts
			if entry.Default != "" {
				models[i].DefaultReasoningEffort = entry.Default
			}
		}
	}
}
