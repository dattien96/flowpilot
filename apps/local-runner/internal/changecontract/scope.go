package changecontract

import (
	"context"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/structure"
)

// ScopeDiff computes the paths in diff that fall outside c's declared scope
// (SD-21 D-2): actual_touched \ declared_paths, doc/audit files excluded.
// A Contract with no declared_paths at all (e.g. a partial/malformed
// declaration) has nothing to compare against, so it yields no drift rather
// than flagging everything — Task-184's "degrade gracefully" contract.
//
// outSymbols is populated only when c.DeclaredSymbols is non-empty (no part
// of this codebase currently populates it — symbol-level declaration is a
// possible future extension, not implemented by parse.go/infer.go today).
// This intentionally does not extract symbols from diff hunks itself: doing
// so would require AST/code-graph work, which is exactly what SD-21 D-2
// avoids (that cost is what got SD-17 D-11 deferred in the first place).
func ScopeDiff(c Contract, diff []flowgate.ChangedFile, sp structure.Provider) (outPaths []string, outSymbols []string) {
	if len(c.DeclaredPaths) == 0 {
		return nil, nil
	}
	for _, f := range diff {
		path := strings.TrimSpace(f.Path)
		if path == "" || flowgate.IsDocOrAuditFile(path) {
			continue
		}
		if !matchesDeclaredScope(path, c.DeclaredPaths) {
			outPaths = append(outPaths, path)
		}
	}
	if len(c.DeclaredSymbols) > 0 && sp != nil && sp.Available() {
		// No symbol extraction from diff hunks exists yet, so there is
		// nothing concrete to diff declared symbols against — left as a
		// documented no-op until a symbol-extraction source exists.
		_ = sp
	}
	return outPaths, outSymbols
}

// matchesDeclaredScope reports whether path is covered by any entry in
// declared: an exact match, a directory-prefix match (covers InferFromDiff's
// top-level-dir buckets), or a filepath.Match glob against the full path or
// just its base name.
func matchesDeclaredScope(path string, declared []string) bool {
	path = filepath.ToSlash(path)
	for _, d := range declared {
		d = filepath.ToSlash(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if path == d {
			return true
		}
		if strings.HasPrefix(path, d+"/") {
			return true
		}
		if ok, _ := filepath.Match(d, path); ok {
			return true
		}
		if ok, _ := filepath.Match(d, filepath.Base(path)); ok {
			return true
		}
	}
	return false
}

// HighSeverity reports whether any out-of-scope path has dependents per sp
// (SD-21 §6 T-4): an out-of-scope edit to something nothing else depends on
// stays a low-severity warn; one with dependents may escalate to block, but
// only when structure is actually available (D-5/Q-2 — file-level truth
// alone never justifies a block). Non-fatal: a Dependents error for one path
// is skipped, not treated as high severity.
func HighSeverity(ctx context.Context, sp structure.Provider, outPaths []string) bool {
	if sp == nil || !sp.Available() {
		return false
	}
	for _, p := range outPaths {
		summary, err := sp.Dependents(ctx, p)
		if err != nil {
			continue
		}
		if summary.Count > 0 {
			return true
		}
	}
	return false
}
