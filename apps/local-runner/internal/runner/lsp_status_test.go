package runner

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func lspStatusGet(t *testing.T, base, path string) (int, map[string]any) {
	t.Helper()
	u := base + "/client/lsp-status"
	if path != "" {
		u += "?path=" + url.QueryEscape(path)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET lsp-status: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode lsp-status: %v", err)
	}
	return resp.StatusCode, out
}

func TestHandleLSPStatusMissingPath(t *testing.T) {
	_, srv := newTestServer(t)
	code, _ := lspStatusGet(t, srv.URL, "")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}

func TestHandleLSPStatusMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, srv := newTestServer(t)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := lspStatusGet(t, srv.URL, ws)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if out["platform"] != "golang" || out["binary"] != "gopls" {
		t.Fatalf("body = %+v", out)
	}
	if out["installed"] != false || out["warn"] != true {
		t.Fatalf("body = %+v, want missing with warning", out)
	}
	if hint, _ := out["installHint"].(string); hint == "" {
		t.Fatalf("body = %+v, want install hint", out)
	}
}

func TestHandleLSPStatusInstalled(t *testing.T) {
	binDir := t.TempDir()
	name := "gopls"
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, srv := newTestServer(t)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := lspStatusGet(t, srv.URL, ws)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if out["installed"] != true || out["warn"] != false {
		t.Fatalf("body = %+v, want installed without warning", out)
	}
}
