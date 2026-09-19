package skillpack

import (
	"strings"
	"testing"
)

// reactNativeScaffoldSkills is the exact declared set in
// flow-pack/react-native/scaffold.yaml (4 skills, CP-68 P-1).
var reactNativeScaffoldSkills = []string{
	"react-native-scaffold-bootstrap",
	"react-native-mobile-plumbing",
	"react-native-core-ui-tokens",
	"react-native-screen-archetypes",
}

func TestLoadScaffoldRecipe_ReactNative(t *testing.T) {
	recipe, ok, err := LoadScaffoldRecipe("react-native")
	if err != nil {
		t.Fatalf("LoadScaffoldRecipe(react-native) error = %v, want nil (fail-safe)", err)
	}
	if !ok || recipe == nil {
		t.Fatalf("LoadScaffoldRecipe(react-native) ok=%v recipe=%v, want found recipe", ok, recipe)
	}
	if recipe.Platform != "react-native" {
		t.Fatalf("platform = %q, want react-native", recipe.Platform)
	}
	if !recipe.Enabled {
		t.Fatal("react-native recipe must be enabled")
	}
	if recipe.Version < 1 {
		t.Fatalf("version = %d, want >= 1", recipe.Version)
	}
	if len(recipe.ScaffoldSkills) != len(reactNativeScaffoldSkills) {
		t.Fatalf("scaffold_skills = %v, want exactly %v", recipe.ScaffoldSkills, reactNativeScaffoldSkills)
	}
	for i, want := range reactNativeScaffoldSkills {
		if recipe.ScaffoldSkills[i] != want {
			t.Fatalf("scaffold_skills[%d] = %q, want %q (order is part of the contract)", i, recipe.ScaffoldSkills[i], want)
		}
	}
	if recipe.VerificationGate.Command != "pnpm install && pnpm tsc --noEmit" {
		t.Fatalf("verification command = %q", recipe.VerificationGate.Command)
	}
	if recipe.VerificationGate.TimeoutSeconds != 300 {
		t.Fatalf("verification timeout = %d, want 300", recipe.VerificationGate.TimeoutSeconds)
	}
	if recipe.VerificationGate.Cap != 3 {
		t.Fatalf("verification cap = %d, want 3", recipe.VerificationGate.Cap)
	}
	if strings.TrimSpace(recipe.Description) == "" {
		t.Fatal("description must be populated")
	}
}

func TestLoadScaffoldRecipe_MissingPlatform(t *testing.T) {
	for _, platform := range []string{"vuejs", "ruby", "unknown", "", "   ", "none"} {
		t.Run(platform, func(t *testing.T) {
			recipe, ok, err := LoadScaffoldRecipe(platform)
			if err != nil {
				t.Fatalf("LoadScaffoldRecipe(%q) error = %v, want nil (graceful ignore)", platform, err)
			}
			if ok || recipe != nil {
				t.Fatalf("LoadScaffoldRecipe(%q) = (%v, %v), want (nil, false)", platform, recipe, ok)
			}
		})
	}
}

func TestLoadScaffoldRecipe_NormalizesPlatformInput(t *testing.T) {
	for _, platform := range []string{"react-native", "React-Native", "  react-native  ", "REACT-NATIVE"} {
		recipe, ok, err := LoadScaffoldRecipe(platform)
		if err != nil || !ok || recipe == nil {
			t.Fatalf("LoadScaffoldRecipe(%q) = (%v, %v, %v), want found recipe", platform, recipe, ok, err)
		}
	}
}

func TestVerifyRecipeSkills_IntegrityPass(t *testing.T) {
	recipe, ok, err := LoadScaffoldRecipe("react-native")
	if err != nil || !ok || recipe == nil {
		t.Fatalf("load react-native recipe: ok=%v err=%v", ok, err)
	}
	missing, verified := VerifyRecipeSkills(recipe)
	if !verified {
		t.Fatalf("react-native recipe skills must all exist in the pack, missing = %v", missing)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want empty", missing)
	}
}

func TestVerifyRecipeSkills_ReportsMissingSkills(t *testing.T) {
	// Near-miss: everything else about the recipe is valid, only one declared
	// skill is absent from the pack. The validator must name it, not just fail.
	recipe := &ScaffoldRecipe{
		Platform:       "react-native",
		Enabled:        true,
		ScaffoldSkills: []string{"react-native-scaffold-bootstrap", "react-native-does-not-exist"},
	}
	missing, ok := VerifyRecipeSkills(recipe)
	if ok {
		t.Fatalf("ok = true, want false for missing skill; missing = %v", missing)
	}
	if len(missing) != 1 || missing[0] != "react-native-does-not-exist" {
		t.Fatalf("missing = %v, want [react-native-does-not-exist]", missing)
	}

	// Degraded input: nil recipe must not panic and must report not-ok.
	missing, ok = VerifyRecipeSkills(nil)
	if ok || missing != nil {
		t.Fatalf("VerifyRecipeSkills(nil) = (%v, %v), want (nil, false)", missing, ok)
	}
}

func TestVerifyRecipeSkills_UsesRecipePlatform(t *testing.T) {
	// A skill that exists under react-native must not be considered present for a
	// platform it was never shipped to — the validator is platform-scoped.
	recipe := &ScaffoldRecipe{
		Platform:       "vuejs",
		Enabled:        true,
		ScaffoldSkills: []string{"react-native-scaffold-bootstrap"},
	}
	missing, ok := VerifyRecipeSkills(recipe)
	if ok || len(missing) != 1 {
		t.Fatalf("cross-platform verification must fail: missing=%v ok=%v", missing, ok)
	}
}

func TestHasScaffoldCapability(t *testing.T) {
	cases := []struct {
		platform string
		want     bool
	}{
		{"react-native", true},
		{"React-Native", true},
		{"vuejs", false},
		{"ruby", false},
		{"android", false},
		{"", false},
		{"unknown", false},
		{"none", false},
	}
	for _, tc := range cases {
		if got := HasScaffoldCapability(tc.platform); got != tc.want {
			t.Fatalf("HasScaffoldCapability(%q) = %v, want %v", tc.platform, got, tc.want)
		}
	}
}

func TestScaffoldYAMLIsNotTreatedAsSkill(t *testing.T) {
	// Regression guard (Task-383 constraint): scaffold.yaml lives inside the
	// platform group directory, and skillsForPlatform reads directory entries —
	// only directories may become skills, so the manifest must never be installed
	// as a skill folder and the react-native skill count must stay 23.
	names, err := SkillNames("react-native")
	if err != nil {
		t.Fatalf("SkillNames(react-native) error = %v", err)
	}
	if len(names) != 23 {
		t.Fatalf("react-native skills = %d (%v), want 23 (13 common + 10 platform)", len(names), names)
	}
	for _, name := range names {
		if strings.Contains(name, "scaffold.yaml") {
			t.Fatalf("scaffold.yaml leaked into skill names: %v", names)
		}
	}

	// Every declared scaffold skill must also be reachable via the normal
	// install path, otherwise "attach skill" would silently no-op.
	installed := map[string]bool{}
	for _, name := range names {
		installed[name] = true
	}
	for _, skill := range reactNativeScaffoldSkills {
		if !installed[skill] {
			t.Fatalf("declared scaffold skill %q is not in the installable skill set", skill)
		}
	}
}
