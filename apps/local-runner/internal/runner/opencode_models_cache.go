package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const opencodeModelsCacheTTL = 30 * time.Minute

type opencodeModelsCacheFile struct {
	FetchedAt string          `json:"fetched_at"`
	Models    []ProviderModel `json:"models"`
}

var opencodeModelsWarmInFlight atomic.Bool

func opencodeModelsCachePath() string {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH")); override != "" {
		return override
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "opencode_models_cache.json")
	}
	if homeDir := preferredUserHomeDir(); homeDir != "" {
		return filepath.Join(homeDir, ".flowpilot", "settings", "opencode_models_cache.json")
	}
	return filepath.Join(".", ".flowpilot", "settings", "opencode_models_cache.json")
}

func readOpencodeModelsCache(allowStale bool) ([]ProviderModel, bool) {
	if runningUnderGoTest() && strings.TrimSpace(os.Getenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH")) == "" {
		return nil, false
	}
	path := opencodeModelsCachePath()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	var payload opencodeModelsCacheFile
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Models) == 0 {
		return nil, false
	}
	if !allowStale && strings.TrimSpace(payload.FetchedAt) != "" {
		fetchedAt, err := time.Parse(time.RFC3339Nano, payload.FetchedAt)
		if err != nil {
			fetchedAt, err = time.Parse(time.RFC3339, payload.FetchedAt)
		}
		if err == nil && time.Since(fetchedAt) > opencodeModelsCacheTTL {
			return nil, false
		}
	}
	return payload.Models, true
}

func writeOpencodeModelsCache(models []ProviderModel) {
	if runningUnderGoTest() && strings.TrimSpace(os.Getenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH")) == "" {
		return
	}
	if len(models) == 0 {
		return
	}
	path := opencodeModelsCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	payload := opencodeModelsCacheFile{
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

func warmOpencodeModelsCacheAsync() {
	if runningUnderGoTest() {
		return
	}
	if !opencodeModelsWarmInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer opencodeModelsWarmInFlight.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, err := detectOpencodeModelsLive(ctx, 18*time.Second)
		if err != nil || len(models) == 0 {
			return
		}
		writeOpencodeModelsCache(models)
	}()
}

func runningUnderGoTest() bool {
	return strings.Contains(filepath.Base(os.Args[0]), ".test")
}
