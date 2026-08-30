package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// opencodeDataDir returns the OpenCode application data directory under a home
// path. Live-verified default: ~/.local/share/opencode on Linux, macOS, and
// Windows (see opencode.ai docs). Managed FlowPilot homes mirror this layout.
func opencodeDataDir(homePath string) string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return ""
	}
	return filepath.Join(homePath, ".local", "share", "opencode")
}

// opencodeAuthFilePaths returns credential file locations relative to a single
// account home directory. It intentionally does not read ambient XDG_DATA_HOME or
// OPENCODE_AUTH_PATH — those belong in opencodeAmbientAuthCandidates and match
// how GROK_HOME-scoped discovery stays under the resolved home.
func opencodeAuthFilePaths(homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	homePath = filepath.Clean(homePath)

	paths := []string{
		filepath.Join(opencodeDataDir(homePath), "auth.json"),
	}
	switch runtime.GOOS {
	case "darwin":
		// OpenCode 1.14.x may still read the platform data dir on macOS when the
		// XDG-style path is absent (live-verified dual-layout reports).
		paths = append(paths, filepath.Join(homePath, "Library", "Application Support", "opencode", "auth.json"))
	case "windows":
		paths = append(paths, filepath.Join(homePath, "AppData", "Roaming", "opencode", "auth.json"))
	}
	paths = append(paths,
		filepath.Join(homePath, ".config", "opencode", "auth.json"),
		filepath.Join(homePath, ".opencode", "auth.json"),
		filepath.Join(homePath, "auth.json"),
	)
	return dedupeFilePaths(paths)
}

// opencodeAmbientAuthCandidates returns auth paths that may live outside the
// passed home but still belong to the user's ambient OpenCode install. Only
// used while probing real user homes (getPossibleHomeDirs), not managed slots.
func opencodeAmbientAuthCandidates(homePath string) []authCandidate {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	homePath = filepath.Clean(homePath)

	candidates := make([]authCandidate, 0, 4)
	if authPath := resolveOpencodeAuthPathFromEnv(homePath); authPath != "" {
		candidates = append(candidates, authCandidate{homePath: homePath, authPath: authPath})
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		candidates = append(candidates, authCandidate{
			homePath: homePath,
			authPath: filepath.Join(xdg, "opencode", "auth.json"),
		})
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			candidates = append(candidates, authCandidate{
				homePath: homePath,
				authPath: filepath.Join(appData, "opencode", "auth.json"),
			})
		}
	}
	return candidates
}

// resolveOpencodeAuthPathFromEnv maps OPENCODE_AUTH_PATH to an absolute path.
// Relative values resolve under the OpenCode data dir for the given home.
func resolveOpencodeAuthPathFromEnv(homePath string) string {
	raw := strings.TrimSpace(os.Getenv("OPENCODE_AUTH_PATH"))
	if raw == "" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	dataDir := opencodeDataDir(homePath)
	if dataDir == "" {
		return ""
	}
	return filepath.Clean(filepath.Join(dataDir, raw))
}

// opencodeConfigFilePath returns the opencode.json config FILE path under an
// account home. OPENCODE_CONFIG must point at a file, not the config
// directory: opencode reads it with readFile and dies with
// "BadResource: FileSystem.readFile" when given a directory (CA-679, live
// verified on 1.18.18 for both `opencode acp` boot and `opencode models`).
// filepath.Join yields OS-correct separators on Windows and POSIX.
func opencodeConfigFilePath(homePath string) string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return ""
	}
	return filepath.Join(homePath, ".config", "opencode", "opencode.json")
}

func opencodeConfigFilePaths(homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	homePath = filepath.Clean(homePath)
	configDir := filepath.Join(homePath, ".config", "opencode")
	return dedupeFilePaths([]string{
		filepath.Join(configDir, "opencode.json"),
		filepath.Join(configDir, "opencode.jsonc"),
		filepath.Join(configDir, "config.json"),
	})
}

func opencodeConfigOrAuthFileExists(homePath string) bool {
	for _, path := range opencodeConfigFilePaths(homePath) {
		if fileExists(path) {
			return true
		}
	}
	for _, path := range opencodeAuthFilePaths(homePath) {
		if fileExists(path) {
			return true
		}
	}
	return false
}

func dedupeFilePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	return out
}

func opencodeAccountIsAmbientHome(homePath string) bool {
	homePath = strings.TrimSpace(homePath)
	userHome := preferredUserHomeDir()
	if homePath == "" || userHome == "" {
		return false
	}
	return canonicalPathKey(homePath) == canonicalPathKey(userHome)
}

// opencodeDataHomeForAccount returns XDG_DATA_HOME for an OpenCode account.
// Default/ambient homes keep a relocated XDG_DATA_HOME; managed slots isolate
// under <home>/.local/share.
func opencodeDataHomeForAccount(homePath string) string {
	if opencodeAccountIsAmbientHome(homePath) {
		if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
			return xdg
		}
	}
	return filepath.Join(homePath, ".local", "share")
}
