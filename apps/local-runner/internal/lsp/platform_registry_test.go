package lsp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// lspFixtureDir creates a workspace dir with the given marker files.
func lspFixtureDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return dir
}

// lspFakeBinary drops a no-op executable named name into dir. On Windows the
// file needs a PATHEXT extension for exec.LookPath to find it.
func lspFakeBinary(t *testing.T, dir, name string) {
	t.Helper()
	filename := name
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		filename = name + ".bat"
		content = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o755); err != nil {
		t.Fatalf("write fake binary %s: %v", name, err)
	}
}

// lspWithBinariesOnPath prepends dir (containing fake binaries) to PATH.
func lspWithBinariesOnPath(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		lspFakeBinary(t, dir, n)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDefaultRegistryContainsAllPlatforms(t *testing.T) {
	reg := DefaultRegistry()
	for _, p := range []string{"golang", "nextjs", "reactjs", "node", "python", "rust", "cpp", "android"} {
		cfg, ok := reg.Lookup(p)
		if !ok {
			t.Fatalf("platform %q missing from registry", p)
		}
		if cfg.Binary == "" || len(cfg.FileExtensions) == 0 {
			t.Fatalf("platform %q has incomplete config: %+v", p, cfg)
		}
	}
	if got := reg["golang"].Binary; got != "gopls" {
		t.Fatalf("golang binary = %q, want gopls", got)
	}
	if got := reg["python"].Binary; got != "pyright-langserver" {
		t.Fatalf("python binary = %q, want pyright-langserver", got)
	}
	if got := reg["cpp"].Binary; got != "clangd" {
		t.Fatalf("cpp binary = %q, want clangd", got)
	}
}

func TestDetectAndResolveFindsGoplsForGolang(t *testing.T) {
	binDir := t.TempDir()
	lspWithBinariesOnPath(t, binDir, "gopls")
	ws := lspFixtureDir(t, map[string]string{"go.mod": "module example.com/x\n"})

	cfg, err := DetectAndResolve(ws)
	if err != nil {
		t.Fatalf("DetectAndResolve: %v", err)
	}
	if cfg.Platform != "golang" || cfg.Binary != "gopls" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestDetectAndResolveFindsVtslsForNextjs(t *testing.T) {
	binDir := t.TempDir()
	lspWithBinariesOnPath(t, binDir, "vtsls")
	ws := lspFixtureDir(t, map[string]string{
		"package.json": `{"dependencies": {"next": "14.0.0"}}`,
	})

	cfg, err := DetectAndResolve(ws)
	if err != nil {
		t.Fatalf("DetectAndResolve: %v", err)
	}
	if cfg.Platform != "nextjs" || cfg.Binary != "vtsls" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestDetectAndResolveReturnErrorWhenBinaryMissing(t *testing.T) {
	// PATH with no servers at all.
	t.Setenv("PATH", t.TempDir())
	ws := lspFixtureDir(t, map[string]string{"go.mod": "module example.com/x\n"})

	_, err := DetectAndResolve(ws)
	if err == nil {
		t.Fatal("expected error when server binary is missing from PATH")
	}
	if !strings.Contains(err.Error(), "gopls") || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("error should name the binary and PATH, got: %v", err)
	}
}

func TestDetectPlatformReusesProjectWizardLogic(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"golang", map[string]string{"go.mod": "module x\n"}, "golang"},
		{"golang wins over js", map[string]string{"go.mod": "module x\n", "package.json": "{}"}, "golang"},
		{"android gradle", map[string]string{"build.gradle": ""}, "android"},
		{"android kts", map[string]string{"settings.gradle.kts": ""}, "android"},
		{"rust", map[string]string{"Cargo.toml": "[package]\n"}, "rust"},
		{"cpp cmake", map[string]string{"CMakeLists.txt": "cmake_minimum_required(VERSION 3.20)\n"}, "cpp"},
		{"cpp makefile", map[string]string{"Makefile": "all:\n"}, "cpp"},
		{"rust beats cmake", map[string]string{"Cargo.toml": "[package]\n", "CMakeLists.txt": ""}, "rust"},
		{"golang beats cmake", map[string]string{"go.mod": "module x\n", "CMakeLists.txt": ""}, "golang"},
		{"python requirements", map[string]string{"requirements.txt": "fastapi\n"}, "python"},
		{"python pyproject", map[string]string{"pyproject.toml": "[project]\n"}, "python"},
		{"nextjs", map[string]string{"package.json": `{"dependencies":{"next":"14"}}`}, "nextjs"},
		{"reactjs", map[string]string{"package.json": `{"dependencies":{"react":"18"}}`}, "reactjs"},
		{"node", map[string]string{"package.json": `{"dependencies":{"express":"4"}}`}, "node"},
		{"general empty", nil, "general"},
		{"general empty dir", map[string]string{"README.md": "hi\n"}, "general"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := DetectPlatform(dir); got != tc.want {
				t.Fatalf("DetectPlatform = %q, want %q", got, tc.want)
			}
		})
	}
	if got := DetectPlatform(""); got != "general" {
		t.Fatalf("DetectPlatform(\"\") = %q, want general", got)
	}
}

