package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CP-70 Task-402 gap closure: Devin MCP provider config writes into the
// dedicated mcp_config.json (v3000.3+) — path resolution, atomic write,
// idempotent re-run, and read-only check. Additive.

func TestDevinMcpConfigPathPrefersConfigDirHome(t *testing.T) {
	home := t.TempDir()
	configStyle := filepath.Join(home, ".config", "devin")
	if err := os.MkdirAll(configStyle, 0o755); err != nil {
		t.Fatal(err)
	}
	got := getDevinMcpConfigPath(configStyle)
	if filepath.Base(got) != "mcp_config.json" || filepath.Dir(got) != configStyle {
		t.Fatalf("config-dir style home must resolve mcp_config.json directly, got %q", got)
	}
}

func TestDevinMcpConfigPathUnderHome(t *testing.T) {
	home := t.TempDir()
	got := getDevinMcpConfigPath(home)
	if filepath.Base(got) != "mcp_config.json" || filepath.Base(filepath.Dir(got)) != "devin" {
		t.Fatalf("expected <...>/devin/mcp_config.json, got %q", got)
	}
	if !strings.HasPrefix(got, home) {
		t.Fatalf("path must stay under home, got %q", got)
	}
}

func TestDevinMcpConfigWriteAndCheck(t *testing.T) {
	home := t.TempDir()
	server := map[string]interface{}{
		"command": "npx",
		"args":    []interface{}{"-y", "@piotr-agier/google-drive-mcp"},
		"env":     map[string]interface{}{"GOOGLE_DRIVE_OAUTH_CREDENTIALS": "/tmp/cred.json"},
	}
	changed, err := ensureDevinMcpServer(home, googleDriveMcpServerName, server)
	if err != nil {
		t.Fatalf("ensureDevinMcpServer: %v", err)
	}
	if !changed {
		t.Fatal("first write must report changed=true")
	}
	ok, err := CheckDevinMcpConfig(home, googleDriveMcpServerName)
	if err != nil || !ok {
		t.Fatalf("CheckDevinMcpConfig = %v, %v; want true, nil", ok, err)
	}
	changed, err = ensureDevinMcpServer(home, googleDriveMcpServerName, server)
	if err != nil {
		t.Fatalf("second ensureDevinMcpServer: %v", err)
	}
	if changed {
		t.Fatal("idempotent re-write must report changed=false")
	}
	// Unrelated keys survive the write.
	doc, err := readDevinMcpConfig(getDevinMcpConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	doc["otherKey"] = "keep-me"
	if err := writeDevinMcpConfigAtomic(getDevinMcpConfigPath(home), doc); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureDevinMcpServer(home, "another", map[string]interface{}{"command": "x"}); err != nil {
		t.Fatal(err)
	}
	doc, err = readDevinMcpConfig(getDevinMcpConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if doc["otherKey"] != "keep-me" {
		t.Fatal("unrelated top-level key was dropped")
	}
}

func TestDevinJiraMcpConfigWriteAndCheck(t *testing.T) {
	home := t.TempDir()
	r := &Runner{}
	auth := jiraMcpAuthConfig{URL: jiraMcpOAuthRemoteURL, Authorization: "Bearer test-token"}
	res, err := r.ensureDevinJiraMcpConfig(home, auth)
	if err != nil {
		t.Fatalf("ensureDevinJiraMcpConfig: %v", err)
	}
	if res.Status != "configured" || res.ProviderKey != "devin" {
		t.Fatalf("unexpected response: %+v", res)
	}
	if filepath.Base(res.ConfigPath) != "mcp_config.json" {
		t.Fatalf("config path = %q, want mcp_config.json", res.ConfigPath)
	}
	status, err := r.checkDevinJiraMcpConfig(home)
	if err != nil {
		t.Fatalf("checkDevinJiraMcpConfig: %v", err)
	}
	if !status.Configured || status.Stale {
		t.Fatalf("expected configured+fresh, got %+v", status)
	}
	res2, err := r.ensureDevinJiraMcpConfig(home, auth)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Changed {
		t.Fatal("idempotent re-ensure must report Changed=false")
	}
}

func TestDevinJiraMcpConfigCheckMissingIsNotConfigured(t *testing.T) {
	r := &Runner{}
	status, err := r.checkDevinJiraMcpConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured {
		t.Fatal("missing mcp_config.json must not report configured")
	}
}

// CP-70 live-fix: session/load ignores the ACP mcpServers param, so the turn's
// entries are persisted to <cwd>/.devin/mcp_config.local.json instead.
func TestEnsureDevinLocalMcpServersWritesLocalScope(t *testing.T) {
	cwd := t.TempDir()
	servers := []interface{}{
		map[string]interface{}{
			"name":    "flowpilot",
			"command": `C:\bin\flowpilot-runner.exe`,
			"args":    []interface{}{"devin-mcp-stdio", "--url", "http://127.0.0.1:4317/internal/claude-permission-mcp?token=tok1"},
			"env":     []interface{}{map[string]interface{}{"name": "FOO", "value": "bar"}},
		},
	}
	if err := ensureDevinLocalMcpServers(cwd, servers); err != nil {
		t.Fatalf("ensureDevinLocalMcpServers: %v", err)
	}
	doc, err := readDevinMcpConfig(filepath.Join(cwd, ".devin", "mcp_config.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := doc["mcpServers"].(map[string]interface{})["flowpilot"].(map[string]interface{})
	if entry == nil {
		t.Fatal("flowpilot entry missing from mcp_config.local.json")
	}
	if entry["command"] != `C:\bin\flowpilot-runner.exe` || entry["transport"] != "stdio" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	env, _ := entry["env"].(map[string]interface{})
	if env["FOO"] != "bar" {
		t.Fatalf("env not converted to map: %+v", entry["env"])
	}
	// Merge: a pre-existing user server must survive the upsert.
	servers[0].(map[string]interface{})["args"] = []interface{}{"devin-mcp-stdio", "--url", "http://x?token=tok2"}
	if err := ensureDevinLocalMcpServers(cwd, servers); err != nil {
		t.Fatal(err)
	}
	doc, _ = readDevinMcpConfig(filepath.Join(cwd, ".devin", "mcp_config.local.json"))
	entry, _ = doc["mcpServers"].(map[string]interface{})["flowpilot"].(map[string]interface{})
	args, _ := entry["args"].([]interface{})
	if len(args) == 0 || !strings.Contains(args[len(args)-1].(string), "tok2") {
		t.Fatalf("entry not refreshed with new token: %+v", entry)
	}
}

func TestEnsureDevinLocalMcpServersEmptyIsNoop(t *testing.T) {
	cwd := t.TempDir()
	if err := ensureDevinLocalMcpServers(cwd, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".devin", "mcp_config.local.json")); !os.IsNotExist(err) {
		t.Fatal("no servers must not create the file")
	}
}
