package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkillFixture(t *testing.T, dir, name string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// TestDiscoverProjectSkillsGrokReadsDotGrokSkills proves Grok gets its own
// dedicated project-local skill directory (.grok/skills), matching Claude's
// dedicated-directory treatment rather than Codex/Gemini's shared
// .agents/skills convention (Task-214 T-5).
func TestDiscoverProjectSkillsGrokReadsDotGrokSkills(t *testing.T) {
	cwd := t.TempDir()
	writeSkillFixture(t, filepath.Join(cwd, ".grok", "skills"), "commit")

	got := discoverProjectSkills("grok", cwd)
	if len(got) != 1 {
		t.Fatalf("discoverProjectSkills(grok) = %d skills, want 1: %+v", len(got), got)
	}
	if got[0].Name != "commit" {
		t.Errorf("skill name = %q, want %q", got[0].Name, "commit")
	}
	if got[0].Source != "workspace" {
		t.Errorf("skill source = %q, want %q", got[0].Source, "workspace")
	}
}

// TestDiscoverProjectSkillsBaseRegression proves the pre-existing
// codex/gemini/claude branches are unchanged by the grok addition.
func TestDiscoverProjectSkillsBaseRegression(t *testing.T) {
	for _, tc := range []struct {
		provider string
		relDir   []string
		source   string
	}{
		{"codex", []string{".agents", "skills"}, "flowpilot"},
		{"gemini", []string{".agents", "skills"}, "flowpilot"},
		{"claude", []string{".claude", "skills"}, "workspace"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cwd := t.TempDir()
			writeSkillFixture(t, filepath.Join(cwd, filepath.Join(tc.relDir...)), "coder")

			got := discoverProjectSkills(tc.provider, cwd)
			if len(got) != 1 {
				t.Fatalf("discoverProjectSkills(%s) = %d skills, want 1: %+v", tc.provider, len(got), got)
			}
			if got[0].Source != tc.source {
				t.Errorf("skill source = %q, want %q", got[0].Source, tc.source)
			}

			// The grok-only fixture must not leak into these providers.
			writeSkillFixture(t, filepath.Join(cwd, ".grok", "skills"), "grok-only")
			got = discoverProjectSkills(tc.provider, cwd)
			if len(got) != 1 {
				t.Fatalf("discoverProjectSkills(%s) after grok fixture = %d skills, want 1 (unaffected): %+v", tc.provider, len(got), got)
			}
		})
	}
}

// TestProviderHomeSkillDirsGrokMirrorsCodexOrdering proves Grok's home skill
// roots use GROK_HOME-direct ordering (homePath IS the .grok dir, same as
// Codex/CODEX_HOME) rather than Claude's synthetic-$HOME ordering
// (Task-214 T-6).
func TestProviderHomeSkillDirsGrokMirrorsCodexOrdering(t *testing.T) {
	home := filepath.Join("fake", "grok-home")

	got := providerHomeSkillDirs("grok", home)
	want := []string{
		filepath.Join(home, "skills"),
		filepath.Join(home, ".grok", "skills"),
	}
	if len(got) != len(want) {
		t.Fatalf("providerHomeSkillDirs(grok) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("providerHomeSkillDirs(grok)[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	codexGot := providerHomeSkillDirs("codex", home)
	if len(codexGot) != 2 || codexGot[0] != filepath.Join(home, "skills") {
		t.Errorf("codex ordering changed unexpectedly: %v", codexGot)
	}
}

// TestProviderHomeSkillDirsBaseRegression proves codex/claude/gemini are
// unchanged by the grok addition.
func TestProviderHomeSkillDirsBaseRegression(t *testing.T) {
	home := filepath.Join("fake", "home")

	claudeGot := providerHomeSkillDirs("claude", home)
	claudeWant := []string{filepath.Join(home, ".claude", "skills"), filepath.Join(home, "skills")}
	if len(claudeGot) != 2 || claudeGot[0] != claudeWant[0] || claudeGot[1] != claudeWant[1] {
		t.Errorf("providerHomeSkillDirs(claude) = %v, want %v", claudeGot, claudeWant)
	}

	geminiGot := providerHomeSkillDirs("gemini", home)
	geminiWant := []string{filepath.Join(home, ".gemini", "skills"), filepath.Join(home, "skills")}
	if len(geminiGot) != 2 || geminiGot[0] != geminiWant[0] || geminiGot[1] != geminiWant[1] {
		t.Errorf("providerHomeSkillDirs(gemini) = %v, want %v", geminiGot, geminiWant)
	}

	codexGot := providerHomeSkillDirs("codex", home)
	codexWant := []string{filepath.Join(home, "skills"), filepath.Join(home, ".codex", "skills")}
	if len(codexGot) != 2 || codexGot[0] != codexWant[0] || codexGot[1] != codexWant[1] {
		t.Errorf("providerHomeSkillDirs(codex) = %v, want %v", codexGot, codexWant)
	}
}

func TestProviderHomeSkillDirsUnknownProviderReturnsNil(t *testing.T) {
	if got := providerHomeSkillDirs("unknown-provider", "/tmp/home"); got != nil {
		t.Errorf("providerHomeSkillDirs(unknown) = %v, want nil", got)
	}
}

func TestInteractiveCatalogCreateProject(t *testing.T) {
	cat := newInteractiveCatalog()
	initialList, err := cat.ListProjects(nil)
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}
	initialLen := len(initialList)

	newProj, err := cat.CreateProject(nil, CreateProjectInput{
		Name:          "Gate Sandbox",
		DirectoryPath: "D:\\working\\gate-sandbox",
		Platform:      "golang",
		DefaultModel:  "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if newProj.Name != "Gate Sandbox" {
		t.Fatalf("project name = %q, want 'Gate Sandbox'", newProj.Name)
	}
	if newProj.Platform != "golang" {
		t.Fatalf("project platform = %q, want 'golang'", newProj.Platform)
	}

	afterList, err := cat.ListProjects(nil)
	if err != nil {
		t.Fatalf("ListProjects after error: %v", err)
	}
	if len(afterList) != initialLen+1 {
		t.Fatalf("expected project count %d, got %d", initialLen+1, len(afterList))
	}
}
