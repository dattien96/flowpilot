package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Live re-verification (2026-07-10, real logged-in Grok Build account, per
// explicit request to re-test against a real account). These checks read
// local files / run `--version`-class commands only — no LLM API calls, no
// cost — so unlike grok_live_chat_test.go they are NOT gated behind
// FLOWPILOT_LIVE_GROK and run in the normal suite. They degrade to a Skip
// (not a failure) on a machine with no real grok installation/account, so
// they never break CI or a contributor's machine without one.

// TestLiveRealMachineDetectGrokModels runs the real detectGrokModels against
// this machine's actual ~/.grok/models_cache.json (no fixture). Caught a real
// second model (grok-composer-2.5-fast) beyond the grok-4.5 assumed
// elsewhere, proving the parser handles more than one cached model.
func TestLiveRealMachineDetectGrokModels(t *testing.T) {
	skipIfNoRealGrokAccount(t)
	models, err := detectGrokModels()
	if err != nil {
		t.Skipf("no real models_cache.json on this machine: %v", err)
	}
	for _, m := range models {
		t.Logf("live model: id=%q displayName=%q source=%q", m.ID, m.DisplayName, m.Source)
	}
	if len(models) == 0 {
		t.Fatal("models_cache.json exists but detectGrokModels returned zero models")
	}
	for _, m := range models {
		if m.Source != "grok_models_cache" {
			t.Fatalf("expected every model's Source to be grok_models_cache, got %+v", m)
		}
	}
}

// TestLiveRealMachineDetectGrokProvider runs the real detectProvider end to
// end (providerAuthStatus + resolveProviderModels + DiscoverProviderAccounts)
// against this machine's actual grok installation/account.
func TestLiveRealMachineDetectGrokProvider(t *testing.T) {
	skipIfNoRealGrokAccount(t)
	spec, ok := lookupProviderSpec("grok")
	if !ok {
		t.Fatal("no grok providerSpec registered")
	}
	provider := detectProvider(context.Background(), spec)
	b, _ := json.MarshalIndent(provider, "", "  ")
	t.Logf("live provider: %s", string(b))

	if !provider.Installed {
		t.Fatal("expected grok to be detected as installed on this machine")
	}
	if provider.AuthStatus != "READY" {
		t.Fatalf("expected AuthStatus=READY (real logged-in account), got %q", provider.AuthStatus)
	}
	for _, m := range provider.Models {
		if m.Source != "grok_models_cache" {
			t.Fatalf("expected real detected models (source=grok_models_cache), got model %+v — "+
				"the static fallback list leaked through instead of the live detector", m)
		}
	}
}

// TestLiveRealGrokConfigPermissionMode is a read-only check of this machine's
// real ~/.grok/config.toml, documenting whatever permission_mode is currently
// set (informational — either value is a valid account state, this just
// pins that grokConfigPermissionModeBypassesGating parses the real file
// without error). See grok_process.go's grokConfigPermissionModeBypassesGating
// doc comment for the live finding this function exists to surface: a real
// account with permission_mode="always-approve" let a write tool call
// execute with ZERO session/request_permission round-trip.
func TestLiveRealGrokConfigPermissionMode(t *testing.T) {
	grokHome := skipIfNoRealGrokAccount(t)
	mode, bypasses := grokConfigPermissionModeBypassesGating(grokHome)
	t.Logf("real ~/.grok config.toml permission_mode=%q bypasses=%v", mode, bypasses)
}

// skipIfNoRealGrokAccount skips the calling test when this machine has no
// real, locally-authenticated grok account, and returns the resolved
// ~/.grok home path otherwise.
func skipIfNoRealGrokAccount(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot resolve home dir")
	}
	grokHome := filepath.Join(home, ".grok")
	if !HasLocalAuthAtPath("grok", grokHome) {
		t.Skip("no real, locally-authenticated grok account on this machine")
	}
	return grokHome
}
