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

// DevinCredentialFilePaths exports devinCredentialFilePaths for the cli
// package's account-metadata loader (CP-70 usage/status). Home-relative only —
// ambient env dirs stay out of managed slots by design (see
// accountAuthPaths).
func DevinCredentialFilePaths(homePath string) []string {
	return devinCredentialFilePaths(homePath)
}

// devinWellKnownBinaryPaths returns the install locations the official Devin
// CLI setup scripts use (F-12): Windows `irm static.devin.ai/cli/setup.ps1`
// lands at %LOCALAPPDATA%\devin\cli\bin\devin.exe and does NOT add the dir to
// PATH, so a healthy install still fails bare LookPath("devin"). The unix
// install.sh drops a ~/.local/bin/devin symlink pointing at
// _versions/current/bin/devin — both are candidates because ~/.local/bin is
// not universally on PATH either.
func devinWellKnownBinaryPaths() []string {
	var paths []string
	if runtime.GOOS == "windows" {
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			paths = append(paths, filepath.Join(localAppData, "devin", "cli", "bin", "devin.exe"))
		}
		if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
			paths = append(paths, filepath.Join(profile, "AppData", "Local", "devin", "cli", "bin", "devin.exe"))
		}
		return dedupeFilePaths(paths)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, ".local", "bin", "devin"),
			filepath.Join(home, ".local", "share", "devin", "_versions", "current", "bin", "devin"),
		)
	}
	return dedupeFilePaths(paths)
}

// resolveDevinBinaryPath locates the devin binary for detect/spawn/probe:
// FLOWPILOT_DEVIN_BIN (via devinBinaryName) when it is an existing path,
// PATH lookup next, then the well-known install locations. Returns "" when
// nothing is found so callers can report NOT_INSTALLED instead of spawning a
// guaranteed-to-fail command.
func resolveDevinBinaryPath() string {
	name := devinBinaryName()
	if filepath.IsAbs(name) || strings.ContainsRune(name, os.PathSeparator) {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return name
		}
		return ""
	}
	if p, err := lookPathFn(name); err == nil && p != "" {
		return p
	}
	for _, candidate := range devinWellKnownBinaryPaths() {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// devinSpawnBinary is resolveDevinBinaryPath with the configured name as the
// last resort so exec surfaces its standard not-found error when nothing is
// installed.
func devinSpawnBinary() string {
	if p := resolveDevinBinaryPath(); p != "" {
		return p
	}
	return devinBinaryName()
}
