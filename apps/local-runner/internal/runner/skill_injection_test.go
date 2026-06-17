package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInjectSelectedSkillsDeliversSelection verifies the skill injection contract:
//   - each selected skill appears as a path pointer (not embedded content) so token
//     usage stays small regardless of skill file size
//   - the one-line frontmatter description is included for context
//   - the skill block is prepended before the user prompt
//   - unselected skills are never mentioned
//   - path-less selections are resolved via workspace discovery
func TestInjectSelectedSkillsDeliversSelection(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, ".agents", "skills")
	writeSkill := func(slug, desc, body string) string {
		dir := filepath.Join(skillsDir, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		path := filepath.Join(dir, "skill.md")
		content := "---\nname: " + slug + "\ndescription: " + desc + "\n---\n" + body + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return path
	}
	alphaPath := writeSkill("alpha", "Alpha does X", "ALPHA_SKILL_BODY")
	gammaPath := writeSkill("gamma", "Gamma does Y", "GAMMA_SKILL_BODY")
	writeSkill("beta", "Beta does Z", "BETA_SKILL_BODY")

	r := &Runner{workspace: ws}

	out := r.injectSelectedSkills(ws, "do the task", []SkillSelection{
		{Name: "alpha", Path: alphaPath, Source: "slash_picker"},
		{Name: "gamma", Path: gammaPath, Source: "slash_picker"},
	})

	if !strings.Contains(out, "do the task") {
		t.Fatal("injected prompt dropped the original prompt")
	}
	if !strings.Contains(out, "## Selected Skills") {
		t.Fatalf("missing selected-skills header:\n%s", out)
	}

	// Skills must be prepended — header must appear BEFORE the user's task text.
	if strings.Index(out, "## Selected Skills") > strings.Index(out, "do the task") {
		t.Fatalf("skill header appears after the user prompt — must be prepended:\n%s", out)
	}

	// Skill paths must appear so the model can Read the files.
	if !strings.Contains(out, alphaPath) || !strings.Contains(out, gammaPath) {
		t.Fatalf("skill paths missing from output:\n%s", out)
	}

	// Frontmatter descriptions must appear for one-line context.
	if !strings.Contains(out, "Alpha does X") || !strings.Contains(out, "Gamma does Y") {
		t.Fatalf("skill descriptions missing from output:\n%s", out)
	}

	// Full skill body content must NOT be embedded — path pointer only.
	if strings.Contains(out, "ALPHA_SKILL_BODY") || strings.Contains(out, "GAMMA_SKILL_BODY") {
		t.Fatalf("runner embedded full skill content — must emit path pointers only:\n%s", out)
	}

	// Unselected skill must not appear at all.
	if strings.Contains(out, "beta") || strings.Contains(out, "BETA_SKILL_BODY") {
		t.Fatalf("runner injected an unselected skill:\n%s", out)
	}

	// Path-less fallback: resolver finds the skill by name under the workspace and emits its path.
	betaPath := filepath.Join(skillsDir, "beta", "skill.md")
	fallback := r.injectSelectedSkills(ws, "task2", []SkillSelection{{Name: "beta", Source: "slash_picker"}})
	if !strings.Contains(fallback, betaPath) {
		t.Fatalf("path-less selection did not resolve via workspace discovery:\n%s", fallback)
	}
	if strings.Contains(fallback, "BETA_SKILL_BODY") {
		t.Fatalf("path-less fallback must emit path pointer, not embedded content:\n%s", fallback)
	}

	// No selection → prompt returned untouched (no skills block injected).
	if got := r.injectSelectedSkills(ws, "bare", nil); got != "bare" {
		t.Fatalf("empty selection mutated prompt: %q", got)
	}
}
