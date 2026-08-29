package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// CP-57 Task-302 gap closure: the opencode install command and the
// opencode.json MCP one-click writer had complete wiring but zero tests
// (DOD-named TestOpencodeMcpWrite). Additive — no pre-existing test is edited.

func TestOpencodeProviderInstallCommand(t *testing.T) {
	spec, ok := lookupProviderSpec("opencode")
	if !ok {
		t.Fatal("expected an opencode provider spec")
	}
	command, args, err := providerInstallCommand(spec)
	if err != nil {
		t.Fatalf("providerInstallCommand(opencode): %v", err)
	}
	if command != "npm" {
		t.Fatalf("install command = %q, want npm", command)
	}
	want := []string{"install", "-g", "opencode-ai@latest"}
	if len(args) != len(want) {
		t.Fatalf("install args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("install args = %v, want %v", args, want)
		}
	}
}

func TestOpencodeMcpWriteGoogleDriveAndJiraIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := EnsureOpencodeGoogleDriveMcpConfig(home, false); err != nil {
		t.Fatalf("EnsureOpencodeGoogleDriveMcpConfig: %v", err)
	}
	if err := EnsureOpencodeJiraMcpConfig(home, false); err != nil {
		t.Fatalf("EnsureOpencodeJiraMcpConfig: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("opencode.json must be written under <home>/.config/opencode: %v", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("opencode.json must stay valid JSON: %v", err)
	}
	servers, _ := doc["mcpServers"].(map[string]interface{})
	if servers == nil {
		t.Fatal("mcpServers missing after one-click write")
	}
	for _, name := range []string{"google-drive", "jira"} {
		srv, ok := servers[name].(map[string]interface{})
		if !ok {
			t.Fatalf("mcpServers.%s missing, got %v", name, servers)
		}
		if srv["type"] != "stdio" {
			t.Fatalf("mcpServers.%s type = %v, want stdio", name, srv["type"])
		}
	}
	if ok, err := CheckOpencodeMcpConfig(home, "google-drive"); err != nil || !ok {
		t.Fatalf("CheckOpencodeMcpConfig(google-drive) = %v, %v; want true, nil", ok, err)
	}

	// Idempotent second write must not duplicate or corrupt (same input → same file).
	if err := EnsureOpencodeGoogleDriveMcpConfig(home, false); err != nil {
		t.Fatalf("second EnsureOpencodeGoogleDriveMcpConfig: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	var doc2 map[string]interface{}
	if err := json.Unmarshal(after, &doc2); err != nil {
		t.Fatalf("second write must keep valid JSON: %v", err)
	}
	servers2, _ := doc2["mcpServers"].(map[string]interface{})
	if len(servers2) != len(servers) {
		t.Fatalf("second write must not duplicate servers, got %v", servers2)
	}
}

func TestOpencodeMcpConfigPathPrefersOpencodeJSON(t *testing.T) {
	home := t.TempDir()
	if got := getOpencodeMcpConfigPath(home); filepath.Base(got) != "opencode.json" || filepath.ToSlash(got) != filepath.ToSlash(filepath.Join(home, ".config", "opencode", "opencode.json")) {
		t.Fatalf("default config path = %q, want the opencode.json file", got)
	}
	// Windows home layouts still resolve via filepath.Join (OS separators).
	if runtime.GOOS == "windows" {
		if got := getOpencodeMcpConfigPath(`C:\Users\u`); !jsonContains([]byte(got), "opencode.json") {
			t.Fatalf("windows path must end in opencode.json, got %q", got)
		}
	}
}
