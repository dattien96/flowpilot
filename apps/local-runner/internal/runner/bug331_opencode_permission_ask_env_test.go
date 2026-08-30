package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-331: opencode's default permission config is allow-all, so a YOLO-off
// turn never received a session/request_permission and wrote files with no
// approval card. Fix: opencodeLaunchEnv (the single env choke point shared by
// the turn adapter and the variants prober) always injects an
// OPENCODE_CONFIG_CONTENT overlay pinning permission edit/bash=ask; the
// per-turn YOLO/posture decision stays runner-side. Live probes (1.18.25,
// recorded in requirements/09-BugFix BUG-331) proved the overlay gates real
// writes and deep-merges without clobbering the account config file.

func TestBug331LaunchEnvFallbackCarriesPermissionOverlay(t *testing.T) {
	// No resolvable opencode account: isolated HOME + a missing accounts config
	// file force the env-HOME fallback branch of opencodeLaunchEnv.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(home, "missing", "provider-accounts.json"))

	r := &Runner{}
	scopeKey, env, err := r.opencodeLaunchEnv()
	if err != nil {
		t.Fatalf("opencodeLaunchEnv fallback: %v", err)
	}
	if !strings.HasPrefix(scopeKey, "env:") {
		t.Fatalf("fallback scopeKey = %q, want env: prefix", scopeKey)
	}
	if env[opencodePermissionOverlayEnv] != opencodePermissionOverlayJSON {
		t.Fatalf("fallback env overlay = %q, want %q", env[opencodePermissionOverlayEnv], opencodePermissionOverlayJSON)
	}
	if env["OPENCODE_CONFIG"] != opencodeConfigFilePath(home) {
		t.Fatalf("fallback OPENCODE_CONFIG = %q, want %q (CA-679 intact)", env["OPENCODE_CONFIG"], opencodeConfigFilePath(home))
	}
	if env["HOME"] != home {
		t.Fatalf("fallback HOME = %q, want %q", env["HOME"], home)
	}
}

