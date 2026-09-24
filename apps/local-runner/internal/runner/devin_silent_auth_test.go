package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// F-2 (user request): when the account's credentials.toml already holds a
// windsurf_api_key, authenticate must take the silent `windsurf-api-key`
// method (API key via _meta — live-verified: "ACP: API key provided
// directly via authenticate meta", ~0.5s, no browser) instead of always
// opening the PKCE browser flow. No key → devin-browser as before.

// stubDevinAuthCaptureSpawn scripts an ACP server that records every
// authenticate request line into captureFile, then replies per-script:
// failFirst optionally returns a JSON-RPC error for the first authenticate
// (to exercise the browser fallback), then succeeds.
func stubDevinAuthCaptureSpawn(t *testing.T, captureFile string, failFirst bool) {
	t.Helper()
	orig := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		var s strings.Builder
		s.WriteString(shellReadLine())
		s.WriteString(shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"browser"}]}}`))
		if failFirst {
			s.WriteString("IFS= read -r authline || true\n")
			s.WriteString("printf '%s\\n' \"$authline\" >> " + shellQuote(captureFile) + "\n")
			s.WriteString(shellOutputLine(`{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"invalid api key"}}`))
			s.WriteString("IFS= read -r authline || true\n")
			s.WriteString("printf '%s\\n' \"$authline\" >> " + shellQuote(captureFile) + "\n")
			s.WriteString(shellOutputLine(`{"jsonrpc":"2.0","id":3,"result":{}}`))
		} else {
			s.WriteString("IFS= read -r authline || true\n")
			s.WriteString("printf '%s\\n' \"$authline\" >> " + shellQuote(captureFile) + "\n")
			s.WriteString(shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{}}`))
		}
		s.WriteString("sleep 30\n")
		return testShellCommand(ctx, s.String())
	}
	t.Cleanup(func() { commandContextFn = orig })
}

func readAuthCapture(t *testing.T, captureFile string) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(captureFile)
		if err == nil && len(b) > 0 {
			var out []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				var m map[string]any
				if err := json.Unmarshal([]byte(line), &m); err == nil {
					out = append(out, m)
				}
			}
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no authenticate request captured at %s", captureFile)
	return nil
}

// Key present → silent windsurf-api-key with _meta.api_key, no browser.
func TestEnsureDevinProcessSilentAuthWhenAPIKeyPresent(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome-key")
	writeDevinCredentials(t, home)
	capture := filepath.Join(root, "auth-capture.txt")
	stubDevinAuthCaptureSpawn(t, capture, false)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	if _, err := r.ensureDevinProcess(context.Background(), "acct-key", "", map[string]string{"HOME": home}, "", ""); err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}

	auths := readAuthCapture(t, capture)
	params, _ := auths[0]["params"].(map[string]any)
	if got, _ := params["methodId"].(string); got != "windsurf-api-key" {
		t.Fatalf("expected silent windsurf-api-key auth, got %q", got)
	}
	meta, _ := params["_meta"].(map[string]any)
	if got, _ := meta["api_key"].(string); got != "test" {
		t.Fatalf("expected _meta.api_key from credentials.toml, got %q", got)
	}
	if len(auths) != 1 {
		t.Fatalf("silent auth should not need a second authenticate call, got %d", len(auths))
	}
}

// No key → unchanged devin-browser behavior.
func TestEnsureDevinProcessBrowserAuthWhenNoKey(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome-nokey")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, "auth-capture2.txt")
	stubDevinAuthCaptureSpawn(t, capture, false)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	if _, err := r.ensureDevinProcess(context.Background(), "acct-nokey", "", map[string]string{"HOME": home}, "", ""); err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}

	auths := readAuthCapture(t, capture)
	params, _ := auths[0]["params"].(map[string]any)
	if got, _ := params["methodId"].(string); got != "devin-browser" {
		t.Fatalf("no stored key must fall back to devin-browser, got %q", got)
	}
}

// Silent auth RPC error → retry authenticate with devin-browser so the user
// still gets the web login (the requested fallback contract).
func TestEnsureDevinProcessSilentAuthFailureFallsBackToBrowser(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome-badkey")
	writeDevinCredentials(t, home)
	capture := filepath.Join(root, "auth-capture3.txt")
	stubDevinAuthCaptureSpawn(t, capture, true)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	if _, err := r.ensureDevinProcess(context.Background(), "acct-badkey", "", map[string]string{"HOME": home}, "", ""); err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}

	auths := readAuthCapture(t, capture)
	if len(auths) != 2 {
		t.Fatalf("expected silent-then-browser auth, got %d authenticate calls", len(auths))
	}
	p0, _ := auths[0]["params"].(map[string]any)
	p1, _ := auths[1]["params"].(map[string]any)
	if got, _ := p0["methodId"].(string); got != "windsurf-api-key" {
		t.Fatalf("first authenticate should be silent windsurf-api-key, got %q", got)
	}
	if got, _ := p1["methodId"].(string); got != "devin-browser" {
		t.Fatalf("fallback authenticate should be devin-browser, got %q", got)
	}
}
