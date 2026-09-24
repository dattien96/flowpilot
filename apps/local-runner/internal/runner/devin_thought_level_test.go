package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Task-438 (CP-70 follow-up): Devin's new-schema catalog collapses each model
// family to ONE id whose suffix is only the DEFAULT effort (live-verified
// 3000.11.3: "swe-2-high" is the single SWE-2 entry) and exposes the reasoning
// knob as a separate session config option "thought_level" (medium/high/max).
// FlowPilot's reasoning picker must set thought_level via
// session/set_config_option — the old effort-suffix remap ("swe-2-max") no
// longer resolves because those ids were dropped from the catalog.

// devinThoughtLevelForEffort maps a FlowPilot reasoning effort onto the
// session's advertised thought_level options: exact match wins, otherwise the
// nearest offered rank (xhigh→max; minimal/low→medium — the lowest offered).
func TestDevinThoughtLevelForEffort(t *testing.T) {
	options := []string{"medium", "high", "max"}
	cases := []struct {
		effort string
		want   string
	}{
		{"", ""},
		{"medium", "medium"},
		{"high", "high"},
		{"max", "max"},
		{"xhigh", "max"},
		{"x-high", "max"},
		{"low", "medium"},
		{"minimal", "medium"},
		{"bogus", ""},
	}
	for _, tc := range cases {
		if got := devinThoughtLevelForEffort(tc.effort, options); got != tc.want {
			t.Fatalf("devinThoughtLevelForEffort(%q)=%q want %q", tc.effort, got, tc.want)
		}
	}
	if got := devinThoughtLevelForEffort("high", nil); got != "" {
		t.Fatalf("no options → want \"\", got %q", got)
	}
}

// devinCatalogModelFor resolves a bare model id against the session's live
// catalog: exact match wins; a stale effort-suffixed id falls back to the
// same-family catalog id ("swe-2-max" → "swe-2-high"); unknown ids pass
// through unchanged so the provider error stays honest.
func TestDevinCatalogModelFor(t *testing.T) {
	catalog := []DevinConfigChoice{
		{Value: "swe-2-high", Name: "SWE-2"},
		{Value: "claude-opus-4-7-medium", Name: "Claude Opus 4.7"},
		{Value: "adaptive", Name: "Adaptive"},
	}
	cases := []struct{ in, want string }{
		{"swe-2-high", "swe-2-high"},
		{"swe-2-max", "swe-2-high"},
		{"swe-2-medium", "swe-2-high"},
		{"claude-opus-4-7-xhigh", "claude-opus-4-7-medium"},
		{"adaptive", "adaptive"},
		{"unknown-9", "unknown-9"},
	}
	for _, tc := range cases {
		if got := devinCatalogModelFor(tc.in, catalog); got != tc.want {
			t.Fatalf("devinCatalogModelFor(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if got := devinCatalogModelFor("swe-2-max", nil); got != "swe-2-max" {
		t.Fatalf("empty catalog → passthrough, got %q", got)
	}
}

// New-schema session (thought_level advertised): the turn's reasoning effort
// must arrive as set_config_option{configId:"thought_level"}, and a stale
// effort-suffixed model id must resolve to the catalog's family id — NOT be
// sent verbatim (live failure: "Invalid value 'swe-2-max' for config option
// 'model'").
func TestDevinAdapterAppliesThoughtLevelConfig(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var cfgCalls []map[string]any
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			var res map[string]any
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", "session_new_thought_level_result.json"))
			_ = json.Unmarshal(data, &res)
			fd.reply(m["id"], res)
		case "session/set_config_option":
			params, _ := m["params"].(map[string]any)
			cfgCalls = append(cfgCalls, map[string]any{"configId": params["configId"], "value": params["value"]})
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "x"})
		}
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{}
	req := TurnRequest{RunID: "run-1", Prompt: "hi", ModelName: "devin/swe-2-max", ReasoningEffort: "max", YoloMode: true}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	got := map[string]string{}
	for _, c := range cfgCalls {
		got[c["configId"].(string)] = c["value"].(string)
	}
	if got["model"] != "swe-2-high" {
		t.Fatalf("stale id must resolve to the catalog family id, got model=%q (calls %+v)", got["model"], cfgCalls)
	}
	if got["thought_level"] != "max" {
		t.Fatalf("reasoning effort must map to thought_level=max, got %+v", cfgCalls)
	}
}

// Old-schema session (no thought_level option, per-effort model ids — the
// pre-3000.11 catalog): the legacy suffix-remap path still applies effort into
// the model id and never sends a thought_level config call.
func TestDevinAdapterOldSchemaKeepsSuffixRemap(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var cfgCalls []map[string]any
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			var res map[string]any
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", "session_new_result.json"))
			_ = json.Unmarshal(data, &res)
			fd.reply(m["id"], res)
		case "session/set_config_option":
			params, _ := m["params"].(map[string]any)
			cfgCalls = append(cfgCalls, map[string]any{"configId": params["configId"], "value": params["value"]})
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "x"})
		}
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{}
	req := TurnRequest{RunID: "run-1", Prompt: "hi", ModelName: "devin/claude-opus-5-high", ReasoningEffort: "xhigh", YoloMode: true}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	var modelSet string
	thoughtSent := false
	for _, c := range cfgCalls {
		if c["configId"] == "model" {
			modelSet = c["value"].(string)
		}
		if c["configId"] == "thought_level" {
			thoughtSent = true
		}
	}
	if modelSet != "claude-opus-5-xhigh" {
		t.Fatalf("old-schema catalog keeps suffix remap, got model=%q (calls %+v)", modelSet, cfgCalls)
	}
	if thoughtSent {
		t.Fatal("old-schema session has no thought_level option — must not send it")
	}
}

// The probe harvests the session's thought_level options into every model's
// supported_reasoning_efforts — the new-schema catalog gives each family one
// id, so suffix-derived efforts alone would wrongly show a single level.
func TestProbeDevinModelCatalogCapturesThoughtLevelEfforts(t *testing.T) {
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
			fd.reply(m["id"], load("session_new_thought_level_result.json"))
		}
	})

	models, err := probeDevinModelCatalog(context.Background(), d, t.TempDir())
	if err != nil {
		t.Fatalf("probeDevinModelCatalog: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected catalog models")
	}
	want := []string{"medium", "high", "max"}
	for _, m := range models {
		if len(m.SupportedReasoningEfforts) != len(want) {
			t.Fatalf("model %q efforts = %v, want %v", m.ID, m.SupportedReasoningEfforts, want)
		}
		for i := range want {
			if m.SupportedReasoningEfforts[i] != want[i] {
				t.Fatalf("model %q efforts = %v, want %v", m.ID, m.SupportedReasoningEfforts, want)
			}
		}
	}
}
