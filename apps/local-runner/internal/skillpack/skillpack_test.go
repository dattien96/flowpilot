package skillpack

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

var commonSkills = []string{
	"git-commit-format",
	"oracle-rule",
	"audit-logging",
	"phase-doc",
	"context-discipline",
}

func TestInstall_CommonOnlyForNonePlatform(t *testing.T) {
	dir := t.TempDir()

	result, err := Install(dir, "none")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Install reported errors: %v", result.Errors)
	}

	for _, skill := range commonSkills {
		if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected Claude skill missing for %s", skill)
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".agents", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected agents skill missing for %s", skill)
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", "flowpilot", skill, "SKILL.md")); !os.IsNotExist(statErr) {
			t.Errorf("unexpected nested flowpilot dir for Claude skill %s", skill)
		}
	}

	// A none/common-only platform must NOT install any platform-specific skill.
	if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", "android-conventions", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Error("android-conventions should not be installed for platform=none")
	}

	// common skills * 2 physical install roots
	want := len(commonSkills) * len(installRoots)
	if len(result.Installed) != want {
		t.Errorf("Installed count = %d, want %d", len(result.Installed), want)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped count = %d, want 0", len(result.Skipped))
	}
}

func TestInstall_AndroidIncludesCommonAndAndroid(t *testing.T) {
	dir := t.TempDir()

	result, err := Install(dir, "android")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Install reported errors: %v", result.Errors)
	}

	if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", "android-conventions", "SKILL.md")); statErr != nil {
		t.Error("android-conventions should be installed for platform=android")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", "git-commit-format", "SKILL.md")); statErr != nil {
		t.Error("common skill git-commit-format should be installed for platform=android")
	}

	// (common + 1 android skill) * 2 roots
	want := (len(commonSkills) + 1) * len(installRoots)
	if len(result.Installed) != want {
		t.Errorf("Installed count = %d, want %d", len(result.Installed), want)
	}
}

func TestInstall_KMMIncludesCommonAndroidIosAndKmm(t *testing.T) {
	dir := t.TempDir()

	result, err := Install(dir, "kmm")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Install reported errors: %v", result.Errors)
	}

	for _, skill := range []string{"kmm-conventions", "android-conventions", "ios-conventions"} {
		if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected %s installed for platform=kmm", skill)
		}
	}

	// (common + kmm + android + ios) * 2 roots
	want := (len(commonSkills) + 3) * len(installRoots)
	if len(result.Installed) != want {
		t.Errorf("Installed count = %d, want %d", len(result.Installed), want)
	}
}

func TestIsInstalled_TrueAfterInstall(t *testing.T) {
	dir := t.TempDir()

	if IsInstalled(dir) {
		t.Fatal("IsInstalled should be false before install")
	}

	if _, err := Install(dir, "none"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !IsInstalled(dir) {
		t.Fatal("IsInstalled should be true after install")
	}
}

func TestInstall_SkipsExistingSameVersionFiles(t *testing.T) {
	dir := t.TempDir()

	first, err := Install(dir, "android")
	if err != nil {
		t.Fatalf("first Install returned error: %v", err)
	}
	if len(first.Errors) > 0 {
		t.Fatalf("first Install errors: %v", first.Errors)
	}

	second, err := Install(dir, "android")
	if err != nil {
		t.Fatalf("second Install returned error: %v", err)
	}
	if len(second.Errors) > 0 {
		t.Fatalf("second Install errors: %v", second.Errors)
	}

	if len(second.Installed) != 0 {
		t.Errorf("second Install.Installed = %d, want 0 (all should be skipped)", len(second.Installed))
	}

	want := (len(commonSkills) + 1) * len(installRoots)
	if len(second.Skipped) != want {
		t.Errorf("second Install.Skipped = %d, want %d", len(second.Skipped), want)
	}
}

func TestInstall_ReinstallsWhenVersionMismatch(t *testing.T) {
	dir := t.TempDir()

	// Write a file with a different version so it should be overwritten.
	stalePath := filepath.Join(dir, ".claude", "skills", "git-commit-format", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("---\nname: git-commit-format\ndescription: stale skill\nversion: 0\n---\n\nold content"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Install(dir, "none")
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

func TestPlatformGroups(t *testing.T) {
	cases := map[string][]string{
		"":                       {"common"},
		"none":                   {"common"},
		"unknown-thing":          {"common"},
		"android":                {"common", "android"},
		"ios":                    {"common", "ios"},
		"Android":                {"common", "android"},
		"  python  ":             {"common", "python"},
		"kmm":                    {"common", "kmm", "android", "ios"},
		"react-native":           {"common", "react-native"},
		"nodejs":                 {"common", "nodejs"},
	}
	for input, want := range cases {
		got := platformGroups(input)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("platformGroups(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestSkillNames_IncludesPlatformSkill(t *testing.T) {
	names, err := SkillNames("android")
	if err != nil {
		t.Fatalf("SkillNames error: %v", err)
	}
	sort.Strings(names)
	if !contains(names, "git-commit-format") {
		t.Error("expected common skill git-commit-format in SkillNames(android)")
	}
	if !contains(names, "android-conventions") {
		t.Error("expected android-conventions in SkillNames(android)")
	}
}

func TestStatus_CurrentAfterInstall(t *testing.T) {
	dir := t.TempDir()

	if _, err := Install(dir, "android"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	status, err := Status(dir, "android")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.Installed {
		t.Error("Status.Installed = false, want true after install")
	}
	if !status.Current {
		t.Error("Status.Current = false, want true after install")
	}
	if status.PackVersion != PackVersion {
		t.Errorf("Status.PackVersion = %d, want %d", status.PackVersion, PackVersion)
	}
}

func TestFileMatchesVersion(t *testing.T) {
	f, err := os.CreateTemp("", "skill-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("---\nname: test-skill\ndescription: test skill\nversion: 1\n---\n\nsome content\n")
	f.Close()

	if !fileMatchesVersion(f.Name(), 1) {
		t.Error("expected version match for version 1")
	}
	if fileMatchesVersion(f.Name(), 2) {
		t.Error("expected no match for version 2")
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
