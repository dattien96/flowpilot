package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeProjectPathInput_WindowsBackslashes(t *testing.T) {
	got := NormalizeProjectPathInput(`D:\working\gate-sandbox`)
	want := filepath.FromSlash("D:/working/gate-sandbox")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeProjectPathInput_ForwardSlashes(t *testing.T) {
	got := NormalizeProjectPathInput("D:/working/gate-sandbox")
	want := filepath.FromSlash("D:/working/gate-sandbox")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeProjectPathInput_QuotedUnix(t *testing.T) {
	got := NormalizeProjectPathInput(`"/Users/me/gate-sandbox"`)
	want := filepath.FromSlash("/Users/me/gate-sandbox")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeProjectPathInput_SingleQuoted(t *testing.T) {
	got := NormalizeProjectPathInput(`'C:\Users\me\proj'`)
	want := filepath.FromSlash("C:/Users/me/proj")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveProjectPath_EmptyUsesCwd(t *testing.T) {
	got, err := ResolveProjectPath("")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected abs path, got %q", got)
	}
}

func TestResolveProjectPath_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveProjectPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(dir) && got != dir {
		// Abs may differ by symlink/volume casing; compare cleaned.
		if filepath.Clean(got) != filepath.Clean(dir) {
			t.Fatalf("got %q want %q", got, dir)
		}
	}
}

func TestResolveProjectPath_WindowsStyleOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only abs drive letter check")
	}
	dir := t.TempDir()
	// Re-encode temp dir with backslashes like a user paste.
	raw := filepath.FromSlash(dir)
	raw = filepath.ToSlash(raw)
	raw = filepath.FromSlash(raw) // native
	// Force backslash form in the string we pass.
	mixed := filepath.ToSlash(dir)
	back := ""
	for _, r := range mixed {
		if r == '/' {
			back += `\`
		} else {
			back += string(r)
		}
	}
	got, err := ResolveProjectPath(back)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(dir) {
		t.Fatalf("got %q want %q (from %q)", got, dir, back)
	}
}
