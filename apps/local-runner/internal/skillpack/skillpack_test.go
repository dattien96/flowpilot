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
	"additive-tests-only",
	"kill-review",
	"codex-claude-review-loop",
	"codex-grok-review-loop",
	"cross-provider-parity",
	"safe-fix-contract",
	"web-design-guidelines",
	"flow-mode-orchestrator",
	// CA-903 / Task-412 intentionally added these two common skills; the list
	// must track the pack or the install-count assertions drift.
	"flow-harness-contract",
	"vibe-lanes",
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

	androidSkills, err := skillsForPlatform("android")
	if err != nil {
		t.Fatalf("skillsForPlatform(android) failed: %v", err)
	}
	want := len(androidSkills) * len(installRoots)
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

	kmmSkills, err := skillsForPlatform("kmm")
	if err != nil {
		t.Fatalf("skillsForPlatform(kmm) failed: %v", err)
	}
	want := len(kmmSkills) * len(installRoots)
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

// TestInstall_WritesGrokSkillsRoot proves Grok gets its own native install
// root (.grok/skills), matching Claude's dedicated-directory treatment,
// without altering the pre-existing .claude/skills or .agents/skills writes
// (Task-214 T-1).
func TestInstall_WritesGrokSkillsRoot(t *testing.T) {
	dir := t.TempDir()

	if _, err := Install(dir, "none"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	for _, skill := range commonSkills {
		if _, statErr := os.Stat(filepath.Join(dir, ".grok", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected Grok skill missing for %s: %v", skill, statErr)
		}
		// Pre-existing roots must be unaffected by the new root.
		if _, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected Claude skill missing for %s: %v", skill, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".agents", "skills", skill, "SKILL.md")); statErr != nil {
			t.Errorf("expected agents skill missing for %s: %v", skill, statErr)
		}
	}
}

func TestIsInstalled_RequiresGrokSentinel(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, ".claude", "skills", "git-commit-format"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "skills", "git-commit-format", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "skills", "git-commit-format"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agents", "skills", "git-commit-format", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-Task-214 shape: .claude and .agents sentinels exist but .grok does
	// not (simulates a project bound before this change) -- IsInstalled must
	// now report false so the next bind check re-runs Install and backfills
	// .grok/skills (Task-214 T-2's self-heal).
	if IsInstalled(dir) {
		t.Fatal("IsInstalled should be false when the .grok sentinel is missing")
	}

	if _, err := Install(dir, "none"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !IsInstalled(dir) {
		t.Fatal("IsInstalled should be true once all three sentinels exist")
	}
}

func TestProviderStatuses_IncludesGrokWithoutAlteringOthers(t *testing.T) {
	want := map[string]string{
		"claude":   filepath.Join(".claude", "skills"),
		"codex":    filepath.Join(".agents", "skills"),
		"gemini":   filepath.Join(".agents", "skills"),
		"grok":     filepath.Join(".grok", "skills"),
		"opencode": filepath.Join(".opencode", "skills"),
	}
	if len(providerStatuses) != len(want) {
		t.Fatalf("providerStatuses has %d entries, want %d", len(providerStatuses), len(want))
	}
	for _, entry := range providerStatuses {
		wantPath, ok := want[entry.Provider]
		if !ok {
			t.Fatalf("unexpected provider %q in providerStatuses", entry.Provider)
		}
		if entry.RootPath != wantPath {
			t.Errorf("providerStatuses[%q].RootPath = %q, want %q", entry.Provider, entry.RootPath, wantPath)
		}
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

	androidSkills, err := skillsForPlatform("android")
	if err != nil {
		t.Fatalf("skillsForPlatform(android) failed: %v", err)
	}
	want := len(androidSkills) * len(installRoots)
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

func TestPlatformGroupsNodeAlias(t *testing.T) {
	// DetectPlatform returns the LSP token "node" for plain JS repos while the
	// embedded pack group is "nodejs"; the stored platform must still resolve
	// to the nodejs group instead of silently installing common only.
	got := platformGroups("node")
	want := []string{"common", "nodejs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("platformGroups(%q) = %v, want %v", "node", got, want)
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
