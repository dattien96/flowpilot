package lsp

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// PlatformLSPConfig describes how to serve one project platform: which
// server binary to spawn, with what args, for which files.
type PlatformLSPConfig struct {
	// Platform is the DetectPlatform token, e.g. "golang".
	Platform string
	// Binary is the server executable resolved via PATH, e.g. "gopls".
	Binary string
	// Args are passed verbatim on spawn, e.g. ["--stdio"].
	Args []string
	// FileExtensions selects the files this server owns, e.g. [".go"].
	FileExtensions []string
	// InstallHint is the one-liner shown when Binary is missing from PATH.
	InstallHint string
	// InitializationOptions is sent as initialize.initializationOptions.
	InitializationOptions map[string]any
}

// Registry maps platform tokens to server configs.
type Registry map[string]PlatformLSPConfig

// DefaultRegistry returns the built-in platform -> server mapping. The
// android entry is a draft until Task-360 finalizes the Kotlin-specific
// initialization options.
func DefaultRegistry() Registry {
	return Registry{
		"golang": {
			Platform: "golang", Binary: "gopls", Args: nil,
			FileExtensions: []string{".go"},
			InstallHint:    "go install golang.org/x/tools/gopls@latest",
		},
		"nextjs": {
			Platform: "nextjs", Binary: "vtsls", Args: []string{"--stdio"},
			FileExtensions: []string{".ts", ".tsx", ".js", ".jsx"},
			InstallHint:    "npm i -g vtsls",
		},
		"reactjs": {
			Platform: "reactjs", Binary: "vtsls", Args: []string{"--stdio"},
			FileExtensions: []string{".ts", ".tsx", ".js", ".jsx"},
			InstallHint:    "npm i -g vtsls",
		},
		"node": {
			Platform: "node", Binary: "vtsls", Args: []string{"--stdio"},
			FileExtensions: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"},
			InstallHint:    "npm i -g vtsls",
		},
		"python": {
			// The LSP server binary shipped by the pyright npm package.
			Platform: "python", Binary: "pyright-langserver", Args: []string{"--stdio"},
			FileExtensions: []string{".py", ".pyi"},
			InstallHint:    "npm i -g pyright",
		},
		"rust": {
			Platform: "rust", Binary: "rust-analyzer", Args: nil,
			FileExtensions: []string{".rs"},
			InstallHint:    "rustup component add rust-analyzer",
		},
		"cpp": {
			Platform: "cpp", Binary: "clangd", Args: nil,
			FileExtensions: []string{".c", ".h", ".hpp", ".hh", ".cc", ".cpp", ".cxx"},
			InstallHint:    "Install LLVM (llvm.org) or your OS package manager",
		},
		"android": {
			// Draft: Task-360 finalizes args and initialization options.
			Platform: "android", Binary: "kotlin-language-server", Args: []string{"--stdio"},
			FileExtensions: []string{".kt", ".kts"},
			InstallHint:    "Download from fwcd/kotlin-language-server releases",
		},
	}
}

// Lookup returns the config for a platform token.
func (r Registry) Lookup(platform string) (PlatformLSPConfig, bool) {
	cfg, ok := r[platform]
	return cfg, ok
}

// DetectAndResolve detects the workspace platform and resolves its server
// binary via PATH. A missing binary is a descriptive error — the caller
// logs a warning and continues without LSP (graceful degradation).
func DetectAndResolve(workspaceRoot string) (PlatformLSPConfig, error) {
	return DetectAndResolveWith(DefaultRegistry(), workspaceRoot)
}

// DetectAndResolveWith is DetectAndResolve over an explicit registry
// (tests inject stub registries through it).
func DetectAndResolveWith(reg Registry, workspaceRoot string) (PlatformLSPConfig, error) {
	platform := DetectPlatform(workspaceRoot)
	cfg, ok := reg.Lookup(platform)
	if !ok {
		return PlatformLSPConfig{}, fmt.Errorf("lsp: no language server registered for platform %q", platform)
	}
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		return PlatformLSPConfig{}, fmt.Errorf("lsp: server binary %q for platform %q not found in PATH: %w", cfg.Binary, platform, err)
	}
	return cfg, nil
}

// extensionLanguages maps file extensions to LSP languageId values.
var extensionLanguages = map[string]string{
	".go": "go",
	".py": "python", ".pyi": "python",
	".rs": "rust",
	".c":  "c", ".h": "c",
	".hpp": "cpp", ".hh": "cpp", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp",
	".kt": "kotlin", ".kts": "kotlin",
	".ts": "typescript", ".tsx": "typescript",
	".js": "javascript", ".jsx": "javascript",
	".mjs": "javascript", ".cjs": "javascript",
}

// LanguageID returns the LSP languageId for a file extension (".go") or
// path ("main.go"). It returns "" when the extension is unknown.
func LanguageID(nameOrExt string) string {
	ext := strings.ToLower(nameOrExt)
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return extensionLanguages[ext]
}

// ConfigForFile returns the registry entry for platform when it owns path
// (by extension), or false. Ownership is always resolved per platform —
// extensions alone are ambiguous (.ts belongs to nextjs, reactjs and node),
// so callers first resolve the workspace platform via DetectAndResolve and
// then confirm the file belongs to it here.
func (r Registry) ConfigForFile(platform, path string) (PlatformLSPConfig, bool) {
	cfg, ok := r.Lookup(platform)
	if !ok {
		return PlatformLSPConfig{}, false
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range cfg.FileExtensions {
		if e == ext {
			return cfg, true
		}
	}
	return PlatformLSPConfig{}, false
}
