package changecontract

import (
	"sort"
	"strings"

	"flowpilot-runner/internal/flowgate"
)

// InferFromDiff synthesizes a Contract when the AI did not emit a Change
// Contract declaration (T-4). declared_paths is the distinct set of
// top-level directories touched by non-doc/audit files in diff, so
// Task-185's scope-drift check still has something to compare against.
// Confidence is always ConfidenceInferred; callers (T-3) decide whether to
// confirm it with the user (only under gate_mode=enforce, SD-21 D-7).
func InferFromDiff(featureKey string, diff []flowgate.ChangedFile) Contract {
	seen := make(map[string]struct{})
	for _, f := range diff {
		path := strings.TrimSpace(f.Path)
		if path == "" || flowgate.IsDocOrAuditFile(path) {
			continue
		}
		seen[topLevelDir(path)] = struct{}{}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		paths = nil
	}

	return Contract{
		FeatureKey:    featureKey,
		DeclaredPaths: paths,
		Confidence:    ConfidenceInferred,
	}
}

// topLevelDir returns the first path segment of a forward-slash path (e.g.
// "apps/local-runner/foo.go" -> "apps"). A root-level file with no directory
// component (e.g. "go.mod") buckets to itself.
func topLevelDir(path string) string {
	path = strings.TrimPrefix(path, "/")
	if idx := strings.Index(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return path
}
