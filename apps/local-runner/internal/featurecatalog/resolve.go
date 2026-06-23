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

		globStr := strings.ToLower(strings.Join(feat.FileGlobs, " "))
		if globStr != "" {
			for _, qt := range queryTokens {
				if strings.Contains(globStr, qt) {
					score += 2.0
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
