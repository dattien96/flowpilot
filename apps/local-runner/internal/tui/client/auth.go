package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SupabaseConfig mirrors GET /supabase-config (no secret).
type SupabaseConfig struct {
	APIURL            string `json:"apiUrl"`
	AnonKey           string `json:"anonKey"`
	HasServiceRoleKey bool   `json:"hasServiceRoleKey"`
	Status            string `json:"status,omitempty"`
}

// SupabaseLoginResult mirrors POST /supabase-auth/login.
type SupabaseLoginResult struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	UserID       string `json:"userId"`
	Email        string `json:"email,omitempty"`
}

// DesktopAuthSession matches Electron's supabase-auth-session.json shape
// (apps/desktop-flowpilot/electron/main.ts).
type DesktopAuthSession struct {
	ClientKey    string  `json:"clientKey"`
	AccessToken  string  `json:"accessToken"`
	RefreshToken string  `json:"refreshToken"`
	UserID       string  `json:"userId"`
	Email        *string `json:"email"`
}

// GetSupabaseConfig calls GET /supabase-config.
func (c *Client) GetSupabaseConfig(ctx context.Context) (SupabaseConfig, error) {
	var cfg SupabaseConfig
	err := c.getJSON(ctx, "/supabase-config", &cfg)
	return cfg, err
}

// LoginSupabase calls POST /supabase-auth/login (same path Desktop uses).
func (c *Client) LoginSupabase(ctx context.Context, email, password string) (SupabaseLoginResult, error) {
	var out SupabaseLoginResult
	err := c.postJSON(ctx, "/supabase-auth/login", map[string]string{
		"email":    strings.TrimSpace(email),
		"password": password,
	}, &out)
	return out, err
}

// ClientKeyForConfig builds the Desktop clientKey: apiUrl::anonKey.
func ClientKeyForConfig(cfg SupabaseConfig) string {
	return strings.TrimSpace(cfg.APIURL) + "::" + strings.TrimSpace(cfg.AnonKey)
}

// DesktopAuthSessionPaths returns candidate Electron userData session files.
func DesktopAuthSessionPaths() []string {
	var out []string
	switch runtime.GOOS {
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out,
				filepath.Join(appData, "FlowPilot", "supabase-auth-session.json"),
				filepath.Join(appData, "desktop-flowpilot", "supabase-auth-session.json"),
			)
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, "Library", "Application Support", "FlowPilot", "supabase-auth-session.json"),
				filepath.Join(home, "Library", "Application Support", "desktop-flowpilot", "supabase-auth-session.json"),
			)
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, ".config", "FlowPilot", "supabase-auth-session.json"),
				filepath.Join(home, ".config", "desktop-flowpilot", "supabase-auth-session.json"),
			)
		}
	}
	return out
}

// LoadDesktopAuthSession reads the first existing Desktop auth session file.
func LoadDesktopAuthSession() (*DesktopAuthSession, string, error) {
	for _, path := range DesktopAuthSessionPaths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, path, err
		}
		var session DesktopAuthSession
		if err := json.Unmarshal(raw, &session); err != nil {
			continue
		}
		if strings.TrimSpace(session.AccessToken) == "" || strings.TrimSpace(session.UserID) == "" {
			continue
		}
		return &session, path, nil
	}
	return nil, "", os.ErrNotExist
}

// PersistDesktopAuthSession writes tokens so Desktop can restore on next launch.
// Writes to every candidate Electron userData path (FlowPilot + desktop-flowpilot)
// because Electron uses package.json "name" (desktop-flowpilot) while older TUI
// writes only returned after the first success under FlowPilot — leaving Electron
// on the Login screen. Returns the first path written successfully.
func PersistDesktopAuthSession(session DesktopAuthSession) (string, error) {
	paths := DesktopAuthSessionPaths()
	if len(paths) == 0 {
		return "", fmt.Errorf("no desktop auth session path for this OS")
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	var (
		firstOK string
		wrote   int
		lastErr error
	)
	for _, dest := range paths {
		dir := filepath.Dir(dest)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastErr = err
			continue
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			lastErr = err
			continue
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			lastErr = err
			continue
		}
		wrote++
		if firstOK == "" {
			firstOK = dest
		}
	}
	if wrote == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("unable to persist desktop auth session")
		}
		return "", lastErr
	}
	return firstOK, nil
}

// SyncDesktopAuthSession copies an existing session file onto every candidate
// path (idempotent). Used before /settings so a TUI login is visible to Electron.
func SyncDesktopAuthSession() (string, error) {
	session, src, err := LoadDesktopAuthSession()
	if err != nil {
		return "", err
	}
	dest, err := PersistDesktopAuthSession(*session)
	if err != nil {
		return "", err
	}
	if dest != "" {
		return dest, nil
	}
	return src, nil
}

// SaveSupabaseConfigRequest holds connection parameters to save.
type SaveSupabaseConfigRequest struct {
	APIURL         string `json:"apiUrl"`
	AnonKey        string `json:"anonKey"`
	ServiceRoleKey string `json:"serviceRoleKey,omitempty"`
}

// SaveSupabaseConfig calls PUT /supabase-config.
func (c *Client) SaveSupabaseConfig(ctx context.Context, cfg SaveSupabaseConfigRequest) error {
	return c.putJSON(ctx, "/supabase-config", cfg, nil)
}
