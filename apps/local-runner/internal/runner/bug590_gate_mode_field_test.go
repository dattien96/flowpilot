package runner

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// BUG-590 (live run-139670): POST engine/gate-config accepted only
// `gate_mode` while GET (and the POST response) speak `gateMode`. A
// camelCase client silently no-ops — the empty value is normalized to
// "enforce" and the API returns 200. Field names must be symmetric and a
// value that resolves to neither enforce nor warn must be rejected, not
// silently rewritten.
func TestBUG590_GateModeFieldSymmetry(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.runner = &Runner{}
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".flowpilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	post := func(body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost,
			"/client/projects/p/engine/gate-config",
			bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		svc.handleSetEngineGateConfig(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	// camelCase must set the mode, not silently fall back to enforce.
	code, out := post(`{"workingDirectory":` + jq(cwd) + `,"gateMode":"warn"}`)
	if code != http.StatusOK || out["gateMode"] != "warn" {
		t.Fatalf("camelCase gateMode was dropped: code=%d out=%v", code, out)
	}

	// snake_case still works (regression for the documented field).
	code, out = post(`{"workingDirectory":` + jq(cwd) + `,"gate_mode":"enforce"}`)
	if code != http.StatusOK || out["gateMode"] != "enforce" {
		t.Fatalf("snake_case regression: code=%d out=%v", code, out)
	}

	// An unrecognized value must be rejected — never normalize to enforce.
	code, _ = post(`{"workingDirectory":` + jq(cwd) + `,"gate_mode":"bogus"}`)
	if code == http.StatusOK {
		t.Fatal("bogus gate_mode must not return 200")
	}
	if readGateMode(filepath.Join(cwd, ".flowpilot")) != "enforce" {
		t.Fatal("rejected write must not clobber the persisted mode")
	}
}

func jq(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
