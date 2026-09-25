package featurecatalog

import (
	"fmt"
	"testing"
)

// TestBug417_MegaGlobFeatureDoesNotDominate pins the live defect from
// BUG-LIVE-CP37-003: a chore commit auto-cataloged feature `claude` with ~100
// file_globs (calc_test.go, stringutil/**, requirements/**, .claude/skills/**,
// …). ResolveFeature's +2-per-token glob-substring rule let that coverage
// breadth out-score the real registered feature on almost any query —
// bucketing, reprompt suggestions, and canonical-head injection all hijacked.
func TestBug417_MegaGlobFeatureDoesNotDominate(t *testing.T) {
	cat := New()
	cat.Add(Feature{
		Key:       "calc-core",
		Title:     "Calculator core",
		Keywords:  []string{"calc"},
		FileGlobs: []string{"calc*.go", "internal/calc/**"},
	})
	// The noise feature: ledger-key-only entry whose inferred globs blanket the
	// repo — every query token hits some glob substring.
	globs := []string{"requirements/**", ".claude/skills/**", "stringutil/**", "docs/calculator.md"}
	for i := 0; i < 97; i++ {
		globs = append(globs, fmt.Sprintf("area%d/fixture%d_calc_divide_guard.go", i, i))
	}
	cat.Add(Feature{Key: "claude", FileGlobs: globs})

	candidates, err := ResolveFeature("fix the calc divide guard in the calculator", cat)
	if err != nil {
		t.Fatalf("ResolveFeature: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if candidates[0].Key != "calc-core" {
		t.Fatalf("mega-glob noise feature %q out-scored the registered feature (score %.1f vs calc-core)",
			candidates[0].Key, candidates[0].Score)
	}
}
