package featurecatalog

import (
	"sort"
	"strings"
)

type Candidate struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`
}

// ResolveFeature scores catalog features against a natural-language query using
// lexical matching only. LLM-based disambiguation is the orchestrator's responsibility.
func ResolveFeature(nl string, catalog *Catalog) ([]Candidate, error) {
	queryTokens := tokenize(nl)
	nlLower := strings.ToLower(nl)

	var candidates []Candidate
	for _, feat := range catalog.All() {
		var score float64

		keywordSet := make(map[string]struct{}, len(feat.Keywords))
		for _, kw := range feat.Keywords {
			keywordSet[kw] = struct{}{}
		}

		for _, qt := range queryTokens {
			if _, ok := keywordSet[qt]; ok {
				score += 3.0
			}
		}

		if strings.Contains(nlLower, strings.ToLower(feat.Key)) {
			score += 5.0
		}

		if feat.Title != "" && strings.Contains(nlLower, strings.ToLower(feat.Title)) {
			score += 5.0
		}

		// BUG-417: award the glob-substring hit ONCE per feature, not per
		// token. Auto-cataloged noise features (a chore commit's ~100-entry
		// file_globs covering most of the repo) matched nearly every query
		// token — coverage breadth amplified into dominance over registered
		// FEATURE-KEYS.md entries whose real signal is keyword/key/title
		// matches. One hit still credits a genuinely glob-relevant feature;
		// breadth no longer compounds.
		globStr := strings.ToLower(strings.Join(feat.FileGlobs, " "))
		if globStr != "" {
			for _, qt := range queryTokens {
				if strings.Contains(globStr, qt) {
					score += 2.0
					break
				}
			}
		}

		if score > 0 {
			candidates = append(candidates, Candidate{Key: feat.Key, Score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

// TopCandidate returns the highest-scoring candidate if its score meets threshold.
func TopCandidate(candidates []Candidate, threshold float64) (Candidate, bool) {
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	if candidates[0].Score >= threshold {
		return candidates[0], true
	}
	return Candidate{}, false
}

// SuggestKey ranks keys using the files actually changed plus the natural-language
// intent. It is deterministic and reuses the same lexical score surface as
// ResolveFeature, with path matches weighted above pure text hints.
func SuggestKey(changedPaths []string, message string, catalog *Catalog) []Candidate {
	scores := make(map[string]float64)

	for _, c := range mustResolve(message, catalog) {
		scores[c.Key] += c.Score
	}

	changed := normalizePaths(changedPaths)
	for _, feat := range catalog.All() {
		for _, glob := range feat.FileGlobs {
			if glob == "" {
				continue
			}
			prefix := globPrefix(glob)
			for _, path := range changed {
				if prefix != "" && strings.HasPrefix(path, prefix) {
					scores[feat.Key] += 6.0
					break
				}
			}
		}
	}

	out := make([]Candidate, 0, len(scores))
	for key, score := range scores {
		if score > 0 {
			out = append(out, Candidate{Key: key, Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Key < out[j].Key
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func mustResolve(message string, catalog *Catalog) []Candidate {
	candidates, err := ResolveFeature(message, catalog)
	if err != nil {
		return nil
	}
	return candidates
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(p, "\\", "/")))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func globPrefix(glob string) string {
	glob = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(glob), "\\", "/"))
	glob = strings.TrimSuffix(glob, "/**")
	glob = strings.TrimSuffix(glob, "/*")
	return glob
}