func TestBug331LaunchEnvAccountBranchCarriesPermissionOverlay(t *testing.T) {
	home := filepath.Join(t.TempDir(), "opencode-account-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	// syncProviderAccounts re-validates non-default accounts via the local auth
	// file (CA-659 paths) and marks them failed without one — seed a minimal
	// auth.json so the account resolves as connected.
	authDir := opencodeDataDir(home)
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(`{"opencode":{"type":"api","key":"bug331-test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	accountsPath := filepath.Join(t.TempDir(), "provider-accounts.json")
	writeProviderAccountsConfig(t, accountsPath, []ProviderAccount{{
		ID:          "oc-bug331",
		ProviderKey: "opencode",
		HomePath:    home,
		AuthStatus:  "connected",
	}})

	r := &Runner{}
	scopeKey, env, err := r.opencodeLaunchEnv()
	if err != nil {
		t.Fatalf("opencodeLaunchEnv account branch: %v", err)
	}
	if scopeKey != "oc-bug331" {
		t.Fatalf("account scopeKey = %q, want oc-bug331", scopeKey)
	}
	if env[opencodePermissionOverlayEnv] != opencodePermissionOverlayJSON {
		t.Fatalf("account env overlay = %q, want %q", env[opencodePermissionOverlayEnv], opencodePermissionOverlayJSON)
	}
	// The overlay must ride ALONGSIDE the account isolation, never replace it.
	if env["OPENCODE_CONFIG"] != opencodeConfigFilePath(home) {
		t.Fatalf("account OPENCODE_CONFIG = %q, want %q", env["OPENCODE_CONFIG"], opencodeConfigFilePath(home))
	}
	if env["HOME"] != home {
		t.Fatalf("account HOME = %q, want %q", env["HOME"], home)
	}
	if env["XDG_DATA_HOME"] != opencodeDataHomeForAccount(home) {
		t.Fatalf("account XDG_DATA_HOME = %q, want %q", env["XDG_DATA_HOME"], opencodeDataHomeForAccount(home))
	}
}

func TestBug331PermissionOverlayPinsEditAndBashAskOnly(t *testing.T) {
	var top map[string]any
	if err := json.Unmarshal([]byte(opencodePermissionOverlayJSON), &top); err != nil {
		t.Fatalf("overlay JSON invalid: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("overlay must pin ONLY permission, got keys %v", top)
	}
	perm, _ := top["permission"].(map[string]any)
	if perm == nil {
		t.Fatal("overlay missing permission object")
	}
	if len(perm) != 2 {
		t.Fatalf("permission must pin exactly edit+bash (webfetch stays default for Codex/Claude/Grok parity), got %v", perm)
	}
	if perm["edit"] != "ask" || perm["bash"] != "ask" {
		t.Fatalf("permission = %v, want edit=ask bash=ask", perm)
	}
}

func TestBug331ProcessEnvKeepsOverlayAndConfigFileDistinct(t *testing.T) {
	home := t.TempDir()
	env := opencodeProcessEnv(map[string]string{
		"HOME":                        home,
		opencodePermissionOverlayEnv: opencodePermissionOverlayJSON,
	})
	overlayCount, configCount := 0, 0
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, opencodePermissionOverlayEnv+"="):
			overlayCount++
			if got := strings.TrimPrefix(kv, opencodePermissionOverlayEnv+"="); got != opencodePermissionOverlayJSON {
				t.Fatalf("overlay mangled: %q", got)
			}
		case strings.HasPrefix(kv, "OPENCODE_CONFIG="):
			// OPENCODE_CONFIG_CONTENT= must NOT satisfy the OPENCODE_CONFIG= prefix.
			configCount++
		}
	}
	if overlayCount != 1 {
		t.Fatalf("OPENCODE_CONFIG_CONTENT appears %d times, want 1", overlayCount)
	}
	if configCount != 1 {
		t.Fatalf("OPENCODE_CONFIG appears %d times, want 1 (CA-679 coexistence)", configCount)
	}
}

// TestBug331OverlaySurvivesAccountExtraEnv (CA-690 review): account.ExtraEnv
// is applied before the overlay, so a stale/custom account env carrying
// OPENCODE_CONFIG_CONTENT must never ungate the process.
func TestBug331OverlaySurvivesAccountExtraEnv(t *testing.T) {
	home := filepath.Join(t.TempDir(), "opencode-account-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	authDir := opencodeDataDir(home)
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(`{"opencode":{"type":"api","key":"bug331-test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	accountsPath := filepath.Join(t.TempDir(), "provider-accounts.json")
	writeProviderAccountsConfig(t, accountsPath, []ProviderAccount{{
		ID:          "oc-bug331-evil",
		ProviderKey: "opencode",
		HomePath:    home,
		AuthStatus:  "connected",
		ExtraEnv: map[string]string{
			opencodePermissionOverlayEnv: `{"permission":{"edit":"allow","bash":"allow"}}`,
			// Sentinel: proves ExtraEnv really loads and spreads onto the map,
			// so the overlay assertion below proves LAST-WRITE, not that the
			// ExtraEnv loop silently never ran (CA-690 review).
			"FP_BUG331_EXTRA_ENV_SENTINEL": "loaded",
		},
	}})

	r := &Runner{}
	_, env, err := r.opencodeLaunchEnv()
	if err != nil {
		t.Fatalf("opencodeLaunchEnv: %v", err)
	}
	if env["FP_BUG331_EXTRA_ENV_SENTINEL"] != "loaded" {
		t.Fatal("account ExtraEnv must still load and spread (sentinel missing) — without it the overlay assertion is vacuous")
	}
	if env[opencodePermissionOverlayEnv] != opencodePermissionOverlayJSON {
		t.Fatalf("account ExtraEnv must not defeat the ask-gate overlay, got %q", env[opencodePermissionOverlayEnv])
	}
}

// TestBug331ProcessEnvStripsHostOverlayContent (CA-690 review): an ambient
// host OPENCODE_CONFIG_CONTENT must not sneak through os.Environ — the gate
// travels only via extraEnv.
func TestBug331ProcessEnvStripsHostOverlayContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv(opencodePermissionOverlayEnv, `{"permission":{"edit":"allow"}}`)

	ungated := opencodeProcessEnv(map[string]string{"HOME": home})
	for _, kv := range ungated {
		if strings.HasPrefix(kv, opencodePermissionOverlayEnv+"=") {
			t.Fatalf("host overlay value must be stripped when extraEnv does not carry it: %q", kv)
		}
	}

	gated := opencodeProcessEnv(map[string]string{
		"HOME":                       home,
		opencodePermissionOverlayEnv: opencodePermissionOverlayJSON,
	})
	count := 0
	for _, kv := range gated {
		if strings.HasPrefix(kv, opencodePermissionOverlayEnv+"=") {
			count++
			if got := strings.TrimPrefix(kv, opencodePermissionOverlayEnv+"="); got != opencodePermissionOverlayJSON {
				t.Fatalf("extraEnv overlay mangled: %q", got)
			}
		}
	}
	if count != 1 {
		t.Fatalf("overlay appears %d times in spawn env, want 1", count)
	}
}

// TestBug331ReuseStillIgnoresModelVariantAutoWithOverlay guards the BUG-329
// contract under the new always-ask env: the overlay is process env only, so
// a YOLO/model/effort change must still REUSE the live same-scope handle
// (respawning on toggle would orphan sessions again).
func TestBug331ReuseStillIgnoresModelVariantAutoWithOverlay(t *testing.T) {
	r, h1, _ := bug329LiveHandle("acct-1", "opencode/muse-spark-1.2-contributor-free")
	for _, tc := range []struct {
		model, variant string
		auto           bool
	}{
		{"opencode-go/deepseek-v4-flash", "medium", false},
		{"opencode-go/deepseek-v4-flash", "xhigh", true},
	} {
		got, err := r.ensureOpencodeProcess(context.Background(), "acct-1", "/tmp",
			map[string]string{opencodePermissionOverlayEnv: opencodePermissionOverlayJSON},
			tc.model, tc.variant, tc.auto)
		if err != nil {
			t.Fatalf("ensure model=%s variant=%s auto=%v: %v", tc.model, tc.variant, tc.auto, err)
		}
		if got != h1 {
			t.Fatalf("model/variant/auto change must reuse the live same-scope handle (BUG-329), got a different one for %+v", tc)
		}
	}
	if len(r.opencodeProcesses) != 1 {
		t.Fatalf("reuse must not grow the process map, got %d entries", len(r.opencodeProcesses))
	}
}
