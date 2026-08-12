// Package prefs persists last TUI session choices (provider/model) across restarts.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Session holds last-selected chat defaults for `flowpilot chat`.
type Session struct {
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

// Paths returns candidate prefs file locations (FlowPilot + desktop-flowpilot userData).
func Paths() []string {
	var out []string
	switch runtime.GOOS {
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out,
				filepath.Join(appData, "FlowPilot", "tui-session.json"),
				filepath.Join(appData, "desktop-flowpilot", "tui-session.json"),
			)
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, "Library", "Application Support", "FlowPilot", "tui-session.json"),
				filepath.Join(home, "Library", "Application Support", "desktop-flowpilot", "tui-session.json"),
			)
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, ".config", "FlowPilot", "tui-session.json"),
				filepath.Join(home, ".config", "desktop-flowpilot", "tui-session.json"),
			)
		}
	}
	return out
}

// Load reads the first existing prefs file.
func Load() (Session, string, error) {
	for _, path := range Paths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Session{}, path, err
		}
		var s Session
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		s.Provider = strings.TrimSpace(s.Provider)
		s.Model = strings.TrimSpace(s.Model)
		s.ReasoningEffort = strings.TrimSpace(s.ReasoningEffort)
		if s.Provider == "" && s.Model == "" {
			continue
		}
		return s, path, nil
	}
	return Session{}, "", os.ErrNotExist
}

// Save writes provider/model prefs to every candidate path (best-effort).
// Returns the first path written successfully.
func Save(s Session) (string, error) {
	s.Provider = strings.TrimSpace(s.Provider)
	s.Model = strings.TrimSpace(s.Model)
	s.ReasoningEffort = strings.TrimSpace(s.ReasoningEffort)
	paths := Paths()
	if len(paths) == 0 {
		return "", os.ErrNotExist
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	var (
		firstOK string
		lastErr error
	)
	for _, dest := range paths {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(dest, raw, 0o600); err != nil {
			lastErr = err
			continue
		}
		if firstOK == "" {
			firstOK = dest
		}
	}
	if firstOK == "" {
		if lastErr == nil {
			lastErr = os.ErrNotExist
		}
		return "", lastErr
	}
	return firstOK, nil
}
