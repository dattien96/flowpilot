package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpencodeAuthFileLooksValid_liveCredentialShape(t *testing.T) {
	t.Parallel()

	payload := `{
  "xai": {
    "type": "oauth",
    "refresh": "refresh-token",
    "access": "access-token",
    "expires": 1787820129233
  },
  "opencode": {
    "type": "api",
    "key": "sk-test"
  }
}`
	if !opencodeAuthFileLooksValid([]byte(payload)) {
		t.Fatal("expected live opencode auth.json shape to be valid")
	}
}

func TestOpencodeAuthFilePaths_includesXDGDefault(t *testing.T) {
	t.Parallel()

	home := "/home/user"
	paths := opencodeAuthFilePaths(home)
	want := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	if !containsPath(paths, want) {
		t.Fatalf("paths %v missing default XDG auth path %q", paths, want)
	}
}

func TestOpencodeAuthFilePaths_darwinApplicationSupportFallback(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only fallback path")
	}

	home := "/Users/test"
	paths := opencodeAuthFilePaths(home)
	want := filepath.Join(home, "Library", "Application Support", "opencode", "auth.json")
	if !containsPath(paths, want) {
		t.Fatalf("paths %v missing macOS fallback %q", paths, want)
	}
}

func TestOpencodeAuthFilePaths_windowsAppDataFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only fallback path")
	}

	home := `C:\Users\test`
	paths := opencodeAuthFilePaths(home)
	want := filepath.Join(home, "AppData", "Roaming", "opencode", "auth.json")
	if !containsPath(paths, want) {
		t.Fatalf("paths %v missing Windows fallback %q", paths, want)
	}
}

func TestDetectDefaultAccountHomePath_opencodeHonorsXDGDataHome(t *testing.T) {
	home := t.TempDir()
	customData := filepath.Join(home, "custom-data")
	authDir := filepath.Join(customData, "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"opencode":{"type":"api","key":"sk-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", customData)

	gotHome, ok := DetectDefaultAccountHomePath("opencode")
	if !ok {
		t.Fatal("expected auth under XDG_DATA_HOME to be discovered")
	}
	if filepath.Clean(gotHome) != filepath.Clean(home) {
		t.Fatalf("home = %q, want %q", gotHome, home)
	}
}

func TestHasValidProviderAuthFile_opencodeLocalSharePath(t *testing.T) {
	home := t.TempDir()
	authDir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"opencode":{"type":"api","key":"sk-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !hasValidProviderAuthFile("opencode", authPath) {
		t.Fatal("expected ~/.local/share/opencode/auth.json to count as valid auth")
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gotHome, ok := DetectDefaultAccountHomePath("opencode")
	if !ok {
		t.Fatal("expected DetectDefaultAccountHomePath to find opencode auth in temp home")
	}
	if filepath.Clean(gotHome) != filepath.Clean(home) {
		t.Fatalf("home = %q, want %q", gotHome, home)
	}
}

func TestIsValidOpencodeAccountPath_localShareAuthOnly(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	authDir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"opencode":{"type":"api","key":"sk-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !isValidOpencodeAccountPath(home) {
		t.Fatal("expected local share auth.json to mark home as valid opencode account path")
	}
}

func TestIsValidOpencodeAccountPath_darwinApplicationSupportAuth(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only fallback path")
	}

	home := t.TempDir()
	authDir := filepath.Join(home, "Library", "Application Support", "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"opencode":{"type":"api","key":"sk-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !isValidOpencodeAccountPath(home) {
		t.Fatal("expected macOS Application Support auth.json to mark home as valid")
	}
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}
