package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoginSupabase_PostsPasswordGrant(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/supabase-auth/login" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  "at",
			"refreshToken": "rt",
			"userId":       "u1",
			"email":        "a@b.com",
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.LoginSupabase(context.Background(), " a@b.com ", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["email"] != "a@b.com" || gotBody["password"] != "secret" {
		t.Fatalf("body=%v", gotBody)
	}
	if res.UserID != "u1" || res.Email != "a@b.com" || res.AccessToken != "at" {
		t.Fatalf("res=%+v", res)
	}
}

func TestGetSupabaseConfig_Parses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/supabase-config" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiUrl":            "https://x.supabase.co",
			"anonKey":           "anon",
			"hasServiceRoleKey": true,
		})
	}))
	defer srv.Close()

	cfg, err := New(srv.URL).GetSupabaseConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "https://x.supabase.co" || !cfg.HasServiceRoleKey {
		t.Fatalf("cfg=%+v", cfg)
	}
	if ClientKeyForConfig(cfg) != "https://x.supabase.co::anon" {
		t.Fatalf("clientKey=%q", ClientKeyForConfig(cfg))
	}
}

func TestPersistAndLoadDesktopAuthSession_WindowsAPPDATA(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("APPDATA path helper is Windows-specific")
	}
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	email := "user@example.com"
	session := DesktopAuthSession{
		ClientKey:    "url::key",
		AccessToken:  "at",
		RefreshToken: "rt",
		UserID:       "uid",
		Email:        &email,
	}
	path, err := PersistDesktopAuthSession(session)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("expected write under APPDATA temp, got %s", path)
	}
	// Electron userData uses package name desktop-flowpilot — must also be written.
	electronPath := filepath.Join(dir, "desktop-flowpilot", "supabase-auth-session.json")
	if _, err := os.Stat(electronPath); err != nil {
		t.Fatalf("missing Electron session file %s: %v", electronPath, err)
	}
	got, gotPath, err := LoadDesktopAuthSession()
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "uid" || got.Email == nil || *got.Email != email {
		t.Fatalf("got=%+v path=%s", got, gotPath)
	}
	// Ensure file content matches Desktop bridge shape.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"accessToken":"at"`) {
		t.Fatalf("raw=%s", raw)
	}
}

func TestSyncDesktopAuthSession_MirrorsToAllCandidates(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("APPDATA path helper is Windows-specific")
	}
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	legacy := filepath.Join(dir, "FlowPilot", "supabase-auth-session.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"clientKey":"k","accessToken":"at","refreshToken":"rt","userId":"u1","email":"a@b.com"}`)
	if err := os.WriteFile(legacy, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncDesktopAuthSession(); err != nil {
		t.Fatal(err)
	}
	electronPath := filepath.Join(dir, "desktop-flowpilot", "supabase-auth-session.json")
	if _, err := os.Stat(electronPath); err != nil {
		t.Fatalf("sync should create %s: %v", electronPath, err)
	}
}