func TestRegistryFileExtensionsCorrect(t *testing.T) {
	reg := DefaultRegistry()
	checks := []struct{ platform, file string }{
		{"golang", "main.go"},
		{"nextjs", "app.ts"},
		{"reactjs", "app.tsx"},
		{"node", "server.mjs"},
		{"python", "main.py"},
		{"rust", "main.rs"},
		{"cpp", "main.c"},
		{"cpp", "util.cpp"},
		{"cpp", "util.hpp"},
		{"android", "Main.kt"},
		{"android", "build.kts"},
	}
	for _, tc := range checks {
		cfg, ok := reg.ConfigForFile(tc.platform, tc.file)
		if !ok {
			t.Fatalf("%s does not claim %s", tc.platform, tc.file)
		}
		if cfg.Platform != tc.platform {
			t.Fatalf("got platform %q, want %q", cfg.Platform, tc.platform)
		}
	}
	// Cross-platform files are scoped: a .ts file is not owned by golang,
	// and markdown belongs to nobody.
	if _, ok := reg.ConfigForFile("golang", "app.ts"); ok {
		t.Fatal("golang must not claim .ts files")
	}
	if _, ok := reg.ConfigForFile("nextjs", "notes.md"); ok {
		t.Fatal("markdown must not be claimed by any server")
	}
	if _, ok := reg.ConfigForFile("general", "main.go"); ok {
		t.Fatal("unregistered platform must claim nothing")
	}
}

func TestDetectAndResolveFindsClangdForCpp(t *testing.T) {
	binDir := t.TempDir()
	lspWithBinariesOnPath(t, binDir, "clangd")
	ws := lspFixtureDir(t, map[string]string{"CMakeLists.txt": "cmake_minimum_required(VERSION 3.20)\n"})

	cfg, err := DetectAndResolve(ws)
	if err != nil {
		t.Fatalf("DetectAndResolve: %v", err)
	}
	if cfg.Platform != "cpp" || cfg.Binary != "clangd" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestDetectPlatformCpp(t *testing.T) {
	for _, marker := range []string{"CMakeLists.txt", "Makefile", "makefile", "GNUmakefile"} {
		ws := lspFixtureDir(t, map[string]string{marker: ""})
		if got := DetectPlatform(ws); got != "cpp" {
			t.Fatalf("marker %s -> %q, want cpp", marker, got)
		}
	}
}

func TestDetectPlatformAndroid(t *testing.T) {
	for _, marker := range []string{"build.gradle", "settings.gradle", "build.gradle.kts", "settings.gradle.kts"} {
		ws := lspFixtureDir(t, map[string]string{marker: ""})
		if got := DetectPlatform(ws); got != "android" {
			t.Fatalf("marker %s -> %q, want android", marker, got)
		}
	}
}

func TestDetectPlatformPython(t *testing.T) {
	for _, marker := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "setup.py"} {
		ws := lspFixtureDir(t, map[string]string{marker: ""})
		if got := DetectPlatform(ws); got != "python" {
			t.Fatalf("marker %s -> %q, want python", marker, got)
		}
	}
}

func TestLanguageIDMapping(t *testing.T) {
	cases := map[string]string{
		".go": "go", "main.go": "go",
		".py": "python", ".pyi": "python",
		".rs": "rust",
		".c":  "c", ".h": "c",
		".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp",
		".kt": "kotlin", ".kts": "kotlin",
		".ts": "typescript", ".tsx": "typescript",
		".js": "javascript", ".jsx": "javascript",
	}
	for in, want := range cases {
		if got := LanguageID(in); got != want {
			t.Fatalf("LanguageID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := LanguageID(".md"); got != "" {
		t.Fatalf("LanguageID(.md) = %q, want empty", got)
	}
}
