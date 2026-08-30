package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CA-679: OPENCODE_CONFIG must point at the opencode.json config FILE, never
// the config directory. opencode reads it with readFile and dies with
// "BadResource: FileSystem.readFile" when the value is a directory — killing
// `opencode acp` boot ("opencode initialize: opencode acp stream closed: EOF")
// and `opencode models` probes alike. Additive: the codex/grok env branches
// below are asserted unchanged (safe-fix-contract R2 parity).

func TestOpencodeConfigFilePathUnderHome(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "u")
	got := opencodeConfigFilePath(home)
	want := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got != want {
		t.Fatalf("opencodeConfigFilePath = %q, want %q", got, want)
	}
	if opencodeConfigFilePath("") != "" {
		t.Fatal("empty home must yield empty config path")
	}
	if filepath.Base(got) != "opencode.json" {
		t.Fatalf("OPENCODE_CONFIG target must be a file name, got %q", got)
	}
}

func TestOpencodeProcessEnvSetsConfigFileNotDir(t *testing.T) {
	home := t.TempDir()
	env := opencodeProcessEnv(map[string]string{"HOME": home})
	found := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG=") {
			found = strings.TrimPrefix(kv, "OPENCODE_CONFIG=")
		}
	}
	if found == "" {
		t.Fatal("opencodeProcessEnv must set OPENCODE_CONFIG when HOME is set")
	}
	if found != opencodeConfigFilePath(home) {
		t.Fatalf("OPENCODE_CONFIG = %q, want file path %q", found, opencodeConfigFilePath(home))
	}
	if strings.HasSuffix(filepath.ToSlash(found), "/opencode") {
		t.Fatalf("OPENCODE_CONFIG must not be the config directory, got %q", found)
	}
}

func TestOpencodeProcessEnvKeepsCallerConfigValue(t *testing.T) {
	home := t.TempDir()
	caller := filepath.Join(home, "custom-opencode.json")
	env := opencodeProcessEnv(map[string]string{
		"HOME":            home,
		"OPENCODE_CONFIG": caller,
	})
	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG=") {
			count++
			if got := strings.TrimPrefix(kv, "OPENCODE_CONFIG="); got != caller {
				t.Fatalf("caller OPENCODE_CONFIG overwritten: %q", got)
			}
		}
	}
	if count != 1 {
		t.Fatalf("OPENCODE_CONFIG appears %d times, want 1", count)
	}
}

func TestDiscoverOpencodeAccountHomesAcceptsConfigFileAndLegacyDir(t *testing.T) {
	root := t.TempDir()
	homeFile := filepath.Join(root, "home-file")
	legacyHome := filepath.Join(root, "home-legacy-dir")
	for _, home := range []string{homeFile, legacyHome} {
		cfgDir := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "opencode.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// New file style (CA-679).
	t.Setenv("OPENCODE_CONFIG", opencodeConfigFilePath(homeFile))
	got, err := discoverOpencodeAccountHomes()
	if err != nil {
		t.Fatalf("discover with file-style OPENCODE_CONFIG: %v", err)
	}
	if !ca679ContainsPath(got, homeFile) {
		t.Fatalf("file-style OPENCODE_CONFIG must discover %q, got %v", homeFile, got)
	}

	// Legacy directory style must keep working (no regression for operator shells).
	t.Setenv("OPENCODE_CONFIG", filepath.Join(legacyHome, ".config", "opencode"))
	got, err = discoverOpencodeAccountHomes()
	if err != nil {
		t.Fatalf("discover with dir-style OPENCODE_CONFIG: %v", err)
	}
	if !ca679ContainsPath(got, legacyHome) {
		t.Fatalf("dir-style OPENCODE_CONFIG must still discover %q, got %v", legacyHome, got)
	}
}

func ca679ContainsPath(paths []string, want string) bool {
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Clean(want) {
			return true
		}
	}
	return false
}

func TestGetEnvForExecutionOpencodeConfigFileParity(t *testing.T) {
	home := t.TempDir()
	r := &Runner{}

	env := envMap(r.getEnvForExecution("opencode", home, nil, ""))
	if got := env["OPENCODE_CONFIG"]; got != opencodeConfigFilePath(home) {
		t.Fatalf("opencode OPENCODE_CONFIG = %q, want %q", got, opencodeConfigFilePath(home))
	}
	if env["HOME"] != home || env["OPENCODE_HOME"] != home {
		t.Fatalf("opencode HOME/OPENCODE_HOME must stay the account home, got %v", env)
	}
	if env["XDG_CONFIG_HOME"] != filepath.Join(home, ".config") {
		t.Fatalf("opencode XDG_CONFIG_HOME changed: %q", env["XDG_CONFIG_HOME"])
	}

	// R2 parity: codex/grok branches unchanged by CA-679.
	codexEnv := envMap(r.getEnvForExecution("codex", home, nil, ""))
	if codexEnv["CODEX_HOME"] != home {
		t.Fatalf("codex CODEX_HOME = %q, want %q", codexEnv["CODEX_HOME"], home)
	}
	if _, ok := codexEnv["OPENCODE_CONFIG"]; ok {
		t.Fatal("codex branch must not gain OPENCODE_CONFIG")
	}
	grokEnv := envMap(r.getEnvForExecution("grok", home, nil, ""))
	if grokEnv["GROK_HOME"] != home {
		t.Fatalf("grok GROK_HOME = %q, want %q", grokEnv["GROK_HOME"], home)
	}
	if _, ok := grokEnv["OPENCODE_CONFIG"]; ok {
		t.Fatal("grok branch must not gain OPENCODE_CONFIG")
	}
}

func envMap(env []string) map[string]string {
	out := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func TestProviderEnvSetCommandOpencodeUsesConfigFile(t *testing.T) {
	posix := providerEnvSetCommand("opencode", "/home/u", "posix")
	if !strings.Contains(posix, "OPENCODE_CONFIG='/home/u/.config/opencode/opencode.json'") {
		t.Fatalf("posix export must point OPENCODE_CONFIG at the file, got %q", posix)
	}
	win := providerEnvSetCommand("opencode", `C:\Users\u`, "windows")
	if !strings.Contains(win, `OPENCODE_CONFIG=C:\Users\u\.config\opencode\opencode.json`) {
		t.Fatalf("windows set must point OPENCODE_CONFIG at the file, got %q", win)
	}
	// Parity: codex/grok shell exports unchanged.
	if strings.Contains(providerEnvSetCommand("codex", "/home/u", "posix"), "OPENCODE_CONFIG") ||
		strings.Contains(providerEnvSetCommand("grok", "/home/u", "posix"), "OPENCODE_CONFIG") {
		t.Fatal("codex/grok shell exports must not gain OPENCODE_CONFIG")
	}
}

func TestOpencodeAccountCommandEnvUsesConfigFile(t *testing.T) {
	home := t.TempDir()
	env := opencodeAccountCommandEnv(home)
	found := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG=") {
			found = strings.TrimPrefix(kv, "OPENCODE_CONFIG=")
		}
	}
	if found != opencodeConfigFilePath(home) {
		t.Fatalf("account probe OPENCODE_CONFIG = %q, want %q", found, opencodeConfigFilePath(home))
	}
}
