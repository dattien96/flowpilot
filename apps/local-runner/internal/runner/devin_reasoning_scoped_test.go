package runner

import (
	"reflect"
	"testing"
)

// CP-70 follow-up: modifier-aware effort split + per-model catalog efforts.
// Additive — TestDevinSplitModelEffort/TestDevinModelForEffort pin the legacy
// helpers and stay untouched.

func TestDevinSplitModelEffortScoped(t *testing.T) {
	cases := []struct{ model, family, effort, modifiers string }{
		{"swe-2-high", "swe-2", "high", ""},
		{"claude-opus-5-high-fast", "claude-opus-5", "high", "fast"},
		{"gpt-5-6-sol-low-priority", "gpt-5-6-sol", "low", "priority"},
		{"gpt-5-6-sol-none", "gpt-5-6-sol", "none", ""},
		{"glm-5-2-max-1m", "glm-5-2", "max", "1m"},
		{"gemini-3-5-flash-minimal", "gemini-3-5-flash", "minimal", ""},
		{"fusion-claude-fable-5-1-high-sidekick-swe-2-medium", "fusion-claude-fable-5-1-high-sidekick-swe-2", "medium", ""},
		{"adaptive", "adaptive", "", ""},
		{"swe-1-6-fast", "swe-1-6-fast", "", ""},
		{"max", "max", "", ""},
		{"", "", "", ""},
	}
	for _, tc := range cases {
		fam, eff, mods := devinSplitModelEffortScoped(tc.model)
		if fam != tc.family || eff != tc.effort || mods != tc.modifiers {
			t.Fatalf("devinSplitModelEffortScoped(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.model, fam, eff, mods, tc.family, tc.effort, tc.modifiers)
		}
	}
}

// Modifier tails are preserved on remap — the fast/priority/1m variants exist
// per effort in the live catalog.
func TestDevinModelForEffortPreservesModifiers(t *testing.T) {
	cases := []struct{ model, effort, want string }{
		{"claude-opus-5-high-fast", "low", "claude-opus-5-low-fast"},
		{"gpt-5-6-sol-low-priority", "max", "gpt-5-6-sol-max-priority"},
		{"glm-5-2-max-1m", "none", "glm-5-2-none-1m"},
		{"gpt-5-6-sol-none", "high", "gpt-5-6-sol-high"},
		{"gemini-3-5-flash-minimal", "low", "gemini-3-5-flash-low"},
		{"claude-opus-5-high-fast", "high", ""}, // same effort → no-op
	}
	for _, tc := range cases {
		if got := devinModelForEffort(tc.model, tc.effort); got != tc.want {
			t.Fatalf("devinModelForEffort(%q,%q)=%q want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}

// Family siblings contribute the selectable effort set; bare-only ids get none.
func TestDevinModelEffortsByID(t *testing.T) {
	ids := []string{
		"deepseek-v4-pro-high", "deepseek-v4-pro-max",
		"claude-opus-5-low", "claude-opus-5-medium", "claude-opus-5-high",
		"claude-opus-5-xhigh", "claude-opus-5-max",
		"claude-opus-5-low-fast", "claude-opus-5-high-fast",
		"gpt-5-6-sol-none", "gpt-5-6-sol-max-priority",
		"adaptive",
	}
	got := devinModelEffortsByID(ids)

	if want := []string{"high", "max"}; !reflect.DeepEqual(got["deepseek-v4-pro-high"], want) {
		t.Fatalf("deepseek-v4-pro-high efforts=%v want %v", got["deepseek-v4-pro-high"], want)
	}
	// fast variants share the family scope but keep their modifier — the picker
	// shows the full family set, remap re-appends "-fast".
	if want := []string{"low", "high"}; !reflect.DeepEqual(got["claude-opus-5-low-fast"], want) {
		t.Fatalf("claude-opus-5-low-fast efforts=%v want %v", got["claude-opus-5-low-fast"], want)
	}
	if want := []string{"low", "medium", "high", "xhigh", "max"}; !reflect.DeepEqual(got["claude-opus-5-xhigh"], want) {
		t.Fatalf("claude-opus-5-xhigh efforts=%v want %v", got["claude-opus-5-xhigh"], want)
	}
	// (family, modifiers) scoping: -priority siblings don't leak into bare scope.
	if want := []string{"max"}; !reflect.DeepEqual(got["gpt-5-6-sol-max-priority"], want) {
		t.Fatalf("gpt-5-6-sol-max-priority efforts=%v want %v", got["gpt-5-6-sol-max-priority"], want)
	}
	if _, ok := got["adaptive"]; ok {
		t.Fatalf("adaptive must not get efforts (bare-only family), got %v", got["adaptive"])
	}
}

func TestDevinCatalogChoicesAttachEfforts(t *testing.T) {
	choices := []DevinConfigChoice{
		{Value: "swe-2-low", Name: "SWE-2 Low"},
		{Value: "swe-2-high", Name: "SWE-2 High"},
		{Value: "adaptive", Name: "Adaptive"},
	}
	models := devinCatalogChoicesToProviderModels(choices)
	if len(models) != 3 {
		t.Fatalf("models=%d", len(models))
	}
	if want := []string{"low", "high"}; !reflect.DeepEqual(models[0].SupportedReasoningEfforts, want) {
		t.Fatalf("swe-2-low efforts=%v want %v", models[0].SupportedReasoningEfforts, want)
	}
	if len(models[2].SupportedReasoningEfforts) != 0 {
		t.Fatalf("adaptive efforts=%v want none", models[2].SupportedReasoningEfforts)
	}
}

// Stale caches written before efforts existed still get derived options.
func TestDevinPrefixedProviderModelsDerivesEffortsOnStaleCache(t *testing.T) {
	cached := []ProviderModel{
		{ID: "swe-2-low", DisplayName: "SWE-2 Low"},
		{ID: "swe-2-max", DisplayName: "SWE-2 Max"},
	}
	got := devinPrefixedProviderModels(cached)
	if want := []string{"low", "max"}; !reflect.DeepEqual(got[0].SupportedReasoningEfforts, want) {
		t.Fatalf("efforts=%v want %v", got[0].SupportedReasoningEfforts, want)
	}
	if got[0].ID != "devin/swe-2-low" {
		t.Fatalf("id=%q", got[0].ID)
	}
}
