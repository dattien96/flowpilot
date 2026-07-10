// CP-55 P-2: shared declared-path normalization. Runner's retrieval-locus
// builder (CP-54 P-2 / Task-262) and this package's frozen-contract freeze
// path both need identical "is this a concrete code target" semantics, so
// the predicate lives here once and runner calls it rather than keeping a
// second copy that could silently drift.
package changecontract

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/flowgate"
)

// IsConcreteCodeTarget reports whether p names a specific code file — not a
// directory bucket ("apps", "internal" — what inference produces), a glob,
// a flag-like string (guards a downstream tool runner from reading "-x" as an
// option), or a doc/audit file. Anything else matches nearly every commit,
// which is the dilution CP-54 exists to remove.
func IsConcreteCodeTarget(p string) bool {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return false
	case strings.HasPrefix(p, "-"):
		return false
	case strings.ContainsAny(p, "*?["):
		return false
	case filepath.Ext(p) == "":
		return false
	case flowgate.IsDocOrAuditFile(p):
		return false
	}
	return true
}

// NormalizeDeclaredCodePaths validates and normalizes a frozen-contract
// declared-path list (CP-55 P-2 §3.4).
//
// Two different kinds of rejection happen here, deliberately not treated the
// same way:
//   - Workspace escape (absolute paths outside workspace, ../ traversal, a
//     symlink that resolves outside workspace when the target already exists
//     on disk) is a security boundary — any offending entry fails the whole
//     call.
//   - "Not a concrete code target" (glob, directory bucket, doc/audit file,
//     flag-like string) is noise, not an attack — each such entry is dropped
//     silently via IsConcreteCodeTarget, the same way buildRetrievalLocus
//     already treats it. A frozen writer contract still requires at least one
//     concrete path to survive; an all-noise input (all globs, all docs) is
//     reported as the same "no concrete code path" error the empty-input case
//     gets, not as a per-entry error, since there is nothing more specific to
//     name once every entry has been filtered.
//
// Survivors are forward-slash normalized, path.Clean'd, sorted and deduped
// (case preserved — this is not a case-insensitive filesystem operation).
func NormalizeDeclaredCodePaths(workspace string, paths []string) ([]string, error) {
	absWorkspace := ""
	if strings.TrimSpace(workspace) != "" {
		abs, err := filepath.Abs(workspace)
		if err != nil {
			return nil, fmt.Errorf("changecontract: resolve workspace %q: %w", workspace, err)
		}
		absWorkspace = abs
	}

	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		// IsAbs must run on the OS-native form: path.IsAbs (POSIX-only) never
		// recognizes a Windows drive-letter path like "C:/outside/x.go" once it
		// has been slash-normalized, which would let it slip through unrejected.
		native := filepath.Clean(filepath.FromSlash(trimmed))
		if filepath.IsAbs(native) {
			if absWorkspace == "" {
				return nil, fmt.Errorf("changecontract: declared path %q is absolute but no workspace was given", raw)
			}
			rel, err := filepath.Rel(absWorkspace, native)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("changecontract: declared path %q escapes workspace %q", raw, workspace)
			}
			native = rel
		}
		p := path.Clean(filepath.ToSlash(native))
		if p == "" || p == "." {
			continue
		}
		if p == ".." || strings.HasPrefix(p, "../") {
			return nil, fmt.Errorf("changecontract: declared path %q escapes workspace", raw)
		}
		if absWorkspace != "" {
			// Symlink escape check "when resolvable" — a path that does not
			// exist yet (the common case: the coder hasn't created it) simply
			// skips this check rather than failing.
			if resolved, err := filepath.EvalSymlinks(filepath.Join(absWorkspace, filepath.FromSlash(p))); err == nil {
				rel, relErr := filepath.Rel(absWorkspace, resolved)
				if relErr != nil || rel == ".." || strings.HasPrefix(rel, "../") {
					return nil, fmt.Errorf("changecontract: declared path %q resolves outside workspace via symlink", raw)
				}
			}
		}
		if !IsConcreteCodeTarget(p) {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, errors.New("changecontract: no concrete code path declared")
	}
	return out, nil
}
