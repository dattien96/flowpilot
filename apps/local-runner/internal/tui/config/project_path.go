package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeProjectPathInput trims quotes and unifies separators so paths from
// Windows/Linux/macOS shells and `just` recipes all parse the same way.
// Examples accepted equivalently:
//
//	D:\working\gate-sandbox
//	D:/working/gate-sandbox
//	"/Users/me/gate-sandbox"
func NormalizeProjectPathInput(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.Trim(p, `"'`)
	if p == "" {
		return ""
	}
	// Convert Windows backslashes before filepath so bash/just handoff cannot
	// leave a half-escaped path. filepath.FromSlash then applies OS separators.
	p = strings.ReplaceAll(p, `\`, `/`)
	return filepath.FromSlash(p)
}

// ResolveProjectPath returns an absolute, cleaned project directory.
// Empty raw falls back to the process working directory (direct `flowpilot chat`
// when already inside the target repo). `just chat-dev` should always pass
// --project explicitly because its cwd is the FlowPilot checkout.
func ResolveProjectPath(raw string) (string, error) {
	p := NormalizeProjectPathInput(raw)
	if p == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve project cwd: %w", err)
		}
		return cwd, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve project path %q: %w", raw, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project path %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path %q is not a directory", abs)
	}
	return abs, nil
}
