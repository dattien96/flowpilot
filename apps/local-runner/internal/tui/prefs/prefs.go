// Package prefs persists last TUI session choices (provider/model/mode/flow) across restarts.
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
	// Yolo is the chat-mode YOLO toggle (nil = never set). Flow mode is always
	// auto-on and does not rewrite this preference.
	Yolo *bool `json:"yolo,omitempty"`
	// Mode is chat | flow | step (Mode.String()).
	Mode string `json:"mode,omitempty"`
	// Flow identity when Mode is flow/step (empty in chat mode).
	FlowRef    string `json:"flowRef,omitempty"`    // builtin pack ref
	WorkflowID string `json:"workflowId,omitempty"` // catalog workflow id
	FlowLabel  string `json:"flowLabel,omitempty"`  // human name for status
}

// Paths returns candidate prefs file locations (FlowPilot + desktop-flowpilot userData).
func Paths() []string {
	if envPath := strings.TrimSpace(os.Getenv("FLOWPILOT_TUI_SESSION_FILE")); envPath != "" {
		return []string{envPath}
	}
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
		normalize(&s)
		if !s.hasAny() {
			continue
		}
		return s, path, nil
	}
	return Session{}, "", os.ErrNotExist
}

// Save writes session prefs to every candidate path (best-effort).
// Returns the first path written successfully.
func Save(s Session) (string, error) {
	normalize(&s)
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

func normalize(s *Session) {
	s.Provider = strings.TrimSpace(s.Provider)
	s.Model = strings.TrimSpace(s.Model)
	s.ReasoningEffort = strings.TrimSpace(s.ReasoningEffort)
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	s.FlowRef = strings.TrimSpace(s.FlowRef)
	s.WorkflowID = strings.TrimSpace(s.WorkflowID)
	s.FlowLabel = strings.TrimSpace(s.FlowLabel)
	// Chat mode must not keep a stale flow arm on disk.
	if s.Mode == "" || s.Mode == "chat" {
		s.FlowRef = ""
		s.WorkflowID = ""
		s.FlowLabel = ""
	}
}

func (s Session) hasAny() bool {
	return s.Provider != "" || s.Model != "" || s.Mode != "" ||
		s.FlowRef != "" || s.WorkflowID != "" || s.ReasoningEffort != "" ||
		s.Yolo != nil
}
