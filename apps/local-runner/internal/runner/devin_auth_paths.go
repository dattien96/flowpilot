package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// devinCredentialFilePaths returns credentials.toml locations relative to a
// single account home. Devin is XDG-based on unix — the binary honors
// XDG_CONFIG_HOME/XDG_DATA_HOME and has no ~/Library/Application Support
// layout (verified against devin.exe strings) — while Windows resolves to
// %APPDATA%\devin (Roaming, live-verified on 3000.10.31) and possibly
// %LOCALAPPDATA%\devin. Deliberately env-free so managed slots never match
// the host's credential dirs — ambient overrides live in
// devinAmbientAuthCandidates (mirrors opencodeAuthFilePaths).
func devinCredentialFilePaths(homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	homePath = filepath.Clean(homePath)

	paths := []string{
		filepath.Join(homePath, ".local", "share", "devin", "credentials.toml"),
		filepath.Join(homePath, ".config", "devin", "credentials.toml"),
		// Managed slots may keep the store directly at the home root.
		filepath.Join(homePath, "credentials.toml"),
	}
	if runtime.GOOS == "windows" {
		paths = append(paths,
			filepath.Join(homePath, "AppData", "Roaming", "devin", "credentials.toml"),
			filepath.Join(homePath, "AppData", "Local", "devin", "credentials.toml"),
		)
	}
	return dedupeFilePaths(paths)
}

// devinAmbientAuthCandidates returns credentials.toml paths that may live
// outside the passed home but still belong to the user's ambient Devin
// install (XDG_DATA_HOME / XDG_CONFIG_HOME, %APPDATA% / %LOCALAPPDATA% on
// Windows). Only used while probing real user homes (getPossibleHomeDirs),
// not managed slots — mirrors opencodeAmbientAuthCandidates.
func devinAmbientAuthCandidates(homePath string) []authCandidate {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	homePath = filepath.Clean(homePath)

	candidates := make([]authCandidate, 0, 4)
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		candidates = append(candidates, authCandidate{
			homePath: homePath,
			authPath: filepath.Join(xdg, "devin", "credentials.toml"),
		})
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		candidates = append(candidates, authCandidate{
			homePath: homePath,
			authPath: filepath.Join(xdg, "devin", "credentials.toml"),
		})
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			candidates = append(candidates, authCandidate{
				homePath: homePath,
				authPath: filepath.Join(appData, "devin", "credentials.toml"),
			})
		}
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			candidates = append(candidates, authCandidate{
				homePath: homePath,
				authPath: filepath.Join(localAppData, "devin", "credentials.toml"),
			})
		}
	}
	return candidates
}
