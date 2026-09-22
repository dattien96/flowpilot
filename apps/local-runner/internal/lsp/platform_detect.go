package lsp

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectPlatform inspects dir and infers its technology stack. It is the
// shared implementation behind the TUI project wizard's platform detection
// (Task-356): the wizard delegates here so the LSP layer and the UI can
// never disagree on what a workspace is.
//
// Detection order is intentional and stable: go.mod wins over everything
// (a Go repo may vendor JS/Python tooling), then Gradle, Cargo, C/C++
// build files, Python markers, then package.json content.
func DetectPlatform(dir string) string {
	if dir == "" {
		return "general"
	}

	// 1. Golang
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "golang"
	}

	// 2. Android
	for _, f := range []string{"build.gradle", "settings.gradle", "build.gradle.kts", "settings.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "android"
		}
	}

	// 3. Rust
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "rust"
	}

	// 4. C/C++ (clangd): CMake or Make based layouts.
	for _, f := range []string{"CMakeLists.txt", "Makefile", "makefile", "GNUmakefile"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "cpp"
		}
	}

	// 5. Python
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "setup.py"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "python"
		}
	}

	// 6. JavaScript / TypeScript
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		s := strings.ToLower(string(data))
		if strings.Contains(s, "\"next\"") {
			return "nextjs"
		}
		if strings.Contains(s, "\"react-native\"") {
			return "react-native"
		}
		if strings.Contains(s, "\"react\"") {
			return "reactjs"
		}
		return "node"
	}

	return "general"
}
