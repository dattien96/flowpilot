package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// CA-689b: opencode per-model reasoning variants must come from the LIVE ACP
// configOptions (set_config_option response), not the uniform guessed list —
// the real opencode UI shows hy3 with default/none/low/high only.

func TestOpencodeVariantsCapturedFromSetConfigResponse(t *testing.T) {
	variantsFile := filepath.Join(t.TempDir(), "opencode_variants_cache.json")
	t.Setenv("FLOWPILOT_OPENCODE_VARIANTS_FILE", variantsFile)

	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_var1"})
		case "session/set_config_option":
			// The live shape: model set echoes configOptions with the effort
			// select for the freshly-selected model.
			fg.reply(m["id"], map[string]any{"configOptions": []any{
				map[string]any{"id": "model", "currentValue": "opencode-go/hy3"},
				map[string]any{
					"id":           "effort",
					"currentValue": "high",
					"options": []any{
						map[string]any{"value": "default"},
						map[string]any{"value": "none"},
						map[string]any{"value": "low"},
						map[string]any{"value": "high"},
					},
				},
			}})
		case "session/set_config":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			fg.reply(m["id"], map[string]any{"stopReason": "end_turn"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	var gotModel string
	var gotEfforts []string
	a.onVariantsCaptured = func(modelID string, efforts []string, current string) {
		gotModel = modelID
		gotEfforts = efforts
		// Mirror the registry wiring: capture persists to the cache file.
		recordOpencodeModelVariants(modelID, efforts, current)
	}
	a.mu.Lock()
	a.yoloModes = map[string]bool{}
	a.mu.Unlock()

	if err := a.SendTurn(context.Background(), TurnRequest{
		RunID: "run-var", Prompt: "hi", Cwd: "/tmp", ProviderSessionID: "",
		ModelName: "opencode-go/hy3",
	}, &fakeOpencodeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if gotModel != "opencode-go/hy3" {
		t.Fatalf("captured model = %q", gotModel)
	}
	if len(gotEfforts) != 4 || gotEfforts[0] != "default" || gotEfforts[1] != "none" {
		t.Fatalf("captured efforts = %v, want default/none/low/high", gotEfforts)
	}

	// Detection merge: observed variants override the guessed uniform list.
	models := []ProviderModel{{
		ID:                        "opencode-go/hy3",
		SupportedReasoningEfforts: []string{"minimal", "low", "medium", "high", "xhigh"},
		DefaultReasoningEffort:    "medium",
	}}
	mergeOpencodeVariantOverrides(models)
	if len(models[0].SupportedReasoningEfforts) != 4 || models[0].SupportedReasoningEfforts[0] != "default" {
		t.Fatalf("merge did not apply observed variants: %+v", models[0].SupportedReasoningEfforts)
	}
	if models[0].DefaultReasoningEffort != "high" {
		t.Fatalf("merge did not apply observed default: %q", models[0].DefaultReasoningEffort)
	}

	// Persisted to disk (fresh process overlay).
	if _, err := os.Stat(variantsFile); err != nil {
		t.Fatalf("variants cache file missing: %v", err)
	}
	var disk opencodeVariantCache
	raw, _ := os.ReadFile(variantsFile)
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("variants cache json: %v", err)
	}
	if entry := disk.Variants["opencode-go/hy3"]; len(entry.Efforts) != 4 || entry.Default != "high" {
		t.Fatalf("disk entry = %+v", entry)
	}
}

func TestOpencodeEffortOptionsFromConfigMalformed(t *testing.T) {
	if efforts, _ := opencodeEffortOptionsFromConfig(nil); len(efforts) != 0 {
		t.Fatal("nil result must be empty")
	}
	if efforts, _ := opencodeEffortOptionsFromConfig(map[string]any{"configOptions": []any{}}); len(efforts) != 0 {
		t.Fatal("no effort entry must be empty")
	}
	efforts, current := opencodeEffortOptionsFromConfig(map[string]any{"configOptions": []any{
		map[string]any{"id": "effort", "currentValue": "high", "options": []any{
			map[string]any{"value": "low"}, "garbage", map[string]any{},
		}},
	}})
	if len(efforts) != 1 || efforts[0] != "low" || current != "high" {
		t.Fatalf("malformed entries must be skipped, got %v / %q", efforts, current)
	}
}
