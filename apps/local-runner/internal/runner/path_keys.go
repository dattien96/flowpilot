package runner

import (
	"path/filepath"
	"runtime"
	"strings"
)

func canonicalPathKey(value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	if cleaned == "." || cleaned == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func samePath(a, b string) bool {
	return canonicalPathKey(a) != "" && canonicalPathKey(a) == canonicalPathKey(b)
}

// workspaceRelPath converts a provider-reported file path to workspace-relative
// slash form. Absolute paths under the workspace relativize; absolute paths
// outside it (and everything already relative) pass through unchanged so audit
// trails keep the provider's original signal (BUG-463).
func workspaceRelPath(cwd, p string) string {
	p = strings.TrimSpace(p)
	ws := strings.TrimSpace(cwd)
	if p == "" || ws == "" {
		return p
	}
	normalized := filepath.FromSlash(strings.ReplaceAll(p, `\`, `/`))
	if !filepath.IsAbs(normalized) {
		return filepath.ToSlash(normalized)
	}
	rel, err := filepath.Rel(ws, normalized)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(normalized)
	}
	return filepath.ToSlash(rel)
}

func workspaceRelPaths(cwd string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, workspaceRelPath(cwd, p))
	}
	return out
}
