package skillpack

import (
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

// VerificationGateConfig is the Compiler Verification Gate contract declared by a
// platform scaffold recipe (CP-68 P-4): the command the runner must run before a
// scaffold is considered done, how long it may take, and how many self-healing
// AI attempts are allowed when it fails.
type VerificationGateConfig struct {
	Command        string `yaml:"command" json:"command"`
	TimeoutSeconds int    `yaml:"timeout_seconds" json:"timeoutSeconds"`
	Cap            int    `yaml:"cap" json:"cap"`
}

// ScaffoldRecipe mirrors one `flow-pack/<platform>/scaffold.yaml` document. The
// runner never hardcodes platform or skill names: everything it needs to decide
// "is AI scaffolding supported for this platform?" comes from this document.
type ScaffoldRecipe struct {
	Platform         string                 `yaml:"platform" json:"platform"`
	Version          int                    `yaml:"version" json:"version"`
	Enabled          bool                   `yaml:"enabled" json:"enabled"`
	Description      string                 `yaml:"description" json:"description"`
	ScaffoldSkills   []string               `yaml:"scaffold_skills" json:"scaffoldSkills"`
	VerificationGate VerificationGateConfig `yaml:"verification_gate" json:"verificationGate"`
}

// scaffoldRecipePath is the embedded manifest location for a platform recipe.
func scaffoldRecipePath(platform string) string {
	return "flow-pack/" + normalizePlatform(platform) + "/scaffold.yaml"
}

// scaffoldSkillPath is the embedded location of one declared blueprint skill's
// SKILL.md, used by VerifyRecipeSkills to prove the declared skills really ship
// with the pack.
func scaffoldSkillPath(platform string, skill string) string {
	return "flow-pack/" + normalizePlatform(platform) + "/" + strings.TrimSpace(skill) + "/SKILL.md"
}

// LoadScaffoldRecipe reads and parses `flow-pack/<platform>/scaffold.yaml` from
// the embedded pack.
//
// Fail-safe contract (Task-383 T-3): a missing manifest, an unparseable manifest,
// or an unknown/empty platform is NOT a fatal error. It degrades to
// (nil, false, nil) so the CP-34 static init flow is never interrupted. The
// error return is retained for interface symmetry and is always nil today.
//
// The bool reports whether a manifest was found and parsed — it does NOT report
// whether the recipe is enabled; callers that need "can we scaffold?" must check
// recipe.Enabled or call HasScaffoldCapability.
func LoadScaffoldRecipe(platform string) (*ScaffoldRecipe, bool, error) {
	normalized := normalizePlatform(platform)
	if normalized == "" {
		return nil, false, nil
	}

	raw, err := fs.ReadFile(flowPackFS, scaffoldRecipePath(normalized))
	if err != nil {
		// Missing recipe = platform has no AI scaffold support (graceful ignore).
		return nil, false, nil
	}

	var recipe ScaffoldRecipe
	if err := yaml.Unmarshal(raw, &recipe); err != nil {
		// A malformed manifest must not break init; treat it as "no recipe".
		return nil, false, nil
	}

	// A manifest without an explicit platform field still resolves to the
	// directory it was read from, so downstream logs are never empty.
	if strings.TrimSpace(recipe.Platform) == "" {
		recipe.Platform = normalized
	}

	return &recipe, true, nil
}

// VerifyRecipeSkills checks that every skill declared in recipe.ScaffoldSkills
// actually ships in the embedded pack for the recipe's platform (each skill
// directory must contain SKILL.md). It returns the declared-but-missing skill
// names and whether the recipe is complete.
func VerifyRecipeSkills(recipe *ScaffoldRecipe) (missing []string, ok bool) {
	if recipe == nil {
		return nil, false
	}

	missing = make([]string, 0)
	for _, skill := range recipe.ScaffoldSkills {
		name := strings.TrimSpace(skill)
		if name == "" {
			continue
		}
		if _, err := fs.Stat(flowPackFS, scaffoldSkillPath(recipe.Platform, name)); err != nil {
			missing = append(missing, name)
		}
	}

	return missing, len(missing) == 0
}

// HasScaffoldCapability reports whether AI scaffolding is fully supported for the
// platform: the recipe manifest exists, is enabled, declares at least one skill,
// and every declared skill is present in the pack.
//
// Any failure mode degrades to false so callers can "graceful ignore" the
// scaffold branch (CP-68 single-command design) instead of erroring.
func HasScaffoldCapability(platform string) bool {
	recipe, found, _ := LoadScaffoldRecipe(platform)
	if !found || recipe == nil || !recipe.Enabled {
		return false
	}
	if len(recipe.ScaffoldSkills) == 0 {
		return false
	}
	_, skillsOK := VerifyRecipeSkills(recipe)
	return skillsOK
}
