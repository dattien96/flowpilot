package runner

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveClaudeHTTPMCPDenyBlocks is the live end-to-end check (07 live-acceptance): it
// stands up the real runner-hosted MCP server over httptest, runs the REAL `claude` binary
// with --permission-prompt-tool pointing at it, and asserts the approve tool was invoked
// over HTTP and that a DENY actually blocked a Write (no file created).
//
// Gated behind FLOWPILOT_LIVE_CLAUDE=1 because it spawns the real binary (consumes the
// subscription's included allowance — no extra/overage billing). Skipped in normal `go test`.
func TestLiveClaudeHTTPMCPDenyBlocks(t *testing.T) {
	if os.Getenv("FLOWPILOT_LIVE_CLAUDE") == "" {
		t.Skip("set FLOWPILOT_LIVE_CLAUDE=1 to run the billed-to-allowance live e2e")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("claude binary not on PATH: %v", err)
	}

	// 1. Runner-hosted MCP server + a deny bridge, exposed over HTTP.
	srv := newClaudeMCPServer()
	bridge := &fakeClaudeBridge{approval: "deny"}
	token := srv.register(bridge, true)
	defer srv.unregister(token)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 2. Clean claude config dir (auth + gating posture, no broad allow-rules).
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	creds, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		t.Skipf("no claude credentials to drive a live run: %v", err)
	}
	cfgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfgDir, ".credentials.json"), creds, 0o600); err != nil {
		t.Fatalf("seed creds: %v", err)
	}
	if err := ensureClaudeConfigSettings(cfgDir); err != nil {
		t.Fatalf("ensure settings: %v", err)
	}

	// 3. Per-turn --mcp-config pointing at the httptest MCP endpoint with this turn's token.
	cfgPath, cleanup, err := writeClaudeMCPConfig(ts.URL, token, nil)
	if err != nil {
		t.Fatalf("write mcp-config: %v", err)
	}
	defer cleanup()

	// 4. Run real claude with a gated Write in a temp cwd.
	cwd := t.TempDir()
	target := filepath.Join(cwd, "spike-live.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p",
		"Use the Write tool to create a file named spike-live.txt in the current directory with content hi. Do nothing else, then stop.",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "default",
		"--permission-prompt-tool", "mcp__flowpilot__approve",
		"--mcp-config", cfgPath, "--strict-mcp-config")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+cfgDir)
	out, runErr := cmd.CombinedOutput()
	t.Logf("claude exit=%v\n%s", runErr, string(out))

	// 5. Assertions: approve tool reached our bridge over HTTP, and deny blocked the write.
	bridge.mu.Lock()
	calls := len(bridge.approvalCalls)
	bridge.mu.Unlock()
	if calls == 0 {
		t.Fatalf("approve tool was never called over HTTP MCP — transport not exercised")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("DENY should have blocked the Write, but %s exists", target)
	}
}
