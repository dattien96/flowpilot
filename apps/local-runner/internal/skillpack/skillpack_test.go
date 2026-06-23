package skillpack

import (
	"os"
	"path/filepath"
	"testing"
)

var expectedSkills = []string{
	"git-commit-format",
	"oracle-rule",
	"audit-logging",
	"phase-doc",
	"context-discipline",
}

func TestInstall_CreatesFilesInAllProviderDirs(t *testing.T) {
	dir := t.TempDir()

	result, err := Install(dir)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	if len(result.Errors) > 0 {
		t.Fatalf("Install reported errors: %v", result.Errors)
	}

	for _, provider := range providerDirs {
		for _, skill := range expectedSkills {
			path := filepath.Join(dir, provider, "skills", "flowpilot", skill, "SKILL.md")
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("expected file missing: %s", path)
			}
		}
	}

	// 5 skills * 3 providers = 15 files installed
	want := len(expectedSkills) * len(providerDirs)
	if len(result.Installed) != want {
		t.Errorf("Installed count = %d, want %d", len(result.Installed), want)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped count = %d, want 0", len(result.Skipped))
	}
}

func TestIsInstalled_TrueAfterInstall(t *testing.T) {
	dir := t.TempDir()

	if IsInstalled(dir) {
		t.Fatal("IsInstalled should be false before install")
	}

	if _, err := Install(dir); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !IsInstalled(dir) {
		t.Fatal("IsInstalled should be true after install")
	}
}

func TestInstall_SkipsExistingSameVersionFiles(t *testing.T) {
	dir := t.TempDir()

	first, err := Install(dir)
	if err != nil {
		t.Fatalf("first Install returned error: %v", err)
	}
	if len(first.Errors) > 0 {
		t.Fatalf("first Install errors: %v", first.Errors)
	}

	second, err := Install(dir)
	if err != nil {
		t.Fatalf("second Install returned error: %v", err)
	}
	if len(second.Errors) > 0 {
		t.Fatalf("second Install errors: %v", second.Errors)
	}

	if len(second.Installed) != 0 {
		t.Errorf("second Install.Installed = %d, want 0 (all should be skipped)", len(second.Installed))
	}

	want := len(expectedSkills) * len(providerDirs)
	if len(second.Skipped) != want {
		t.Errorf("second Install.Skipped = %d, want %d", len(second.Skipped), want)
	}
}

func TestInstall_ReinstallsWhenVersionMismatch(t *testing.T) {
	dir := t.TempDir()

	// Write a file with a different version so it should be overwritten.
	stalePath := filepath.Join(dir, ".claude", "skills", "flowpilot", "git-commit-format", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("version: 0\n\nold content"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Install(dir)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Install errors: %v", result.Errors)
	}

	// The stale file must have been overwritten with the current pack version.
	if !fileMatchesVersion(stalePath, PackVersion) {
		t.Errorf("stale file was not updated to PackVersion %d", PackVersion)
	}
}

func TestFileMatchesVersion(t *testing.T) {
	f, err := os.CreateTemp("", "skill-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("version: 1\n\nsome content\n")
	f.Close()

	if !fileMatchesVersion(f.Name(), 1) {
		t.Error("expected version match for version 1")
	}
	if fileMatchesVersion(f.Name(), 2) {
		t.Error("expected no match for version 2")
	}
}
