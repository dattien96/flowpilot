package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Task-210 (CP-46 P-8): account model additive-switch coverage + base-
// regression guard (every switch below appends a "grok" case; the assertions
// on codex/claude/gemini prove their prior values are byte-identical).

func TestGrokProviderSpecRegistered(t *testing.T) {
	spec, ok := lookupProviderSpec("grok")
	if !ok {
		t.Fatal("expected a registered grok providerSpec")
	}
	if spec.BinaryName != "grok" {
		t.Fatalf("expected BinaryName=grok, got %q", spec.BinaryName)
	}
	if len(spec.Models) == 0 {
		t.Fatal("expected at least one default grok model")
	}
}

func TestManagedProviderHomePrefixBaseRegression(t *testing.T) {
	cases := map[string]string{"codex": ".codexHome", "claude": ".claudeHome", "gemini": ".geminiHome", "grok": ".grokHome"}
	for key, want := range cases {
		got, ok := managedProviderHomePrefix(key)
		if !ok || got != want {
			t.Errorf("managedProviderHomePrefix(%q) = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
}

func TestNextAccountHomePathGrokUsesGrokHomePrefix(t *testing.T) {
	path, slot, err := NextAccountHomePath("grok", nil)
	if err != nil {
		t.Fatalf("NextAccountHomePath: %v", err)
	}
	if slot != 1 {
		t.Fatalf("expected slot 1, got %d", slot)
	}
	if !strings.Contains(path, ".grokHome1") {
		t.Fatalf("expected path to contain .grokHome1, got %q", path)
	}
}

func TestProviderKeyFromModelBaseRegressionPlusGrok(t *testing.T) {
	cases := []struct {
		model string
		want  ProviderKey
	}{
		{"gpt-5.4-mini", ProviderKeyCodex},
		{"gemini-3.5-flash-medium", ProviderKeyGemini},
		{"auto-gemini-3", ProviderKeyGemini},
		{"claude-sonnet", ProviderKeyClaude},
		{"grok-4.5", ProviderKeyGrok},
		{"grok-build", ProviderKeyGrok},
	}
	for _, c := range cases {
		got, ok := providerKeyFromModel(c.model)
		if !ok || got != c.want {
			t.Errorf("providerKeyFromModel(%q) = (%q, %v), want (%q, true)", c.model, got, ok, c.want)
		}
	}
}

func TestDefaultModelForProviderBaseRegressionPlusGrok(t *testing.T) {
	if got := defaultModelForProvider(ProviderKeyCodex); got != "gpt-5.4-mini" {
		t.Errorf("codex default model changed: got %q", got)
	}
	if got := defaultModelForProvider(ProviderKeyClaude); got != "sonnet" {
		t.Errorf("claude default model changed: got %q", got)
	}
	if got := defaultModelForProvider(ProviderKeyGrok); got == "" {
		t.Error("expected a non-empty default model for grok")
	}
}

func TestIsProviderUsageLimitErrorBaseRegressionPlusGrok402(t *testing.T) {
	if !isProviderUsageLimitError(errors.New("usage limit reached")) {
		t.Error("existing 'usage limit reached' classification regressed")
	}
	if !isProviderUsageLimitError(errors.New("rate limit exceeded")) {
		t.Error("existing 'rate limit' classification regressed")
	}
	if !isProviderUsageLimitError(errors.New("402 personal-team-blocked:spending-limit")) {
		t.Error("expected the live-observed Grok 402 signature to classify as a usage-limit error")
	}
	if isProviderUsageLimitError(errors.New("network timeout")) {
		t.Error("an unrelated error must not be classified as a usage-limit error")
	}
}

func TestGetEnvForExecutionSetsGrokHomeAndStripsInheritedCodexHome(t *testing.T) {
	env := (&Runner{}).getEnvForExecution("grok", "/tmp/grok-home", nil, "")
	has := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if !has("GROK_HOME=/tmp/grok-home") {
		t.Fatalf("expected GROK_HOME to be set, got %v", env)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			t.Fatalf("CODEX_HOME must not leak into a Grok execution env: %v", env)
		}
	}
}

func TestGetEnvForExecutionCodexBaseRegression(t *testing.T) {
	env := (&Runner{}).getEnvForExecution("codex", "/tmp/codex-home", nil, "")
	has := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if !has("CODEX_HOME=/tmp/codex-home") {
		t.Fatalf("codex getEnvForExecution regressed: %v", env)
	}
}

func TestHasValidProviderAuthFileGrokRecognizesLiveShape(t *testing.T) {
	// Mirrors the real ~/.grok/auth.json shape captured live during Task-206/210
	// authoring: a map keyed by "issuer::userId" with email/refresh_token.
	dir := t.TempDir()
	authPath := dir + "/auth.json"
	content := `{"https://auth.x.ai::abc": {"email":"user@example.com","refresh_token":"xyz","team_id":""}}`
	if err := os.WriteFile(authPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !hasValidProviderAuthFile("grok", authPath) {
		t.Fatal("expected the live-shaped grok auth.json to be recognized as valid")
	}
}

// ---- Task-213: detectGrokModels -------------------------------------------

// liveGrokModelsCacheFixture mirrors the real ~/.grok/models_cache.json
// captured live (Grok Build 0.2.93, Task-213 authoring; reasoning_efforts/
// supports_reasoning_effort/reasoning_effort added Task-215 from a second
// live capture), with an extra hidden/unsupported entry added to prove the
// filter works.
const liveGrokModelsCacheFixture = `{
  "fetched_at": "2026-07-09T23:54:35.533125300Z",
  "grok_version": "0.2.93",
  "auth_method": "session",
  "origin": "https://cli-chat-proxy.grok.com/v1/models",
  "models": {
    "grok-4.5": {
      "info": {
        "id": "grok-4.5",
        "name": "Grok 4.5",
        "context_window": 500000,
        "hidden": false,
        "supported_in_api": true,
        "reasoning_effort": "high",
        "supports_reasoning_effort": true,
        "reasoning_efforts": [
          {"id": "high", "value": "high", "label": "High Effort", "default": true},
          {"id": "medium", "value": "medium", "label": "Medium Effort", "default": false},
          {"id": "low", "value": "low", "label": "Low Effort", "default": false}
        ]
      }
    },
    "grok-internal-preview": {
      "info": {
        "id": "grok-internal-preview",
        "name": "Grok Internal Preview",
        "hidden": true,
        "supported_in_api": true
      }
    },
    "grok-legacy": {
      "info": {
        "id": "grok-legacy",
        "name": "Grok Legacy",
        "hidden": false,
        "supported_in_api": false
      }
    }
  }
}`

func writeGrokModelsCacheFixture(t *testing.T, content string) {
	t.Helper()
	grokHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(grokHome, "models_cache.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write models_cache.json fixture: %v", err)
	}
	t.Setenv("GROK_HOME", grokHome)
}

func TestDetectGrokModelsFiltersHiddenAndUnsupported(t *testing.T) {
	writeGrokModelsCacheFixture(t, liveGrokModelsCacheFixture)

	models, err := detectGrokModels()
	if err != nil {
		t.Fatalf("detectGrokModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected exactly 1 non-hidden, supported model, got %d: %+v", len(models), models)
	}
	if models[0].ID != "grok-4.5" || models[0].DisplayName != "Grok 4.5" {
		t.Fatalf("unexpected model: %+v", models[0])
	}
	if models[0].Source != "grok_models_cache" {
		t.Fatalf("expected source=grok_models_cache, got %q", models[0].Source)
	}
	if models[0].ContextWindowTokens != 500000 {
		t.Fatalf("expected context_window_tokens=500000, got %d", models[0].ContextWindowTokens)
	}
	if models[0].DefaultReasoningEffort != "high" {
		t.Fatalf("expected default_reasoning_effort=high, got %q", models[0].DefaultReasoningEffort)
	}
	wantEfforts := []string{"high", "medium", "low"}
	if !reflect.DeepEqual(models[0].SupportedReasoningEfforts, wantEfforts) {
		t.Fatalf("expected supported_reasoning_efforts=%v, got %v", wantEfforts, models[0].SupportedReasoningEfforts)
	}
}

func TestDetectGrokModelsMissingCacheReturnsError(t *testing.T) {
	t.Setenv("GROK_HOME", t.TempDir()) // no models_cache.json written
	if _, err := detectGrokModels(); err == nil {
		t.Fatal("expected an error when models_cache.json does not exist")
	}
}

func TestResolveProviderModelsGrokFallsBackToStaticListOnDetectFailure(t *testing.T) {
	t.Setenv("GROK_HOME", t.TempDir()) // no cache file -> detectGrokModels fails
	spec, ok := lookupProviderSpec("grok")
	if !ok {
		t.Fatal("expected a registered grok providerSpec")
	}
	models := resolveProviderModels(context.Background(), spec, "grok")
	if len(models) == 0 {
		t.Fatal("expected the static default model list as a fallback")
	}
}

func TestResolveProviderModelsGrokUsesDetectedCache(t *testing.T) {
	writeGrokModelsCacheFixture(t, liveGrokModelsCacheFixture)
	spec, ok := lookupProviderSpec("grok")
	if !ok {
		t.Fatal("expected a registered grok providerSpec")
	}
	models := resolveProviderModels(context.Background(), spec, "grok")
	if len(models) != 1 || models[0].ID != "grok-4.5" {
		t.Fatalf("expected the detected cache list, got %+v", models)
	}
}
