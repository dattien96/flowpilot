package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInjectSelectedSkillsDeliversSelection locks the BUG-063 follow-up: the FULL content
// of every selected skill is injected (resolved by its explicit path), unselected skills
// are not added by the runner, and the path-less id/name fallback still resolves. The model
// may auto-read other skills natively — that is acceptable, so there is no exclusion claim.
func TestInjectSelectedSkillsDeliversSelection(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, ".agents", "skills")
	writeSkill := func(slug, body string) string {
		dir := filepath.Join(skillsDir, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		path := filepath.Join(dir, "skill.md")
		content := "---\nname: " + slug + "\n---\n" + body + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return path
	}
	alphaPath := writeSkill("alpha", "ALPHA_SKILL_BODY")
	gammaPath := writeSkill("gamma", "GAMMA_SKILL_BODY")
	writeSkill("beta", "BETA_SKILL_BODY")

	r := &Runner{workspace: ws}

	// Every selected skill is injected by path; the unselected beta is not added by the runner.
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
	// Skills must be prepended — the header must appear BEFORE the user's task text so
	// the model reads process constraints before forming its response plan.
	if strings.Index(out, "## Selected Skills") > strings.Index(out, "do the task") {
		t.Fatalf("skill header appears after the user prompt — must be prepended:\n%s", out)
	}
	if !strings.Contains(out, "ALPHA_SKILL_BODY") || !strings.Contains(out, "GAMMA_SKILL_BODY") {
		t.Fatalf("not all selected skills were injected:\n%s", out)
	}
	if !strings.Contains(out, "Selected skill names: /alpha, /gamma") {
		t.Fatalf("selected skill names were not listed explicitly:\n%s", out)
	}
	if strings.Contains(out, "BETA_SKILL_BODY") {
		t.Fatalf("runner injected an unselected skill:\n%s", out)
	}

	// Path-less fallback resolves by id/name under the run workspace.
	fallback := r.injectSelectedSkills(ws, "task2", []SkillSelection{{Name: "beta", Source: "slash_picker"}})
	if !strings.Contains(fallback, "BETA_SKILL_BODY") {
		t.Fatalf("path-less selection did not resolve via workspace discovery:\n%s", fallback)
	}

	// No selection → prompt is returned untouched (no skills block).
	if got := r.injectSelectedSkills(ws, "bare", nil); got != "bare" {
		t.Fatalf("empty selection mutated prompt: %q", got)
	}
}
